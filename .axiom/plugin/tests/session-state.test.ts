/**
 * Session state machine tests.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-127
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-127 plan=phase-107-6 jira_ref=DEX-392
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest"
import { createSessionStateTracker, type SessionStateTracker } from "../src/metrics/session-state"

describe("SessionStateTracker", () => {
  let tracker: SessionStateTracker

  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it("starts in running state", () => {
    tracker = createSessionStateTracker({ idleTimeoutMs: 50 })
    expect(tracker.current).toBe("running")
  })

  it("transitions to idle after timeout", async () => {
    tracker = createSessionStateTracker({ idleTimeoutMs: 50 })

    // Fast-forward past the idle timeout
    vi.advanceTimersByTime(60)

    expect(tracker.current).toBe("idle")
  })

  it("signal() resets to running from idle", () => {
    tracker = createSessionStateTracker({ idleTimeoutMs: 50 })

    // Transition to idle
    vi.advanceTimersByTime(60)
    expect(tracker.current).toBe("idle")

    // Signal should transition back to running
    tracker.signal()
    expect(tracker.current).toBe("running")
  })

  it("signal() keeps running when already running", () => {
    tracker = createSessionStateTracker({ idleTimeoutMs: 50 })

    expect(tracker.current).toBe("running")
    tracker.signal()
    expect(tracker.current).toBe("running")
  })

  it("stop() transitions to stopped", () => {
    tracker = createSessionStateTracker({ idleTimeoutMs: 50 })

    tracker.stop()
    expect(tracker.current).toBe("stopped")
  })

  it("no transitions after stopped (terminal state)", () => {
    tracker = createSessionStateTracker({ idleTimeoutMs: 50 })

    tracker.stop()
    expect(tracker.current).toBe("stopped")

    // Signal should not change state
    tracker.signal()
    expect(tracker.current).toBe("stopped")

    // Advance timer should not change state
    vi.advanceTimersByTime(100)
    expect(tracker.current).toBe("stopped")
  })

  it("onTransition callback fires on state changes", () => {
    const callback = vi.fn()
    tracker = createSessionStateTracker({ idleTimeoutMs: 50 })

    tracker.onTransition(callback)

    // Transition to idle
    vi.advanceTimersByTime(60)
    expect(callback).toHaveBeenCalledWith("running", "idle")

    // Transition back to running
    tracker.signal()
    expect(callback).toHaveBeenCalledWith("idle", "running")

    // Transition to stopped
    tracker.stop()
    expect(callback).toHaveBeenCalledWith("running", "stopped")
  })

  it("uses default idle timeout of 5 minutes", () => {
    tracker = createSessionStateTracker()

    // Should still be running before 5 minutes
    vi.advanceTimersByTime(4 * 60 * 1000) // 4 minutes
    expect(tracker.current).toBe("running")

    // After 5 minutes + 1ms, should be idle
    vi.advanceTimersByTime(60 * 1000 + 1) // 1 more minute + 1ms
    expect(tracker.current).toBe("idle")
  })
})