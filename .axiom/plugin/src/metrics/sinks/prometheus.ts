/**
 * Prometheus metrics sink — pushes metrics to a Prometheus push gateway.
 *
 * REQ-PLG-120: prometheus sink targets a Prometheus push gateway or exposition endpoint.
 *   OFF by default; enabled when `metrics.sinks` includes `prometheus`.
 *
 * Pushes metrics in Prometheus text exposition format via POST to the push gateway.
 * Each model's token counts and costs become labeled gauge/counter metrics.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-120
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-120 jira_ref=DEX-392
 */

import type { MetricsSink, MetricsEvent, SinkPublishResult, PrometheusSinkConfig } from "../sink-interface"

/** Options for creating a Prometheus sink. */
export interface PrometheusSinkOptions {
  config: PrometheusSinkConfig
  /** Request timeout in milliseconds (default: 5000). */
  timeoutMs?: number
}

/**
 * Convert a MetricsEvent into Prometheus text exposition format.
 *
 * Produces gauge metrics for each model's token counts and costs,
 * plus session-level activity metrics.
 */
export function formatPrometheusMetrics(event: MetricsEvent, namespace: string): string {
  const lines: string[] = []
  const sessionLabels = `session_id="${event.session_id}",repo="${event.repo}",user_email="${event.user_email}"`

  // Per-model delta metrics
  for (const [modelId, delta] of Object.entries(event.delta)) {
    const labels = `${sessionLabels},model="${modelId}"`
    const prefix = `${namespace}_model`

    lines.push(`# HELP ${prefix}_input_tokens Input tokens consumed`)
    lines.push(`# TYPE ${prefix}_input_tokens gauge`)
    lines.push(`${prefix}_input_tokens{${labels}} ${delta.input_tokens}`)

    lines.push(`# HELP ${prefix}_output_tokens Output tokens generated`)
    lines.push(`# TYPE ${prefix}_output_tokens gauge`)
    lines.push(`${prefix}_output_tokens{${labels}} ${delta.output_tokens}`)

    lines.push(`# HELP ${prefix}_cache_read_tokens Cache read tokens`)
    lines.push(`# TYPE ${prefix}_cache_read_tokens gauge`)
    lines.push(`${prefix}_cache_read_tokens{${labels}} ${delta.cache_read_tokens}`)

    lines.push(`# HELP ${prefix}_cache_write_tokens Cache write tokens`)
    lines.push(`# TYPE ${prefix}_cache_write_tokens gauge`)
    lines.push(`${prefix}_cache_write_tokens{${labels}} ${delta.cache_write_tokens}`)

    lines.push(`# HELP ${prefix}_cost_microdollars Estimated cost in microdollars`)
    lines.push(`# TYPE ${prefix}_cost_microdollars gauge`)
    lines.push(`${prefix}_cost_microdollars{${labels}} ${delta.estimated_cost_microdollars}`)

    lines.push(`# HELP ${prefix}_llm_calls Number of LLM calls`)
    lines.push(`# TYPE ${prefix}_llm_calls gauge`)
    lines.push(`${prefix}_llm_calls{${labels}} ${delta.llm_calls}`)
  }

  // Session-level delta metrics
  const sessionPrefix = `${namespace}_session`
  lines.push(`# HELP ${sessionPrefix}_messages Messages in interval`)
  lines.push(`# TYPE ${sessionPrefix}_messages gauge`)
  lines.push(`${sessionPrefix}_messages{${sessionLabels}} ${event.delta_session.messages}`)

  lines.push(`# HELP ${sessionPrefix}_tool_invocations Tool invocations in interval`)
  lines.push(`# TYPE ${sessionPrefix}_tool_invocations gauge`)
  lines.push(`${sessionPrefix}_tool_invocations{${sessionLabels}} ${event.delta_session.tool_invocations}`)

  lines.push(`# HELP ${sessionPrefix}_duration_seconds Total session duration`)
  lines.push(`# TYPE ${sessionPrefix}_duration_seconds gauge`)
  lines.push(`${sessionPrefix}_duration_seconds{${sessionLabels}} ${event.session_totals.duration_seconds}`)

  // L7 Listener metrics (REQ-LNP-078)
  // axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-078 plan=step-spec-002
  if (event.listener_metrics) {
    const lm = event.listener_metrics
    const listenerLabels = `session_id="${event.session_id}"`

    // codeops_listener_uptime_seconds (gauge) — per listener
    for (const [listenerName, uptimeSecs] of Object.entries(lm.listener_uptime)) {
      const labels = `${listenerLabels},listener="${listenerName}"`
      lines.push(`# HELP ${namespace}_listener_uptime_seconds Listener uptime in seconds`)
      lines.push(`# TYPE ${namespace}_listener_uptime_seconds gauge`)
      lines.push(`${namespace}_listener_uptime_seconds{${labels}} ${uptimeSecs}`)
    }

    // codeops_listener_events_dropped_total (counter) — per listener
    for (const [listenerName, dropped] of Object.entries(lm.drops_by_listener)) {
      const labels = `${listenerLabels},listener="${listenerName}"`
      lines.push(`# HELP ${namespace}_listener_events_dropped_total Total events dropped by listener`)
      lines.push(`# TYPE ${namespace}_listener_events_dropped_total counter`)
      lines.push(`${namespace}_listener_events_dropped_total{${labels}} ${dropped}`)
    }

    // codeops_listener_delivery_latency_seconds (histogram — real buckets from per-event timing)
    // REQ-LNP-078: Real histogram buckets from delivery latency samples
    // axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-078 plan=phase-3/task-3.2 jira_ref=DEX-434
    if (lm.delivery_latencies) {
      // Standard Prometheus latency buckets in seconds
      const BUCKETS_S = [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10];

      for (const [listenerName, latenciesMs] of Object.entries(lm.delivery_latencies)) {
        if (latenciesMs.length === 0) continue;

        const labels = `${listenerLabels},listener="${listenerName}"`;
        const sortedS = [...latenciesMs].map(ms => ms / 1000).sort((a, b) => a - b);
        const count = sortedS.length;
        const sum = sortedS.reduce((a, b) => a + b, 0);

        lines.push(`# HELP ${namespace}_listener_delivery_latency_seconds Event delivery latency in seconds`);
        lines.push(`# TYPE ${namespace}_listener_delivery_latency_seconds histogram`);

        // Emit bucket lines
        for (const bucket of BUCKETS_S) {
          const bucketCount = sortedS.filter(s => s <= bucket).length;
          lines.push(`${namespace}_listener_delivery_latency_seconds_bucket{${labels},le="${bucket}"} ${bucketCount}`);
        }
        lines.push(`${namespace}_listener_delivery_latency_seconds_bucket{${labels},le="+Inf"} ${count}`);
        lines.push(`${namespace}_listener_delivery_latency_seconds_sum{${labels}} ${sum.toFixed(6)}`);
        lines.push(`${namespace}_listener_delivery_latency_seconds_count{${labels}} ${count}`);
      }
    } else {
      // Fallback stub when no latency data available
      lines.push(`# HELP ${namespace}_listener_delivery_latency_seconds Event delivery latency in seconds`);
      lines.push(`# TYPE ${namespace}_listener_delivery_latency_seconds histogram`);
      lines.push(`${namespace}_listener_delivery_latency_seconds_count{${listenerLabels}} ${lm.events_delivered_total}`);
      lines.push(`${namespace}_listener_delivery_latency_seconds_sum{${listenerLabels}} 0`);
      lines.push(`${namespace}_listener_delivery_latency_seconds_bucket{${listenerLabels},le="1"} ${lm.events_delivered_total}`);
      lines.push(`${namespace}_listener_delivery_latency_seconds_bucket{${listenerLabels},le="+Inf"} ${lm.events_delivered_total}`);
    }
  }

  return lines.join("\n") + "\n"
}

/**
 * Create a Prometheus push gateway metrics sink.
 *
 * Pushes metrics to `POST <push_gateway_url>/metrics/job/<job_name>`.
 */
export function createPrometheusSink(options: PrometheusSinkOptions): MetricsSink {
  const { config, timeoutMs = 5_000 } = options
  const pushUrl = `${config.push_gateway_url.replace(/\/+$/, "")}/metrics/job/${encodeURIComponent(config.job_name)}`

  return {
    id: "prometheus",
    name: "Prometheus Push Gateway",

    async publish(event: MetricsEvent): Promise<SinkPublishResult> {
      try {
        const body = formatPrometheusMetrics(event, config.namespace)

        const controller = new AbortController()
        const timeout = setTimeout(() => controller.abort(), timeoutMs)

        try {
          const response = await fetch(pushUrl, {
            method: "POST",
            headers: { "Content-Type": "text/plain; version=0.0.4" },
            body,
            signal: controller.signal,
          })

          clearTimeout(timeout)

          if (response.ok) {
            return { success: true, retryable: false }
          }

          return {
            success: false,
            statusCode: response.status,
            retryable: response.status >= 500 || response.status === 429,
            error: `Prometheus push gateway returned ${response.status}: ${response.statusText}`,
          }
        } finally {
          clearTimeout(timeout)
        }
      } catch (err) {
        return {
          success: false,
          retryable: true,
          error: err instanceof Error ? err.message : "Prometheus push failed",
        }
      }
    },
  }
}
