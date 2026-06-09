/**
 * Idle detector — fires a callback after a configurable period of inactivity.
 *
 * REQ-PLG-411a: on_session_idle keep-alive — fire on_session_idle hooks when
 * no tool/message activity for configurable idle window (default: 60s).
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-411a
 * axiom:trace work_item=plugin-v2-enhancements-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-411a plan=phase-70-9/task-70-9-4/step-70-9-4-1
 */

/** Options for creating an idle detector. */
export interface IdleDetectorOptions {
	/** How long (ms) of inactivity before onIdle fires. Default: 60000. */
	idleTimeoutMs: number;
	/** Callback fired when the idle window expires. */
	onIdle: () => void;
}

/** An idle detector that fires after a configurable inactivity window. */
export interface IdleDetector {
	/** Reset the inactivity timer (call on every tool/message event). */
	reset(): void;
	/** Stop the detector and prevent any future onIdle calls. */
	stop(): void;
}

/**
 * Create an idle detector.
 *
 * REQ-PLG-411a: fires onIdle after idleTimeoutMs of no activity.
 * Call reset() on every tool/message event to restart the timer.
 * Call stop() to clean up when the session ends.
 */
export function createIdleDetector(options: IdleDetectorOptions): IdleDetector {
	const { idleTimeoutMs, onIdle } = options;
	let timer: ReturnType<typeof setTimeout> | null = null;
	let stopped = false;

	function scheduleIdle(): void {
		if (stopped) return;
		if (timer !== null) {
			clearTimeout(timer);
		}
		timer = setTimeout(() => {
			if (!stopped) {
				onIdle();
			}
		}, idleTimeoutMs);
	}

	function reset(): void {
		if (stopped) return;
		scheduleIdle();
	}

	function stop(): void {
		stopped = true;
		if (timer !== null) {
			clearTimeout(timer);
			timer = null;
		}
	}

	// Start the initial timer
	scheduleIdle();

	return { reset, stop };
}
