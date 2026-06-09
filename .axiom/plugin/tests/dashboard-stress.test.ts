/**
 * TUI Dashboard — stress tests for high session counts.
 *
 * Verifies that the dashboard handles 50+ and 100+ sessions without errors,
 * with correct truncation behavior and JSON serialization.
 *
 * REQ-TUI-014: The table MUST paginate at a configurable max rows (default: 20).
 *
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-014
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-014 plan=phase-6/task-6.2 test=dashboard-stress.test.ts
 */

import { describe, it, expect } from "bun:test"
import { renderDashboard, renderDashboardJson } from "../src/dashboard/renderer"
import type { DashboardData, SessionRow, DashboardAggregates } from "../src/dashboard/types"

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

/** Build a minimal valid SessionRow for stress testing. */
function makeStressRow(overrides: Partial<SessionRow> = {}): SessionRow {
  return {
    sessionId: `stress-sess-${Math.random().toString(36).slice(2, 8)}`,
    title: "Stress session",
    workItemId: "STRESS-001",
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
function makeMockDashboardData(n: number): DashboardData {
  const sessions: SessionRow[] = Array.from({ length: n }, (_, i) =>
    makeStressRow({
      sessionId: `stress-sess-${String(i).padStart(4, "0")}`,
      title: `Session ${String(i).padStart(3, "0")}`,
      workItemId: `WI-${i}`,
      costUsd: 0.1 + i * 0.01,
      totalTokens: 1000 + i * 100,
      durationSeconds: 60 + i * 10,
      status: i % 3 === 0 ? "running" : i % 3 === 1 ? "idle" : "stopped",
    })
  )

  const aggregates: DashboardAggregates = {
    totalSessions: n,
    totalCostUsd: sessions.reduce((sum, s) => sum + s.costUsd, 0),
    totalTokens: sessions.reduce((sum, s) => sum + s.totalTokens, 0),
    avgSpeedTokensPerSec: 58,
  }

  return {
    sessions,
    aggregates,
    sources: {
      inMemory: { available: true, lastUpdate: new Date() },
      file: { available: false, lastUpdate: null },
      network: { available: false, lastUpdate: null },
    },
    generatedAt: new Date(),
  }
}

// ---------------------------------------------------------------------------
// Stress test: 50+ sessions (REQ-TUI-014)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-014 plan=phase-6/task-6.2
// ---------------------------------------------------------------------------

describe("stress test: 50+ sessions (REQ-TUI-014)", () => {
  it("handles 50 sessions without error", () => {
    const data = makeMockDashboardData(50)
    const result = renderDashboard(data, { maxRows: 20 })
    expect(result.truncated).toBe(true)
    expect(result.totalSessions).toBe(50)
    // Only 20 rows rendered — verify via totalSessions vs truncated flag
    // (The │ \d pattern also matches stats bar content, so we use the API result)
    expect(result.output).toContain("Showing 20 of 50 sessions")
  })

  it("handles 100 sessions without error", () => {
    const data = makeMockDashboardData(100)
    const result = renderDashboard(data, { maxRows: 20 })
    expect(result.truncated).toBe(true)
    expect(result.totalSessions).toBe(100)
  })

  it("renderDashboardJson handles 100 sessions", () => {
    const data = makeMockDashboardData(100)
    const json = renderDashboardJson(data)
    const parsed = JSON.parse(json)
    expect(parsed.sessions).toHaveLength(100)
  })

  it("50 sessions: output is a non-empty string", () => {
    const data = makeMockDashboardData(50)
    const result = renderDashboard(data, { maxRows: 20 })
    expect(result.output).toBeTruthy()
    expect(result.output.length).toBeGreaterThan(0)
  })

  it("50 sessions: output contains 'Axiom Dashboard' header", () => {
    const data = makeMockDashboardData(50)
    const result = renderDashboard(data, { maxRows: 20 })
    expect(result.output).toContain("Axiom Dashboard")
  })

  it("50 sessions: output contains truncation hint", () => {
    const data = makeMockDashboardData(50)
    const result = renderDashboard(data, { maxRows: 20 })
    expect(result.output).toContain("Showing 20 of 50 sessions")
  })

  it("50 sessions: output contains 'show all' hint", () => {
    const data = makeMockDashboardData(50)
    const result = renderDashboard(data, { maxRows: 20 })
    expect(result.output).toContain("show all")
  })

  it("50 sessions: output starts with ┌ and ends with ┘", () => {
    const data = makeMockDashboardData(50)
    const result = renderDashboard(data, { maxRows: 20 })
    expect(result.output.startsWith("┌")).toBe(true)
    expect(result.output.trimEnd().endsWith("┘")).toBe(true)
  })

  it("100 sessions: output contains truncation hint", () => {
    const data = makeMockDashboardData(100)
    const result = renderDashboard(data, { maxRows: 20 })
    expect(result.output).toContain("Showing 20 of 100 sessions")
  })

  it("100 sessions: output is valid box-drawing table", () => {
    const data = makeMockDashboardData(100)
    const result = renderDashboard(data, { maxRows: 20 })
    expect(result.output).toContain("│")
    expect(result.output).toContain("─")
    expect(result.output).toContain("Session")
    expect(result.output).toContain("Cost")
  })

  it("200 sessions: handles without error (extreme stress)", () => {
    const data = makeMockDashboardData(200)
    const result = renderDashboard(data, { maxRows: 20 })
    expect(result.truncated).toBe(true)
    expect(result.totalSessions).toBe(200)
    expect(result.output).toContain("Showing 20 of 200 sessions")
  })
})

// ---------------------------------------------------------------------------
// Stress test: custom maxRows
// ---------------------------------------------------------------------------

describe("stress test: custom maxRows with large session counts", () => {
  it("50 sessions with maxRows=50: not truncated", () => {
    const data = makeMockDashboardData(50)
    const result = renderDashboard(data, { maxRows: 50 })
    expect(result.truncated).toBe(false)
    expect(result.totalSessions).toBe(50)
  })

  it("51 sessions with maxRows=50: truncated", () => {
    const data = makeMockDashboardData(51)
    const result = renderDashboard(data, { maxRows: 50 })
    expect(result.truncated).toBe(true)
    expect(result.totalSessions).toBe(51)
  })

  it("100 sessions with maxRows=5: only 5 rows rendered", () => {
    const data = makeMockDashboardData(100)
    const result = renderDashboard(data, { maxRows: 5 })
    expect(result.truncated).toBe(true)
    expect(result.totalSessions).toBe(100)
    // Verify via the truncation hint line
    expect(result.output).toContain("Showing 5 of 100 sessions")
  })
})

// ---------------------------------------------------------------------------
// Stress test: JSON output with large session counts
// ---------------------------------------------------------------------------

describe("stress test: JSON output with large session counts", () => {
  it("renderDashboardJson for 50 sessions produces valid JSON", () => {
    const data = makeMockDashboardData(50)
    const json = renderDashboardJson(data)
    expect(() => JSON.parse(json)).not.toThrow()
    const parsed = JSON.parse(json)
    expect(parsed.sessions).toHaveLength(50)
  })

  it("renderDashboardJson for 200 sessions produces valid JSON", () => {
    const data = makeMockDashboardData(200)
    const json = renderDashboardJson(data)
    expect(() => JSON.parse(json)).not.toThrow()
    const parsed = JSON.parse(json)
    expect(parsed.sessions).toHaveLength(200)
  })

  it("renderDashboardJson compact mode for 100 sessions produces valid JSON", () => {
    const data = makeMockDashboardData(100)
    const json = renderDashboardJson(data, { pretty: false })
    expect(() => JSON.parse(json)).not.toThrow()
    const parsed = JSON.parse(json)
    expect(parsed.sessions).toHaveLength(100)
    // Compact mode: no newlines
    expect(json).not.toContain("\n")
  })

  it("renderDashboardJson preserves all session fields for 50 sessions", () => {
    const data = makeMockDashboardData(50)
    const json = renderDashboardJson(data)
    const parsed = JSON.parse(json)
    // Spot-check first and last session
    const first = parsed.sessions[0]
    const last = parsed.sessions[49]
    expect(first).toHaveProperty("sessionId")
    expect(first).toHaveProperty("costUsd")
    expect(first).toHaveProperty("totalTokens")
    expect(first).toHaveProperty("updatedAt")
    expect(last).toHaveProperty("sessionId")
  })
})
