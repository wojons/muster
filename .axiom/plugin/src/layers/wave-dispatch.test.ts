/**
 * Tests for Wave command dispatch.
 *
 * axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md#wave-integration plan=phase-1.3/task-1.3.2/step-1.3.2.1
 */

import { describe, expect, it, vi } from "vitest"
import { dispatchWaveCommand, isWaveCommand } from "./wave-dispatch"
import * as wave from "./wave"

const CONFIG = { backendUrl: "http://localhost:8100", workItemId: "wave-01", sessionId: "sess-1" }

describe("isWaveCommand", () => {
  it("returns true for /axiom-wave", () => {
    expect(isWaveCommand("/axiom-wave")).toBe(true)
  })
  it("returns true for /axiom-review", () => {
    expect(isWaveCommand("/axiom-review")).toBe(true)
  })
  it("returns false for other commands", () => {
    expect(isWaveCommand("/axiom-share")).toBe(false)
    expect(isWaveCommand("/axiom-run")).toBe(false)
  })
})

describe("dispatchWaveCommand", () => {
  it("/axiom-wave returns success message with URL", async () => {
    vi.spyOn(wave, "handleWaveCommand").mockResolvedValue({
      spaceId: "sp-1", url: "http://localhost:8100/ui/spaces/sp-1", title: "My Space", isNew: true,
    })
    const msg = await dispatchWaveCommand("/axiom-wave", "My Space", CONFIG)
    expect(msg).toContain("Wave Space created")
    expect(msg).toContain("sp-1")
  })

  it("/axiom-review returns success message with URL", async () => {
    vi.spyOn(wave, "handleReviewCommand").mockResolvedValue({
      spaceId: "sp-2", url: "http://localhost:8100/ui/spaces/sp-2", title: "Session Review", isNew: true,
    })
    const msg = await dispatchWaveCommand("/axiom-review", "", CONFIG)
    expect(msg).toContain("Session review Space created")
    expect(msg).toContain("sp-2")
  })

  it("unknown command returns error message", async () => {
    const msg = await dispatchWaveCommand("/axiom-unknown", "", CONFIG)
    expect(msg).toContain("Unknown Wave command")
  })

  it("/axiom-wave passes args as title", async () => {
    const spy = vi.spyOn(wave, "handleWaveCommand").mockResolvedValue({
      spaceId: "sp-3", url: "http://x/sp-3", title: "Custom Title", isNew: true,
    })
    await dispatchWaveCommand("/axiom-wave", "Custom Title", CONFIG)
    expect(spy).toHaveBeenCalledWith("Custom Title", expect.any(Object))
  })

  it("/axiom-wave with empty args passes undefined title", async () => {
    const spy = vi.spyOn(wave, "handleWaveCommand").mockResolvedValue({
      spaceId: "sp-4", url: "http://x/sp-4", title: "Wave Space", isNew: true,
    })
    await dispatchWaveCommand("/axiom-wave", "  ", CONFIG)
    expect(spy).toHaveBeenCalledWith(undefined, expect.any(Object))
  })
})