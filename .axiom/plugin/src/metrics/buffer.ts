/**
 * Buffer manager — in-memory buffer for undelivered metrics deltas.
 *
 * REQ-PLG-128: The plugin MUST NOT buffer more than `max_buffered_intervals`
 *   (default: 12) intervals of undelivered deltas. When the buffer cap is
 *   reached, auto-summarize the oldest deltas into a single consolidated delta.
 * REQ-PLG-135 (DEX-419 Fix 1): estimated_cost_usd merged alongside microdollars.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-128
 * axiom:trace work_item=plugin-metrics-fixes-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-128 jira_ref=DEX-419
 */

import type { MetricsEvent, ModelDelta, SessionDelta } from "./sink-interface"
import { createFallbackLogger } from "../logger"

// Module-level fallback logger — routes to stderr, never stdout (REQ-PLG-134)
const _log = createFallbackLogger()

/** A buffered metrics interval waiting for delivery. */
export interface BufferedInterval {
  /** The original event that failed to deliver. */
  event: MetricsEvent
  /** Number of consecutive delivery failures for this interval. */
  failureCount: number
  /** Whether this is a consolidated (auto-summarized) interval. */
  consolidated: boolean
  /** Number of original intervals summarized into this one (1 if not consolidated). */
  summarizedCount: number
  /** Per-sink delivery tracking (REQ-PLG-130 independence). Internal use by drainBuffer(). */
  _sinkSucceeded?: Set<string>
}

/** Options for creating a buffer manager. */
export interface BufferManagerOptions {
  /** Maximum buffered intervals before auto-summarization (default: 12). */
  maxBufferedIntervals?: number
}

/** Buffer manager state and operations. */
export interface BufferManager {
  /** Add a failed event to the buffer. */
  add(event: MetricsEvent): void

  /** Get all buffered intervals (oldest first). */
  getAll(): readonly BufferedInterval[]

  /** Remove a specific interval from the buffer (after successful delivery). */
  remove(index: number): void

  /** Clear all buffered intervals. */
  clear(): void

  /** Current buffer size. */
  readonly size: number

  /** Whether the buffer is at capacity. */
  readonly isFull: boolean
}

/**
 * Merge multiple model deltas into a single consolidated delta.
 *
 * Sums all token counts, costs, and call counts per model.
 * REQ-PLG-135 (DEX-419 Fix 1): also merges estimated_cost_usd.
 */
export function mergeModelDeltas(
  deltas: Array<Record<string, ModelDelta>>,
): Record<string, ModelDelta> {
  const merged: Record<string, ModelDelta> = {}

  for (const delta of deltas) {
    for (const [modelId, modelDelta] of Object.entries(delta)) {
      if (!merged[modelId]) {
        merged[modelId] = { ...modelDelta }
        if (modelDelta.reasoning_tokens !== undefined) {
          merged[modelId].reasoning_tokens = modelDelta.reasoning_tokens
        }
      } else {
        const existing = merged[modelId]
        existing.input_tokens += modelDelta.input_tokens
        existing.output_tokens += modelDelta.output_tokens
        existing.cache_read_tokens += modelDelta.cache_read_tokens
        existing.cache_write_tokens += modelDelta.cache_write_tokens
        existing.estimated_cost_microdollars += modelDelta.estimated_cost_microdollars
        // REQ-PLG-135: recompute estimated_cost_usd from the integer microdollar sum
        // to avoid float-accumulation drift (per REQ-PLG-131 integer-arithmetic rationale).
        // MUST NOT use += on floats — always derive from the microdollar source of truth.
        existing.estimated_cost_usd = existing.estimated_cost_microdollars / 1_000_000
        existing.llm_calls += modelDelta.llm_calls
        if (modelDelta.reasoning_tokens) {
          existing.reasoning_tokens = (existing.reasoning_tokens ?? 0) + modelDelta.reasoning_tokens
        }
      }
    }
  }

  return merged
}

/**
 * Merge multiple session deltas into a single consolidated delta.
 */
export function mergeSessionDeltas(deltas: SessionDelta[]): SessionDelta {
  const merged: SessionDelta = {
    messages: 0,
    tool_invocations: 0,
    commands_executed: 0,
    files_read: 0,
    files_written: 0,
  }

  for (const delta of deltas) {
    merged.messages += delta.messages
    merged.tool_invocations += delta.tool_invocations
    merged.commands_executed += delta.commands_executed
    merged.files_read += delta.files_read
    merged.files_written += delta.files_written
  }

  return merged
}

/**
 * Create a buffer manager for undelivered metrics intervals.
 *
 * When the buffer reaches capacity, the oldest intervals are auto-summarized
 * into a single consolidated interval to prevent unbounded memory growth.
 */
export function createBufferManager(options: BufferManagerOptions = {}): BufferManager {
  const maxBuffered = options.maxBufferedIntervals ?? 12
  const buffer: BufferedInterval[] = []

  function autoSummarize(): void {
    if (buffer.length < 2) return

    // Summarize the oldest half of the buffer into one consolidated interval
    const summarizeCount = Math.ceil(buffer.length / 2)
    const toSummarize = buffer.splice(0, summarizeCount)

    // Use the newest event as the base, merge all deltas
    const baseEvent = toSummarize[toSummarize.length - 1].event
    const mergedDelta = mergeModelDeltas(toSummarize.map((b) => b.event.delta))
    const mergedSessionDelta = mergeSessionDeltas(toSummarize.map((b) => b.event.delta_session))
    const totalSummarized = toSummarize.reduce((sum, b) => sum + b.summarizedCount, 0)

    // Compute union of _sinkSucceeded from all source intervals
    // so drainBuffer() doesn't re-deliver to sinks that already succeeded
    const unionSucceeded = new Set<string>()
    for (const b of toSummarize) {
      if (b._sinkSucceeded) {
        for (const sinkId of b._sinkSucceeded) {
          unionSucceeded.add(sinkId)
        }
      }
    }

    const consolidated: BufferedInterval = {
      event: {
        ...baseEvent,
        delta: mergedDelta,
        delta_session: mergedSessionDelta,
        // Keep the latest snapshot_seq and totals
      },
      failureCount: 0,
      consolidated: true,
      summarizedCount: totalSummarized,
      _sinkSucceeded: unionSucceeded.size > 0 ? unionSucceeded : undefined,
    }

    // Insert at the front (oldest position)
    buffer.unshift(consolidated)

    // Emit warning per REQ-PLG-128
    _log.warn(
      `Metrics buffer full — summarized ${totalSummarized} intervals into 1 consolidated delta.`,
    )
  }

  return {
    add(event: MetricsEvent): void {
      buffer.push({
        event,
        failureCount: 1,
        consolidated: false,
        summarizedCount: 1,
      })

      // Auto-summarize if over capacity
      if (buffer.length > maxBuffered) {
        autoSummarize()
      }
    },

    getAll(): readonly BufferedInterval[] {
      return buffer
    },

    remove(index: number): void {
      if (index >= 0 && index < buffer.length) {
        buffer.splice(index, 1)
      }
    },

    clear(): void {
      buffer.length = 0
    },

    get size(): number {
      return buffer.length
    },

    get isFull(): boolean {
      return buffer.length >= maxBuffered
    },
  }
}
