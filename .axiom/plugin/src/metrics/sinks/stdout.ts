/**
 * OpenCode log sink — routes metrics events through OpenCode's structured logger.
 *
 * REQ-PLG-120: "stdout" sink is always available as fallback.
 * REQ-PLG-130: When no sinks are configured, the plugin MUST still emit events.
 *
 * IMPORTANT: This sink uses client.app.log() (OpenCode's log file), NOT
 * process.stdout. Writing to process.stdout would corrupt the OpenCode UI
 * message stream. All plugin output MUST go through the OpenCode logger or
 * to a file — never to process.stdout.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-120, #REQ-PLG-130
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-120 jira_ref=DEX-392
 */

import type { MetricsSink, MetricsEvent, SinkPublishResult } from "../sink-interface"
import type { PluginLogger } from "../../logger"

/**
 * Create the OpenCode log sink.
 *
 * Routes each metrics event as structured JSON through the plugin logger
 * (which uses client.app.log → OpenCode log file). Never writes to stdout.
 *
 * @param logger - Plugin logger from createPluginLogger(client)
 */
export function createStdoutSink(logger: PluginLogger): MetricsSink {
  return {
    id: "stdout",
    name: "OpenCode log (structured)",

    async publish(event: MetricsEvent): Promise<SinkPublishResult> {
      try {
        // Route through OpenCode's logger — goes to log file, not UI stdout
        logger.info(JSON.stringify(event))
        return { success: true, retryable: false }
      } catch (err) {
        return {
          success: false,
          retryable: false,
          error: err instanceof Error ? err.message : "log write failed",
        }
      }
    },
  }
}
