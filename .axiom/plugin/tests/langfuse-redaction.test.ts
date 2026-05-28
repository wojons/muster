/**
 * Tests for Langfuse redaction span processor.
 *
 * REQ-LF-004: Redaction before sending to Langfuse.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-LF-004
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-LF-004 plan=phase-3/task-3-4/step-3-4-1 jira_ref=DEX-403
 */

import { describe, it, expect } from "bun:test"
import { RedactingSpanProcessor, createRedactingProcessor, type RedactionFn } from "../src/layers/langfuse/redaction"
import type { SpanProcessor } from "@opentelemetry/sdk-trace-base"
import type { Context, Span } from "@opentelemetry/api"

// ---------------------------------------------------------------------------
// Mock helpers
// ---------------------------------------------------------------------------

/** Create a mock redaction engine that replaces known secret patterns. */
function createMockRedactionEngine(): RedactionFn {
  return {
    redact(content: string) {
      let redacted = content
      let count = 0

      // Simulate AWS key redaction
      const awsPattern = /AKIA[A-Z0-9]{16}/g
      const awsMatches = redacted.match(awsPattern)
      if (awsMatches) {
        redacted = redacted.replace(awsPattern, "[REDACTED:aws-key]")
        count += awsMatches.length
      }

      // Simulate generic API key redaction
      const apiKeyPattern = /sk-[a-zA-Z0-9]{20,}/g
      const apiMatches = redacted.match(apiKeyPattern)
      if (apiMatches) {
        redacted = redacted.replace(apiKeyPattern, "[REDACTED:api-key]")
        count += apiMatches.length
      }

      return { redacted, count }
    },
  }
}

/** Create a mock span with trackable setAttribute calls. */
function createMockSpan(initialAttrs: Record<string, unknown> = {}): Span & {
  attributes: Record<string, unknown>
  setAttributeCalls: Array<{ key: string; value: unknown }>
} {
  const attributes: Record<string, unknown> = { ...initialAttrs }
  const setAttributeCalls: Array<{ key: string; value: unknown }> = []

  return {
    attributes,
    setAttributeCalls,
    setAttribute(key: string, value: unknown) {
      attributes[key] = value
      setAttributeCalls.push({ key, value })
      return this
    },
    setAttributes() { return this },
    addEvent() { return this },
    addLink() { return this },
    setStatus() { return this },
    updateName() { return this },
    end() {},
    isRecording() { return true },
    recordException() {},
    spanContext() {
      return { traceId: "trace-1", spanId: "span-1", traceFlags: 1 } as any
    },
  } as any
}

/** Create a mock delegate processor that tracks calls. */
function createMockDelegate(): SpanProcessor & {
  onStartCalls: number
  onEndCalls: number
  shutdownCalled: boolean
  flushCalled: boolean
} {
  return {
    onStartCalls: 0,
    onEndCalls: 0,
    shutdownCalled: false,
    flushCalled: false,
    onStart() { this.onStartCalls++ },
    onEnd() { this.onEndCalls++ },
    async shutdown() { this.shutdownCalled = true },
    async forceFlush() { this.flushCalled = true },
  }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("RedactingSpanProcessor (REQ-LF-004)", () => {
  it("redacts string attributes containing secrets", () => {
    const engine = createMockRedactionEngine()
    const delegate = createMockDelegate()
    const processor = createRedactingProcessor(delegate, engine)

    const span = createMockSpan({
      "tool.input": "Use key AKIAIOSFODNN7EXAMPLE to access S3",
      "command.args": "safe content here",
    })

    processor.onStart(span, undefined as unknown as Context)

    // Secret should be redacted
    expect(span.attributes["tool.input"]).toContain("[REDACTED:aws-key]")
    expect(span.attributes["tool.input"]).not.toContain("AKIAIOSFODNN7EXAMPLE")

    // Safe content should be unchanged
    expect(span.attributes["command.args"]).toBe("safe content here")
  })

  it("does NOT redact numeric attributes", () => {
    const engine = createMockRedactionEngine()
    const delegate = createMockDelegate()
    const processor = createRedactingProcessor(delegate, engine)

    const span = createMockSpan({
      "token.count": 42,
      "latency.ms": 150.5,
      "cost.microdollars": 70000,
    })

    processor.onStart(span, undefined as unknown as Context)

    expect(span.attributes["token.count"]).toBe(42)
    expect(span.attributes["latency.ms"]).toBe(150.5)
    expect(span.attributes["cost.microdollars"]).toBe(70000)
  })

  it("does NOT redact axiom.* prefixed attributes", () => {
    const engine = createMockRedactionEngine()
    const delegate = createMockDelegate()
    const processor = createRedactingProcessor(delegate, engine)

    const span = createMockSpan({
      "axiom.work_item_id": "DEX-392",
      "axiom.user_email": "alice@example.com",
      "axiom.session_identity": "github.com/acme/backend",
    })

    processor.onStart(span, undefined as unknown as Context)

    // axiom.* attributes must NOT be redacted
    expect(span.attributes["axiom.work_item_id"]).toBe("DEX-392")
    expect(span.attributes["axiom.user_email"]).toBe("alice@example.com")
    expect(span.attributes["axiom.session_identity"]).toBe("github.com/acme/backend")
  })

  it("does NOT redact otel.* structural attributes", () => {
    const engine = createMockRedactionEngine()
    const delegate = createMockDelegate()
    const processor = createRedactingProcessor(delegate, engine)

    const span = createMockSpan({
      "otel.library.name": "opencode",
      "service.name": "axiom-plugin",
    })

    processor.onStart(span, undefined as unknown as Context)

    expect(span.attributes["otel.library.name"]).toBe("opencode")
    expect(span.attributes["service.name"]).toBe("axiom-plugin")
  })

  it("delegates onStart to wrapped processor", () => {
    const engine = createMockRedactionEngine()
    const delegate = createMockDelegate()
    const processor = createRedactingProcessor(delegate, engine)

    const span = createMockSpan()
    processor.onStart(span, undefined as unknown as Context)

    expect(delegate.onStartCalls).toBe(1)
  })

  it("delegates onEnd to wrapped processor", () => {
    const engine = createMockRedactionEngine()
    const delegate = createMockDelegate()
    const processor = createRedactingProcessor(delegate, engine)

    const span = createMockSpan()
    processor.onEnd(span)

    expect(delegate.onEndCalls).toBe(1)
  })

  it("delegates shutdown to wrapped processor", async () => {
    const engine = createMockRedactionEngine()
    const delegate = createMockDelegate()
    const processor = createRedactingProcessor(delegate, engine)

    await processor.shutdown()
    expect(delegate.shutdownCalled).toBe(true)
  })

  it("delegates forceFlush to wrapped processor", async () => {
    const engine = createMockRedactionEngine()
    const delegate = createMockDelegate()
    const processor = createRedactingProcessor(delegate, engine)

    await processor.forceFlush()
    expect(delegate.flushCalled).toBe(true)
  })

  it("handles sk-* API key patterns", () => {
    const engine = createMockRedactionEngine()
    const delegate = createMockDelegate()
    const processor = createRedactingProcessor(delegate, engine)

    const span = createMockSpan({
      "gen.prompt": "Call the API with sk-proj1234567890abcdefghij as the key",
    })

    processor.onStart(span, undefined as unknown as Context)

    expect(span.attributes["gen.prompt"]).toContain("[REDACTED:api-key]")
    expect(span.attributes["gen.prompt"]).not.toContain("sk-proj1234567890abcdefghij")
  })

  it("handles boolean attributes without error", () => {
    const engine = createMockRedactionEngine()
    const delegate = createMockDelegate()
    const processor = createRedactingProcessor(delegate, engine)

    const span = createMockSpan({
      "is.cached": true,
      "is.error": false,
    })

    processor.onStart(span, undefined as unknown as Context)

    expect(span.attributes["is.cached"]).toBe(true)
    expect(span.attributes["is.error"]).toBe(false)
  })

  it("handles span with no attributes gracefully", () => {
    const engine = createMockRedactionEngine()
    const delegate = createMockDelegate()
    const processor = createRedactingProcessor(delegate, engine)

    // Span without .attributes property
    const span = {
      setAttribute() { return this },
      setAttributes() { return this },
      addEvent() { return this },
      addLink() { return this },
      setStatus() { return this },
      updateName() { return this },
      end() {},
      isRecording() { return true },
      recordException() {},
      spanContext() { return { traceId: "t", spanId: "s", traceFlags: 1 } as any },
    } as unknown as Span

    // Should not throw
    processor.onStart(span, undefined as unknown as Context)
    expect(delegate.onStartCalls).toBe(1)
  })
})

describe("createRedactingProcessor factory", () => {
  it("returns a RedactingSpanProcessor instance", () => {
    const engine = createMockRedactionEngine()
    const delegate = createMockDelegate()
    const processor = createRedactingProcessor(delegate, engine)

    expect(processor).toBeInstanceOf(RedactingSpanProcessor)
  })
})
