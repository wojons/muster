/**
 * Unit tests for htop-header.ts utility functions:
 *   - extractWorktree(directory: string): string
 *   - renderSparkline(value: number, maxValue: number): string
 *   - renderSparklineHistory(samples: number[], maxValue: number): string
 *
 * REQ-TUI-110: htop-style header meters with per-model token usage bars.
 * REQ-TUI-120: Multi-char sparkline history in watch mode.
 *
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-110
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-110 plan=phase-9/task-9.5
 */

import { describe, it, expect } from "bun:test"
import { extractWorktree, renderSparkline, renderSparklineHistory, aggregateByProvider, groupByWorktree } from "../src/dashboard/htop-header"

// ---------------------------------------------------------------------------
// extractWorktree
// ---------------------------------------------------------------------------

describe("extractWorktree", () => {
  // Standard worktree paths
  it("extracts worktree name from .worktrees/ path", () => {
    expect(extractWorktree("/Users/foo/code/Axiom/.worktrees/tui-dashboard")).toBe("tui-dashboard")
  })

  it("extracts worktree name: wave", () => {
    expect(extractWorktree("/Users/foo/code/Axiom/.worktrees/wave")).toBe("wave")
  })

  it("extracts worktree name: ci-quality", () => {
    expect(extractWorktree("/Users/foo/code/Axiom/.worktrees/ci-quality")).toBe("ci-quality")
  })

  // Non-worktree paths
  it("returns last segment for non-worktree paths", () => {
    expect(extractWorktree("/Users/foo/code/Axiom")).toBe("Axiom")
  })

  it("returns last segment for project paths", () => {
    expect(extractWorktree("/Users/foo/code/recall")).toBe("recall")
  })

  // Edge cases
  it("returns — for empty string", () => {
    expect(extractWorktree("")).toBe("—")
  })

  it("handles nested worktree path (e.g., .worktrees/wave/.axiom)", () => {
    // Regex \.worktrees\/([^/]+) captures 'wave' — stops at the next slash
    const result = extractWorktree("/Users/foo/code/Axiom/.worktrees/wave/.axiom")
    expect(result).toBe("wave")
  })
})

// ---------------------------------------------------------------------------
// renderSparkline
// ---------------------------------------------------------------------------

describe("renderSparkline", () => {
  it("returns dim ▏ when value is 0", () => {
    const result = renderSparkline(0, 100)
    expect(result).toContain("▏")
  })

  it("returns dim ▏ when maxValue is 0", () => {
    const result = renderSparkline(50, 0)
    expect(result).toContain("▏")
  })

  it("returns a string containing a block character for non-zero values", () => {
    const result = renderSparkline(50, 100)
    const blockChars = ["▏", "▎", "▍", "▌", "▋", "▊", "▉", "█"]
    expect(blockChars.some(c => result.includes(c))).toBe(true)
  })

  it("returns full block █ for value equal to maxValue", () => {
    const result = renderSparkline(100, 100)
    expect(result).toContain("█")
  })

  it("returns a string (ANSI codes + block char)", () => {
    const result = renderSparkline(75, 100)
    expect(typeof result).toBe("string")
    expect(result.length).toBeGreaterThan(0)
  })

  it("high value (>60% of max) contains green ANSI code", () => {
    const result = renderSparkline(80, 100)
    expect(result).toContain("\x1b[32m")  // green
  })

  it("low value (<30% of max) contains red ANSI code", () => {
    const result = renderSparkline(10, 100)
    expect(result).toContain("\x1b[31m")  // red
  })

  it("medium value (30-60% of max) contains yellow ANSI code", () => {
    const result = renderSparkline(45, 100)
    expect(result).toContain("\x1b[33m")  // yellow
  })
})

// ---------------------------------------------------------------------------
// renderHtopHeader — reasoning segment regression (INJ-2)
// ---------------------------------------------------------------------------

import { renderHtopHeader } from "../src/dashboard/htop-header"
import type { ModelUsage, DashboardAggregates, SessionRow } from "../src/dashboard/types"

describe("renderHtopHeader — reasoning segment (REQ-TUI-100)", () => {
  const makeModelUsage = (overrides: Partial<ModelUsage> = {}): ModelUsage => ({
    model: "Claude Sonnet 4.6",
    modelRaw: "anthropic.claude-sonnet-4-6",
    provider: "amazon-bedrock",
    sessions: 1,
    calls: 10,
    costUsd: 1.5,
    outputTokens: 5000,
    inputTokens: 1000,
    cacheReadTokens: 3000,
    reasoningTokens: 0,
    ...overrides,
  })

  const makeAggregates = (): DashboardAggregates => ({
    totalSessions: 1,
    totalCostUsd: 1.5,
    totalTokens: 9000,
    avgSpeedTokensPerSec: 10,
  })

  it("INJ-2: yellow bgYellow segment NOT rendered in bar when reasoningTokens=0 (REQ-TUI-100)", () => {
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-100 plan=F-003/zero-reasoning
    const usage = [makeModelUsage({ reasoningTokens: 0 })]
    const result = renderHtopHeader(usage, makeAggregates(), [])
    // The legend line always shows bgYellow — check only the model bar lines
    // Bar lines contain "calls" and "out" text; legend line contains "reasoning"
    const lines = result.split("\n")
    const barLines = lines.filter((l) => l.includes("calls") && l.includes("out"))
    // Bar lines should not contain bgYellow (\x1b[43m) since reasoningTokens=0
    for (const line of barLines) {
      expect(line).not.toContain("\x1b[43m")
    }
  })

  it("yellow bgYellow segment IS rendered when reasoningTokens > 0 (REQ-TUI-100)", () => {
    const usage = [makeModelUsage({ reasoningTokens: 2000 })]
    const result = renderHtopHeader(usage, makeAggregates(), [])
    // bgYellow = \x1b[43m — should appear when reasoningTokens > 0
    expect(result).toContain("\x1b[43m")
  })

  it("returns empty string when modelUsage is empty", () => {
    const result = renderHtopHeader([], makeAggregates(), [])
    expect(result).toBe("")
  })
})

// ---------------------------------------------------------------------------
// aggregateByProvider (task-10.3)
// ---------------------------------------------------------------------------

describe("aggregateByProvider (task-10.3)", () => {
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-100 plan=phase-10/task-10.3
  const makeModelUsage = (overrides: Partial<ModelUsage> = {}): ModelUsage => ({
    model: "Claude Sonnet 4.6",
    modelRaw: "anthropic.claude-sonnet-4-6",
    provider: "amazon-bedrock",
    sessions: 1,
    calls: 10,
    costUsd: 1.5,
    outputTokens: 5000,
    inputTokens: 1000,
    cacheReadTokens: 3000,
    reasoningTokens: 0,
    ...overrides,
  })

  it("aggregates multiple models from same provider", () => {
    const models = [
      makeModelUsage({ provider: "amazon-bedrock", costUsd: 10, calls: 5 }),
      makeModelUsage({ provider: "amazon-bedrock", costUsd: 20, calls: 3 }),
      makeModelUsage({ provider: "openai", costUsd: 5, calls: 2 }),
    ]
    const providers = aggregateByProvider(models)
    expect(providers).toHaveLength(2)
    const bedrock = providers.find(p => p.provider === "amazon-bedrock")!
    expect(bedrock.costUsd).toBeCloseTo(30)
    expect(bedrock.calls).toBe(8)
  })

  it("sorts providers by cost descending", () => {
    const models = [
      makeModelUsage({ provider: "openai", costUsd: 5 }),
      makeModelUsage({ provider: "amazon-bedrock", costUsd: 100 }),
    ]
    const providers = aggregateByProvider(models)
    expect(providers[0]!.provider).toBe("amazon-bedrock")
    expect(providers[1]!.provider).toBe("openai")
  })

  it("handles empty modelUsage", () => {
    expect(aggregateByProvider([])).toHaveLength(0)
  })

  it("uses 'unknown' for missing provider", () => {
    const models = [makeModelUsage({ provider: "" })]
    const providers = aggregateByProvider(models)
    expect(providers[0]!.provider).toBe("unknown")
  })
})

// ---------------------------------------------------------------------------
// renderSparklineHistory (task-10.5)
// ---------------------------------------------------------------------------

/**
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-120 plan=phase-10/task-10.5
 */
describe("renderSparklineHistory (task-10.5)", () => {
  it("returns empty string for empty samples", () => {
    expect(renderSparklineHistory([], 100)).toBe("")
  })

  it("returns a string with one char per sample", () => {
    const result = renderSparklineHistory([10, 50, 100], 100)
    // 3 samples → 3 block chars (each wrapped in ANSI codes)
    // Count block chars by stripping ANSI
    const stripped = result.replace(/\u001b\[[0-9;]*m/g, "")
    expect(stripped).toHaveLength(3)
  })

  it("high values use green color", () => {
    const result = renderSparklineHistory([100], 100)
    expect(result).toContain("\x1b[32m")  // green
  })

  it("low values use red color", () => {
    const result = renderSparklineHistory([10], 100)
    expect(result).toContain("\x1b[31m")  // red
  })

  it("last sample is bold", () => {
    const result = renderSparklineHistory([50, 80], 100)
    expect(result).toContain("\x1b[1m")  // bold for last sample
  })
})

// ---------------------------------------------------------------------------
// groupByWorktree (task-10.6)
// ---------------------------------------------------------------------------

describe("groupByWorktree (task-10.6)", () => {
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-110 plan=phase-10/task-10.6

  /** Build a minimal valid SessionRow for groupByWorktree tests. */
  const makeSessionRow = (overrides: Partial<SessionRow> = {}): SessionRow => ({
    sessionId: "sess-001",
    title: "Test session",
    model: "Sonnet",
    modelRaw: "anthropic.claude-sonnet-4-6",
    costUsd: 1.0,
    totalTokens: 10_000,
    durationSeconds: 300,
    status: "running",
    source: "sqlite",
    updatedAt: new Date("2026-04-13T10:00:00.000Z"),
    isTeam: false,
    ...overrides,
  })

  it("groups sessions by worktree", () => {
    const sessions = [
      makeSessionRow({ sessionId: "s1", directory: "/code/.worktrees/tui-dashboard", costUsd: 10 }),
      makeSessionRow({ sessionId: "s2", directory: "/code/.worktrees/tui-dashboard", costUsd: 20 }),
      makeSessionRow({ sessionId: "s3", directory: "/code/.worktrees/wave", costUsd: 5 }),
    ]
    const groups = groupByWorktree(sessions)
    expect(groups).toHaveLength(2)
    const tui = groups.find(g => g.worktree === "tui-dashboard")!
    expect(tui).toBeDefined()
    expect(tui.sessionCount).toBe(2)
    expect(tui.totalCostUsd).toBeCloseTo(30)
  })

  it("sorts groups by total cost descending", () => {
    const sessions = [
      makeSessionRow({ sessionId: "s1", directory: "/code/.worktrees/cheap", costUsd: 1 }),
      makeSessionRow({ sessionId: "s2", directory: "/code/.worktrees/expensive", costUsd: 100 }),
    ]
    const groups = groupByWorktree(sessions)
    expect(groups[0]!.worktree).toBe("expensive")
    expect(groups[1]!.worktree).toBe("cheap")
  })

  it("handles sessions without directory", () => {
    const sessions = [makeSessionRow({ sessionId: "s1", directory: undefined })]
    const groups = groupByWorktree(sessions)
    expect(groups).toHaveLength(1)
    expect(groups[0]!.worktree).toBe("—")
  })
})

// ---------------------------------------------------------------------------
// renderHtopHeader — 24h session warning (REQ-TUI-171, step-p10-M002)
// ---------------------------------------------------------------------------

describe("renderHtopHeader — 24h session warning (REQ-TUI-171, step-p10-M002)", () => {
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-171 plan=phase-10/step-p10-M002

  const makeModelUsage = (overrides: Partial<ModelUsage> = {}): ModelUsage => ({
    model: "Claude Sonnet 4.6",
    modelRaw: "anthropic.claude-sonnet-4-6",
    provider: "amazon-bedrock",
    sessions: 1,
    calls: 10,
    costUsd: 1.5,
    outputTokens: 5000,
    inputTokens: 1000,
    cacheReadTokens: 3000,
    reasoningTokens: 0,
    ...overrides,
  })

  const makeAggregates = (): DashboardAggregates => ({
    totalSessions: 1,
    totalCostUsd: 1.5,
    totalTokens: 9000,
    avgSpeedTokensPerSec: 10,
  })

  const makeSessionRow = (overrides: Partial<SessionRow> = {}): SessionRow => ({
    sessionId: "sess-001",
    title: "Test session",
    model: "Sonnet",
    modelRaw: "anthropic.claude-sonnet-4-6",
    costUsd: 1.0,
    totalTokens: 10_000,
    durationSeconds: 300,
    status: "running",
    source: "sqlite",
    updatedAt: new Date("2026-04-13T10:00:00.000Z"),
    isTeam: false,
    ...overrides,
  })

  it("shows red ⚠ warning when any session is > 24h old", () => {
    const usage = [makeModelUsage()]
    const agg = makeAggregates()
    const sessions = [
      { ...makeSessionRow(), durationSeconds: 25 * 3600 },  // 25 hours
    ]
    const result = renderHtopHeader(usage, agg, sessions as any)
    expect(result).toContain("⚠")
    expect(result).toContain("24h")
    expect(result).toContain("\x1b[31m")  // red ANSI
  })

  it("does NOT show warning when all sessions are < 24h", () => {
    const usage = [makeModelUsage()]
    const agg = makeAggregates()
    const sessions = [
      { ...makeSessionRow(), durationSeconds: 23 * 3600 },  // 23 hours
    ]
    const result = renderHtopHeader(usage, agg, sessions as any)
    // Should not contain the 24h warning
    const lines = result.split("\n")
    const warningLine = lines.find(l => l.includes("24h") && l.includes("⚠"))
    expect(warningLine).toBeUndefined()
  })

  it("shows correct count when multiple sessions are > 24h", () => {
    const usage = [makeModelUsage()]
    const agg = makeAggregates()
    const sessions = [
      { ...makeSessionRow(), durationSeconds: 25 * 3600 },
      { ...makeSessionRow(), durationSeconds: 30 * 3600 },
      { ...makeSessionRow(), durationSeconds: 1 * 3600 },  // not > 24h
    ]
    const result = renderHtopHeader(usage, agg, sessions as any)
    expect(result).toContain("2 sessions")  // 2 sessions > 24h
  })
})
