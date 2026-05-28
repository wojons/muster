/**
 * Security blocked-file pattern contract tests — verifies that the
 * `tool.execute.before` hook prevents reading secrets files matching
 * the REQ-PLG-700 blocked patterns.
 *
 * This is a TEST-FIRST contract: the tests import from
 * ../src/security/blocked-files which does not exist yet. All tests are
 * expected to fail at the module-resolution level until the blocked-file
 * enforcement is implemented.
 *
 * REQ-PLG-700: The plugin MUST prevent reading secrets files via
 *   `tool.execute.before` hook. Blocked patterns:
 *   - `.env`, `.env.*`
 *   - `credentials*`, `*credential*`
 *   - `*secret*`, `*token*`, `*key*` (in filenames, not content)
 *   - `*.pem`, `*.p12`, `*.pfx`
 *   - Custom patterns from `.axiom/plugin.yaml` `security.blocked_patterns`
 *
 * Out of scope for this slice:
 * - REQ-PLG-701/702/703: Redaction engine (task-70-5-1, already done)
 * - REQ-PLG-720/721: First-run trust prompt (task-70-5-3)
 * - REQ-PLG-715: Privacy command (task-70-5-4)
 * - Phase 70.4 config automation (deferred per spec section 1.2)
 * - Implementation of src/security/blocked-files.ts (step-70-5-2-2)
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-700
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-700 plan=phase-70-5/task-70-5-2/step-70-5-2-1
 */
import { describe, expect, it } from "bun:test";

// Import from the not-yet-existing security/blocked-files module — this WILL
// fail at module resolution, which is the expected test-first signal.
import {
	type BlockedFileConfig,
	createBlockedFileChecker,
	DEFAULT_BLOCKED_PATTERNS,
	isBlockedFile,
} from "../src/security/blocked-files";

// ===========================================================================
// REQ-PLG-700: .env and .env.* patterns
// ===========================================================================

describe("REQ-PLG-700: .env file blocking", () => {
	it("blocks .env at repo root", () => {
		const result = isBlockedFile(".env");
		expect(result.blocked).toBe(true);
		expect(result.pattern).toBeDefined();
	});

	it("blocks .env.local", () => {
		const result = isBlockedFile(".env.local");
		expect(result.blocked).toBe(true);
	});

	it("blocks .env.production", () => {
		const result = isBlockedFile(".env.production");
		expect(result.blocked).toBe(true);
	});

	it("blocks nested .env files (e.g. config/.env)", () => {
		const result = isBlockedFile("config/.env");
		expect(result.blocked).toBe(true);
	});

	it("blocks nested .env.* files (e.g. deploy/.env.staging)", () => {
		const result = isBlockedFile("deploy/.env.staging");
		expect(result.blocked).toBe(true);
	});

	it("does NOT block files that merely contain 'env' (e.g. environment.ts)", () => {
		const result = isBlockedFile("src/environment.ts");
		expect(result.blocked).toBe(false);
	});
});

// ===========================================================================
// REQ-PLG-700: credentials* and *credential* patterns
// ===========================================================================

describe("REQ-PLG-700: credential file blocking", () => {
	it("blocks credentials.json", () => {
		const result = isBlockedFile("credentials.json");
		expect(result.blocked).toBe(true);
	});

	it("blocks credentials.yaml", () => {
		const result = isBlockedFile("credentials.yaml");
		expect(result.blocked).toBe(true);
	});

	it("blocks my-credential-file.txt", () => {
		const result = isBlockedFile("my-credential-file.txt");
		expect(result.blocked).toBe(true);
	});

	it("blocks gcp-credentials.json (nested)", () => {
		const result = isBlockedFile("config/gcp-credentials.json");
		expect(result.blocked).toBe(true);
	});
});

// ===========================================================================
// REQ-PLG-700: *secret*, *token*, *key* filename patterns
// ===========================================================================

describe("REQ-PLG-700: secret/token/key filename blocking", () => {
	it("blocks secret.yaml", () => {
		const result = isBlockedFile("secret.yaml");
		expect(result.blocked).toBe(true);
	});

	it("blocks my-secret-config.json", () => {
		const result = isBlockedFile("my-secret-config.json");
		expect(result.blocked).toBe(true);
	});

	it("blocks api-token.txt", () => {
		const result = isBlockedFile("api-token.txt");
		expect(result.blocked).toBe(true);
	});

	it("blocks github-token", () => {
		const result = isBlockedFile("github-token");
		expect(result.blocked).toBe(true);
	});

	it("blocks service-key.json", () => {
		const result = isBlockedFile("service-key.json");
		expect(result.blocked).toBe(true);
	});

	it("blocks deploy/master-key.enc", () => {
		const result = isBlockedFile("deploy/master-key.enc");
		expect(result.blocked).toBe(true);
	});
});

// ===========================================================================
// REQ-PLG-700: *.pem, *.p12, *.pfx certificate patterns
// ===========================================================================

describe("REQ-PLG-700: certificate file blocking", () => {
	it("blocks server.pem", () => {
		const result = isBlockedFile("server.pem");
		expect(result.blocked).toBe(true);
	});

	it("blocks certs/ca-bundle.pem", () => {
		const result = isBlockedFile("certs/ca-bundle.pem");
		expect(result.blocked).toBe(true);
	});

	it("blocks keystore.p12", () => {
		const result = isBlockedFile("keystore.p12");
		expect(result.blocked).toBe(true);
	});

	it("blocks identity.pfx", () => {
		const result = isBlockedFile("identity.pfx");
		expect(result.blocked).toBe(true);
	});
});

// ===========================================================================
// REQ-PLG-700: Custom patterns from security.blocked_patterns
// ===========================================================================

describe("REQ-PLG-700: custom blocked_patterns from plugin.yaml", () => {
	it("blocks files matching a custom glob pattern", () => {
		const config: BlockedFileConfig = {
			customPatterns: ["*.vault", "internal-*"],
		};
		const checker = createBlockedFileChecker(config);
		const result = checker.check("secrets.vault");
		expect(result.blocked).toBe(true);
		expect(result.pattern).toBeDefined();
	});

	it("blocks files matching a custom prefix pattern", () => {
		const config: BlockedFileConfig = {
			customPatterns: ["internal-*"],
		};
		const checker = createBlockedFileChecker(config);
		const result = checker.check("internal-config.yaml");
		expect(result.blocked).toBe(true);
	});

	it("does NOT block files that do not match any custom pattern", () => {
		const config: BlockedFileConfig = {
			customPatterns: ["*.vault"],
		};
		const checker = createBlockedFileChecker(config);
		const result = checker.check("README.md");
		expect(result.blocked).toBe(false);
	});

	it("combines default patterns with custom patterns", () => {
		const config: BlockedFileConfig = {
			customPatterns: ["*.vault"],
		};
		const checker = createBlockedFileChecker(config);
		// Default pattern should still block .env
		expect(checker.check(".env").blocked).toBe(true);
		// Custom pattern should also block .vault
		expect(checker.check("data.vault").blocked).toBe(true);
	});
});

// ===========================================================================
// REQ-PLG-700: Default patterns constant
// ===========================================================================

describe("REQ-PLG-700: DEFAULT_BLOCKED_PATTERNS", () => {
	it("exports a non-empty array of default blocked patterns", () => {
		expect(Array.isArray(DEFAULT_BLOCKED_PATTERNS)).toBe(true);
		expect(DEFAULT_BLOCKED_PATTERNS.length).toBeGreaterThan(0);
	});

	it("includes .env pattern", () => {
		expect(
			DEFAULT_BLOCKED_PATTERNS.some((p: string) => p.includes(".env")),
		).toBe(true);
	});

	it("includes credential pattern", () => {
		expect(
			DEFAULT_BLOCKED_PATTERNS.some((p: string) => p.includes("credential")),
		).toBe(true);
	});

	it("includes .pem pattern", () => {
		expect(
			DEFAULT_BLOCKED_PATTERNS.some((p: string) => p.includes(".pem")),
		).toBe(true);
	});
});

// ===========================================================================
// REQ-PLG-700: BlockedFileResult shape
// ===========================================================================

describe("REQ-PLG-700: BlockedFileResult shape", () => {
	it("returns { blocked: true, pattern, reason } for blocked files", () => {
		const result = isBlockedFile(".env");
		expect(result.blocked).toBe(true);
		expect(typeof result.pattern).toBe("string");
		expect(typeof result.reason).toBe("string");
	});

	it("returns { blocked: false } for allowed files", () => {
		const result = isBlockedFile("src/main.ts");
		expect(result.blocked).toBe(false);
	});
});

// ===========================================================================
// REQ-PLG-700: Safe files are NOT blocked
// ===========================================================================

describe("REQ-PLG-700: safe files pass through", () => {
	it("allows README.md", () => {
		expect(isBlockedFile("README.md").blocked).toBe(false);
	});

	it("allows src/index.ts", () => {
		expect(isBlockedFile("src/index.ts").blocked).toBe(false);
	});

	it("allows package.json", () => {
		expect(isBlockedFile("package.json").blocked).toBe(false);
	});

	it("allows tsconfig.json", () => {
		expect(isBlockedFile("tsconfig.json").blocked).toBe(false);
	});

	it("allows .gitignore", () => {
		expect(isBlockedFile(".gitignore").blocked).toBe(false);
	});
});

// ===========================================================================
// REQ-PLG-700: Plugin-surface integration — tool.execute.before hook
// ===========================================================================

import { CodeOpsPlugin } from "../src/index";

describe("REQ-PLG-700: CodeOpsPlugin tool.execute.before blocks secret files", () => {
	it("registers a tool.execute.before hook", async () => {
		// Create a minimal plugin input with a temp directory
		const tmpDir = await import("node:os").then((os) => os.tmpdir());
		const hooks = await CodeOpsPlugin({
			directory: tmpDir,
			worktree: tmpDir,
			serverUrl: new URL("http://localhost:4096"),
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			client: {} as any,
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			project: {} as any,
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			$: {} as any,
		});
		expect(hooks["tool.execute.before"]).toBeDefined();
		expect(typeof hooks["tool.execute.before"]).toBe("function");
	});

	it("blocks .env file reads through tool.execute.before", async () => {
		const tmpDir = await import("node:os").then((os) => os.tmpdir());
		const hooks = await CodeOpsPlugin({
			directory: tmpDir,
			worktree: tmpDir,
			serverUrl: new URL("http://localhost:4096"),
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			client: {} as any,
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			project: {} as any,
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			$: {} as any,
		});
		const hook = hooks["tool.execute.before"] as NonNullable<
			(typeof hooks)["tool.execute.before"]
		>;
		const output = { args: { filePath: ".env" } };
		// The hook should throw or modify args to prevent the read
		let blocked = false;
		try {
			await hook(
				{ tool: "read", sessionID: "test-session", callID: "call-1" },
				output,
			);
			// If the hook set args to signal blocking (e.g., empty or error marker)
			if (output.args?.__blocked || output.args?.filePath === "") {
				blocked = true;
			}
		} catch (e: unknown) {
			// Hook threw to prevent execution — this is the expected blocking path
			blocked = true;
			expect((e as Error).message).toContain("blocked");
		}
		expect(blocked).toBe(true);
	});

	it("blocks *.pem file reads through tool.execute.before", async () => {
		const tmpDir = await import("node:os").then((os) => os.tmpdir());
		const hooks = await CodeOpsPlugin({
			directory: tmpDir,
			worktree: tmpDir,
			serverUrl: new URL("http://localhost:4096"),
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			client: {} as any,
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			project: {} as any,
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			$: {} as any,
		});
		const hook = hooks["tool.execute.before"] as NonNullable<
			(typeof hooks)["tool.execute.before"]
		>;
		const output = { args: { filePath: "certs/server.pem" } };
		let blocked = false;
		try {
			await hook(
				{ tool: "read", sessionID: "test-session", callID: "call-2" },
				output,
			);
			if (output.args?.__blocked || output.args?.filePath === "") {
				blocked = true;
			}
		} catch (e: unknown) {
			blocked = true;
			expect((e as Error).message).toContain("blocked");
		}
		expect(blocked).toBe(true);
	});

	it("allows safe files through tool.execute.before", async () => {
		const tmpDir = await import("node:os").then((os) => os.tmpdir());
		const hooks = await CodeOpsPlugin({
			directory: tmpDir,
			worktree: tmpDir,
			serverUrl: new URL("http://localhost:4096"),
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			client: {} as any,
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			project: {} as any,
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			$: {} as any,
		});
		const hook = hooks["tool.execute.before"] as NonNullable<
			(typeof hooks)["tool.execute.before"]
		>;
		const output = { args: { filePath: "src/index.ts" } };
		// Should NOT throw or block
		await hook(
			{ tool: "read", sessionID: "test-session", callID: "call-3" },
			output,
		);
		// args should remain unchanged
		expect(output.args.filePath).toBe("src/index.ts");
		expect(output.args.__blocked).toBeUndefined();
	});
});
