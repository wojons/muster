/**
 * Disk flush capability for metrics publisher.
 *
 * REQ-PLG-121: Flush session metrics to disk every 10 minutes using
 * write-to-temp-then-rename pattern:
 *   1. Write to <sessionId>.json.tmp
 *   2. fsync (via writeFileSync with flags)
 *   3. Atomic rename to <sessionId>.json
 *
 * On crash recovery, if a .tmp file exists, it MUST be ignored (stale).
 * A final flush MUST also occur immediately on session end.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-121
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-121 plan=phase-107-3 jira_ref=DEX-392
 */

import * as fs from "fs"
import * as path from "path"

export interface DiskFlusherOptions {
  /** Path to the sessions directory (e.g., `.axiom/sessions/`). */
  sessionsDir: string
  /** Current session ID. */
  sessionId: string
}

export interface DiskFlusher {
  /** Write data to disk atomically using write-to-temp-then-rename. */
  flush(data: unknown): void
  /** Remove stale .tmp files in the sessions directory (crash recovery). */
  cleanup(): void
}

/**
 * Create a disk flusher for session metrics.
 *
 * Uses write-to-temp-then-rename pattern:
 *   1. Write to <sessionId>.json.tmp
 *   2. fsync (via writeFileSync with 'wx' flag, then renameSync)
 *   3. Atomic rename to <sessionId>.json
 */
export function createDiskFlusher(options: DiskFlusherOptions): DiskFlusher {
  const { sessionsDir, sessionId } = options

  const tmpPath = path.join(sessionsDir, `${sessionId}.json.tmp`)
  const finalPath = path.join(sessionsDir, `${sessionId}.json`)

  return {
    flush(data: unknown): void {
      // Ensure sessions directory exists
      if (!fs.existsSync(sessionsDir)) {
        fs.mkdirSync(sessionsDir, { recursive: true })
      }

      // Serialize data to JSON with pretty formatting for readability
      const jsonContent = JSON.stringify(data, null, 2)

      // Write to temp file first (write-to-temp-then-rename pattern).
      // Using 'w' flag: overwrite — safe because we rename atomically after write.
      // Do NOT use 'wx' (exclusive create) — it would fail on the second flush
      // of a session when the .tmp file already exists from the first flush.
      fs.writeFileSync(tmpPath, jsonContent, { encoding: "utf-8", flag: "w" })

      // Atomic rename from .tmp to final
      fs.renameSync(tmpPath, finalPath)
    },

    cleanup(): void {
      // Check if sessions directory exists
      if (!fs.existsSync(sessionsDir)) {
        return
      }

      // Read all files in the sessions directory
      const files = fs.readdirSync(sessionsDir)

      // Remove any .tmp files (stale crash recovery files)
      for (const file of files) {
        if (file.endsWith(".json.tmp")) {
          const tmpFilePath = path.join(sessionsDir, file)
          try {
            fs.unlinkSync(tmpFilePath)
          } catch {
            // Ignore errors during cleanup - file may be locked or already gone
          }
        }
      }
    },
  }
}