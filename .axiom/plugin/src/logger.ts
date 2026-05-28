/**
 * Plugin logger — routes all plugin log output through OpenCode's structured
 * logger (client.app.log) so nothing pollutes the OpenCode UI stdout.
 *
 * Falls back to stderr (NOT stdout) when client is unavailable, so even
 * fallback output does not corrupt the OpenCode message stream.
 *
 * Usage:
 *   const log = createPluginLogger(client)
 *   log.info("Plugin initialized")
 *   log.warn("No sinks configured")
 *   log.error("Sink failed", { error: err.message })
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-037 (hooks must be non-blocking)
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-037 jira_ref=DEX-392
 */

import type { createOpencodeClient } from "@opencode-ai/sdk"

type OpenCodeClient = ReturnType<typeof createOpencodeClient>

export type LogLevel = "debug" | "info" | "warn" | "error"

export interface PluginLogger {
  debug(message: string, extra?: Record<string, unknown>): void
  info(message: string, extra?: Record<string, unknown>): void
  warn(message: string, extra?: Record<string, unknown>): void
  error(message: string, extra?: Record<string, unknown>): void
}

/**
 * Create a plugin logger that routes through OpenCode's structured logger.
 *
 * All output goes to OpenCode's log file — never to stdout (which would
 * corrupt the OpenCode UI message stream).
 *
 * @param client - The OpenCode SDK client from PluginInput
 * @param service - Service name for log entries (default: "axiom-plugin")
 */
export function createPluginLogger(
  client: OpenCodeClient | null | undefined,
  service = "axiom-plugin",
): PluginLogger {
  function log(level: LogLevel, message: string, extra?: Record<string, unknown>): void {
    const sdkLevel = level === "debug" ? "debug"
      : level === "info" ? "info"
      : level === "warn" ? "warn"
      : "error"

    if (client && typeof client.app?.log === "function") {
      // Route through OpenCode's structured logger — goes to log file, not stdout
      void client.app.log({
        body: {
          service,
          level: sdkLevel,
          message: `[axiom plugin] ${message}`,
          ...(extra ? { extra } : {}),
        },
      }).catch(() => {
        // If the log call fails, fall back to stderr (never stdout)
        process.stderr.write(`[axiom plugin] [${level.toUpperCase()}] ${message}\n`)
      })
    } else {
      // No client available — write to stderr only (never stdout)
      process.stderr.write(`[axiom plugin] [${level.toUpperCase()}] ${message}\n`)
    }
  }

  return {
    debug: (msg, extra) => log("debug", msg, extra),
    info: (msg, extra) => log("info", msg, extra),
    warn: (msg, extra) => log("warn", msg, extra),
    error: (msg, extra) => log("error", msg, extra),
  }
}

/** Singleton no-op logger for use before client is available. */
export const noopLogger: PluginLogger = {
  debug: () => {},
  info: () => {},
  warn: () => {},
  error: () => {},
}

/**
 * Minimal fallback logger for subsystems initialized before `client` is available.
 *
 * Routes to process.stderr (NEVER stdout) per REQ-PLG-134 clause 3.
 * Use this at module level in subsystems that cannot accept a PluginLogger at
 * construction time.
 *
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-134 plan=phase-70-fixes/task-f2-ext/step-f2-ext-1
 */
export function createFallbackLogger() {
  return {
    warn: (msg: string) => process.stderr.write(`[axiom plugin] WARN: ${msg}\n`),
    error: (msg: string) => process.stderr.write(`[axiom plugin] ERROR: ${msg}\n`),
  }
}
