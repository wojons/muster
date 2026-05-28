/**
 * End-to-end plugin lifecycle proof — verifies that the delivered v1 layers
 * work together in a single lifecycle path:
 *
 *   1. Setup/Bootstrap (L0) — REQ-PLG-011
 *   2. Backend detection (L1) — REQ-PLG-050
 *   3. Metrics surface (L2) — REQ-PLG-100
 *   4. Security control (L5) — REQ-PLG-700, REQ-PLG-800
 *
 * This is NOT a unit test for individual modules — those exist in their
 * respective test files. This test proves the modules compose correctly
 * when exercised through a realistic lifecycle sequence.
 *
 * Spec: specs/70-OpenCode-Plugin.md
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md plan=phase-70-8/task-70-8-1/step-70-8-1-1
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { existsSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
// ── L1: Backend imports ─────────────────────────────────────────────────
import { detectBackendMode } from "../src/backend";
import { createBackendRouter } from "../src/backend-routing";
import { bootstrap } from "../src/bootstrap";
import { getCommandsForLevel } from "../src/commands";
// ── L0: Onboarding imports ──────────────────────────────────────────────
import { detectMaturity, MaturityLevel } from "../src/detect";
// ── Plugin entry point ──────────────────────────────────────────────────
import { CodeOpsPlugin } from "../src/index";
// ── L2: Metrics imports ─────────────────────────────────────────────────
import {
	computeSessionCost,
	loadEmbeddedPricing,
	parseSessionMessages,
} from "../src/layers/metrics";
import {
	evaluateSessionBudget,
	loadSessionBudgetConfig,
} from "../src/layers/metrics-budget";
import {
	buildSessionCostResult,
	formatSessionCostSummary,
} from "../src/layers/metrics-tools";
// ── L5/Security imports ─────────────────────────────────────────────────
import {
	createBlockedFileChecker,
	isBlockedFile,
} from "../src/security/blocked-files";
import {
	buildPrivacyReport,
	formatPrivacyReport,
} from "../src/security/privacy-command";
import {
	createRedactionEngine,
	redactContent,
} from "../src/security/redaction";
import {
	buildStartupAuditLog,
	verifyPackageName,
} from "../src/security/supply-chain";

/** Create a unique temp directory for each test */
function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-e2e-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
	);
	mkdirSync(dir, { recursive: true });
	return dir;
}

/** Create a minimal OpenCode session message file for metrics testing */
function writeSessionMessage(
	baseDir: string,
	sessionId: string,
	filename: string,
	overrides: Record<string, unknown> = {},
): void {
	const sessionDir = join(baseDir, sessionId);
	mkdirSync(sessionDir, { recursive: true });
	const msg = {
		id: "msg_test001",
		sessionID: sessionId,
		role: "assistant",
		parentID: "msg_test000",
		modelID: "claude-3-5-sonnet-20241022",
		providerID: "anthropic",
		tokens: {
			input: 12500,
			output: 850,
			reasoning: 0,
			cache: { read: 1000, write: 500 },
		},
		...overrides,
	};
	writeFileSync(join(sessionDir, filename), JSON.stringify(msg));
}

// ═══════════════════════════════════════════════════════════════════════════
// E2E Lifecycle: Fresh repo → bootstrap → detect → metrics → security
// ═══════════════════════════════════════════════════════════════════════════

describe("E2E Lifecycle: fresh repo through all v1 layers", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	// ── Phase 1: L0 Onboarding ──────────────────────────────────────────

	it("detects Level 0 for a bare directory, then bootstrap raises maturity", async () => {
		// Step 1: Fresh directory is Level 0 (New)
		const before = detectMaturity(tempDir);
		expect(before.level).toBe(MaturityLevel.New);

		// Step 2: Bootstrap creates the scaffold
		const result = await bootstrap(tempDir);
		expect(result.stepsCompleted.length).toBeGreaterThan(0);

		// Step 3: After bootstrap, maturity should be at least Level 1 (Scaffolded)
		const after = detectMaturity(tempDir);
		expect(after.level).toBeGreaterThanOrEqual(MaturityLevel.Scaffolded);
	});

	it("progressive commands expand as maturity increases", () => {
		// Level 0 commands
		const l0Commands = getCommandsForLevel(MaturityLevel.New);
		expect(l0Commands.length).toBeGreaterThan(0);
		const l0Names = l0Commands.map((c) => c.name);
		expect(l0Names).toContain("/axiom-setup");
		expect(l0Names).toContain("/axiom-help");

		// Level 2 commands should include more than Level 0
		const l2Commands = getCommandsForLevel(MaturityLevel.Specced);
		expect(l2Commands.length).toBeGreaterThanOrEqual(l0Commands.length);
	});

	// ── Phase 2: L1 Backend Detection ───────────────────────────────────

	it("detects backend mode for a bootstrapped repo", async () => {
		// Bootstrap first to create the scaffold
		await bootstrap(tempDir);

		// Backend detection should return a valid mode
		const backendResult = detectBackendMode({ repoDir: tempDir });
		expect(backendResult.mode).toBeDefined();
		// A freshly bootstrapped repo with no Python/Go code is spec-only
		expect(["spec-only", "python", "go", "hybrid"]).toContain(
			backendResult.mode,
		);
	});

	it("backend router provides status report after detection", async () => {
		await bootstrap(tempDir);

		const router = createBackendRouter({ repoDir: tempDir });
		const status = router.status();
		expect(status).toBeDefined();
		expect(status.available).toBeDefined();
		expect(Array.isArray(status.available)).toBe(true);
	});

	// ── Phase 3: L2 Metrics ─────────────────────────────────────────────

	it("metrics pipeline: parse messages → compute cost → format summary", () => {
		// Create a temp message directory with a session
		const messageDir = join(tempDir, "messages");
		const sessionId = "ses_e2e_test";

		writeSessionMessage(messageDir, sessionId, "001.json", {
			id: "msg_001",
			modelID: "claude-3-5-sonnet-20241022",
			tokens: {
				input: 12500,
				output: 850,
				reasoning: 0,
				cache: { read: 1000, write: 500 },
			},
		});
		writeSessionMessage(messageDir, sessionId, "002.json", {
			id: "msg_002",
			modelID: "claude-3-5-sonnet-20241022",
			tokens: {
				input: 8000,
				output: 1200,
				reasoning: 0,
				cache: { read: 500, write: 200 },
			},
		});

		// Step 1: Parse session messages from directory
		const parsed = parseSessionMessages(messageDir, sessionId);
		expect(parsed.length).toBe(2);

		// Step 2: Load embedded pricing (no network required per REQ-PLG-104)
		const pricing = loadEmbeddedPricing();
		expect(pricing).toBeDefined();
		expect(pricing.models.length).toBeGreaterThan(0);

		// Step 3: Compute session cost
		const cost = computeSessionCost(parsed, pricing);
		expect(cost.total_cost_usd).toBeGreaterThanOrEqual(0);
		expect(cost.by_model).toBeDefined();

		// Step 4: Build tool result and format summary
		const toolResult = buildSessionCostResult({
			sessionId,
			costResult: cost,
			durationMinutes: 5,
			messageCount: 2,
			toolsUsed: 3,
		});
		expect(toolResult.total_cost_usd).toBeGreaterThanOrEqual(0);
		expect(toolResult.session_id).toBe(sessionId);

		const summary = formatSessionCostSummary(toolResult);
		expect(typeof summary).toBe("string");
		expect(summary.length).toBeGreaterThan(0);
	});

	it("budget evaluation works with default config (no budget set)", () => {
		// Load config from an empty plugin.yaml equivalent
		const config = loadSessionBudgetConfig({});
		expect(config).toBeDefined();
		expect(config.session_budget_usd).toBeNull();

		// Evaluate a small cost against default (no budget) config
		const evaluation = evaluateSessionBudget(0.01, config);
		expect(evaluation).toBeDefined();
		expect(evaluation.status).toBe("ok");
		expect(evaluation.blocked).toBe(false);
	});

	it("budget evaluation warns when cost exceeds threshold", () => {
		const config = loadSessionBudgetConfig({
			metrics: { session_budget_usd: 1.0, warning_threshold_pct: 80 },
		});
		expect(config.session_budget_usd).toBe(1.0);

		// 90% of budget should trigger warning
		const evaluation = evaluateSessionBudget(0.9, config);
		expect(evaluation.status).toBe("warning");
		expect(evaluation.warning).not.toBeNull();
		expect(evaluation.blocked).toBe(false);
	});

	// ── Phase 4: Security Controls ──────────────────────────────────────

	it("REQ-PLG-700: blocked-file enforcement prevents .env reads", () => {
		const result = isBlockedFile(".env");
		expect(result.blocked).toBe(true);
		expect(result.reason).toBeDefined();

		// Non-sensitive files should pass
		const safe = isBlockedFile("README.md");
		expect(safe.blocked).toBe(false);
	});

	it("REQ-PLG-701: redaction engine strips secrets from content", () => {
		// Use a pattern that matches the built-in AWS key regex
		const content =
			"My key is AKIAIOSFODNN7EXAMPLE and secret is wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY";
		const result = redactContent(content);
		expect(result.count).toBeGreaterThan(0);
		expect(result.redacted).not.toContain("AKIAIOSFODNN7EXAMPLE");
	});

	it("REQ-PLG-715: privacy report includes required data categories", () => {
		const report = buildPrivacyReport({ repoDir: tempDir });
		expect(report).toBeDefined();
		expect(report.dataCategories).toBeDefined();
		expect(Array.isArray(report.dataCategories)).toBe(true);
		expect(report.dataCategories.length).toBeGreaterThan(0);

		const formatted = formatPrivacyReport(report);
		expect(typeof formatted).toBe("string");
		expect(formatted.length).toBeGreaterThan(0);
	});

	it("REQ-PLG-800: supply-chain package name verification", () => {
		// Correct package name loaded from expected path should pass
		const correct = verifyPackageName({
			loadedFrom: "/path/to/node_modules/opencode-axiom/dist/index.js",
		});
		expect(correct.isExpectedPackage).toBe(true);

		// Typosquat loaded from wrong package should fail
		const typo = verifyPackageName({
			loadedFrom: "/path/to/node_modules/opencode-codops/dist/index.js",
		});
		expect(typo.isExpectedPackage).toBe(false);
		expect(typo.warning).toBeDefined();
	});

	it("REQ-PLG-802: startup audit log captures version and path", () => {
		const auditLog = buildStartupAuditLog({
			version: "0.1.0",
			sourcePath: "/path/to/plugin",
		});
		expect(auditLog).toBeDefined();
		expect(auditLog.version).toBe("0.1.0");
		expect(auditLog.sourcePath).toBe("/path/to/plugin");
		expect(auditLog.timestamp).toBeDefined();
		expect(auditLog.message).toContain("v0.1.0");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// E2E: Plugin entry point integration
// ═══════════════════════════════════════════════════════════════════════════

describe("E2E: CodeOpsPlugin entry point integration", () => {
	let tempDir: string;

	beforeEach(async () => {
		tempDir = makeTempDir();
		// Bootstrap the repo so the plugin has something to work with
		await bootstrap(tempDir);
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("CodeOpsPlugin initializes and returns expected hook/tool surface", async () => {
		const plugin = await CodeOpsPlugin({ directory: tempDir });
		expect(plugin).toBeDefined();

		// Should have event hook (REQ-PLG-030)
		expect(plugin.event).toBeDefined();
		expect(typeof plugin.event).toBe("function");

		// Should have copilot system transform (REQ-PLG-013)
		expect(plugin["experimental.chat.system.transform"]).toBeDefined();

		// Should have compaction hook (REQ-PLG-031)
		expect(plugin["experimental.session.compacting"]).toBeDefined();

		// Should have shell env hook (REQ-PLG-036)
		expect(plugin["shell.env"]).toBeDefined();

		// Should have tool.execute.before for blocked files (REQ-PLG-700)
		expect(plugin["tool.execute.before"]).toBeDefined();
		expect(typeof plugin["tool.execute.before"]).toBe("function");

		// Should have tool registrations
		expect(plugin.tool).toBeDefined();
		expect(plugin.tool?.codeops_detect).toBeDefined();
		expect(plugin.tool?.codeops_backend).toBeDefined();
		expect(plugin.tool?.codeops_privacy).toBeDefined();
	});

	it("codeops_detect tool returns valid JSON with maturity info", async () => {
		const plugin = await CodeOpsPlugin({ directory: tempDir });
		const detectTool = plugin.tool?.codeops_detect;

		// Execute the tool
		// biome-ignore lint/suspicious/noExplicitAny: intentional test mock for tool context
		const result = await detectTool.execute({}, {} as any);
		expect(typeof result).toBe("string");

		const parsed = JSON.parse(result as string);
		expect(parsed.level).toBeDefined();
		expect(typeof parsed.level).toBe("number");
		expect(parsed.backends).toBeDefined();
	});

	it("codeops_backend tool returns status report", async () => {
		const plugin = await CodeOpsPlugin({ directory: tempDir });
		const backendTool = plugin.tool?.codeops_backend;

		// biome-ignore lint/suspicious/noExplicitAny: intentional test mock for tool context
		const result = await backendTool.execute({}, {} as any);
		expect(typeof result).toBe("string");

		const parsed = JSON.parse(result as string);
		expect(parsed.available).toBeDefined();
	});

	it("codeops_privacy tool returns formatted privacy report", async () => {
		const plugin = await CodeOpsPlugin({ directory: tempDir });
		const privacyTool = plugin.tool?.codeops_privacy;

		// biome-ignore lint/suspicious/noExplicitAny: intentional test mock for tool context
		const result = await privacyTool.execute({}, {} as any);
		expect(typeof result).toBe("string");
		expect((result as string).length).toBeGreaterThan(0);
	});

	it("tool.execute.before blocks .env file access through plugin surface", async () => {
		const plugin = await CodeOpsPlugin({ directory: tempDir });
		const beforeHook = plugin["tool.execute.before"] as (
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			input: any,
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			output: any,
		) => Promise<void>;

		// Simulate a tool trying to read .env
		await expect(
			beforeHook({}, { args: { filePath: ".env" } }),
		).rejects.toThrow(/blocked by REQ-PLG-700/);

		// Safe files should pass through without throwing
		await beforeHook({}, { args: { filePath: "README.md" } });
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// E2E: Cross-layer composition
// ═══════════════════════════════════════════════════════════════════════════

describe("E2E: cross-layer composition", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("full lifecycle: bare dir → bootstrap → detect → backend → metrics → security check", async () => {
		// 1. Start with bare directory (Level 0)
		const initialMaturity = detectMaturity(tempDir);
		expect(initialMaturity.level).toBe(MaturityLevel.New);

		// 2. Bootstrap the repo
		const bootstrapResult = await bootstrap(tempDir);
		expect(bootstrapResult.stepsCompleted.length).toBeGreaterThan(0);

		// 3. Verify maturity increased
		const postBootstrap = detectMaturity(tempDir);
		expect(postBootstrap.level).toBeGreaterThanOrEqual(
			MaturityLevel.Scaffolded,
		);

		// 4. Detect backend mode
		const backend = detectBackendMode({ repoDir: tempDir });
		expect(backend.mode).toBeDefined();

		// 5. Create a backend router and get status
		const router = createBackendRouter({ repoDir: tempDir });
		const status = router.status();
		expect(status.available).toBeDefined();

		// 6. Run metrics on fixture session data
		const messageDir = join(tempDir, ".opencode", "sessions");
		const sessionId = "ses_e2e_lifecycle";
		writeSessionMessage(messageDir, sessionId, "001.json");
		const parsed = parseSessionMessages(messageDir, sessionId);
		const pricing = loadEmbeddedPricing();
		const cost = computeSessionCost(parsed, pricing);
		expect(cost.total_cost_usd).toBeGreaterThanOrEqual(0);

		// 7. Verify security controls are active
		const envBlocked = isBlockedFile(".env");
		expect(envBlocked.blocked).toBe(true);

		const redacted = redactContent("secret key: AKIAIOSFODNN7EXAMPLE");
		expect(redacted.count).toBeGreaterThan(0);

		// 8. Privacy report works for the bootstrapped repo
		const privacy = buildPrivacyReport({ repoDir: tempDir });
		expect(privacy.dataCategories.length).toBeGreaterThan(0);

		// 9. Supply-chain verification
		const pkgVerify = verifyPackageName({
			loadedFrom: "/path/to/node_modules/opencode-axiom/dist/index.js",
		});
		expect(pkgVerify.isExpectedPackage).toBe(true);
	});

	it("blocked-file checker with custom patterns composes with redaction engine", () => {
		// Custom blocked patterns
		const checker = createBlockedFileChecker({
			additionalPatterns: ["*.secret.yaml"],
		});

		// Custom pattern blocks
		const customBlocked = checker.check("config.secret.yaml");
		expect(customBlocked.blocked).toBe(true);

		// Default patterns still work
		const envBlocked = checker.check(".env.production");
		expect(envBlocked.blocked).toBe(true);

		// Redaction engine works independently
		const engine = createRedactionEngine();
		const result = engine.redact(
			"token: AKIAIOSFODNN7EXAMPLE secret access key",
		);
		expect(result.count).toBeGreaterThan(0);
		expect(result.redacted).not.toContain("AKIAIOSFODNN7EXAMPLE");
	});
});
