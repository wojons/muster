/**
 * InMemorySource — reads live session metrics from the plugin's deltaAccumulator.
 *
 * This is the highest-priority data source (priority=3). It reads directly from
 * the in-process deltaAccumulator that the plugin maintains during a session.
 * Because it reads live in-memory state, it is always fresh and never stale.
 *
 * REQ-TUI-010: The dashboard MUST define a typed data layer with SessionDataSource.
 * REQ-TUI-012: Model display aliases MUST be applied to raw model IDs.
 *
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-010
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-010 plan=phase-1/task-1.2
 */

import type { SessionDataSource, SessionRow } from "../types"
import type { SessionState } from "../../metrics/sink-interface"
import { resolveModelDisplayName } from "../model-aliases"

// ---------------------------------------------------------------------------
// Minimal interfaces for injected objects (decoupled from index.ts internals)
// ---------------------------------------------------------------------------

/**
 * Minimal interface for the deltaAccumulator injected into InMemorySource.
 * Decoupled from the concrete implementation in index.ts for testability.
 */
export interface DeltaAccumulatorLike {
  getSessionTotals(): {
    duration_seconds: number
    messages: number
    tool_invocations: number
  }
  getSessionSummary(): {
    by_model: Record<
      string,
      {
        input_tokens: number
        output_tokens: number
        estimated_cost_microdollars: number
        llm_calls: number
      }
    >
    total_estimated_cost_microdollars: number
  }
}

/**
 * Minimal interface for the session state tracker injected into InMemorySource.
 */
export interface SessionStateTrackerLike {
  readonly current: SessionState
}

// ---------------------------------------------------------------------------
// InMemorySourceOptions
// ---------------------------------------------------------------------------

/**
 * Options for constructing an InMemorySource.
 *
 * All live-data accessors are injected as functions so the source can be
 * constructed before the plugin's internal objects are initialised, and so
 * tests can supply mock implementations without touching index.ts.
 */
export interface InMemorySourceOptions {
  /** The session ID for the current session (fallback when getSessionId not provided). */
  sessionId: string
  /** Getter for dynamic session ID — read at getSessions() call time (step-reverify-005). */
  getSessionId?: () => string
  /** Returns the live deltaAccumulator, or null when not yet initialised. */
  getDeltaAccumulator: () => DeltaAccumulatorLike | null
  /** Returns the live session state tracker, or null when not yet initialised. */
  getSessionStateTracker: () => SessionStateTrackerLike | null
  /** Returns the session start time as a Unix timestamp (Date.now()). */
  getSessionStartTime: () => number
}

/**
 * Pick the model with the most llm_calls from the by_model map.
 * Returns an empty string when by_model is empty.
 */
function pickDominantModel(
  byModel: Record<string, { llm_calls: number }>
): string {
  let dominant = ""
  let maxCalls = -1
  for (const [modelId, stats] of Object.entries(byModel)) {
    if (stats.llm_calls > maxCalls) {
      maxCalls = stats.llm_calls
      dominant = modelId
    }
  }
  return dominant
}

// ---------------------------------------------------------------------------
// InMemorySource
// ---------------------------------------------------------------------------

/**
 * InMemorySource — highest-priority SessionDataSource (priority=3).
 *
 * Reads live session metrics directly from the plugin's in-memory
 * deltaAccumulator. Returns a single SessionRow for the current session.
 * Degrades gracefully when the accumulator is not yet available (returns []).
 */
export class InMemorySource implements SessionDataSource {
  readonly id = "in-memory" as const
  readonly priority = 3

  private readonly options: InMemorySourceOptions

  constructor(options: InMemorySourceOptions) {
    this.options = options
  }

  /**
   * Returns true when the deltaAccumulator is available (non-null).
   */
  isAvailable(): boolean {
    return this.options.getDeltaAccumulator() !== null
  }

  /**
   * Always returns a fresh Date — in-memory data is always current.
   */
  getLastUpdate(): Date | null {
    return new Date()
  }

  /**
   * Reads the current session's live metrics and returns a single SessionRow.
   *
   * Returns an empty array when the deltaAccumulator is null (graceful degradation).
   */
  async getSessions(): Promise<SessionRow[]> {
    const accumulator = this.options.getDeltaAccumulator()
    if (accumulator === null) {
      return []
    }

    const summary = accumulator.getSessionSummary()
    const totals = accumulator.getSessionTotals()
    const stateTracker = this.options.getSessionStateTracker()

    // Determine dominant model (most llm_calls)
    const dominantModelRaw = pickDominantModel(summary.by_model)

    // Sum input + output tokens across all models
    let totalTokens = 0
    for (const stats of Object.values(summary.by_model)) {
      totalTokens += stats.input_tokens + stats.output_tokens
    }

    // Convert microdollars to USD
    const costUsd = summary.total_estimated_cost_microdollars / 1_000_000

    // Resolve session state
    const status: SessionState = stateTracker?.current ?? "running"

    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-010 plan=step-reverify-005
    const row: SessionRow = {
      sessionId: this.options.getSessionId?.() ?? this.options.sessionId,
      model: resolveModelDisplayName(dominantModelRaw),
      modelRaw: dominantModelRaw,
      costUsd,
      totalTokens,
      durationSeconds: totals.duration_seconds,
      status,
      source: "in-memory",
      updatedAt: new Date(),
      isTeam: false,
    }

    return [row]
  }
}

// ---------------------------------------------------------------------------
// Factory helper
// ---------------------------------------------------------------------------

/**
 * Convenience factory for constructing an InMemorySource.
 *
 * @example
 * ```typescript
 * const source = createInMemorySource({
 *   sessionId: currentSessionId,
 *   getDeltaAccumulator: () => (metricsPublisher as any)._deltaAccumulator,
 *   getSessionStateTracker: () => (metricsPublisher as any)._sessionStateTracker,
 *   getSessionStartTime: () => sessionStartTime,
 * })
 * ```
 */
export function createInMemorySource(options: InMemorySourceOptions): InMemorySource {
  return new InMemorySource(options)
}
