/**
 * Progressive command surface tests — verifies maturity-based command visibility
 * and hidden unavailable commands per REQ-PLG-020 and REQ-PLG-021.
 *
 * These tests are written BEFORE the command gating implementation exists.
 * They MUST fail initially and drive the implementation.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-020, #REQ-PLG-021
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-020 plan=phase-70-1/task-70-1-4/step-70-1-4-1
 */
import { describe, expect, it } from "bun:test";
import { mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
// ── Import the command surface module (will fail until implemented) ──────
// REQ-PLG-020: Progressive command surface based on maturity level
import {
	COMMAND_VISIBILITY_MATRIX,
	getCommandsForLevel,
	handleAboveLevelCommand,
} from "../src/commands";
import { MaturityLevel } from "../src/detect";

/** Create a unique temp directory for each test */
function _makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-commands-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
	);
	mkdirSync(dir, { recursive: true });
	return dir;
}

/** Helper to create nested directories */
function _mkdirs(...paths: string[]): void {
	for (const p of paths) {
		mkdirSync(p, { recursive: true });
	}
}

/** Helper to create a file with content */
function _touch(filePath: string, content = ""): void {
	mkdirSync(join(filePath, ".."), { recursive: true });
	writeFileSync(filePath, content);
}

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-020: Progressive command visibility matrix
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-020: Progressive command visibility matrix", () => {
	// ── Level 0: New repo ─────────────────────────────────────────────────
	describe("Level 0 (New)", () => {
		it("exposes /axiom-setup at Level 0", () => {
			const cmds = getCommandsForLevel(MaturityLevel.New);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-setup");
		});

		it("exposes /axiom-help at Level 0", () => {
			const cmds = getCommandsForLevel(MaturityLevel.New);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-help");
		});

		it("exposes /axiom-status at Level 0", () => {
			const cmds = getCommandsForLevel(MaturityLevel.New);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-status");
		});

		it("exposes /axiom-next at Level 0", () => {
			const cmds = getCommandsForLevel(MaturityLevel.New);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-next");
		});

		it("exposes /axiom-glossary at Level 0", () => {
			const cmds = getCommandsForLevel(MaturityLevel.New);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-glossary");
		});

		it("exposes /axiom-privacy at Level 0", () => {
			// REQ-WEAVE-305: /axiom-privacy is available at Level 0 (all users)
			const cmds = getCommandsForLevel(MaturityLevel.New);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-privacy");
		});

		it("exposes exactly 6 commands at Level 0", () => {
			// REQ-PLG-020 matrix: setup, help, status, next, glossary, privacy
			const cmds = getCommandsForLevel(MaturityLevel.New);
			expect(cmds.length).toBe(6);
		});

		it("does NOT expose /axiom-spec at Level 0", () => {
			const cmds = getCommandsForLevel(MaturityLevel.New);
			const names = cmds.map((c) => c.name);
			expect(names).not.toContain("/axiom-spec");
		});

		it("does NOT expose /axiom-plan at Level 0", () => {
			const cmds = getCommandsForLevel(MaturityLevel.New);
			const names = cmds.map((c) => c.name);
			expect(names).not.toContain("/axiom-plan");
		});

		it("does NOT expose /axiom-run at Level 0", () => {
			const cmds = getCommandsForLevel(MaturityLevel.New);
			const names = cmds.map((c) => c.name);
			expect(names).not.toContain("/axiom-run");
		});

		it("does NOT expose /axiom-release at Level 0", () => {
			const cmds = getCommandsForLevel(MaturityLevel.New);
			const names = cmds.map((c) => c.name);
			expect(names).not.toContain("/axiom-release");
		});
	});

	// ── Level 1: Scaffolded ───────────────────────────────────────────────
	describe("Level 1 (Scaffolded)", () => {
		it("exposes /axiom-spec at Level 1", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Scaffolded);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-spec");
		});

		it("still exposes Level 0 commands at Level 1", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Scaffolded);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-setup");
			expect(names).toContain("/axiom-help");
			expect(names).toContain("/axiom-status");
			expect(names).toContain("/axiom-next");
			expect(names).toContain("/axiom-glossary");
		});

		it("exposes exactly 7 commands at Level 1", () => {
			// Level 0 commands (6) + /axiom-spec
			const cmds = getCommandsForLevel(MaturityLevel.Scaffolded);
			expect(cmds.length).toBe(7);
		});

		it("does NOT expose /axiom-plan at Level 1", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Scaffolded);
			const names = cmds.map((c) => c.name);
			expect(names).not.toContain("/axiom-plan");
		});

		it("does NOT expose /axiom-run at Level 1", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Scaffolded);
			const names = cmds.map((c) => c.name);
			expect(names).not.toContain("/axiom-run");
		});
	});

	// ── Level 2: Specced ──────────────────────────────────────────────────
	describe("Level 2 (Specced)", () => {
		it("exposes /axiom-plan at Level 2", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Specced);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-plan");
		});

		it("still exposes Level 0 and Level 1 commands at Level 2", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Specced);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-setup");
			expect(names).toContain("/axiom-help");
			expect(names).toContain("/axiom-status");
			expect(names).toContain("/axiom-spec");
			expect(names).toContain("/axiom-next");
			expect(names).toContain("/axiom-glossary");
		});

		it("exposes exactly 9 commands at Level 2", () => {
			// Level 1 commands (7) + /axiom-plan + /axiom-annotate
			const cmds = getCommandsForLevel(MaturityLevel.Specced);
			expect(cmds.length).toBe(9);
		});

		it("exposes /axiom-annotate at Level 2", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Specced);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-annotate");
		});

		it("does NOT expose /axiom-run at Level 2", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Specced);
			const names = cmds.map((c) => c.name);
			expect(names).not.toContain("/axiom-run");
		});

		it("does NOT expose /axiom-verify at Level 2", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Specced);
			const names = cmds.map((c) => c.name);
			expect(names).not.toContain("/axiom-verify");
		});

		it("does NOT expose /axiom-loop at Level 2", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Specced);
			const names = cmds.map((c) => c.name);
			expect(names).not.toContain("/axiom-loop");
		});
	});

	// ── Level 3: Active ───────────────────────────────────────────────────
	describe("Level 3 (Active)", () => {
		it("exposes /axiom-run at Level 3", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Active);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-run");
		});

		it("exposes /axiom-verify at Level 3", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Active);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-verify");
		});

		it("exposes /axiom-loop at Level 3", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Active);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-loop");
		});

		it("exposes /axiom-share at Level 3", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Active);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-share");
		});

		it("exposes /axiom-pulse at Level 3", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Active);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-pulse");
		});

		it("exposes /axiom-spinup at Level 3", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Active);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-spinup");
		});

		it("still exposes all lower-level commands at Level 3", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Active);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-setup");
			expect(names).toContain("/axiom-help");
			expect(names).toContain("/axiom-status");
			expect(names).toContain("/axiom-spec");
			expect(names).toContain("/axiom-plan");
			expect(names).toContain("/axiom-next");
			expect(names).toContain("/axiom-glossary");
		});

		it("exposes exactly 20 commands at Level 3", () => {
			// Level 2 (9) + run, verify, loop, share, pulse, spinup, backend, wave, review, render, status-page = 20
			const cmds = getCommandsForLevel(MaturityLevel.Active);
			expect(cmds.length).toBe(20);
		});

		it("exposes /axiom-wave at Level 3", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Active);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-wave");
		});
		it("exposes /axiom-status-page at Level 3", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Active);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-status-page");
		});

		it("does NOT expose /axiom-release at Level 3", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Active);
			const names = cmds.map((c) => c.name);
			expect(names).not.toContain("/axiom-release");
		});
	});

	// ── Level 4: Mature ───────────────────────────────────────────────────
	describe("Level 4 (Mature)", () => {
		it("exposes /axiom-release at Level 4", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Mature);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-release");
		});

		it("does NOT expose /axiom-next at Level 4", () => {
			// REQ-PLG-020 matrix: /axiom-next is NOT available at Level 4
			// (already at max maturity, no "next" to advance to)
			const cmds = getCommandsForLevel(MaturityLevel.Mature);
			const names = cmds.map((c) => c.name);
			expect(names).not.toContain("/axiom-next");
		});

		it("still exposes all Level 3 commands except /axiom-next at Level 4", () => {
			const cmds = getCommandsForLevel(MaturityLevel.Mature);
			const names = cmds.map((c) => c.name);
			expect(names).toContain("/axiom-setup");
			expect(names).toContain("/axiom-help");
			expect(names).toContain("/axiom-status");
			expect(names).toContain("/axiom-spec");
			expect(names).toContain("/axiom-plan");
			expect(names).toContain("/axiom-run");
			expect(names).toContain("/axiom-verify");
			expect(names).toContain("/axiom-loop");
			expect(names).toContain("/axiom-share");
			expect(names).toContain("/axiom-pulse");
			expect(names).toContain("/axiom-spinup");
			expect(names).toContain("/axiom-backend");
			expect(names).toContain("/axiom-glossary");
		});

		it("exposes exactly 20 commands at Level 4", () => {
			// Level 3 (20) - /axiom-next + /axiom-release = 20
			const cmds = getCommandsForLevel(MaturityLevel.Mature);
			expect(cmds.length).toBe(20);
		});
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-021: Hidden unavailable commands
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-021: Commands not yet available MUST NOT appear", () => {
	it("Level 0 command list contains no Level 1+ commands", () => {
		const cmds = getCommandsForLevel(MaturityLevel.New);
		const names = new Set(cmds.map((c) => c.name));
		// These commands should be completely absent, not just disabled
		const higherLevelCommands = [
			"/axiom-spec",
			"/axiom-plan",
			"/axiom-run",
			"/axiom-verify",
			"/axiom-loop",
			"/axiom-share",
			"/axiom-pulse",
			"/axiom-release",
			"/axiom-spinup",
		];
		for (const cmd of higherLevelCommands) {
			expect(names.has(cmd)).toBe(false);
		}
	});

	it("Level 1 command list contains no Level 2+ commands", () => {
		const cmds = getCommandsForLevel(MaturityLevel.Scaffolded);
		const names = new Set(cmds.map((c) => c.name));
		const higherLevelCommands = [
			"/axiom-plan",
			"/axiom-run",
			"/axiom-verify",
			"/axiom-loop",
			"/axiom-share",
			"/axiom-pulse",
			"/axiom-release",
			"/axiom-spinup",
		];
		for (const cmd of higherLevelCommands) {
			expect(names.has(cmd)).toBe(false);
		}
	});

	it("Level 2 command list contains no Level 3+ commands", () => {
		const cmds = getCommandsForLevel(MaturityLevel.Specced);
		const names = new Set(cmds.map((c) => c.name));
		const higherLevelCommands = [
			"/axiom-run",
			"/axiom-verify",
			"/axiom-loop",
			"/axiom-share",
			"/axiom-pulse",
			"/axiom-release",
			"/axiom-spinup",
		];
		for (const cmd of higherLevelCommands) {
			expect(names.has(cmd)).toBe(false);
		}
	});

	it("Level 3 command list does not contain /axiom-release", () => {
		const cmds = getCommandsForLevel(MaturityLevel.Active);
		const names = new Set(cmds.map((c) => c.name));
		expect(names.has("/axiom-release")).toBe(false);
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// COMMAND_VISIBILITY_MATRIX structure validation
// ═══════════════════════════════════════════════════════════════════════════
describe("COMMAND_VISIBILITY_MATRIX structure", () => {
	it("is a non-empty array", () => {
		expect(Array.isArray(COMMAND_VISIBILITY_MATRIX)).toBe(true);
		expect(COMMAND_VISIBILITY_MATRIX.length).toBeGreaterThan(0);
	});

	it("each entry has name, description, and minLevel", () => {
		for (const cmd of COMMAND_VISIBILITY_MATRIX) {
			expect(typeof cmd.name).toBe("string");
			expect(cmd.name.startsWith("/axiom-")).toBe(true);
			expect(typeof cmd.description).toBe("string");
			expect(cmd.description.length).toBeGreaterThan(0);
			expect(typeof cmd.minLevel).toBe("number");
			expect(cmd.minLevel).toBeGreaterThanOrEqual(0);
			expect(cmd.minLevel).toBeLessThanOrEqual(4);
		}
	});

	it("contains all 20 spec-defined commands", () => {
		// REQ-PLG-020 matrix defines these commands (20 total including Wave commands)
		const expectedCommands = [
			"/axiom-setup",
			"/axiom-help",
			"/axiom-status",
			"/axiom-spec",
			"/axiom-plan",
			"/axiom-annotate",
			"/axiom-run",
			"/axiom-verify",
			"/axiom-loop",
			"/axiom-share",
			"/axiom-pulse",
			"/axiom-release",
			"/axiom-spinup",
			"/axiom-backend",
			"/axiom-wave",
			"/axiom-review",
			"/axiom-render",
			"/axiom-status-page",
			"/axiom-next",
			"/axiom-glossary",
			// axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-305 plan=phase-5.0/task-5.0.privacy-command/step-5.0.1
			"/axiom-privacy",
		];
		const matrixNames = COMMAND_VISIBILITY_MATRIX.map((c) => c.name);
		for (const expected of expectedCommands) {
			expect(matrixNames).toContain(expected);
		}
	});

	it("/axiom-next has maxLevel 3 (not available at Level 4)", () => {
		const nextCmd = COMMAND_VISIBILITY_MATRIX.find(
			(c) => c.name === "/axiom-next",
		);
		expect(nextCmd).toBeDefined();
		// /axiom-next should have a maxLevel property set to 3
		expect(nextCmd?.maxLevel).toBe(3);
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// CommandDefinition type validation
// ═══════════════════════════════════════════════════════════════════════════
describe("CommandDefinition type", () => {
	it("getCommandsForLevel returns CommandDefinition objects", () => {
		const cmds = getCommandsForLevel(MaturityLevel.New);
		expect(cmds.length).toBeGreaterThan(0);
		const first = cmds[0];
		expect(typeof first.name).toBe("string");
		expect(typeof first.description).toBe("string");
		expect(typeof first.minLevel).toBe("number");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// Progressive disclosure: commands are additive (REQ-PLG-025 at Level 3+)
// ═══════════════════════════════════════════════════════════════════════════
describe("Progressive disclosure is additive", () => {
	it("Level 1 commands are a superset of Level 0 commands (minus /axiom-next edge case)", () => {
		const l0 = getCommandsForLevel(MaturityLevel.New).map((c) => c.name);
		const l1 = getCommandsForLevel(MaturityLevel.Scaffolded).map((c) => c.name);
		for (const cmd of l0) {
			expect(l1).toContain(cmd);
		}
	});

	it("Level 2 commands are a superset of Level 1 commands", () => {
		const l1 = getCommandsForLevel(MaturityLevel.Scaffolded).map((c) => c.name);
		const l2 = getCommandsForLevel(MaturityLevel.Specced).map((c) => c.name);
		for (const cmd of l1) {
			expect(l2).toContain(cmd);
		}
	});

	it("Level 3 commands are a superset of Level 2 commands", () => {
		const l2 = getCommandsForLevel(MaturityLevel.Specced).map((c) => c.name);
		const l3 = getCommandsForLevel(MaturityLevel.Active).map((c) => c.name);
		for (const cmd of l2) {
			expect(l3).toContain(cmd);
		}
	});

	it("Level 4 commands include all Level 3 commands except /axiom-next", () => {
		const l3 = getCommandsForLevel(MaturityLevel.Active).map((c) => c.name);
		const l4 = getCommandsForLevel(MaturityLevel.Mature).map((c) => c.name);
		for (const cmd of l3) {
			if (cmd === "/axiom-next") continue; // /axiom-next drops off at Level 4
			expect(l4).toContain(cmd);
		}
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-022: Above-level command attempts return conversational guidance
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-022: Above-level command guidance", () => {
	describe("returns 'allowed' for commands at or above current level", () => {
		it("/axiom-setup is allowed at Level 0", () => {
			const result = handleAboveLevelCommand(
				"/axiom-setup",
				MaturityLevel.New,
			);
			expect(result.status).toBe("allowed");
			expect(result.command).toBeDefined();
			expect(result.guidance).toBeUndefined();
		});

		it("/axiom-spec is allowed at Level 1", () => {
			const result = handleAboveLevelCommand(
				"/axiom-spec",
				MaturityLevel.Scaffolded,
			);
			expect(result.status).toBe("allowed");
		});

		it("/axiom-plan is allowed at Level 2", () => {
			const result = handleAboveLevelCommand(
				"/axiom-plan",
				MaturityLevel.Specced,
			);
			expect(result.status).toBe("allowed");
		});

		it("/axiom-run is allowed at Level 3", () => {
			const result = handleAboveLevelCommand(
				"/axiom-run",
				MaturityLevel.Active,
			);
			expect(result.status).toBe("allowed");
		});

		it("/axiom-release is allowed at Level 4", () => {
			const result = handleAboveLevelCommand(
				"/axiom-release",
				MaturityLevel.Mature,
			);
			expect(result.status).toBe("allowed");
		});

		it("/axiom-setup is still allowed at Level 4", () => {
			const result = handleAboveLevelCommand(
				"/axiom-setup",
				MaturityLevel.Mature,
			);
			expect(result.status).toBe("allowed");
		});
	});

	describe("returns 'above_level' with conversational guidance for above-level commands", () => {
		it("/axiom-spec at Level 0 returns guidance", () => {
			const result = handleAboveLevelCommand(
				"/axiom-spec",
				MaturityLevel.New,
			);
			expect(result.status).toBe("above_level");
			expect(result.guidance).toBeDefined();
			expect(typeof result.guidance).toBe("string");
			expect(result.guidance?.length).toBeGreaterThan(0);
			expect(result.requiredLevel).toBe(MaturityLevel.Scaffolded);
		});

		it("/axiom-plan at Level 0 returns guidance mentioning specs", () => {
			const result = handleAboveLevelCommand(
				"/axiom-plan",
				MaturityLevel.New,
			);
			expect(result.status).toBe("above_level");
			expect(result.guidance).toContain("/axiom-plan");
			expect(result.guidance).toContain("spec");
		});

		it("/axiom-run at Level 0 returns guidance per REQ-PLG-022 example", () => {
			const result = handleAboveLevelCommand("/axiom-run", MaturityLevel.New);
			expect(result.status).toBe("above_level");
			expect(result.guidance).toContain("/axiom-run");
			expect(result.guidance).toContain("spec");
			expect(result.guidance).toContain("plan");
		});

		it("/axiom-run at Level 1 returns guidance", () => {
			const result = handleAboveLevelCommand(
				"/axiom-run",
				MaturityLevel.Scaffolded,
			);
			expect(result.status).toBe("above_level");
			expect(result.guidance).toBeDefined();
		});

		it("/axiom-run at Level 2 returns guidance", () => {
			const result = handleAboveLevelCommand(
				"/axiom-run",
				MaturityLevel.Specced,
			);
			expect(result.status).toBe("above_level");
			expect(result.guidance).toBeDefined();
		});

		it("/axiom-release at Level 3 returns guidance", () => {
			const result = handleAboveLevelCommand(
				"/axiom-release",
				MaturityLevel.Active,
			);
			expect(result.status).toBe("above_level");
			expect(result.guidance).toContain("/axiom-release");
			expect(result.requiredLevel).toBe(MaturityLevel.Mature);
		});

		it("/axiom-verify at Level 0 returns guidance", () => {
			const result = handleAboveLevelCommand(
				"/axiom-verify",
				MaturityLevel.New,
			);
			expect(result.status).toBe("above_level");
			expect(result.guidance).toBeDefined();
			expect(result.requiredLevel).toBe(MaturityLevel.Active);
		});

		it("/axiom-loop at Level 1 returns guidance", () => {
			const result = handleAboveLevelCommand(
				"/axiom-loop",
				MaturityLevel.Scaffolded,
			);
			expect(result.status).toBe("above_level");
			expect(result.guidance).toBeDefined();
		});

		it("/axiom-share at Level 2 returns guidance", () => {
			const result = handleAboveLevelCommand(
				"/axiom-share",
				MaturityLevel.Specced,
			);
			expect(result.status).toBe("above_level");
			expect(result.guidance).toBeDefined();
		});

		it("/axiom-pulse at Level 0 returns guidance", () => {
			const result = handleAboveLevelCommand(
				"/axiom-pulse",
				MaturityLevel.New,
			);
			expect(result.status).toBe("above_level");
			expect(result.guidance).toBeDefined();
		});

		it("/axiom-spinup at Level 2 returns guidance", () => {
			const result = handleAboveLevelCommand(
				"/axiom-spinup",
				MaturityLevel.Specced,
			);
			expect(result.status).toBe("above_level");
			expect(result.guidance).toBeDefined();
		});
	});

	describe("guidance messages are conversational (not error-like)", () => {
		it("guidance contains a question or offer to help", () => {
			// REQ-PLG-022 example: "Want me to help you create your first spec?"
			const result = handleAboveLevelCommand("/axiom-run", MaturityLevel.New);
			expect(result.guidance).toMatch(/\?/);
		});

		it("guidance mentions the command name", () => {
			const result = handleAboveLevelCommand(
				"/axiom-plan",
				MaturityLevel.New,
			);
			expect(result.guidance).toContain("/axiom-plan");
		});

		it("guidance does not contain error-like language", () => {
			const result = handleAboveLevelCommand("/axiom-run", MaturityLevel.New);
			expect(result.guidance?.toLowerCase()).not.toContain("error");
			expect(result.guidance?.toLowerCase()).not.toContain("forbidden");
			expect(result.guidance?.toLowerCase()).not.toContain("denied");
		});
	});

	describe("returns 'unknown' for unrecognized commands", () => {
		it("unknown command returns status 'unknown'", () => {
			const result = handleAboveLevelCommand(
				"/axiom-nonexistent",
				MaturityLevel.New,
			);
			expect(result.status).toBe("unknown");
			expect(result.guidance).toBeUndefined();
			expect(result.command).toBeUndefined();
		});

		it("empty string returns status 'unknown'", () => {
			const result = handleAboveLevelCommand("", MaturityLevel.New);
			expect(result.status).toBe("unknown");
		});
	});

	describe("/axiom-next at Level 4 returns above_level (maxLevel exceeded)", () => {
		it("/axiom-next at Level 4 is above_level, not allowed", () => {
			const result = handleAboveLevelCommand(
				"/axiom-next",
				MaturityLevel.Mature,
			);
			expect(result.status).toBe("above_level");
			expect(result.guidance).toBeDefined();
		});

		it("/axiom-next at Level 3 is allowed", () => {
			const result = handleAboveLevelCommand(
				"/axiom-next",
				MaturityLevel.Active,
			);
			expect(result.status).toBe("allowed");
		});
	});

	describe("includes command definition and required level in result", () => {
		it("above_level result includes the command definition", () => {
			const result = handleAboveLevelCommand("/axiom-run", MaturityLevel.New);
			expect(result.command).toBeDefined();
			expect(result.command?.name).toBe("/axiom-run");
		});

		it("above_level result includes the required maturity level", () => {
			const result = handleAboveLevelCommand("/axiom-run", MaturityLevel.New);
			expect(result.requiredLevel).toBe(MaturityLevel.Active);
		});

		it("allowed result includes the command definition", () => {
			const result = handleAboveLevelCommand(
				"/axiom-setup",
				MaturityLevel.New,
			);
			expect(result.command).toBeDefined();
			expect(result.command?.name).toBe("/axiom-setup");
		});

		it("allowed result does not include requiredLevel", () => {
			const result = handleAboveLevelCommand(
				"/axiom-setup",
				MaturityLevel.New,
			);
			expect(result.requiredLevel).toBeUndefined();
		});
	});
});
