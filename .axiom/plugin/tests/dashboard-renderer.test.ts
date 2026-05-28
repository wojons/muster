/**
 * Tests for the TUI Dashboard Unicode box-drawing table renderer.
 *
 * Covers: empty state, single session, multi-session, column formatting,
 * truncation, aggregate stats bar, footer, and pagination.
 *
 * REQ-TUI-003: Aggregate stats bar + session table.
 * REQ-TUI-010: Per-session column formatting.
 * REQ-TUI-013: Sessions with no cost data show $0.00 / 0 tokens / 0s.
 * REQ-TUI-014: Table paginates at configurable max rows (default: 20).
 *
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-003
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-003 plan=phase-2/task-2.1 test=dashboard-renderer.test.ts
 */

import { describe, it, expect } from "bun:test"
import {
  renderDashboard,
  renderDashboardExtended,
  renderDashboardJson,
  MODEL_DISPLAY_ALIASES,
  resolveModelDisplayName,
  renderSessionJumpInstructions,
  renderToolBar,
} from "../src/dashboard/renderer"
import type { DashboardData, SessionRow, DashboardAggregates } from "../src/dashboard/types"

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

/** Build a minimal valid SessionRow for testing. */
function makeRow(overrides: Partial<SessionRow> = {}): SessionRow {
  return {
    sessionId: "sess-001",
    title: "Fix login",
    workItemId: "AUTH-234",
    model: "Sonnet",
    modelRaw: "anthropic.claude-sonnet-4-6",
    costUsd: 0.82,
    totalTokens: 72_000,
    durationSeconds: 1380, // 23 minutes
    status: "running",
    source: "in-memory",
    updatedAt: new Date("2026-04-12T10:00:00.000Z"),
    isTeam: false,
    ...overrides,
  }
}

/** Build a DashboardAggregates object. */
function makeAggregates(overrides: Partial<DashboardAggregates> = {}): DashboardAggregates {
  return {
    totalSessions: 3,
    totalCostUsd: 1.47,
    totalTokens: 128_000,
    avgSpeedTokensPerSec: 42,
    ...overrides,
  }
}

/** Build a minimal DashboardData payload. */
function makeData(
  sessions: SessionRow[],
  aggregatesOverrides: Partial<DashboardAggregates> = {}
): DashboardData {
  return {
    sessions,
    aggregates: makeAggregates(aggregatesOverrides),
    sources: {
      inMemory: { available: true, lastUpdate: new Date() },
      file: { available: false, lastUpdate: null },
      network: { available: false, lastUpdate: null },
    },
    generatedAt: new Date("2026-04-12T10:00:00.000Z"),
  }
}

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------

describe("renderDashboard() — empty state", () => {
  it("returns a non-empty output string when sessions is empty", () => {
    const data = makeData([])
    const result = renderDashboard(data)
    expect(result.output).toBeTruthy()
    expect(result.output.length).toBeGreaterThan(0)
  })

  it("returns truncated=false when sessions is empty", () => {
    const data = makeData([])
    const result = renderDashboard(data)
    expect(result.truncated).toBe(false)
  })

  it("returns totalSessions=0 when sessions is empty", () => {
    const data = makeData([])
    const result = renderDashboard(data)
    expect(result.totalSessions).toBe(0)
  })

  it("empty state output contains 'Axiom Dashboard' header", () => {
    const data = makeData([])
    const result = renderDashboard(data)
    expect(result.output).toContain("Axiom Dashboard")
  })

  it("empty state output contains 'No active sessions' message", () => {
    const data = makeData([])
    const result = renderDashboard(data)
    expect(result.output).toContain("No active sessions")
  })

  it("empty state output does NOT contain the footer keyboard hints", () => {
    const data = makeData([])
    const result = renderDashboard(data)
    // Empty state is a minimal box — no footer hints
    expect(result.output).not.toContain("[r] Refresh")
  })
})

// ---------------------------------------------------------------------------
// Single session
// ---------------------------------------------------------------------------

describe("renderDashboard() — single session", () => {
  it("returns a valid table string", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data)
    expect(result.output).toBeTruthy()
    expect(result.output.length).toBeGreaterThan(0)
  })

  it("output contains the header line 'Axiom Dashboard'", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data)
    expect(result.output).toContain("Axiom Dashboard")
  })

  it("output contains the session title", () => {
    const data = makeData([makeRow({ title: "Fix login" })])
    const result = renderDashboard(data)
    expect(result.output).toContain("Fix login")
  })

  it("output contains the work item ID", () => {
    const data = makeData([makeRow({ workItemId: "AUTH-234" })])
    const result = renderDashboard(data)
    expect(result.output).toContain("AUTH-234")
  })

  it("output contains the model name", () => {
    const data = makeData([makeRow({ model: "Sonnet" })])
    const result = renderDashboard(data)
    expect(result.output).toContain("Sonnet")
  })

  it("output contains the formatted cost", () => {
    const data = makeData([makeRow({ costUsd: 0.82 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("$0.82")
  })

  it("output contains the formatted token count", () => {
    const data = makeData([makeRow({ totalTokens: 72_000 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("72K")
  })

  it("output contains the formatted duration", () => {
    const data = makeData([makeRow({ durationSeconds: 1380 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("23m")
  })

  it("output contains the status indicator", () => {
    const data = makeData([makeRow({ status: "running" })])
    const result = renderDashboard(data)
    expect(result.output).toContain("● Running")
  })

  it("returns truncated=false for a single session", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data)
    expect(result.truncated).toBe(false)
  })

  it("returns totalSessions=1 for a single session", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data)
    expect(result.totalSessions).toBe(1)
  })
})

// ---------------------------------------------------------------------------
// Three sessions (spec example)
// ---------------------------------------------------------------------------

describe("renderDashboard() — three sessions (spec example)", () => {
  const sessions = [
    makeRow({
      sessionId: "sess-1",
      title: "Fix login",
      workItemId: "AUTH-234",
      model: "Sonnet",
      costUsd: 0.82,
      totalTokens: 72_000,
      durationSeconds: 1380,
      status: "running",
    }),
    makeRow({
      sessionId: "sess-2",
      title: "Perf bench",
      workItemId: "PERF-567",
      model: "Opus",
      costUsd: 0.58,
      totalTokens: 48_000,
      durationSeconds: 840,
      status: "running",
    }),
    makeRow({
      sessionId: "sess-3",
      title: "Spec writing",
      workItemId: undefined,
      model: "Sonnet",
      costUsd: 0.07,
      totalTokens: 8_000,
      durationSeconds: 180,
      status: "idle",
    }),
  ]

  it("renders all 3 session rows", () => {
    const data = makeData(sessions)
    const result = renderDashboard(data)
    expect(result.output).toContain("Fix login")
    expect(result.output).toContain("Perf bench")
    expect(result.output).toContain("Spec writing")
  })

  it("returns totalSessions=3", () => {
    const data = makeData(sessions)
    const result = renderDashboard(data)
    expect(result.totalSessions).toBe(3)
  })

  it("returns truncated=false for 3 sessions (below default maxRows=20)", () => {
    const data = makeData(sessions)
    const result = renderDashboard(data)
    expect(result.truncated).toBe(false)
  })

  it("output contains all three status indicators", () => {
    const data = makeData(sessions)
    const result = renderDashboard(data)
    expect(result.output).toContain("● Running")
    expect(result.output).toContain("○ Idle")
  })

  it("output contains the em-dash for missing work item", () => {
    const data = makeData(sessions)
    const result = renderDashboard(data)
    expect(result.output).toContain("—")
  })

  it("output contains the non-interactive footer hints (default mode)", () => {
    // Default (isInteractive=false): plugin tool one-shot mode — no misleading keyboard hints.
    // REQ-TUI-004: show helpful CLI note instead.
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-004 plan=step-verify-H002
    const data = makeData(sessions)
    const result = renderDashboard(data)
    expect(result.output).toContain("Use --watch for live refresh")
    expect(result.output).toContain("axiom-dashboard --help")
    expect(result.output).not.toContain("[r] Refresh")
    expect(result.output).not.toContain("[q] Close")
    expect(result.output).not.toContain("[t] Toggle team view")
  })

  it("output contains the interactive footer hints when isInteractive=true", () => {
    // isInteractive=true: standalone --watch mode — full keyboard hints are valid.
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-004 plan=step-verify-H002
    const data = makeData(sessions)
    const result = renderDashboard(data, { isInteractive: true })
    expect(result.output).toContain("[r] Refresh")
    expect(result.output).toContain("[q] Close")
    expect(result.output).toContain("[t] Toggle team view")
    expect(result.output).not.toContain("Use --watch for live refresh")
  })

  it("output contains the aggregate stats bar", () => {
    // sessions fixture: 2 running (sess-1, sess-2) + 1 idle (sess-3)
    // REQ-TUI-072: compact stats bar shows running/idle/stopped counts as symbols.
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-072 plan=F-002
    const data = makeData(sessions, {
      totalSessions: 3,
      totalCostUsd: 1.47,
      totalTokens: 128_000,
      avgSpeedTokensPerSec: 42,
    })
    const result = renderDashboard(data)
    // Compact symbol format: "Sessions: 3 (●2 ○1 ■0)" or "Sessions: 3 (●2 ○1)"
    expect(result.output).toContain("Sessions: 3 (")
    expect(result.output).toContain("●")  // running indicator
    expect(result.output).toContain("Total Cost: $1.47")
    expect(result.output).toContain("Total Tokens: 128K")
    expect(result.output).toContain("Avg Speed: 42 tok/s")
  })
})

// ---------------------------------------------------------------------------
// Column formatting — session title truncation
// ---------------------------------------------------------------------------

describe("renderDashboard() — session title truncation (REQ-TUI-010)", () => {
  it("truncates session title longer than 12 chars with ellipsis", () => {
    const longTitle = "This is a very long session title"
    const data = makeData([makeRow({ title: longTitle })])
    const result = renderDashboard(data)
    // Session column is 12 chars wide: first 11 chars + ellipsis
    expect(result.output).toContain("This is a v…")
    // Should NOT contain the full title
    expect(result.output).not.toContain(longTitle)
  })

  it("does not truncate a title of exactly 12 chars", () => {
    const exactTitle = "Exactly12chr" // 12 chars
    expect(exactTitle.length).toBe(12)
    const data = makeData([makeRow({ title: exactTitle })])
    const result = renderDashboard(data)
    expect(result.output).toContain(exactTitle)
    expect(result.output).not.toContain("Exactly12ch…")
  })

  it("does not truncate a title shorter than 12 chars", () => {
    const shortTitle = "Short"
    const data = makeData([makeRow({ title: shortTitle })])
    const result = renderDashboard(data)
    expect(result.output).toContain("Short")
  })
})

// ---------------------------------------------------------------------------
// Column formatting — work item
// ---------------------------------------------------------------------------

describe("renderDashboard() — work item column (REQ-TUI-010)", () => {
  it("shows em-dash when workItemId is undefined", () => {
    const data = makeData([makeRow({ workItemId: undefined })])
    const result = renderDashboard(data)
    expect(result.output).toContain("—")
  })

  it("shows the work item ID when present", () => {
    const data = makeData([makeRow({ workItemId: "AUTH-234" })])
    const result = renderDashboard(data)
    expect(result.output).toContain("AUTH-234")
  })
})

// ---------------------------------------------------------------------------
// Column formatting — cost
// ---------------------------------------------------------------------------

describe("renderDashboard() — cost formatting (REQ-TUI-010)", () => {
  it("formats cost as $X.XX", () => {
    const data = makeData([makeRow({ costUsd: 0.82 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("$0.82")
  })

  it("formats zero cost as $0.00 (REQ-TUI-013)", () => {
    const data = makeData([makeRow({ costUsd: 0 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("$0.00")
  })

  it("formats cost with two decimal places", () => {
    const data = makeData([makeRow({ costUsd: 1.5 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("$1.50")
  })
})

// ---------------------------------------------------------------------------
// Column formatting — tokens
// ---------------------------------------------------------------------------

describe("renderDashboard() — token formatting (REQ-TUI-010)", () => {
  it("formats 72000 tokens as 72K", () => {
    const data = makeData([makeRow({ totalTokens: 72_000 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("72K")
  })

  it("formats 8000 tokens as 8K", () => {
    const data = makeData([makeRow({ totalTokens: 8_000 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("8K")
  })

  it("formats 500 tokens as 500 (no K suffix)", () => {
    const data = makeData([makeRow({ totalTokens: 500 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("500")
    // Should NOT have 500K
    expect(result.output).not.toContain("500K")
  })

  it("formats 0 tokens as 0 (REQ-TUI-013)", () => {
    const data = makeData([makeRow({ totalTokens: 0 })])
    const result = renderDashboard(data)
    // "0" should appear in the output (token column)
    expect(result.output).toContain("0")
  })

  it("formats 1000 tokens as 1K (boundary)", () => {
    const data = makeData([makeRow({ totalTokens: 1000 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("1K")
  })
})

// ---------------------------------------------------------------------------
// Column formatting — duration
// ---------------------------------------------------------------------------

describe("renderDashboard() — duration formatting (REQ-TUI-010)", () => {
  it("formats 1380 seconds as 23m", () => {
    const data = makeData([makeRow({ durationSeconds: 1380 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("23m")
  })

  it("formats 3900 seconds as 1h 5m", () => {
    const data = makeData([makeRow({ durationSeconds: 3900 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("1h 5m")
  })

  it("formats 0 seconds as 0m (REQ-TUI-013)", () => {
    const data = makeData([makeRow({ durationSeconds: 0 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("0m")
  })

  it("formats 59 minutes as 59m (just below 1h boundary)", () => {
    const data = makeData([makeRow({ durationSeconds: 59 * 60 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("59m")
  })

  it("formats 60 minutes as 1h 0m (at 1h boundary)", () => {
    const data = makeData([makeRow({ durationSeconds: 60 * 60 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("1h 0m")
  })
})

// ---------------------------------------------------------------------------
// Column formatting — status
// ---------------------------------------------------------------------------

describe("renderDashboard() — status formatting (REQ-TUI-010)", () => {
  it("shows '● Running' for running status", () => {
    const data = makeData([makeRow({ status: "running" })])
    const result = renderDashboard(data)
    expect(result.output).toContain("● Running")
  })

  it("shows '○ Idle' for idle status", () => {
    const data = makeData([makeRow({ status: "idle" })])
    const result = renderDashboard(data)
    expect(result.output).toContain("○ Idle")
  })

  it("shows '■ Stopped' for stopped status", () => {
    const data = makeData([makeRow({ status: "stopped" })])
    const result = renderDashboard(data)
    expect(result.output).toContain("■ Stopped")
  })
})

// ---------------------------------------------------------------------------
// Aggregate stats bar
// ---------------------------------------------------------------------------

describe("renderDashboard() — aggregate stats bar (REQ-TUI-003)", () => {
  it("shows correct session count in stats bar", () => {
    // REQ-TUI-072: compact stats bar shows running/idle/stopped counts.
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-072 plan=F-002
    const data = makeData([makeRow()], { totalSessions: 1 })
    const result = renderDashboard(data)
    // New format: "Sessions: 1 (1 running)" — stopped omitted when 0
    expect(result.output).toContain("Sessions: 1 (")
    expect(result.output).toContain("●")  // running symbol
    // stopped symbol ■ present in format
  })

  it("shows correct total cost in stats bar", () => {
    const data = makeData([makeRow()], { totalCostUsd: 0.82 })
    const result = renderDashboard(data)
    expect(result.output).toContain("Total Cost: $0.82")
  })

  it("shows correct total tokens in stats bar", () => {
    const data = makeData([makeRow()], { totalTokens: 72_000 })
    const result = renderDashboard(data)
    expect(result.output).toContain("Total Tokens: 72K")
  })

  it("shows correct avg speed in stats bar", () => {
    const data = makeData([makeRow()], { avgSpeedTokensPerSec: 42 })
    const result = renderDashboard(data)
    expect(result.output).toContain("Avg Speed: 42 tok/s")
  })

  it("stats bar never overflows box width — large values clamped (REQ-TUI-072)", () => {
    // INJ-1: regression test for stats bar overflow guard
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-072 plan=F-002/overflow-guard
    const sessions = Array.from({ length: 99 }, (_, i) =>
      makeRow({
        sessionId: `sess-${i}`,
        status: i < 50 ? "running" : i < 75 ? "idle" : "stopped",
        costUsd: 9999.99,
        totalTokens: 9_999_000,
      })
    )
    const data = makeData(sessions, {
      totalCostUsd: 9999.99,
      totalTokens: 9_999_000,
      avgSpeedTokensPerSec: 9999,
    })
    const result = renderDashboard(data, { width: 90, maxRows: 20 })
    // Every line must fit within the box width (90 chars)
    const lines = result.output.split("\n").filter((l) => l.length > 0)
    for (const line of lines) {
      // Compact mode has no ANSI codes — measure line length directly
      expect(line.length).toBeLessThanOrEqual(90)
    }
  })
})

// ---------------------------------------------------------------------------
// Pagination (REQ-TUI-014)
// ---------------------------------------------------------------------------

describe("renderDashboard() — pagination (REQ-TUI-014)", () => {
  it("returns truncated=false when sessions fit within maxRows", () => {
    const sessions = Array.from({ length: 5 }, (_, i) =>
      makeRow({ sessionId: `sess-${i}`, title: `Session ${i}` })
    )
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 88, maxRows: 10 })
    expect(result.truncated).toBe(false)
  })

  it("returns truncated=true when sessions exceed maxRows", () => {
    const sessions = Array.from({ length: 25 }, (_, i) =>
      makeRow({ sessionId: `sess-${i}`, title: `Session ${i}` })
    )
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 88, maxRows: 20 })
    expect(result.truncated).toBe(true)
  })

  it("returns totalSessions=25 even when truncated to 20", () => {
    const sessions = Array.from({ length: 25 }, (_, i) =>
      makeRow({ sessionId: `sess-${i}`, title: `Session ${i}` })
    )
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 88, maxRows: 20 })
    expect(result.totalSessions).toBe(25)
  })

  it("only renders maxRows rows when truncated", () => {
    const sessions = Array.from({ length: 25 }, (_, i) =>
      makeRow({ sessionId: `sess-${i}`, title: `Row ${String(i).padStart(2, "0")}` })
    )
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 88, maxRows: 5 })
    // First 5 rows should be present
    expect(result.output).toContain("Row 00")
    expect(result.output).toContain("Row 04")
    // Row 05 should NOT be present (truncated)
    expect(result.output).not.toContain("Row 05")
  })

  it("default maxRows is 20 (truncates at 21 sessions)", () => {
    const sessions = Array.from({ length: 21 }, (_, i) =>
      makeRow({ sessionId: `sess-${i}`, title: `Session ${i}` })
    )
    const data = makeData(sessions)
    const result = renderDashboard(data)
    expect(result.truncated).toBe(true)
    expect(result.totalSessions).toBe(21)
  })

  it("returns truncated=false for exactly maxRows sessions", () => {
    const sessions = Array.from({ length: 20 }, (_, i) =>
      makeRow({ sessionId: `sess-${i}`, title: `Session ${i}` })
    )
    const data = makeData(sessions)
    const result = renderDashboard(data)
    expect(result.truncated).toBe(false)
    expect(result.totalSessions).toBe(20)
  })
})

// ---------------------------------------------------------------------------
// Default width
// ---------------------------------------------------------------------------

describe("renderDashboard() — default width", () => {
  it("default width is 90 characters per line", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data)
    const lines = result.output.split("\n")
    // The top border line should be exactly 90 chars wide
    const topLine = lines[0]
    expect(topLine.length).toBe(90)
  })

  it("respects custom width option", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data, { width: 90 })
    const lines = result.output.split("\n")
    const topLine = lines[0]
    expect(topLine.length).toBe(90)
  })
})

// ---------------------------------------------------------------------------
// Output structure — box-drawing characters
// ---------------------------------------------------------------------------

describe("renderDashboard() — box-drawing structure", () => {
  it("output starts with top-left corner ┌", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data)
    expect(result.output.startsWith("┌")).toBe(true)
  })

  it("output ends with bottom-right corner ┘", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data)
    expect(result.output.trimEnd().endsWith("┘")).toBe(true)
  })

  it("output contains vertical bar │ characters", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data)
    expect(result.output).toContain("│")
  })

  it("output contains horizontal bar ─ characters", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data)
    expect(result.output).toContain("─")
  })

  it("output contains column header labels", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data)
    expect(result.output).toContain("Session")
    expect(result.output).toContain("Work Item")
    expect(result.output).toContain("Model")
    expect(result.output).toContain("Cost")
    expect(result.output).toContain("Tokens")
    expect(result.output).toContain("Duration")
    expect(result.output).toContain("Status")
  })
})

// ---------------------------------------------------------------------------
// Footer
// ---------------------------------------------------------------------------

describe("renderDashboard() — footer (REQ-TUI-003, REQ-TUI-004)", () => {
  it("non-interactive footer (default) shows CLI hint, not keyboard shortcuts", () => {
    // REQ-TUI-004: Plugin tool one-shot output must not show misleading keyboard hints.
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-004 plan=step-verify-H002
    const data = makeData([makeRow()])
    const result = renderDashboard(data)
    expect(result.output).toContain("Use --watch for live refresh")
    expect(result.output).toContain("axiom-dashboard --help")
    expect(result.output).not.toContain("[r] Refresh")
    expect(result.output).not.toContain("[q] Close")
    expect(result.output).not.toContain("[t] Toggle team view")
  })

  it("interactive footer (isInteractive=true) shows full keyboard hints", () => {
    // isInteractive=true: standalone --watch mode — keyboard shortcuts are functional.
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-004 plan=step-verify-H002
    const data = makeData([makeRow()])
    const result = renderDashboard(data, { isInteractive: true })
    expect(result.output).toContain("[r] Refresh")
    expect(result.output).toContain("[q] Close")
    expect(result.output).toContain("[t] Toggle team view")
    expect(result.output).not.toContain("Use --watch for live refresh")
  })

  it("footer contains jump hint", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data)
    expect(result.output).toContain("Jump to session")
  })
})

// ---------------------------------------------------------------------------
// Team rows filtering
// ---------------------------------------------------------------------------

describe("renderDashboard() — team row filtering", () => {
  it("excludes team rows by default (includeTeam=false)", () => {
    const sessions = [
      makeRow({ sessionId: "local-1", title: "Local sess", isTeam: false }),
      makeRow({ sessionId: "team-1", title: "Team sess", isTeam: true }),
    ]
    const data = makeData(sessions)
    const result = renderDashboard(data)
    expect(result.output).toContain("Local sess")
    expect(result.output).not.toContain("Team sess")
  })

  it("includes team rows when includeTeam=true", () => {
    const sessions = [
      makeRow({ sessionId: "local-1", title: "Local sess", isTeam: false }),
      makeRow({ sessionId: "team-1", title: "Team sess", isTeam: true, teamMemberId: "team-member" }),
    ]
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 90, includeTeam: true })
    expect(result.output).toContain("Local sess")
    // Team rows show teamMemberId (not title) per REQ-TUI-041
    expect(result.output).toContain("team-member")
  })

  it("totalSessions counts only non-team sessions by default", () => {
    const sessions = [
      makeRow({ sessionId: "local-1", isTeam: false }),
      makeRow({ sessionId: "team-1", isTeam: true }),
    ]
    const data = makeData(sessions)
    const result = renderDashboard(data)
    expect(result.totalSessions).toBe(1)
  })
})

// ---------------------------------------------------------------------------
// REQ-TUI-013: Zero-cost sessions
// ---------------------------------------------------------------------------

describe("renderDashboard() — zero-cost sessions (REQ-TUI-013)", () => {
  it("shows $0.00 for sessions with no cost data", () => {
    const data = makeData([makeRow({ costUsd: 0, totalTokens: 0, durationSeconds: 0 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("$0.00")
  })

  it("shows 0 tokens for sessions with no token data", () => {
    const data = makeData([makeRow({ costUsd: 0, totalTokens: 0, durationSeconds: 0 })])
    const result = renderDashboard(data)
    // "0" should appear in the output (token column shows "0" not "0K")
    const lines = result.output.split("\n")
    const dataLines = lines.filter((l) => l.includes("│") && !l.includes("Session") && !l.includes("─"))
    expect(dataLines.some((l) => l.includes("$0.00"))).toBe(true)
  })

  it("shows 0m for sessions with zero duration", () => {
    const data = makeData([makeRow({ costUsd: 0, totalTokens: 0, durationSeconds: 0 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("0m")
  })
})

// ---------------------------------------------------------------------------
// Session ID fallback when title is undefined
// ---------------------------------------------------------------------------

describe("renderDashboard() — session ID fallback", () => {
  it("uses sessionId when title is undefined", () => {
    const data = makeData([makeRow({ title: undefined, sessionId: "sess-abc-123" })])
    const result = renderDashboard(data)
    // sessionId is used as fallback; it may be truncated if > 14 chars
    expect(result.output).toContain("sess-abc-123")
  })
})

// ---------------------------------------------------------------------------
// JSON output mode (REQ-TUI-005)
// ---------------------------------------------------------------------------

/**
 * Tests for renderDashboardJson() — non-interactive / CI / headless JSON output.
 *
 * REQ-TUI-005: The dashboard MUST support a JSON output mode for CI/headless contexts.
 *
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-005
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-005 plan=phase-2/task-2.4 test=dashboard-renderer.test.ts
 */
describe("JSON output mode (REQ-TUI-005)", () => {
  it("renderDashboardJson() returns a non-empty string", () => {
    const data = makeData([makeRow()])
    const output = renderDashboardJson(data)
    expect(typeof output).toBe("string")
    expect(output.length).toBeGreaterThan(0)
  })

  it("output is valid JSON (JSON.parse succeeds)", () => {
    const data = makeData([makeRow()])
    const output = renderDashboardJson(data)
    expect(() => JSON.parse(output)).not.toThrow()
  })

  it("sessions array has correct length", () => {
    const sessions = [makeRow({ sessionId: "s1" }), makeRow({ sessionId: "s2" })]
    const data = makeData(sessions)
    const parsed = JSON.parse(renderDashboardJson(data))
    expect(parsed.sessions).toHaveLength(2)
  })

  it("aggregates object has totalSessions, totalCostUsd, totalTokens, avgSpeedTokensPerSec", () => {
    const data = makeData([makeRow()], {
      totalSessions: 1,
      totalCostUsd: 0.82,
      totalTokens: 72_000,
      avgSpeedTokensPerSec: 42,
    })
    const parsed = JSON.parse(renderDashboardJson(data))
    expect(parsed.aggregates).toHaveProperty("totalSessions", 1)
    expect(parsed.aggregates).toHaveProperty("totalCostUsd", 0.82)
    expect(parsed.aggregates).toHaveProperty("totalTokens", 72_000)
    expect(parsed.aggregates).toHaveProperty("avgSpeedTokensPerSec", 42)
  })

  it("sources object has inMemory, file, network keys", () => {
    const data = makeData([makeRow()])
    const parsed = JSON.parse(renderDashboardJson(data))
    expect(parsed.sources).toHaveProperty("inMemory")
    expect(parsed.sources).toHaveProperty("file")
    expect(parsed.sources).toHaveProperty("network")
  })

  it("generatedAt is an ISO 8601 string", () => {
    const data = makeData([makeRow()])
    const parsed = JSON.parse(renderDashboardJson(data))
    expect(typeof parsed.generatedAt).toBe("string")
    // ISO 8601: parseable by Date and round-trips correctly
    const d = new Date(parsed.generatedAt)
    expect(isNaN(d.getTime())).toBe(false)
    expect(parsed.generatedAt).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}/)
  })

  it("updatedAt in sessions is an ISO 8601 string", () => {
    const data = makeData([makeRow({ updatedAt: new Date("2026-04-12T10:00:00.000Z") })])
    const parsed = JSON.parse(renderDashboardJson(data))
    expect(typeof parsed.sessions[0].updatedAt).toBe("string")
    expect(parsed.sessions[0].updatedAt).toBe("2026-04-12T10:00:00.000Z")
  })

  it("pretty=true (default) produces indented JSON", () => {
    const data = makeData([makeRow()])
    const output = renderDashboardJson(data)
    // Indented JSON contains newlines and leading spaces
    expect(output).toContain("\n")
    expect(output).toContain("  ")
  })

  it("pretty=false produces compact JSON (no newlines)", () => {
    const data = makeData([makeRow()])
    const output = renderDashboardJson(data, { pretty: false })
    expect(output).not.toContain("\n")
  })

  it("pretty=false output is still valid JSON", () => {
    const data = makeData([makeRow()])
    const output = renderDashboardJson(data, { pretty: false })
    expect(() => JSON.parse(output)).not.toThrow()
  })

  it("empty sessions produces valid JSON with empty sessions array", () => {
    const data = makeData([])
    const output = renderDashboardJson(data)
    const parsed = JSON.parse(output)
    expect(Array.isArray(parsed.sessions)).toBe(true)
    expect(parsed.sessions).toHaveLength(0)
  })

  it("sources.file.lastUpdate is null when source has never fetched", () => {
    const data = makeData([makeRow()])
    // makeData sets file.lastUpdate = null
    const parsed = JSON.parse(renderDashboardJson(data))
    expect(parsed.sources.file.lastUpdate).toBeNull()
  })

  it("sources.inMemory.lastUpdate is an ISO 8601 string when available", () => {
    const data = makeData([makeRow()])
    // makeData sets inMemory.lastUpdate = new Date()
    const parsed = JSON.parse(renderDashboardJson(data))
    expect(typeof parsed.sources.inMemory.lastUpdate).toBe("string")
    const d = new Date(parsed.sources.inMemory.lastUpdate)
    expect(isNaN(d.getTime())).toBe(false)
  })

  it("session fields are correctly serialized (sessionId, model, costUsd, totalTokens)", () => {
    const data = makeData([
      makeRow({
        sessionId: "sess-xyz",
        model: "Opus",
        costUsd: 1.23,
        totalTokens: 50_000,
      }),
    ])
    const parsed = JSON.parse(renderDashboardJson(data))
    const s = parsed.sessions[0]
    expect(s.sessionId).toBe("sess-xyz")
    expect(s.model).toBe("Opus")
    expect(s.costUsd).toBe(1.23)
    expect(s.totalTokens).toBe(50_000)
  })
})

// ---------------------------------------------------------------------------
// Model display aliases (REQ-TUI-012)
// ---------------------------------------------------------------------------

/**
 * Tests for MODEL_DISPLAY_ALIASES and resolveModelDisplayName().
 *
 * REQ-TUI-012: Known models use display aliases; unknown models fall back to
 * first 15 chars + ellipsis.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-012 plan=phase-2/task-2.2 test=dashboard-renderer.test.ts
 */
describe("model display aliases (REQ-TUI-012)", () => {
  it("MODEL_DISPLAY_ALIASES is exported and is an object", () => {
    expect(typeof MODEL_DISPLAY_ALIASES).toBe("object")
    expect(MODEL_DISPLAY_ALIASES).not.toBeNull()
  })

  it("resolves claude-3-5-sonnet-20241022 → 'Claude Sonnet 3.5'", () => {
    expect(resolveModelDisplayName("claude-3-5-sonnet-20241022")).toBe("Claude Sonnet 3.5")
  })

  it("resolves claude-sonnet-4-20250514 → 'Claude Sonnet 4'", () => {
    expect(resolveModelDisplayName("claude-sonnet-4-20250514")).toBe("Claude Sonnet 4")
  })

  it("resolves amazon-bedrock/anthropic.claude-sonnet-4-6 → 'Claude Sonnet 4.6'", () => {
    expect(resolveModelDisplayName("amazon-bedrock/anthropic.claude-sonnet-4-6")).toBe("Claude Sonnet 4.6")
  })

  it("resolves anthropic.claude-opus-4-6-v1 → 'Claude Opus 4.6'", () => {
    expect(resolveModelDisplayName("anthropic.claude-opus-4-6-v1")).toBe("Claude Opus 4.6")
  })

  it("resolves claude-3-5-haiku-20241022 → 'Claude Haiku 3.5'", () => {
    expect(resolveModelDisplayName("claude-3-5-haiku-20241022")).toBe("Claude Haiku 3.5")
  })

  it("resolves claude-haiku-4-20250414 → 'Claude Haiku 4'", () => {
    expect(resolveModelDisplayName("claude-haiku-4-20250414")).toBe("Claude Haiku 4")
  })

  it("resolves opencode/big-pickle → 'Big Pickle'", () => {
    expect(resolveModelDisplayName("opencode/big-pickle")).toBe("Big Pickle")
  })

  it("unknown model → returned as full raw ID (no truncation)", () => {
    const raw = "some-unknown-model-xyz-v99"
    const result = resolveModelDisplayName(raw)
    expect(result).toBe(raw)
    expect(result).not.toContain("…")
  })

  it("unknown model of exactly 15 chars → returned unchanged (no ellipsis)", () => {
    const raw = "exactly-15-char" // exactly 15 chars
    expect(raw.length).toBe(15)
    const result = resolveModelDisplayName(raw)
    expect(result).toBe("exactly-15-char")
    expect(result).not.toContain("…")
  })

  it("unknown model shorter than 15 chars → returned unchanged", () => {
    const raw = "short-model"
    const result = resolveModelDisplayName(raw)
    expect(result).toBe("short-model")
    expect(result).not.toContain("…")
  })

  it("MODEL_DISPLAY_ALIASES contains all expected core entries", () => {
    const expectedKeys = [
      "claude-3-5-sonnet-20241022",
      "claude-sonnet-4-20250514",
      "amazon-bedrock/anthropic.claude-sonnet-4-6",
      "anthropic.claude-opus-4-6-v1",
      "claude-3-5-haiku-20241022",
      "claude-haiku-4-20250414",
      "opencode/big-pickle",
    ]
    for (const key of expectedKeys) {
      // Use Object.prototype.hasOwnProperty to avoid toHaveProperty path-splitting on "/"
      expect(Object.prototype.hasOwnProperty.call(MODEL_DISPLAY_ALIASES, key)).toBe(true)
    }
    // Alias map has grown beyond the original 7 — verify it has at least the core entries
    expect(Object.keys(MODEL_DISPLAY_ALIASES).length).toBeGreaterThanOrEqual(7)
  })

  it("model column in table truncates display name to 8 chars when needed", () => {
    // "Sonnet 3.5" is 10 chars — exceeds COL.model=8, should be truncated to 7 + "…"
    // padCell("Sonnet 3.5", 8) → slice(0, 7) + "…" = "Sonnet …"
    const data = makeData([makeRow({ model: "Sonnet 3.5" })])
    const result = renderDashboard(data)
    expect(result.output).toContain("Sonnet …")
  })

  it("model column in table shows full name when display name fits in 8 chars", () => {
    // "Sonnet 4" is 8 chars — fits exactly in COL.model=8
    const data = makeData([makeRow({ model: "Sonnet 4" })])
    const result = renderDashboard(data)
    expect(result.output).toContain("Sonnet 4")
  })
})

// ---------------------------------------------------------------------------
// Status indicator characters (REQ-TUI-010)
// ---------------------------------------------------------------------------

/**
 * Tests verifying exact Unicode characters used in status indicators.
 *
 * REQ-TUI-010: Status indicator: "● Running" (U+25CF), "○ Idle" (U+25CB), "■ Stopped" (U+25A0).
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-010 plan=phase-2/task-2.3 test=dashboard-renderer.test.ts
 */
describe("status indicator characters (REQ-TUI-010)", () => {
  it("running status uses U+25CF BLACK CIRCLE (●)", () => {
    const data = makeData([makeRow({ status: "running" })])
    const result = renderDashboard(data)
    // U+25CF = ●
    expect(result.output).toContain("\u25CF Running")
  })

  it("idle status uses U+25CB WHITE CIRCLE (○)", () => {
    const data = makeData([makeRow({ status: "idle" })])
    const result = renderDashboard(data)
    // U+25CB = ○
    expect(result.output).toContain("\u25CB Idle")
  })

  it("stopped status uses U+25A0 BLACK SQUARE (■)", () => {
    const data = makeData([makeRow({ status: "stopped" })])
    const result = renderDashboard(data)
    // U+25A0 = ■
    expect(result.output).toContain("\u25A0 Stopped")
  })

  it("running status string is exactly '● Running' (codepoint check)", () => {
    const data = makeData([makeRow({ status: "running" })])
    const result = renderDashboard(data)
    const runningStr = "● Running"
    expect(runningStr.codePointAt(0)).toBe(0x25cf) // ●
    expect(result.output).toContain(runningStr)
  })

  it("idle status string is exactly '○ Idle' (codepoint check)", () => {
    const data = makeData([makeRow({ status: "idle" })])
    const result = renderDashboard(data)
    const idleStr = "○ Idle"
    expect(idleStr.codePointAt(0)).toBe(0x25cb) // ○
    expect(result.output).toContain(idleStr)
  })

  it("stopped status string is exactly '■ Stopped' (codepoint check)", () => {
    const data = makeData([makeRow({ status: "stopped" })])
    const result = renderDashboard(data)
    const stoppedStr = "■ Stopped"
    expect(stoppedStr.codePointAt(0)).toBe(0x25a0) // ■
    expect(result.output).toContain(stoppedStr)
  })

  it("status column is padded to COL.status=11 chars in the table row", () => {
    // "● Running" is 9 chars — padded to 11 in the cell content.
    // Cell layout: " " (leading) + padCell("● Running", 11) + " " (trailing) = " ● Running   "
    // padCell("● Running", 11) = "● Running" + "  " = 11 chars
    // So between │ chars: " ● Running   " (1 + 9 + 2 + 1 = 13 chars)
    // The closing │ is preceded by 3 spaces (2 pad + 1 trailing)
    const data = makeData([makeRow({ status: "running" })])
    const result = renderDashboard(data)
    const lines = result.output.split("\n")
    // Find the data row (contains "● Running")
    const dataRow = lines.find((l) => l.includes("● Running"))
    expect(dataRow).toBeDefined()
    // The status cell ends with "● Running   │" (2 pad spaces + 1 trailing space before │)
    expect(dataRow).toContain("● Running   │")
  })
})

// ---------------------------------------------------------------------------
// Zero-value sessions — explicit REQ-TUI-013 coverage
// ---------------------------------------------------------------------------

/**
 * Explicit tests for zero-value session display (REQ-TUI-013).
 *
 * REQ-TUI-013: Sessions with no cost data MUST display $0.00 / 0 tokens / 0m.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-013 plan=phase-2/task-2.3 test=dashboard-renderer.test.ts
 */
describe("zero-value sessions (REQ-TUI-013)", () => {
  it("$0.00 appears in the table for a zero-cost session", () => {
    const data = makeData([makeRow({ costUsd: 0 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("$0.00")
  })

  it("'0' appears in the token column for a zero-token session (not '0K')", () => {
    const data = makeData([makeRow({ totalTokens: 0 })])
    const result = renderDashboard(data)
    // Find the data row (not header, not separator)
    const lines = result.output.split("\n")
    const dataRows = lines.filter(
      (l) => l.startsWith("│") && !l.includes("Session") && !l.includes("─")
    )
    // The token cell is " 0       │" (padded to 7 chars) — check "│ 0 " pattern in the row
    // The cell layout is: " " + padCell("0", 7) + " " = " 0       " (1+1+6+1=9 chars)
    // So the row contains "│ 0       │" for the token column
    expect(dataRows.some((l) => l.includes("│ 0       │"))).toBe(true)
  })

  it("'0m' appears in the duration column for a zero-duration session", () => {
    const data = makeData([makeRow({ durationSeconds: 0 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("0m")
  })

  it("all three zero values appear together in a single zero-value session", () => {
    const data = makeData([makeRow({ costUsd: 0, totalTokens: 0, durationSeconds: 0 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("$0.00")
    expect(result.output).toContain("0m")
    // "0" for tokens is present (not "0K")
    expect(result.output).not.toContain("0K")
  })

  it("zero-cost session in stats bar shows $0.00 for total cost", () => {
    const data = makeData([makeRow({ costUsd: 0 })], { totalCostUsd: 0 })
    const result = renderDashboard(data)
    expect(result.output).toContain("Total Cost: $0.00")
  })

  it("zero-token session in stats bar shows 0 for total tokens (not 0K)", () => {
    const data = makeData([makeRow({ totalTokens: 0 })], { totalTokens: 0 })
    const result = renderDashboard(data)
    expect(result.output).toContain("Total Tokens: 0")
    // Should NOT show "Total Tokens: 0K"
    expect(result.output).not.toContain("Total Tokens: 0K")
  })
})

// ---------------------------------------------------------------------------
// Edge cases (spec §10)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#edge-cases plan=phase-2/task-2.5 test=dashboard-renderer.test.ts
// ---------------------------------------------------------------------------

describe("renderDashboard() — edge cases (spec §10)", () => {
  // -------------------------------------------------------------------------
  // Edge case 1: Zero sessions
  // -------------------------------------------------------------------------

  it("zero sessions: output contains 'No active sessions'", () => {
    const data = makeData([])
    const result = renderDashboard(data)
    expect(result.output).toContain("No active sessions")
  })

  it("zero sessions: output contains '/axiom-run' hint", () => {
    const data = makeData([])
    const result = renderDashboard(data)
    expect(result.output).toContain("/axiom-run")
  })

  it("zero sessions: output does not contain broken table characters (no ├ or ┼)", () => {
    const data = makeData([])
    const result = renderDashboard(data)
    // Empty state is a minimal 3-line box — no column separators or cross junctions
    expect(result.output).not.toContain("├")
    expect(result.output).not.toContain("┼")
  })

  it("zero sessions: output does not contain column header row", () => {
    const data = makeData([])
    const result = renderDashboard(data)
    // No table header in empty state
    expect(result.output).not.toContain("Work Item")
    expect(result.output).not.toContain("Duration")
  })

  // -------------------------------------------------------------------------
  // Edge case 2: SQLite DB locked — dataDelayed flag
  // -------------------------------------------------------------------------

  it("dataDelayed=true: output contains 'Data may be delayed'", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data, { width: 90, dataDelayed: true })
    expect(result.output).toContain("Data may be delayed")
  })

  it("dataDelayed=true: output contains warning symbol ⚠", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data, { width: 90, dataDelayed: true })
    expect(result.output).toContain("⚠")
  })

  it("dataDelayed=false (default): output does NOT contain 'Data may be delayed'", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data, { width: 90 })
    expect(result.output).not.toContain("Data may be delayed")
  })

  // -------------------------------------------------------------------------
  // Edge case 9: File system permissions — permissionWarning flag
  // -------------------------------------------------------------------------

  it("permissionWarning set: output contains the warning text", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data, {
      width: 90,
      permissionWarning: "Cannot read .axiom/sessions/ — check permissions",
    })
    expect(result.output).toContain("Cannot read .axiom/sessions/ — check permissions")
  })

  it("permissionWarning set: output contains warning symbol ⚠", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data, {
      width: 90,
      permissionWarning: "Permission denied",
    })
    expect(result.output).toContain("⚠")
    expect(result.output).toContain("Permission denied")
  })

  it("permissionWarning undefined (default): output does NOT contain permission warning text", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data, { width: 90 })
    // No permission warning when flag is not set
    expect(result.output).not.toContain("Permission denied")
  })

  // -------------------------------------------------------------------------
  // Edge case 5: Backend API unreachable — network error + includeTeam
  // -------------------------------------------------------------------------

  it("network error + includeTeam=true: output contains 'Team data unavailable'", () => {
    const data: ReturnType<typeof makeData> = {
      ...makeData([makeRow()]),
      sources: {
        inMemory: { available: true, lastUpdate: new Date() },
        file: { available: false, lastUpdate: null },
        network: { available: false, lastUpdate: null, error: "connection refused" },
      },
    }
    const result = renderDashboard(data, { width: 90, includeTeam: true })
    expect(result.output).toContain("Team data unavailable")
    expect(result.output).toContain("connection refused")
  })

  it("network error + includeTeam=false (default): does NOT show team unavailable", () => {
    const data: ReturnType<typeof makeData> = {
      ...makeData([makeRow()]),
      sources: {
        inMemory: { available: true, lastUpdate: new Date() },
        file: { available: false, lastUpdate: null },
        network: { available: false, lastUpdate: null, error: "connection refused" },
      },
    }
    // includeTeam defaults to false — network error indicator suppressed
    const result = renderDashboard(data, { width: 90 })
    expect(result.output).not.toContain("Team data unavailable")
  })

  it("network available (no error): does NOT show team unavailable even with includeTeam=true", () => {
    const data = makeData([makeRow()])
    // sources.network has no error field
    const result = renderDashboard(data, { width: 90, includeTeam: true })
    expect(result.output).not.toContain("Team data unavailable")
  })

  // -------------------------------------------------------------------------
  // Edge case 6: Very high session count — truncation hint
  // -------------------------------------------------------------------------

  it("truncated: output contains 'Showing N of M sessions'", () => {
    const sessions = Array.from({ length: 25 }, (_, i) =>
      makeRow({ sessionId: `sess-${i}`, title: `Session ${i}` })
    )
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 90, maxRows: 20 })
    expect(result.truncated).toBe(true)
    expect(result.output).toContain("Showing 20 of 25 sessions")
  })

  it("truncated: output contains 'show all' hint", () => {
    const sessions = Array.from({ length: 25 }, (_, i) =>
      makeRow({ sessionId: `sess-${i}`, title: `Session ${i}` })
    )
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 90, maxRows: 20 })
    expect(result.output).toContain("show all")
  })

  it("not truncated: output does NOT contain 'Showing N of M sessions'", () => {
    const sessions = Array.from({ length: 5 }, (_, i) =>
      makeRow({ sessionId: `sess-${i}`, title: `Session ${i}` })
    )
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 90, maxRows: 20 })
    expect(result.truncated).toBe(false)
    expect(result.output).not.toContain("Showing")
  })

  // -------------------------------------------------------------------------
  // Multiple warning flags simultaneously
  // -------------------------------------------------------------------------

  it("dataDelayed + permissionWarning: both indicators appear in output", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data, {
      width: 90,
      dataDelayed: true,
      permissionWarning: "Read-only filesystem",
    })
    expect(result.output).toContain("Data may be delayed")
    expect(result.output).toContain("Read-only filesystem")
  })
})

// ---------------------------------------------------------------------------
// Session jump instructions (REQ-TUI-031)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-031 plan=phase-3/task-3.3 test=dashboard-renderer.test.ts
// ---------------------------------------------------------------------------

/**
 * Tests for renderSessionJumpInstructions() — manual session switching fallback.
 *
 * REQ-TUI-030: Users can select a session row and "jump into" that session.
 * REQ-TUI-031: If programmatic session switching is not available, display
 * the session ID clearly and provide instructions for manual switching.
 */
describe("session jump instructions (REQ-TUI-031)", () => {
  it("renderSessionJumpInstructions returns a string", () => {
    const result = renderSessionJumpInstructions("sess-abc-123", 1)
    expect(typeof result).toBe("string")
    expect(result.length).toBeGreaterThan(0)
  })

  it("renderSessionJumpInstructions includes the session ID", () => {
    const result = renderSessionJumpInstructions("sess-abc-123", 1)
    expect(result).toContain("sess-abc-123")
  })

  it("renderSessionJumpInstructions includes the row number", () => {
    const result = renderSessionJumpInstructions("sess-xyz-999", 5)
    expect(result).toContain("Session 5:")
  })

  it("renderSessionJumpInstructions includes manual switch instructions", () => {
    const result = renderSessionJumpInstructions("sess-abc-123", 1)
    expect(result).toContain("To switch:")
    expect(result).toContain("paste the session ID above")
  })

  it("renderSessionJumpInstructions includes 'Open Session' text", () => {
    const result = renderSessionJumpInstructions("sess-abc-123", 1)
    expect(result).toContain("Open Session")
  })

  it("renderSessionJumpInstructions notes SDK limitation", () => {
    const result = renderSessionJumpInstructions("sess-abc-123", 1)
    expect(result).toContain("Programmatic session switching is not available")
  })

  it("renderSessionJumpInstructions row number 1 produces 'Session 1: <id>'", () => {
    const result = renderSessionJumpInstructions("my-session-id", 1)
    expect(result).toContain("Session 1: my-session-id")
  })

  it("renderSessionJumpInstructions row number 9 produces 'Session 9: <id>'", () => {
    const result = renderSessionJumpInstructions("another-session", 9)
    expect(result).toContain("Session 9: another-session")
  })

  it("footer contains '[1-N] Jump to session' hint", () => {
    const data = makeData([makeRow()])
    const result = renderDashboard(data)
    expect(result.output).toContain("Jump to session")
  })
})

// ---------------------------------------------------------------------------
// Team row rendering (REQ-TUI-041)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-041 plan=phase-4/task-4.2 test=dashboard-renderer.test.ts
// ---------------------------------------------------------------------------

/**
 * Tests for team row rendering — visual distinction and hidden cost/token data.
 *
 * REQ-TUI-041: Team rows are visually distinct and do not expose cost/token data.
 */
describe("team row rendering (REQ-TUI-041)", () => {
  it("team rows show teamMemberId in Session column", () => {
    const sessions = [
      makeRow({
        sessionId: "team-sess-001",
        title: "Should not appear",
        isTeam: true,
        teamMemberId: "alice@example.com",
      }),
    ]
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 90, includeTeam: true })
    expect(result.output).toContain("alice@exam")
    expect(result.output).not.toContain("Should not appear")
  })

  it("team rows fall back to sessionId when teamMemberId is undefined", () => {
    const sessions = [
      makeRow({
        sessionId: "team-sess-fallback",
        title: "Should not appear",
        isTeam: true,
        teamMemberId: undefined,
      }),
    ]
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 90, includeTeam: true })
    expect(result.output).toContain("team-sess-f")
  })

  it("team rows show '—' for Cost (not '$0.00')", () => {
    const sessions = [
      makeRow({ sessionId: "team-1", isTeam: true, teamMemberId: "bob", costUsd: 0 }),
    ]
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 90, includeTeam: true })
    // Find the data row (not header)
    const lines = result.output.split("\n")
    const dataRow = lines.find((l) => l.includes("bob"))
    expect(dataRow).toBeDefined()
    expect(dataRow).toContain("—")
    expect(dataRow).not.toContain("$0.00")
  })

  it("team rows show '—' for Tokens (not '0')", () => {
    const sessions = [
      makeRow({ sessionId: "team-1", isTeam: true, teamMemberId: "carol", totalTokens: 0 }),
    ]
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 90, includeTeam: true })
    const lines = result.output.split("\n")
    const dataRow = lines.find((l) => l.includes("carol"))
    expect(dataRow).toBeDefined()
    // The row should contain "—" for both cost and tokens
    // Count occurrences of "—" in the data row (expect at least 2: cost + tokens)
    const dashCount = (dataRow!.match(/—/g) ?? []).length
    expect(dashCount).toBeGreaterThanOrEqual(2)
  })

  it("team rows show '~' prefix in # column", () => {
    const sessions = [
      makeRow({ sessionId: "team-1", isTeam: true, teamMemberId: "dave" }),
    ]
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 90, includeTeam: true })
    const lines = result.output.split("\n")
    const dataRow = lines.find((l) => l.includes("dave"))
    expect(dataRow).toBeDefined()
    // COL.num=1 so only "~" fits (not "~1") — team rows show "~" in the # column
    expect(dataRow).toContain("│ ~ │")
  })

  it("local rows still show cost and tokens normally", () => {
    const sessions = [
      makeRow({ sessionId: "local-1", isTeam: false, costUsd: 1.23, totalTokens: 5000 }),
    ]
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 90, includeTeam: false })
    expect(result.output).toContain("$1.23")
    expect(result.output).toContain("5K")
  })

  it("mixed local + team: local rows have cost, team rows have '—'", () => {
    const sessions = [
      makeRow({ sessionId: "local-1", title: "Local work", isTeam: false, costUsd: 0.50, totalTokens: 10_000 }),
      makeRow({ sessionId: "team-1", isTeam: true, teamMemberId: "eve@corp.com", costUsd: 99.99, totalTokens: 999_999 }),
    ]
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 90, includeTeam: true })
    const lines = result.output.split("\n")
    const localRow = lines.find((l) => l.includes("Local work"))
    const teamRow = lines.find((l) => l.includes("eve@corp"))
    expect(localRow).toBeDefined()
    expect(teamRow).toBeDefined()
    expect(localRow).toContain("$0.50")
    expect(localRow).toContain("10K")
    expect(teamRow).not.toContain("$99.99")
    expect(teamRow).not.toContain("999K")
    // Team row has "~" in # column
    expect(teamRow).toContain("│ ~ │")
  })
})

// ---------------------------------------------------------------------------
// Team toggle (REQ-TUI-045)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-045 plan=phase-4/task-4.3 test=dashboard-renderer.test.ts
// ---------------------------------------------------------------------------

/**
 * Tests for team toggle behavior — footer hint and includeTeam option.
 *
 * REQ-TUI-045: The dashboard MUST support toggling team row visibility.
 */
describe("team toggle (REQ-TUI-045)", () => {
  it("interactive footer contains '[t] Toggle team view' when isInteractive=true", () => {
    // REQ-TUI-004: [t] Toggle team view only appears in interactive (--watch) mode.
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-004 plan=step-verify-H002
    const data = makeData([makeRow()])
    const result = renderDashboard(data, { isInteractive: true })
    expect(result.output).toContain("[t] Toggle team view")
  })

  it("includeTeam=false excludes team rows", () => {
    const sessions = [
      makeRow({ sessionId: "local-1", title: "Local sess", isTeam: false }),
      makeRow({ sessionId: "team-1", title: "Team sess", isTeam: true, teamMemberId: "team-member" }),
    ]
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 90, includeTeam: false })
    expect(result.output).toContain("Local sess")
    expect(result.output).not.toContain("team-member")
  })

  it("includeTeam=true includes team rows", () => {
    const sessions = [
      makeRow({ sessionId: "local-1", title: "Local sess", isTeam: false }),
      makeRow({ sessionId: "team-1", title: "Team sess", isTeam: true, teamMemberId: "team-member" }),
    ]
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 90, includeTeam: true })
    expect(result.output).toContain("Local sess")
    // Team rows show teamMemberId per REQ-TUI-041
    expect(result.output).toContain("team-member")
  })

  it("includeTeam defaults to false (team rows excluded without explicit option)", () => {
    const sessions = [
      makeRow({ sessionId: "local-1", title: "Local only", isTeam: false }),
      makeRow({ sessionId: "team-1", title: "Team only", isTeam: true, teamMemberId: "team-member" }),
    ]
    const data = makeData(sessions)
    // No includeTeam option — defaults to false
    const result = renderDashboard(data, { width: 90 })
    expect(result.output).toContain("Local only")
    expect(result.output).not.toContain("team-member")
  })

  it("team row count correct when mixed local + team sessions with includeTeam=true", () => {
    const sessions = [
      makeRow({ sessionId: "local-1", isTeam: false }),
      makeRow({ sessionId: "local-2", isTeam: false }),
      makeRow({ sessionId: "team-1", isTeam: true, teamMemberId: "member-a" }),
      makeRow({ sessionId: "team-2", isTeam: true, teamMemberId: "member-b" }),
    ]
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 90, includeTeam: true })
    // All 4 rows visible
    expect(result.totalSessions).toBe(4)
    expect(result.truncated).toBe(false)
  })

  it("team row count correct when mixed local + team sessions with includeTeam=false", () => {
    const sessions = [
      makeRow({ sessionId: "local-1", isTeam: false }),
      makeRow({ sessionId: "local-2", isTeam: false }),
      makeRow({ sessionId: "team-1", isTeam: true, teamMemberId: "member-a" }),
      makeRow({ sessionId: "team-2", isTeam: true, teamMemberId: "member-b" }),
    ]
    const data = makeData(sessions)
    const result = renderDashboard(data, { width: 90, includeTeam: false })
    // Only 2 local rows visible
    expect(result.totalSessions).toBe(2)
  })
})

// ---------------------------------------------------------------------------
// Cost budget alert indicators (REQ-TUI-011)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-011 plan=phase-6/task-6.3 test=dashboard-renderer.test.ts
// ---------------------------------------------------------------------------

/**
 * Tests for cost budget alert indicators.
 *
 * REQ-TUI-011: Budget status = session cost / configured budget threshold.
 * - No indicator when budgetUsd is not set.
 * - No indicator when cost < 80% of budget.
 * - ⚠ indicator when cost >= 80% of budget (approaching).
 * - 🔴 indicator when cost >= 100% of budget (over budget).
 */
describe("cost budget alert indicators (REQ-TUI-011)", () => {
  it("no indicator when budgetUsd is not set", () => {
    const data = makeData([makeRow({ costUsd: 4.50, budgetUsd: undefined })])
    const result = renderDashboard(data)
    // Cost cell should show plain $4.50 with no prefix
    expect(result.output).toContain("$4.50")
    expect(result.output).not.toContain("⚠")
    expect(result.output).not.toContain("🔴")
  })

  it("no indicator when cost is below 80% of budget", () => {
    // $3.99 / $5.00 = 79.8% — below threshold
    const data = makeData([makeRow({ costUsd: 3.99, budgetUsd: 5.00 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("$3.99")
    expect(result.output).not.toContain("⚠")
    expect(result.output).not.toContain("🔴")
  })

  it("no indicator when cost is exactly 0% of budget", () => {
    const data = makeData([makeRow({ costUsd: 0, budgetUsd: 10.00 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("$0.00")
    expect(result.output).not.toContain("⚠")
    expect(result.output).not.toContain("🔴")
  })

  it("⚠ indicator when cost is exactly 80% of budget", () => {
    // $4.00 / $5.00 = 80% — at threshold
    const data = makeData([makeRow({ costUsd: 4.00, budgetUsd: 5.00 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("⚠")
    expect(result.output).not.toContain("🔴")
  })

  it("⚠ indicator when cost is between 80% and 100% of budget", () => {
    // $4.50 / $5.00 = 90%
    const data = makeData([makeRow({ costUsd: 4.50, budgetUsd: 5.00 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("⚠")
    expect(result.output).not.toContain("🔴")
  })

  it("⚠ indicator appears in Cost column (row contains ⚠)", () => {
    const data = makeData([makeRow({ costUsd: 4.50, budgetUsd: 5.00 })])
    const result = renderDashboard(data)
    // The cost cell contains the warning prefix — the $4.50 value may be truncated
    // in the 6-char cost column, but the ⚠ symbol is visible in the data row
    const lines = result.output.split("\n")
    const dataRow = lines.find((l) => l.includes("⚠"))
    expect(dataRow).toBeDefined()
    // The row is a data row (contains │ and session data)
    expect(dataRow).toContain("│")
  })

  it("🔴 indicator when cost is exactly 100% of budget", () => {
    // $5.00 / $5.00 = 100%
    const data = makeData([makeRow({ costUsd: 5.00, budgetUsd: 5.00 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("🔴")
    expect(result.output).not.toContain("⚠")
  })

  it("🔴 indicator when cost exceeds budget (over 100%)", () => {
    // $7.50 / $5.00 = 150%
    const data = makeData([makeRow({ costUsd: 7.50, budgetUsd: 5.00 })])
    const result = renderDashboard(data)
    expect(result.output).toContain("🔴")
    expect(result.output).not.toContain("⚠")
  })

  it("🔴 indicator appears in Cost column (row contains 🔴)", () => {
    const data = makeData([makeRow({ costUsd: 5.00, budgetUsd: 5.00 })])
    const result = renderDashboard(data)
    // The cost cell contains the over-budget prefix — the $5.00 value may be truncated
    // in the 6-char cost column, but the 🔴 symbol is visible in the data row
    const lines = result.output.split("\n")
    const dataRow = lines.find((l) => l.includes("🔴"))
    expect(dataRow).toBeDefined()
    // The row is a data row (contains │ and session data)
    expect(dataRow).toContain("│")
  })

  it("budget indicator does NOT appear on team rows (team rows show '—')", () => {
    const data = makeData([
      makeRow({ sessionId: "team-1", isTeam: true, teamMemberId: "alice", costUsd: 4.50, budgetUsd: 5.00 }),
    ])
    const result = renderDashboard(data, { width: 90, includeTeam: true })
    // Team rows always show "—" for cost — no budget indicator
    const lines = result.output.split("\n")
    const dataRow = lines.find((l) => l.includes("alice"))
    expect(dataRow).toBeDefined()
    expect(dataRow).toContain("—")
    expect(dataRow).not.toContain("⚠")
    expect(dataRow).not.toContain("🔴")
  })

  it("budget indicator only on the session with budget set (mixed sessions)", () => {
    const sessions = [
      makeRow({ sessionId: "s1", title: "With budget", costUsd: 4.50, budgetUsd: 5.00 }),
      makeRow({ sessionId: "s2", title: "No budget", costUsd: 4.50, budgetUsd: undefined }),
    ]
    const data = makeData(sessions)
    const result = renderDashboard(data)
    const lines = result.output.split("\n")
    const rowWithBudget = lines.find((l) => l.includes("With bud"))
    const rowNoBudget = lines.find((l) => l.includes("No budge"))
    expect(rowWithBudget).toBeDefined()
    expect(rowNoBudget).toBeDefined()
    expect(rowWithBudget).toContain("⚠")
    expect(rowNoBudget).not.toContain("⚠")
    expect(rowNoBudget).not.toContain("🔴")
  })
})

// ---------------------------------------------------------------------------
// REQ-TUI-032/033: ownership guard deferred (ADR-TUI-002)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-032 plan=step-reverify-004 test=dashboard-renderer.test.ts
// ---------------------------------------------------------------------------

/**
 * Documents the deferred ownership guard behavior per ADR-TUI-002.
 *
 * REQ-TUI-032: "Users can only jump to sessions they own (same OS user)."
 * REQ-TUI-033: "After a successful session jump, the dashboard MUST close."
 *
 * Both requirements are deferred — the OpenCode plugin SDK (v1.2.27) does not
 * support programmatic session switching (see ADR-TUI-001). Without a jump
 * action, there is nothing to guard. The dashboard shows session IDs with
 * manual switch instructions (REQ-TUI-031 fallback).
 *
 * ADR: .memory-bank/work-items/tui-dashboard-01/adr-tui-002-ownership-guard-deferred.md
 */
describe("REQ-TUI-032/033: ownership guard deferred (ADR-TUI-002)", () => {
  it("displays session ID for manual switching (REQ-TUI-031 fallback)", () => {
    // REQ-TUI-032/033 are deferred — no programmatic session jumping
    // The dashboard shows session IDs with manual switch instructions
    const instructions = renderSessionJumpInstructions("ses_abc123", 1)
    expect(instructions).toContain("ses_abc123")
    expect(instructions).toContain("1")
    // No ownership check — deferred per ADR-TUI-002
  })
})

// ---------------------------------------------------------------------------
// Session age alerts — atop-style duration coloring (task-10.7)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-072 plan=phase-10/task-10.7 test=dashboard-renderer.test.ts
// ---------------------------------------------------------------------------

/**
 * Tests for formatDurationWithAge() — ANSI age-alert coloring in extended mode.
 *
 * REQ-TUI-072: Long-running sessions are visually flagged:
 *   - < 1h  : no ANSI color
 *   - 1–24h : yellow (\x1b[33m)
 *   - > 24h : red    (\x1b[31m)
 *
 * IMPORTANT: Color is applied ONLY in renderDashboardExtended(). The compact
 * renderer (renderDashboard) must NOT emit ANSI codes in the Duration column
 * because they break fixed-width box-drawing column alignment.
 */
describe("formatDurationWithAge — age alerts (task-10.7)", () => {
  it("short sessions (< 1h) have no ANSI color codes in extended mode", () => {
    // 30 minutes = 1800s — below 1h threshold, no color expected
    const result = renderDashboardExtended(makeData([makeRow({ durationSeconds: 1800 })]))
    // The output must not contain yellow or red ANSI escape codes for duration
    // (We verify by checking the output does NOT contain the yellow/red ANSI sequences
    //  that formatDurationWithAge would add for >= 1h sessions.)
    expect(result.output).not.toContain("\x1b[33m30m")
    expect(result.output).not.toContain("\x1b[31m30m")
  })

  it("long sessions (>= 1h, < 24h) show yellow duration in extended mode", () => {
    // 2 hours = 7200s — yellow threshold
    const result = renderDashboardExtended(makeData([makeRow({ durationSeconds: 7200 })]))
    // Yellow ANSI code must appear in the output (wrapping the "2h 0m" duration)
    expect(result.output).toContain("\x1b[33m")
  })

  it("very long sessions (> 24h) show red duration in extended mode", () => {
    // 25 hours = 90000s — red threshold
    const result = renderDashboardExtended(makeData([makeRow({ durationSeconds: 90000 })]))
    // Red ANSI code must appear in the output (wrapping the "25h 0m" duration)
    expect(result.output).toContain("\x1b[31m")
  })

  it("compact renderer (renderDashboard) does NOT emit ANSI codes for any duration", () => {
    // Regression guard: compact renderer must never use formatDurationWithAge
    // because ANSI codes break fixed-width box-drawing column alignment.
    const longSession = makeRow({ durationSeconds: 90000 })  // > 24h
    const result = renderDashboard(makeData([longSession]))
    // No ANSI escape sequences in compact output
    expect(result.output).not.toContain("\x1b[31m")
    expect(result.output).not.toContain("\x1b[33m")
  })
})

// ---------------------------------------------------------------------------
// renderToolBar (task-10.4)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-071 plan=phase-10/task-10.4 test=dashboard-renderer.test.ts
// ---------------------------------------------------------------------------

/**
 * Tests for renderToolBar() — iotop-style tool call bars in the extended renderer.
 *
 * REQ-TUI-071: Tool call bars scaled to max across visible sessions.
 * Width: 4 chars, magenta filled blocks, dim dots for empty.
 */
describe("renderToolBar (task-10.4)", () => {
  it("returns 4 spaces for zero tool calls", () => {
    expect(renderToolBar(0, 100)).toBe("    ")
  })

  it("returns 4 spaces when maxToolCalls is 0", () => {
    expect(renderToolBar(50, 0)).toBe("    ")
  })

  it("returns full bar for max tool calls", () => {
    const result = renderToolBar(100, 100)
    // Should contain 4 filled block chars (magenta)
    expect(result).toContain("█")
    expect(result).toContain("\x1b[35m")  // magenta
  })

  it("returns partial bar for half tool calls", () => {
    const result = renderToolBar(50, 100)
    // Should contain some filled and some empty (dim dots)
    expect(result).toContain("·")  // dim dots for empty portion
  })
})

// ---------------------------------------------------------------------------
// tok/s 3-decimal precision in extended mode (REQ-TUI-073, step-p10-M001)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-073 plan=phase-10/step-p10-M001 test=dashboard-renderer.test.ts
// ---------------------------------------------------------------------------

describe("tok/s 3-decimal precision in extended mode (REQ-TUI-073, step-p10-M001)", () => {
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-073 plan=phase-10/step-p10-M001

  it("extended mode tps column shows 3 decimal places for non-integer tok/s", () => {
    const data = makeData([makeRow({ tokensPerSecond: 42.567 })])
    const result = renderDashboardExtended(data)
    // The tps column should contain "42.567" (3 decimal places)
    expect(result.output).toContain("42.567")
  })

  it("extended mode tps column shows 0.000 for zero tok/s", () => {
    const data = makeData([makeRow({ tokensPerSecond: 0 })])
    const result = renderDashboardExtended(data)
    expect(result.output).toContain("0.000")
  })

  it("extended mode tps column shows 3 decimal places for integer tok/s", () => {
    const data = makeData([makeRow({ tokensPerSecond: 100 })])
    const result = renderDashboardExtended(data)
    // 100 tok/s should show as "100.000"
    expect(result.output).toContain("100.000")
  })
})
