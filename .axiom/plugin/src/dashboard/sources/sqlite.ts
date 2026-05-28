/**
 * SQLiteSource — reads real session data from OpenCode's SQLite database.
 *
 * This is the highest-priority data source (priority=4). It reads directly from
 * OpenCode's SQLite database at ~/.local/share/opencode/opencode.db, which
 * contains authoritative cost, token, model, title, duration, and directory
 * data for every session.
 *
 * REQ-TUI-060: SQLiteSource MUST read session data from the OpenCode SQLite
 *   database and return correctly shaped SessionRow objects.
 *
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-060
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-060 plan=trace-fix
 */

import { join } from "node:path"
import { existsSync } from "node:fs"
import type { SessionDataSource, SessionRow } from "../types"
import { resolveModelDisplayName } from "../model-aliases"

// ---------------------------------------------------------------------------
// SQLiteSessionRow — raw row shape returned by the aggregation query
// ---------------------------------------------------------------------------

/**
 * Raw row shape returned by the SQLite aggregation query.
 * All numeric fields come from SQLite's json_extract / SUM / ROUND.
 */
export interface SQLiteSessionRow {
  id: string
  title: string
  directory: string
  message_count: number
  total_cost: number
  total_tokens: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  reasoning_tokens: number
  tool_calls: number
  last_model: string | null
  time_created: number
  time_updated: number
  duration_seconds: number
}

// ---------------------------------------------------------------------------
// SQLiteSourceOptions
// ---------------------------------------------------------------------------

/**
 * Options for constructing a SQLiteSource.
 *
 * `_queryRunner` is an injection point for testing — when provided, no real
 * SQLite I/O is performed. The injected function receives the SQL string and
 * returns an array of SQLiteSessionRow objects directly.
 */
export interface SQLiteSourceOptions {
  /**
   * Path to the OpenCode SQLite database.
   * Defaults to ~/.local/share/opencode/opencode.db
   */
  dbPath?: string
  /**
   * Only show sessions for this directory (filter by session.directory prefix).
   * When not set, all sessions are returned.
   */
  filterDirectory?: string
  /**
   * Maximum number of sessions to return. Default: 50.
   */
  maxSessions?: number
  /**
   * Epoch-millisecond cutoff for time-range filtering (--since flag).
   * When set, the WHERE clause is injected into the CTE so filtering happens
   * at the DB level BEFORE the LIMIT — making --since correct on large DBs.
   *
   * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-082 plan=phase-10/task-10.1
   */
  sinceMs?: number
  /**
   * For testing: inject a custom query runner.
   * When provided, no real SQLite database is opened.
   */
  _queryRunner?: (sql: string) => SQLiteSessionRow[]
}

// ---------------------------------------------------------------------------
// SQL aggregation query
// ---------------------------------------------------------------------------

/**
 * Build the aggregation SQL query for the given limit and optional sinceMs cutoff.
 *
 * Returns session rows with aggregated cost, tokens, message count, and
 * the most recent assistant model ID.
 *
 * When sinceMs is provided, a WHERE clause is injected into the CTE so that
 * time-range filtering happens at the DB level BEFORE the LIMIT — making
 * --since correct on large databases (e.g., 31K sessions).
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-082 plan=phase-10/task-10.1
 */
function buildAggregationQuery(limit: number, sinceMs?: number): string {
  // Two-phase query: first get the N most recent sessions, then aggregate
  // only their messages. This avoids scanning all 500K+ messages.
  // Tool calls are counted from the `part` table (type='tool'), not messages.
  const sinceClause = sinceMs ? `AND time_updated >= ${sinceMs}` : ""
  return `
WITH recent_sessions AS (
  SELECT id, title, directory, time_created, time_updated
  FROM session
  WHERE 1=1 ${sinceClause}
  ORDER BY time_updated DESC
  LIMIT ${limit}
)
SELECT
  rs.id,
  rs.title,
  rs.directory,
  COUNT(CASE WHEN json_extract(m.data, '$.role') = 'assistant' THEN 1 END) as message_count,
  COALESCE(SUM(CASE WHEN json_extract(m.data, '$.role') = 'assistant' THEN json_extract(m.data, '$.cost') END), 0) as total_cost,
  COALESCE(SUM(CASE WHEN json_extract(m.data, '$.role') = 'assistant' THEN json_extract(m.data, '$.tokens.total') END), 0) as total_tokens,
  COALESCE(SUM(CASE WHEN json_extract(m.data, '$.role') = 'assistant' THEN json_extract(m.data, '$.tokens.input') END), 0) as input_tokens,
  COALESCE(SUM(CASE WHEN json_extract(m.data, '$.role') = 'assistant' THEN json_extract(m.data, '$.tokens.output') END), 0) as output_tokens,
  COALESCE(SUM(CASE WHEN json_extract(m.data, '$.role') = 'assistant' THEN json_extract(m.data, '$.tokens.cache.read') END), 0) as cache_read_tokens,
  COALESCE(SUM(CASE WHEN json_extract(m.data, '$.role') = 'assistant' THEN json_extract(m.data, '$.tokens.cache.write') END), 0) as cache_write_tokens,
  COALESCE(SUM(CASE WHEN json_extract(m.data, '$.role') = 'assistant' THEN json_extract(m.data, '$.tokens.reasoning') END), 0) as reasoning_tokens,
  COALESCE((SELECT COUNT(*) FROM part WHERE session_id = rs.id AND json_extract(data, '$.type') = 'tool'), 0) as tool_calls,
  json_extract(
    (SELECT data FROM message WHERE session_id = rs.id AND json_extract(data, '$.role') = 'assistant' ORDER BY time_created DESC LIMIT 1),
    '$.modelID'
  ) as last_model,
  rs.time_created,
  rs.time_updated,
  ROUND((rs.time_updated - rs.time_created) / 1000.0) as duration_seconds
FROM recent_sessions rs
LEFT JOIN message m ON m.session_id = rs.id
GROUP BY rs.id
ORDER BY rs.time_updated DESC
`.trim()
}

/**
 * Build the per-model usage aggregation query.
 * Uses the same CTE to limit to recent sessions first.
 *
 * When sinceMs is provided, the CTE WHERE clause filters by time_updated
 * at the DB level before the LIMIT — consistent with buildAggregationQuery.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-082 plan=phase-10/task-10.1
 */
function buildModelUsageQuery(limit: number, sinceMs?: number): string {
  const sinceClause = sinceMs ? `AND time_updated >= ${sinceMs}` : ""
  return `
WITH recent_sessions AS (
  SELECT id FROM session WHERE 1=1 ${sinceClause} ORDER BY time_updated DESC LIMIT ${limit}
)
SELECT
  json_extract(m.data, '$.modelID') as model_id,
  json_extract(m.data, '$.providerID') as provider_id,
  COUNT(DISTINCT m.session_id) as session_count,
  COUNT(*) as call_count,
  COALESCE(SUM(json_extract(m.data, '$.cost')), 0) as total_cost,
  COALESCE(SUM(json_extract(m.data, '$.tokens.output')), 0) as output_tokens,
  COALESCE(SUM(json_extract(m.data, '$.tokens.input')), 0) as input_tokens,
  COALESCE(SUM(json_extract(m.data, '$.tokens.cache.read')), 0) as cache_read_tokens,
  COALESCE(SUM(json_extract(m.data, '$.tokens.reasoning')), 0) as reasoning_tokens
FROM message m
WHERE m.session_id IN (SELECT id FROM recent_sessions)
AND json_extract(m.data, '$.role') = 'assistant'
AND json_extract(m.data, '$.modelID') IS NOT NULL
GROUP BY model_id
ORDER BY total_cost DESC
`.trim()
}

export interface SQLiteModelUsageRow {
  model_id: string
  provider_id: string
  session_count: number
  call_count: number
  total_cost: number
  output_tokens: number
  input_tokens: number
  cache_read_tokens: number
  reasoning_tokens: number
}

// ---------------------------------------------------------------------------
// Status heuristic
// ---------------------------------------------------------------------------

/**
 * Determine session status from time_updated.
 *
 * If the session was updated within the last 5 minutes, it is considered
 * "running". Otherwise it is "stopped".
 *
 * This is a simple heuristic — the SQLite DB does not store explicit status.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-060 plan=trace-fix
 */
function inferStatus(timeUpdatedMs: number): "running" | "stopped" {
  const fiveMinutesMs = 5 * 60 * 1000
  const age = Date.now() - timeUpdatedMs
  return age <= fiveMinutesMs ? "running" : "stopped"
}

// ---------------------------------------------------------------------------
// Work item ID extraction
// ---------------------------------------------------------------------------

/**
 * Attempt to extract a work item ID from a session title.
 *
 * Matches common patterns:
 * - Jira-style keys: "ABC-123", "DEX-205", "AUTH-1234"
 * - Worktree prefixes: "WORKTREE ABC-123 ..."
 *
 * Returns undefined when no work item ID is found.
 */
function extractWorkItemId(title: string): string | undefined {
  if (!title) return undefined
  // Match Jira-style keys: 2-10 uppercase letters, dash, 1-6 digits
  const match = title.match(/\b([A-Z]{2,10}-\d{1,6})\b/)
  return match ? match[1] : undefined
}

// ---------------------------------------------------------------------------
// Default DB path
// ---------------------------------------------------------------------------

/**
 * Resolve the default OpenCode SQLite database path.
 * Uses HOME env var to expand ~.
 */
function defaultDbPath(): string {
  const home = process.env.HOME ?? process.env.USERPROFILE ?? "~"
  return join(home, ".local", "share", "opencode", "opencode.db")
}

// ---------------------------------------------------------------------------
// SQLiteSource
// ---------------------------------------------------------------------------

/**
 * SQLiteSource — highest-priority SessionDataSource (priority=4).
 *
 * Reads session data directly from OpenCode's SQLite database. This source
 * is more authoritative than file-based sources because it contains the
 * complete, aggregated session history with real cost and token data.
 *
 * - id: 'sqlite', priority: 4
 * - Opens DB read-only (no writes, no locking side effects)
 * - Returns [] gracefully when DB is unavailable or query fails
 * - Exposes getLastError() for diagnostics
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-060 plan=trace-fix
 */
export class SQLiteSource implements SessionDataSource {
  readonly id = "sqlite" as const
  readonly priority = 4

  private readonly dbPath: string
  private readonly filterDirectory: string | undefined
  private readonly maxSessions: number
  private readonly sinceMs: number | undefined
  private readonly queryRunner: ((sql: string) => SQLiteSessionRow[]) | undefined

  private lastUpdate: Date | null = null
  private lastError: string | null = null

  constructor(options: SQLiteSourceOptions = {}) {
    this.dbPath = options.dbPath ?? defaultDbPath()
    this.filterDirectory = options.filterDirectory
    this.maxSessions = options.maxSessions ?? 50
    this.sinceMs = options.sinceMs
    this.queryRunner = options._queryRunner
  }

  /**
   * Returns true when the SQLite database file exists on disk.
   *
   * When _queryRunner is injected (test mode), always returns true unless
   * the dbPath is explicitly set to a non-existent path.
   *
   * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-060 plan=trace-fix
   */
  isAvailable(): boolean {
    // When a custom query runner is injected, check if dbPath exists
    // (test can set dbPath to a non-existent path to simulate unavailability)
    return existsSync(this.dbPath)
  }

  /**
   * Returns the timestamp of the most recently updated session from the last
   * successful query, or null if no query has succeeded yet.
   */
  getLastUpdate(): Date | null {
    return this.lastUpdate
  }

  /**
   * Returns the last error message from a failed query, or null.
   *
   * Used for diagnostics when getSessions() returns [].
   */
  getLastError(): string | null {
    return this.lastError
  }

  /**
   * Fetch sessions from the OpenCode SQLite database.
   *
   * - Opens the DB read-only (no side effects on the OpenCode DB)
   * - Runs the aggregation query
   * - Converts each row to a SessionRow
   * - Applies filterDirectory when set
   * - Closes the DB after the query
   * - Returns [] on any error (graceful degradation)
   *
   * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-060 plan=trace-fix
   */
  async getSessions(): Promise<SessionRow[]> {
    // When _queryRunner is injected, use it directly (test mode)
    if (this.queryRunner) {
      return this._runQuery(this.queryRunner)
    }

    // Real mode: check DB exists before attempting to open
    if (!existsSync(this.dbPath)) {
      return []
    }

    // Open DB read-only using bun:sqlite
    let db: import("bun:sqlite").Database | null = null
    try {
      // Dynamic import to avoid bundler issues when bun:sqlite is unavailable
      const { Database } = await import("bun:sqlite")
      db = new Database(this.dbPath, { readonly: true })
      const runner = (sql: string): SQLiteSessionRow[] => {
        // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-060 plan=trace-fix
        const stmt = db!.prepare(sql)
        return stmt.all() as SQLiteSessionRow[]
      }
      return this._runQuery(runner)
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err)
      this.lastError = message
      return []
    } finally {
      if (db !== null) {
        try {
          db.close()
        } catch {
          // Ignore close errors
        }
      }
    }
  }

  /**
   * Internal: run the aggregation query using the provided runner, convert
   * rows to SessionRow[], apply directory filter, and update lastUpdate.
   *
   * Extracted so both real and injected (test) paths share the same logic.
   *
   * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-060 plan=trace-fix
   */
  private _runQuery(runner: (sql: string) => SQLiteSessionRow[]): SessionRow[] {
    let rawRows: SQLiteSessionRow[]
    try {
      // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-082 plan=phase-10/task-10.1
      const sql = buildAggregationQuery(this.maxSessions, this.sinceMs)
      rawRows = runner(sql)
      this.lastError = null
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err)
      this.lastError = message
      return []
    }

    // Apply directory filter when set
    const filtered = this.filterDirectory
      ? rawRows.filter((r) => r.directory.startsWith(this.filterDirectory!))
      : rawRows

    const rows: SessionRow[] = []
    let latestUpdated: Date | null = null

    for (const raw of filtered) {
      const updatedAt = new Date(raw.time_updated)

      if (latestUpdated === null || updatedAt > latestUpdated) {
        latestUpdated = updatedAt
      }

      const durationSec = raw.duration_seconds > 0 ? raw.duration_seconds : 1
      const outputTok = raw.output_tokens ?? 0

      // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-060 plan=trace-fix
      const row: SessionRow = {
        sessionId: raw.id,
        title: raw.title,
        workItemId: extractWorkItemId(raw.title),
        model: resolveModelDisplayName(raw.last_model ?? ""),
        modelRaw: raw.last_model ?? "",
        costUsd: raw.total_cost,
        totalTokens: raw.total_tokens,
        durationSeconds: raw.duration_seconds,
        status: inferStatus(raw.time_updated),
        source: "sqlite",
        updatedAt,
        isTeam: false,
        directory: raw.directory,
        // Extended token breakdown
        inputTokens: raw.input_tokens ?? 0,
        outputTokens: outputTok,
        cacheReadTokens: raw.cache_read_tokens ?? 0,
        cacheWriteTokens: raw.cache_write_tokens ?? 0,
        reasoningTokens: raw.reasoning_tokens ?? 0,
        toolCalls: raw.tool_calls ?? 0,
        messageCount: raw.message_count ?? 0,
        tokensPerSecond: outputTok > 0 ? Math.round(outputTok / durationSec) : 0,
      }

      rows.push(row)
    }

    if (latestUpdated !== null) {
      this.lastUpdate = latestUpdated
    }

    return rows
  }

  /**
   * Fetch per-model usage breakdown for the htop-style header meters.
   */
  async getModelUsage(): Promise<import("../types").ModelUsage[]> {
    if (this.queryRunner) {
      // Test mode — no model usage query available via _queryRunner
      return []
    }
    if (!existsSync(this.dbPath)) return []

    let db: import("bun:sqlite").Database | null = null
    try {
      const { Database } = await import("bun:sqlite")
      db = new Database(this.dbPath, { readonly: true })
      const sql = buildModelUsageQuery(this.maxSessions, this.sinceMs)
      const rows = db.prepare(sql).all() as SQLiteModelUsageRow[]
      return rows.map((r) => ({
        model: resolveModelDisplayName(r.model_id ?? ""),
        modelRaw: r.model_id ?? "",
        provider: r.provider_id ?? "",
        sessions: r.session_count ?? 0,
        calls: r.call_count ?? 0,
        costUsd: r.total_cost ?? 0,
        outputTokens: r.output_tokens ?? 0,
        inputTokens: r.input_tokens ?? 0,
        cacheReadTokens: r.cache_read_tokens ?? 0,
        reasoningTokens: r.reasoning_tokens ?? 0,
      }))
    } catch {
      return []
    } finally {
      try { db?.close() } catch { /* ignore */ }
    }
  }
}

// ---------------------------------------------------------------------------
// Factory helper
// ---------------------------------------------------------------------------

/**
 * Convenience factory for constructing a SQLiteSource.
 *
 * @example
 * ```typescript
 * const source = createSQLiteSource({
 *   filterDirectory: repoRoot,
 *   maxSessions: 50,
 * })
 * ```
 */
export function createSQLiteSource(options: SQLiteSourceOptions = {}): SQLiteSource {
  return new SQLiteSource(options)
}
