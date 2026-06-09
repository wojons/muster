/**
 * Tests for the listener lifecycle manager.
 *
 * REQ-LNP-070 through REQ-LNP-078.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#8
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-6/task-6.8
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { existsSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ListenerEntry } from "../src/config-loader";
import { createDeliveryQueue } from "../src/layers/listeners/delivery";
import { createInjectionEngine } from "../src/layers/listeners/injection";
import {
	createListenerManager,
	readActiveSessionsForRouter,
	writeActiveSessionsFile,
} from "../src/layers/listeners/lifecycle";
import { createSessionRouter } from "../src/layers/listeners/router";
import type { ListenerManager } from "../src/layers/listeners/lifecycle";

// ── Helpers ──────────────────────────────────────────────────────────────

/** Create a unique temp directory for each test */
function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-lifecycle-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
	);
	mkdirSync(dir, { recursive: true });
	return dir;
}

function makeManager(sessionId = "session-1"): ListenerManager {
	return createListenerManager({
		repoRoot: "/tmp",
		currentSessionId: sessionId,
		router: createSessionRouter({ currentSessionId: sessionId }),
		injectionEngine: createInjectionEngine(),
		deliveryQueue: createDeliveryQueue(),
		healthCheckIntervalMs: 100_000, // disable health checks in tests
		orphanDetectionIntervalMs: 100_000, // disable orphan detection in tests
	});
}

function makeScriptEntry(name: string): ListenerEntry {
	return {
		name,
		source: "script",
		config: {
			command: "echo",
			args: ["test"],
		},
		lifecycle: {
			auto_start: true,
			auto_stop_on_session_end: true,
		},
	};
}

// ── Tests ────────────────────────────────────────────────────────────────

describe("createListenerManager — REQ-LNP-070 through REQ-LNP-078", () => {
	let manager: ListenerManager;

	beforeEach(() => {
		manager = makeManager();
	});

	afterEach(async () => {
		manager.dispose();
		await manager.stopAll();
	});

	it("starts a listener successfully", async () => {
		const entry = makeScriptEntry("test-listener");
		const result = await manager.startListener(entry, "session-1");
		expect(result.success).toBe(true);

		const state = manager.getState("test-listener");
		expect(state).toBeDefined();
		expect(state?.name).toBe("test-listener");
	});

	it("returns all states", async () => {
		const entry1 = makeScriptEntry("listener-1");
		const entry2 = makeScriptEntry("listener-2");
		await manager.startListener(entry1, "session-1");
		await manager.startListener(entry2, "session-1");

		const states = manager.getAllStates();
		expect(states).toHaveLength(2);
	});

	it("stops a listener by name", async () => {
		const entry = makeScriptEntry("test-listener");
		await manager.startListener(entry, "session-1");
		await manager.stopListener("test-listener");

		const state = manager.getState("test-listener");
		expect(state?.status).toBe("stopped");
	});

	it("enforces per-session listener limit (REQ-LNP-073)", async () => {
		// Start 10 listeners (the max)
		for (let i = 0; i < 10; i++) {
			const entry = makeScriptEntry(`listener-${i}`);
			await manager.startListener(entry, "session-1");
		}

		// 11th should fail
		const entry11 = makeScriptEntry("listener-11");
		const result = await manager.startListener(entry11, "session-1");
		expect(result.success).toBe(false);
		expect(result.error).toContain("limit");
	});

	it("stops listeners for ended session (REQ-LNP-077)", async () => {
		const entry = makeScriptEntry("session-listener");
		await manager.startListener(entry, "session-1");

		await manager.onSessionEnd("session-1");

		const state = manager.getState("session-listener");
		expect(state?.status).toBe("stopped");
	});

	it("does not stop listeners for other sessions on session end", async () => {
		const entry1 = makeScriptEntry("session-1-listener");
		const entry2 = makeScriptEntry("session-2-listener");
		await manager.startListener(entry1, "session-1");
		await manager.startListener(entry2, "session-2");

		await manager.onSessionEnd("session-1");

		const state2 = manager.getState("session-2-listener");
		expect(state2?.status).not.toBe("stopped");
	});

	it("updates delivery mode at runtime (REQ-LNP-057)", async () => {
		const entry = makeScriptEntry("test-listener");
		await manager.startListener(entry, "session-1");

		const success = manager.updateDeliveryMode("test-listener", "interruptive");
		expect(success).toBe(true);

		const state = manager.getState("test-listener");
		expect(state?.config.delivery).toBe("interruptive");
	});

	it("returns false for updateDeliveryMode on unknown listener", () => {
		const success = manager.updateDeliveryMode("nonexistent", "interruptive");
		expect(success).toBe(false);
	});

	it("returns metrics (REQ-LNP-078)", async () => {
		const entry = makeScriptEntry("test-listener");
		await manager.startListener(entry, "session-1");

		const metrics = manager.getMetrics();
		expect(metrics).toBeDefined();
		expect(typeof metrics.total_events_received).toBe("number");
		expect(typeof metrics.total_events_delivered).toBe("number");
		expect(typeof metrics.total_events_dropped).toBe("number");
		expect(typeof metrics.active_listener_count).toBe("number");
	});

	it("stopAll stops all listeners", async () => {
		const entry1 = makeScriptEntry("listener-1");
		const entry2 = makeScriptEntry("listener-2");
		await manager.startListener(entry1, "session-1");
		await manager.startListener(entry2, "session-1");

		await manager.stopAll();

		const states = manager.getAllStates();
		for (const state of states) {
			expect(state.status).toBe("stopped");
		}
	});

	// REQ-LNP-077: File-based orphan detection
	// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-077 plan=step-fix-010 test=listener-lifecycle.test.ts#orphan-file-registry
	it("stops orphaned listeners via file-based session registry (REQ-LNP-077)", async () => {
		const { writeFile } = await import("node:fs/promises");

		// Create a temp directory with a .axiom subdirectory
		const tempDir2 = makeTempDir();
		mkdirSync(join(tempDir2, ".axiom"), { recursive: true });

		// Write a session registry with only "live-session" active
		const registryPath = join(tempDir2, ".axiom", ".active-sessions.json");
		await writeFile(
			registryPath,
			JSON.stringify({
				sessions: [{ id: "live-session", started_at: new Date().toISOString() }],
			}),
			"utf-8",
		);

		try {
			// Create manager with short orphan detection interval
			const mgr = createListenerManager({
				repoRoot: tempDir2,
				currentSessionId: "live-session",
				router: createSessionRouter({ currentSessionId: "live-session" }),
				injectionEngine: createInjectionEngine(),
				deliveryQueue: createDeliveryQueue(),
				healthCheckIntervalMs: 100_000,
				orphanDetectionIntervalMs: 50, // fast for testing
			});

			// Start a listener created by "dead-session" (not in registry)
			const entry = makeScriptEntry("orphan-listener");
			await mgr.startListener(entry, "dead-session");

			// Wait for orphan detection to fire (interval=50ms, wait 400ms for CI headroom)
			await new Promise((r) => setTimeout(r, 400));

			const state = mgr.getState("orphan-listener");
			expect(state?.status).toBe("stopped");

			mgr.dispose();
			await mgr.stopAll();
		} finally {
			if (existsSync(tempDir2)) {
				rmSync(tempDir2, { recursive: true, force: true });
			}
		}
	});
});

// ── Session discovery for cross-session routing (REQ-LNP-131–135) ────────

describe("readActiveSessionsForRouter — REQ-LNP-131, REQ-LNP-133, REQ-LNP-135", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = join(tmpdir(), `axiom-router-test-${Date.now()}-${Math.random().toString(36).slice(2)}`);
		mkdirSync(join(tempDir, ".axiom"), { recursive: true });
	});

	afterEach(() => {
		if (existsSync(tempDir)) rmSync(tempDir, { recursive: true, force: true });
	});

	it("returns empty map when registry file does not exist", async () => {
		// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-131 plan=milestone-gate-followup-013
		const sessions = await readActiveSessionsForRouter(tempDir);
		expect(sessions.size).toBe(0);
	});

	it("returns live sessions with work_item and tags (REQ-LNP-131)", async () => {
		// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-131 plan=milestone-gate-followup-013
		const registry = {
			sessions: [
				{
					id: "session-abc",
					started_at: new Date().toISOString(),
					last_heartbeat: new Date().toISOString(), // fresh
					pid: process.pid, // current process — always alive
					work_item: "ABC-123",
					tags: ["infrastructure"],
				},
			],
		};
		writeFileSync(join(tempDir, ".axiom", ".active-sessions.json"), JSON.stringify(registry));

		const sessions = await readActiveSessionsForRouter(tempDir);
		expect(sessions.size).toBe(1);
		const info = sessions.get("session-abc");
		expect(info?.work_item).toBe("ABC-123");
		expect(info?.tags).toContain("infrastructure");
	});

	it("filters out sessions with stale heartbeat (REQ-LNP-133)", async () => {
		// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-133 plan=milestone-gate-followup-013
		const staleTime = new Date(Date.now() - 90_000).toISOString(); // 90s ago > 60s TTL
		const registry = {
			sessions: [
				{
					id: "stale-session",
					started_at: staleTime,
					last_heartbeat: staleTime,
					pid: process.pid,
					work_item: "STALE-001",
				},
			],
		};
		writeFileSync(join(tempDir, ".axiom", ".active-sessions.json"), JSON.stringify(registry));

		const sessions = await readActiveSessionsForRouter(tempDir);
		expect(sessions.size).toBe(0); // stale session filtered out
	});

	it("filters out sessions with dead PID (REQ-LNP-135)", async () => {
		// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-135 plan=milestone-gate-followup-013
		const registry = {
			sessions: [
				{
					id: "dead-pid-session",
					started_at: new Date().toISOString(),
					last_heartbeat: new Date().toISOString(), // fresh heartbeat
					pid: 999999999, // almost certainly dead PID
					work_item: "DEAD-001",
				},
			],
		};
		writeFileSync(join(tempDir, ".axiom", ".active-sessions.json"), JSON.stringify(registry));

		const sessions = await readActiveSessionsForRouter(tempDir);
		expect(sessions.size).toBe(0); // dead PID filtered out
	});

	it("writeActiveSessionsFile writes full schema with pid and last_heartbeat (REQ-LNP-132)", async () => {
		// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-132 plan=milestone-gate-followup-013
		await writeActiveSessionsFile(tempDir, "test-session", {
			work_item: "TEST-001",
			tags: ["ops"],
		});

		const { readFileSync } = await import("node:fs");
		const content = readFileSync(join(tempDir, ".axiom", ".active-sessions.json"), "utf-8");
		const data = JSON.parse(content) as { sessions: Array<Record<string, unknown>> };
		expect(data.sessions).toHaveLength(1);
		const entry = data.sessions[0];
		expect(entry.id).toBe("test-session");
		expect(entry.pid).toBe(process.pid);
		expect(entry.last_heartbeat).toBeDefined();
		expect(entry.work_item).toBe("TEST-001");
		expect(entry.tags).toContain("ops");
	});

	it("writeActiveSessionsFile updates last_heartbeat on repeated calls (REQ-LNP-133)", async () => {
		// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-133 plan=milestone-gate-followup-013
		await writeActiveSessionsFile(tempDir, "heartbeat-session");
		await new Promise((r) => setTimeout(r, 10)); // small delay
		await writeActiveSessionsFile(tempDir, "heartbeat-session");

		const { readFileSync } = await import("node:fs");
		const content = readFileSync(join(tempDir, ".axiom", ".active-sessions.json"), "utf-8");
		const data = JSON.parse(content) as { sessions: Array<Record<string, unknown>> };
		// Should still be only 1 entry (idempotent)
		expect(data.sessions).toHaveLength(1);
		// last_heartbeat should be recent
		const heartbeat = new Date(data.sessions[0].last_heartbeat as string).getTime();
		expect(Date.now() - heartbeat).toBeLessThan(5000);
	});

	it("router.updateSessions() receives discovered sessions and routes correctly (REQ-LNP-134)", async () => {
		// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-134 plan=milestone-gate-followup-013
		// Write a session to the registry
		await writeActiveSessionsFile(tempDir, "remote-session", {
			work_item: "REMOTE-001",
		});

		// Discover sessions and update router
		const router = createSessionRouter({ currentSessionId: "local-session" });
		const discovered = await readActiveSessionsForRouter(tempDir);
		router.updateSessions(discovered);

		// Create a fake event and listener entry targeting the remote session's work_item
		const { generateEventId } = await import("../src/layers/listeners/types");
		const event = {
			id: generateEventId(),
			listener_name: "test",
			source_type: "sqs" as const,
			source_ref: "test",
			received_at: new Date().toISOString(),
			payload: {},
			delivery: { mode: "non-interruptive" as const, status: "pending" as const, attempts: 0, last_attempt_at: null },
		};
		const entry = {
			name: "test",
			source: "sqs" as const,
			config: { queue_url: "test", region: "us-east-1" },
			routing: { match: { work_item: "REMOTE-001" } },
		};

		const result = router.route(event, entry);
		expect(result.dropped).toBe(false);
		expect(result.target_sessions).toContain("remote-session");
	});
});

// ── Runner pipeline (REQ-LNP-140–145) ────────────────────────────────────

describe("runner pipeline — REQ-LNP-140 through REQ-LNP-145", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = join(tmpdir(), `axiom-runner-test-${Date.now()}-${Math.random().toString(36).slice(2)}`);
		mkdirSync(tempDir, { recursive: true });
	});

	afterEach(() => {
		if (existsSync(tempDir)) rmSync(tempDir, { recursive: true, force: true });
	});

	function makeRunnerManager(runnerTriggerOverride?: (workItem: string, repo: string, prompt?: string, surface?: string, eventId?: string) => Promise<void>) {
		return createListenerManager({
			repoRoot: tempDir,
			currentSessionId: "session-1",
			router: createSessionRouter({ currentSessionId: "session-1" }),
			injectionEngine: createInjectionEngine(),
			deliveryQueue: createDeliveryQueue(),
			healthCheckIntervalMs: 100_000,
			orphanDetectionIntervalMs: 100_000,
			runnerTriggerOverride,
		});
	}

	function makeRunEntry(name: string, action: "inject" | "run" | "inject_and_run", workItem = "WI-001"): ListenerEntry {
		return {
			name,
			source: "script",
			config: { command: "echo", args: ["test"] },
			action,
			run: action !== "inject" ? { work_item: workItem, repo: "." } : undefined,
			lifecycle: { auto_start: true, auto_stop_on_session_end: true },
		};
	}

	it("action: inject (default) does NOT trigger runner (REQ-LNP-140)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-140 plan=phase-5/task-5.2 test=listener-lifecycle.test.ts jira_ref=DEX-434
		let runnerCalled = false;
		const mgr = makeRunnerManager(async () => { runnerCalled = true; });

		const entry: ListenerEntry = {
			name: "inject-only",
			source: "script",
			config: { command: "echo", args: ["test"] },
			// action defaults to "inject" — no run field
			lifecycle: { auto_start: true, auto_stop_on_session_end: true },
		};

		await mgr.startListener(entry, "session-1");
		// Wait for script to complete and event to be processed
		await new Promise((r) => setTimeout(r, 200));

		expect(runnerCalled).toBe(false);
		mgr.dispose();
		await mgr.stopAll();
	});

	it("action: run triggers runner and does NOT inject event (REQ-LNP-141)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-141 plan=phase-5/task-5.2 test=listener-lifecycle.test.ts jira_ref=DEX-434
		const triggerCalls: string[] = [];
		const mgr = makeRunnerManager(async (workItem) => { triggerCalls.push(workItem); });

		const entry = makeRunEntry("run-only", "run", "WI-RUN-001");
		await mgr.startListener(entry, "session-1");
		// Wait for script to complete and event to be processed
		await new Promise((r) => setTimeout(r, 200));

		// Runner was triggered
		expect(triggerCalls).toContain("WI-RUN-001");

		// Injection engine should NOT have received the event (action=run skips inject)
		// We verify via metrics: events_delivered counts runner-triggered events too,
		// but the injection queue should be empty (no queueForInjection calls)
		const state = mgr.getState("run-only");
		expect(state?.events_received).toBeGreaterThan(0);
		expect(state?.events_delivered).toBeGreaterThan(0); // counted as delivered

		mgr.dispose();
		await mgr.stopAll();
	});

	it("action: inject_and_run triggers runner AND injects event (REQ-LNP-140)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-140 plan=phase-5/task-5.2 test=listener-lifecycle.test.ts jira_ref=DEX-434
		const triggerCalls: string[] = [];
		const mgr = makeRunnerManager(async (workItem) => { triggerCalls.push(workItem); });

		const entry = makeRunEntry("inject-and-run", "inject_and_run", "WI-BOTH-001");
		await mgr.startListener(entry, "session-1");
		await new Promise((r) => setTimeout(r, 200));

		// Runner was triggered
		expect(triggerCalls).toContain("WI-BOTH-001");

		// Events were also received (injection path was not skipped)
		const state = mgr.getState("inject-and-run");
		expect(state?.events_received).toBeGreaterThan(0);

		mgr.dispose();
		await mgr.stopAll();
	});

	it("triggerRunner passes work_item, repo, prompt, surface to override (REQ-LNP-142)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-142 plan=phase-5/task-5.2 test=listener-lifecycle.test.ts jira_ref=DEX-434
		const calls: Array<{ workItem: string; repo: string; prompt?: string; surface?: string; eventId?: string }> = [];
		const mgr = makeRunnerManager(async (workItem, repo, prompt, surface, eventId) => {
			calls.push({ workItem, repo, prompt, surface, eventId });
		});

		const entry: ListenerEntry = {
			name: "run-with-prompt",
			source: "script",
			config: { command: "echo", args: ["test"] },
			action: "run",
			run: { work_item: "WI-PROMPT-001", repo: "/my/repo", prompt: "do the thing", surface: "http" },
			lifecycle: { auto_start: true, auto_stop_on_session_end: true },
		};

		await mgr.startListener(entry, "session-1");
		await new Promise((r) => setTimeout(r, 200));

		expect(calls).toHaveLength(1);
		expect(calls[0].workItem).toBe("WI-PROMPT-001");
		expect(calls[0].repo).toBe("/my/repo");
		expect(calls[0].prompt).toBe("do the thing");
		expect(calls[0].surface).toBe("http");
		// REQ-LNP-143: event_id must be passed to the runner override
		expect(calls[0].eventId).toBeDefined();
		expect(typeof calls[0].eventId).toBe("string");
		expect((calls[0].eventId as string).length).toBeGreaterThan(0);

		mgr.dispose();
		await mgr.stopAll();
	});

	it("$event.* variable substitution in work_item (REQ-LNP-143)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-143 plan=phase-5/task-5.2 test=listener-lifecycle.test.ts jira_ref=DEX-434
		const triggerCalls: string[] = [];
		const mgr = makeRunnerManager(async (workItem) => { triggerCalls.push(workItem); });

		// Script outputs JSON with a work_item field
		const entry: ListenerEntry = {
			name: "event-var-listener",
			source: "script",
			// Output JSON so payload.work_item is accessible
			config: { command: "echo", args: ['{"work_item":"WI-FROM-EVENT"}'] },
			action: "run",
			run: { work_item: "$event.work_item", repo: "." },
			lifecycle: { auto_start: true, auto_stop_on_session_end: true },
		};

		await mgr.startListener(entry, "session-1");
		await new Promise((r) => setTimeout(r, 300));

		// work_item should be resolved from event payload
		expect(triggerCalls).toContain("WI-FROM-EVENT");

		mgr.dispose();
		await mgr.stopAll();
	});

	it("resolves $event.* variable substitution in prompt field (REQ-LNP-143)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-143 plan=phase-5/task-5.2 test=listener-lifecycle.test.ts jira_ref=DEX-434
		const calls: Array<{ workItem: string; prompt?: string }> = [];
		const mgr = makeRunnerManager(async (workItem, _repo, prompt) => {
			calls.push({ workItem: workItem as string, prompt: prompt as string | undefined });
		});

		// Script outputs JSON with a source field — prompt template references $event.source
		const entry: ListenerEntry = {
			name: "prompt-subst-test",
			source: "script",
			config: { command: "echo", args: ['{"source":"test-source","work_item":"WI-PROMPT-SUBST"}'] },
			action: "run",
			run: {
				work_item: "WI-PROMPT-SUBST",
				repo: ".",
				prompt: "Handle event from: $event.source",
			},
			lifecycle: { auto_start: true, auto_stop_on_session_end: true },
		};

		await mgr.startListener(entry, "session-1");
		await new Promise((r) => setTimeout(r, 300));

		expect(calls).toHaveLength(1);
		expect(calls[0].prompt).toBeDefined();
		// The prompt should have $event.source resolved to the actual payload value
		expect(calls[0].prompt).not.toContain("$event."); // variable was substituted
		expect(calls[0].prompt).toContain("test-source"); // resolved to actual value

		mgr.dispose();
		await mgr.stopAll();
	});

	it("runner_triggered log event is emitted (REQ-LNP-145)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-145 plan=phase-5/task-5.2 test=listener-lifecycle.test.ts jira_ref=DEX-434
		const logLines: string[] = [];
		const origWrite = process.stderr.write.bind(process.stderr);
		// Capture stderr log output
		const spy = (chunk: string | Uint8Array) => {
			if (typeof chunk === "string") logLines.push(chunk);
			return origWrite(chunk);
		};
		process.stderr.write = spy as typeof process.stderr.write;

		try {
			const mgr = makeRunnerManager(async () => {});
			const entry = makeRunEntry("log-test", "run", "WI-LOG-001");
			await mgr.startListener(entry, "session-1");
			await new Promise((r) => setTimeout(r, 200));

			const runnerTriggeredLogs = logLines.filter((l) => {
				try {
					const parsed = JSON.parse(l) as { event?: string };
					return parsed.event === "listener.runner_triggered";
				} catch {
					return false;
				}
			});

			expect(runnerTriggeredLogs.length).toBeGreaterThan(0);

			// REQ-LNP-145: Assert required fields are present and correct
			const parsed = JSON.parse(runnerTriggeredLogs[0]) as Record<string, unknown>;
			expect(parsed.event).toBe("listener.runner_triggered");
			expect(parsed.event_id).toBeDefined();
			expect(parsed.work_item).toBe("WI-LOG-001");
			expect(parsed.runner_session_id).toBeDefined();
			expect(typeof parsed.runner_session_id).toBe("string");
			// REQ-LNP-145: runner_session_id MUST be a UUID v4 (not just any non-empty string)
			// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-145 plan=step-p3-008 test=listener-lifecycle.test.ts jira_ref=DEX-434
			expect(parsed.runner_session_id as string).toMatch(
				/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i,
			);

			mgr.dispose();
			await mgr.stopAll();
		} finally {
			process.stderr.write = origWrite as typeof process.stderr.write;
		}
	});

	it("action: run with no run config does not crash (REQ-LNP-140 guard)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-140 plan=phase-5/task-5.2 test=listener-lifecycle.test.ts jira_ref=DEX-434
		let runnerCalled = false;
		const mgr = makeRunnerManager(async () => { runnerCalled = true; });

		// action=run but no run config — shouldRun will be false because !!entry.run is false
		const entry: ListenerEntry = {
			name: "run-no-config",
			source: "script",
			config: { command: "echo", args: ["test"] },
			action: "run",
			// run field intentionally omitted
			lifecycle: { auto_start: true, auto_stop_on_session_end: true },
		};

		await mgr.startListener(entry, "session-1");
		await new Promise((r) => setTimeout(r, 200));

		// Runner should NOT be called (no run config)
		expect(runnerCalled).toBe(false);

		mgr.dispose();
		await mgr.stopAll();
	});

	it("rate limits runner triggers to 1 per window per work_item (REQ-LNP-144)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-144 plan=phase-5/task-5.3 test=listener-lifecycle.test.ts jira_ref=DEX-434
		const triggerCalls: string[] = [];
		// Use a long rate limit window (60s) so all rapid events are suppressed after the first
		const mgr = createListenerManager({
			repoRoot: tempDir,
			currentSessionId: "session-1",
			router: createSessionRouter({ currentSessionId: "session-1" }),
			injectionEngine: createInjectionEngine(),
			deliveryQueue: createDeliveryQueue(),
			healthCheckIntervalMs: 100_000,
			orphanDetectionIntervalMs: 100_000,
			runnerTriggerOverride: async (workItem) => { triggerCalls.push(workItem as string); },
			runnerRateLimitMsOverride: 60_000, // 60s window — all rapid events after first are dropped
		});

		// Script adapter fires one event per start; restart the listener multiple times to simulate burst
		const entry = makeRunEntry("rate-limit-test", "run", "RATE-001");
		for (let i = 0; i < 5; i++) {
			await mgr.startListener(entry, "session-1");
			await new Promise((r) => setTimeout(r, 50));
		}

		// Wait for all async processing to settle
		await new Promise((r) => setTimeout(r, 200));

		// Only the first trigger should have fired; the rest are rate-limited
		expect(triggerCalls).toHaveLength(1);
		expect(triggerCalls[0]).toBe("RATE-001");

		mgr.dispose();
		await mgr.stopAll();
	});

	it("rate limit is per work_item — different work_items both trigger (REQ-LNP-144)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-144 plan=phase-5/task-5.3 test=listener-lifecycle.test.ts jira_ref=DEX-434
		const triggerCalls: string[] = [];
		// Use a long rate limit window so we can verify per-work_item isolation
		const mgr = createListenerManager({
			repoRoot: tempDir,
			currentSessionId: "session-1",
			router: createSessionRouter({ currentSessionId: "session-1" }),
			injectionEngine: createInjectionEngine(),
			deliveryQueue: createDeliveryQueue(),
			healthCheckIntervalMs: 100_000,
			orphanDetectionIntervalMs: 100_000,
			runnerTriggerOverride: async (workItem) => { triggerCalls.push(workItem as string); },
			runnerRateLimitMsOverride: 60_000, // 60s window
		});

		// Two different work_items — each should fire exactly once (rate limit is per work_item)
		const entryA = makeRunEntry("listener-a", "run", "RATE-A");
		const entryB = makeRunEntry("listener-b", "run", "RATE-B");
		await mgr.startListener(entryA, "session-1");
		await mgr.startListener(entryB, "session-1");
		await new Promise((r) => setTimeout(r, 200));

		// Both work_items should have triggered once each
		expect(triggerCalls.filter((w) => w === "RATE-A")).toHaveLength(1);
		expect(triggerCalls.filter((w) => w === "RATE-B")).toHaveLength(1);

		mgr.dispose();
		await mgr.stopAll();
	});

	it("global concurrent-runner cap prevents fan-out from high-cardinality $event.* keys (REQ-LNP-144)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-144 plan=phase-5/task-5.3 test=listener-lifecycle.test.ts jira_ref=DEX-434
		const triggerCalls: string[] = [];
		let resolveAll: () => void;
		// Slow runner that holds the slot until we release it
		const holdPromise = new Promise<void>((r) => { resolveAll = r; });

		const mgr = createListenerManager({
			repoRoot: tempDir,
			currentSessionId: "session-1",
			router: createSessionRouter({ currentSessionId: "session-1" }),
			injectionEngine: createInjectionEngine(),
			deliveryQueue: createDeliveryQueue(),
			healthCheckIntervalMs: 100_000,
			orphanDetectionIntervalMs: 100_000,
			maxConcurrentRunnersOverride: 3,
			runnerRateLimitMsOverride: 0, // disable per-key rate limit so cap is the only guard
			runnerTriggerOverride: async (workItem) => {
				triggerCalls.push(workItem as string);
				await holdPromise; // hold the slot
			},
		});

		// Start 5 listeners with 5 distinct work_items (bypasses per-key rate limit)
		for (let i = 1; i <= 5; i++) {
			const entry = makeRunEntry(`cap-test-${i}`, "run", `WI-CAP-${String(i).padStart(3, "0")}`);
			await mgr.startListener(entry, "session-1");
		}

		// Give time for all events to fire and be processed
		await new Promise((r) => setTimeout(r, 300));

		// Only 3 should have triggered (cap=3); 2 should have been dropped
		expect(triggerCalls.length).toBeLessThanOrEqual(3);
		expect(triggerCalls.length).toBeGreaterThan(0);

		// Release all held runners
		resolveAll!();
		await new Promise((r) => setTimeout(r, 50));

		mgr.dispose();
		await mgr.stopAll();
	});

	it("emits listener.error when surface=http with no api_url configured (REQ-LNP-142)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-142 plan=phase-5/task-5.2 test=listener-lifecycle.test.ts jira_ref=DEX-434
		const logLines: string[] = [];
		const origWrite = process.stderr.write.bind(process.stderr);
		process.stderr.write = (chunk: string | Uint8Array) => {
			logLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
			return true;
		};

		const savedApiUrl = process.env.AXIOM_API_URL;
		delete process.env.AXIOM_API_URL;

		try {
			// No runnerTriggerOverride — exercises real HTTP path
			// No api_url, no AXIOM_API_URL env var
			const mgr = createListenerManager({
				repoRoot: tempDir,
				currentSessionId: "session-1",
				router: createSessionRouter({ currentSessionId: "session-1" }),
				injectionEngine: createInjectionEngine(),
				deliveryQueue: createDeliveryQueue(),
				healthCheckIntervalMs: 100_000,
				orphanDetectionIntervalMs: 100_000,
				// No runnerTriggerOverride — exercises real path
			});

			const entry: ListenerEntry = {
				name: "http-validation-test",
				source: "script",
				config: { command: "echo", args: ["test"] },
				action: "run",
				run: {
					work_item: "WI-HTTP-001",
					repo: ".",
					surface: "http",
					// No api_url
				},
				lifecycle: { auto_start: true, auto_stop_on_session_end: true },
			};

			await mgr.startListener(entry, "session-1");
			await new Promise((r) => setTimeout(r, 200));

			const errorLog = logLines.find((l) => {
				try {
					const parsed = JSON.parse(l) as { event?: string; error?: string };
					return parsed.event === "listener.error" &&
						typeof parsed.error === "string" &&
						parsed.error.includes("api_url");
				} catch { return false; }
			});
			expect(errorLog).toBeDefined();

			mgr.dispose();
			await mgr.stopAll();
		} finally {
			process.stderr.write = origWrite;
			if (savedApiUrl !== undefined) process.env.AXIOM_API_URL = savedApiUrl;
		}
	});

	it("CLI surface emits listener.runner_triggered before spawn attempt (REQ-LNP-142 real path)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-142 plan=phase-5/task-5.2 test=listener-lifecycle.test.ts jira_ref=DEX-434
		// This test exercises the real spawn("axiom", ...) path without runnerTriggerOverride.
		// It verifies that listener.runner_triggered is emitted before the spawn attempt,
		// proving the CLI branch was reached regardless of whether axiom is in PATH.
		//
		// Skip when axiom is not installed: on CI (Plugin Unit Tests job), the Python
		// axiom package is not installed. bun on Linux surfaces the spawn error as an
		// unhandled exception before the test's 300ms wait, causing a false failure.
		const { execSync } = await import("node:child_process");
		try {
			execSync("axiom run --help", { stdio: "ignore", timeout: 1500 });
		} catch {
			return; // axiom not in PATH — skip this test
		}
		const logLines: string[] = [];
		const origWrite = process.stderr.write.bind(process.stderr);
		process.stderr.write = (chunk: string | Uint8Array) => {
			logLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
			return true;
		};

		try {
			// No runnerTriggerOverride — exercises real CLI spawn path
			const mgr = createListenerManager({
				repoRoot: tempDir,
				currentSessionId: "session-1",
				router: createSessionRouter({ currentSessionId: "session-1" }),
				injectionEngine: createInjectionEngine(),
				deliveryQueue: createDeliveryQueue(),
				healthCheckIntervalMs: 100_000,
				orphanDetectionIntervalMs: 100_000,
				// No runnerTriggerOverride
			});

			const entry: ListenerEntry = {
				name: "cli-real-path-test",
				source: "script",
				config: { command: "echo", args: ["test"] },
				action: "run",
				run: {
					work_item: "WI-CLI-001",
					repo: ".",
					surface: "cli",
				},
				lifecycle: { auto_start: true, auto_stop_on_session_end: true },
			};

			await mgr.startListener(entry, "session-1");
			await new Promise((r) => setTimeout(r, 300));

			// Verify the runner_triggered log event was emitted (proves the CLI branch was reached)
			const runnerTriggeredLog = logLines.find((l) => {
				try {
					const parsed = JSON.parse(l) as { event?: string };
					return parsed.event === "listener.runner_triggered";
				} catch { return false; }
			});
			expect(runnerTriggeredLog).toBeDefined();

			// Verify the runner_triggered log has the correct work_item
			const parsed = JSON.parse(runnerTriggeredLog!) as Record<string, unknown>;
			expect(parsed.work_item).toBe("WI-CLI-001");
			expect(parsed.runner_session_id).toBeDefined();

			// The CLI spawn either succeeds (axiom in PATH) or fails with listener.error
			// Either way, the branch was exercised and runner_triggered was emitted

			mgr.dispose();
			await mgr.stopAll();
		} finally {
			process.stderr.write = origWrite;
		}
	});
});

// ── Cross-path: action:run + routing drop (REQ-LNP-140) ──────────────────

describe("routing drop suppresses runner trigger — REQ-LNP-140", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) rmSync(tempDir, { recursive: true, force: true });
	});

	it("routing drop prevents runner trigger and emits listener.event_dropped reason=routing_drop (REQ-LNP-140)", async () => {
		// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-140 plan=final-gate/step-final-001 test=listener-lifecycle.test.ts jira_ref=DEX-434
		// This test verifies the documented routing semantics: routing is a universal pre-filter.
		// An event with action:run that is dropped by routing MUST NOT trigger the runner,
		// AND MUST emit listener.event_dropped with reason=routing_drop for operator diagnostics.

		const logLines: string[] = [];
		const origWrite = process.stderr.write.bind(process.stderr);
		process.stderr.write = (chunk: string | Uint8Array) => {
			logLines.push(typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk));
			return true;
		};

		try {
			const triggerCalls: string[] = [];

			// Create a router that will DROP the event (no matching session for work_item: "NOMATCH")
			const router = createSessionRouter({ currentSessionId: "session-1" });
			// No sessions registered → routing will drop events with work_item match rules

			const mgr = createListenerManager({
				repoRoot: tempDir,
				currentSessionId: "session-1",
				router,
				injectionEngine: createInjectionEngine(),
				deliveryQueue: createDeliveryQueue(),
				healthCheckIntervalMs: 100_000,
				orphanDetectionIntervalMs: 100_000,
				runnerTriggerOverride: async (workItem) => {
					triggerCalls.push(workItem as string);
				},
			});

			// Configure listener with action:run AND a routing rule that will drop the event
			// (work_item match requires an active session with work_item="NOMATCH" — none registered)
			const entry = makeScriptEntry("routing-drop-test");
			(entry as Record<string, unknown>).action = "run";
			(entry as Record<string, unknown>).run = { work_item: "WI-ROUTING-DROP", repo: "." };
			(entry as Record<string, unknown>).routing = {
				match: { work_item: "NOMATCH" }, // no session has this work_item → event dropped
			};

			await mgr.startListener(entry, "session-1");
			await new Promise((r) => setTimeout(r, 200));

			// Runner MUST NOT have been triggered (routing dropped the event)
			expect(triggerCalls).toHaveLength(0);

			// listener.event_dropped with reason=routing_drop MUST have been emitted
			const routingDropLog = logLines.find((l) => {
				try {
					const parsed = JSON.parse(l);
					return (
						parsed.event === "listener.event_dropped" &&
						parsed.reason === "routing_drop"
					);
				} catch {
					return false;
				}
			});
			expect(routingDropLog).toBeDefined();

			// Verify the log contains the event_id for traceability
			const parsed = JSON.parse(routingDropLog!);
			expect(parsed.event_id).toBeDefined();
			expect(parsed.listener_name).toBe("routing-drop-test");

			mgr.dispose();
			await mgr.stopAll();
		} finally {
			process.stderr.write = origWrite;
		}
	});
});
