/**
 * Listener lifecycle manager — manages the full lifecycle of all listeners.
 *
 * REQ-LNP-070: Listener startup time < 2s (p99), async startup.
 * REQ-LNP-073: Max 10 listeners per session, 50 per machine.
 * REQ-LNP-074: Memory consumption per listener < 20MB.
 * REQ-LNP-076: Health checks every 30 seconds.
 * REQ-LNP-077: Orphan detection within 60 seconds of session end.
 * REQ-LNP-078: SLO metrics exposure.
 *
 * Also handles:
 * - Edge case 1: Listener source unreachable at startup — degraded state + exponential backoff.
 * - Edge case 9: Plugin crash recovery — orphan detection via AXIOM_LISTENER env marker.
 * - Edge case 10: Config file modified while listeners running — stop/start/reconfigure.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#8
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-2/task-2.4
 */

import { join } from "node:path";
import type { EventBridgeListenerConfig, HttpListenerConfig, KafkaListenerConfig, ListenerEntry, NatsListenerConfig, RunActionConfig, ScriptListenerConfig, SqsListenerConfig } from "../../config-loader";
import { createEventBridgeAdapter } from "./adapters/eventbridge";
import { createHttpAdapter } from "./adapters/http";
import { createKafkaAdapter } from "./adapters/kafka";
import { createNatsAdapter } from "./adapters/nats";
import { createScriptAdapter } from "./adapters/script";
import { buildSafeEnv } from "./adapters/script";
import { createSqsAdapter } from "./adapters/sqs";
import type { DeliveryQueue } from "./delivery";
import type { InjectionEngine } from "./injection";
import { getNestedField } from "./router";
import type { SessionRouter } from "./router";
import type { EventCallback, EventEnvelope, ListenerAdapter, ListenerState } from "./types";
import {
	emitListenerLog,
	HEALTH_CHECK_INTERVAL_MS,
	MAX_LISTENERS_PER_MACHINE,
	MAX_LISTENERS_PER_SESSION,
	ORPHAN_DETECTION_INTERVAL_MS,
} from "./types";

// ── Types ────────────────────────────────────────────────────────────────

/** Options for creating a listener manager. */
export interface ListenerManagerOptions {
	repoRoot: string;
	currentSessionId?: string;
	router: SessionRouter;
	injectionEngine: InjectionEngine;
	deliveryQueue: DeliveryQueue;
	/** Override health check interval in ms (for testing). */
	healthCheckIntervalMs?: number;
	/** Override orphan detection interval in ms (for testing). */
	orphanDetectionIntervalMs?: number;
	/**
	 * Optional runner trigger override for testing (REQ-LNP-142 DI).
	 * When provided, replaces the real CLI/HTTP trigger with this function.
	 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-142 plan=phase-5/task-5.2 jira_ref=DEX-434
	 */
	runnerTriggerOverride?: (workItem: string, repo: string, prompt?: string, surface?: string, eventId?: string) => Promise<void>;
	/**
	 * Override rate limit window in ms for testing (REQ-LNP-144).
	 * Defaults to 5000ms (5s). Set to 0 to disable rate limiting in tests.
	 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-144 plan=phase-5/task-5.3 jira_ref=DEX-434
	 */
	runnerRateLimitMsOverride?: number;
	/**
	 * Override max concurrent runners for testing (REQ-LNP-144).
	 * Defaults to 5. Prevents fan-out from high-cardinality $event.* substitution.
	 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-144 plan=phase-5/task-5.3 jira_ref=DEX-434
	 */
	maxConcurrentRunnersOverride?: number;
}

/** The listener manager interface. */
export interface ListenerManager {
	/**
	 * Start all listeners from config.
	 * REQ-LNP-070: Async startup — does not block plugin init.
	 */
	startFromConfig(listeners: ListenerEntry[]): Promise<void>;

	/**
	 * Start a single listener by name.
	 * REQ-LNP-073: Enforces per-session and per-machine limits.
	 */
	startListener(
		entry: ListenerEntry,
		creatorSessionId?: string,
	): Promise<{ success: boolean; error?: string }>;

	/** Stop a listener by name. */
	stopListener(name: string): Promise<void>;

	/** Stop all listeners. */
	stopAll(): Promise<void>;

	/** Get the state of all listeners. */
	getAllStates(): ListenerState[];

	/** Get the state of a specific listener. */
	getState(name: string): ListenerState | undefined;

	/**
	 * Update delivery mode for a listener at runtime.
	 * REQ-LNP-057: Dynamic delivery mode switching.
	 */
	updateDeliveryMode(
		name: string,
		mode: "interruptive" | "non-interruptive",
	): boolean;

	/**
	 * Notify the manager that a session has ended.
	 * REQ-LNP-077: Orphan detection — stop listeners for ended sessions.
	 */
	onSessionEnd(sessionId: string): Promise<void>;

	/** Get SLO metrics (REQ-LNP-078). */
	getMetrics(): ListenerMetrics;

	/** Dispose of all resources (timers, etc.). */
	dispose(): void;
}

/** SLO metrics per REQ-LNP-078. */
export interface ListenerMetrics {
	/** Total events received across all listeners. */
	total_events_received: number;
	/** Total events delivered across all listeners. */
	total_events_delivered: number;
	/** Total events dropped across all listeners. */
	total_events_dropped: number;
	/** Per-listener uptime in seconds. */
	listener_uptime: Record<string, number>;
	/** Active listener count. */
	active_listener_count: number;
	/**
	 * Per-listener drop counts (for Prometheus counter labels).
	 * REQ-LNP-078: codeops_listener_events_dropped_total{listener="<name>"}
	 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-078 plan=step-spec-003
	 */
	drops_by_listener: Record<string, number>;
	/**
	 * Per-listener delivery latency samples in milliseconds (REQ-LNP-078).
	 * Used to compute p50/p95/p99 histogram buckets in the Prometheus sink.
	 * Capped at 1000 samples per listener to bound memory usage.
	 * axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-078 plan=phase-3/task-3.1 jira_ref=DEX-434
	 */
	delivery_latencies: Record<string, number[]>;
}

// ── Session Registry Helpers ─────────────────────────────────────────────

/**
 * Read the active sessions registry file.
 * Returns a Set of active session IDs.
 *
 * REQ-LNP-077: Used by orphan detection to determine whether a creator session
 * is still alive. If the file is missing or malformed, returns an empty set so
 * that all foreign-session listeners are treated as orphaned (fail-closed).
 *
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-077 plan=step-fix-010
 */
async function readActiveSessionsFile(repoRoot: string): Promise<Set<string>> {
	const filePath = join(repoRoot, ".axiom", ".active-sessions.json");
	try {
		const { readFile } = await import("node:fs/promises");
		const content = await readFile(filePath, "utf-8");
		const data = JSON.parse(content) as { sessions: Array<{ id: string }> };
		return new Set(data.sessions.map((s) => s.id));
	} catch {
		// File doesn't exist or is malformed — return empty set (all sessions considered dead)
		return new Set<string>();
	}
}

/** Schema for a session entry in .active-sessions.json (REQ-LNP-132). */
interface ActiveSessionEntry {
	id: string;
	started_at: string;
	last_heartbeat: string;
	pid: number;
	work_item?: string;
	tags?: string[];
}

/**
 * Read the active sessions registry and return a Map<sessionId, SessionInfo>
 * suitable for passing to the session router's updateSessions() method.
 *
 * REQ-LNP-131: Session discovery from .active-sessions.json.
 * REQ-LNP-133: Sessions with last_heartbeat older than 60s are treated as dead.
 * REQ-LNP-135: PID liveness check — sessions whose PID is no longer running are dead.
 *
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-131 plan=milestone-gate-followup-013
 */
export async function readActiveSessionsForRouter(
	repoRoot: string,
): Promise<Map<string, import("./router").SessionInfo>> {
	const filePath = join(repoRoot, ".axiom", ".active-sessions.json");
	const result = new Map<string, import("./router").SessionInfo>();
	try {
		const { readFile } = await import("node:fs/promises");
		const content = await readFile(filePath, "utf-8");
		const data = JSON.parse(content) as { sessions: ActiveSessionEntry[] };
		const now = Date.now();
		const TTL_MS = 60_000; // REQ-LNP-133: 60s TTL

		for (const entry of data.sessions ?? []) {
			// REQ-LNP-133: Skip sessions with stale heartbeat
			const heartbeatAge = now - new Date(entry.last_heartbeat).getTime();
			if (heartbeatAge > TTL_MS) continue;

			// REQ-LNP-135: PID liveness check
			if (entry.pid && entry.pid !== process.pid) {
				try {
					process.kill(entry.pid, 0); // throws if PID is dead
				} catch {
					continue; // PID is dead — skip this session
				}
			}

			result.set(entry.id, {
				session_id: entry.id,
				work_item: entry.work_item,
				tags: entry.tags,
			});
		}
	} catch {
		// File doesn't exist or is malformed — return empty map
	}
	return result;
}

/**
 * Write the current session to the active sessions registry file.
 * Idempotent — safe to call multiple times for the same session.
 * Updates last_heartbeat on each call (REQ-LNP-133).
 *
 * REQ-LNP-077: Called at plugin startup so that orphan detection in other
 * plugin instances can see this session as alive.
 * REQ-LNP-132: Writes full session entry schema (id, started_at, last_heartbeat, pid, work_item, tags).
 *
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-077 plan=step-fix-010
 */
export async function writeActiveSessionsFile(
	repoRoot: string,
	sessionId: string,
	opts?: { work_item?: string; tags?: string[] },
): Promise<void> {
	const filePath = join(repoRoot, ".axiom", ".active-sessions.json");
	try {
		const { mkdir, readFile, writeFile } = await import("node:fs/promises");
		await mkdir(join(repoRoot, ".axiom"), { recursive: true });

		// Read existing sessions
		let sessions: ActiveSessionEntry[] = [];
		try {
			const content = await readFile(filePath, "utf-8");
			const data = JSON.parse(content) as { sessions: ActiveSessionEntry[] };
			sessions = data.sessions ?? [];
		} catch {
			// File doesn't exist yet — start fresh
		}

		const now = new Date().toISOString();
		const existing = sessions.find((s) => s.id === sessionId);
		if (existing) {
			// Update heartbeat and optional routing metadata
			existing.last_heartbeat = now;
			if (opts?.work_item !== undefined) existing.work_item = opts.work_item;
			if (opts?.tags !== undefined) existing.tags = opts.tags;
		} else {
			sessions.push({
				id: sessionId,
				started_at: now,
				last_heartbeat: now,
				pid: process.pid,
				work_item: opts?.work_item,
				tags: opts?.tags,
			});
		}

		// Atomic write: write to temp file then rename to avoid partial-read race condition.
		// If the discovery loop reads during a write, it gets either the old or new file,
		// never a truncated/partial file. Self-heals in 30s if rename fails.
		const tmpPath = `${filePath}.tmp`;
		await writeFile(tmpPath, JSON.stringify({ sessions }, null, 2), "utf-8");
		const { rename } = await import("node:fs/promises");
		await rename(tmpPath, filePath);
	} catch {
		// Non-fatal — session registry write failure should not crash the plugin
	}
}

/**
 * Remove a session from the active sessions registry file on shutdown.
 * Idempotent — safe to call if the session was never written or file is missing.
 *
 * REQ-LNP-077: Called at plugin exit so that orphan detection in other
 * plugin instances stops treating this session as alive.
 *
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-077 plan=step-audit-011
 */
export async function removeActiveSession(
	repoRoot: string,
	sessionId: string,
): Promise<void> {
	const filePath = join(repoRoot, ".axiom", ".active-sessions.json");
	try {
		const { readFile, writeFile, rename } = await import("node:fs/promises");
		const content = await readFile(filePath, "utf-8");
		const data = JSON.parse(content) as { sessions: Array<{ id: string; started_at: string }> };
		const filtered = data.sessions.filter((s) => s.id !== sessionId);
		// Atomic write: write to temp file then rename to avoid torn-write window during concurrent reads.
		// Matches the pattern in writeActiveSessionsFile() — readers see either the old or new complete file.
		// REQ-LNP-077: session registry integrity during shutdown.
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-077 plan=final-gate/atomic-remove jira_ref=DEX-434
		const tmpPath = `${filePath}.tmp`;
		await writeFile(tmpPath, JSON.stringify({ sessions: filtered }, null, 2), "utf-8");
		await rename(tmpPath, filePath);
	} catch {
		// Non-fatal — session registry removal failure should not crash the plugin
	}
}

// ── Implementation ───────────────────────────────────────────────────────

/**
 * Create a listener lifecycle manager.
 */
export function createListenerManager(
	options: ListenerManagerOptions,
): ListenerManager {
	const {
		repoRoot,
		currentSessionId,
		router,
		injectionEngine,
		deliveryQueue,
		healthCheckIntervalMs = HEALTH_CHECK_INTERVAL_MS,
		orphanDetectionIntervalMs = ORPHAN_DETECTION_INTERVAL_MS,
		runnerTriggerOverride,
	} = options;

	// REQ-LNP-144: Rate limit runner triggers — max 1 per 5s per resolved work_item
	// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-144 plan=phase-5/task-5.3 jira_ref=DEX-434
	const RUNNER_RATE_LIMIT_MS = options.runnerRateLimitMsOverride ?? 5_000;
	const runnerLastTriggered = new Map<string, number>();

	// REQ-LNP-144: Global concurrent-runner cap — prevents fan-out from high-cardinality $event.* substitution
	// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-144 plan=phase-5/task-5.3 jira_ref=DEX-434
	const MAX_CONCURRENT_RUNNERS = options.maxConcurrentRunnersOverride ?? 5;
	let activeRunnerCount = 0;

	function isRunnerRateLimited(workItem: string): boolean {
		const last = runnerLastTriggered.get(workItem) ?? 0;
		return Date.now() - last < RUNNER_RATE_LIMIT_MS;
	}

	function recordRunnerTrigger(workItem: string): void {
		runnerLastTriggered.set(workItem, Date.now());
	}

	const adapters = new Map<string, ListenerAdapter>();
	const states = new Map<string, ListenerState>();
	const startTimes = new Map<string, number>();
	let totalEventsReceived = 0;
	let totalEventsDelivered = 0;
	let totalEventsDropped = 0;

	// REQ-LNP-078: Per-listener delivery latency samples for Prometheus histogram
	// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-078 plan=phase-3/task-3.1 jira_ref=DEX-434
	const deliveryLatencies = new Map<string, number[]>();
	const MAX_LATENCY_SAMPLES = 1000; // cap per listener to bound memory

	let healthCheckTimer: ReturnType<typeof setInterval> | null = null;
	let orphanTimer: ReturnType<typeof setInterval> | null = null;

	// Start health check loop
	healthCheckTimer = setInterval(() => {
		for (const [name, adapter] of adapters) {
			const state = states.get(name);
			if (!state) continue;

			const wasHealthy = state.status === "running";
			const isHealthy = adapter.isHealthy();

			if (wasHealthy && !isHealthy) {
				state.status = "degraded";
				state.last_error = adapter.statusDescription();
				emitListenerLog({
					event: "listener.error",
					listener_name: name,
					timestamp: new Date().toISOString(),
					error: `Listener became unhealthy: ${adapter.statusDescription()}`,
				});
			} else if (!wasHealthy && isHealthy) {
				state.status = "running";
			}

			state.last_health_check = new Date().toISOString();
		}
	}, healthCheckIntervalMs);
	if (healthCheckTimer && typeof healthCheckTimer.unref === "function") healthCheckTimer.unref();

	// Start orphan detection loop
	// REQ-LNP-077: Orphan listeners (running after their creating session has ended) MUST be
	// terminated within 60 seconds. Uses file-based session registry at .axiom/.active-sessions.json.
	// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-077 plan=step-fix-010
	orphanTimer = setInterval(async () => {
		try {
			const activeSessions = await readActiveSessionsFile(repoRoot);

			for (const [name, state] of states) {
				if (
					state.creator_session_id &&
					state.creator_session_id !== currentSessionId &&
					state.status === "running" &&
					state.config.lifecycle?.auto_stop_on_session_end !== false &&
					!activeSessions.has(state.creator_session_id)
				) {
					// Creator session is no longer in the active sessions registry — treat as orphan
					const adapter = adapters.get(name);
					if (adapter) {
						emitListenerLog({
							event: "listener.stopped",
							listener_name: name,
							timestamp: new Date().toISOString(),
							reason: "orphan",
						});
						await adapter.stop();
						adapters.delete(name);
						state.status = "stopped";
						state.stopped_at = new Date().toISOString();
					}
				}
			}
		} catch {
			// Non-fatal — orphan detection failure should not crash the plugin
		}
	}, orphanDetectionIntervalMs);
	// NOTE: orphanTimer is intentionally NOT .unref()'d because tests rely on
	// it firing within tight timing windows (200ms wait for 50ms interval).
	// The timer is properly cleaned up by dispose() which calls clearInterval.

	function createAdapter(entry: ListenerEntry): ListenerAdapter {
		switch (entry.source) {
			case "sqs":
				return createSqsAdapter({
					listenerName: entry.name,
					config: entry.config as SqsListenerConfig,
				});
			case "http":
				return createHttpAdapter({
					listenerName: entry.name,
					config: entry.config as HttpListenerConfig,
				});
			case "script":
				return createScriptAdapter({
					listenerName: entry.name,
					config: entry.config as ScriptListenerConfig,
					repoRoot,
				});
			case "nats":
				// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-100 plan=phase-1/task-1.3
				return createNatsAdapter({
					listenerName: entry.name,
					config: entry.config as NatsListenerConfig,
				});
			case "kafka":
				// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-110 plan=phase-2/task-2.2
				return createKafkaAdapter({
					listenerName: entry.name,
					config: entry.config as KafkaListenerConfig,
				});
			case "eventbridge":
				// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-120 plan=phase-3/task-3.2
				return createEventBridgeAdapter({
					listenerName: entry.name,
					config: entry.config as EventBridgeListenerConfig,
				});
			default:
				throw new Error(`Unknown listener source type: ${entry.source}`);
		}
	}

	/**
	 * Resolve $event.field.path variables in a string template.
	 * REQ-LNP-143: Event context is passed to runner via variable substitution.
	 * Supports both full-value substitution ("$event.foo") and inline substitution
	 * ("prefix $event.foo suffix") for prompt templates.
	 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-143 plan=phase-5/task-5.2 jira_ref=DEX-434
	 */
	function resolveEventVar(template: string, event: EventEnvelope): string {
		if (!template.includes("$event.")) return template;
		// Replace all $event.field.path occurrences in the template string.
		// This handles both full-value ("$event.foo") and inline ("prefix $event.foo suffix").
		// Falls back to the original $event.path token if the field is not found in the payload.
		return template.replace(/\$event\.([a-zA-Z0-9_.]+)/g, (_match, path: string) => {
			const fieldValue = getNestedField(event.payload, path);
			return fieldValue !== undefined ? String(fieldValue) : `$event.${path}`;
		});
	}

	/**
	 * Trigger a Axiom execution surface with event as context.
	 * REQ-LNP-142: CLI surface spawns `axiom run`; HTTP surface calls POST /api/v1/runs.
	 * REQ-LNP-143: Execution is traceable via event_id in the runner context.
	 * Fire-and-forget — does not wait for runner completion.
	 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-142 plan=phase-5/task-5.2 jira_ref=DEX-434
	 */
	async function triggerRunner(
		event: EventEnvelope,
		runConfig: RunActionConfig,
	): Promise<void> {
		const resolvedWorkItem = resolveEventVar(runConfig.work_item, event);
		const resolvedPrompt = runConfig.prompt
			? resolveEventVar(runConfig.prompt, event)
			: undefined;

		if (!resolvedWorkItem) {
			emitListenerLog({
				event: "listener.error",
				listener_name: "lifecycle",
				timestamp: new Date().toISOString(),
				error: "Runner trigger skipped: work_item resolved to empty string",
			});
			return;
		}

		// REQ-LNP-144: Global concurrent-runner cap — prevents fan-out from high-cardinality $event.* substitution
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-144 plan=phase-5/task-5.3 jira_ref=DEX-434
		if (activeRunnerCount >= MAX_CONCURRENT_RUNNERS) {
			emitListenerLog({
				event: "listener.event_dropped",
				listener_name: "lifecycle",
				timestamp: new Date().toISOString(),
				event_id: event.id,
				reason: "runner_cap_exceeded",
			});
			return;
		}

		// REQ-LNP-144: Rate limit — max 1 runner trigger per window per work_item
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-144 plan=phase-5/task-5.3 jira_ref=DEX-434
		if (isRunnerRateLimited(resolvedWorkItem)) {
			emitListenerLog({
				event: "listener.event_dropped",
				listener_name: "lifecycle",
				timestamp: new Date().toISOString(),
				event_id: event.id,
				reason: "rate_limited",
			});
			return;
		}
		recordRunnerTrigger(resolvedWorkItem);
		activeRunnerCount++;

		const surface = runConfig.surface ?? "cli";
		const repo = runConfig.repo ?? ".";

		// REQ-LNP-145: Emit listener.runner_triggered with required fields
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-145 plan=phase-5/task-5.2 jira_ref=DEX-434
		const runnerSessionId = crypto.randomUUID();
		emitListenerLog({
			event: "listener.runner_triggered",
			listener_name: "lifecycle",
			timestamp: new Date().toISOString(),
			event_id: event.id,
			work_item: resolvedWorkItem,           // ← was session_id: resolvedWorkItem (WRONG)
			runner_session_id: runnerSessionId,    // ← NEW: UUID for this runner execution
			reason: `action=run surface=${surface}`,
		});

		// Use override if provided (for testing)
		if (runnerTriggerOverride) {
			try {
				await runnerTriggerOverride(resolvedWorkItem, repo, resolvedPrompt, surface, event.id);
			} finally {
				activeRunnerCount--;
			}
			return;
		}

		if (surface === "cli") {
			// CLI surface: spawn axiom run (fire-and-forget, non-blocking)
			// REQ-LNP-142: Use buildSafeEnv() to strip credentials from subprocess env
			try {
				const { spawn } = await import("node:child_process");
				const args = ["run", "--work-item", resolvedWorkItem, "--repo", repo];
				if (resolvedPrompt) args.push("--prompt", resolvedPrompt);
				// REQ-LNP-143: Pass event_id for traceability
				// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-143 plan=phase-5/task-5.2 jira_ref=DEX-434
				args.push("--event-id", event.id);
				const child = spawn("axiom", args, {
					env: buildSafeEnv(),
					detached: true,
					stdio: "ignore",
				});
				// REQ-LNP-144: Decrement counter immediately after spawn (fire-and-forget semantics).
				// child.unref() removes the child from the event loop — the exit event may never
				// fire if the parent exits first. Decrement before unref() for reliable counter behavior.
				// This means the cap tracks rate-of-fire, not true concurrency. See ADR: CLI Runner
				// activeRunnerCount Decrement Semantics.
				// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-144 plan=phase-5/task-5.3 jira_ref=DEX-434
				activeRunnerCount--;
				child.unref();
			} catch (err) {
				activeRunnerCount--;
				const msg = err instanceof Error ? err.message : String(err);
				emitListenerLog({
					event: "listener.error",
					listener_name: "lifecycle",
					timestamp: new Date().toISOString(),
					error: `Runner CLI trigger failed: ${msg}`,
				});
			}
		} else {
			// HTTP surface: POST /api/v1/runs (fire-and-forget)
			const apiUrl = runConfig.api_url ?? process.env.AXIOM_API_URL;

			// REQ-LNP-142: Fail-closed when api_url is not configured for HTTP surface
			// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-142 plan=phase-5/task-5.2 jira_ref=DEX-434
			if (!apiUrl) {
				activeRunnerCount--;
				emitListenerLog({
					event: "listener.error",
					listener_name: "lifecycle",
					timestamp: new Date().toISOString(),
					event_id: event.id,
					error: "Runner HTTP trigger failed: surface=http requires run.api_url in listener config or AXIOM_API_URL env var (see specs/88-Listen-Notify-Plugin.md#REQ-LNP-142). Note: the implicit http://127.0.0.1:8100 default was removed in listen-notify-02 Phase 3.",
				});
				return;
			}

			const body: Record<string, unknown> = { work_item: resolvedWorkItem, repo };
			if (resolvedPrompt) body.prompt = resolvedPrompt;
			// REQ-LNP-143: Include event_id for traceability
			// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-143 plan=phase-5/task-5.2 jira_ref=DEX-434
			body.event_id = event.id;
			void fetch(`${apiUrl}/api/v1/runs`, {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify(body),
			}).then(() => { activeRunnerCount--; }).catch((err) => {
				activeRunnerCount--;
				emitListenerLog({
					event: "listener.error",
					listener_name: "lifecycle",
					timestamp: new Date().toISOString(),
					error: `Runner HTTP trigger failed: ${err instanceof Error ? err.message : String(err)}`,
				});
			});
		}
	}

	function buildEventCallback(entry: ListenerEntry): EventCallback {
		return async (event) => {
			totalEventsReceived++;

			// Update state
			const state = states.get(entry.name);
			if (state) {
				state.events_received++;
			}

			// Sanitize via Rubix if configured
			const sanitizedEvent = entry.rubix?.enabled
				? await injectionEngine.sanitizePayload(event)
				: event;

			// Route the event
			const routingResult = router.route(sanitizedEvent, entry);

			if (routingResult.dropped) {
				totalEventsDropped++;
				if (state) state.events_dropped++;
				// REQ-LNP-140: Routing acts as a universal pre-filter — dropped events never reach
				// the runner trigger. Emit a diagnostic log when action=run/inject_and_run so
				// operators can diagnose silent runner suppression.
				// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-140 plan=final-gate/step-final-001 jira_ref=DEX-434
				const entryAction = entry.action ?? "inject";
				if (entryAction === "run" || entryAction === "inject_and_run") {
					emitListenerLog({
						event: "listener.event_dropped",
						listener_name: entry.name,
						timestamp: new Date().toISOString(),
						event_id: event.id,
						reason: "routing_drop",
					});
				}
				return;
			}

			// Determine action (REQ-LNP-140)
			const action = entry.action ?? "inject";
			const shouldInject = action === "inject" || action === "inject_and_run";
			const shouldRun = (action === "run" || action === "inject_and_run") && !!entry.run;

			// REQ-LNP-142: Trigger runner if action requires it (fire-and-forget)
			// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-142 plan=phase-5/task-5.2 jira_ref=DEX-434
			if (shouldRun && entry.run) {
				void triggerRunner(sanitizedEvent, entry.run).catch(() => {});
			}

			// REQ-LNP-141: Skip injection for action="run"
			if (!shouldInject) {
				// Still count as delivered (runner was triggered)
				totalEventsDelivered++;
				if (state) state.events_delivered++;
				return;
			}

			// Determine delivery mode
			const deliveryMode = entry.delivery ?? "non-interruptive";

			// REQ-LNP-078: Record delivery latency for Prometheus histogram.
			// SCOPE: This timer measures injection-path latency only (action=inject or inject_and_run).
			// For action=run events, the code returns early at the !shouldInject check (above) before
			// reaching this timer — so run-only events are correctly excluded from the histogram.
			// Runner execution time is fire-and-forget and not captured here.
			// See specs/88-Listen-Notify-Plugin.md#REQ-LNP-078 latency scope clarification.
			// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-078 plan=phase-3/task-3.1 jira_ref=DEX-434
			const deliveryStartMs = Date.now();

			// Deliver to all matching sessions
			for (const sessionId of routingResult.target_sessions) {
				const attempt = deliveryQueue.attempt(
					sanitizedEvent,
					sessionId,
					deliveryMode,
				);

				if (attempt.dropped) {
					totalEventsDropped++;
					if (state) state.events_dropped++;
					continue;
				}

				if (attempt.deliver_now) {
					// Queue for injection via transform hook
					injectionEngine.queueForInjection(
						sanitizedEvent,
						sessionId,
						deliveryMode,
					);
					totalEventsDelivered++;
					if (state) state.events_delivered++;
				}
				// If queued, it will be delivered at next natural pause
			}

			// Record latency sample after all session deliveries complete
			const latencyMs = Date.now() - deliveryStartMs;
			const samples = deliveryLatencies.get(entry.name) ?? [];
			samples.push(latencyMs);
			if (samples.length > MAX_LATENCY_SAMPLES) samples.shift(); // drop oldest
			deliveryLatencies.set(entry.name, samples);
		};
	}

	return {
		async startFromConfig(listeners: ListenerEntry[]): Promise<void> {
			// REQ-LNP-070: Async startup — start all listeners in parallel
			const startPromises = listeners
				.filter((l) => l.enabled !== false && l.lifecycle?.auto_start !== false)
				.map((entry) =>
					this.startListener(entry, currentSessionId).catch((err) => {
						const msg = err instanceof Error ? err.message : String(err);
						emitListenerLog({
							event: "listener.error",
							listener_name: entry.name,
							timestamp: new Date().toISOString(),
							error: `Failed to start listener: ${msg}`,
						});
					}),
				);

			// Don't await — startup is non-blocking per REQ-LNP-070
			void Promise.all(startPromises);
		},

		async startListener(
			entry: ListenerEntry,
			creatorSessionId?: string,
		): Promise<{ success: boolean; error?: string }> {
			// REQ-LNP-073: Enforce per-machine limit
			if (adapters.size >= MAX_LISTENERS_PER_MACHINE) {
				return {
					success: false,
					error: `Machine listener limit reached (${MAX_LISTENERS_PER_MACHINE})`,
				};
			}

			// REQ-LNP-073: Enforce per-session limit
			if (creatorSessionId) {
				const sessionListeners = [...states.values()].filter(
					(s) => s.creator_session_id === creatorSessionId && s.status !== "stopped",
				);
				if (sessionListeners.length >= MAX_LISTENERS_PER_SESSION) {
					return {
						success: false,
						error: `Session listener limit reached (${MAX_LISTENERS_PER_SESSION})`,
					};
				}
			}

			// Stop existing listener with same name if any
			if (adapters.has(entry.name)) {
				await this.stopListener(entry.name);
			}

			const adapter = createAdapter(entry);
			adapters.set(entry.name, adapter);

			const state: ListenerState = {
				name: entry.name,
				status: "starting",
				started_at: new Date().toISOString(),
				stopped_at: null,
				creator_session_id: creatorSessionId,
				events_received: 0,
				events_delivered: 0,
				events_dropped: 0,
				config: entry,
			};
			states.set(entry.name, state);
			startTimes.set(entry.name, Date.now());

			try {
				await adapter.start(buildEventCallback(entry));
				state.status = "running";
				return { success: true };
			} catch (err) {
				const msg = err instanceof Error ? err.message : String(err);
				state.status = "degraded";
				state.last_error = msg;
				// REQ-LNP-070: Edge case 1 — degraded state, retries handled externally
				return { success: false, error: msg };
			}
		},

		async stopListener(name: string): Promise<void> {
			const adapter = adapters.get(name);
			if (!adapter) return;

			await adapter.stop();
			adapters.delete(name);

			const state = states.get(name);
			if (state) {
				state.status = "stopped";
				state.stopped_at = new Date().toISOString();
			}
		},

		async stopAll(): Promise<void> {
			const stopPromises = [...adapters.keys()].map((name) =>
				this.stopListener(name),
			);
			await Promise.all(stopPromises);
		},

		getAllStates(): ListenerState[] {
			return [...states.values()];
		},

		getState(name: string): ListenerState | undefined {
			return states.get(name);
		},

		updateDeliveryMode(
			name: string,
			mode: "interruptive" | "non-interruptive",
		): boolean {
			const state = states.get(name);
			if (!state) return false;
			// Update the config entry's delivery mode
			(state.config as { delivery?: string }).delivery = mode;
			return true;
		},

		async onSessionEnd(sessionId: string): Promise<void> {
			// REQ-LNP-077: Stop listeners for ended sessions
			const toStop: string[] = [];
			for (const [name, state] of states) {
				if (
					state.creator_session_id === sessionId &&
					state.config.lifecycle?.auto_stop_on_session_end !== false &&
					state.status !== "stopped"
				) {
					toStop.push(name);
				}
			}

			// Clear delivery queue for the session
			deliveryQueue.clearSession(sessionId);

			for (const name of toStop) {
				await this.stopListener(name);
			}
		},

		getMetrics(): ListenerMetrics {
			const uptime: Record<string, number> = {};
			for (const [name, startTime] of startTimes) {
				const state = states.get(name);
				if (state?.status === "running") {
					uptime[name] = (Date.now() - startTime) / 1000;
				}
			}

			// Build per-listener drop counts for Prometheus counter labels (REQ-LNP-078)
			const dropsByListener: Record<string, number> = {};
			for (const [name, state] of states) {
				if (state.events_dropped > 0) {
					dropsByListener[name] = state.events_dropped;
				}
			}

			return {
				total_events_received: totalEventsReceived,
				total_events_delivered: totalEventsDelivered,
				total_events_dropped: totalEventsDropped,
				listener_uptime: uptime,
				active_listener_count: [...states.values()].filter(
					(s) => s.status === "running",
				).length,
				drops_by_listener: dropsByListener,
				delivery_latencies: Object.fromEntries(deliveryLatencies),
			};
		},

		dispose(): void {
			if (healthCheckTimer) {
				clearInterval(healthCheckTimer);
				healthCheckTimer = null;
			}
			if (orphanTimer) {
				clearInterval(orphanTimer);
				orphanTimer = null;
			}
		},
	};
}
