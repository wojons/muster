/**
 * Snapshot sequence tracker — monotonically increasing per-session counter.
 *
 * REQ-PLG-122: Each event includes a monotonically increasing `snapshot_seq`
 *   (integer, starting at 1, per session) for gap detection.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-122
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-122 jira_ref=DEX-392
 */

/** Snapshot sequence tracker. */
export interface SnapshotSequence {
  /** Get the next sequence number (auto-increments). */
  next(): number

  /** Get the current sequence number without incrementing. */
  readonly current: number

  /** Reset the sequence (for testing only). */
  reset(): void
}

/**
 * Create a snapshot sequence tracker.
 *
 * Starts at 1 and increments monotonically per session.
 * Thread-safe in single-threaded JS runtime.
 */
export function createSnapshotSequence(): SnapshotSequence {
  let seq = 0

  return {
    next(): number {
      seq += 1
      return seq
    },

    get current(): number {
      return seq
    },

    reset(): void {
      seq = 0
    },
  }
}
