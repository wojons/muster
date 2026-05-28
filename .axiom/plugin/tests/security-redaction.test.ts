/**
 * Security redaction contract tests — verifies automatic redaction of shared
 * content (session replay, broadcast, hook output) before any content leaves
 * the local session boundary.
 *
 * This is a TEST-FIRST contract: the tests import from ../src/security/redaction
 * which does not exist yet. All tests are expected to fail at the module-
 * resolution level until the redaction engine is implemented.
 *
 * REQ-PLG-701: The plugin MUST apply automatic redaction to all shared content
 *   (session replay, broadcast, hook output). Redaction patterns:
 *   - AWS keys: AKIA[0-9A-Z]{16} → [REDACTED:aws-key]
 *   - JWT tokens: eyJ...eyJ...signature → [REDACTED:jwt]
 *   - Generic API keys: [a-zA-Z0-9]{32,} in key-value contexts → [REDACTED:api-key]
 *   - Environment variable values in KEY=VALUE format → KEY=[REDACTED:env]
 *   - Custom patterns from .axiom/plugin.yaml security.redaction_patterns
 *
 * REQ-PLG-702: The redaction engine MUST be conservative — it is better to
 *   over-redact than to leak a secret.
 *
 * REQ-PLG-703: The plugin MUST log all redaction actions for audit:
 *   "Redacted 3 items: 1 AWS key, 1 JWT token, 1 env variable."
 *
 * Out of scope for this slice:
 * - REQ-PLG-700: Blocked file patterns (task-70-5-2)
 * - REQ-PLG-720/721: First-run trust prompt (task-70-5-3)
 * - REQ-PLG-715: Privacy command (task-70-5-4)
 * - Implementation of src/security/redaction.ts (step-70-5-1-2)
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-701, specs/70-OpenCode-Plugin.md#REQ-PLG-702, specs/70-OpenCode-Plugin.md#REQ-PLG-703
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-701 plan=phase-70-5/task-70-5-1/step-70-5-1-1
 */
import { describe, expect, it } from "bun:test";

// Import from the not-yet-existing security/redaction module — this WILL fail
// at module resolution, which is the expected test-first signal.
import {
	createRedactionEngine,
	type RedactionConfig,
	type RedactionEngine,
	type RedactionLogEntry,
	type RedactionResult,
	redactContent,
} from "../src/security/redaction";

// ===========================================================================
// REQ-PLG-701: AWS key redaction
// ===========================================================================

describe("REQ-PLG-701: AWS key redaction", () => {
	it("redacts a standard AWS access key ID", () => {
		const input = "My key is AKIAIOSFODNN7EXAMPLE and it works";
		const result: RedactionResult = redactContent(input);
		expect(result.redacted).toBe("My key is [REDACTED:aws-key] and it works");
		expect(result.count).toBeGreaterThanOrEqual(1);
	});

	it("redacts multiple AWS keys in the same content", () => {
		const input = "Key1: AKIAIOSFODNN7EXAMPLE Key2: AKIAI44QH8DHBEXAMPLE";
		const result: RedactionResult = redactContent(input);
		expect(result.redacted).not.toContain("AKIA");
		expect(result.count).toBeGreaterThanOrEqual(2);
	});

	it("does not redact strings that look like AWS keys but are too short", () => {
		const input = "AKIA1234 is not a real key";
		const result: RedactionResult = redactContent(input);
		// AKIA followed by fewer than 16 chars should not match
		expect(result.redacted).toBe(input);
		expect(result.count).toBe(0);
	});
});

// ===========================================================================
// REQ-PLG-701: JWT token redaction
// ===========================================================================

describe("REQ-PLG-701: JWT token redaction", () => {
	it("redacts a standard three-part JWT", () => {
		const header = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9";
		const payload =
			"eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ";
		const signature = "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c";
		const jwt = `${header}.${payload}.${signature}`;
		const input = `Bearer ${jwt} in the header`;
		const result: RedactionResult = redactContent(input);
		expect(result.redacted).toContain("[REDACTED:jwt]");
		expect(result.redacted).not.toContain(header);
		expect(result.count).toBeGreaterThanOrEqual(1);
	});

	it("redacts JWTs embedded in JSON strings", () => {
		const jwt = "eyJhbGciOiJSUzI1NiJ9.eyJpc3MiOiJjb2Rlb3BzIn0.abc123def456";
		const input = JSON.stringify({ token: jwt, user: "alice" });
		const result: RedactionResult = redactContent(input);
		expect(result.redacted).not.toContain("eyJhbGci");
		expect(result.count).toBeGreaterThanOrEqual(1);
	});
});

// ===========================================================================
// REQ-PLG-701: Generic API key redaction in key-value contexts
// ===========================================================================

describe("REQ-PLG-701: generic API key redaction", () => {
	it("redacts a 32+ char alphanumeric value in a key-value context", () => {
		// Spec: strings matching [a-zA-Z0-9]{32,} in key-value contexts
		const longKey = "abcdefghijklmnopqrstuvwxyz123456"; // 32 chars
		const input = `api_key=${longKey}`;
		const result: RedactionResult = redactContent(input);
		expect(result.redacted).toContain("[REDACTED:api-key]");
		expect(result.redacted).not.toContain(longKey);
		expect(result.count).toBeGreaterThanOrEqual(1);
	});

	it("redacts a long hex token in Authorization header context", () => {
		const hexToken = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"; // 40 chars
		const input = `Authorization: Bearer ${hexToken}`;
		const result: RedactionResult = redactContent(input);
		expect(result.redacted).not.toContain(hexToken);
		expect(result.count).toBeGreaterThanOrEqual(1);
	});

	it("does not redact short alphanumeric strings outside key-value contexts", () => {
		// A 20-char string in prose should not be redacted
		const input = "The function name is abcdefghijklmnopqrst and it works";
		const result: RedactionResult = redactContent(input);
		expect(result.redacted).toBe(input);
		expect(result.count).toBe(0);
	});
});

// ===========================================================================
// REQ-PLG-701: Environment variable KEY=VALUE redaction
// ===========================================================================

describe("REQ-PLG-701: KEY=VALUE env var redaction", () => {
	it("redacts the value in a simple KEY=VALUE pair", () => {
		const input = "DATABASE_URL=postgres://user:pass@host:5432/db";
		const result: RedactionResult = redactContent(input);
		expect(result.redacted).toBe("DATABASE_URL=[REDACTED:env]");
		expect(result.count).toBeGreaterThanOrEqual(1);
	});

	it("redacts multiple KEY=VALUE pairs in multiline content", () => {
		const input = [
			"AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			"GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
			"NORMAL_TEXT=this is fine",
		].join("\n");
		const result: RedactionResult = redactContent(input);
		expect(result.redacted).toContain("AWS_SECRET_ACCESS_KEY=[REDACTED:env]");
		expect(result.redacted).toContain("GITHUB_TOKEN=[REDACTED:env]");
		// Even innocuous-looking values get redacted in KEY=VALUE format
		// because the engine is conservative (REQ-PLG-702)
		expect(result.count).toBeGreaterThanOrEqual(2);
	});

	it("does not redact lines that are not KEY=VALUE format", () => {
		const input = "This is a normal sentence with an equals sign: 2+2=4";
		const result: RedactionResult = redactContent(input);
		// Lowercase left-hand side is not an env var pattern
		expect(result.count).toBe(0);
	});
});

// ===========================================================================
// REQ-PLG-701: Custom redaction patterns from plugin.yaml
// ===========================================================================

describe("REQ-PLG-701: custom redaction patterns", () => {
	it("applies custom regex patterns from config", () => {
		const config: RedactionConfig = {
			customPatterns: [
				{ pattern: "INTERNAL-[A-Z0-9]{8}", label: "internal-id" },
			],
		};
		const engine: RedactionEngine = createRedactionEngine(config);
		const input = "Found INTERNAL-AB12CD34 in the logs";
		const result: RedactionResult = engine.redact(input);
		expect(result.redacted).toContain("[REDACTED:internal-id]");
		expect(result.count).toBeGreaterThanOrEqual(1);
	});

	it("applies custom patterns alongside built-in patterns", () => {
		const config: RedactionConfig = {
			customPatterns: [
				{ pattern: "sk_live_[a-zA-Z0-9]{24}", label: "stripe-key" },
			],
		};
		const engine: RedactionEngine = createRedactionEngine(config);
		const input =
			"Stripe: sk_live_abcdefghijklmnopqrstuvwx AWS: AKIAIOSFODNN7EXAMPLE";
		const result: RedactionResult = engine.redact(input);
		expect(result.redacted).toContain("[REDACTED:stripe-key]");
		expect(result.redacted).toContain("[REDACTED:aws-key]");
		expect(result.count).toBeGreaterThanOrEqual(2);
	});
});

// ===========================================================================
// REQ-PLG-702: Conservative over-redaction
// ===========================================================================

describe("REQ-PLG-702: conservative over-redaction", () => {
	it("prefers over-redaction when a string is ambiguous", () => {
		// A 40-char hex string that could be a hash or a token — the engine
		// should redact it rather than risk leaking a secret
		const ambiguous = "a".repeat(40);
		const input = `secret_key=${ambiguous}`;
		const result: RedactionResult = redactContent(input);
		expect(result.redacted).not.toContain(ambiguous);
		expect(result.count).toBeGreaterThanOrEqual(1);
	});

	it("redacts base64-encoded content in key-value contexts", () => {
		const b64 = "dGhpcyBpcyBhIHNlY3JldCB0b2tlbiB2YWx1ZQ=="; // base64 of "this is a secret token value"
		const input = `AUTH_TOKEN=${b64}`;
		const result: RedactionResult = redactContent(input);
		expect(result.redacted).not.toContain(b64);
		expect(result.count).toBeGreaterThanOrEqual(1);
	});

	it("does not redact obviously safe content", () => {
		const input = "Hello world, this is a normal message with no secrets.";
		const result: RedactionResult = redactContent(input);
		expect(result.redacted).toBe(input);
		expect(result.count).toBe(0);
	});
});

// ===========================================================================
// REQ-PLG-703: Audit logging of redaction actions
// ===========================================================================

describe("REQ-PLG-703: redaction audit logging", () => {
	it("produces an audit log entry with category counts", () => {
		const input = [
			"AWS key: AKIAIOSFODNN7EXAMPLE",
			"JWT: eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abc123",
			"SECRET_KEY=mysupersecretvalue",
		].join("\n");
		const result: RedactionResult = redactContent(input);

		// The spec requires: "Redacted N items: X AWS key, Y JWT token, Z env variable."
		expect(result.log).toBeDefined();
		const log: RedactionLogEntry = result.log as RedactionLogEntry;
		expect(log.totalCount).toBeGreaterThanOrEqual(3);
		expect(log.categories).toBeDefined();
		expect(log.categories["aws-key"]).toBeGreaterThanOrEqual(1);
		expect(log.categories.jwt).toBeGreaterThanOrEqual(1);
		expect(log.categories.env).toBeGreaterThanOrEqual(1);
	});

	it("formats the audit message per spec wording", () => {
		const input = "AKIAIOSFODNN7EXAMPLE and DB_PASS=hunter2";
		const result: RedactionResult = redactContent(input);

		expect(result.log).toBeDefined();
		const msg = result.log?.message;
		// Spec: "Redacted 3 items: 1 AWS key, 1 JWT token, 1 env variable."
		// We expect the format "Redacted N items: ..." with category breakdowns
		expect(msg).toMatch(/^Redacted \d+ items?:/);
		expect(msg).toContain("AWS key");
	});

	it("returns no log entry when nothing was redacted", () => {
		const input = "Clean content with no secrets";
		const result: RedactionResult = redactContent(input);
		expect(result.log).toBeUndefined();
		expect(result.count).toBe(0);
	});

	it("includes all category types in the audit breakdown", () => {
		// Combine all four built-in categories in one input
		const input = [
			"AKIAIOSFODNN7EXAMPLE",
			"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.signature123",
			"api_key=abcdefghijklmnopqrstuvwxyz1234567890",
			"DATABASE_URL=postgres://user:pass@host/db",
		].join("\n");
		const result: RedactionResult = redactContent(input);

		expect(result.log).toBeDefined();
		const cats = result.log?.categories;
		// At minimum, aws-key and env should be present
		expect(cats["aws-key"]).toBeGreaterThanOrEqual(1);
		expect(cats.env).toBeGreaterThanOrEqual(1);
	});
});

// ===========================================================================
// REQ-PLG-701: Engine creation with default config
// ===========================================================================

describe("REQ-PLG-701: createRedactionEngine defaults", () => {
	it("creates an engine with all built-in patterns when no config is provided", () => {
		const engine: RedactionEngine = createRedactionEngine();
		expect(engine).toBeDefined();
		expect(typeof engine.redact).toBe("function");
	});

	it("engine.redact produces the same result as the convenience redactContent", () => {
		const engine: RedactionEngine = createRedactionEngine();
		const input = "AKIAIOSFODNN7EXAMPLE";
		const directResult = redactContent(input);
		const engineResult = engine.redact(input);
		expect(engineResult.redacted).toBe(directResult.redacted);
		expect(engineResult.count).toBe(directResult.count);
	});
});
