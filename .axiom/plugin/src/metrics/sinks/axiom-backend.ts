/**
 * Axiom Backend metrics sink — POST events to the Axiom backend API.
 *
 * REQ-PLG-120: axiom-backend sink targets `POST <backend_url>/api/v1/events`.
 *   ON by default when `network.backend_url` is set.
 * REQ-PLG-128: Distinguish error types — 400/401 are non-retryable (bug/invalid token),
 *   429/503 are retryable with Retry-After header.
 * REQ-PLG-129: All publish calls MUST be non-blocking.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-120, #REQ-PLG-128
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-120 jira_ref=DEX-392
 */

import type { MetricsSink, MetricsEvent, SinkPublishResult } from "../sink-interface"

/** Options for creating a Axiom backend sink. */
export interface BackendSinkOptions {
  /** Backend base URL (e.g., "https://axiom.example.com"). */
  backendUrl: string
  /** API token from AXIOM_API_TOKEN env var. */
  apiToken: string
  /** Request timeout in milliseconds (default: 10000). */
  timeoutMs?: number
}

/**
 * Create a Axiom backend metrics sink.
 *
 * Posts events to `POST <backend_url>/api/v1/events` with bearer token auth.
 */
export function createBackendSink(options: BackendSinkOptions): MetricsSink {
  const { backendUrl, apiToken, timeoutMs = 10_000 } = options
  const endpoint = `${backendUrl.replace(/\/+$/, "")}/api/v1/events`

  return {
    id: "axiom-backend",
    name: "Axiom Backend API",

    async publish(event: MetricsEvent): Promise<SinkPublishResult> {
      try {
        const controller = new AbortController()
        const timeout = setTimeout(() => controller.abort(), timeoutMs)

        try {
          const response = await fetch(endpoint, {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
              Authorization: `Bearer ${apiToken}`,
            },
            body: JSON.stringify(event),
            signal: controller.signal,
          })

          clearTimeout(timeout)

          if (response.ok) {
            return { success: true, retryable: false }
          }

          const statusCode = response.status

          // REQ-PLG-128: 400/401 are non-retryable (bug or invalid token)
          if (statusCode === 400 || statusCode === 401) {
            return {
              success: false,
              statusCode,
              retryable: false,
              error: `Backend returned ${statusCode}: ${response.statusText}`,
            }
          }

          // REQ-PLG-128: 429/503 are retryable with Retry-After
          if (statusCode === 429 || statusCode === 503) {
            const retryAfter = response.headers.get("Retry-After")
            const retryAfterSeconds = retryAfter ? parseInt(retryAfter, 10) : undefined

            return {
              success: false,
              statusCode,
              retryable: true,
              error: `Backend returned ${statusCode}: ${response.statusText}`,
              retryAfterSeconds: Number.isFinite(retryAfterSeconds) ? retryAfterSeconds : undefined,
            }
          }

          // Other server errors — retryable
          return {
            success: false,
            statusCode,
            retryable: statusCode >= 500,
            error: `Backend returned ${statusCode}: ${response.statusText}`,
          }
        } finally {
          clearTimeout(timeout)
        }
      } catch (err) {
        // Network errors, timeouts — retryable
        const message = err instanceof Error ? err.message : "Unknown network error"
        return {
          success: false,
          retryable: true,
          error: message,
        }
      }
    },
  }
}
