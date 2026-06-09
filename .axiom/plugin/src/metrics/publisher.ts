/**
 * Dual-cadence metrics publisher — streams + reconciliation + final.
 *
 * REQ-PLG-122: Dual-cadence publishing:
 *   1. Near-real-time stream (default: every 30s) with delta values.
 *   2. Reconciliation push (default: every 30min) with cumulative totals.
 *   3. Final publish on session end with snapshot_type: "final".
 *
 * REQ-PLG-128: Publish failure handling — retry on next interval, buffer cap,
 *   auto-summarize, 3-failure user warning.
 * REQ-PLG-129: All publish calls MUST be non-blocking.
 * REQ-PLG-130: Each sink operates independently — failure in one MUST NOT
 *   affect other sinks.
 * REQ-PLG-135 (DEX-419 Fix 1): estimated_cost_usd added to ModelDelta.
 * REQ-PLG-136 (DEX-419 Fix 2): reasoning_tokens always present at top-level.
 * REQ-PLG-137 (DEX-419 Fix 4): Metadata fields propagated through publisher.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-122, #REQ-PLG-128, #REQ-PLG-129, #REQ-PLG-130
 * axiom:trace work_item=plugin-metrics-fixes-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-134 jira_ref=DEX-419
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-122 jira_ref=DEX-392
 */

import type {
  MetricsSink,
  MetricsEvent,
  MetricsEventType,
  SnapshotType,
  SessionState,
  EmailSource,
  ModelDelta,
  SessionDelta,
  SessionTotals,
  SessionSummary,
} from "./sink-interface"
import { createBufferManager, type BufferManager } from "./buffer"
import { createSnapshotSequence, type SnapshotSequence } from "./snapshot"
import type { DiskFlusher } from "./disk-flush"

// ---------------------------------------------------------------------------
// Publisher types
// ---------------------------------------------------------------------------

/** Options for creating a metrics publisher. */
export interface PublisherOptions {
  /** Active sinks to publish to (fan-out). */
  sinks: MetricsSink[]
  /** Session ID — callback so the publisher always reads the current value.
   * Use a callback (not a string) so that session ID updates from the event
   * handler (e.g., capturing OpenCode's real session ID) are reflected in
   * all emitted events. REQ-PLG-109. */
  getSessionId: () => string
  /** Repository identifier (e.g., "acme/backend" or "hostname:mymachine"). */
  repo: string
  /** Current git branch name. Omitted when not in a git repo or detached HEAD. REQ-PLG-133. */
  gitBranch?: string
  /** Absolute local filesystem path to the repo root. REQ-PLG-133. */
  repoPath: string
  /** Resolved user email. */
  userEmail: string
  /** Email resolution source. */
  emailSource: EmailSource
  /** Session identity for cost attribution. */
  sessionIdentity: string
  /** REQ-PLG-109: Work item ID for cross-system correlation. Omitted when unknown. */
  workItemId?: string
  /** REQ-PLG-109: Run ID for cross-system correlation. Omitted when unknown. */
  runId?: string
  /** Stream interval in seconds (default: 30). */
  streamIntervalSeconds?: number
  /** Reconciliation interval in seconds (default: 1800). */
  reconciliationIntervalSeconds?: number
  /** Max buffered intervals (default: 12). */
  maxBufferedIntervals?: number
  /** Callback to get current session state. */
  getSessionState?: () => SessionState
  /** Callback to get current delta data since last publish. */
  getDelta?: () => { model: Record<string, ModelDelta>; session: SessionDelta }
  /** Callback to get cumulative session totals. */
  getSessionTotals?: () => SessionTotals
  /** Callback to get session summary (for final event). */
  getSessionSummary?: () => SessionSummary
  /** Disk flush interval in seconds (default: 600 = 10 minutes). */
  flushIntervalSeconds?: number
  /** Disk flusher for persisting metrics to disk. */
  diskFlusher?: DiskFlusher
  /** Callback to get data to flush to disk. */
  getDiskFlushData?: () => unknown

  // -------------------------------------------------------------------------
  // REQ-PLG-136 (DEX-419 Fix 2): reasoning_tokens callback
  // -------------------------------------------------------------------------
  /** Callback to get current reasoning token count. Returns null when unavailable. */
  getReasoningTokens?: () => number | null

  // -------------------------------------------------------------------------
  // REQ-PLG-137 (DEX-419 Fix 4): Metadata fields
  // -------------------------------------------------------------------------
  /** Model provider (e.g., "anthropic", "amazon-bedrock"). null when unknown. */
  modelProvider?: string | null
  /** Active agent name (e.g., "tower-axiom"). null when unknown. */
  agentName?: string | null
  /** OpenCode runtime version. null when unavailable. */
  opencodeVersion?: string | null
  /** Axiom plugin version. null when unavailable. */
  pluginVersion?: string | null
  /** Model thinking level. null when unknown. */
  thinkingLevel?: "none" | "low" | "medium" | "high" | null
  /** Callback to get current thinking token count. Returns null when unavailable. */
  getThinkingTokens?: () => number | null
  /** List of connected MCP server names. */
  connectedMcps?: string[]

  /**
   * Optional callback to get L7 listener metrics for Prometheus emission (REQ-LNP-078).
   * When provided and listeners are active, the returned data is attached to each MetricsEvent.
   * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-078 plan=step-spec-002
   */
  getListenerMetrics?: () => MetricsEvent["listener_metrics"]
}

/** Per-sink failure tracking. */
interface SinkFailureState {
  consecutiveFailures: number
  userWarned: boolean
}

/** Metrics publisher instance. */
export interface MetricsPublisher {
  /** Start the dual-cadence timers. */
  start(): void

  /** Stop the timers (does NOT emit final event — call publishFinal() first). */
  stop(): void

  /** Publish a final event and shut down all sinks. */
  publishFinal(): Promise<void>

  /** Force a stream publish now (for testing or manual trigger). */
  publishStream(): Promise<void>

  /** Force a reconciliation publish now. */
  publishReconciliation(): Promise<void>

  /** Whether the publisher is currently running. */
  readonly isRunning: boolean

  /** Get the snapshot sequence tracker (for testing). */
  readonly sequence: SnapshotSequence

  /** Get the buffer manager (for testing). */
  readonly buffer: BufferManager
}

/**
 * Create a dual-cadence metrics publisher.
 *
 * Manages stream (30s) and reconciliation (30min) timers, fans out to all
 * active sinks, handles failures with buffering and auto-summarization.
 */
export function createPublisher(options: PublisherOptions): MetricsPublisher {
  const {
    sinks,
    getSessionId,
    repo,
    gitBranch,
    repoPath,
    userEmail,
    emailSource,
    sessionIdentity,
    workItemId,
    runId,
    streamIntervalSeconds = 30,
    reconciliationIntervalSeconds = 1800,
    maxBufferedIntervals = 12,
    getSessionState = () => "running",
    getDelta = () => ({
      model: {},
      session: { messages: 0, tool_invocations: 0, commands_executed: 0, files_read: 0, files_written: 0 },
    }),
    getSessionTotals = () => ({ duration_seconds: 0, messages: 0, tool_invocations: 0 }),
    getSessionSummary,
    flushIntervalSeconds = 600,
    diskFlusher,
    getDiskFlushData,
    // REQ-PLG-136 (DEX-419 Fix 2)
    getReasoningTokens = () => null,
    // REQ-PLG-137 (DEX-419 Fix 4)
    modelProvider = null,
    agentName = null,
    opencodeVersion = null,
    pluginVersion = null,
    thinkingLevel = null,
    getThinkingTokens = () => null,
    connectedMcps = [],
    getListenerMetrics,
  } = options

  const sequence = createSnapshotSequence()
  const bufferManager = createBufferManager({ maxBufferedIntervals })
  const sinkFailures = new Map<string, SinkFailureState>()

  let streamTimer: ReturnType<typeof setInterval> | null = null
  let reconciliationTimer: ReturnType<typeof setInterval> | null = null
  let diskFlushTimer: ReturnType<typeof setInterval> | null = null
  let running = false
  let publishedFinal = false

  // Initialize failure tracking for each sink
  for (const sink of sinks) {
    sinkFailures.set(sink.id, { consecutiveFailures: 0, userWarned: false })
  }

  /** Build a metrics event. */
  function buildEvent(
    eventType: MetricsEventType,
    snapshotType: SnapshotType,
    intervalSeconds: number,
  ): MetricsEvent {
    const delta = getDelta()
    const totals = getSessionTotals()

    const event: MetricsEvent = {
      timestamp: new Date().toISOString(),
      level: "INFO",
      event_type: eventType,
      component: "opencode_plugin",
      session_id: getSessionId(),
      ...(workItemId ? { work_item_id: workItemId } : {}),
      ...(runId ? { run_id: runId } : {}),
      repo,
      ...(gitBranch ? { git_branch: gitBranch } : {}),
      repo_path: repoPath,
      user_email: userEmail,
      resolved_email_source: emailSource,
      session_identity: sessionIdentity,
      session_state: getSessionState(),
      snapshot_type: snapshotType,
      snapshot_seq: sequence.next(),
      interval_seconds: intervalSeconds,
      delta: delta.model,
      delta_session: delta.session,
      session_totals: totals,
      // REQ-PLG-136 (DEX-419 Fix 2): always present, null when unavailable
      reasoning_tokens: getReasoningTokens(),
      // REQ-PLG-137 (DEX-419 Fix 4): metadata fields
      model_provider: modelProvider,
      agent_name: agentName,
      opencode_version: opencodeVersion,
      plugin_version: pluginVersion,
      thinking_level: thinkingLevel,
      thinking_tokens: getThinkingTokens(),
      connected_mcps: connectedMcps,
    }

    // Add session_summary for final events
    if (snapshotType === "final" && getSessionSummary) {
      event.session_summary = getSessionSummary()
    }

    // REQ-LNP-078: Attach L7 listener metrics when listeners are active.
    // axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-078 plan=step-spec-002
    if (getListenerMetrics) {
      const lm = getListenerMetrics()
      if (lm !== undefined) {
        event.listener_metrics = lm
      }
    }

    return event
  }

  /** Publish an event to all sinks (fan-out, independent). */
  async function publishToAllSinks(event: MetricsEvent): Promise<void> {
    // REQ-PLG-130: Each sink operates independently
    const publishPromises = sinks.map(async (sink) => {
      try {
        const result = await sink.publish(event)

        const failureState = sinkFailures.get(sink.id)!

        if (result.success) {
          // Reset failure counter on success
          failureState.consecutiveFailures = 0
          failureState.userWarned = false
        } else {
          failureState.consecutiveFailures += 1

          if (!result.retryable) {
            // REQ-PLG-128: 400/401 — log as ERROR, don't retry
            console.error(
              `[axiom plugin] Sink "${sink.name}" non-retryable error: ${result.error}`,
            )
          } else {
            // Retryable — log as WARN
            console.warn(
              `[axiom plugin] Sink "${sink.name}" publish failed (attempt ${failureState.consecutiveFailures}): ${result.error}`,
            )
          }

          // REQ-PLG-128: After 3 consecutive failures, emit user-visible warning
          if (failureState.consecutiveFailures >= 3 && !failureState.userWarned) {
            console.warn(
              `[axiom plugin] Metrics publishing has failed 3 times. Check your sink configuration.`,
            )
            failureState.userWarned = true
          }

          // Buffer the event for retry (only for retryable failures)
          if (result.retryable) {
            bufferManager.add(event)
          }
        }
      } catch (err) {
        // Unexpected error — treat as retryable
        console.warn(
          `[axiom plugin] Sink "${sink.name}" unexpected error: ${err instanceof Error ? err.message : "unknown"}`,
        )
        bufferManager.add(event)
      }
    })

    await Promise.allSettled(publishPromises)
  }

  /** Attempt to deliver buffered intervals (per-sink independence per REQ-PLG-130). */
  async function drainBuffer(): Promise<void> {
    const buffered = bufferManager.getAll()
    if (buffered.length === 0) return

    // Track per-interval: how many sinks succeeded
    // An interval is removed only when ALL sinks have delivered it.
    // Per-sink tracking: we track which sinks have already succeeded for each
    // buffered interval to avoid duplicate delivery to healthy sinks.
    const toRemove: number[] = []
    for (let i = 0; i < buffered.length; i++) {
      const interval = buffered[i]

      // Initialize per-sink success tracking if not present
      if (!interval._sinkSucceeded) {
        interval._sinkSucceeded = new Set<string>()
      }

      for (const sink of sinks) {
        // Skip sinks that already succeeded for this interval (REQ-PLG-130)
        if (interval._sinkSucceeded.has(sink.id)) continue

        try {
          const result = await sink.publish(interval.event)
          if (result.success) {
            interval._sinkSucceeded.add(sink.id)
          }
        } catch {
          // Sink failed — will retry on next drain
        }
      }

      // Remove interval when all sinks have succeeded
      if (interval._sinkSucceeded.size >= sinks.length) {
        toRemove.push(i)
      }
    }

    // Remove delivered intervals (reverse order to maintain indices)
    for (let i = toRemove.length - 1; i >= 0; i--) {
      bufferManager.remove(toRemove[i])
    }
  }

  return {
    start(): void {
      if (running) return
      running = true

      // REQ-PLG-122: Near-real-time stream timer
      streamTimer = setInterval(() => {
        const event = buildEvent("plugin.metrics.snapshot", "stream", streamIntervalSeconds)
        // REQ-PLG-129: Non-blocking — fire and forget
        void publishToAllSinks(event).then(() => drainBuffer())
      }, streamIntervalSeconds * 1000)
      if (streamTimer && typeof streamTimer.unref === "function") streamTimer.unref()

      // REQ-PLG-122: Reconciliation timer
      reconciliationTimer = setInterval(() => {
        const event = buildEvent("plugin.metrics.snapshot", "reconciliation", reconciliationIntervalSeconds)
        void publishToAllSinks(event).then(() => drainBuffer())
      }, reconciliationIntervalSeconds * 1000)
      if (reconciliationTimer && typeof reconciliationTimer.unref === "function") reconciliationTimer.unref()

      // REQ-PLG-121: Disk flush timer (every 10 minutes by default)
      if (diskFlusher && getDiskFlushData) {
        diskFlushTimer = setInterval(() => {
          diskFlusher.flush(getDiskFlushData())
        }, flushIntervalSeconds * 1000)
        if (diskFlushTimer && typeof diskFlushTimer.unref === "function") diskFlushTimer.unref()
      }
    },

    stop(): void {
      if (!running) return
      running = false

      if (streamTimer) {
        clearInterval(streamTimer)
        streamTimer = null
      }
      if (reconciliationTimer) {
        clearInterval(reconciliationTimer)
        reconciliationTimer = null
      }
      if (diskFlushTimer) {
        clearInterval(diskFlushTimer)
        diskFlushTimer = null
      }
    },

    async publishFinal(): Promise<void> {
      // Idempotency guard — prevent double-emission on SIGTERM+exit
      if (publishedFinal) return
      publishedFinal = true

      // Stop timers first
      this.stop()

      // REQ-PLG-121: Final flush to disk BEFORE emitting final event
      if (diskFlusher && getDiskFlushData) {
        diskFlusher.flush(getDiskFlushData())
      }

      // REQ-PLG-125: Final event with session_summary
      const event = buildEvent("plugin.session.ended", "final", 0)
      await publishToAllSinks(event)

      // Drain any remaining buffer
      await drainBuffer()

      // Shutdown all sinks
      await Promise.allSettled(
        sinks.map((sink) => sink.shutdown?.()),
      )
    },

    async publishStream(): Promise<void> {
      const event = buildEvent("plugin.metrics.snapshot", "stream", streamIntervalSeconds)
      await publishToAllSinks(event)
      await drainBuffer()
    },

    async publishReconciliation(): Promise<void> {
      const event = buildEvent("plugin.metrics.snapshot", "reconciliation", reconciliationIntervalSeconds)
      await publishToAllSinks(event)
      await drainBuffer()
    },

    get isRunning(): boolean {
      return running
    },

    get sequence(): SnapshotSequence {
      return sequence
    },

    get buffer(): BufferManager {
      return bufferManager
    },
  }
}
