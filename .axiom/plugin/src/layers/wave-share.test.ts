/**
 * Tests for Wave-aware share handler.
 *
 * axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md#wave-integration plan=phase-1.3/task-1.3.3/step-1.3.3.1
 */

import { beforeEach, describe, expect, it, vi } from "vitest"
import { createWaveShare, shareWithWaveFallback } from "./wave-share"

// Clear all mocks before each test to prevent vi.fn() state leakage in bun vitest compat.
beforeEach(() => { vi.clearAllMocks() })

const BACKEND = "http://localhost:8100"

function mockFetchSequence(...responses: object[]) {
  let i = 0
  return vi.fn().mockImplementation(async () => {
    const r = responses[i++] ?? responses[responses.length - 1]
    return { ok: true, status: 201, json: async () => r } as unknown as Response
  })
}

describe("createWaveShare", () => {
  it("creates a Space then a share link when no spaceId given", async () => {
    const mockFetch = mockFetchSequence(
      { space_id: "sp-new" },
      { share_id: "sh-1", url: "/s/sh-1", expires_at: "2026-05-05", access_scope: "link", space_id: "sp-new" },
    )
    const result = await createWaveShare({ backendUrl: BACKEND, _mockFetch: mockFetch })
    expect(result.shareId).toBe("sh-1")
    expect(result.url).toBe("/s/sh-1")
    expect(result.isWaveShare).toBe(true)
    expect(mockFetch).toHaveBeenCalledTimes(2)
  })

  it("skips Space creation when spaceId is provided", async () => {
    const mockFetch = mockFetchSequence(
      { share_id: "sh-2", url: "/s/sh-2", expires_at: "2026-05-05", access_scope: "link", space_id: "sp-existing" },
    )
    const result = await createWaveShare({ backendUrl: BACKEND, spaceId: "sp-existing", _mockFetch: mockFetch })
    expect(result.shareId).toBe("sh-2")
    expect(mockFetch).toHaveBeenCalledTimes(1)
  })

  it("uses default 30-day expiry", async () => {
    const mockFetch = mockFetchSequence(
      { space_id: "sp-x" },
      { share_id: "sh-3", url: "/s/sh-3", expires_at: "2026-05-05", access_scope: "link", space_id: "sp-x" },
    )
    await createWaveShare({ backendUrl: BACKEND, _mockFetch: mockFetch })
    const shareBody = JSON.parse(mockFetch.mock.calls[1][1].body)
    expect(shareBody.expires_in_days).toBe(30)
  })

  it("throws on non-ok Space creation response", async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: false, status: 500 } as Response)
    await expect(createWaveShare({ backendUrl: BACKEND, _mockFetch: mockFetch })).rejects.toThrow("500")
  })

  it("throws on non-ok share creation response", async () => {
    const mockFetch = vi.fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ space_id: "sp-y" }) } as unknown as Response)
      .mockResolvedValueOnce({ ok: false, status: 403 } as Response)
    await expect(createWaveShare({ backendUrl: BACKEND, _mockFetch: mockFetch })).rejects.toThrow("403")
  })
})

describe("shareWithWaveFallback", () => {
  it("returns Wave share on success", async () => {
    const mockFetch = mockFetchSequence(
      { space_id: "sp-z" },
      { share_id: "sh-4", url: "/s/sh-4", expires_at: "2026-05-05", access_scope: "link", space_id: "sp-z" },
    )
    const result = await shareWithWaveFallback(
      { backendUrl: BACKEND, _mockFetch: mockFetch },
      async () => "local://fallback",
    )
    expect(result.isWaveShare).toBe(true)
    if (result.isWaveShare) expect(result.shareId).toBe("sh-4")
  })

  it("falls back to local share on Wave failure", async () => {
    const mockFetch = vi.fn().mockRejectedValue(new Error("network error"))
    const result = await shareWithWaveFallback(
      { backendUrl: BACKEND, _mockFetch: mockFetch },
      async () => "local://fallback-url",
    )
    expect(result.isWaveShare).toBe(false)
    if (!result.isWaveShare) expect(result.url).toBe("local://fallback-url")
  })
})