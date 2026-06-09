/**
 * TUI Dashboard — performance SLO tests.
 *
 * Verifies that the dashboard meets its latency and resource SLOs using
 * deterministic mock sources (no filesystem or network I/O).
 *
 * SLOs under test:
 * - REQ-TUI-050: Dashboard open latency <1s (collect + first render).
 * - REQ-TUI-051: Refresh cycle latency <200ms (data collection + rendering for one frame).
 * - REQ-TUI-052: Memory overhead <5MB (dashboard state including session list).
 * - REQ-TUI-054: File read latency <10ms per file (mocked — verifies no overhead in collector).
 * - REQ-TUI-056: Zero CPU when not visible — renderDashboard() is a pure function with no timers.
 *
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-050
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-050 plan=phase-6/task-6.1 test=dashboard-performance.test.ts
 */

import { describe, it, expect } from "bun:test"
import { renderDashboard, renderDashboardJson } from "../src/dashboard/renderer"
import { SessionDataCollector } from "../src/dashboard/collector"
import type {
  DashboardData,
  SessionRow,
  DashboardAggregates,
  SessionDataSource,
} from "../src/dashboard/types"

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

/** Build a minimal valid SessionRow for performance testing. */
function makePerfRow(overrides: Partial<SessionRow> = {}): SessionRow {
  return {
    sessionId: `perf-sess-${Math.random().toString(36).slice(2, 8)}`,
    title: "Perf test session",
    workItemId: "PERF-001",
    model: "Sonnet 4",
    modelRaw: "amazon-bedrock/anthropic.claude-sonnet-4-6",
    costUsd: 0.42,
    totalTokens: 35_000,
    durationSeconds: 600,
    status: "running",
    source: "in-memory",
    updatedAt: new Date(),
    isTeam: false,
    ...overrides,
  }
}

/** Build N mock SessionRows with unique session IDs. */
function makeMockSessions(n: number): SessionRow[] {
  return Array.from({ length: n }, (_, i) =>
    makePerfRow({
      sessionId: `perf-sess-${String(i).padStart(4, "0")}`,
      title: `Session ${i}`,
      workItemId: `WI-${i}`,
      costUsd: 0.1 + i * 0.01,
      totalTokens: 1000 + i * 100,
      durationSeconds: 60 + i * 10,
    })
  )
}

/** Build a DashboardAggregates object for N sessions. */
function makePerfAggregates(n: number): DashboardAggregates {
  return {
    totalSessions: n,
    totalCostUsd: n * 0.42,
    totalTokens: n * 35_000,
    avgSpeedTokensPerSec: 58,
  }
}

/** Build a full DashboardData payload for N sessions. */
function makeMockDashboardData(n: number): DashboardData {
  const sessions = makeMockSessions(n)
  return {
    sessions,
    aggregates: makePerfAggregates(n),
    sources: {
      inMemory: { available: true, lastUpdate: new Date() },
      file: { available: false, lastUpdate: null },
      network: { available: false, lastUpdate: null },
    },
    generatedAt: new Date(),
  }
}

/** A mock SessionDataSource that returns N sessions synchronously. */
function makeMockSource(n: number): SessionDataSource {
  const sessions = makeMockSessions(n)
  const lastUpdate = new Date()
  return {
    id: "in-memory",
    priority: 3,
    isAvailable: () => true,
    getLastUpdate: () => lastUpdate,
    getSessions: async () => sessions,
  }
}

// ---------------------------------------------------------------------------
// SLO: open latency <1s (REQ-TUI-050)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-050 plan=phase-6/task-6.1
// ---------------------------------------------------------------------------

describe("SLO: open latency <1s (REQ-TUI-050)", () => {
  it("collects and renders 10 sessions in <1000ms", async () => {
    const collector = new SessionDataCollector()
    const mockSource = makeMockSource(10)

    const start = performance.now()
    const data = await collector.collect({
      repoRoot: "/tmp/perf-test",
      sources: [mockSource],
    })
    const result = renderDashboard(data)
    const elapsed = performance.now() - start

    expect(result.output.length).toBeGreaterThan(0)
    expect(elapsed).toBeLessThan(1000)
  })

  it("collects and renders 20 sessions in <1000ms", async () => {
    const collector = new SessionDataCollector()
    const mockSource = makeMockSource(20)

    const start = performance.now()
    const data = await collector.collect({
      repoRoot: "/tmp/perf-test",
      sources: [mockSource],
    })
    const result = renderDashboard(data)
    const elapsed = performance.now() - start

    expect(result.output.length).toBeGreaterThan(0)
    expect(elapsed).toBeLessThan(1000)
  })

  it("collect() alone for 10 sessions completes in <500ms", async () => {
    const collector = new SessionDataCollector()
    const mockSource = makeMockSource(10)

    const start = performance.now()
    await collector.collect({
      repoRoot: "/tmp/perf-test",
      sources: [mockSource],
    })
    const elapsed = performance.now() - start

    expect(elapsed).toBeLessThan(500)
  })
})

// ---------------------------------------------------------------------------
// SLO: refresh cycle <200ms (REQ-TUI-051)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-051 plan=phase-6/task-6.1
// ---------------------------------------------------------------------------

describe("SLO: refresh cycle <200ms (REQ-TUI-051)", () => {
  it("renders 10 sessions in <200ms", () => {
    const data = makeMockDashboardData(10)

    const start = performance.now()
    const result = renderDashboard(data)
    const elapsed = performance.now() - start

    expect(result.output.length).toBeGreaterThan(0)
    expect(elapsed).toBeLessThan(200)
  })

  it("renders 20 sessions in <200ms", () => {
    const data = makeMockDashboardData(20)

    const start = performance.now()
    const result = renderDashboard(data)
    const elapsed = performance.now() - start

    expect(result.output.length).toBeGreaterThan(0)
    expect(elapsed).toBeLessThan(200)
  })

  it("renders 50 sessions in <200ms", () => {
    const data = makeMockDashboardData(50)

    const start = performance.now()
    const result = renderDashboard(data, { maxRows: 20 })
    const elapsed = performance.now() - start

    expect(result.output.length).toBeGreaterThan(0)
    expect(elapsed).toBeLessThan(200)
  })

  it("renderDashboardJson for 10 sessions completes in <200ms", () => {
    const data = makeMockDashboardData(10)

    const start = performance.now()
    const json = renderDashboardJson(data)
    const elapsed = performance.now() - start

    expect(json.length).toBeGreaterThan(0)
    expect(elapsed).toBeLessThan(200)
  })

  it("full collect + render cycle for 10 sessions completes in <200ms", async () => {
    const collector = new SessionDataCollector()
    const mockSource = makeMockSource(10)

    const start = performance.now()
    const data = await collector.collect({
      repoRoot: "/tmp/perf-test",
      sources: [mockSource],
    })
    renderDashboard(data)
    const elapsed = performance.now() - start

    expect(elapsed).toBeLessThan(200)
  })
})

// ---------------------------------------------------------------------------
// SLO: memory overhead <5MB (REQ-TUI-052)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-052 plan=phase-6/task-6.1
// ---------------------------------------------------------------------------

describe("SLO: memory overhead <5MB (REQ-TUI-052)", () => {
  it("DashboardData for 10 sessions has a reasonable serialized size (<5MB)", () => {
    const data = makeMockDashboardData(10)
    // Proxy for memory overhead: JSON serialization size of the data payload
    const serialized = JSON.stringify(data)
    const sizeBytes = new TextEncoder().encode(serialized).length
    const sizeMB = sizeBytes / (1024 * 1024)
    // 10 sessions should be well under 5MB
    expect(sizeMB).toBeLessThan(5)
  })

  it("DashboardData for 100 sessions has a reasonable serialized size (<5MB)", () => {
    const data = makeMockDashboardData(100)
    const serialized = JSON.stringify(data)
    const sizeBytes = new TextEncoder().encode(serialized).length
    const sizeMB = sizeBytes / (1024 * 1024)
    expect(sizeMB).toBeLessThan(5)
  })

  it("rendered output for 20 sessions is a reasonable string size (<1MB)", () => {
    const data = makeMockDashboardData(20)
    const result = renderDashboard(data)
    const sizeBytes = new TextEncoder().encode(result.output).length
    const sizeMB = sizeBytes / (1024 * 1024)
    expect(sizeMB).toBeLessThan(1)
  })
})

// ---------------------------------------------------------------------------
// SLO: file read latency <10ms per file (REQ-TUI-054)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-054 plan=phase-6/task-6.1
// ---------------------------------------------------------------------------

describe("SLO: file read latency <10ms per file (REQ-TUI-054)", () => {
  it("mock source getSessions() for 1 session completes in <10ms", async () => {
    const source = makeMockSource(1)

    const start = performance.now()
    await source.getSessions()
    const elapsed = performance.now() - start

    // Mock source has no I/O — verifies no overhead in the source interface itself
    expect(elapsed).toBeLessThan(10)
  })

  it("mock source getSessions() for 10 sessions completes in <10ms", async () => {
    const source = makeMockSource(10)

    const start = performance.now()
    await source.getSessions()
    const elapsed = performance.now() - start

    expect(elapsed).toBeLessThan(10)
  })

  it("collector processes a single-source 10-session payload in <10ms (excluding I/O)", async () => {
    // This tests the collector's merge/dedup/sort logic overhead only
    const collector = new SessionDataCollector()
    const source = makeMockSource(10)

    // Pre-warm: call once to avoid any JIT overhead in the measurement
    await collector.collect({ repoRoot: "/tmp/perf-test", sources: [source] })

    const start = performance.now()
    await collector.collect({ repoRoot: "/tmp/perf-test", sources: [source] })
    const elapsed = performance.now() - start

    expect(elapsed).toBeLessThan(10)
  })
})

// ---------------------------------------------------------------------------
// SLO: zero CPU when not visible (REQ-TUI-056)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-056 plan=phase-6/task-6.1
// ---------------------------------------------------------------------------

describe("SLO: zero CPU when not visible (REQ-TUI-056)", () => {
  it("one-shot execution: renderDashboard() is a pure function with no side effects", () => {
    // Verify renderDashboard() is a pure function with no side effects.
    // Calling it multiple times with independent data produces independent results.
    const data1 = makeMockDashboardData(10)
    const data2 = makeMockDashboardData(10)

    const result1 = renderDashboard(data1)
    const result2 = renderDashboard(data2)

    // Results should be non-empty and independent (no shared state)
    expect(result1.output).not.toBe("")
    expect(result2.output).not.toBe("")
    // Both results have the same structure (same session count)
    expect(result1.totalSessions).toBe(10)
    expect(result2.totalSessions).toBe(10)
  })

  it("renderDashboard() does not mutate the input DashboardData", () => {
    const data = makeMockDashboardData(5)
    const originalSessionCount = data.sessions.length
    const originalGeneratedAt = data.generatedAt.toISOString()

    renderDashboard(data)

    // Input data should be unchanged after rendering
    expect(data.sessions.length).toBe(originalSessionCount)
    expect(data.generatedAt.toISOString()).toBe(originalGeneratedAt)
  })

  it("calling renderDashboard() 100 times does not accumulate state", () => {
    const data = makeMockDashboardData(5)

    // Call 100 times — if there were timer leaks or shared state, results would diverge
    const results = Array.from({ length: 100 }, () => renderDashboard(data))

    // All results should be identical (pure function)
    const firstOutput = results[0].output
    for (const result of results) {
      expect(result.output).toBe(firstOutput)
      expect(result.totalSessions).toBe(5)
      expect(result.truncated).toBe(false)
    }
  })

  it("renderDashboard() with different data produces different output (no stale cache)", () => {
    const data1 = makeMockDashboardData(3)
    const data2 = makeMockDashboardData(7)

    const result1 = renderDashboard(data1)
    const result2 = renderDashboard(data2)

    // Different input → different output (no stale shared state)
    expect(result1.totalSessions).toBe(3)
    expect(result2.totalSessions).toBe(7)
    expect(result1.output).not.toBe(result2.output)
  })

  it("collector.collect() is one-shot: each call returns a fresh DashboardData", async () => {
    const collector = new SessionDataCollector()
    const source = makeMockSource(5)

    const data1 = await collector.collect({ repoRoot: "/tmp/perf-test", sources: [source] })
    const data2 = await collector.collect({ repoRoot: "/tmp/perf-test", sources: [source] })

    // Each call returns a fresh payload (different generatedAt timestamps possible)
    expect(data1.sessions.length).toBe(5)
    expect(data2.sessions.length).toBe(5)
    // generatedAt should be a valid Date in both cases
    expect(data1.generatedAt instanceof Date).toBe(true)
    expect(data2.generatedAt instanceof Date).toBe(true)
  })
})
