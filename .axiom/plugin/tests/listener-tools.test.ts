/**
 * Tests for the agent listener API tools.
 *
 * REQ-LNP-020, REQ-LNP-048, REQ-LNP-057.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#2
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-6/task-6.9
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { createDeliveryQueue } from "../src/layers/listeners/delivery";
import { createInjectionEngine } from "../src/layers/listeners/injection";
import { createListenerManager } from "../src/layers/listeners/lifecycle";
import { createSessionRouter } from "../src/layers/listeners/router";
import {
	executeCreateListener,
	executeDeleteListener,
	executeListListeners,
	executeUpdateListener,
} from "../src/layers/listeners/tools";
import type { ListenerManager } from "../src/layers/listeners/lifecycle";

// ── Helpers ──────────────────────────────────────────────────────────────

function makeManager(sessionId = "session-1"): ListenerManager {
	return createListenerManager({
		repoRoot: "/tmp",
		currentSessionId: sessionId,
		router: createSessionRouter({ currentSessionId: sessionId }),
		injectionEngine: createInjectionEngine(),
		deliveryQueue: createDeliveryQueue(),
		healthCheckIntervalMs: 100_000,
		orphanDetectionIntervalMs: 100_000,
	});
}

// ── Tests ────────────────────────────────────────────────────────────────

describe("executeCreateListener", () => {
	let manager: ListenerManager;

	beforeEach(() => {
		manager = makeManager();
	});

	afterEach(async () => {
		manager.dispose();
		await manager.stopAll();
	});

	it("creates a script listener successfully", async () => {
		const result = await executeCreateListener(manager, "session-1", {
			name: "test-listener",
			source: "script",
			config: JSON.stringify({ command: "echo", args: ["hello"] }),
		});
		expect(result).toContain("created and started");
		expect(result).toContain("test-listener");
	});

	it("returns error for invalid source type", async () => {
		const result = await executeCreateListener(manager, "session-1", {
			name: "test-listener",
			source: "invalid-source",
			config: "{}",
		});
		expect(result).toContain("Invalid source type");
	});

	it("returns error for invalid config JSON", async () => {
		const result = await executeCreateListener(manager, "session-1", {
			name: "test-listener",
			source: "script",
			config: "not-valid-json",
		});
		expect(result).toContain("Invalid config JSON");
	});

	it("defaults routing to self (REQ-LNP-048)", async () => {
		const result = await executeCreateListener(manager, "session-1", {
			name: "test-listener",
			source: "script",
			config: JSON.stringify({ command: "echo", args: ["hello"] }),
		});
		expect(result).toContain("self");
	});

	it("accepts custom routing", async () => {
		const result = await executeCreateListener(manager, "session-1", {
			name: "test-listener",
			source: "script",
			config: JSON.stringify({ command: "echo", args: ["hello"] }),
			routing: JSON.stringify({ match: { work_item: "ABC-123" } }),
		});
		expect(result).toContain("ABC-123");
	});
});

describe("executeListListeners", () => {
	let manager: ListenerManager;

	beforeEach(() => {
		manager = makeManager();
	});

	afterEach(async () => {
		manager.dispose();
		await manager.stopAll();
	});

	it("returns 'No active listeners' when empty", () => {
		const result = executeListListeners(manager);
		expect(result).toContain("No active listeners");
	});

	it("lists active listeners in text format", async () => {
		await executeCreateListener(manager, "session-1", {
			name: "my-listener",
			source: "script",
			config: JSON.stringify({ command: "echo", args: ["hello"] }),
		});
		const result = executeListListeners(manager, "text");
		expect(result).toContain("my-listener");
	});

	it("lists active listeners in JSON format", async () => {
		await executeCreateListener(manager, "session-1", {
			name: "my-listener",
			source: "script",
			config: JSON.stringify({ command: "echo", args: ["hello"] }),
		});
		const result = executeListListeners(manager, "json");
		const parsed = JSON.parse(result);
		expect(Array.isArray(parsed)).toBe(true);
		expect(parsed[0].name).toBe("my-listener");
	});
});

describe("executeUpdateListener — REQ-LNP-057", () => {
	let manager: ListenerManager;

	beforeEach(() => {
		manager = makeManager();
	});

	afterEach(async () => {
		manager.dispose();
		await manager.stopAll();
	});

	it("updates delivery mode", async () => {
		await executeCreateListener(manager, "session-1", {
			name: "my-listener",
			source: "script",
			config: JSON.stringify({ command: "echo", args: ["hello"] }),
		});
		const result = executeUpdateListener(manager, {
			name: "my-listener",
			delivery: "interruptive",
		});
		expect(result).toContain("interruptive");
	});

	it("returns error for invalid delivery mode", async () => {
		await executeCreateListener(manager, "session-1", {
			name: "my-listener",
			source: "script",
			config: JSON.stringify({ command: "echo", args: ["hello"] }),
		});
		const result = executeUpdateListener(manager, {
			name: "my-listener",
			delivery: "invalid-mode",
		});
		expect(result).toContain("Invalid delivery mode");
	});

	it("returns error for unknown listener", () => {
		const result = executeUpdateListener(manager, {
			name: "nonexistent",
			delivery: "interruptive",
		});
		expect(result).toContain("not found");
	});
});

describe("executeDeleteListener", () => {
	let manager: ListenerManager;

	beforeEach(() => {
		manager = makeManager();
	});

	afterEach(async () => {
		manager.dispose();
		await manager.stopAll();
	});

	it("stops and removes a listener", async () => {
		await executeCreateListener(manager, "session-1", {
			name: "my-listener",
			source: "script",
			config: JSON.stringify({ command: "echo", args: ["hello"] }),
		});
		const result = await executeDeleteListener(manager, "my-listener");
		expect(result).toContain("stopped and removed");
	});

	it("returns error for unknown listener", async () => {
		const result = await executeDeleteListener(manager, "nonexistent");
		expect(result).toContain("not found");
	});
});
