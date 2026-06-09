/**
 * Tests for FileSource — reads session analytics from .axiom/sessions/*.json.
 *
 * All tests use injected `_readFile` and `_listFiles` — no real filesystem I/O.
 *
 * REQ-TUI-054: FileSource MUST read session analytics files from
 *   `.axiom/sessions/*.json` with <10ms per file read.
 *
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-054
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-054 plan=phase-1/task-1.3 test=dashboard-file.test.ts
 */

import { describe, it, expect } from "bun:test"
import { FileSource, MAX_SESSION_FILES } from "../src/dashboard/sources/file"
import { MODEL_DISPLAY_ALIASES } from "../src/dashboard/model-aliases"
import type { SessionAnalyticsFile } from "../src/layers/session-persistence"

// ---------------------------------------------------------------------------
// Fixture helpers
// ---------------------------------------------------------------------------

/** Build a minimal valid SessionAnalyticsFile for testing. */
function makeSessionFile(
  overrides: Partial<SessionAnalyticsFile> = {},
): SessionAnalyticsFile {
  return {
    session_id: "sess-test-001",
    total_cost_usd: 0.42,
    total_tokens: { prompt: 10000, completion: 3000 },
    by_model: [
      { model: "claude-sonnet-4-20250514", cost_usd: 0.38, calls: 10 },
      { model: "claude-haiku-4-20250414", cost_usd: 0.04, calls: 3 },
    ],
    duration_minutes: 5,
    messages: 12,
    tools_used: ["bash", "read"],
    started_at: "2026-04-12T09:00:00.000Z",
    ended_at: "2026-04-12T09:05:00.000Z",
    file_path: "/repo/.axiom/sessions/sess-test-001.json",
    ...overrides,
  }
}

/** Serialize a SessionAnalyticsFile to JSON string. */
function toJson(file: SessionAnalyticsFile): string {
  return JSON.stringify(file)
}

/** Build a FileSource with injected FS helpers (no real I/O). */
function makeFileSource(
  files: Record<string, SessionAnalyticsFile | string>,
  opts: { sessionsDir?: string } = {},
) {
  const sessionsDir = opts.sessionsDir ?? "/fake/sessions"

  const _listFiles = (_dir: string): string[] => Object.keys(files)

  const _readFile = (path: string): string => {
    // Extract filename from path
    const fileName = path.split("/").pop()!
    const entry = files[fileName]
    if (entry === undefined) throw new Error(`File not found: ${path}`)
    if (typeof entry === "string") return entry
    return toJson(entry)
  }

  return new FileSource({
    repoRoot: "/fake/repo",
    sessionsDir,
    _readFile,
    _listFiles,
  })
}

// ===========================================================================
// getSessions() — basic shape
// ===========================================================================

describe("FileSource.getSessions() — basic shape", () => {
  it("returns a correctly shaped SessionRow from a valid session file", async () => {
    const source = makeFileSource({
      "sess-test-001.json": makeSessionFile(),
    })

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(1)

    const row = sessions[0]
    expect(row.sessionId).toBe("sess-test-001")
    expect(row.source).toBe("file")
    expect(row.status).toBe("stopped")
    expect(row.isTeam).toBe(false)
    expect(row.updatedAt).toBeInstanceOf(Date)
  })

  it("costUsd comes from total_cost_usd", async () => {
    const source = makeFileSource({
      "sess-cost.json": makeSessionFile({ total_cost_usd: 1.23 }),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].costUsd).toBeCloseTo(1.23, 4)
  })

  it("totalTokens = prompt + completion", async () => {
    const source = makeFileSource({
      "sess-tokens.json": makeSessionFile({
        total_tokens: { prompt: 8000, completion: 2500 },
      }),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].totalTokens).toBe(10500)
  })

  it("durationSeconds = duration_minutes * 60", async () => {
    const source = makeFileSource({
      "sess-duration.json": makeSessionFile({ duration_minutes: 7.5 }),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].durationSeconds).toBeCloseTo(450, 1)
  })

  it("status is always 'stopped' for file-sourced sessions", async () => {
    const source = makeFileSource({
      "sess-status.json": makeSessionFile(),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].status).toBe("stopped")
  })

  it("source is always 'file'", async () => {
    const source = makeFileSource({
      "sess-source.json": makeSessionFile(),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].source).toBe("file")
  })

  it("isTeam is always false", async () => {
    const source = makeFileSource({
      "sess-team.json": makeSessionFile(),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].isTeam).toBe(false)
  })

  it("updatedAt is parsed from ended_at", async () => {
    const source = makeFileSource({
      "sess-date.json": makeSessionFile({
        ended_at: "2026-04-12T09:05:00.000Z",
      }),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].updatedAt).toEqual(new Date("2026-04-12T09:05:00.000Z"))
  })
})

// ===========================================================================
// getSessions() — skip rules
// ===========================================================================

describe("FileSource.getSessions() — skip rules", () => {
  it("skips corrupted JSON files without crashing", async () => {
    const source = makeFileSource({
      "good.json": makeSessionFile({ session_id: "good-session" }),
      "bad.json": "{ this is not valid json !!!",
    })

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(1)
    expect(sessions[0].sessionId).toBe("good-session")
  })

  it("skips files where dry_run === true", async () => {
    const source = makeFileSource({
      "dry.json": makeSessionFile({ session_id: "dry-session", dry_run: true }),
      "real.json": makeSessionFile({ session_id: "real-session" }),
    })

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(1)
    expect(sessions[0].sessionId).toBe("real-session")
  })

  it("skips .tmp files", async () => {
    // .tmp files should not be listed by _listFiles in real usage,
    // but we test the filter explicitly here.
    const sessionsDir = "/fake/sessions"
    const _listFiles = (_dir: string) => [
      "good.json",
      "partial.json.tmp",
      "another.tmp",
    ]
    const _readFile = (path: string): string => {
      const fileName = path.split("/").pop()!
      if (fileName === "good.json") {
        return toJson(makeSessionFile({ session_id: "good-session" }))
      }
      throw new Error(`Unexpected read: ${path}`)
    }

    const source = new FileSource({
      repoRoot: "/fake/repo",
      sessionsDir,
      _readFile,
      _listFiles,
    })

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(1)
    expect(sessions[0].sessionId).toBe("good-session")
  })

  it("skips non-.json files", async () => {
    const sessionsDir = "/fake/sessions"
    const _listFiles = (_dir: string) => ["session.json", "README.md", ".gitkeep"]
    const _readFile = (path: string): string => {
      const fileName = path.split("/").pop()!
      if (fileName === "session.json") {
        return toJson(makeSessionFile({ session_id: "only-json" }))
      }
      throw new Error(`Unexpected read: ${path}`)
    }

    const source = new FileSource({
      repoRoot: "/fake/repo",
      sessionsDir,
      _readFile,
      _listFiles,
    })

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(1)
    expect(sessions[0].sessionId).toBe("only-json")
  })
})

// ===========================================================================
// getSessions() — multiple files
// ===========================================================================

describe("FileSource.getSessions() — multiple files", () => {
  it("returns all valid session files", async () => {
    const source = makeFileSource({
      "sess-a.json": makeSessionFile({ session_id: "sess-a" }),
      "sess-b.json": makeSessionFile({ session_id: "sess-b" }),
      "sess-c.json": makeSessionFile({ session_id: "sess-c" }),
    })

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(3)

    const ids = sessions.map((s) => s.sessionId).sort()
    expect(ids).toEqual(["sess-a", "sess-b", "sess-c"])
  })

  it("returns empty array when no files exist", async () => {
    const source = makeFileSource({})

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(0)
  })
})

// ===========================================================================
// isAvailable()
// ===========================================================================

describe("FileSource.isAvailable()", () => {
  it("returns false when sessions dir does not exist (no injection)", () => {
    // Use a path that definitely doesn't exist, no _listFiles injection
    const source = new FileSource({
      repoRoot: "/nonexistent/repo/path/that/does/not/exist",
    })
    expect(source.isAvailable()).toBe(false)
  })

  it("returns true when sessions dir exists on real filesystem", () => {
    // Use the test file's own directory — guaranteed to exist in any checkout
    const source = new FileSource({
      repoRoot: import.meta.dir,
      sessionsDir: import.meta.dir,
    })
    expect(source.isAvailable()).toBe(true)
  })
})

// ===========================================================================
// getSessions() — returns empty array when sessions dir doesn't exist
// ===========================================================================

describe("FileSource.getSessions() — missing sessions dir", () => {
  it("returns empty array when sessions dir does not exist", async () => {
    // No _listFiles injection — real fs check on nonexistent path
    const source = new FileSource({
      repoRoot: "/nonexistent/repo/path/that/does/not/exist",
    })

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(0)
  })
})

// ===========================================================================
// getLastUpdate()
// ===========================================================================

describe("FileSource.getLastUpdate()", () => {
  it("returns null when no files have been read", () => {
    const source = makeFileSource({})
    expect(source.getLastUpdate()).toBeNull()
  })

  it("returns null before getSessions() is called", () => {
    const source = makeFileSource({
      "sess.json": makeSessionFile(),
    })
    // Before calling getSessions(), lastUpdate is null
    expect(source.getLastUpdate()).toBeNull()
  })

  it("returns the most recent ended_at after getSessions()", async () => {
    const source = makeFileSource({
      "older.json": makeSessionFile({
        session_id: "older",
        ended_at: "2026-04-10T08:00:00.000Z",
      }),
      "newer.json": makeSessionFile({
        session_id: "newer",
        ended_at: "2026-04-12T09:05:00.000Z",
      }),
    })

    await source.getSessions()
    const lastUpdate = source.getLastUpdate()
    expect(lastUpdate).not.toBeNull()
    expect(lastUpdate!.toISOString()).toBe("2026-04-12T09:05:00.000Z")
  })

  it("returns null when no files exist after getSessions()", async () => {
    const source = makeFileSource({})
    await source.getSessions()
    expect(source.getLastUpdate()).toBeNull()
  })
})

// ===========================================================================
// Model alias lookup
// ===========================================================================

describe("FileSource — model alias lookup", () => {
  it("resolves known model alias: claude-sonnet-4-20250514 → Claude Sonnet 4", async () => {
    const source = makeFileSource({
      "sess.json": makeSessionFile({
        by_model: [{ model: "claude-sonnet-4-20250514", cost_usd: 0.1, calls: 5 }],
      }),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Claude Sonnet 4")
    expect(sessions[0].modelRaw).toBe("claude-sonnet-4-20250514")
  })

  it("resolves known model alias: amazon-bedrock/anthropic.claude-sonnet-4-6 → Claude Sonnet 4.6", async () => {
    const source = makeFileSource({
      "sess.json": makeSessionFile({
        by_model: [
          {
            model: "amazon-bedrock/anthropic.claude-sonnet-4-6",
            cost_usd: 0.1,
            calls: 8,
          },
        ],
      }),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Claude Sonnet 4.6")
    expect(sessions[0].modelRaw).toBe("amazon-bedrock/anthropic.claude-sonnet-4-6")
  })

  it("resolves known model alias: claude-3-5-sonnet-20241022 → Claude Sonnet 3.5", async () => {
    const source = makeFileSource({
      "sess.json": makeSessionFile({
        by_model: [{ model: "claude-3-5-sonnet-20241022", cost_usd: 0.05, calls: 3 }],
      }),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Claude Sonnet 3.5")
  })

  it("resolves known model alias: anthropic.claude-opus-4-6-v1 → Claude Opus 4.6", async () => {
    const source = makeFileSource({
      "sess.json": makeSessionFile({
        by_model: [{ model: "anthropic.claude-opus-4-6-v1", cost_usd: 0.5, calls: 2 }],
      }),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Claude Opus 4.6")
  })

  it("resolves known model alias: claude-3-5-haiku-20241022 → Claude Haiku 3.5", async () => {
    const source = makeFileSource({
      "sess.json": makeSessionFile({
        by_model: [{ model: "claude-3-5-haiku-20241022", cost_usd: 0.01, calls: 7 }],
      }),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Claude Haiku 3.5")
  })

  it("resolves known model alias: claude-haiku-4-20250414 → Claude Haiku 4", async () => {
    const source = makeFileSource({
      "sess.json": makeSessionFile({
        by_model: [{ model: "claude-haiku-4-20250414", cost_usd: 0.01, calls: 4 }],
      }),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Claude Haiku 4")
  })

  it("resolves known model alias: opencode/big-pickle → Big Pickle", async () => {
    const source = makeFileSource({
      "sess.json": makeSessionFile({
        by_model: [{ model: "opencode/big-pickle", cost_usd: 0.0, calls: 1 }],
      }),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Big Pickle")
  })

  it("returns full raw ID for unknown model IDs (no truncation)", async () => {
    const longModelId = "some-unknown-model-with-a-very-long-name"
    const source = makeFileSource({
      "sess.json": makeSessionFile({
        by_model: [{ model: longModelId, cost_usd: 0.0, calls: 1 }],
      }),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe(longModelId)
    expect(sessions[0].modelRaw).toBe(longModelId)
  })

  it("keeps short unknown model IDs as-is (no truncation)", async () => {
    const shortModelId = "gpt-4o"
    const source = makeFileSource({
      "sess.json": makeSessionFile({
        by_model: [{ model: shortModelId, cost_usd: 0.0, calls: 1 }],
      }),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("gpt-4o")
  })

  it("picks the model with the most calls from by_model", async () => {
    const source = makeFileSource({
      "sess.json": makeSessionFile({
        by_model: [
          { model: "claude-haiku-4-20250414", cost_usd: 0.01, calls: 3 },
          { model: "claude-sonnet-4-20250514", cost_usd: 0.38, calls: 10 }, // most calls
          { model: "anthropic.claude-opus-4-6-v1", cost_usd: 0.5, calls: 1 },
        ],
      }),
    })

    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Claude Sonnet 4")
    expect(sessions[0].modelRaw).toBe("claude-sonnet-4-20250514")
  })

  it("handles empty by_model array gracefully", async () => {
    const source = makeFileSource({
      "sess.json": makeSessionFile({ by_model: [] }),
    })

    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(1)
    expect(sessions[0].model).toBe("")
    expect(sessions[0].modelRaw).toBe("")
  })
})

// ===========================================================================
// SessionDataSource interface compliance
// ===========================================================================

describe("FileSource — SessionDataSource interface compliance", () => {
  it("has id = 'file'", () => {
    const source = makeFileSource({})
    expect(source.id).toBe("file")
  })

  it("has priority = 2", () => {
    const source = makeFileSource({})
    expect(source.priority).toBe(2)
  })

  it("getSessions() returns a Promise", () => {
    const source = makeFileSource({})
    const result = source.getSessions()
    expect(result).toBeInstanceOf(Promise)
  })

  it("getLastUpdate() returns Date | null", () => {
    const source = makeFileSource({})
    const result = source.getLastUpdate()
    expect(result === null || result instanceof Date).toBe(true)
  })

  it("isAvailable() returns boolean", () => {
    const source = makeFileSource({})
    const result = source.isAvailable()
    expect(typeof result).toBe("boolean")
  })
})

// ===========================================================================
// MODEL_ALIASES export
// ===========================================================================

describe("MODEL_DISPLAY_ALIASES export", () => {
  it("exports MODEL_DISPLAY_ALIASES as a Record<string, string>", () => {
    expect(typeof MODEL_DISPLAY_ALIASES).toBe("object")
    expect(MODEL_DISPLAY_ALIASES["claude-sonnet-4-20250514"]).toBe("Claude Sonnet 4")
    expect(MODEL_DISPLAY_ALIASES["amazon-bedrock/anthropic.claude-sonnet-4-6"]).toBe("Claude Sonnet 4.6")
    expect(MODEL_DISPLAY_ALIASES["anthropic.claude-opus-4-6-v1"]).toBe("Claude Opus 4.6")
    expect(MODEL_DISPLAY_ALIASES["claude-3-5-haiku-20241022"]).toBe("Claude Haiku 3.5")
    expect(MODEL_DISPLAY_ALIASES["claude-haiku-4-20250414"]).toBe("Claude Haiku 4")
    expect(MODEL_DISPLAY_ALIASES["opencode/big-pickle"]).toBe("Big Pickle")
  })
})

// ===========================================================================
// MAX_SESSION_FILES limit (step-verify-H003)
// REQ-TUI-054: limit to most recent N sessions for performance (DA-001 fix)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-054 plan=step-verify-H003
// ===========================================================================

describe("MAX_SESSION_FILES limit (step-verify-H003)", () => {
  /**
   * Build a FileSource with injected _listFiles, _readFile, and _statFile.
   * mtimes: Record<filename, mtimeMs> — controls sort order.
   */
  function makeFileSourceWithMtimes(
    files: Record<string, SessionAnalyticsFile | string>,
    mtimes: Record<string, number>,
  ) {
    const sessionsDir = "/fake/sessions"

    const _listFiles = (_dir: string): string[] => Object.keys(files)

    const _readFile = (path: string): string => {
      const fileName = path.split("/").pop()!
      const entry = files[fileName]
      if (entry === undefined) throw new Error(`File not found: ${path}`)
      if (typeof entry === "string") return entry
      return JSON.stringify(entry)
    }

    const _statFile = (path: string): { mtimeMs: number } => {
      const fileName = path.split("/").pop()!
      const mtime = mtimes[fileName]
      if (mtime === undefined) return { mtimeMs: 0 }
      return { mtimeMs: mtime }
    }

    return new FileSource({
      repoRoot: "/fake/repo",
      sessionsDir,
      _readFile,
      _listFiles,
      _statFile,
    })
  }

  it("limits to 50 most recent files when more than 50 exist", async () => {
    // Create 60 mock files with different mtimes (newer = higher mtime)
    const files: Record<string, SessionAnalyticsFile> = {}
    const mtimes: Record<string, number> = {}
    for (let i = 1; i <= 60; i++) {
      const name = `sess-${String(i).padStart(3, "0")}.json`
      files[name] = {
        session_id: `sess-${i}`,
        total_cost_usd: 0.01,
        total_tokens: { prompt: 100, completion: 50 },
        by_model: [{ model: "claude-sonnet-4-20250514", cost_usd: 0.01, calls: 1 }],
        duration_minutes: 1,
        messages: 2,
        tools_used: [],
        started_at: "2026-04-12T09:00:00.000Z",
        ended_at: "2026-04-12T09:01:00.000Z",
        file_path: `/fake/sessions/${name}`,
      }
      // File i has mtime = i * 1000 (file 60 is newest)
      mtimes[name] = i * 1000
    }

    const source = makeFileSourceWithMtimes(files, mtimes)
    const sessions = await source.getSessions()

    // Must return exactly MAX_SESSION_FILES (50) sessions
    expect(sessions).toHaveLength(MAX_SESSION_FILES)
    expect(sessions).toHaveLength(50)
  })

  it("returns all files when fewer than 50 exist", async () => {
    const files: Record<string, SessionAnalyticsFile> = {}
    const mtimes: Record<string, number> = {}
    for (let i = 1; i <= 10; i++) {
      const name = `sess-${i}.json`
      files[name] = {
        session_id: `sess-${i}`,
        total_cost_usd: 0.01,
        total_tokens: { prompt: 100, completion: 50 },
        by_model: [{ model: "claude-sonnet-4-20250514", cost_usd: 0.01, calls: 1 }],
        duration_minutes: 1,
        messages: 2,
        tools_used: [],
        started_at: "2026-04-12T09:00:00.000Z",
        ended_at: "2026-04-12T09:01:00.000Z",
        file_path: `/fake/sessions/${name}`,
      }
      mtimes[name] = i * 1000
    }

    const source = makeFileSourceWithMtimes(files, mtimes)
    const sessions = await source.getSessions()

    // All 10 files should be returned (well under the 50 limit)
    expect(sessions).toHaveLength(10)
  })

  it("sorts by mtime descending — most recent sessions first", async () => {
    // 3 files with distinct mtimes; verify order: newest first
    const files: Record<string, SessionAnalyticsFile> = {
      "old.json": {
        session_id: "old-session",
        total_cost_usd: 0.01,
        total_tokens: { prompt: 100, completion: 50 },
        by_model: [{ model: "claude-sonnet-4-20250514", cost_usd: 0.01, calls: 1 }],
        duration_minutes: 1,
        messages: 2,
        tools_used: [],
        started_at: "2026-04-10T08:00:00.000Z",
        ended_at: "2026-04-10T08:01:00.000Z",
        file_path: "/fake/sessions/old.json",
      },
      "mid.json": {
        session_id: "mid-session",
        total_cost_usd: 0.01,
        total_tokens: { prompt: 100, completion: 50 },
        by_model: [{ model: "claude-sonnet-4-20250514", cost_usd: 0.01, calls: 1 }],
        duration_minutes: 1,
        messages: 2,
        tools_used: [],
        started_at: "2026-04-11T08:00:00.000Z",
        ended_at: "2026-04-11T08:01:00.000Z",
        file_path: "/fake/sessions/mid.json",
      },
      "new.json": {
        session_id: "new-session",
        total_cost_usd: 0.01,
        total_tokens: { prompt: 100, completion: 50 },
        by_model: [{ model: "claude-sonnet-4-20250514", cost_usd: 0.01, calls: 1 }],
        duration_minutes: 1,
        messages: 2,
        tools_used: [],
        started_at: "2026-04-12T08:00:00.000Z",
        ended_at: "2026-04-12T08:01:00.000Z",
        file_path: "/fake/sessions/new.json",
      },
    }
    const mtimes: Record<string, number> = {
      "old.json": 1000,  // oldest
      "mid.json": 2000,  // middle
      "new.json": 3000,  // newest
    }

    const source = makeFileSourceWithMtimes(files, mtimes)
    const sessions = await source.getSessions()

    expect(sessions).toHaveLength(3)
    // Newest file (mtime=3000) must come first
    expect(sessions[0].sessionId).toBe("new-session")
    expect(sessions[1].sessionId).toBe("mid-session")
    expect(sessions[2].sessionId).toBe("old-session")
  })

  it("returns the 50 most recent files (not the oldest) when 60 exist", async () => {
    // Files 1-60; files 11-60 are the 50 most recent (mtime 11000-60000)
    // Files 1-10 are the oldest (mtime 1000-10000) and should be excluded
    const files: Record<string, SessionAnalyticsFile> = {}
    const mtimes: Record<string, number> = {}
    for (let i = 1; i <= 60; i++) {
      const name = `sess-${String(i).padStart(3, "0")}.json`
      files[name] = {
        session_id: `sess-${i}`,
        total_cost_usd: 0.01,
        total_tokens: { prompt: 100, completion: 50 },
        by_model: [{ model: "claude-sonnet-4-20250514", cost_usd: 0.01, calls: 1 }],
        duration_minutes: 1,
        messages: 2,
        tools_used: [],
        started_at: "2026-04-12T09:00:00.000Z",
        ended_at: "2026-04-12T09:01:00.000Z",
        file_path: `/fake/sessions/${name}`,
      }
      mtimes[name] = i * 1000
    }

    const source = makeFileSourceWithMtimes(files, mtimes)
    const sessions = await source.getSessions()

    const returnedIds = sessions.map((s) => s.sessionId)

    // The 10 oldest sessions (sess-1 through sess-10) must NOT be in the result
    for (let i = 1; i <= 10; i++) {
      expect(returnedIds).not.toContain(`sess-${i}`)
    }
    // The 50 most recent sessions (sess-11 through sess-60) MUST be in the result
    for (let i = 11; i <= 60; i++) {
      expect(returnedIds).toContain(`sess-${i}`)
    }
  })
})
