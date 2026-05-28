/**
 * Ambient type declarations for optional AWS SDK dependencies.
 *
 * The CloudWatch metrics sink uses dynamic import() to load these modules
 * at runtime. They are NOT bundled dependencies — they must be installed
 * separately by users who enable the CloudWatch sink.
 *
 * These declarations provide just enough type information for TypeScript
 * to compile the dynamic import() calls without errors, while keeping
 * the AWS SDK as a truly optional dependency.
 *
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-120 jira_ref=DEX-392
 */

declare module "@aws-sdk/client-cloudwatch" {
  export class CloudWatchClient {
    constructor(config?: Record<string, unknown>)
    send(command: unknown): Promise<unknown>
  }
  export class PutMetricDataCommand {
    constructor(input: {
      Namespace: string
      MetricData: Array<{
        MetricName: string
        Value: number
        Unit: string
        Dimensions: Array<{ Name: string; Value: string }>
        Timestamp: Date
      }>
    })
  }
}

declare module "@aws-sdk/client-cloudwatch-logs" {
  export class CloudWatchLogsClient {
    constructor(config?: Record<string, unknown>)
    send(command: unknown): Promise<unknown>
  }
  export class CreateLogGroupCommand {
    constructor(input: { logGroupName: string })
  }
  export class CreateLogStreamCommand {
    constructor(input: { logGroupName: string; logStreamName: string })
  }
  export class PutLogEventsCommand {
    constructor(input: {
      logGroupName: string
      logStreamName: string
      logEvents: Array<{ timestamp: number; message: string }>
    })
  }
}
