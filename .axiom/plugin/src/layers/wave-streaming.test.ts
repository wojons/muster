/**
 * Tests for Wave session streaming.
 *
 * axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md#wave-integration plan=phase-1.3/task-1.3.5/step-1.3.5.1
 */

import { beforeEach, describe, expect, it, vi } from "vitest"
import { WaveSessionStreamer } from "./wave-streaming"

// Clear all mocks before each test to prevent vi.fn() state leakage in bun vitest compat.
beforeEach(() => { vi.clearAllMocks() })

const CONFIG = {
  backendUrl: "http://localhost:8100",
  spaceId: "sp-1",
  panelId: "pn-1",
}

function okFetch() {
  return vi.fn().mockResolvedValue({ ok: true, status: 201 } as Response)
}

describe("WaveSessionStreamer", () => {
  it("sends a pushed item after flush", async () => {
    const mockFetch = okFetch()
    const streamer = new WaveSessionStreamer({ ...CONFIG, _mockFetch: mockFetch })
    streamer.push({ type: "message", author: "agent", content: { text: "Hello" } })
    await streamer.flush()
    expect(mockFetch).toHaveBeenCalledTimes(1)
    expect(streamer.getStats().sent).toBe(1)
  })

  it("sends multiple items after flush", async () => {
    const mockFetch = okFetch()
    const streamer = new WaveSessionStreamer({ ...CONFIG, _mockFetch: mockFetch })
    streamer.push({ type: "message", author: "human", content: { text: "A" } })
    streamer.push({ type: "tool_call", author: "agent", content: { tool: "bash" } })
    streamer.push({ type: "file_change", author: "agent", content: { path: "foo.py" } })
    await streamer.flush()
    expect(mockFetch).toHaveBeenCalledTimes(3)
    expect(streamer.getStats().sent).toBe(3)
  })

  it("does not block on network error", async () => {
    const mockFetch = vi.fn().mockRejectedValue(new Error("network error"))
    const streamer = new WaveSessionStreamer({ ...CONFIG, _mockFetch: mockFetch })
    streamer.push({ type: "message", author: "agent", content: {} })
    await streamer.flush()
    expect(streamer.getStats().errors).toBe(1)
    expect(streamer.getStats().sent).toBe(0)
  })

  it("drops oldest item when buffer is full", async () => {
    const mockFetch = okFetch()
    const streamer = new WaveSessionStreamer({ ...CONFIG, maxBufferSize: 2, _mockFetch: mockFetch })
    streamer.push({ type: "message", author: "agent", content: { seq: 1 } })
    streamer.push({ type: "message", author: "agent", content: { seq: 2 } })
    // This push should drop seq:1 because buffer is full (maxBufferSize=2)
    streamer.push({ type: "message", author: "agent", content: { seq: 3 } })
    // Flush to send all items
    await streamer.flush()
    // Verify that seq:1 was dropped (not sent) and seq:2, seq:3 were sent
    expect(streamer.getStats().dropped).toBe(1)
    expect(streamer.getStats().sent).toBe(2)
    // Check that seq:1 was NOT sent (by checking the sent content)
    const sentBodies = mockFetch.mock.calls.map(([, opts]) => JSON.parse(opts.body as string))
    const seqs = sentBodies.map(b => b.content.seq)
    expect(seqs).toContain(2)
    expect(seqs).toContain(3)
    expect(seqs).not.toContain(1)
  })

  it("posts to the correct URL with item body", async () => {
    const mockFetch = okFetch()
    const streamer = new WaveSessionStreamer({ ...CONFIG, _mockFetch: mockFetch })
    streamer.push({ type: "tool_result", author: "agent", content: { result: "ok" } })
    await streamer.flush()
    const [url, opts] = mockFetch.mock.calls[0]
    expect(url).toContain("/api/v1/spaces/sp-1/panels/pn-1/items")
    const body = JSON.parse(opts.body)
    expect(body.type).toBe("tool_result")
    expect(body.author).toBe("agent")
  })

  it("includes Authorization header when authToken provided", async () => {
    const mockFetch = okFetch()
    const streamer = new WaveSessionStreamer({ ...CONFIG, authToken: "tok-abc", _mockFetch: mockFetch })
    streamer.push({ type: "message", author: "agent", content: {} })
    await streamer.flush()
    const headers = mockFetch.mock.calls[0][1].headers
    expect(headers["Authorization"]).toBe("Bearer tok-abc")
  })

  it("getStats returns correct counts", async () => {
    const mockFetch = vi.fn()
      .mockResolvedValueOnce({ ok: true } as Response)
      .mockResolvedValueOnce({ ok: false, status: 500 } as Response)
    const streamer = new WaveSessionStreamer({ ...CONFIG, _mockFetch: mockFetch })
    streamer.push({ type: "message", author: "agent", content: {} })
    streamer.push({ type: "message", author: "agent", content: {} })
    await streamer.flush()
    const stats = streamer.getStats()
    expect(stats.sent).toBe(1)
    expect(stats.errors).toBe(1)
  })
})