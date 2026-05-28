/**
 * Metrics module barrel export — pluggable metrics sink architecture.
 *
 * REQ-PLG-120: Pluggable metrics sink interface with 4 implementations.
 * REQ-PLG-122: Dual-cadence publisher (30s stream + 30min reconciliation).
 * REQ-PLG-126: 6-step email identity chain.
 * REQ-PLG-128: Buffer manager with auto-summarization.
 * REQ-PLG-131: Integer microdollars cost arithmetic.
 *
 * Spec: specs/70-OpenCode-Plugin.md §5.4
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-120 jira_ref=DEX-392
 */

// Sink interface and types
export type {
  MetricsSink,
  MetricsEvent,
  MetricsEventType,
  SnapshotType,
  SessionState,
  EmailSource,
  SinkId,
  SinkPublishResult,
  ModelDelta,
  SessionDelta,
  SessionTotals,
  ModelSummary,
  SessionSummary,
  MetricsPublishConfig,
  PrometheusSinkConfig,
  CloudWatchSinkConfig,
} from "./sink-interface"
export { DEFAULT_METRICS_PUBLISH_CONFIG } from "./sink-interface"

// Sink implementations
export { createStdoutSink } from "./sinks/stdout"
export { createBackendSink, type BackendSinkOptions } from "./sinks/axiom-backend"
export { createPrometheusSink, formatPrometheusMetrics, type PrometheusSinkOptions } from "./sinks/prometheus"
export { createCloudWatchSink, buildCloudWatchMetricData, buildCloudWatchLogEvent, type CloudWatchSinkOptions } from "./sinks/cloudwatch"

// Publisher
export { createPublisher, type PublisherOptions, type MetricsPublisher } from "./publisher"

// Buffer manager
export { createBufferManager, mergeModelDeltas, mergeSessionDeltas, type BufferManager, type BufferedInterval, type BufferManagerOptions } from "./buffer"

// Identity resolver
export { resolveEmailIdentity, resolveEmailIdentitySync, isNoreplyEmail, resolveGitBranch, type EmailIdentityResult, type EmailIdentityOptions } from "./identity"

// Cost module (microdollars)
export { usdToMicrodollars, microdollarsToUsd, computeCostMicrodollars, pricingToMicrodollars, MICRODOLLARS_PER_USD, type MicrodollarPricing } from "./cost"

// Snapshot sequence
export { createSnapshotSequence, type SnapshotSequence } from "./snapshot"

// Session state machine (REQ-PLG-127)
export { createSessionStateTracker, type SessionStateTracker, type SessionStateTrackerOptions } from "./session-state"

// Disk flush (REQ-PLG-121)
export { createDiskFlusher, type DiskFlusher, type DiskFlusherOptions } from "./disk-flush"
