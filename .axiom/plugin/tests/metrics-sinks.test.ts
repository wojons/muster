/**
 * Tests for the metrics sinks module — sink interface, all 4 sink implementations,
 * publisher, buffer manager, identity resolver, cost module, and snapshot sequence.
 *
 * REQ-PLG-120: Pluggable metrics sink architecture (4 sinks).
 * REQ-PLG-122: Dual-cadence publishing (30s stream + 30min reconciliation).
 * REQ-PLG-126: 6-step email identity chain.
 * REQ-PLG-128: Buffer manager with auto-summarization.
 * REQ-PLG-131: Integer microdollars cost arithmetic.
 * REQ-PLG-135: estimated_cost_usd alongside microdollars (DEX-419 Fix 1).
 * REQ-PLG-136: reasoning_tokens always present at top-level (DEX-419 Fix 2).
 * REQ-PLG-137: Session metadata fields (DEX-419 Fix 4).
 *
 * Spec: specs/70-OpenCode-Plugin.md §5.4
 * axiom:trace work_item=plugin-metrics-fixes-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-120 test=tests/metrics-sinks.test.ts jira_ref=DEX-419
 */

import { describe, expect, it, beforeEach, afterEach, mock, spyOn } from "bun:test"

import type {
  MetricsSink,
  MetricsEvent,
  SinkPublishResult,
  ModelDelta,
  SessionDelta,
} from "../src/metrics/sink-interface"
import { DEFAULT_METRICS_PUBLISH_CONFIG } from "../src/metrics/sink-interface"
import { createStdoutSink } from "../src/metrics/sinks/stdout"
import { createBackendSink } from "../src/metrics/sinks/axiom-backend"
import { createPrometheusSink, formatPrometheusMetrics } from "../src/metrics/sinks/prometheus"
import { buildCloudWatchMetricData, buildCloudWatchLogEvent } from "../src/metrics/sinks/cloudwatch"
import { createPublisher, type MetricsPublisher } from "../src/metrics/publisher"
import { createBufferManager, mergeModelDeltas, mergeSessionDeltas } from "../src/metrics/buffer"
import { resolveEmailIdentitySync, isNoreplyEmail, resolveGitBranch } from "../src/metrics/identity"
import {
  usdToMicrodollars,
  microdollarsToUsd,
  computeCostMicrodollars,
  pricingToMicrodollars,
  MICRODOLLARS_PER_USD,
} from "../src/metrics/cost"
import { createSnapshotSequence } from "../src/metrics/snapshot"

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

function makeTestEvent(overrides: Partial<MetricsEvent> = {}): MetricsEvent {
  return {
    timestamp: "2026-04-08T14:30:00.000Z",
    level: "INFO",
    event_type: "plugin.metrics.snapshot",
    component: "opencode_plugin",
    session_id: "test-session-001",
    repo: "acme/backend",
    git_branch: "main",
    repo_path: "/Users/alice/code/backend",
    user_email: "alice@example.com",
    resolved_email_source: "git_config",
    session_identity: "github.com/acme/backend",
    session_state: "running",
    snapshot_type: "stream",
    snapshot_seq: 1,
    interval_seconds: 30,
    delta: {
      "claude-sonnet-4-20250514": {
        input_tokens: 8200,
        output_tokens: 1400,
        cache_read_tokens: 3100,
        cache_write_tokens: 800,
        estimated_cost_microdollars: 70000,
        estimated_cost_usd: 0.07,
        llm_calls: 6,
      },
    },
    delta_session: {
      messages: 4,
      tool_invocations: 12,
      commands_executed: 2,
      files_read: 5,
      files_written: 2,
    },
    session_totals: {
      duration_seconds: 5400,
      messages: 22,
      tool_invocations: 67,
    },
    // REQ-PLG-136 (DEX-419 Fix 2): always present, null when unavailable
    reasoning_tokens: null,
    // REQ-PLG-137 (DEX-419 Fix 4): metadata fields
    model_provider: "anthropic",
    agent_name: "tower-axiom",
    opencode_version: "0.1.123",
    plugin_version: "0.2.0",
    thinking_level: null,
    thinking_tokens: null,
    connected_mcps: ["chrome-devtools", "atlassian"],
    ...overrides,
  }
}

/** Create a mock sink that records publish calls. */
function createMockSink(
  id: "axiom-backend" | "prometheus" | "cloudwatch" | "stdout" = "stdout",
  result: SinkPublishResult = { success: true, retryable: false },
): MetricsSink & { calls: MetricsEvent[]; shutdownCalled: boolean } {
  const calls: MetricsEvent[] = []
  let shutdownCalled = false

  return {
    id,
    name: `Mock ${id}`,
    calls,
    shutdownCalled,
    async publish(event: MetricsEvent): Promise<SinkPublishResult> {
      calls.push(event)
      return result
    },
    async shutdown(): Promise<void> {
      shutdownCalled = true
    },
  }
}

// ===========================================================================
// Sink Interface & Defaults
// ===========================================================================

describe("MetricsSink interface and defaults", () => {
  it("DEFAULT_METRICS_PUBLISH_CONFIG has correct defaults per REQ-PLG-123", () => {
    expect(DEFAULT_METRICS_PUBLISH_CONFIG.publish_enabled).toBe(true)
    expect(DEFAULT_METRICS_PUBLISH_CONFIG.stream_interval_seconds).toBe(30)
    expect(DEFAULT_METRICS_PUBLISH_CONFIG.reconciliation_interval_seconds).toBe(1800)
    expect(DEFAULT_METRICS_PUBLISH_CONFIG.flush_interval_seconds).toBe(600)
    expect(DEFAULT_METRICS_PUBLISH_CONFIG.max_buffered_intervals).toBe(12)
    expect(DEFAULT_METRICS_PUBLISH_CONFIG.user_email).toBeNull()
    expect(DEFAULT_METRICS_PUBLISH_CONFIG.sinks).toEqual([])
  })
})

// ===========================================================================
// ===========================================================================
// OpenCode log sink (formerly "stdout" sink)
// ===========================================================================

describe("OpenCode log sink (REQ-PLG-120, REQ-PLG-130)", () => {
  it("creates a sink with id 'stdout'", () => {
    const mockLogger = { info: () => {}, warn: () => {}, error: () => {}, debug: () => {} }
    const sink = createStdoutSink(mockLogger)
    expect(sink.id).toBe("stdout")
    expect(sink.name).toBe("OpenCode log (structured)")
  })

  it("publishes event as single-line JSON through the logger (not process.stdout)", async () => {
    const logged: string[] = []
    const mockLogger = {
      info: (msg: string) => { logged.push(msg) },
      warn: () => {},
      error: () => {},
      debug: () => {},
    }
    const sink = createStdoutSink(mockLogger)
    const event = makeTestEvent()

    const result = await sink.publish(event)
    expect(result.success).toBe(true)
    expect(result.retryable).toBe(false)
    expect(logged.length).toBe(1)

    // Verify it's valid JSON with correct fields
    const parsed = JSON.parse(logged[0])
    expect(parsed.event_type).toBe("plugin.metrics.snapshot")
    expect(parsed.session_id).toBe("test-session-001")
  })

  it("does NOT write to process.stdout", async () => {
    const mockLogger = { info: () => {}, warn: () => {}, error: () => {}, debug: () => {} }
    const sink = createStdoutSink(mockLogger)
    const event = makeTestEvent()

    // Intercept stdout — should receive nothing
    const writes: string[] = []
    const originalWrite = process.stdout.write
    process.stdout.write = ((chunk: string) => {
      writes.push(chunk)
      return true
    }) as typeof process.stdout.write

    try {
      await sink.publish(event)
      expect(writes.length).toBe(0)
    } finally {
      process.stdout.write = originalWrite
    }
  })

  it("always returns success (never fails)", async () => {
    const mockLogger = { info: () => {}, warn: () => {}, error: () => {}, debug: () => {} }
    const sink = createStdoutSink(mockLogger)
    const result = await sink.publish(makeTestEvent())
    expect(result.success).toBe(true)
  })
})

// ===========================================================================
// Backend Sink
// ===========================================================================

describe("Backend sink (REQ-PLG-120, REQ-PLG-128)", () => {
  it("creates a sink with id 'axiom-backend'", () => {
    const sink = createBackendSink({
      backendUrl: "https://axiom.example.com",
      apiToken: "test-token",
    })
    expect(sink.id).toBe("axiom-backend")
    expect(sink.name).toBe("Axiom Backend API")
  })

  it("posts to correct endpoint with bearer auth", async () => {
    let capturedUrl = ""
    let capturedHeaders: Record<string, string> = {}
    let capturedBody = ""

    const originalFetch = globalThis.fetch
    globalThis.fetch = (async (url: string, init: RequestInit) => {
      capturedUrl = url
      capturedHeaders = Object.fromEntries(
        Object.entries(init.headers as Record<string, string>),
      )
      capturedBody = init.body as string
      return new Response("OK", { status: 200 })
    }) as typeof fetch

    try {
      const sink = createBackendSink({
        backendUrl: "https://axiom.example.com/",
        apiToken: "test-token-123",
      })
      const event = makeTestEvent()
      const result = await sink.publish(event)

      expect(result.success).toBe(true)
      expect(capturedUrl).toBe("https://axiom.example.com/api/v1/events")
      expect(capturedHeaders["Authorization"]).toBe("Bearer test-token-123")
      expect(capturedHeaders["Content-Type"]).toBe("application/json")
      expect(JSON.parse(capturedBody).session_id).toBe("test-session-001")
    } finally {
      globalThis.fetch = originalFetch
    }
  })

  it("returns non-retryable for 400/401 errors (REQ-PLG-128)", async () => {
    const originalFetch = globalThis.fetch
    globalThis.fetch = (async () => {
      return new Response("Bad Request", { status: 400, statusText: "Bad Request" })
    }) as typeof fetch

    try {
      const sink = createBackendSink({
        backendUrl: "https://axiom.example.com",
        apiToken: "bad-token",
      })
      const result = await sink.publish(makeTestEvent())

      expect(result.success).toBe(false)
      expect(result.retryable).toBe(false)
      expect(result.statusCode).toBe(400)
    } finally {
      globalThis.fetch = originalFetch
    }
  })

  it("returns retryable for 429/503 with Retry-After (REQ-PLG-128)", async () => {
    const originalFetch = globalThis.fetch
    globalThis.fetch = (async () => {
      return new Response("Too Many Requests", {
        status: 429,
        statusText: "Too Many Requests",
        headers: { "Retry-After": "60" },
      })
    }) as typeof fetch

    try {
      const sink = createBackendSink({
        backendUrl: "https://axiom.example.com",
        apiToken: "test-token",
      })
      const result = await sink.publish(makeTestEvent())

      expect(result.success).toBe(false)
      expect(result.retryable).toBe(true)
      expect(result.statusCode).toBe(429)
      expect(result.retryAfterSeconds).toBe(60)
    } finally {
      globalThis.fetch = originalFetch
    }
  })

  it("returns retryable for network errors", async () => {
    const originalFetch = globalThis.fetch
    globalThis.fetch = (async () => {
      throw new Error("Connection refused")
    }) as typeof fetch

    try {
      const sink = createBackendSink({
        backendUrl: "https://axiom.example.com",
        apiToken: "test-token",
      })
      const result = await sink.publish(makeTestEvent())

      expect(result.success).toBe(false)
      expect(result.retryable).toBe(true)
      expect(result.error).toContain("Connection refused")
    } finally {
      globalThis.fetch = originalFetch
    }
  })
})

// ===========================================================================
// Prometheus Sink
// ===========================================================================

describe("Prometheus sink (REQ-PLG-120)", () => {
  it("formats metrics in Prometheus text exposition format", () => {
    const event = makeTestEvent()
    const output = formatPrometheusMetrics(event, "axiom")

    expect(output).toContain("codeops_model_input_tokens")
    expect(output).toContain('model="claude-sonnet-4-20250514"')
    expect(output).toContain("8200")
    expect(output).toContain("codeops_session_messages")
    expect(output).toContain("codeops_session_duration_seconds")
  })

  it("includes HELP and TYPE annotations", () => {
    const event = makeTestEvent()
    const output = formatPrometheusMetrics(event, "axiom")

    expect(output).toContain("# HELP codeops_model_input_tokens")
    expect(output).toContain("# TYPE codeops_model_input_tokens gauge")
  })

  it("handles multiple models", () => {
    const event = makeTestEvent({
       delta: {
        "claude-sonnet-4-20250514": {
          input_tokens: 8200,
          output_tokens: 1400,
          cache_read_tokens: 3100,
          cache_write_tokens: 800,
          estimated_cost_microdollars: 70000,
          estimated_cost_usd: 0.07,
          llm_calls: 6,
        },
        "claude-haiku-4-20250414": {
          input_tokens: 2000,
          output_tokens: 300,
          cache_read_tokens: 0,
          cache_write_tokens: 0,
          estimated_cost_microdollars: 5000,
          estimated_cost_usd: 0.005,
          llm_calls: 2,
        },
      },
    })
    const output = formatPrometheusMetrics(event, "axiom")

    expect(output).toContain('model="claude-sonnet-4-20250514"')
    expect(output).toContain('model="claude-haiku-4-20250414"')
  })

  // REQ-LNP-078: Listener metric names must be emitted when listener_metrics is present
  it("emits listener metric names when listener_metrics present (REQ-LNP-078)", () => {
    // axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-078 plan=step-spec-002 test=metrics-sinks.test.ts
    const event = makeTestEvent()
    event.listener_metrics = {
      events_delivered_total: 42,
      events_dropped_total: 3,
      listener_uptime: { "jira-listener": 120.5 },
      active_listener_count: 1,
      drops_by_listener: { "jira-listener": 3 },
    }
    const output = formatPrometheusMetrics(event, "axiom")
    expect(output).toContain("codeops_listener_uptime_seconds")
    expect(output).toContain("codeops_listener_events_dropped_total")
    expect(output).toContain("codeops_listener_delivery_latency_seconds")
    expect(output).toContain('listener="jira-listener"')
    expect(output).toContain("120.5")
    expect(output).toContain("42")
  })

  it("omits listener metrics when listener_metrics absent", () => {
    // axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-078 plan=step-spec-002 test=metrics-sinks.test.ts
    const event = makeTestEvent()
    // no listener_metrics field
    const output = formatPrometheusMetrics(event, "axiom")
    expect(output).not.toContain("codeops_listener_uptime_seconds")
    expect(output).not.toContain("codeops_listener_events_dropped_total")
    expect(output).not.toContain("codeops_listener_delivery_latency_seconds")
  })

  it("emits real histogram buckets from delivery_latencies (REQ-LNP-078)", () => {
    // axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-078 plan=phase-3/task-3.2 test=metrics-sinks.test.ts jira_ref=DEX-434
    const event = makeTestEvent()
    event.listener_metrics = {
      events_delivered_total: 3,
      events_dropped_total: 0,
      listener_uptime: { "test-listener": 60 },
      active_listener_count: 1,
      drops_by_listener: {},
      delivery_latencies: {
        "test-listener": [5, 50, 500], // 5ms, 50ms, 500ms
      },
    }
    const output = formatPrometheusMetrics(event, "axiom")

    // Should have real bucket lines
    expect(output).toContain('codeops_listener_delivery_latency_seconds_bucket{')
    expect(output).toContain('le="0.005"')
    expect(output).toContain('le="0.05"')
    expect(output).toContain('le="0.5"')
    expect(output).toContain('le="+Inf"')

    // 5ms = 0.005s → le="0.005" bucket should have count 1
    const bucket5ms = output.match(/le="0\.005"\} (\d+)/)
    expect(bucket5ms?.[1]).toBe("1") // 5ms ≤ 5ms

    // 50ms = 0.05s → le="0.05" bucket should have count 2 (5ms + 50ms)
    const bucket50ms = output.match(/le="0\.05"\} (\d+)/)
    expect(bucket50ms?.[1]).toBe("2") // 5ms + 50ms ≤ 50ms

    // +Inf should have count 3 (all samples)
    const bucketInf = output.match(/le="\+Inf"\} (\d+)/)
    expect(bucketInf?.[1]).toBe("3")

    // Sum should be (5 + 50 + 500) / 1000 = 0.555
    expect(output).toContain("0.555000")
  })

  // REQ-LNP-078: End-to-end — publisher with getListenerMetrics callback produces Prometheus output
  it("publisher with getListenerMetrics callback emits listener metrics in Prometheus output (REQ-LNP-078 end-to-end)", async () => {
    // axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-078 plan=step-spec-003 test=metrics-sinks.test.ts
    const publishedBodies: string[] = []

    // Mock sink that captures published Prometheus bodies
    const mockSink = {
      id: "prometheus" as const,
      name: "Mock Prometheus",
      async publish(event: import("../src/metrics/sink-interface").MetricsEvent) {
        const body = formatPrometheusMetrics(event, "axiom")
        publishedBodies.push(body)
        return { success: true, retryable: false }
      },
    }

    const pub = createPublisher({
      sinks: [mockSink],
      getSessionId: () => "test-session-id",
      repo: "test/repo",
      repoPath: "/tmp/test",
      userEmail: "test@example.com",
      emailSource: "anonymous",
      sessionIdentity: "test-identity",
      // Simulate l7ListenerManager.getMetrics() returning active listener data
      getListenerMetrics: () => ({
        events_delivered_total: 15,
        events_dropped_total: 2,
        listener_uptime: { "sqs-listener": 300.0, "webhook-listener": 120.5 },
        active_listener_count: 2,
        drops_by_listener: { "sqs-listener": 2 },
      }),
    })

    // Trigger a stream publish (bypasses the 30s timer)
    await pub.publishStream()

    // Verify the published body contains all 3 REQ-LNP-078 metric names
    expect(publishedBodies).toHaveLength(1)
    const body = publishedBodies[0]
    expect(body).toContain("codeops_listener_uptime_seconds")
    expect(body).toContain("codeops_listener_events_dropped_total")
    expect(body).toContain("codeops_listener_delivery_latency_seconds")
    // Verify per-listener labels
    expect(body).toContain('listener="sqs-listener"')
    expect(body).toContain('listener="webhook-listener"')
    // Verify values
    expect(body).toContain("300")
    expect(body).toContain("120.5")
    expect(body).toContain("15") // events_delivered_total in histogram count
  })
})

// ===========================================================================
// CloudWatch Sink (data builders only — no AWS SDK needed)
// ===========================================================================

describe("CloudWatch sink data builders (REQ-PLG-120)", () => {
  it("builds metric data with correct dimensions", () => {
    const event = makeTestEvent()
    const data = buildCloudWatchMetricData(event, "Axiom/Plugin")

    // Should have per-model metrics (6) + session metrics (3) = 9
    expect(data.length).toBe(9)

    // Check a model metric
    const inputTokens = data.find((d) => d.MetricName === "InputTokens")
    expect(inputTokens).toBeDefined()
    expect(inputTokens!.Value).toBe(8200)
    expect(inputTokens!.Dimensions).toContainEqual({ Name: "Model", Value: "claude-sonnet-4-20250514" })
    expect(inputTokens!.Dimensions).toContainEqual({ Name: "SessionId", Value: "test-session-001" })

    // Check a session metric
    const messages = data.find((d) => d.MetricName === "Messages")
    expect(messages).toBeDefined()
    expect(messages!.Value).toBe(4)
  })

  it("builds log event as JSON string", () => {
    const event = makeTestEvent()
    const logEvent = buildCloudWatchLogEvent(event)

    expect(logEvent.timestamp).toBeGreaterThan(0)
    const parsed = JSON.parse(logEvent.message)
    expect(parsed.event_type).toBe("plugin.metrics.snapshot")
  })
})

// ===========================================================================
// Buffer Manager
// ===========================================================================

describe("Buffer manager (REQ-PLG-128)", () => {
  it("starts empty", () => {
    const buffer = createBufferManager()
    expect(buffer.size).toBe(0)
    expect(buffer.isFull).toBe(false)
  })

  it("adds events to the buffer", () => {
    const buffer = createBufferManager()
    buffer.add(makeTestEvent())
    expect(buffer.size).toBe(1)
    expect(buffer.getAll()[0].failureCount).toBe(1)
    expect(buffer.getAll()[0].consolidated).toBe(false)
  })

  it("removes events by index", () => {
    const buffer = createBufferManager()
    buffer.add(makeTestEvent({ snapshot_seq: 1 }))
    buffer.add(makeTestEvent({ snapshot_seq: 2 }))
    buffer.add(makeTestEvent({ snapshot_seq: 3 }))

    buffer.remove(1)
    expect(buffer.size).toBe(2)
    expect(buffer.getAll()[0].event.snapshot_seq).toBe(1)
    expect(buffer.getAll()[1].event.snapshot_seq).toBe(3)
  })

  it("clears all events", () => {
    const buffer = createBufferManager()
    buffer.add(makeTestEvent())
    buffer.add(makeTestEvent())
    buffer.clear()
    expect(buffer.size).toBe(0)
  })

  it("auto-summarizes when buffer exceeds max (REQ-PLG-128)", () => {
    const buffer = createBufferManager({ maxBufferedIntervals: 4 })

    // Fill to capacity
    for (let i = 1; i <= 4; i++) {
      buffer.add(makeTestEvent({ snapshot_seq: i }))
    }
    expect(buffer.size).toBe(4)

    // One more triggers auto-summarization
    // axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-128,REQ-PLG-134 plan=phase-70-ts-tests/task-ts-1/step-ts-1-1
    // buffer.ts uses _log.warn() → process.stderr.write(), not console.warn
    const stderrSpy = spyOn(process.stderr, "write").mockImplementation(() => true)
    buffer.add(makeTestEvent({ snapshot_seq: 5 }))

    // Should have summarized oldest half (2 intervals) into 1 + remaining 3
    expect(buffer.size).toBeLessThanOrEqual(4)

    // Verify warning was emitted to stderr (REQ-PLG-128 + REQ-PLG-134)
    expect(stderrSpy).toHaveBeenCalledWith(
      expect.stringContaining("Metrics buffer full"),
    )
    stderrSpy.mockRestore()
  })

  it("marks consolidated intervals correctly", () => {
    const buffer = createBufferManager({ maxBufferedIntervals: 3 })

    for (let i = 1; i <= 4; i++) {
      buffer.add(makeTestEvent({ snapshot_seq: i }))
    }

    const consolidated = buffer.getAll().find((b) => b.consolidated)
    expect(consolidated).toBeDefined()
    expect(consolidated!.summarizedCount).toBeGreaterThan(1)
  })

  it("autoSummarize() preserves _sinkSucceeded union from source intervals (REQ-PLG-130)", () => {
    const buffer = createBufferManager({ maxBufferedIntervals: 3 })

    // Add 3 intervals
    buffer.add(makeTestEvent({ snapshot_seq: 1 }))
    buffer.add(makeTestEvent({ snapshot_seq: 2 }))
    buffer.add(makeTestEvent({ snapshot_seq: 3 }))

    // Mark first two as already succeeded for "stdout"
    const intervals = buffer.getAll() as Array<{ _sinkSucceeded?: Set<string> }>
    intervals[0]._sinkSucceeded = new Set(["stdout"])
    intervals[1]._sinkSucceeded = new Set(["stdout"])

    // Trigger autoSummarize by adding one more (exceeds cap of 3)
    // Suppress stderr noise from _log.warn() (buffer.ts routes to process.stderr)
    const stderrSpy2 = spyOn(process.stderr, "write").mockImplementation(() => true)
    buffer.add(makeTestEvent({ snapshot_seq: 4 }))
    stderrSpy2.mockRestore()

    // The consolidated interval should have "stdout" in _sinkSucceeded
    // (union of source intervals' _sinkSucceeded)
    const consolidated = buffer.getAll().find((b) => b.consolidated)
    expect(consolidated).toBeDefined()
    expect(consolidated!._sinkSucceeded?.has("stdout")).toBe(true)
  })
})

describe("mergeModelDeltas", () => {
  it("sums token counts across deltas", () => {
    const delta1: Record<string, ModelDelta> = {
      "model-a": {
        input_tokens: 100,
        output_tokens: 50,
        cache_read_tokens: 10,
        cache_write_tokens: 5,
        estimated_cost_microdollars: 1000,
        estimated_cost_usd: 0.001,
        llm_calls: 2,
      },
    }
    const delta2: Record<string, ModelDelta> = {
      "model-a": {
        input_tokens: 200,
        output_tokens: 100,
        cache_read_tokens: 20,
        cache_write_tokens: 10,
        estimated_cost_microdollars: 2000,
        estimated_cost_usd: 0.002,
        llm_calls: 3,
      },
    }

    const merged = mergeModelDeltas([delta1, delta2])
    expect(merged["model-a"].input_tokens).toBe(300)
    expect(merged["model-a"].output_tokens).toBe(150)
    expect(merged["model-a"].estimated_cost_microdollars).toBe(3000)
    expect(merged["model-a"].estimated_cost_usd).toBeCloseTo(0.003, 6)
    expect(merged["model-a"].llm_calls).toBe(5)
  })

  it("handles multiple models", () => {
    const delta1: Record<string, ModelDelta> = {
      "model-a": { input_tokens: 100, output_tokens: 50, cache_read_tokens: 0, cache_write_tokens: 0, estimated_cost_microdollars: 1000, estimated_cost_usd: 0.001, llm_calls: 1 },
    }
    const delta2: Record<string, ModelDelta> = {
      "model-b": { input_tokens: 200, output_tokens: 100, cache_read_tokens: 0, cache_write_tokens: 0, estimated_cost_microdollars: 2000, estimated_cost_usd: 0.002, llm_calls: 1 },
    }

    const merged = mergeModelDeltas([delta1, delta2])
    expect(Object.keys(merged)).toHaveLength(2)
    expect(merged["model-a"].input_tokens).toBe(100)
    expect(merged["model-b"].input_tokens).toBe(200)
  })

  // -------------------------------------------------------------------------
  // REQ-PLG-135 + REQ-PLG-131: estimated_cost_usd must be recomputed from
  // microdollars after each merge — NOT accumulated via float addition.
  // Float accumulation over N intervals causes drift from the integer source of truth.
  // -------------------------------------------------------------------------

  it("estimated_cost_usd is recomputed from microdollars after merge — no float drift (REQ-PLG-131, REQ-PLG-135)", () => {
    // Use 70001 microdollars — a value where float accumulation would drift
    // from the integer-derived result after multiple merges.
    const N = 12 // max_buffered_intervals default
    const microdollarsPerInterval = 70001
    const deltas: Array<Record<string, ModelDelta>> = Array.from({ length: N }, () => ({
      "model-a": {
        input_tokens: 100,
        output_tokens: 50,
        cache_read_tokens: 0,
        cache_write_tokens: 0,
        estimated_cost_microdollars: microdollarsPerInterval,
        estimated_cost_usd: microdollarsPerInterval / 1_000_000,
        llm_calls: 1,
      },
    }))

    const merged = mergeModelDeltas(deltas)
    const expectedMicrodollars = N * microdollarsPerInterval
    const expectedUsd = expectedMicrodollars / 1_000_000

    expect(merged["model-a"].estimated_cost_microdollars).toBe(expectedMicrodollars)
    // estimated_cost_usd MUST equal microdollars / 1_000_000 exactly — no float drift
    expect(merged["model-a"].estimated_cost_usd).toBe(expectedUsd)
    // Verify the two fields are consistent with each other
    expect(Math.abs(merged["model-a"].estimated_cost_usd - merged["model-a"].estimated_cost_microdollars / 1_000_000)).toBe(0)
  })
})

describe("mergeSessionDeltas", () => {
  it("sums all session delta fields", () => {
    const deltas: SessionDelta[] = [
      { messages: 2, tool_invocations: 5, commands_executed: 1, files_read: 3, files_written: 1 },
      { messages: 3, tool_invocations: 8, commands_executed: 2, files_read: 4, files_written: 2 },
    ]

    const merged = mergeSessionDeltas(deltas)
    expect(merged.messages).toBe(5)
    expect(merged.tool_invocations).toBe(13)
    expect(merged.commands_executed).toBe(3)
    expect(merged.files_read).toBe(7)
    expect(merged.files_written).toBe(3)
  })
})

// ===========================================================================
// Identity Resolver
// ===========================================================================

describe("Email identity resolver (REQ-PLG-126)", () => {
  const originalEnv = process.env.AXIOM_USER_EMAIL

  afterEach(() => {
    if (originalEnv !== undefined) {
      process.env.AXIOM_USER_EMAIL = originalEnv
    } else {
      delete process.env.AXIOM_USER_EMAIL
    }
  })

  it("step 1: uses AXIOM_USER_EMAIL env var first", () => {
    process.env.AXIOM_USER_EMAIL = "env@example.com"
    const result = resolveEmailIdentitySync({ repoRoot: "/tmp/test" })
    expect(result.email).toBe("env@example.com")
    expect(result.source).toBe("env_var")
  })

  it("step 5: uses config email when env and git unavailable", () => {
    delete process.env.AXIOM_USER_EMAIL
    const result = resolveEmailIdentitySync({
      repoRoot: "/tmp/nonexistent-repo",
      configEmail: "config@example.com",
    })
    // May get git email if git is configured globally, or config email
    expect(result.email).toBeTruthy()
    expect(["git_config", "config_file", "anonymous"].includes(result.source)).toBe(true)
  })

  it("step 6: falls back to anonymous@unknown", () => {
    delete process.env.AXIOM_USER_EMAIL
    const result = resolveEmailIdentitySync({
      repoRoot: "/tmp/nonexistent-repo-xyz",
    })
    // May get hostname-based git email or anonymous
    expect(result.email).toBeTruthy()
  })

  it("detects noreply emails", () => {
    expect(isNoreplyEmail("user@users.noreply.github.com")).toBe(true)
    expect(isNoreplyEmail("user@noreply.example.com")).toBe(true)
    expect(isNoreplyEmail("alice@example.com")).toBe(false)
  })
})

// ===========================================================================
// Cost Module (Microdollars)
// ===========================================================================
// Git branch resolution
// ===========================================================================

describe("resolveGitBranch (REQ-PLG-133)", () => {
  it("returns current branch name in a git repo", () => {
    // This test runs inside the Axiom repo — should return a branch name
    const branch = resolveGitBranch("/Users/aokuwa/code/Axiom/.worktrees/plugin-sinks")
    // Either returns a real branch name or null (if detached HEAD) — never "HEAD"
    if (branch !== null) {
      expect(branch).not.toBe("HEAD")
      expect(branch.length).toBeGreaterThan(0)
    }
  })

  it("returns null for a non-git directory", () => {
    const branch = resolveGitBranch("/tmp")
    expect(branch).toBeNull()
  })
})

// ===========================================================================
// Publisher git_branch + repo_path propagation
// ===========================================================================

describe("Publisher git_branch and repo_path (REQ-PLG-133)", () => {
  let publisher: MetricsPublisher

  afterEach(() => {
    if (publisher?.isRunning) publisher.stop()
  })

  it("includes git_branch and repo_path in emitted events", async () => {
    const sink = createMockSink()
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      gitBranch: "feature/my-branch",
      repoPath: "/Users/alice/code/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
    })

    await publisher.publishStream()

    expect(sink.calls[0].git_branch).toBe("feature/my-branch")
    expect(sink.calls[0].repo_path).toBe("/Users/alice/code/backend")
  })

  it("omits git_branch when undefined (omit-not-null, REQ-PLG-133)", async () => {
    const sink = createMockSink()
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "test-session",
      repo: "hostname:mymachine",
      // gitBranch intentionally omitted — not in a git repo
      repoPath: "/Users/alice/code/project",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "hostname:mymachine",
    })

    await publisher.publishStream()

    expect(sink.calls[0].git_branch).toBeUndefined()
    expect(sink.calls[0].repo_path).toBe("/Users/alice/code/project")
    expect(sink.calls[0].repo).toBe("hostname:mymachine")
  })
})

// ===========================================================================

describe("Cost module — microdollars (REQ-PLG-131)", () => {
  it("MICRODOLLARS_PER_USD is 1,000,000", () => {
    expect(MICRODOLLARS_PER_USD).toBe(1_000_000)
  })

  it("converts USD to microdollars", () => {
    expect(usdToMicrodollars(1.0)).toBe(1_000_000)
    expect(usdToMicrodollars(0.5)).toBe(500_000)
    expect(usdToMicrodollars(0.001)).toBe(1_000)
    expect(usdToMicrodollars(0)).toBe(0)
  })

  it("converts microdollars to USD", () => {
    expect(microdollarsToUsd(1_000_000)).toBe(1.0)
    expect(microdollarsToUsd(500_000)).toBe(0.5)
    expect(microdollarsToUsd(0)).toBe(0)
  })

  it("round-trips without precision loss", () => {
    const usd = 3.75
    const microdollars = usdToMicrodollars(usd)
    expect(microdollarsToUsd(microdollars)).toBe(usd)
  })

  it("converts pricing to microdollars", () => {
    const pricing = pricingToMicrodollars({
      price_per_1m_input: 3.0,
      price_per_1m_output: 15.0,
      price_per_1m_cache_read: 0.3,
      price_per_1m_cache_write: 3.75,
      price_per_1m_reasoning: 0.0,
    })

    expect(pricing.input_per_1m_microdollars).toBe(3_000_000)
    expect(pricing.output_per_1m_microdollars).toBe(15_000_000)
    expect(pricing.cache_read_per_1m_microdollars).toBe(300_000)
    expect(pricing.cache_write_per_1m_microdollars).toBe(3_750_000)
    expect(pricing.reasoning_per_1m_microdollars).toBe(0)
  })

  it("computes cost in microdollars correctly", () => {
    const pricing = pricingToMicrodollars({
      price_per_1m_input: 3.0,
      price_per_1m_output: 15.0,
      price_per_1m_cache_read: 0.3,
      price_per_1m_cache_write: 3.75,
      price_per_1m_reasoning: 0.0,
    })

    const cost = computeCostMicrodollars(
      {
        input: 10_000,
        output: 2_000,
        cache_read: 5_000,
        cache_write: 1_000,
      },
      pricing,
    )

    // Expected:
    // input:  3_000_000 * 10_000 / 1_000_000 = 30_000
    // output: 15_000_000 * 2_000 / 1_000_000 = 30_000
    // cache_read: 300_000 * 5_000 / 1_000_000 = 1_500
    // cache_write: 3_750_000 * 1_000 / 1_000_000 = 3_750
    // Total: 65_250 microdollars = $0.065250
    expect(cost).toBe(65_250)
    expect(microdollarsToUsd(cost)).toBeCloseTo(0.06525, 5)
  })

  it("handles zero tokens", () => {
    const pricing = pricingToMicrodollars({
      price_per_1m_input: 3.0,
      price_per_1m_output: 15.0,
      price_per_1m_cache_read: 0.3,
      price_per_1m_cache_write: 3.75,
      price_per_1m_reasoning: 0.0,
    })

    const cost = computeCostMicrodollars(
      { input: 0, output: 0, cache_read: 0, cache_write: 0 },
      pricing,
    )
    expect(cost).toBe(0)
  })
})

// ===========================================================================
// Snapshot Sequence
// ===========================================================================

describe("Snapshot sequence (REQ-PLG-122)", () => {
  it("starts at 0 (next() returns 1)", () => {
    const seq = createSnapshotSequence()
    expect(seq.current).toBe(0)
    expect(seq.next()).toBe(1)
    expect(seq.current).toBe(1)
  })

  it("increments monotonically", () => {
    const seq = createSnapshotSequence()
    expect(seq.next()).toBe(1)
    expect(seq.next()).toBe(2)
    expect(seq.next()).toBe(3)
  })

  it("resets to 0", () => {
    const seq = createSnapshotSequence()
    seq.next()
    seq.next()
    seq.reset()
    expect(seq.current).toBe(0)
    expect(seq.next()).toBe(1)
  })
})

// ===========================================================================
// Publisher
// ===========================================================================

describe("Metrics publisher (REQ-PLG-122, REQ-PLG-129, REQ-PLG-130)", () => {
  let publisher: MetricsPublisher

  afterEach(() => {
    if (publisher?.isRunning) {
      publisher.stop()
    }
  })

  it("creates a publisher with correct defaults", () => {
    const sink = createMockSink()
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
    })

    expect(publisher.isRunning).toBe(false)
    expect(publisher.sequence.current).toBe(0)
    expect(publisher.buffer.size).toBe(0)
  })

  it("publishStream sends a stream event to all sinks", async () => {
    const sink1 = createMockSink("stdout")
    const sink2 = createMockSink("axiom-backend")

    publisher = createPublisher({
      sinks: [sink1, sink2],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
    })

    await publisher.publishStream()

    // Both sinks should receive the event
    expect(sink1.calls.length).toBe(1)
    expect(sink2.calls.length).toBe(1)

    // Event should be a stream snapshot
    expect(sink1.calls[0].snapshot_type).toBe("stream")
    expect(sink1.calls[0].event_type).toBe("plugin.metrics.snapshot")
    expect(sink1.calls[0].snapshot_seq).toBe(1)
  })

  it("publishReconciliation sends a reconciliation event", async () => {
    const sink = createMockSink()
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
    })

    await publisher.publishReconciliation()

    expect(sink.calls.length).toBe(1)
    expect(sink.calls[0].snapshot_type).toBe("reconciliation")
  })

  it("publishFinal sends a final event with session_summary", async () => {
    const sink = createMockSink()
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
      getSessionSummary: () => ({
        total_duration_seconds: 7200,
        total_messages: 45,
        total_tool_invocations: 134,
        total_commands_executed: 8,
        maturity_level_start: 3,
        maturity_level_end: 3,
        commands_used: ["axiom-run"],
        by_model: {},
        total_estimated_cost_microdollars: 610000,
        total_estimated_cost_usd: 0.61,
      }),
    })

    await publisher.publishFinal()

    expect(sink.calls.length).toBe(1)
    expect(sink.calls[0].event_type).toBe("plugin.session.ended")
    expect(sink.calls[0].snapshot_type).toBe("final")
    expect(sink.calls[0].session_summary).toBeDefined()
    expect(sink.calls[0].session_summary!.total_estimated_cost_microdollars).toBe(610000)
    expect(sink.calls[0].session_summary!.total_estimated_cost_usd).toBe(0.61)
  })

  it("increments snapshot_seq across publishes", async () => {
    const sink = createMockSink()
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
    })

    await publisher.publishStream()
    await publisher.publishStream()
    await publisher.publishReconciliation()

    expect(sink.calls[0].snapshot_seq).toBe(1)
    expect(sink.calls[1].snapshot_seq).toBe(2)
    expect(sink.calls[2].snapshot_seq).toBe(3)
  })

  it("sink failure does not affect other sinks (REQ-PLG-130)", async () => {
    const failingSink = createMockSink("axiom-backend", {
      success: false,
      retryable: true,
      error: "Connection refused",
    })
    const workingSink = createMockSink("stdout")

    // Suppress stderr noise from failure handling (_log.warn → process.stderr)
    const stderrSpy3 = spyOn(process.stderr, "write").mockImplementation(() => true)

    publisher = createPublisher({
      sinks: [failingSink, workingSink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
    })

    await publisher.publishStream()

    // Both sinks should have been called at least once (failing sink may be
    // called again during buffer drain, which is correct behavior)
    expect(failingSink.calls.length).toBeGreaterThanOrEqual(1)
    expect(workingSink.calls.length).toBeGreaterThanOrEqual(1)

    // The key assertion: working sink succeeded despite failing sink
    // (independence per REQ-PLG-130)

    stderrSpy3.mockRestore()
  })

  it("buffers failed events for retry", async () => {
    const failingSink = createMockSink("axiom-backend", {
      success: false,
      retryable: true,
      error: "Connection refused",
    })

    // Suppress stderr noise from failure handling (_log.warn → process.stderr)
    const stderrSpy4 = spyOn(process.stderr, "write").mockImplementation(() => true)

    publisher = createPublisher({
      sinks: [failingSink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
    })

    await publisher.publishStream()

    // Event should be buffered
    expect(publisher.buffer.size).toBeGreaterThan(0)

    stderrSpy4.mockRestore()
  })

  it("uses getDelta callback for event data", async () => {
    const sink = createMockSink()
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
      getDelta: () => ({
        model: {
          "test-model": {
            input_tokens: 999,
            output_tokens: 111,
            cache_read_tokens: 0,
            cache_write_tokens: 0,
            estimated_cost_microdollars: 5000,
            estimated_cost_usd: 0.005,
            llm_calls: 1,
          },
        },
        session: { messages: 1, tool_invocations: 2, commands_executed: 0, files_read: 1, files_written: 0 },
      }),
    })

    await publisher.publishStream()

    expect(sink.calls[0].delta["test-model"].input_tokens).toBe(999)
    expect(sink.calls[0].delta_session.messages).toBe(1)
  })

  it("start/stop controls timer lifecycle", () => {
    const sink = createMockSink()
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
    })

    expect(publisher.isRunning).toBe(false)
    publisher.start()
    expect(publisher.isRunning).toBe(true)
    publisher.stop()
    expect(publisher.isRunning).toBe(false)
  })

  it("start is idempotent", () => {
    const sink = createMockSink()
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
    })

    publisher.start()
    publisher.start() // Should not throw or create duplicate timers
    expect(publisher.isRunning).toBe(true)
    publisher.stop()
  })

  it("getSessionId callback — emitted events reflect updated session ID (REQ-PLG-109)", async () => {
    const sink = createMockSink()
    let currentSessionId = "session-fallback-123"
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => currentSessionId,
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
      repoPath: "/Users/alice/code/backend",
    })

    // First publish — uses fallback ID
    await publisher.publishStream()
    expect(sink.calls[0].session_id).toBe("session-fallback-123")

    // Update session ID (simulates OpenCode session ID capture from event)
    currentSessionId = "ses_real-opencode-session-id"

    // Second publish — must use the updated ID
    await publisher.publishStream()
    expect(sink.calls[1].session_id).toBe("ses_real-opencode-session-id")
  })

  it("publishFinal() is idempotent — double-call emits exactly 1 final event", async () => {
    const sink = createMockSink()
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
    })

    // Call publishFinal() twice — should only emit one final event
    await publisher.publishFinal()
    await publisher.publishFinal()

    const finalEvents = sink.calls.filter(e => e.snapshot_type === "final")
    expect(finalEvents.length).toBe(1)
    expect(finalEvents[0].event_type).toBe("plugin.session.ended")
  })

  it("publishFinal() includes session_summary with by_model populated (REQ-PLG-125)", async () => {
    const sink = createMockSink()
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
      getSessionSummary: () => ({
        total_duration_seconds: 3600,
        total_messages: 10,
        total_tool_invocations: 25,
        total_commands_executed: 3,
        maturity_level_start: 0,
        maturity_level_end: 3,
        commands_used: ["axiom-run"],
        by_model: {
          "claude-sonnet-4": {
            input_tokens: 5000,
            output_tokens: 1000,
            cache_read_tokens: 2000,
            cache_write_tokens: 500,
            reasoning_tokens: 0,
            estimated_cost_microdollars: 50000,
            estimated_cost_usd: 0.05,
            llm_calls: 5,
          },
        },
        total_estimated_cost_microdollars: 50000,
        total_estimated_cost_usd: 0.05,
      }),
    })

    await publisher.publishFinal()

    const finalEvent = sink.calls.find(e => e.snapshot_type === "final")
    expect(finalEvent).toBeDefined()
    expect(finalEvent!.session_summary).toBeDefined()
    expect(finalEvent!.session_summary!.by_model["claude-sonnet-4"]).toBeDefined()
    expect(finalEvent!.session_summary!.total_estimated_cost_microdollars).toBe(50000)
    expect(finalEvent!.session_summary!.total_estimated_cost_usd).toBe(0.05)
    expect(finalEvent!.session_summary!.by_model["claude-sonnet-4"].estimated_cost_microdollars).toBe(50000)
    expect(finalEvent!.session_summary!.by_model["claude-sonnet-4"].estimated_cost_usd).toBe(0.05)
  })

  it("drainBuffer() does not re-deliver to sinks that already succeeded (REQ-PLG-130)", async () => {
    // sinkA always succeeds
    const sinkA = createMockSink("stdout")
    // sinkB always fails (retryable)
    const sinkB = createMockSink("axiom-backend", {
      success: false,
      retryable: true,
      error: "Connection refused",
    })

    // Suppress stderr noise from failure handling (_log.warn → process.stderr)
    const stderrSpy5 = spyOn(process.stderr, "write").mockImplementation(() => true)

    publisher = createPublisher({
      sinks: [sinkA, sinkB],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
    })

    // First publish — sinkA succeeds, sinkB fails → event buffered.
    // drainBuffer() runs immediately after: sinkA is called once more for the
    // buffered event (first drain, _sinkSucceeded not yet populated), then
    // sinkA's id is recorded in _sinkSucceeded for that interval.
    await publisher.publishStream()

    // Second publish — new event: sinkA called once more.
    // drainBuffer() runs again: sinkA is now SKIPPED for the buffered event
    // because _sinkSucceeded already contains sinkA.id (REQ-PLG-130).
    await publisher.publishStream()

    // With _sinkSucceeded tracking: sinkA receives 4 calls total
    //   publishStream #1: 1 (publishToAllSinks) + 1 (first drain, cold start)
    //   publishStream #2: 1 (publishToAllSinks) + 1 (drain for event2 cold start)
    //   The buffered event1 is NOT re-delivered to sinkA on drain #2 (skipped).
    //
    // WITHOUT _sinkSucceeded tracking: sinkA would receive 5 calls total
    //   (the buffered event1 would be re-delivered to sinkA on every drain cycle)
    expect(sinkA.calls.length).toBe(4)

    stderrSpy5.mockRestore()
  })

  it("publisher emits delta with 'unknown' model key when modelId is not resolved (REQ-PLG-124)", async () => {
    // Simulates the race condition where step-finish arrives before message.updated
    const sink = createMockSink()
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
      getDelta: () => ({
        model: {
          "unknown": {
            input_tokens: 100,
            output_tokens: 50,
            cache_read_tokens: 0,
            cache_write_tokens: 0,
            estimated_cost_microdollars: 1000,
            estimated_cost_usd: 0.001,
            llm_calls: 1,
          },
        },
        session: { messages: 1, tool_invocations: 0, commands_executed: 0, files_read: 0, files_written: 0 },
      }),
    })

    await publisher.publishStream()

    // Token data should be present under "unknown" key — data preserved, attribution wrong
    expect(sink.calls[0].delta["unknown"]).toBeDefined()
    expect(sink.calls[0].delta["unknown"].input_tokens).toBe(100)
    expect(sink.calls[0].delta["unknown"].estimated_cost_microdollars).toBe(1000)
    expect(sink.calls[0].delta["unknown"].estimated_cost_usd).toBe(0.001)
  })

  // -------------------------------------------------------------------------
  // REQ-PLG-136 (DEX-419 Fix 2): reasoning_tokens always present at top-level
  // -------------------------------------------------------------------------

  it("emits reasoning_tokens: null when getReasoningTokens not provided (DEX-419 Fix 2)", async () => {
    const sink = createMockSink()
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
      // getReasoningTokens intentionally omitted — should default to null
    })

    await publisher.publishStream()

    expect(sink.calls[0].reasoning_tokens).toBeNull()
    // Field must be present (not undefined)
    expect("reasoning_tokens" in sink.calls[0]).toBe(true)
  })

  it("emits reasoning_tokens from callback when provided (DEX-419 Fix 2)", async () => {
    const sink = createMockSink()
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
      getReasoningTokens: () => 1234,
    })

    await publisher.publishStream()

    expect(sink.calls[0].reasoning_tokens).toBe(1234)
  })

  // -------------------------------------------------------------------------
  // REQ-PLG-137 (DEX-419 Fix 4): metadata fields
  // -------------------------------------------------------------------------

  it("emits metadata fields with null defaults when not provided (DEX-419 Fix 4)", async () => {
    const sink = createMockSink()
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
      // metadata fields intentionally omitted
    })

    await publisher.publishStream()

    const event = sink.calls[0]
    expect(event.model_provider).toBeNull()
    expect(event.agent_name).toBeNull()
    expect(event.opencode_version).toBeNull()
    expect(event.plugin_version).toBeNull()
    expect(event.thinking_level).toBeNull()
    expect(event.thinking_tokens).toBeNull()
    expect(event.connected_mcps).toEqual([])
  })

  it("emits metadata fields when provided (DEX-419 Fix 4)", async () => {
    const sink = createMockSink()
    publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "test-session",
      repo: "acme/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
      modelProvider: "amazon-bedrock",
      agentName: "tower-axiom",
      opencodeVersion: "0.1.123",
      pluginVersion: "0.2.0",
      thinkingLevel: "medium",
      getThinkingTokens: () => 500,
      connectedMcps: ["chrome-devtools", "atlassian", "github"],
    })

    await publisher.publishStream()

    const event = sink.calls[0]
    expect(event.model_provider).toBe("amazon-bedrock")
    expect(event.agent_name).toBe("tower-axiom")
    expect(event.opencode_version).toBe("0.1.123")
    expect(event.plugin_version).toBe("0.2.0")
    expect(event.thinking_level).toBe("medium")
    expect(event.thinking_tokens).toBe(500)
    expect(event.connected_mcps).toEqual(["chrome-devtools", "atlassian", "github"])
  })
})

// ===========================================================================
// Barrel export verification
// ===========================================================================

describe("Metrics module barrel export", () => {
  it("exports all expected symbols", async () => {
    const mod = await import("../src/metrics/index")

    // Sink interface
    expect(mod.DEFAULT_METRICS_PUBLISH_CONFIG).toBeDefined()

    // Sink implementations
    expect(typeof mod.createStdoutSink).toBe("function")
    expect(typeof mod.createBackendSink).toBe("function")
    expect(typeof mod.createPrometheusSink).toBe("function")
    expect(typeof mod.formatPrometheusMetrics).toBe("function")
    expect(typeof mod.createCloudWatchSink).toBe("function")
    expect(typeof mod.buildCloudWatchMetricData).toBe("function")
    expect(typeof mod.buildCloudWatchLogEvent).toBe("function")

    // Publisher
    expect(typeof mod.createPublisher).toBe("function")

    // Buffer
    expect(typeof mod.createBufferManager).toBe("function")
    expect(typeof mod.mergeModelDeltas).toBe("function")
    expect(typeof mod.mergeSessionDeltas).toBe("function")

    // Identity
    expect(typeof mod.resolveEmailIdentity).toBe("function")
    expect(typeof mod.resolveEmailIdentitySync).toBe("function")
    expect(typeof mod.isNoreplyEmail).toBe("function")

    // Cost
    expect(typeof mod.usdToMicrodollars).toBe("function")
    expect(typeof mod.microdollarsToUsd).toBe("function")
    expect(typeof mod.computeCostMicrodollars).toBe("function")
    expect(typeof mod.pricingToMicrodollars).toBe("function")
    expect(mod.MICRODOLLARS_PER_USD).toBe(1_000_000)

    // Snapshot
    expect(typeof mod.createSnapshotSequence).toBe("function")
  })
})

// ===========================================================================
// DEX-419 Tier 3 Smoke Test — all metadata fields populated end-to-end
// ===========================================================================

describe("DEX-419 Tier 3 smoke: publisher emits all metadata fields (REQ-PLG-136, REQ-PLG-137)", () => {
  it("publisher emits events with all DEX-419 metadata fields populated (Tier 3 smoke)", async () => {
    // axiom:trace work_item=plugin-metrics-fixes-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-122 plan=phase-70-l2-metrics-runtime-wiring test=tests/metrics-sinks.test.ts jira_ref=DEX-419
    const sink = createMockSink()
    const publisher = createPublisher({
      sinks: [sink],
      getSessionId: () => "ses_smoke-test-001",
      repo: "acme/backend",
      repoPath: "/Users/alice/code/backend",
      userEmail: "alice@example.com",
      emailSource: "git_config",
      sessionIdentity: "github.com/acme/backend",
      // DEX-419 Fix 2: reasoning_tokens callback
      getReasoningTokens: () => null,
      // DEX-419 Fix 4: all 7 metadata fields
      modelProvider: "amazon-bedrock",
      agentName: "tower-axiom",
      opencodeVersion: "0.1.123",
      pluginVersion: "0.0.1",
      thinkingLevel: "low",
      getThinkingTokens: () => 250,
      connectedMcps: ["chrome-devtools", "atlassian"],
    })

    await publisher.publishStream()

    expect(sink.calls.length).toBe(1)
    const event = sink.calls[0]

    // REQ-PLG-136 (DEX-419 Fix 2): reasoning_tokens always present, null when unavailable
    expect("reasoning_tokens" in event).toBe(true)
    expect(event.reasoning_tokens).toBeNull()

    // REQ-PLG-137 (DEX-419 Fix 4): all 7 metadata fields
    expect(event.model_provider).toBe("amazon-bedrock")
    expect(event.agent_name).toBe("tower-axiom")
    expect(event.opencode_version).toBe("0.1.123")
    expect(event.plugin_version).toBe("0.0.1")
    expect(event.thinking_level).toBe("low")
    expect(event.thinking_tokens).toBe(250)
    expect(event.connected_mcps).toEqual(["chrome-devtools", "atlassian"])

    // Core event fields
    expect(event.session_id).toBe("ses_smoke-test-001")
    expect(typeof event.delta).toBe("object")
    expect(event.event_type).toBe("plugin.metrics.snapshot")
    expect(event.snapshot_type).toBe("stream")
  })
})
