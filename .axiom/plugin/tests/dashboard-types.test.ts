/**
 * Tests for TUI Dashboard type definitions.
 *
 * Verifies that all interfaces compile correctly and are correctly shaped.
 * These are structural/compile-time tests — they prove the type contract
 * is sound and that SessionState is correctly imported (not redefined).
 *
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-010
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-010 plan=phase-1/task-1.1 test=dashboard-types.test.ts
 */

import { describe, it, expect } from "bun:test"
import type {
  SessionRow,
  DashboardAggregates,
  SourceStatus,
  DashboardData,
  SessionDataSource,
  CollectorOptions,
  SessionState,
} from "../src/dashboard/types"

// ---------------------------------------------------------------------------
// Compile-time shape checks via assignability
// ---------------------------------------------------------------------------

describe("SessionRow interface", () => {
  it("accepts a fully-populated SessionRow", () => {
    const row: SessionRow = {
      sessionId: "sess-abc-123",
      title: "Fix auth bug",
      workItemId: "AUTH-234",
      model: "Sonnet",
      modelRaw: "anthropic.claude-sonnet-4-6",
      costUsd: 0.042,
      totalTokens: 12_500,
      durationSeconds: 300,
      status: "running",
      source: "in-memory",
      updatedAt: new Date("2026-04-12T10:00:00Z"),
      isTeam: false,
      budgetUsd: 5.0,
    }
    expect(row.sessionId).toBe("sess-abc-123")
    expect(row.status).toBe("running")
    expect(row.source).toBe("in-memory")
    expect(row.isTeam).toBe(false)
  })

  it("accepts a minimal SessionRow (optional fields omitted)", () => {
    const row: SessionRow = {
      sessionId: "sess-min-001",
      model: "Opus 4",
      modelRaw: "anthropic.claude-opus-4",
      costUsd: 0,
      totalTokens: 0,
      durationSeconds: 0,
      status: "stopped",
      source: "file",
      updatedAt: new Date(),
      isTeam: false,
    }
    expect(row.sessionId).toBe("sess-min-001")
    expect(row.title).toBeUndefined()
    expect(row.workItemId).toBeUndefined()
    expect(row.budgetUsd).toBeUndefined()
    expect(row.teamMemberId).toBeUndefined()
  })

  it("accepts a team member SessionRow", () => {
    const row: SessionRow = {
      sessionId: "sess-team-007",
      model: "Sonnet",
      modelRaw: "anthropic.claude-sonnet-4-6",
      costUsd: 0,
      totalTokens: 0,
      durationSeconds: 120,
      status: "idle",
      source: "network",
      updatedAt: new Date(),
      isTeam: true,
      teamMemberId: "alice@example.com",
    }
    expect(row.isTeam).toBe(true)
    expect(row.teamMemberId).toBe("alice@example.com")
  })

  it("enforces the source discriminator union", () => {
    const sources: Array<SessionRow["source"]> = ["in-memory", "file", "network"]
    expect(sources).toHaveLength(3)
    expect(sources).toContain("in-memory")
    expect(sources).toContain("file")
    expect(sources).toContain("network")
  })
})

// ---------------------------------------------------------------------------
// DashboardAggregates
// ---------------------------------------------------------------------------

describe("DashboardAggregates interface", () => {
  it("accepts a valid aggregates object", () => {
    const agg: DashboardAggregates = {
      totalSessions: 5,
      totalCostUsd: 1.23,
      totalTokens: 50_000,
      avgSpeedTokensPerSec: 42.7,
    }
    expect(agg.totalSessions).toBe(5)
    expect(agg.totalCostUsd).toBe(1.23)
    expect(agg.totalTokens).toBe(50_000)
    expect(agg.avgSpeedTokensPerSec).toBe(42.7)
  })
})

// ---------------------------------------------------------------------------
// SourceStatus
// ---------------------------------------------------------------------------

describe("SourceStatus interface", () => {
  it("accepts an available source with a lastUpdate", () => {
    const status: SourceStatus = {
      available: true,
      lastUpdate: new Date("2026-04-12T09:55:00Z"),
    }
    expect(status.available).toBe(true)
    expect(status.lastUpdate).toBeInstanceOf(Date)
    expect(status.error).toBeUndefined()
  })

  it("accepts an unavailable source with null lastUpdate and error", () => {
    const status: SourceStatus = {
      available: false,
      lastUpdate: null,
      error: "Connection refused",
    }
    expect(status.available).toBe(false)
    expect(status.lastUpdate).toBeNull()
    expect(status.error).toBe("Connection refused")
  })
})

// ---------------------------------------------------------------------------
// DashboardData
// ---------------------------------------------------------------------------

describe("DashboardData interface", () => {
  it("has sessions, aggregates, sources, and generatedAt", () => {
    const data: DashboardData = {
      sessions: [],
      aggregates: {
        totalSessions: 0,
        totalCostUsd: 0,
        totalTokens: 0,
        avgSpeedTokensPerSec: 0,
      },
      sources: {
        inMemory: { available: true, lastUpdate: null },
        file: { available: false, lastUpdate: null, error: "No session files found" },
        network: { available: false, lastUpdate: null },
      },
      generatedAt: new Date("2026-04-12T10:00:00Z"),
    }
    expect(data.sessions).toBeArray()
    expect(data.aggregates.totalSessions).toBe(0)
    expect(data.sources.inMemory.available).toBe(true)
    expect(data.sources.file.error).toBe("No session files found")
    expect(data.sources.network.available).toBe(false)
    expect(data.generatedAt).toBeInstanceOf(Date)
  })
})

// ---------------------------------------------------------------------------
// SessionDataSource interface
// ---------------------------------------------------------------------------

describe("SessionDataSource interface", () => {
  it("can be implemented as a mock in-memory source", async () => {
    const mockSource: SessionDataSource = {
      id: "in-memory",
      priority: 3,
      getSessions: async () => [],
      getLastUpdate: () => null,
      isAvailable: () => true,
    }
    expect(mockSource.id).toBe("in-memory")
    expect(mockSource.priority).toBe(3)
    expect(mockSource.isAvailable()).toBe(true)
    expect(mockSource.getLastUpdate()).toBeNull()
    const sessions = await mockSource.getSessions()
    expect(sessions).toBeArray()
    expect(sessions).toHaveLength(0)
  })

  it("can be implemented as a mock file source", async () => {
    const mockSource: SessionDataSource = {
      id: "file",
      priority: 2,
      getSessions: async () => [],
      getLastUpdate: () => new Date("2026-04-12T09:00:00Z"),
      isAvailable: () => false,
    }
    expect(mockSource.id).toBe("file")
    expect(mockSource.priority).toBe(2)
    expect(mockSource.isAvailable()).toBe(false)
    expect(mockSource.getLastUpdate()).toBeInstanceOf(Date)
  })

  it("can be implemented as a mock network source", async () => {
    const mockSource: SessionDataSource = {
      id: "network",
      priority: 1,
      getSessions: async () => [],
      getLastUpdate: () => null,
      isAvailable: () => false,
    }
    expect(mockSource.id).toBe("network")
    expect(mockSource.priority).toBe(1)
  })

  it("enforces the id discriminator union", () => {
    const ids: Array<SessionDataSource["id"]> = ["in-memory", "file", "network"]
    expect(ids).toHaveLength(3)
  })
})

// ---------------------------------------------------------------------------
// CollectorOptions interface
// ---------------------------------------------------------------------------

describe("CollectorOptions interface", () => {
  it("accepts minimal options (repoRoot only)", () => {
    const opts: CollectorOptions = {
      repoRoot: "/Users/aokuwa/code/Axiom/.worktrees/tui-dashboard",
    }
    expect(opts.repoRoot).toContain("tui-dashboard")
    expect(opts.includeTeam).toBeUndefined()
    expect(opts.sources).toBeUndefined()
  })

  it("accepts full options with injected sources", () => {
    const mockSource: SessionDataSource = {
      id: "in-memory",
      priority: 3,
      getSessions: async () => [],
      getLastUpdate: () => null,
      isAvailable: () => true,
    }
    const opts: CollectorOptions = {
      repoRoot: "/repo",
      includeTeam: true,
      sources: [mockSource],
    }
    expect(opts.includeTeam).toBe(true)
    expect(opts.sources).toHaveLength(1)
    expect(opts.sources![0].id).toBe("in-memory")
  })
})

// ---------------------------------------------------------------------------
// SessionState — verify it is correctly re-exported (not redefined)
// ---------------------------------------------------------------------------

describe("SessionState type (re-exported from sink-interface)", () => {
  it("covers all three lifecycle states", () => {
    const states: SessionState[] = ["running", "idle", "stopped"]
    expect(states).toHaveLength(3)
    expect(states).toContain("running")
    expect(states).toContain("idle")
    expect(states).toContain("stopped")
  })

  it("is assignable to SessionRow.status", () => {
    const state: SessionState = "running"
    const row: SessionRow = {
      sessionId: "sess-state-test",
      model: "Sonnet",
      modelRaw: "anthropic.claude-sonnet-4-6",
      costUsd: 0,
      totalTokens: 0,
      durationSeconds: 0,
      status: state,
      source: "in-memory",
      updatedAt: new Date(),
      isTeam: false,
    }
    expect(row.status).toBe("running")
  })
})
