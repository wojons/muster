/**
 * TUI Dashboard — session data collector.
 *
 * Merges all `SessionDataSource` implementations, deduplicates sessions by
 * session ID (higher-priority source wins), and computes aggregate stats for
 * the dashboard stats bar.
 *
 * ## Key exports
 * - `SessionDataCollector` — class with a single `collect(options)` method that
 *   polls all configured sources, deduplicates, sorts, and returns a
 *   fully-assembled `DashboardData` payload.
 * - `createSessionDataCollector()` — convenience factory that returns a new
 *   `SessionDataCollector` instance.
 *
 * ## Source priority order (higher wins on duplicate sessionId)
 * 1. `in-memory` (priority 3) — live data from the running OpenCode process.
 * 2. `file` (priority 2) — persisted session JSON files in `.axiom/sessions/`.
 * 3. `network` (priority 1) — remote team data from the Axiom backend API.
 *
 * ## Error handling
 * Source errors are caught gracefully: a failing source is marked unavailable
 * and the collector continues with the remaining sources.
 *
 * REQ-TUI-003: The command MUST display an aggregate stats bar showing:
 *   total active sessions, total cost, total tokens, average speed.
 * REQ-TUI-011: The table MUST compute derived fields at render time.
 *
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-003
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-011
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-003 plan=phase-1/task-1.4
 */

import type {
  CollectorOptions,
  DashboardAggregates,
  DashboardData,
  SessionDataSource,
  SessionRow,
  SourceStatus,
} from "./types"

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

/**
 * Deduplicate sessions by sessionId, keeping the row from the highest-priority
 * source when the same sessionId appears in multiple sources.
 *
 * Priority order: in-memory (3) > file (2) > network (1).
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-003 plan=phase-1/task-1.4
 */
function deduplicateSessions(
  allRows: Array<{ row: SessionRow; priority: number }>
): SessionRow[] {
  // Map from sessionId → { row, priority } — keep highest priority
  const best = new Map<string, { row: SessionRow; priority: number }>()

  for (const entry of allRows) {
    const existing = best.get(entry.row.sessionId)
    if (existing === undefined || entry.priority > existing.priority) {
      best.set(entry.row.sessionId, entry)
    }
  }

  return Array.from(best.values()).map((e) => e.row)
}

/**
 * Compute aggregate statistics across all non-team sessions.
 *
 * - `totalSessions`: count of non-team sessions
 * - `totalCostUsd`: sum of costUsd for non-team sessions
 * - `totalTokens`: sum of totalTokens for non-team sessions
 * - `avgSpeedTokensPerSec`: totalTokens / sum(durationSeconds); 0 when duration is 0
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-003 plan=phase-1/task-1.4
 */
function computeAggregates(sessions: SessionRow[]): DashboardAggregates {
  const nonTeam = sessions.filter((s) => !s.isTeam)

  let totalCostUsd = 0
  let totalTokens = 0
  let totalDurationSeconds = 0

  for (const session of nonTeam) {
    totalCostUsd += session.costUsd
    totalTokens += session.totalTokens
    totalDurationSeconds += session.durationSeconds
  }

  const avgSpeedTokensPerSec =
    totalDurationSeconds > 0 ? totalTokens / totalDurationSeconds : 0

  return {
    totalSessions: nonTeam.length,
    totalCostUsd,
    totalTokens,
    avgSpeedTokensPerSec,
  }
}

// ---------------------------------------------------------------------------
// SessionDataCollector
// ---------------------------------------------------------------------------

/**
 * SessionDataCollector — the central data aggregation layer for the TUI dashboard.
 *
 * Responsibilities:
 * 1. Calls `getSessions()` on each available source (skips unavailable sources).
 * 2. Deduplicates sessions by sessionId — higher-priority source wins.
 * 3. Computes aggregate stats (excluding team sessions).
 * 4. Returns a fully assembled `DashboardData` payload.
 *
 * Source errors are caught gracefully: a failing source is marked unavailable
 * and the collector continues with the remaining sources.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-003 plan=phase-1/task-1.4
 */
export class SessionDataCollector {
  /**
   * Collect and merge session data from all configured sources.
   *
   * @param options - CollectorOptions including optional injected sources.
   * @returns A fully assembled DashboardData payload.
   *
   * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-003 plan=phase-1/task-1.4
   */
  async collect(options: CollectorOptions): Promise<DashboardData> {
    // Resolve sources: use injected sources when provided, else default to empty
    // (callers are responsible for constructing InMemorySource / FileSource).
    const sources: SessionDataSource[] = options.sources ?? []

    // Track per-source status and collected rows
    const sourceStatusMap = new Map<string, SourceStatus>()
    const allRows: Array<{ row: SessionRow; priority: number }> = []

    // Initialize status for all known source IDs
    const knownSourceIds: Array<"in-memory" | "file" | "network"> = [
      "in-memory",
      "file",
      "network",
    ]
    for (const id of knownSourceIds) {
      sourceStatusMap.set(id, { available: false, lastUpdate: null })
    }

    // Poll each source
    for (const source of sources) {
      // Record availability before attempting to fetch
      const available = source.isAvailable()
      const lastUpdate = source.getLastUpdate()

      if (!available) {
        // Source is not available — skip getSessions() entirely
        sourceStatusMap.set(source.id, {
          available: false,
          lastUpdate,
          error: undefined,
        })
        continue
      }

      // Attempt to fetch sessions; catch errors gracefully
      let rows: SessionRow[]
      try {
        rows = await source.getSessions()
        sourceStatusMap.set(source.id, {
          available: true,
          lastUpdate: source.getLastUpdate(),
          error: undefined,
        })
      } catch (err) {
        const errorMessage =
          err instanceof Error ? err.message : String(err)
        sourceStatusMap.set(source.id, {
          available: false,
          lastUpdate,
          error: errorMessage,
        })
        continue
      }

      // Accumulate rows with their source priority for deduplication
      for (const row of rows) {
        allRows.push({ row, priority: source.priority })
      }
    }

    // Deduplicate by sessionId (highest priority wins)
    const deduped = deduplicateSessions(allRows)

    // REQ-TUI-015: Filter out stopped in-memory sessions past TTL
    // File-sourced and sqlite-sourced sessions are historical records and are NOT filtered by TTL.
    // Only in-memory sessions (live state) are subject to TTL.
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-015 plan=step-reverify-002
    const ttlMs = (options.stoppedTtlSeconds ?? 300) * 1000
    const now = Date.now()
    const sessions = deduped.filter((session) => {
      // Only apply TTL to in-memory sessions (live state)
      if (session.source !== "in-memory") return true
      if (session.status !== "stopped") return true
      const age = now - session.updatedAt.getTime()
      return age <= ttlMs
    })

    // Sort by updatedAt descending (most recently updated first)
    sessions.sort((a, b) => b.updatedAt.getTime() - a.updatedAt.getTime())

    // Compute aggregate stats (excluding team sessions)
    const aggregates = computeAggregates(sessions)

    // Build per-source status for the DashboardData payload
    const inMemoryStatus = sourceStatusMap.get("in-memory") ?? {
      available: false,
      lastUpdate: null,
    }
    const fileStatus = sourceStatusMap.get("file") ?? {
      available: false,
      lastUpdate: null,
    }
    const networkStatus = sourceStatusMap.get("network") ?? {
      available: false,
      lastUpdate: null,
    }

    return {
      sessions,
      aggregates,
      sources: {
        inMemory: inMemoryStatus,
        file: fileStatus,
        network: networkStatus,
      },
      generatedAt: new Date(),
    }
  }
}

// ---------------------------------------------------------------------------
// Factory helper
// ---------------------------------------------------------------------------

/**
 * Convenience factory for constructing a SessionDataCollector.
 *
 * @example
 * ```typescript
 * const collector = createSessionDataCollector()
 * const data = await collector.collect({
 *   repoRoot: "/path/to/repo",
 *   sources: [inMemorySource, fileSource],
 * })
 * ```
 */
export function createSessionDataCollector(): SessionDataCollector {
  return new SessionDataCollector()
}
