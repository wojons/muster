/**
 * Tests for SessionDataCollector — merges sources, deduplicates by session ID,
 * computes aggregate stats, and handles source errors gracefully.
 *
 * All tests use mock SessionDataSource implementations (not real InMemorySource
 * or FileSource) to keep tests fast and fully isolated.
 *
 * REQ-TUI-003: The command MUST display an aggregate stats bar showing:
 *   total active sessions, total cost, total tokens, average speed.
 * REQ-TUI-011: The table MUST compute derived fields at render time.
 *
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-003
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-011
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-003 plan=phase-1/task-1.4 test=dashboard-collector.test.ts
 */

import { describe, it, expect } from "bun:test"
import { SessionDataCollector, createSessionDataCollector } from "../src/dashboard/collector"
import type { SessionDataSource, SessionRow, CollectorOptions } from "../src/dashboard/types"

// ---------------------------------------------------------------------------
// Test helpers / fixtures
// ---------------------------------------------------------------------------

/** Build a minimal valid SessionRow for testing. */
function makeRow(overrides: Partial<SessionRow> = {}): SessionRow {
  return {
    sessionId: "sess-001",
    model: "Sonnet 4",
    modelRaw: "claude-sonnet-4-20250514",
    costUsd: 0.10,
    totalTokens: 1000,
    durationSeconds: 60,
    status: "running",
    source: "in-memory",
    updatedAt: new Date("2026-04-12T10:00:00.000Z"),
    isTeam: false,
    ...overrides,
  }
}

/**
 * Build a mock SessionDataSource.
 *
 * - `available`: controls isAvailable() return value
 * - `rows`: sessions returned by getSessions()
 * - `throwOnGet`: if true, getSessions() throws an Error
 * - `lastUpdate`: returned by getLastUpdate()
 */
function makeMockSource(opts: {
  id: "in-memory" | "file" | "network"
  priority: number
  available?: boolean
  rows?: SessionRow[]
  throwOnGet?: boolean
  lastUpdate?: Date | null
}): SessionDataSource {
  const available = opts.available ?? true
  const rows = opts.rows ?? []
  const lastUpdate = opts.lastUpdate ?? new Date("2026-04-12T10:00:00.000Z")

  return {
    id: opts.id,
    priority: opts.priority,
    isAvailable: () => available,
    getLastUpdate: () => lastUpdate,
    getSessions: async () => {
      if (opts.throwOnGet) {
        throw new Error(`Source ${opts.id} failed`)
      }
      return rows
    },
  }
}

/** Build standard CollectorOptions with injected sources. */
function makeOptions(sources: SessionDataSource[]): CollectorOptions {
  return {
    repoRoot: "/fake/repo",
    sources,
  }
}

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

describe("SessionDataCollector construction", () => {
  it("can be constructed directly", () => {
    const collector = new SessionDataCollector()
    expect(collector).toBeInstanceOf(SessionDataCollector)
  })

  it("createSessionDataCollector factory returns a SessionDataCollector", () => {
    const collector = createSessionDataCollector()
    expect(collector).toBeInstanceOf(SessionDataCollector)
  })
})

// ---------------------------------------------------------------------------
// collect() — empty sources
// ---------------------------------------------------------------------------

describe("SessionDataCollector.collect() — empty sources", () => {
  it("returns empty DashboardData with zero aggregates when sources array is empty", async () => {
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([]))

    expect(data.sessions).toBeArray()
    expect(data.sessions).toHaveLength(0)
    expect(data.aggregates.totalSessions).toBe(0)
    expect(data.aggregates.totalCostUsd).toBe(0)
    expect(data.aggregates.totalTokens).toBe(0)
    expect(data.aggregates.avgSpeedTokensPerSec).toBe(0)
  })

  it("returns empty DashboardData when no sources are provided (undefined)", async () => {
    const collector = new SessionDataCollector()
    const data = await collector.collect({ repoRoot: "/fake/repo" })

    expect(data.sessions).toHaveLength(0)
    expect(data.aggregates.totalSessions).toBe(0)
  })

  it("all source statuses are available=false when no sources provided", async () => {
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([]))

    expect(data.sources.inMemory.available).toBe(false)
    expect(data.sources.file.available).toBe(false)
    expect(data.sources.network.available).toBe(false)
  })

  it("generatedAt is a recent Date", async () => {
    const before = Date.now()
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([]))
    const after = Date.now()

    expect(data.generatedAt).toBeInstanceOf(Date)
    expect(data.generatedAt.getTime()).toBeGreaterThanOrEqual(before)
    expect(data.generatedAt.getTime()).toBeLessThanOrEqual(after)
  })
})

// ---------------------------------------------------------------------------
// collect() — single source
// ---------------------------------------------------------------------------

describe("SessionDataCollector.collect() — single in-memory source", () => {
  it("returns sessions from a single available source", async () => {
    const rows = [
      makeRow({ sessionId: "sess-001" }),
      makeRow({ sessionId: "sess-002" }),
    ]
    const source = makeMockSource({ id: "in-memory", priority: 3, rows })
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([source]))

    expect(data.sessions).toHaveLength(2)
    const ids = data.sessions.map((s) => s.sessionId).sort()
    expect(ids).toEqual(["sess-001", "sess-002"])
  })

  it("inMemory source status is available=true when source is available", async () => {
    const lastUpdate = new Date("2026-04-12T10:00:00.000Z")
    const source = makeMockSource({
      id: "in-memory",
      priority: 3,
      available: true,
      lastUpdate,
      rows: [makeRow()],
    })
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([source]))

    expect(data.sources.inMemory.available).toBe(true)
    expect(data.sources.inMemory.lastUpdate).toEqual(lastUpdate)
    expect(data.sources.inMemory.error).toBeUndefined()
  })

  it("file and network source statuses are available=false when not provided", async () => {
    const source = makeMockSource({ id: "in-memory", priority: 3, rows: [makeRow()] })
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([source]))

    expect(data.sources.file.available).toBe(false)
    expect(data.sources.network.available).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// collect() — deduplication
// ---------------------------------------------------------------------------

describe("SessionDataCollector.collect() — deduplication", () => {
  it("deduplicates by sessionId: higher-priority source wins", async () => {
    const inMemoryRow = makeRow({
      sessionId: "shared-sess",
      source: "in-memory",
      costUsd: 0.99,
      model: "Sonnet 4 (live)",
    })
    const fileRow = makeRow({
      sessionId: "shared-sess",
      source: "file",
      costUsd: 0.50,
      model: "Sonnet 4 (file)",
    })

    const inMemorySource = makeMockSource({
      id: "in-memory",
      priority: 3,
      rows: [inMemoryRow],
    })
    const fileSource = makeMockSource({
      id: "file",
      priority: 2,
      rows: [fileRow],
    })

    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([inMemorySource, fileSource]))

    // Only one row for the shared session
    expect(data.sessions).toHaveLength(1)
    // In-memory (priority=3) wins over file (priority=2)
    expect(data.sessions[0].source).toBe("in-memory")
    expect(data.sessions[0].costUsd).toBeCloseTo(0.99, 4)
  })

  it("deduplicates: file wins over network (priority 2 > 1)", async () => {
    const fileRow = makeRow({ sessionId: "shared-sess", source: "file", costUsd: 0.50 })
    const networkRow = makeRow({ sessionId: "shared-sess", source: "network", costUsd: 0.25 })

    const fileSource = makeMockSource({ id: "file", priority: 2, rows: [fileRow] })
    const networkSource = makeMockSource({ id: "network", priority: 1, rows: [networkRow] })

    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([fileSource, networkSource]))

    expect(data.sessions).toHaveLength(1)
    expect(data.sessions[0].source).toBe("file")
    expect(data.sessions[0].costUsd).toBeCloseTo(0.50, 4)
  })

  it("deduplicates: in-memory wins over network (priority 3 > 1)", async () => {
    const inMemoryRow = makeRow({ sessionId: "shared-sess", source: "in-memory", costUsd: 0.99 })
    const networkRow = makeRow({ sessionId: "shared-sess", source: "network", costUsd: 0.10 })

    const inMemorySource = makeMockSource({ id: "in-memory", priority: 3, rows: [inMemoryRow] })
    const networkSource = makeMockSource({ id: "network", priority: 1, rows: [networkRow] })

    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([inMemorySource, networkSource]))

    expect(data.sessions).toHaveLength(1)
    expect(data.sessions[0].source).toBe("in-memory")
  })

  it("merges unique sessions from multiple sources (no dedup needed)", async () => {
    const inMemoryRows = [makeRow({ sessionId: "sess-A", source: "in-memory" })]
    const fileRows = [makeRow({ sessionId: "sess-B", source: "file" })]

    const inMemorySource = makeMockSource({ id: "in-memory", priority: 3, rows: inMemoryRows })
    const fileSource = makeMockSource({ id: "file", priority: 2, rows: fileRows })

    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([inMemorySource, fileSource]))

    expect(data.sessions).toHaveLength(2)
    const ids = data.sessions.map((s) => s.sessionId).sort()
    expect(ids).toEqual(["sess-A", "sess-B"])
  })

  it("handles three sources with overlapping sessions correctly", async () => {
    // sess-X appears in all three sources; in-memory should win
    const inMemoryRows = [makeRow({ sessionId: "sess-X", source: "in-memory", costUsd: 0.99 })]
    const fileRows = [
      makeRow({ sessionId: "sess-X", source: "file", costUsd: 0.50 }),
      makeRow({ sessionId: "sess-Y", source: "file", costUsd: 0.20 }),
    ]
    const networkRows = [
      makeRow({ sessionId: "sess-X", source: "network", costUsd: 0.10 }),
      makeRow({ sessionId: "sess-Z", source: "network", costUsd: 0.05 }),
    ]

    const inMemorySource = makeMockSource({ id: "in-memory", priority: 3, rows: inMemoryRows })
    const fileSource = makeMockSource({ id: "file", priority: 2, rows: fileRows })
    const networkSource = makeMockSource({ id: "network", priority: 1, rows: networkRows })

    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([inMemorySource, fileSource, networkSource]))

    // sess-X (in-memory wins), sess-Y (file), sess-Z (network) = 3 unique sessions
    expect(data.sessions).toHaveLength(3)
    const sessX = data.sessions.find((s) => s.sessionId === "sess-X")
    expect(sessX).toBeDefined()
    expect(sessX!.source).toBe("in-memory")
    expect(sessX!.costUsd).toBeCloseTo(0.99, 4)
  })
})

// ---------------------------------------------------------------------------
// collect() — aggregate computation
// ---------------------------------------------------------------------------

describe("SessionDataCollector.collect() — aggregate computation", () => {
  it("computes correct totalSessions (non-team only)", async () => {
    const rows = [
      makeRow({ sessionId: "sess-1", isTeam: false }),
      makeRow({ sessionId: "sess-2", isTeam: false }),
      makeRow({ sessionId: "sess-3", isTeam: true }), // excluded
    ]
    const source = makeMockSource({ id: "in-memory", priority: 3, rows })
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([source]))

    expect(data.aggregates.totalSessions).toBe(2)
  })

  it("computes correct totalCostUsd (non-team only)", async () => {
    const rows = [
      makeRow({ sessionId: "sess-1", isTeam: false, costUsd: 0.10 }),
      makeRow({ sessionId: "sess-2", isTeam: false, costUsd: 0.25 }),
      makeRow({ sessionId: "sess-3", isTeam: true, costUsd: 99.99 }), // excluded
    ]
    const source = makeMockSource({ id: "in-memory", priority: 3, rows })
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([source]))

    expect(data.aggregates.totalCostUsd).toBeCloseTo(0.35, 4)
  })

  it("computes correct totalTokens (non-team only)", async () => {
    const rows = [
      makeRow({ sessionId: "sess-1", isTeam: false, totalTokens: 1000 }),
      makeRow({ sessionId: "sess-2", isTeam: false, totalTokens: 2500 }),
      makeRow({ sessionId: "sess-3", isTeam: true, totalTokens: 99999 }), // excluded
    ]
    const source = makeMockSource({ id: "in-memory", priority: 3, rows })
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([source]))

    expect(data.aggregates.totalTokens).toBe(3500)
  })

  it("computes correct avgSpeedTokensPerSec", async () => {
    // 3000 tokens / 60 seconds = 50 tokens/sec
    const rows = [
      makeRow({ sessionId: "sess-1", isTeam: false, totalTokens: 1000, durationSeconds: 20 }),
      makeRow({ sessionId: "sess-2", isTeam: false, totalTokens: 2000, durationSeconds: 40 }),
    ]
    const source = makeMockSource({ id: "in-memory", priority: 3, rows })
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([source]))

    // 3000 tokens / 60 seconds = 50 tokens/sec
    expect(data.aggregates.avgSpeedTokensPerSec).toBeCloseTo(50, 4)
  })

  it("avgSpeedTokensPerSec is 0 when total duration is 0", async () => {
    const rows = [
      makeRow({ sessionId: "sess-1", isTeam: false, totalTokens: 1000, durationSeconds: 0 }),
      makeRow({ sessionId: "sess-2", isTeam: false, totalTokens: 500, durationSeconds: 0 }),
    ]
    const source = makeMockSource({ id: "in-memory", priority: 3, rows })
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([source]))

    expect(data.aggregates.avgSpeedTokensPerSec).toBe(0)
  })

  it("avgSpeedTokensPerSec is 0 when there are no sessions", async () => {
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([]))

    expect(data.aggregates.avgSpeedTokensPerSec).toBe(0)
  })

  it("team sessions are fully excluded from all aggregate totals", async () => {
    const rows = [
      makeRow({ sessionId: "team-1", isTeam: true, costUsd: 100, totalTokens: 50000, durationSeconds: 3600 }),
      makeRow({ sessionId: "team-2", isTeam: true, costUsd: 200, totalTokens: 80000, durationSeconds: 7200 }),
    ]
    const source = makeMockSource({ id: "in-memory", priority: 3, rows })
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([source]))

    expect(data.aggregates.totalSessions).toBe(0)
    expect(data.aggregates.totalCostUsd).toBe(0)
    expect(data.aggregates.totalTokens).toBe(0)
    expect(data.aggregates.avgSpeedTokensPerSec).toBe(0)
  })

  it("aggregates across sessions from multiple sources", async () => {
    const inMemoryRows = [
      makeRow({ sessionId: "sess-A", source: "in-memory", isTeam: false, costUsd: 0.10, totalTokens: 1000, durationSeconds: 30 }),
    ]
    const fileRows = [
      makeRow({ sessionId: "sess-B", source: "file", isTeam: false, costUsd: 0.20, totalTokens: 2000, durationSeconds: 60 }),
    ]

    const inMemorySource = makeMockSource({ id: "in-memory", priority: 3, rows: inMemoryRows })
    const fileSource = makeMockSource({ id: "file", priority: 2, rows: fileRows })

    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([inMemorySource, fileSource]))

    expect(data.aggregates.totalSessions).toBe(2)
    expect(data.aggregates.totalCostUsd).toBeCloseTo(0.30, 4)
    expect(data.aggregates.totalTokens).toBe(3000)
    // 3000 tokens / 90 seconds = 33.33... tokens/sec
    expect(data.aggregates.avgSpeedTokensPerSec).toBeCloseTo(33.333, 2)
  })
})

// ---------------------------------------------------------------------------
// collect() — source error handling
// ---------------------------------------------------------------------------

describe("SessionDataCollector.collect() — source error handling", () => {
  it("handles a source that throws: other sources still return data", async () => {
    const throwingSource = makeMockSource({
      id: "in-memory",
      priority: 3,
      available: true,
      throwOnGet: true,
    })
    const goodSource = makeMockSource({
      id: "file",
      priority: 2,
      available: true,
      rows: [makeRow({ sessionId: "sess-from-file", source: "file" })],
    })

    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([throwingSource, goodSource]))

    // File source data is still returned
    expect(data.sessions).toHaveLength(1)
    expect(data.sessions[0].sessionId).toBe("sess-from-file")
  })

  it("marks a throwing source as available=false with error message", async () => {
    const throwingSource = makeMockSource({
      id: "in-memory",
      priority: 3,
      available: true,
      throwOnGet: true,
    })

    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([throwingSource]))

    expect(data.sources.inMemory.available).toBe(false)
    expect(data.sources.inMemory.error).toBe("Source in-memory failed")
  })

  it("does not crash when all sources throw", async () => {
    const throwingSource1 = makeMockSource({
      id: "in-memory",
      priority: 3,
      available: true,
      throwOnGet: true,
    })
    const throwingSource2 = makeMockSource({
      id: "file",
      priority: 2,
      available: true,
      throwOnGet: true,
    })

    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([throwingSource1, throwingSource2]))

    expect(data.sessions).toHaveLength(0)
    expect(data.aggregates.totalSessions).toBe(0)
    expect(data.sources.inMemory.available).toBe(false)
    expect(data.sources.file.available).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// collect() — unavailable source handling
// ---------------------------------------------------------------------------

describe("SessionDataCollector.collect() — unavailable source", () => {
  it("skips getSessions() on unavailable source", async () => {
    let getSessionsCalled = false
    const unavailableSource: SessionDataSource = {
      id: "in-memory",
      priority: 3,
      isAvailable: () => false,
      getLastUpdate: () => null,
      getSessions: async () => {
        getSessionsCalled = true
        return [makeRow()]
      },
    }

    const collector = new SessionDataCollector()
    await collector.collect(makeOptions([unavailableSource]))

    expect(getSessionsCalled).toBe(false)
  })

  it("marks unavailable source as available=false in sources status", async () => {
    const unavailableSource = makeMockSource({
      id: "file",
      priority: 2,
      available: false,
      rows: [makeRow()],
    })

    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([unavailableSource]))

    expect(data.sources.file.available).toBe(false)
    expect(data.sessions).toHaveLength(0)
  })

  it("available source still returns data when another source is unavailable", async () => {
    const unavailableSource = makeMockSource({
      id: "in-memory",
      priority: 3,
      available: false,
    })
    const availableSource = makeMockSource({
      id: "file",
      priority: 2,
      available: true,
      rows: [makeRow({ sessionId: "sess-from-file", source: "file" })],
    })

    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([unavailableSource, availableSource]))

    expect(data.sessions).toHaveLength(1)
    expect(data.sessions[0].sessionId).toBe("sess-from-file")
    expect(data.sources.inMemory.available).toBe(false)
    expect(data.sources.file.available).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// collect() — sources status
// ---------------------------------------------------------------------------

describe("SessionDataCollector.collect() — sources status", () => {
  it("sources status reflects each source's availability", async () => {
    const inMemoryLastUpdate = new Date("2026-04-12T10:00:00.000Z")
    const fileLastUpdate = new Date("2026-04-12T09:00:00.000Z")

    const inMemorySource = makeMockSource({
      id: "in-memory",
      priority: 3,
      available: true,
      lastUpdate: inMemoryLastUpdate,
      rows: [makeRow()],
    })
    const fileSource = makeMockSource({
      id: "file",
      priority: 2,
      available: true,
      lastUpdate: fileLastUpdate,
      rows: [],
    })

    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([inMemorySource, fileSource]))

    expect(data.sources.inMemory.available).toBe(true)
    expect(data.sources.file.available).toBe(true)
    expect(data.sources.network.available).toBe(false)
    expect(data.sources.network.lastUpdate).toBeNull()
  })

  it("network source status is available=false with null lastUpdate when no network source provided", async () => {
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([]))

    expect(data.sources.network.available).toBe(false)
    expect(data.sources.network.lastUpdate).toBeNull()
  })

  it("network source status reflects availability when network source is provided", async () => {
    const networkLastUpdate = new Date("2026-04-12T08:00:00.000Z")
    const networkSource = makeMockSource({
      id: "network",
      priority: 1,
      available: true,
      lastUpdate: networkLastUpdate,
      rows: [makeRow({ source: "network" })],
    })

    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([networkSource]))

    expect(data.sources.network.available).toBe(true)
    expect(data.sources.network.lastUpdate).toEqual(networkLastUpdate)
  })
})

// ---------------------------------------------------------------------------
// collect() — generatedAt
// ---------------------------------------------------------------------------

describe("SessionDataCollector.collect() — generatedAt", () => {
  it("generatedAt is a recent Date", async () => {
    const before = Date.now()
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([]))
    const after = Date.now()

    expect(data.generatedAt).toBeInstanceOf(Date)
    expect(data.generatedAt.getTime()).toBeGreaterThanOrEqual(before)
    expect(data.generatedAt.getTime()).toBeLessThanOrEqual(after)
  })

  it("generatedAt is a new Date on each collect() call", async () => {
    const collector = new SessionDataCollector()
    const data1 = await collector.collect(makeOptions([]))
    // Small delay to ensure timestamps differ
    await new Promise((resolve) => setTimeout(resolve, 5))
    const data2 = await collector.collect(makeOptions([]))

    expect(data2.generatedAt.getTime()).toBeGreaterThanOrEqual(data1.generatedAt.getTime())
  })
})

// ---------------------------------------------------------------------------
// collect() — sessions ordering
// ---------------------------------------------------------------------------

describe("SessionDataCollector.collect() — sessions ordering", () => {
  it("sessions are ordered by updatedAt descending (most recent first)", async () => {
    const rows = [
      makeRow({ sessionId: "sess-old", updatedAt: new Date("2026-04-10T08:00:00.000Z") }),
      makeRow({ sessionId: "sess-new", updatedAt: new Date("2026-04-12T10:00:00.000Z") }),
      makeRow({ sessionId: "sess-mid", updatedAt: new Date("2026-04-11T09:00:00.000Z") }),
    ]
    const source = makeMockSource({ id: "in-memory", priority: 3, rows })
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([source]))

    expect(data.sessions[0].sessionId).toBe("sess-new")
    expect(data.sessions[1].sessionId).toBe("sess-mid")
    expect(data.sessions[2].sessionId).toBe("sess-old")
  })
})

// ---------------------------------------------------------------------------
// DashboardData shape
// ---------------------------------------------------------------------------

describe("SessionDataCollector.collect() — DashboardData shape", () => {
  it("returned DashboardData has all required fields", async () => {
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([]))

    // sessions
    expect(data.sessions).toBeArray()
    // aggregates
    expect(typeof data.aggregates.totalSessions).toBe("number")
    expect(typeof data.aggregates.totalCostUsd).toBe("number")
    expect(typeof data.aggregates.totalTokens).toBe("number")
    expect(typeof data.aggregates.avgSpeedTokensPerSec).toBe("number")
    // sources
    expect(typeof data.sources.inMemory.available).toBe("boolean")
    expect(typeof data.sources.file.available).toBe("boolean")
    expect(typeof data.sources.network.available).toBe("boolean")
    // generatedAt
    expect(data.generatedAt).toBeInstanceOf(Date)
  })
})

// ---------------------------------------------------------------------------
// REQ-TUI-015: stopped-session TTL
// ---------------------------------------------------------------------------

/**
 * Tests for the stopped-session TTL feature.
 *
 * REQ-TUI-015: Stopped in-memory sessions MUST be removed from the dashboard
 * after the configured TTL (default 300 s / 5 min) has elapsed since updatedAt.
 * File-sourced sessions are historical records and are NOT subject to TTL.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-015 plan=step-reverify-002 test=dashboard-collector.test.ts#REQ-TUI-015
 */
describe("REQ-TUI-015: stopped-session TTL", () => {
  it("stopped in-memory session within TTL appears in results", async () => {
    // 2 minutes old — within the default 5-minute TTL
    const recentlyStopped = makeRow({
      sessionId: "recent-stopped",
      status: "stopped",
      source: "in-memory",
      updatedAt: new Date(Date.now() - 2 * 60 * 1000),
    })
    const source = makeMockSource({ id: "in-memory", priority: 3, rows: [recentlyStopped] })
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([source]))

    const ids = data.sessions.map((s) => s.sessionId)
    expect(ids).toContain("recent-stopped")
  })

  it("stopped in-memory session past TTL is removed from results", async () => {
    // 10 minutes old — past the default 5-minute TTL
    const oldStopped = makeRow({
      sessionId: "old-stopped",
      status: "stopped",
      source: "in-memory",
      updatedAt: new Date(Date.now() - 10 * 60 * 1000),
    })
    const source = makeMockSource({ id: "in-memory", priority: 3, rows: [oldStopped] })
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([source]))

    const ids = data.sessions.map((s) => s.sessionId)
    expect(ids).not.toContain("old-stopped")
    expect(data.sessions).toHaveLength(0)
  })

  it("configurable TTL override works for in-memory stopped sessions", async () => {
    // Session is 2 minutes old; custom TTL is 60 s (1 min) → should be removed
    const twoMinOldStopped = makeRow({
      sessionId: "two-min-stopped",
      status: "stopped",
      source: "in-memory",
      updatedAt: new Date(Date.now() - 2 * 60 * 1000),
    })
    const source = makeMockSource({ id: "in-memory", priority: 3, rows: [twoMinOldStopped] })
    const collector = new SessionDataCollector()
    const data = await collector.collect({
      repoRoot: "/fake/repo",
      sources: [source],
      stoppedTtlSeconds: 60, // 1-minute TTL
    })

    const ids = data.sessions.map((s) => s.sessionId)
    expect(ids).not.toContain("two-min-stopped")
    expect(data.sessions).toHaveLength(0)
  })

  it("running and idle sessions are never filtered by TTL", async () => {
    // Very old running and idle sessions — should always appear regardless of age
    const veryOldRunning = makeRow({
      sessionId: "old-running",
      status: "running",
      source: "in-memory",
      updatedAt: new Date(Date.now() - 60 * 60 * 1000), // 1 hour ago
    })
    const veryOldIdle = makeRow({
      sessionId: "old-idle",
      status: "idle",
      source: "in-memory",
      updatedAt: new Date(Date.now() - 60 * 60 * 1000), // 1 hour ago
    })
    const source = makeMockSource({
      id: "in-memory",
      priority: 3,
      rows: [veryOldRunning, veryOldIdle],
    })
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([source]))

    const ids = data.sessions.map((s) => s.sessionId)
    expect(ids).toContain("old-running")
    expect(ids).toContain("old-idle")
    expect(data.sessions).toHaveLength(2)
  })

  it("file-sourced stopped sessions are NOT filtered by TTL (historical records)", async () => {
    // A file-sourced session 10 minutes old should still appear — it's a historical record
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-015 plan=step-reverify-002
    const oldFileStopped = makeRow({
      sessionId: "old-file-stopped",
      status: "stopped",
      source: "file",
      updatedAt: new Date(Date.now() - 10 * 60 * 1000), // 10 min ago — past default 5-min TTL
    })
    const fileSource = makeMockSource({
      id: "file",
      priority: 2,
      rows: [oldFileStopped],
    })
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([fileSource]))

    // File-sourced historical session must survive TTL filtering
    const ids = data.sessions.map((s) => s.sessionId)
    expect(ids).toContain("old-file-stopped")
    expect(data.sessions).toHaveLength(1)
  })

  it("file-sourced stopped sessions survive even with a very short custom TTL", async () => {
    // Even with a 1-second TTL, file-sourced sessions must not be filtered
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-015 plan=step-reverify-002
    const oldFileStopped = makeRow({
      sessionId: "old-file-stopped-short-ttl",
      status: "stopped",
      source: "file",
      updatedAt: new Date(Date.now() - 60 * 60 * 1000), // 1 hour ago
    })
    const fileSource = makeMockSource({
      id: "file",
      priority: 2,
      rows: [oldFileStopped],
    })
    const collector = new SessionDataCollector()
    const data = await collector.collect({
      repoRoot: "/fake/repo",
      sources: [fileSource],
      stoppedTtlSeconds: 1, // extremely short TTL — must not affect file sessions
    })

    const ids = data.sessions.map((s) => s.sessionId)
    expect(ids).toContain("old-file-stopped-short-ttl")
    expect(data.sessions).toHaveLength(1)
  })

  it("mixed sources: in-memory old stopped is filtered, file old stopped survives", async () => {
    // Validates the boundary: same age, different source → different TTL treatment
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-015 plan=step-reverify-002
    const oldInMemoryStopped = makeRow({
      sessionId: "old-in-memory-stopped",
      status: "stopped",
      source: "in-memory",
      updatedAt: new Date(Date.now() - 10 * 60 * 1000), // 10 min ago
    })
    const oldFileStopped = makeRow({
      sessionId: "old-file-stopped-mixed",
      status: "stopped",
      source: "file",
      updatedAt: new Date(Date.now() - 10 * 60 * 1000), // 10 min ago — same age
    })
    const inMemorySource = makeMockSource({
      id: "in-memory",
      priority: 3,
      rows: [oldInMemoryStopped],
    })
    const fileSource = makeMockSource({
      id: "file",
      priority: 2,
      rows: [oldFileStopped],
    })
    const collector = new SessionDataCollector()
    const data = await collector.collect(makeOptions([inMemorySource, fileSource]))

    const ids = data.sessions.map((s) => s.sessionId)
    // in-memory old stopped → filtered out
    expect(ids).not.toContain("old-in-memory-stopped")
    // file old stopped → survives (historical record)
    expect(ids).toContain("old-file-stopped-mixed")
    expect(data.sessions).toHaveLength(1)
  })
})
