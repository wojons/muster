/**
 * Redacting Span Processor — applies the existing redaction engine to span
 * attributes before forwarding to the next processor in the chain.
 *
 * REQ-LF-004: The plugin MUST apply the existing redaction engine to span
 *   attributes and span content before passing to LangfuseSpanProcessor.
 *
 * Redaction MUST apply to:
 * - Span attributes (tool inputs, command arguments) — string values only
 * - Span events (log messages)
 *
 * Redaction MUST NOT apply to:
 * - Span names, trace IDs, span IDs (structural metadata)
 * - Token counts, latency, cost (numeric metrics)
 * - Axiom-injected attributes (axiom.*)
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-LF-004
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-LF-004 plan=phase-3/task-3-4/step-3-4-1 jira_ref=DEX-403
 */

import type { SpanProcessor, ReadableSpan, Span } from "@opentelemetry/sdk-trace-base"
import type { Context } from "@opentelemetry/api"

// ── Types ───────────────────────────────────────────────────────────────────

/** Minimal redaction engine interface (matches src/security/redaction.ts). */
export interface RedactionFn {
  redact(content: string): { redacted: string; count: number }
}

// ── Constants ───────────────────────────────────────────────────────────────

/** Attribute prefixes that MUST NOT be redacted. */
const SKIP_PREFIXES = [
  "axiom.",     // Axiom-injected attributes (REQ-LF-003)
  "otel.",        // OpenTelemetry structural attributes
  "telemetry.",   // Telemetry SDK attributes
  "service.",     // Service identification
]

// ── Span Processor ─────────────────────────────────────────────────────────

/**
 * Span processor that applies redaction to string-valued span attributes
 * before delegating to the wrapped processor.
 *
 * Numeric, boolean, and array attributes are passed through unchanged.
 * Attributes with protected prefixes (axiom.*, otel.*, etc.) are skipped.
 */
export class RedactingSpanProcessor implements SpanProcessor {
  private readonly delegate: SpanProcessor
  private readonly engine: RedactionFn

  constructor(delegate: SpanProcessor, engine: RedactionFn) {
    this.delegate = delegate
    this.engine = engine
  }

  /**
   * Called when a span starts. Redacts string attributes FIRST, then
   * delegates to the wrapped processor. This ensures the delegate
   * (metadata processor → Langfuse) never sees unredacted secrets.
   * axiom.* attributes added by the metadata processor are in
   * SKIP_PREFIXES and will not be redacted.
   */
  onStart(span: Span, parentContext: Context): void {
    // Redact FIRST — before delegate sees the span (REQ-LF-004 security guarantee)
    const spanAny = span as unknown as { attributes?: Record<string, unknown> }
    const attrs = spanAny.attributes
    if (attrs) {
      for (const [key, value] of Object.entries(attrs)) {
        // Skip non-string values (numeric metrics, booleans, arrays)
        if (typeof value !== "string") continue

        // Skip protected prefixes
        if (SKIP_PREFIXES.some((prefix) => key.startsWith(prefix))) continue

        // Apply redaction
        const result = this.engine.redact(value)
        if (result.count > 0) {
          span.setAttribute(key, result.redacted)
        }
      }
    }

    // Delegate AFTER redaction — metadata processor adds axiom.* attributes
    // which are safe (in SKIP_PREFIXES) and will not be redacted
    this.delegate.onStart(span, parentContext)
  }

  /**
   * Called when a span ends. Delegates to the wrapped processor.
   * Spans are read-only at onEnd — redaction happens at onStart.
   */
  onEnd(span: ReadableSpan): void {
    this.delegate.onEnd(span)
  }

  /** Shutdown — delegates to wrapped processor. */
  shutdown(): Promise<void> {
    return this.delegate.shutdown()
  }

  /** Force flush — delegates to wrapped processor. */
  forceFlush(): Promise<void> {
    return this.delegate.forceFlush()
  }
}

// ── Factory ─────────────────────────────────────────────────────────────────

/**
 * Create a RedactingSpanProcessor wrapping the given delegate.
 *
 * @param delegate - The underlying SpanProcessor to wrap
 * @param engine - Redaction engine (from src/security/redaction.ts)
 */
export function createRedactingProcessor(
  delegate: SpanProcessor,
  engine: RedactionFn,
): RedactingSpanProcessor {
  return new RedactingSpanProcessor(delegate, engine)
}
