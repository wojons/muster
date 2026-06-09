/**
 * Session persistence and retention contract tests — verifies that session
 * analytics are persisted to `.axiom/sessions/<session-id>.json` on session
 * end, that a 90-day / 1000-file retention policy is enforced on session start,
 * and that `/axiom-setup` adds `.axiom/sessions/` to `.gitignore`.
 *
 * This is a TEST-FIRST contract: the tests import from
 * ../src/layers/session-persistence which does not exist yet. All tests are
 * expected to fail at the module-resolution level until the persistence layer
 * is implemented.
 *
 * REQ-PLG-106: Session analytics MUST be persistable to
 *   `.axiom/sessions/<session-id>.json` when the session ends. This file
 *   follows the schema in REQ-PLG-102 with additional fields.
 * REQ-PLG-106a: The `.axiom/sessions/` directory MUST have a local retention
 *   policy: files older than 90 days or exceeding 1000 files (whichever is
 *   reached first) MUST be automatically cleaned up on session start. The
 *   `/axiom-setup` command MUST add `.axiom/sessions/` to `.gitignore` to
 *   prevent repo bloat.
 *
 * Out of scope for this slice:
 * - REQ-PLG-107: codeops_session_stats tool (repo-history aggregation)
 * - REQ-PLG-113: Per-repo cost budgets with session history aggregation
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-106, specs/70-OpenCode-Plugin.md#REQ-PLG-106a
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-106 plan=phase-70-3/task-70-3-4/step-70-3-4-1
 */
import { describe, expect, it } from "bun:test";
import type { SessionCostToolResult } from "../src/layers/metrics-tools";
// Import from the not-yet-existing session-persistence module — this WILL fail
// at module resolution, which is the expected test-first signal.
import {
	enforceRetentionPolicy,
	ensureSessionsGitignore,
	type GitignoreResult,
	MAX_SESSION_AGE_DAYS,
	MAX_SESSION_FILES,
	persistSessionAnalytics,
	type RetentionResult,
	SESSION_DIR_NAME,
	type SessionAnalyticsFile,
} from "../src/layers/session-persistence";

// ---------------------------------------------------------------------------
// Fixture helpers
// ---------------------------------------------------------------------------

/** Build a minimal SessionCostToolResult for persistence testing. */
function makeToolResult(
	overrides: Partial<SessionCostToolResult> = {},
): SessionCostToolResult {
	return {
		session_id: "test-session-abc",
		total_cost_usd: 0.42,
		total_tokens: { prompt: 12000, completion: 3000 },
		by_model: [
			{ model: "claude-sonnet-4-20250514", cost_usd: 0.38, calls: 12 },
			{ model: "claude-haiku-4-20250414", cost_usd: 0.04, calls: 3 },
		],
		duration_minutes: 14.2,
		messages: 15,
		tools_used: ["bash", "read", "write", "edit"],
		...overrides,
	};
}

// ===========================================================================
// REQ-PLG-106: Session persistence to .axiom/sessions/<session-id>.json
// ===========================================================================

describe("REQ-PLG-106: persistSessionAnalytics", () => {
	it("returns a SessionAnalyticsFile with the spec-defined schema", () => {
		const toolResult = makeToolResult();
		const result: SessionAnalyticsFile = persistSessionAnalytics({
			repoDir: "/tmp/test-repo",
			toolResult,
			startedAt: new Date("2026-03-20T01:00:00Z"),
			endedAt: new Date("2026-03-20T01:14:12Z"),
		});

		// REQ-PLG-106: follows the schema in REQ-PLG-102 with additional fields
		expect(result.session_id).toBe("test-session-abc");
		expect(result.total_cost_usd).toBeCloseTo(0.42, 2);
		expect(result.total_tokens).toBeDefined();
		expect(result.total_tokens.prompt).toBe(12000);
		expect(result.total_tokens.completion).toBe(3000);
		expect(Array.isArray(result.by_model)).toBe(true);
		expect(result.duration_minutes).toBeCloseTo(14.2, 1);
		expect(result.messages).toBe(15);
		expect(Array.isArray(result.tools_used)).toBe(true);

		// Additional fields beyond REQ-PLG-102
		expect(typeof result.started_at).toBe("string");
		expect(typeof result.ended_at).toBe("string");
		expect(result.started_at).toBe("2026-03-20T01:00:00.000Z");
		expect(result.ended_at).toBe("2026-03-20T01:14:12.000Z");
	});

	it("writes the file to .axiom/sessions/<session-id>.json", () => {
		const toolResult = makeToolResult({ session_id: "sess-xyz-789" });
		const result = persistSessionAnalytics({
			repoDir: "/tmp/test-repo",
			toolResult,
			startedAt: new Date("2026-03-20T01:00:00Z"),
			endedAt: new Date("2026-03-20T01:10:00Z"),
		});

		// The output path must follow the spec pattern
		expect(result.file_path).toBe(
			"/tmp/test-repo/.axiom/sessions/sess-xyz-789.json",
		);
	});

	it("creates the .axiom/sessions/ directory if it does not exist", () => {
		const toolResult = makeToolResult();
		const result = persistSessionAnalytics({
			repoDir: "/tmp/nonexistent-repo",
			toolResult,
			startedAt: new Date(),
			endedAt: new Date(),
			dryRun: true,
		});

		// In dry-run mode, the function should still compute the correct path
		// and indicate that the directory would be created
		expect(result.file_path).toContain(".axiom/sessions/");
		expect(result.dry_run).toBe(true);
	});

	it("includes maturity_level when provided", () => {
		const toolResult = makeToolResult();
		const result = persistSessionAnalytics({
			repoDir: "/tmp/test-repo",
			toolResult,
			startedAt: new Date(),
			endedAt: new Date(),
			maturityLevel: 3,
		});

		expect(result.maturity_level).toBe(3);
	});

	it("handles sessions with zero cost", () => {
		const toolResult = makeToolResult({
			session_id: "zero-cost-session",
			total_cost_usd: 0,
			total_tokens: { prompt: 0, completion: 0 },
			by_model: [],
			duration_minutes: 0,
			messages: 0,
			tools_used: [],
		});

		const result = persistSessionAnalytics({
			repoDir: "/tmp/test-repo",
			toolResult,
			startedAt: new Date(),
			endedAt: new Date(),
		});

		expect(result.session_id).toBe("zero-cost-session");
		expect(result.total_cost_usd).toBe(0);
		expect(result.file_path).toContain("zero-cost-session.json");
	});

	it("sanitizes session IDs to prevent path traversal", () => {
		const toolResult = makeToolResult({ session_id: "../../../etc/passwd" });
		const result = persistSessionAnalytics({
			repoDir: "/tmp/test-repo",
			toolResult,
			startedAt: new Date(),
			endedAt: new Date(),
		});

		// The file_path must stay within .axiom/sessions/
		expect(result.file_path).not.toContain("../");
		expect(result.file_path).toContain(".axiom/sessions/");
	});
});

// ===========================================================================
// REQ-PLG-106a: Retention policy — 90 days / 1000 files
// ===========================================================================

describe("REQ-PLG-106a: enforceRetentionPolicy", () => {
	it("exports the correct retention constants", () => {
		// Spec: files older than 90 days or exceeding 1000 files
		expect(MAX_SESSION_AGE_DAYS).toBe(90);
		expect(MAX_SESSION_FILES).toBe(1000);
		expect(SESSION_DIR_NAME).toBe("sessions");
	});

	it("returns a RetentionResult with files_removed count", () => {
		const result: RetentionResult = enforceRetentionPolicy({
			sessionsDir: "/tmp/test-repo/.axiom/sessions",
			dryRun: true,
		});

		expect(typeof result.files_removed).toBe("number");
		expect(typeof result.files_remaining).toBe("number");
		expect(typeof result.reason).toBe("string");
		expect(result.files_removed).toBeGreaterThanOrEqual(0);
	});

	it("removes files older than 90 days", () => {
		// This test encodes the age-based cleanup contract.
		// The implementation must stat each file and remove those with
		// mtime older than 90 days from now.
		const result = enforceRetentionPolicy({
			sessionsDir: "/tmp/test-repo/.axiom/sessions",
			dryRun: true,
			// Simulate: inject a list of files with known ages for testing
			_testFiles: [
				{ name: "old-session.json", ageDays: 100 },
				{ name: "recent-session.json", ageDays: 10 },
				{ name: "borderline-session.json", ageDays: 90 },
			],
		});

		// old-session.json (100 days) should be removed
		// borderline-session.json (exactly 90 days) should be kept (> 90, not >=)
		// recent-session.json (10 days) should be kept
		expect(result.files_removed).toBe(1);
		expect(result.files_remaining).toBe(2);
		expect(result.removed_files).toContain("old-session.json");
		expect(result.removed_files).not.toContain("recent-session.json");
		expect(result.removed_files).not.toContain("borderline-session.json");
	});

	it("removes oldest files when count exceeds 1000", () => {
		// This test encodes the count-based cleanup contract.
		// When more than 1000 files exist, the oldest files are removed
		// until exactly 1000 remain.
		const testFiles = Array.from({ length: 1005 }, (_, i) => ({
			name: `session-${String(i).padStart(4, "0")}.json`,
			ageDays: Math.floor((i * 89) / 1004), // 0 to 89 — all under 90 days; session-0000 is newest, session-1004 is oldest
		}));

		const result = enforceRetentionPolicy({
			sessionsDir: "/tmp/test-repo/.axiom/sessions",
			dryRun: true,
			_testFiles: testFiles,
		});

		// 1005 files → remove 5 oldest → 1000 remaining
		expect(result.files_removed).toBe(5);
		expect(result.files_remaining).toBe(1000);
	});

	it("applies both age and count policies (whichever removes more)", () => {
		// If 500 files exist but 200 are older than 90 days, remove the 200
		// even though count is under 1000.
		const testFiles = [
			...Array.from({ length: 200 }, (_, i) => ({
				name: `old-${i}.json`,
				ageDays: 91 + i,
			})),
			...Array.from({ length: 300 }, (_, i) => ({
				name: `recent-${i}.json`,
				ageDays: Math.floor((i * 89) / 299), // 0 to 89 — all under 90 days
			})),
		];

		const result = enforceRetentionPolicy({
			sessionsDir: "/tmp/test-repo/.axiom/sessions",
			dryRun: true,
			_testFiles: testFiles,
		});

		expect(result.files_removed).toBe(200);
		expect(result.files_remaining).toBe(300);
	});

	it("does nothing when directory is empty", () => {
		const result = enforceRetentionPolicy({
			sessionsDir: "/tmp/test-repo/.axiom/sessions",
			dryRun: true,
			_testFiles: [],
		});

		expect(result.files_removed).toBe(0);
		expect(result.files_remaining).toBe(0);
	});

	it("does nothing when directory does not exist", () => {
		const result = enforceRetentionPolicy({
			sessionsDir: "/tmp/nonexistent/.axiom/sessions",
			dryRun: true,
		});

		expect(result.files_removed).toBe(0);
		expect(result.reason).toContain("not found");
	});

	it("only processes .json files (ignores other file types)", () => {
		const testFiles = [
			{ name: "session-1.json", ageDays: 100 },
			{ name: "README.md", ageDays: 100 },
			{ name: ".gitkeep", ageDays: 100 },
			{ name: "session-2.json", ageDays: 5 },
		];

		const result = enforceRetentionPolicy({
			sessionsDir: "/tmp/test-repo/.axiom/sessions",
			dryRun: true,
			_testFiles: testFiles,
		});

		// Only session-1.json should be removed (old + .json)
		// README.md and .gitkeep are not .json files
		expect(result.files_removed).toBe(1);
		expect(result.removed_files).toContain("session-1.json");
		expect(result.removed_files).not.toContain("README.md");
	});
});

// ===========================================================================
// REQ-PLG-106a: .gitignore handling for .axiom/sessions/
// ===========================================================================

describe("REQ-PLG-106a: ensureSessionsGitignore", () => {
	it("returns a GitignoreResult indicating the action taken", () => {
		const result: GitignoreResult = ensureSessionsGitignore({
			repoDir: "/tmp/test-repo",
			dryRun: true,
		});

		expect(typeof result.action).toBe("string");
		// action should be one of: "added", "already_present", "created", "skipped"
		expect(["added", "already_present", "created", "skipped"]).toContain(
			result.action,
		);
		expect(typeof result.gitignore_path).toBe("string");
	});

	it("adds .axiom/sessions/ to existing .gitignore", () => {
		const result = ensureSessionsGitignore({
			repoDir: "/tmp/test-repo",
			dryRun: true,
			_existingGitignore: "node_modules/\ndist/\n",
		});

		// Should indicate the entry was added
		expect(result.action).toBe("added");
		expect(result.entry).toBe(".axiom/sessions/");
	});

	it("reports already_present when entry exists", () => {
		const result = ensureSessionsGitignore({
			repoDir: "/tmp/test-repo",
			dryRun: true,
			_existingGitignore: "node_modules/\n.axiom/sessions/\ndist/\n",
		});

		expect(result.action).toBe("already_present");
	});

	it("creates .gitignore with the entry when file does not exist", () => {
		const result = ensureSessionsGitignore({
			repoDir: "/tmp/test-repo",
			dryRun: true,
			_existingGitignore: null, // file does not exist
		});

		expect(result.action).toBe("created");
		expect(result.entry).toBe(".axiom/sessions/");
	});

	it("handles .gitignore with trailing newline correctly", () => {
		const result = ensureSessionsGitignore({
			repoDir: "/tmp/test-repo",
			dryRun: true,
			_existingGitignore: "node_modules/\ndist/\n",
		});

		// Should not produce double newlines
		expect(result.action).toBe("added");
	});

	it("handles .gitignore without trailing newline", () => {
		const result = ensureSessionsGitignore({
			repoDir: "/tmp/test-repo",
			dryRun: true,
			_existingGitignore: "node_modules/\ndist/",
		});

		expect(result.action).toBe("added");
	});

	it("matches the entry with or without trailing slash", () => {
		// .axiom/sessions and .axiom/sessions/ should both be recognized
		const result = ensureSessionsGitignore({
			repoDir: "/tmp/test-repo",
			dryRun: true,
			_existingGitignore: ".axiom/sessions\n",
		});

		expect(result.action).toBe("already_present");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-104a: session_identity in all analytics events
// ═══════════════════════════════════════════════════════════════════════════

describe("REQ-PLG-104a: session_identity in analytics events", () => {
	it("includes session_identity when AXIOM_SESSION_IDENTITY env var is set", () => {
		const original = process.env.AXIOM_SESSION_IDENTITY;
		const originalSource = process.env.AXIOM_SESSION_IDENTITY_SOURCE;
		try {
			process.env.AXIOM_SESSION_IDENTITY = "github.com/myorg/myrepo";
			process.env.AXIOM_SESSION_IDENTITY_SOURCE = "git";
			const toolResult = makeToolResult({
				session_id: "identity-test-session",
			});
			const result = persistSessionAnalytics({
				repoDir: "/tmp/test-repo",
				toolResult,
				startedAt: new Date(),
				endedAt: new Date(),
				dryRun: true,
			});
			expect(result.session_identity).toBe("github.com/myorg/myrepo");
			expect(result.session_identity_source).toBe("git");
		} finally {
			if (original === undefined) delete process.env.AXIOM_SESSION_IDENTITY;
			else process.env.AXIOM_SESSION_IDENTITY = original;
			if (originalSource === undefined)
				delete process.env.AXIOM_SESSION_IDENTITY_SOURCE;
			else process.env.AXIOM_SESSION_IDENTITY_SOURCE = originalSource;
		}
	});

	it("session_identity is absent when env var is not set", () => {
		const original = process.env.AXIOM_SESSION_IDENTITY;
		try {
			delete process.env.AXIOM_SESSION_IDENTITY;
			const toolResult = makeToolResult({ session_id: "no-identity-session" });
			const result = persistSessionAnalytics({
				repoDir: "/tmp/test-repo",
				toolResult,
				startedAt: new Date(),
				endedAt: new Date(),
				dryRun: true,
			});
			expect(result.session_identity).toBeUndefined();
		} finally {
			if (original !== undefined)
				process.env.AXIOM_SESSION_IDENTITY = original;
		}
	});

	it("SessionCostToolResult interface includes session_identity field", () => {
		// Type-level check: the field must exist in the interface
		const result: import("../src/layers/metrics-tools").SessionCostToolResult =
			{
				session_id: "type-check",
				total_cost_usd: 0,
				total_tokens: { prompt: 0, completion: 0 },
				by_model: [],
				duration_minutes: 0,
				messages: 0,
				tools_used: [],
				session_identity: "github.com/org/repo",
				session_identity_source: "git",
			};
		expect(result.session_identity).toBe("github.com/org/repo");
		expect(result.session_identity_source).toBe("git");
	});
});
