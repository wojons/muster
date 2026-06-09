/**
 * Tests for Wave command handlers.
 *
 * Converted from vitest to bun:test for CI compatibility (vi.fn().mockResolvedValue
 * does not work correctly in bun on Linux).
 *
 * axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md#wave-integration plan=phase-1.3/task-1.3.1/step-1.3.1.1
 */

import { describe, expect, it, mock } from "bun:test"
import { handleReviewCommand, handleWaveCommand } from "./wave"

const BACKEND = "http://localhost:8100"

type CallLog = Array<[string, RequestInit | undefined]>

/** Create a mock fetch that returns a successful space creation response. */
function mockFetchSuccess(spaceId = "sp-abc123", title = "Wave Space") {
  const calls: CallLog = []
  const fn = async (url: string, opts?: RequestInit): Promise<Response> => {
    calls.push([url, opts])
    return {
      ok: true,
      status: 201,
      json: async () => ({ space_id: spaceId, title }),
    } as unknown as Response
  }
  fn.mock = { calls } as unknown as ReturnType<typeof mock>["mock"]
  return fn
}

/** Create a mock fetch that returns a non-ok response. */
function mockFetchError(status = 500, statusText = "Server Error") {
  return async (_url: string, _opts?: RequestInit): Promise<Response> =>
    ({ ok: false, status, statusText }) as unknown as Response
}

describe("handleWaveCommand", () => {
  it("creates a Space and returns the URL", async () => {
    const openCalls: string[] = []
    const mockOpen = (url: string) => { openCalls.push(url) }
    const mockFetch = mockFetchSuccess()
    const result = await handleWaveCommand("My Space", {
      backendUrl: BACKEND,
      authToken: "tok-123",
      _mockFetch: mockFetch,
      _mockOpen: mockOpen,
    })
    expect(result.spaceId).toBe("sp-abc123")
    expect(result.url).toContain("sp-abc123")
    expect(result.isNew).toBe(true)
    expect(openCalls[0]).toContain("sp-abc123")
  })

  it("uses work_item_id as default title when no title given", async () => {
    const mockFetch = mockFetchSuccess()
    await handleWaveCommand(undefined, {
      backendUrl: BACKEND,
      workItemId: "wave-01",
      _mockFetch: mockFetch,
      _mockOpen: () => {},
    })
    const body = JSON.parse(mockFetch.mock.calls[0][1]!.body as string)
    expect(body.title).toContain("wave-01")
  })

  it("sends Authorization header when authToken provided", async () => {
    const mockFetch = mockFetchSuccess()
    await handleWaveCommand("Test", {
      backendUrl: BACKEND,
      authToken: "my-token",
      _mockFetch: mockFetch,
      _mockOpen: () => {},
    })
    const headers = mockFetch.mock.calls[0][1]!.headers as Record<string, string>
    expect(headers["Authorization"]).toBe("Bearer my-token")
  })

  it("throws on non-ok response", async () => {
    await expect(
      handleWaveCommand("Test", {
        backendUrl: BACKEND,
        _mockFetch: mockFetchError(500, "Server Error"),
        _mockOpen: () => {},
      })
    ).rejects.toThrow("500")
  })

  it("posts to /api/v1/spaces with correct body shape", async () => {
    const mockFetch = mockFetchSuccess()
    await handleWaveCommand("My Wave", {
      backendUrl: BACKEND,
      workItemId: "wi-42",
      _mockFetch: mockFetch,
      _mockOpen: () => {},
    })
    const body = JSON.parse(mockFetch.mock.calls[0][1]!.body as string)
    expect(body.type).toBe("collaboration")
    expect(body.access_scope).toBe("link")
    expect(body.work_item_id).toBe("wi-42")
  })
})

describe("handleReviewCommand", () => {
  it("creates a session_review Space", async () => {
    const mockFetch = mockFetchSuccess("sp-review-1", "Session Review: sess-99")
    const result = await handleReviewCommand({
      backendUrl: BACKEND,
      sessionId: "sess-99",
      _mockFetch: mockFetch,
      _mockOpen: () => {},
    })
    expect(result.spaceId).toBe("sp-review-1")
  })

  it("includes session ID in the Space title", async () => {
    const mockFetch = mockFetchSuccess()
    await handleReviewCommand({
      backendUrl: BACKEND,
      sessionId: "sess-42",
      _mockFetch: mockFetch,
      _mockOpen: () => {},
    })
    const body = JSON.parse(mockFetch.mock.calls[0][1]!.body as string)
    expect(body.title).toContain("sess-42")
  })
})
