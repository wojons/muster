/**
 * Tests for SQLiteSource — reads session data from OpenCode's SQLite database.
 *
 * All tests use injected `_queryRunner` — no real SQLite I/O.
 * The `isAvailable()` tests use a real filesystem path check.
 *
 * REQ-TUI-060: SQLiteSource MUST read session data from the OpenCode SQLite
 *   database and return correctly shaped SessionRow objects.
 *
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-060
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-060 plan=trace-fix test=dashboard-sqlite.test.ts
 */

import { describe, it, expect } from "bun:test"
import { SQLiteSource } from "../src/dashboard/sources/sqlite"
import type { SQLiteSessionRow } from "../src/dashboard/sources/sqlite"

// ---------------------------------------------------------------------------
// Fixture helpers
// ---------------------------------------------------------------------------

/** Build a minimal valid SQLiteSessionRow for testing. */
function makeRow(overrides: Partial<SQLiteSessionRow> = {}): SQLiteSessionRow {
  return {
    id: "sess-sqlite-001",
    title: "Test session",
    directory: "/home/user/projects/myapp",
    message_count: 10,
    total_cost: 0.11996,
    total_tokens: 217296,
    input_tokens: 100000,
    output_tokens: 50000,
    cache_read_tokens: 60000,
    cache_write_tokens: 7296,
    reasoning_tokens: 0,
    tool_calls: 5,
    last_model: "anthropic.claude-opus-4-6-v1",
    time_created: 1744000000000,
    time_updated: 1744003600000,
    duration_seconds: 3600,
    ...overrides,
  }
}

/**
 * Build a SQLiteSource with an injected query runner (no real DB I/O).
 * The dbPath is set to a real existing path so isAvailable() returns true.
 */
function makeSource(
  rows: SQLiteSessionRow[],
  opts: {
    filterDirectory?: string
    maxSessions?: number
    dbPath?: string
  } = {},
) {
  return new SQLiteSource({
    // Use a real path that exists so isAvailable() returns true in tests
    // that need it. Tests that want isAvailable()=false pass a fake path.
    dbPath: opts.dbPath ?? "/Users/aokuwa/code/Axiom/.worktrees/tui-dashboard/.axiom/plugin",
    filterDirectory: opts.filterDirectory,
    maxSessions: opts.maxSessions,
    _queryRunner: (_sql: string) => rows,
  })
}

// ===========================================================================
// getSessions() — basic shape
// ===========================================================================

describe("SQLiteSource.getSessions() — basic shape", () => {
  it("returns a correctly shaped SessionRow from a valid SQLite row", async () => {
    const source = makeSource([makeRow()])

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(1)

    const row = sessions[0]
    expect(row.sessionId).toBe("sess-sqlite-001")
    expect(row.source).toBe("sqlite")
    expect(row.isTeam).toBe(false)
    expect(row.updatedAt).toBeInstanceOf(Date)
  })

  it("costUsd comes from total_cost", async () => {
    const source = makeSource([makeRow({ total_cost: 0.42 })])

    const sessions = await source.getSessions()
    expect(sessions[0].costUsd).toBeCloseTo(0.42, 5)
  })

  it("totalTokens comes from total_tokens", async () => {
    const source = makeSource([makeRow({ total_tokens: 150000 })])

    const sessions = await source.getSessions()
    expect(sessions[0].totalTokens).toBe(150000)
  })

  it("durationSeconds comes from duration_seconds", async () => {
    const source = makeSource([makeRow({ duration_seconds: 1800 })])

    const sessions = await source.getSessions()
    expect(sessions[0].durationSeconds).toBe(1800)
  })

  it("source is always 'sqlite'", async () => {
    const source = makeSource([makeRow()])

    const sessions = await source.getSessions()
    expect(sessions[0].source).toBe("sqlite")
  })

  it("isTeam is always false", async () => {
    const source = makeSource([makeRow()])

    const sessions = await source.getSessions()
    expect(sessions[0].isTeam).toBe(false)
  })

  it("title is passed through unchanged", async () => {
    const source = makeSource([makeRow({ title: "My long session title that is not truncated here" })])

    const sessions = await source.getSessions()
    expect(sessions[0].title).toBe("My long session title that is not truncated here")
  })

  it("updatedAt is parsed from time_updated (epoch ms)", async () => {
    const timeUpdated = 1744003600000
    const source = makeSource([makeRow({ time_updated: timeUpdated })])

    const sessions = await source.getSessions()
    expect(sessions[0].updatedAt).toEqual(new Date(timeUpdated))
  })
})

// ===========================================================================
// getSessions() — reasoningTokens mapping (REQ-TUI-100 regression)
// ===========================================================================

describe("SQLiteSource.getSessions() — reasoningTokens mapping", () => {
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-100 plan=F-003 test=dashboard-sqlite.test.ts

  it("maps reasoning_tokens from SQLite row to reasoningTokens on SessionRow", async () => {
    const source = makeSource([makeRow({ reasoning_tokens: 4200 })])

    const sessions = await source.getSessions()
    expect(sessions[0].reasoningTokens).toBe(4200)
  })

  it("defaults reasoningTokens to 0 when reasoning_tokens is 0", async () => {
    const source = makeSource([makeRow({ reasoning_tokens: 0 })])

    const sessions = await source.getSessions()
    expect(sessions[0].reasoningTokens).toBe(0)
  })

  it("handles large reasoning_tokens values without overflow", async () => {
    const source = makeSource([makeRow({ reasoning_tokens: 999_999 })])

    const sessions = await source.getSessions()
    expect(sessions[0].reasoningTokens).toBe(999_999)
  })
})

// ===========================================================================
// getSessions() — model alias lookup
// ===========================================================================

describe("SQLiteSource.getSessions() — model alias lookup", () => {
  it("resolves known model alias: anthropic.claude-opus-4-6-v1 → Claude Opus 4.6", async () => {
    const source = makeSource([makeRow({ last_model: "anthropic.claude-opus-4-6-v1" })])

    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Claude Opus 4.6")
    expect(sessions[0].modelRaw).toBe("anthropic.claude-opus-4-6-v1")
  })

  it("resolves known model alias: claude-sonnet-4-20250514 → Claude Sonnet 4", async () => {
    const source = makeSource([makeRow({ last_model: "claude-sonnet-4-20250514" })])

    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Claude Sonnet 4")
    expect(sessions[0].modelRaw).toBe("claude-sonnet-4-20250514")
  })

  it("resolves known model alias: amazon-bedrock/anthropic.claude-sonnet-4-6 → Claude Sonnet 4.6", async () => {
    const source = makeSource([makeRow({ last_model: "amazon-bedrock/anthropic.claude-sonnet-4-6" })])

    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Claude Sonnet 4.6")
  })

  it("returns full raw ID for unknown models (no truncation)", async () => {
    const longModel = "some-unknown-model-with-a-very-long-name"
    const source = makeSource([makeRow({ last_model: longModel })])

    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe(longModel)  // full ID, no truncation
    expect(sessions[0].modelRaw).toBe(longModel)
  })

  it("handles null last_model gracefully — model and modelRaw are empty string", async () => {
    const source = makeSource([makeRow({ last_model: null })])

    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("")
    expect(sessions[0].modelRaw).toBe("")
  })
})

// ===========================================================================
// getSessions() — status heuristic
// ===========================================================================

describe("SQLiteSource.getSessions() — status heuristic", () => {
  it("status is 'running' when time_updated is within last 5 minutes", async () => {
    const recentMs = Date.now() - 60_000 // 1 minute ago
    const source = makeSource([makeRow({ time_updated: recentMs })])

    const sessions = await source.getSessions()
    expect(sessions[0].status).toBe("running")
  })

  it("status is 'stopped' when time_updated is older than 5 minutes", async () => {
    const oldMs = Date.now() - 10 * 60_000 // 10 minutes ago
    const source = makeSource([makeRow({ time_updated: oldMs })])

    const sessions = await source.getSessions()
    expect(sessions[0].status).toBe("stopped")
  })

  it("status is 'running' when time_updated is exactly at the 5-minute boundary", async () => {
    // Exactly 5 minutes ago — should be "running" (age <= 5min)
    const exactBoundaryMs = Date.now() - 5 * 60_000
    const source = makeSource([makeRow({ time_updated: exactBoundaryMs })])

    const sessions = await source.getSessions()
    // At exactly 5 minutes, age <= fiveMinutesMs is true (boundary is inclusive)
    expect(sessions[0].status).toBe("running")
  })

  it("status is 'stopped' when time_updated is just past the 5-minute boundary", async () => {
    const justPastMs = Date.now() - 5 * 60_000 - 1000 // 5 min + 1 sec ago
    const source = makeSource([makeRow({ time_updated: justPastMs })])

    const sessions = await source.getSessions()
    expect(sessions[0].status).toBe("stopped")
  })
})

// ===========================================================================
// getSessions() — isAvailable() and missing DB
// ===========================================================================

describe("SQLiteSource.isAvailable()", () => {
  it("returns false when DB file does not exist", () => {
    const source = new SQLiteSource({
      dbPath: "/nonexistent/path/to/opencode.db",
    })
    expect(source.isAvailable()).toBe(false)
  })

  it("returns true when DB file exists on real filesystem", () => {
    // Use the test file's own directory — guaranteed to exist in any checkout
    const source = new SQLiteSource({
      dbPath: import.meta.dir,
    })
    expect(source.isAvailable()).toBe(true)
  })
})

describe("SQLiteSource.getSessions() — missing DB", () => {
  it("returns empty array when DB file does not exist (no _queryRunner)", async () => {
    const source = new SQLiteSource({
      dbPath: "/nonexistent/path/to/opencode.db",
    })

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(0)
  })
})

// ===========================================================================
// getSessions() — multiple rows
// ===========================================================================

describe("SQLiteSource.getSessions() — multiple rows", () => {
  it("returns all rows from the query runner", async () => {
    const source = makeSource([
      makeRow({ id: "sess-a", title: "Session A" }),
      makeRow({ id: "sess-b", title: "Session B" }),
      makeRow({ id: "sess-c", title: "Session C" }),
    ])

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(3)

    const ids = sessions.map((s) => s.sessionId).sort()
    expect(ids).toEqual(["sess-a", "sess-b", "sess-c"])
  })

  it("returns empty array when query runner returns no rows", async () => {
    const source = makeSource([])

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(0)
  })
})

// ===========================================================================
// getSessions() — getLastError() after query failure
// ===========================================================================

describe("SQLiteSource.getLastError()", () => {
  it("returns null before any query", () => {
    const source = makeSource([])
    expect(source.getLastError()).toBeNull()
  })

  it("returns null after a successful query", async () => {
    const source = makeSource([makeRow()])
    await source.getSessions()
    expect(source.getLastError()).toBeNull()
  })

  it("returns error message after query failure", async () => {
    const source = new SQLiteSource({
      dbPath: "/Users/aokuwa/code/Axiom/.worktrees/tui-dashboard/.axiom/plugin",
      _queryRunner: (_sql: string) => {
        throw new Error("SQLITE_CORRUPT: database disk image is malformed")
      },
    })

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(0)
    expect(source.getLastError()).toBe("SQLITE_CORRUPT: database disk image is malformed")
  })

  it("clears error after a successful query following a failure", async () => {
    let shouldFail = true
    const source = new SQLiteSource({
      dbPath: "/Users/aokuwa/code/Axiom/.worktrees/tui-dashboard/.axiom/plugin",
      _queryRunner: (_sql: string) => {
        if (shouldFail) throw new Error("temporary error")
        return [makeRow()]
      },
    })

    // First call fails
    await source.getSessions()
    expect(source.getLastError()).toBe("temporary error")

    // Second call succeeds
    shouldFail = false
    await source.getSessions()
    expect(source.getLastError()).toBeNull()
  })
})

// ===========================================================================
// getSessions() — filterDirectory
// ===========================================================================

describe("SQLiteSource.getSessions() — filterDirectory", () => {
  it("returns only sessions whose directory starts with filterDirectory", async () => {
    const source = makeSource(
      [
        makeRow({ id: "sess-match-1", directory: "/home/user/projects/myapp" }),
        makeRow({ id: "sess-match-2", directory: "/home/user/projects/myapp/subdir" }),
        makeRow({ id: "sess-other", directory: "/home/user/other-project" }),
      ],
      { filterDirectory: "/home/user/projects/myapp" },
    )

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(2)

    const ids = sessions.map((s) => s.sessionId).sort()
    expect(ids).toEqual(["sess-match-1", "sess-match-2"])
  })

  it("returns all sessions when filterDirectory is not set", async () => {
    const source = makeSource([
      makeRow({ id: "sess-a", directory: "/home/user/project-a" }),
      makeRow({ id: "sess-b", directory: "/home/user/project-b" }),
    ])

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(2)
  })

  it("returns empty array when no sessions match filterDirectory", async () => {
    const source = makeSource(
      [
        makeRow({ id: "sess-a", directory: "/home/user/project-a" }),
      ],
      { filterDirectory: "/home/user/project-b" },
    )

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(0)
  })
})

// ===========================================================================
// getSessions() — maxSessions
// ===========================================================================

describe("SQLiteSource.getSessions() — maxSessions", () => {
  it("passes maxSessions as LIMIT in the SQL query", async () => {
    let capturedSql = ""
    const source = new SQLiteSource({
      dbPath: "/Users/aokuwa/code/Axiom/.worktrees/tui-dashboard/.axiom/plugin",
      maxSessions: 10,
      _queryRunner: (sql: string) => {
        capturedSql = sql
        return []
      },
    })

    await source.getSessions()
    expect(capturedSql).toContain("LIMIT 10")
  })

  it("defaults to LIMIT 50 when maxSessions is not set", async () => {
    let capturedSql = ""
    const source = new SQLiteSource({
      dbPath: "/Users/aokuwa/code/Axiom/.worktrees/tui-dashboard/.axiom/plugin",
      _queryRunner: (sql: string) => {
        capturedSql = sql
        return []
      },
    })

    await source.getSessions()
    expect(capturedSql).toContain("LIMIT 50")
  })
})

// ===========================================================================
// getLastUpdate()
// ===========================================================================

describe("SQLiteSource.getLastUpdate()", () => {
  it("returns null before any query", () => {
    const source = makeSource([])
    expect(source.getLastUpdate()).toBeNull()
  })

  it("returns null after a query that returns no rows", async () => {
    const source = makeSource([])
    await source.getSessions()
    expect(source.getLastUpdate()).toBeNull()
  })

  it("returns the most recent time_updated after a successful query", async () => {
    const olderMs = 1744000000000
    const newerMs = 1744003600000

    const source = makeSource([
      makeRow({ id: "older", time_updated: olderMs }),
      makeRow({ id: "newer", time_updated: newerMs }),
    ])

    await source.getSessions()
    const lastUpdate = source.getLastUpdate()
    expect(lastUpdate).not.toBeNull()
    expect(lastUpdate!.getTime()).toBe(newerMs)
  })
})

// ===========================================================================
// workItemId extraction
// ===========================================================================

describe("SQLiteSource.getSessions() — workItemId extraction", () => {
  it("extracts Jira-style work item ID from title", async () => {
    const source = makeSource([makeRow({ title: "DEX-205 implement sqlite source" })])

    const sessions = await source.getSessions()
    expect(sessions[0].workItemId).toBe("DEX-205")
  })

  it("extracts work item ID from title with WORKTREE prefix", async () => {
    const source = makeSource([makeRow({ title: "WORKTREE AUTH-123 some description" })])

    const sessions = await source.getSessions()
    expect(sessions[0].workItemId).toBe("AUTH-123")
  })

  it("returns undefined workItemId when title has no Jira key", async () => {
    const source = makeSource([makeRow({ title: "Just a plain session title" })])

    const sessions = await source.getSessions()
    expect(sessions[0].workItemId).toBeUndefined()
  })

  it("returns undefined workItemId for empty title", async () => {
    const source = makeSource([makeRow({ title: "" })])

    const sessions = await source.getSessions()
    expect(sessions[0].workItemId).toBeUndefined()
  })
})

// ===========================================================================
// getSessions() — sinceMs DB-level filtering (task-10.1 / REQ-TUI-082)
// ===========================================================================

describe("SQLiteSource — sinceMs filtering (task-10.1)", () => {
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-082 plan=phase-10/task-10.1 test=dashboard-sqlite.test.ts

  it("sinceMs injects WHERE clause into CTE — cutoff timestamp appears in SQL", async () => {
    let capturedSql = ""
    const source = new SQLiteSource({
      dbPath: "/Users/aokuwa/code/Axiom/.worktrees/tui-dashboard/.axiom/plugin",
      sinceMs: 1700000000000,
      _queryRunner: (sql: string) => {
        capturedSql = sql
        return []
      },
    })

    await source.getSessions()
    // The cutoff epoch-ms value must appear in the SQL so filtering is DB-level
    expect(capturedSql).toContain("1700000000000")
    // Confirm it's in a WHERE clause (not just a comment)
    expect(capturedSql).toContain("time_updated >= 1700000000000")
  })

  it("no sinceMs — no time_updated WHERE clause in query", async () => {
    let capturedSql = ""
    const source = new SQLiteSource({
      dbPath: "/Users/aokuwa/code/Axiom/.worktrees/tui-dashboard/.axiom/plugin",
      _queryRunner: (sql: string) => {
        capturedSql = sql
        return []
      },
    })

    await source.getSessions()
    expect(capturedSql).not.toContain("time_updated >=")
  })
})

describe("SQLiteSource — SessionDataSource interface compliance", () => {
  it("has id = 'sqlite'", () => {
    const source = makeSource([])
    expect(source.id).toBe("sqlite")
  })

  it("has priority = 4", () => {
    const source = makeSource([])
    expect(source.priority).toBe(4)
  })

  it("getSessions() returns a Promise", () => {
    const source = makeSource([])
    const result = source.getSessions()
    expect(result).toBeInstanceOf(Promise)
  })

  it("getLastUpdate() returns Date | null", () => {
    const source = makeSource([])
    const result = source.getLastUpdate()
    expect(result === null || result instanceof Date).toBe(true)
  })

  it("isAvailable() returns boolean", () => {
    const source = makeSource([])
    const result = source.isAvailable()
    expect(typeof result).toBe("boolean")
  })
})
