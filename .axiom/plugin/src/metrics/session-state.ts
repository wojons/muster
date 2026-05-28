/**
 * Session state machine — tracks running/idle/stopped transitions.
 *
 * REQ-PLG-127: Session state machine with 3 states:
 *   - running: Agent is actively processing (tool.execute.before or chat.message)
 *   - idle: No activity for idle timeout (default 300 seconds)
 *   - stopped: Session has ended (session.end event or process exit)
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-127
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-127 plan=phase-107-6 jira_ref=DEX-392
 */

import type { SessionState } from "./sink-interface"

const DEFAULT_IDLE_TIMEOUT_MS = 300_000 // 5 minutes

export interface SessionStateTracker {
  /** Current state. */
  readonly current: SessionState
  /** Signal activity — resets idle timer, transitions idle→running. */
  signal(): void
  /** Transition to stopped (terminal state). */
  stop(): void
  /** Register callback for state transitions. */
  onTransition(cb: (from: SessionState, to: SessionState) => void): void
}

export interface SessionStateTrackerOptions {
  /** Idle timeout in milliseconds. Default: 300_000 (5 minutes). */
  idleTimeoutMs?: number
}

/**
 * Create a session state tracker.
 *
 * Starts in `running` state. After `idleTimeoutMs` of no `signal()` calls,
 * transitions to `idle`. Calling `signal()` resets the timer and transitions
 * `idle` → `running`. Calling `stop()` transitions to `stopped` (terminal).
 */
export function createSessionStateTracker(
  options: SessionStateTrackerOptions = {}
): SessionStateTracker {
  const idleTimeoutMs = options.idleTimeoutMs ?? DEFAULT_IDLE_TIMEOUT_MS

  let current: SessionState = "running"
  let idleTimer: ReturnType<typeof setTimeout> | null = null
  const transitionCallbacks: Array<(from: SessionState, to: SessionState) => void> = []

  const transition = (to: SessionState) => {
    const from = current
    current = to
    for (const cb of transitionCallbacks) {
      cb(from, to)
    }
  }

  const clearIdleTimer = () => {
    if (idleTimer !== null) {
      clearTimeout(idleTimer)
      idleTimer = null
    }
  }

  const startIdleTimer = () => {
    clearIdleTimer()
    idleTimer = setTimeout(() => {
      if (current === "running") {
        transition("idle")
      }
    }, idleTimeoutMs)
  }

  // Start the idle timer immediately
  startIdleTimer()

  return {
    get current() {
      return current
    },

    signal() {
      // No transitions from stopped (terminal state)
      if (current === "stopped") {
        return
      }

      if (current === "idle") {
        transition("running")
      }
      // If already running, just reset the timer
      startIdleTimer()
    },

    stop() {
      clearIdleTimer()
      if (current !== "stopped") {
        transition("stopped")
      }
    },

    onTransition(cb: (from: SessionState, to: SessionState) => void) {
      transitionCallbacks.push(cb)
    },
  }
}