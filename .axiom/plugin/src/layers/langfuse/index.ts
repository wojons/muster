/**
 * Langfuse OTEL Layer — initializes OpenTelemetry SDK with LangfuseSpanProcessor
 * and the Axiom metadata + redaction processor chain.
 *
 * REQ-LF-001: Initialize LangfuseSpanProcessor when Langfuse is configured.
 *   Validate experimental.openTelemetry: true. Flush on session.idle.
 *   Shutdown on server.instance.disposed. All ops non-blocking.
 * REQ-LF-002: Backend proxy mode (primary) or direct mode (fallback).
 * REQ-LF-003: Inject Axiom metadata into every span.
 * REQ-LF-004: Redact secrets before sending.
 *
 * Processor chain: RedactingSpanProcessor → CodeOpsMetadataProcessor → LangfuseSpanProcessor
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-LF-001 through REQ-LF-004
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-134 plan=phase-70-fixes/task-f2-ext/step-f2-ext-1
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-LF-001 plan=phase-3/task-3-5/step-3-5-1 jira_ref=DEX-403
 */

import { fetchLangfuseConfig, type LangfuseConfigResult } from "./config"
import { resolveCodeOpsMetadata, createCodeOpsMetadataProcessor } from "./metadata"
import { createRedactingProcessor, type RedactionFn } from "./redaction"
import { createFallbackLogger } from "../../logger"

// Module-level fallback logger — routes to stderr, never stdout (REQ-PLG-134)
const _log = createFallbackLogger()

// ── Types ───────────────────────────────────────────────────────────────────

/** Options for initializing the Langfuse layer. */
export interface LangfuseLayerOptions {
  /** Repo root directory. */
  repoRoot: string
  /** Backend URL (from network.backend_url config). */
  backendUrl?: string
  /** API token (from AXIOM_API_TOKEN env var). */
  apiToken?: string
  /** Whether proxy mode is enabled (from langfuse.proxy_mode config). */
  proxyMode?: boolean
  /** Cache TTL in seconds (from langfuse.cache_ttl_seconds config). */
  cacheTtlSeconds?: number
  /** Redaction engine instance (from src/security/redaction.ts). */
  redactionEngine?: RedactionFn
  /** Whether experimental.openTelemetry is enabled in OpenCode config. */
  otelEnabled?: boolean
}

/** Result of Langfuse layer initialization. */
export interface LangfuseLayerResult {
  /** Whether the layer was successfully initialized. */
  initialized: boolean
  /** Why initialization was skipped (if not initialized). */
  skipReason?: string
  /** Config source used. */
  configSource?: string
  /** Flush function — call on session.idle. */
  flush: () => Promise<void>
  /** Shutdown function — call on server.instance.disposed. */
  shutdown: () => Promise<void>
}

// ── Initialization ─────────────────────────────────────────────────────────

/**
 * Initialize the Langfuse OTEL layer.
 *
 * Builds the processor chain:
 *   RedactingSpanProcessor → CodeOpsMetadataProcessor → LangfuseSpanProcessor
 *
 * Then initializes the NodeSDK with this chain.
 *
 * Returns flush/shutdown functions for lifecycle management.
 */
export async function initLangfuse(
  options: LangfuseLayerOptions,
): Promise<LangfuseLayerResult> {
  const {
    repoRoot,
    backendUrl,
    apiToken,
    proxyMode = false,
    cacheTtlSeconds = 3600,
    redactionEngine,
    otelEnabled,
  } = options

  // REQ-LF-001: Validate experimental.openTelemetry: true
  if (otelEnabled === false) {
    _log.warn(
      "Langfuse: experimental.openTelemetry is not enabled in OpenCode config. Skipping Langfuse initialization.",
    )
    return {
      initialized: false,
      skipReason: "experimental.openTelemetry not enabled",
      flush: async () => {},
      shutdown: async () => {},
    }
  }

  // Fetch Langfuse config (backend → cache → env vars → defaults)
  let configResult: LangfuseConfigResult
  try {
    configResult = await fetchLangfuseConfig({
      repoRoot,
      backendUrl,
      apiToken,
      proxyMode,
      cacheTtlSeconds,
    })
  } catch (err) {
    _log.warn(
      `Langfuse: config fetch failed: ${err instanceof Error ? err.message : "unknown"}. Skipping.`,
    )
    return {
      initialized: false,
      skipReason: "config fetch failed",
      flush: async () => {},
      shutdown: async () => {},
    }
  }

  // If no public key available, skip
  if (!configResult.config.public_key) {
    _log.warn(
      "Langfuse: no public key configured. Skipping Langfuse initialization.",
    )
    return {
      initialized: false,
      skipReason: "no public key",
      flush: async () => {},
      shutdown: async () => {},
    }
  }

  // Dynamic import of OTEL and Langfuse modules
  let NodeSDK: typeof import("@opentelemetry/sdk-node").NodeSDK | undefined
  let LangfuseSpanProcessor: unknown

  try {
    const sdkModule = await import("@opentelemetry/sdk-node")
    NodeSDK = sdkModule.NodeSDK as unknown as typeof import("@opentelemetry/sdk-node").NodeSDK

    const langfuseModule = await import("@langfuse/otel")
    LangfuseSpanProcessor = (langfuseModule as Record<string, unknown>).LangfuseSpanProcessor
  } catch (err) {
    _log.warn(
      `Langfuse: failed to import OTEL/Langfuse modules: ${err instanceof Error ? err.message : "unknown"}. Skipping.`,
    )
    return {
      initialized: false,
      skipReason: "module import failed",
      flush: async () => {},
      shutdown: async () => {},
    }
  }

  if (!LangfuseSpanProcessor || !NodeSDK) {
    return {
      initialized: false,
      skipReason: "LangfuseSpanProcessor or NodeSDK not found",
      flush: async () => {},
      shutdown: async () => {},
    }
  }

  // Build the processor chain
  // Layer 1 (innermost): LangfuseSpanProcessor — sends to Langfuse
  const langfuseProcessor = new (LangfuseSpanProcessor as new (opts: Record<string, unknown>) => import("@opentelemetry/sdk-trace-base").SpanProcessor)({
    publicKey: configResult.config.public_key,
    secretKey: configResult.config.secret_key,
    baseUrl: configResult.config.base_url,
    environment: configResult.config.environment,
  })

  // Layer 2: CodeOpsMetadataProcessor — injects axiom.* attributes
  const metadata = resolveCodeOpsMetadata(repoRoot)
  const metadataProcessor = createCodeOpsMetadataProcessor(langfuseProcessor, metadata)

  // Layer 3 (outermost): RedactingSpanProcessor — strips secrets
  let outerProcessor: import("@opentelemetry/sdk-trace-base").SpanProcessor = metadataProcessor
  if (redactionEngine) {
    outerProcessor = createRedactingProcessor(metadataProcessor, redactionEngine)
  }

  // Initialize NodeSDK
  let sdk: InstanceType<typeof NodeSDK> | null = null
  try {
    sdk = new NodeSDK({
      spanProcessors: [outerProcessor],
    })
    sdk.start()
  } catch (err) {
    _log.warn(
      `Langfuse: NodeSDK initialization failed: ${err instanceof Error ? err.message : "unknown"}. Skipping.`,
    )
    return {
      initialized: false,
      skipReason: "NodeSDK init failed",
      flush: async () => {},
      shutdown: async () => {},
    }
  }

  return {
    initialized: true,
    configSource: configResult.source,
    flush: async () => {
      try {
        await outerProcessor.forceFlush()
      } catch {
        // Non-blocking — flush failure is not critical
      }
    },
    shutdown: async () => {
      try {
        if (sdk) {
          await sdk.shutdown()
        }
      } catch {
        // Non-blocking — shutdown failure is not critical
      }
    },
  }
}
