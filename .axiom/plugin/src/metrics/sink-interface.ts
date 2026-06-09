/**
 * MetricsSink interface — pluggable metrics sink architecture.
 *
 * REQ-PLG-120: The plugin MUST support a pluggable metrics sink interface.
 *   Multiple sinks MAY be active simultaneously (fan-out).
 * REQ-PLG-124: Each publish MUST emit a single `plugin.metrics.snapshot` event.
 * REQ-PLG-125: On session end, emit `plugin.session.ended` with `snapshot_type: "final"`.
 * REQ-PLG-131: All cost fields MUST use integer microdollars (1 USD = 1,000,000 microdollars).
 * REQ-PLG-135: estimated_cost_usd added alongside estimated_cost_microdollars (DEX-419 Fix 1).
 * REQ-PLG-136: reasoning_tokens always present at top-level (null when unavailable) (DEX-419 Fix 2).
 * REQ-PLG-137: Metadata fields: model_provider, agent_name, opencode_version, plugin_version,
 *   thinking_level, thinking_tokens, connected_mcps (DEX-419 Fix 4).
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-120
 * axiom:trace work_item=plugin-metrics-fixes-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-120 jira_ref=DEX-419
 */

// ---------------------------------------------------------------------------
// Event schemas (REQ-PLG-124, REQ-PLG-125)
// ---------------------------------------------------------------------------

/** Per-model token delta for a single publish interval. */
export interface ModelDelta {
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  /** REQ-PLG-131: Integer microdollars (1 USD = 1,000,000 microdollars). */
  estimated_cost_microdollars: number
  /** REQ-PLG-135 (DEX-419 Fix 1): USD cost alongside microdollars for human readability.
   * Computed as estimated_cost_microdollars / 1_000_000, rounded to 6 decimal places. */
  estimated_cost_usd: number
  llm_calls: number
  /** Omit when zero per omit-not-null rule (specs/25). */
  reasoning_tokens?: number
}

/** Session-level activity deltas — reset after each publish. */
export interface SessionDelta {
  messages: number
  tool_invocations: number
  commands_executed: number
  files_read: number
  files_written: number
}

/** Lightweight running totals for backend correlation. */
export interface SessionTotals {
  duration_seconds: number
  messages: number
  tool_invocations: number
}

/** Per-model cumulative totals for session_summary (final event only). */
export interface ModelSummary {
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  reasoning_tokens: number
  /** REQ-PLG-131: Integer microdollars (1 USD = 1,000,000 microdollars). */
  estimated_cost_microdollars: number
  /** REQ-PLG-135 (DEX-419 Fix 1): USD cost alongside microdollars. */
  estimated_cost_usd: number
  llm_calls: number
}

/** Session summary block — emitted once only at session end (REQ-PLG-125). */
export interface SessionSummary {
  total_duration_seconds: number
  total_messages: number
  total_tool_invocations: number
  total_commands_executed: number
  maturity_level_start: number
  maturity_level_end: number
  commands_used: string[]
  by_model: Record<string, ModelSummary>
  /** REQ-PLG-131: Integer microdollars (1 USD = 1,000,000 microdollars). */
  total_estimated_cost_microdollars: number
  /** REQ-PLG-135 (DEX-419 Fix 1): USD cost alongside microdollars. */
  total_estimated_cost_usd: number
}

/** Snapshot type per REQ-PLG-122. */
export type SnapshotType = "stream" | "reconciliation" | "final"

/** Event type discriminator. */
export type MetricsEventType = "plugin.metrics.snapshot" | "plugin.session.ended"

/**
 * The unified metrics event payload (REQ-PLG-124 + REQ-PLG-125).
 *
 * `plugin.metrics.snapshot` uses `snapshot_type: "stream" | "reconciliation"`.
 * `plugin.session.ended` uses `snapshot_type: "final"` and includes `session_summary`.
 */
export interface MetricsEvent {
  timestamp: string
  level: "INFO"
  event_type: MetricsEventType
  component: "opencode_plugin"
  session_id: string
  /** REQ-PLG-109: Work item ID for cross-system correlation. Omitted when unknown. */
  work_item_id?: string
  /** REQ-PLG-109: Run ID for cross-system correlation. Omitted when unknown. */
  run_id?: string
  /** Git remote identity: owner/repo from git remote, or hostname:<name> fallback. Never empty. REQ-PLG-124. */
  repo: string
  /** Current git branch. Omitted (omit-not-null) when not in a git repo or in detached HEAD state. REQ-PLG-133. */
  git_branch?: string
  /** Absolute local filesystem path to the repo root. Always present. REQ-PLG-133. */
  repo_path: string
  user_email: string
  resolved_email_source: EmailSource
  session_identity: string
  session_state: SessionState
  snapshot_type: SnapshotType
  snapshot_seq: number
  interval_seconds: number
  /** Keyed by modelID. Models with zero activity are omitted. */
  delta: Record<string, ModelDelta>
  delta_session: SessionDelta
  session_totals: SessionTotals
  /** Only present on `plugin.session.ended` events. */
  session_summary?: SessionSummary

  // -------------------------------------------------------------------------
  // REQ-PLG-136 (DEX-419 Fix 2): reasoning_tokens — always present, null when
  // provider data is unavailable. Do NOT omit this field.
  // -------------------------------------------------------------------------
  /** Reasoning tokens for the current interval. null when provider data is unavailable. */
  reasoning_tokens: number | null

  // -------------------------------------------------------------------------
  // REQ-PLG-137 (DEX-419 Fix 4): Metadata fields
  // -------------------------------------------------------------------------
  /** Model provider (e.g., "anthropic", "amazon-bedrock", "openai"). null when unknown. */
  model_provider: string | null
  /** Active agent name (e.g., "tower-axiom"). null when unknown. */
  agent_name: string | null
  /** OpenCode runtime version (e.g., "0.1.123"). null when unavailable. */
  opencode_version: string | null
  /** Axiom plugin version (e.g., "0.2.0"). null when unavailable. */
  plugin_version: string | null
  /** Model thinking level ("none" | "low" | "medium" | "high"). null when unknown. */
  thinking_level: "none" | "low" | "medium" | "high" | null
  /** Thinking tokens for the current interval. null when provider data is unavailable. */
  thinking_tokens: number | null
  /** List of connected MCP server names (e.g., ["chrome-devtools", "atlassian"]). */
  connected_mcps: string[]
  /**
   * Optional L7 listener metrics (REQ-LNP-078). Present when listeners are active.
   * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-078 plan=step-spec-002
   */
  listener_metrics?: {
    /** Total events delivered across all listeners. */
    events_delivered_total: number;
    /** Total events dropped across all listeners. */
    events_dropped_total: number;
    /** Per-listener uptime in seconds. */
    listener_uptime: Record<string, number>;
    /** Active listener count. */
    active_listener_count: number;
    /** Per-listener drop counts by reason (for counter labels). */
    drops_by_listener: Record<string, number>;
    /** Per-listener delivery latency samples in ms (REQ-LNP-078). */
    delivery_latencies?: Record<string, number[]>;
  }
}

/** Session state per REQ-PLG-127. */
export type SessionState = "running" | "idle" | "stopped"

/** Email resolution source per REQ-PLG-126. */
export type EmailSource =
  | "env_var"
  | "git_config"
  | "github_api"
  | "atlassian_mcp"
  | "config_file"
  | "anonymous"

// ---------------------------------------------------------------------------
// Sink interface (REQ-PLG-120)
// ---------------------------------------------------------------------------

/** Sink identifier — matches the config key in metrics.sinks[]. */
export type SinkId = "axiom-backend" | "prometheus" | "cloudwatch" | "stdout"

/**
 * Result of a single sink publish attempt.
 *
 * REQ-PLG-128: Distinguish error types — 400/401 are non-retryable,
 * 429/503 are retryable with Retry-After.
 */
export interface SinkPublishResult {
  success: boolean
  /** HTTP status code or 0 for non-HTTP errors. */
  statusCode?: number
  /** Whether this failure should be retried. */
  retryable: boolean
  /** Error message if failed. */
  error?: string
  /** Retry-After header value in seconds (for 429/503). */
  retryAfterSeconds?: number
}

/**
 * MetricsSink — the pluggable interface for off-system metrics publishing.
 *
 * Each sink implementation handles one target (backend API, Prometheus, etc.).
 * The publisher fans out to all active sinks independently (REQ-PLG-130).
 */
export interface MetricsSink {
  /** Unique sink identifier matching the config key. */
  readonly id: SinkId

  /** Human-readable name for logging. */
  readonly name: string

  /**
   * Publish a metrics event to this sink.
   *
   * MUST be non-blocking from the caller's perspective (REQ-PLG-129).
   * The publisher calls this asynchronously.
   */
  publish(event: MetricsEvent): Promise<SinkPublishResult>

  /**
   * Optional cleanup on session end (e.g., flush buffers, close connections).
   * Called once during graceful shutdown.
   */
  shutdown?(): Promise<void>
}

// ---------------------------------------------------------------------------
// Sink configuration types (REQ-PLG-123)
// ---------------------------------------------------------------------------

/** Prometheus sink config. */
export interface PrometheusSinkConfig {
  push_gateway_url: string
  job_name: string
  namespace: string
}

/** CloudWatch sink config. */
export interface CloudWatchSinkConfig {
  namespace: string
  region: string | null
  log_group: string
}

/** Full metrics publishing config from .axiom/plugin.yaml. */
export interface MetricsPublishConfig {
  publish_enabled: boolean
  sinks: SinkId[]
  stream_interval_seconds: number
  reconciliation_interval_seconds: number
  flush_interval_seconds: number
  max_buffered_intervals: number
  user_email: string | null
  session_budget_usd: number
  warning_threshold_pct: number
  pricing_overrides: Record<string, unknown>
  prometheus?: PrometheusSinkConfig
  cloudwatch?: CloudWatchSinkConfig
}

/** Default metrics publish config values per REQ-PLG-123. */
export const DEFAULT_METRICS_PUBLISH_CONFIG: MetricsPublishConfig = {
  publish_enabled: true,
  sinks: [],
  stream_interval_seconds: 30,
  reconciliation_interval_seconds: 1800,
  flush_interval_seconds: 600,
  max_buffered_intervals: 12,
  user_email: null,
  session_budget_usd: 5.0,
  warning_threshold_pct: 80,
  pricing_overrides: {},
}
