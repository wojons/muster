/**
 * CloudWatch metrics sink — pushes metrics to AWS CloudWatch Metrics + Logs.
 *
 * REQ-PLG-120: cloudwatch sink targets AWS CloudWatch Metrics + Logs.
 *   OFF by default; enabled when `metrics.sinks` includes `cloudwatch`.
 *
 * Uses the AWS SDK v3 via dynamic import. If the SDK is not installed,
 * the sink returns a non-retryable failure with a helpful message.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-120
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-120 jira_ref=DEX-392
 */

import type { MetricsSink, MetricsEvent, SinkPublishResult, CloudWatchSinkConfig } from "../sink-interface"

/** Options for creating a CloudWatch sink. */
export interface CloudWatchSinkOptions {
  config: CloudWatchSinkConfig
}

/**
 * Build CloudWatch metric data from a MetricsEvent.
 *
 * Returns an array of metric datum objects compatible with the
 * CloudWatch PutMetricData API shape.
 */
export function buildCloudWatchMetricData(
  event: MetricsEvent,
  _namespace: string,
): Array<{
  MetricName: string
  Value: number
  Unit: string
  Dimensions: Array<{ Name: string; Value: string }>
  Timestamp: Date
}> {
  const timestamp = new Date(event.timestamp)
  const baseDimensions = [
    { Name: "SessionId", Value: event.session_id },
    { Name: "Repo", Value: event.repo },
  ]

  const data: Array<{
    MetricName: string
    Value: number
    Unit: string
    Dimensions: Array<{ Name: string; Value: string }>
    Timestamp: Date
  }> = []

  // Per-model metrics
  for (const [modelId, delta] of Object.entries(event.delta)) {
    const dims = [...baseDimensions, { Name: "Model", Value: modelId }]

    data.push(
      { MetricName: "InputTokens", Value: delta.input_tokens, Unit: "Count", Dimensions: dims, Timestamp: timestamp },
      { MetricName: "OutputTokens", Value: delta.output_tokens, Unit: "Count", Dimensions: dims, Timestamp: timestamp },
      { MetricName: "CacheReadTokens", Value: delta.cache_read_tokens, Unit: "Count", Dimensions: dims, Timestamp: timestamp },
      { MetricName: "CacheWriteTokens", Value: delta.cache_write_tokens, Unit: "Count", Dimensions: dims, Timestamp: timestamp },
      { MetricName: "CostMicrodollars", Value: delta.estimated_cost_microdollars, Unit: "Count", Dimensions: dims, Timestamp: timestamp },
      { MetricName: "LLMCalls", Value: delta.llm_calls, Unit: "Count", Dimensions: dims, Timestamp: timestamp },
    )
  }

  // Session-level metrics
  data.push(
    { MetricName: "Messages", Value: event.delta_session.messages, Unit: "Count", Dimensions: baseDimensions, Timestamp: timestamp },
    { MetricName: "ToolInvocations", Value: event.delta_session.tool_invocations, Unit: "Count", Dimensions: baseDimensions, Timestamp: timestamp },
    { MetricName: "SessionDuration", Value: event.session_totals.duration_seconds, Unit: "Seconds", Dimensions: baseDimensions, Timestamp: timestamp },
  )

  return data
}

/**
 * Build a CloudWatch Logs event from a MetricsEvent.
 *
 * Returns the log event shape for PutLogEvents API.
 */
export function buildCloudWatchLogEvent(event: MetricsEvent): {
  timestamp: number
  message: string
} {
  return {
    timestamp: new Date(event.timestamp).getTime(),
    message: JSON.stringify(event),
  }
}

/**
 * Create a CloudWatch metrics sink.
 *
 * Attempts to dynamically import `@aws-sdk/client-cloudwatch` and
 * `@aws-sdk/client-cloudwatch-logs`. If unavailable, returns a
 * non-retryable failure with installation instructions.
 */
export function createCloudWatchSink(options: CloudWatchSinkOptions): MetricsSink {
  const { config } = options

  return {
    id: "cloudwatch",
    name: "AWS CloudWatch",

    async publish(event: MetricsEvent): Promise<SinkPublishResult> {
      try {
        // Dynamic import — SDK may not be installed
        const cwModule = await import("@aws-sdk/client-cloudwatch").catch(() => null)
        const cwLogsModule = await import("@aws-sdk/client-cloudwatch-logs").catch(() => null)

        if (!cwModule) {
          return {
            success: false,
            retryable: false,
            error: "AWS SDK not installed. Run: npm install @aws-sdk/client-cloudwatch @aws-sdk/client-cloudwatch-logs",
          }
        }

        const cwClient = new cwModule.CloudWatchClient(
          config.region ? { region: config.region } : {},
        )

        // Push metrics
        const metricData = buildCloudWatchMetricData(event, config.namespace)

        // CloudWatch PutMetricData accepts max 1000 metrics per call
        // Batch in chunks of 25 (conservative for dimension combinations)
        for (let i = 0; i < metricData.length; i += 25) {
          const batch = metricData.slice(i, i + 25)
          await cwClient.send(
            new cwModule.PutMetricDataCommand({
              Namespace: config.namespace,
              MetricData: batch,
            }),
          )
        }

        // Push log event if logs module available
        if (cwLogsModule) {
          try {
            const logsClient = new cwLogsModule.CloudWatchLogsClient(
              config.region ? { region: config.region } : {},
            )
            const logEvent = buildCloudWatchLogEvent(event)
            const logStreamName = `axiom-${event.session_id}`

            // Ensure log group and stream exist (idempotent)
            try {
              await logsClient.send(
                new cwLogsModule.CreateLogGroupCommand({ logGroupName: config.log_group }),
              )
            } catch {
              // Already exists — expected
            }
            try {
              await logsClient.send(
                new cwLogsModule.CreateLogStreamCommand({
                  logGroupName: config.log_group,
                  logStreamName,
                }),
              )
            } catch {
              // Already exists — expected
            }

            await logsClient.send(
              new cwLogsModule.PutLogEventsCommand({
                logGroupName: config.log_group,
                logStreamName,
                logEvents: [logEvent],
              }),
            )
          } catch {
            // Log push failure is non-critical — metrics already sent
          }
        }

        return { success: true, retryable: false }
      } catch (err) {
        return {
          success: false,
          retryable: true,
          error: err instanceof Error ? err.message : "CloudWatch push failed",
        }
      }
    },
  }
}
