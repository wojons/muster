/**
 * FileSource — reads session analytics from `.axiom/sessions/*.json`.
 *
 * REQ-TUI-054: FileSource MUST read session analytics files from
 *   `.axiom/sessions/*.json` with <10ms per file read.
 *
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-054
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-054 plan=phase-1/task-1.3
 */

import { existsSync, readdirSync, readFileSync, statSync } from "node:fs"
import { join } from "node:path"
import type { SessionDataSource, SessionRow } from "../types"
import type { SessionAnalyticsFile } from "../../layers/session-persistence"
import { resolveModelDisplayName } from "../model-aliases"

// ---------------------------------------------------------------------------
// Model display aliases (re-exported from shared module for backward compat)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-012 plan=step-verify-H004
// ---------------------------------------------------------------------------

/** @deprecated Import from `../model-aliases` directly. Re-exported for backward compatibility. */
export { MODEL_DISPLAY_ALIASES as MODEL_ALIASES } from "../model-aliases"

// ---------------------------------------------------------------------------
// Performance limit
// ---------------------------------------------------------------------------

/**
 * Maximum number of session files to read per getSessions() call.
 * Files are sorted by mtime descending so only the most recent N sessions
 * are processed — prevents O(N) unbounded scans at 500+ files.
 *
 * REQ-TUI-054: limit to most recent N sessions for performance.
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-054 plan=step-verify-H003
 */
export const MAX_SESSION_FILES = 50

// ---------------------------------------------------------------------------
// FileSourceOptions
// ---------------------------------------------------------------------------

/**
 * Options for constructing a FileSource.
 *
 * `_readFile`, `_listFiles`, and `_statFile` are injection points for
 * testing — when provided, no real filesystem I/O is performed.
 */
export interface FileSourceOptions {
  /** Absolute path to the repository root. */
  repoRoot: string
  /** Override the sessions directory path (for testing). */
  sessionsDir?: string
  /** Inject a custom file reader (for testing). */
  _readFile?: (path: string) => string
  /** Inject a custom directory lister (for testing). */
  _listFiles?: (dir: string) => string[]
  /** Inject a custom stat function (for testing mtime). */
  _statFile?: (path: string) => { mtimeMs: number }
}

/**
 * Pick the model with the most calls from the by_model array.
 * Returns an empty string if by_model is empty.
 */
function pickDominantModel(byModel: SessionAnalyticsFile["by_model"]): string {
  if (!byModel || byModel.length === 0) return ""
  let best = byModel[0]
  for (const entry of byModel) {
    if (entry.calls > best.calls) {
      best = entry
    }
  }
  return best.model
}

// ---------------------------------------------------------------------------
// FileSource
// ---------------------------------------------------------------------------

/**
 * FileSource — implements SessionDataSource by reading persisted session
 * analytics files from `.axiom/sessions/*.json`.
 *
 * - id: 'file', priority: 2
 * - Skips `.tmp` files, corrupted JSON, and `dry_run: true` sessions.
 * - Uses injected `_readFile` / `_listFiles` for testability.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-054 plan=phase-1/task-1.3
 */
export class FileSource implements SessionDataSource {
  readonly id = "file" as const
  readonly priority = 2

  private readonly sessionsDir: string
  private readonly readFile: (path: string) => string
  private readonly listFiles: (dir: string) => string[]
  private readonly statFile: (path: string) => { mtimeMs: number }
  private lastUpdate: Date | null = null

  constructor(options: FileSourceOptions) {
    this.sessionsDir =
      options.sessionsDir ?? join(options.repoRoot, ".axiom", "sessions")

    // Use injected helpers or fall back to real fs operations
    this.readFile = options._readFile ?? ((path: string) => readFileSync(path, "utf-8"))
    this.listFiles =
      options._listFiles ??
      ((dir: string): string[] => {
        if (!existsSync(dir)) return []
        return readdirSync(dir)
      })
    this.statFile = options._statFile ?? statSync
  }

  /**
   * Returns the mtime (ms since epoch) for a file path.
   * Returns 0 on any error so the file sorts last (oldest).
   *
   * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-054 plan=step-verify-H003
   */
  private getFileMtime(filePath: string): number {
    try {
      return this.statFile(filePath).mtimeMs
    } catch {
      return 0
    }
  }

  /**
   * Returns true when the sessions directory exists (or when _listFiles is
   * injected, indicating a test environment where availability is assumed).
   */
  isAvailable(): boolean {
    // When _listFiles is injected, the source is always considered available
    // (test environment). Otherwise check real filesystem.
    return existsSync(this.sessionsDir)
  }

  /**
   * Returns the timestamp of the most recently modified session file,
   * or null if no files have been read yet.
   */
  getLastUpdate(): Date | null {
    return this.lastUpdate
  }

  /**
   * Reads all `.json` session files from the sessions directory.
   *
   * - Skips `.tmp` files
   * - Skips files with corrupted/malformed JSON (no crash)
   * - Skips sessions where `dry_run === true`
   * - Converts each valid file to a SessionRow
   *
   * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-054 plan=phase-1/task-1.3
   */
  async getSessions(): Promise<SessionRow[]> {
    let allFileNames: string[]
    try {
      allFileNames = this.listFiles(this.sessionsDir)
    } catch {
      return []
    }

    // Filter to valid session JSON files (exclude .tmp files)
    const jsonFileNames = allFileNames.filter(
      (f) => f.endsWith(".json") && !f.endsWith(".tmp"),
    )

    // Sort by mtime descending (most recent first), then limit to MAX_SESSION_FILES.
    // This prevents O(N) unbounded scans at 500+ files (REQ-TUI-054 / DA-001).
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-054 plan=step-verify-H003
    const fileNames = jsonFileNames
      .map((f) => ({ name: f, mtime: this.getFileMtime(join(this.sessionsDir, f)) }))
      .sort((a, b) => b.mtime - a.mtime)
      .slice(0, MAX_SESSION_FILES)
      .map((f) => f.name)

    const rows: SessionRow[] = []
    let latestMtime: Date | null = null

    for (const fileName of fileNames) {
      const filePath = join(this.sessionsDir, fileName)

      // Read file content
      let raw: string
      try {
        raw = this.readFile(filePath)
      } catch {
        // Skip unreadable files
        continue
      }

      // Parse JSON — skip corrupted files
      let parsed: SessionAnalyticsFile
      try {
        parsed = JSON.parse(raw) as SessionAnalyticsFile
      } catch {
        // Skip malformed JSON without crashing
        continue
      }

      // Skip dry-run sessions
      if (parsed.dry_run === true) {
        continue
      }

      // Track last modification time (use ended_at as proxy for file mtime)
      let updatedAt: Date
      try {
        updatedAt = new Date(parsed.ended_at)
        if (isNaN(updatedAt.getTime())) {
          updatedAt = new Date()
        }
      } catch {
        updatedAt = new Date()
      }

      if (latestMtime === null || updatedAt > latestMtime) {
        latestMtime = updatedAt
      }

      // Resolve model display name
      const rawModelId = pickDominantModel(parsed.by_model)
      const modelDisplay = rawModelId ? resolveModelDisplayName(rawModelId) : ""

      // Build SessionRow
      const row: SessionRow = {
        sessionId: parsed.session_id,
        model: modelDisplay,
        modelRaw: rawModelId,
        costUsd: parsed.total_cost_usd ?? 0,
        totalTokens:
          (parsed.total_tokens?.prompt ?? 0) + (parsed.total_tokens?.completion ?? 0),
        durationSeconds: (parsed.duration_minutes ?? 0) * 60,
        status: "stopped",
        source: "file",
        updatedAt,
        isTeam: false,
      }

      rows.push(row)
    }

    // Update lastUpdate to the most recently modified file
    if (latestMtime !== null) {
      this.lastUpdate = latestMtime
    }

    return rows
  }
}

/**
 * Factory function for constructing a FileSource.
 * Convenience wrapper for callers that prefer a function over `new`.
 */
export function createFileSource(options: FileSourceOptions): FileSource {
  return new FileSource(options)
}
