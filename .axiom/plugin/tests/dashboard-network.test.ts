/**
 * Tests for NetworkSource — reads team session data from the L4 pulse API.
 *
 * All tests use injected `_fetch` — no real network calls are made.
 *
 * REQ-TUI-040: Team sessions shown as additional rows when L4 enabled.
 * REQ-TUI-041: Team rows show name, work item, model, duration, status only.
 *   Cost and token data MUST NOT be shown for team sessions.
 * REQ-TUI-042: Team visibility respects network.visibility setting.
 *   Hidden members MUST be filtered out.
 * REQ-TUI-044: Team data uses pulse API; handles failures gracefully.
 *
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-040
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-040 plan=phase-4/task-4.1 test=dashboard-network.test.ts
 */

import { describe, it, expect } from "bun:test"
import {
  NetworkSource,
  createNetworkSource,
  type TeamMember,
  type NetworkSourceOptions,
} from "../src/dashboard/sources/network"
import { MODEL_DISPLAY_ALIASES } from "../src/dashboard/model-aliases"

// ---------------------------------------------------------------------------
// Fixture helpers
// ---------------------------------------------------------------------------

/** Build a minimal valid TeamMember for testing. */
function makeMember(overrides: Partial<TeamMember> = {}): TeamMember {
  return {
    user_id: "user-abc",
    work_item: "AUTH-234",
    model: "claude-sonnet-4-20250514",
    status: "active",
    last_seen: new Date(Date.now() - 60_000).toISOString(), // 60s ago
    visibility: "public",
    ...overrides,
  }
}

/**
 * Build a NetworkSource with an injected fetch function.
 * The injected fetch returns the provided members array.
 */
function makeNetworkSource(
  members: TeamMember[],
  opts: Partial<NetworkSourceOptions> = {},
): NetworkSource {
  return new NetworkSource({
    backendUrl: "https://backend.example.com",
    apiToken: "test-token",
    _fetch: async (_url: string, _token: string) => members,
    ...opts,
  })
}

/**
 * Build a NetworkSource whose injected fetch throws an error.
 */
function makeErrorSource(errorMessage: string): NetworkSource {
  return new NetworkSource({
    backendUrl: "https://backend.example.com",
    apiToken: "test-token",
    _fetch: async () => {
      throw new Error(errorMessage)
    },
  })
}

// ===========================================================================
// getSessions() — basic shape
// ===========================================================================

describe("NetworkSource.getSessions() — basic shape", () => {
  it("returns correctly shaped SessionRow[] with isTeam=true", async () => {
    const source = makeNetworkSource([makeMember()])
    const sessions = await source.getSessions()

    expect(sessions).toHaveLength(1)
    const row = sessions[0]

    expect(row.sessionId).toBe("team-user-abc")
    expect(row.source).toBe("network")
    expect(row.isTeam).toBe(true)
    expect(row.teamMemberId).toBe("user-abc")
    expect(row.updatedAt).toBeInstanceOf(Date)
  })

  it("sessionId is 'team-<user_id>'", async () => {
    const source = makeNetworkSource([makeMember({ user_id: "user-xyz" })])
    const sessions = await source.getSessions()
    expect(sessions[0].sessionId).toBe("team-user-xyz")
  })

  it("workItemId matches member.work_item", async () => {
    const source = makeNetworkSource([makeMember({ work_item: "PROJ-999" })])
    const sessions = await source.getSessions()
    expect(sessions[0].workItemId).toBe("PROJ-999")
  })

  it("workItemId is undefined when member has no work_item", async () => {
    const source = makeNetworkSource([makeMember({ work_item: undefined })])
    const sessions = await source.getSessions()
    expect(sessions[0].workItemId).toBeUndefined()
  })

  it("source is always 'network'", async () => {
    const source = makeNetworkSource([makeMember()])
    const sessions = await source.getSessions()
    expect(sessions[0].source).toBe("network")
  })

  it("isTeam is always true", async () => {
    const source = makeNetworkSource([makeMember()])
    const sessions = await source.getSessions()
    expect(sessions[0].isTeam).toBe(true)
  })

  it("teamMemberId matches user_id", async () => {
    const source = makeNetworkSource([makeMember({ user_id: "member-42" })])
    const sessions = await source.getSessions()
    expect(sessions[0].teamMemberId).toBe("member-42")
  })

  it("updatedAt is parsed from last_seen", async () => {
    const lastSeen = "2026-04-12T10:00:00.000Z"
    const source = makeNetworkSource([makeMember({ last_seen: lastSeen })])
    const sessions = await source.getSessions()
    expect(sessions[0].updatedAt).toEqual(new Date(lastSeen))
  })

  it("returns multiple rows for multiple visible members", async () => {
    const source = makeNetworkSource([
      makeMember({ user_id: "user-1" }),
      makeMember({ user_id: "user-2" }),
      makeMember({ user_id: "user-3" }),
    ])
    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(3)
    const ids = sessions.map((s) => s.sessionId).sort()
    expect(ids).toEqual(["team-user-1", "team-user-2", "team-user-3"])
  })

  it("returns empty array when no members", async () => {
    const source = makeNetworkSource([])
    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(0)
  })
})

// ===========================================================================
// REQ-TUI-041: costUsd and totalTokens are always 0
// ===========================================================================

describe("NetworkSource — REQ-TUI-041: cost and token suppression", () => {
  it("costUsd is always 0 for team sessions", async () => {
    const source = makeNetworkSource([makeMember()])
    const sessions = await source.getSessions()
    expect(sessions[0].costUsd).toBe(0)
  })

  it("totalTokens is always 0 for team sessions", async () => {
    const source = makeNetworkSource([makeMember()])
    const sessions = await source.getSessions()
    expect(sessions[0].totalTokens).toBe(0)
  })

  it("costUsd is 0 even for active members", async () => {
    const source = makeNetworkSource([makeMember({ status: "active" })])
    const sessions = await source.getSessions()
    expect(sessions[0].costUsd).toBe(0)
  })

  it("totalTokens is 0 even for active members", async () => {
    const source = makeNetworkSource([makeMember({ status: "active" })])
    const sessions = await source.getSessions()
    expect(sessions[0].totalTokens).toBe(0)
  })
})

// ===========================================================================
// REQ-TUI-042: hidden members filtered out
// ===========================================================================

describe("NetworkSource — REQ-TUI-042: visibility filtering", () => {
  it("filters out members with visibility='hidden'", async () => {
    const source = makeNetworkSource([
      makeMember({ user_id: "visible-1", visibility: "public" }),
      makeMember({ user_id: "hidden-user", visibility: "hidden" }),
      makeMember({ user_id: "visible-2", visibility: "public" }),
    ])
    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(2)
    const ids = sessions.map((s) => s.teamMemberId)
    expect(ids).toContain("visible-1")
    expect(ids).toContain("visible-2")
    expect(ids).not.toContain("hidden-user")
  })

  it("returns empty array when all members are hidden", async () => {
    const source = makeNetworkSource([
      makeMember({ user_id: "hidden-1", visibility: "hidden" }),
      makeMember({ user_id: "hidden-2", visibility: "hidden" }),
    ])
    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(0)
  })

  it("includes public members when mixed with hidden", async () => {
    const source = makeNetworkSource([
      makeMember({ user_id: "public-user", visibility: "public" }),
      makeMember({ user_id: "hidden-user", visibility: "hidden" }),
    ])
    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(1)
    expect(sessions[0].teamMemberId).toBe("public-user")
  })
})

// ===========================================================================
// Status mapping: "active" → "running", "idle" → "idle", "offline" → "stopped"
// ===========================================================================

describe("NetworkSource — status mapping", () => {
  it("maps 'active' → 'running'", async () => {
    const source = makeNetworkSource([makeMember({ status: "active" })])
    const sessions = await source.getSessions()
    expect(sessions[0].status).toBe("running")
  })

  it("maps 'idle' → 'idle'", async () => {
    const source = makeNetworkSource([makeMember({ status: "idle" })])
    const sessions = await source.getSessions()
    expect(sessions[0].status).toBe("idle")
  })

  it("maps 'offline' → 'stopped'", async () => {
    const source = makeNetworkSource([makeMember({ status: "offline" })])
    const sessions = await source.getSessions()
    expect(sessions[0].status).toBe("stopped")
  })
})

// ===========================================================================
// REQ-TUI-044: Network error handling
// ===========================================================================

describe("NetworkSource — REQ-TUI-044: network error handling", () => {
  it("returns [] when fetch throws an error", async () => {
    const source = makeErrorSource("Connection refused")
    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(0)
  })

  it("getLastError() returns error message after failed fetch", async () => {
    const source = makeErrorSource("Connection refused")
    await source.getSessions()
    expect(source.getLastError()).toBe("Connection refused")
  })

  it("getLastError() returns null before any fetch", () => {
    const source = makeNetworkSource([])
    expect(source.getLastError()).toBeNull()
  })

  it("getLastError() returns null after successful fetch", async () => {
    const source = makeNetworkSource([makeMember()])
    await source.getSessions()
    expect(source.getLastError()).toBeNull()
  })

  it("clears previous error after successful fetch", async () => {
    // First call fails
    let shouldFail = true
    const source = new NetworkSource({
      backendUrl: "https://backend.example.com",
      apiToken: "test-token",
      _fetch: async () => {
        if (shouldFail) throw new Error("Temporary failure")
        return [makeMember()]
      },
    })

    await source.getSessions()
    expect(source.getLastError()).toBe("Temporary failure")

    // Second call succeeds
    shouldFail = false
    await source.getSessions()
    expect(source.getLastError()).toBeNull()
  })

  it("does not crash when fetch returns non-Error rejection", async () => {
    const source = new NetworkSource({
      backendUrl: "https://backend.example.com",
      apiToken: "test-token",
      _fetch: async () => {
        throw "string error"
      },
    })
    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(0)
    expect(source.getLastError()).toBe("string error")
  })
})

// ===========================================================================
// isAvailable()
// ===========================================================================

describe("NetworkSource.isAvailable()", () => {
  it("returns true when backendUrl is set", () => {
    const source = makeNetworkSource([])
    expect(source.isAvailable()).toBe(true)
  })

  it("returns false when backendUrl is empty string", () => {
    const source = new NetworkSource({
      backendUrl: "",
      apiToken: "test-token",
      _fetch: async () => [],
    })
    expect(source.isAvailable()).toBe(false)
  })

  it("returns false when backendUrl is whitespace only", () => {
    const source = new NetworkSource({
      backendUrl: "   ",
      apiToken: "test-token",
      _fetch: async () => [],
    })
    expect(source.isAvailable()).toBe(false)
  })

  it("getSessions() returns [] when not available (empty backendUrl)", async () => {
    const source = new NetworkSource({
      backendUrl: "",
      apiToken: "test-token",
      _fetch: async () => {
        throw new Error("Should not be called")
      },
    })
    const sessions = await source.getSessions()
    expect(sessions).toHaveLength(0)
  })
})

// ===========================================================================
// getLastUpdate()
// ===========================================================================

describe("NetworkSource.getLastUpdate()", () => {
  it("returns null before any fetch", () => {
    const source = makeNetworkSource([])
    expect(source.getLastUpdate()).toBeNull()
  })

  it("returns a Date after a successful fetch", async () => {
    const before = Date.now()
    const source = makeNetworkSource([makeMember()])
    await source.getSessions()
    const after = Date.now()
    const lastUpdate = source.getLastUpdate()
    expect(lastUpdate).toBeInstanceOf(Date)
    expect(lastUpdate!.getTime()).toBeGreaterThanOrEqual(before)
    expect(lastUpdate!.getTime()).toBeLessThanOrEqual(after)
  })

  it("returns null after a failed fetch (no update on error)", async () => {
    const source = makeErrorSource("Network error")
    await source.getSessions()
    expect(source.getLastUpdate()).toBeNull()
  })

  it("updates lastUpdate timestamp on each successful fetch", async () => {
    const source = makeNetworkSource([makeMember()])
    await source.getSessions()
    const first = source.getLastUpdate()!.getTime()

    // Small delay to ensure timestamp advances
    await new Promise((resolve) => setTimeout(resolve, 5))

    await source.getSessions()
    const second = source.getLastUpdate()!.getTime()
    expect(second).toBeGreaterThanOrEqual(first)
  })
})

// ===========================================================================
// Model alias lookup
// ===========================================================================

describe("NetworkSource — model alias lookup", () => {
  it("resolves known model alias: claude-sonnet-4-20250514 → Claude Sonnet 4", async () => {
    const source = makeNetworkSource([makeMember({ model: "claude-sonnet-4-20250514" })])
    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Claude Sonnet 4")
    expect(sessions[0].modelRaw).toBe("claude-sonnet-4-20250514")
  })

  it("resolves known model alias: amazon-bedrock/anthropic.claude-sonnet-4-6 → Claude Sonnet 4.6", async () => {
    const source = makeNetworkSource([
      makeMember({ model: "amazon-bedrock/anthropic.claude-sonnet-4-6" }),
    ])
    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Claude Sonnet 4.6")
    expect(sessions[0].modelRaw).toBe("amazon-bedrock/anthropic.claude-sonnet-4-6")
  })

  it("resolves known model alias: anthropic.claude-opus-4-6-v1 → Claude Opus 4.6", async () => {
    const source = makeNetworkSource([makeMember({ model: "anthropic.claude-opus-4-6-v1" })])
    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Claude Opus 4.6")
  })

  it("resolves known model alias: claude-3-5-haiku-20241022 → Claude Haiku 3.5", async () => {
    const source = makeNetworkSource([makeMember({ model: "claude-3-5-haiku-20241022" })])
    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Claude Haiku 3.5")
  })

  it("resolves known model alias: claude-haiku-4-20250414 → Claude Haiku 4", async () => {
    const source = makeNetworkSource([makeMember({ model: "claude-haiku-4-20250414" })])
    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Claude Haiku 4")
  })

  it("resolves known model alias: opencode/big-pickle → Big Pickle", async () => {
    const source = makeNetworkSource([makeMember({ model: "opencode/big-pickle" })])
    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Big Pickle")
  })

  it("returns full raw ID for unknown model IDs (no truncation)", async () => {
    const longModelId = "some-unknown-model-with-a-very-long-name"
    const source = makeNetworkSource([makeMember({ model: longModelId })])
    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe(longModelId)
    expect(sessions[0].modelRaw).toBe(longModelId)
  })

  it("keeps short unknown model IDs as-is (no truncation)", async () => {
    const source = makeNetworkSource([makeMember({ model: "gpt-4o" })])
    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("gpt-4o")
  })

  it("returns 'Unknown' when model is undefined", async () => {
    const source = makeNetworkSource([makeMember({ model: undefined })])
    const sessions = await source.getSessions()
    expect(sessions[0].model).toBe("Unknown")
    expect(sessions[0].modelRaw).toBe("")
  })
})

// ===========================================================================
// durationSeconds computation
// ===========================================================================

describe("NetworkSource — durationSeconds", () => {
  it("computes durationSeconds from last_seen to now", async () => {
    const lastSeenMs = Date.now() - 120_000 // 120 seconds ago
    const lastSeen = new Date(lastSeenMs).toISOString()
    const source = makeNetworkSource([makeMember({ last_seen: lastSeen })])
    const sessions = await source.getSessions()
    // Allow ±2s tolerance for test execution time
    expect(sessions[0].durationSeconds).toBeGreaterThanOrEqual(118)
    expect(sessions[0].durationSeconds).toBeLessThanOrEqual(122)
  })

  it("returns 0 durationSeconds for invalid last_seen", async () => {
    const source = makeNetworkSource([makeMember({ last_seen: "not-a-date" })])
    const sessions = await source.getSessions()
    expect(sessions[0].durationSeconds).toBe(0)
  })

  it("returns 0 durationSeconds when last_seen is in the future", async () => {
    const futureTime = new Date(Date.now() + 60_000).toISOString()
    const source = makeNetworkSource([makeMember({ last_seen: futureTime })])
    const sessions = await source.getSessions()
    expect(sessions[0].durationSeconds).toBe(0)
  })
})

// ===========================================================================
// SessionDataSource interface compliance
// ===========================================================================

describe("NetworkSource — SessionDataSource interface compliance", () => {
  it("has id = 'network'", () => {
    const source = makeNetworkSource([])
    expect(source.id).toBe("network")
  })

  it("has priority = 1", () => {
    const source = makeNetworkSource([])
    expect(source.priority).toBe(1)
  })

  it("getSessions() returns a Promise", () => {
    const source = makeNetworkSource([])
    const result = source.getSessions()
    expect(result).toBeInstanceOf(Promise)
  })

  it("getLastUpdate() returns Date | null", () => {
    const source = makeNetworkSource([])
    const result = source.getLastUpdate()
    expect(result === null || result instanceof Date).toBe(true)
  })

  it("isAvailable() returns boolean", () => {
    const source = makeNetworkSource([])
    const result = source.isAvailable()
    expect(typeof result).toBe("boolean")
  })

  it("createNetworkSource factory returns a NetworkSource", () => {
    const source = createNetworkSource({
      backendUrl: "https://backend.example.com",
      apiToken: "test-token",
      _fetch: async () => [],
    })
    expect(source).toBeInstanceOf(NetworkSource)
    expect(source.id).toBe("network")
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
// getLastError() — additional edge cases
// ===========================================================================

describe("NetworkSource.getLastError()", () => {
  it("returns null initially", () => {
    const source = makeNetworkSource([])
    expect(source.getLastError()).toBeNull()
  })

  it("returns error message after network failure", async () => {
    const source = makeErrorSource("ECONNREFUSED")
    await source.getSessions()
    expect(source.getLastError()).toBe("ECONNREFUSED")
  })

  it("returns null after successful fetch following an error", async () => {
    let failCount = 1
    const source = new NetworkSource({
      backendUrl: "https://backend.example.com",
      apiToken: "test-token",
      _fetch: async () => {
        if (failCount-- > 0) throw new Error("Transient error")
        return [makeMember()]
      },
    })

    // First call fails
    await source.getSessions()
    expect(source.getLastError()).toBe("Transient error")

    // Second call succeeds
    await source.getSessions()
    expect(source.getLastError()).toBeNull()
  })
})

// ===========================================================================
// REQ-TUI-055: network timeout enforcement
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-055 plan=step-verify-C001 test=dashboard-network.test.ts
// ===========================================================================

describe("REQ-TUI-055: network timeout enforcement", () => {
  it("getSessions() returns [] when _fetch throws TimeoutError", async () => {
    const source = createNetworkSource({
      backendUrl: "https://example.com",
      apiToken: "token",
      _fetch: async () => {
        // Simulate a timeout (DOMException with name "TimeoutError" — same as AbortSignal.timeout fires).
        // getSessions() stores err.message (DOMException extends Error), so the stored string is the
        // message text, not the exception name.
        const err = new DOMException("The operation was aborted due to timeout", "TimeoutError")
        throw err
      },
    })
    const sessions = await source.getSessions()
    expect(sessions).toEqual([])
    // DOMException.message is stored; it contains "timeout" (case-insensitive)
    expect(source.getLastError()?.toLowerCase()).toContain("timeout")
  })

  it("getSessions() returns [] when _fetch takes too long (simulated)", async () => {
    // Verify the real fetch path would use AbortSignal.timeout
    // We test this by checking the source is available but returns [] on error
    const source = createNetworkSource({
      backendUrl: "https://example.com",
      apiToken: "token",
      _fetch: async () => {
        throw new Error("Network timeout after 500ms")
      },
    })
    const sessions = await source.getSessions()
    expect(sessions).toEqual([])
    expect(source.getLastError()).toBeTruthy()
  })
})
