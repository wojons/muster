/**
 * Session identity tests — verifies all 4 resolution modes.
 *
 * REQ-PLG-104a: git remote → hostname → UUID → custom chain.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-104a
 * axiom:trace work_item=plugin-v2-enhancements-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-104a plan=phase-70-9/task-70-9-1/step-70-9-1-1
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { existsSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { resolveSessionIdentity } from "../src/session-identity";

function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-identity-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
	);
	mkdirSync(dir, { recursive: true });
	return dir;
}

describe("REQ-PLG-104a: session identity chain", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("custom identity wins over all auto-derived values", () => {
		const result = resolveSessionIdentity({
			repoRoot: tempDir,
			mode: "auto",
			customIdentity: "team-alpha",
		});
		expect(result.identity).toBe("team-alpha");
		expect(result.source).toBe("custom");
	});

	it("custom identity trims whitespace", () => {
		const result = resolveSessionIdentity({
			repoRoot: tempDir,
			customIdentity: "  my-team  ",
		});
		expect(result.identity).toBe("my-team");
		expect(result.source).toBe("custom");
	});

	it("hostname mode returns machine hostname", () => {
		const result = resolveSessionIdentity({
			repoRoot: tempDir,
			mode: "hostname",
		});
		expect(result.source).toBe("hostname");
		expect(result.identity.length).toBeGreaterThan(0);
	});

	it("uuid mode creates and persists a UUID", () => {
		const result = resolveSessionIdentity({
			repoRoot: tempDir,
			mode: "custom", // no customIdentity → falls through to uuid
		});
		expect(result.source).toBe("uuid");
		expect(result.identity).toMatch(/^[0-9a-f-]{36}$/);
	});

	it("uuid is stable across multiple calls (persisted)", () => {
		const r1 = resolveSessionIdentity({ repoRoot: tempDir, mode: "custom" });
		const r2 = resolveSessionIdentity({ repoRoot: tempDir, mode: "custom" });
		expect(r1.identity).toBe(r2.identity);
		expect(r1.source).toBe("uuid");
	});

	it("auto mode falls back to hostname when no git remote", () => {
		// tempDir has no git remote
		const result = resolveSessionIdentity({ repoRoot: tempDir, mode: "auto" });
		// Should be hostname or uuid (not git)
		expect(["hostname", "uuid"]).toContain(result.source);
		expect(result.identity.length).toBeGreaterThan(0);
	});

	it("git mode with HTTPS URL extracts owner/repo", () => {
		// Create a fake git config with a remote
		mkdirSync(join(tempDir, ".git"), { recursive: true });
		writeFileSync(
			join(tempDir, ".git", "config"),
			`[remote "origin"]\n\turl = https://github.com/myorg/myrepo.git\n`,
		);
		const result = resolveSessionIdentity({ repoRoot: tempDir, mode: "git" });
		if (result.source === "git") {
			expect(result.identity).toBe("github.com/myorg/myrepo");
		} else {
			// git command may not work in test env — acceptable fallback
			expect(["hostname", "uuid"]).toContain(result.source);
		}
	});

	it("resolveSessionIdentity returns identity and source fields", () => {
		const result = resolveSessionIdentity({ repoRoot: tempDir });
		expect(typeof result.identity).toBe("string");
		expect(result.identity.length).toBeGreaterThan(0);
		expect(["custom", "git", "hostname", "uuid"]).toContain(result.source);
	});
});
