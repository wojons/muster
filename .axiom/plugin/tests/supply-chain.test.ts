/**
 * Supply-chain security contract tests — verifies npm provenance attestation
 * guidance, exact-package-name plus typosquat startup warnings, startup audit
 * logging of version and source path, and the explicit no-auto-update rule.
 *
 * This is a TEST-FIRST contract: the tests import from
 * ../src/security/supply-chain which does not exist yet. All tests are
 * expected to fail at the module-resolution level until the supply-chain
 * security controls are implemented.
 *
 * REQ-PLG-800: The npm package MUST be published with npm provenance
 *   attestations (`npm publish --provenance`) to enable consumers to verify
 *   the package was built from the Axiom repo CI pipeline.
 *
 * REQ-PLG-801: The README, install docs, and `/axiom-setup` output MUST
 *   specify the exact package name (`opencode-axiom`) and warn about
 *   typosquats. The plugin MUST verify its own package name on startup and
 *   log a warning if loaded from an unexpected path.
 *
 * REQ-PLG-802: The plugin MUST log its package version and source path on
 *   startup (via `client.app.log()`) for audit:
 *   `"Axiom plugin v1.0.0 loaded from ~/.cache/opencode/node_modules/opencode-axiom"`
 *
 * REQ-PLG-803: The plugin MUST NOT auto-update. Version changes require
 *   explicit user action (updating `opencode.json` or running `npm update`).
 *   The plugin MUST log when a newer version is available (checked against
 *   npm registry at most once per day, best-effort, non-blocking).
 *
 * Out of scope for this slice:
 * - REQ-PLG-700: Blocked file patterns (already done)
 * - REQ-PLG-701/702/703: Redaction engine (already done)
 * - REQ-PLG-715: Privacy command (already done)
 * - REQ-PLG-720/721: First-run trust prompt (deferred v2)
 * - Phase 70.4 config automation (deferred per spec section 1.2)
 * - Implementation of src/security/supply-chain.ts (step-70-5-5-2)
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-800, specs/70-OpenCode-Plugin.md#REQ-PLG-801,
 *       specs/70-OpenCode-Plugin.md#REQ-PLG-802, specs/70-OpenCode-Plugin.md#REQ-PLG-803
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-800 plan=phase-70-5/task-70-5-5/step-70-5-5-1
 */
import { describe, expect, it } from "bun:test";

// Import from the not-yet-existing security/supply-chain module — this WILL
// fail at module resolution, which is the expected test-first signal.
import {
	buildStartupAuditLog,
	checkForUpdates,
	getProvenanceGuidance,
	type PackageNameVerification,
	type ProvenanceGuidance,
	type StartupAuditLog,
	type SupplyChainConfig,
	type UpdateCheckResult,
	verifyPackageName,
} from "../src/security/supply-chain";

// ===========================================================================
// REQ-PLG-800: npm provenance attestation guidance
// ===========================================================================

describe("REQ-PLG-800: npm provenance attestation guidance", () => {
	it("returns provenance guidance including --provenance flag", () => {
		const guidance: ProvenanceGuidance = getProvenanceGuidance();
		expect(guidance.publishCommand).toContain("--provenance");
	});

	it("references the Axiom repo CI pipeline as the expected build source", () => {
		const guidance: ProvenanceGuidance = getProvenanceGuidance();
		expect(guidance.expectedBuildSource).toContain("Axiom");
	});

	it("includes a verification command for consumers", () => {
		const guidance: ProvenanceGuidance = getProvenanceGuidance();
		expect(guidance.verificationCommand).toBeTruthy();
		expect(typeof guidance.verificationCommand).toBe("string");
	});

	it("documents the attestation format (sigstore)", () => {
		const guidance: ProvenanceGuidance = getProvenanceGuidance();
		expect(guidance.attestationFormat).toBe("sigstore");
	});
});

// ===========================================================================
// REQ-PLG-801: exact package name and typosquat warning
// ===========================================================================

describe("REQ-PLG-801: package name verification on startup", () => {
	it("returns ok when loaded from the expected package name", () => {
		const result: PackageNameVerification = verifyPackageName({
			loadedFrom:
				"/home/user/.cache/opencode/node_modules/opencode-axiom/dist/index.js",
		});
		expect(result.isExpectedPackage).toBe(true);
		expect(result.warning).toBeUndefined();
	});

	it("returns a warning when loaded from an unexpected path", () => {
		const result: PackageNameVerification = verifyPackageName({
			loadedFrom:
				"/home/user/.cache/opencode/node_modules/opencode-codops/dist/index.js",
		});
		expect(result.isExpectedPackage).toBe(false);
		expect(result.warning).toBeTruthy();
		expect(result.warning).toContain("opencode-axiom");
	});

	it("detects common typosquat variants", () => {
		const typosquats = [
			"opencode-codops",
			"opencode_codeops",
			"opencodeops",
			"opencode-code-ops",
		];
		for (const name of typosquats) {
			const result = verifyPackageName({
				loadedFrom: `/node_modules/${name}/dist/index.js`,
			});
			expect(result.isExpectedPackage).toBe(false);
			expect(result.warning).toContain("typosquat");
		}
	});

	it("includes the canonical package name in the warning", () => {
		const result = verifyPackageName({
			loadedFrom: "/node_modules/opencode-codops/dist/index.js",
		});
		expect(result.canonicalPackageName).toBe("opencode-axiom");
	});

	it("extracts the detected package name from the load path", () => {
		const result = verifyPackageName({
			loadedFrom: "/node_modules/opencode-codops/dist/index.js",
		});
		expect(result.detectedPackageName).toBe("opencode-codops");
	});
});

// ===========================================================================
// REQ-PLG-802: startup audit logging of version and source path
// ===========================================================================

describe("REQ-PLG-802: startup audit log entry", () => {
	it("includes the plugin version in the log message", () => {
		const log: StartupAuditLog = buildStartupAuditLog({
			version: "1.0.0",
			sourcePath: "/home/user/.cache/opencode/node_modules/opencode-axiom",
		});
		expect(log.message).toContain("v1.0.0");
	});

	it("includes the source path in the log message", () => {
		const log: StartupAuditLog = buildStartupAuditLog({
			version: "1.0.0",
			sourcePath: "/home/user/.cache/opencode/node_modules/opencode-axiom",
		});
		expect(log.message).toContain(
			"/home/user/.cache/opencode/node_modules/opencode-axiom",
		);
	});

	it("follows the spec-defined format: Axiom plugin vX.Y.Z loaded from <path>", () => {
		const log: StartupAuditLog = buildStartupAuditLog({
			version: "2.3.4",
			sourcePath: "/opt/opencode/node_modules/opencode-axiom",
		});
		expect(log.message).toBe(
			"Axiom plugin v2.3.4 loaded from /opt/opencode/node_modules/opencode-axiom",
		);
	});

	it("exposes structured fields for programmatic consumption", () => {
		const log: StartupAuditLog = buildStartupAuditLog({
			version: "1.0.0",
			sourcePath: "/some/path",
		});
		expect(log.version).toBe("1.0.0");
		expect(log.sourcePath).toBe("/some/path");
		expect(log.timestamp).toBeTruthy();
	});
});

// ===========================================================================
// REQ-PLG-803: no auto-update rule
// ===========================================================================

describe("REQ-PLG-803: no auto-update behavior", () => {
	it("never triggers automatic updates", () => {
		const result: UpdateCheckResult = checkForUpdates({
			currentVersion: "1.0.0",
			latestVersion: "2.0.0",
		});
		expect(result.autoUpdateTriggered).toBe(false);
	});

	it("reports when a newer version is available", () => {
		const result: UpdateCheckResult = checkForUpdates({
			currentVersion: "1.0.0",
			latestVersion: "2.0.0",
		});
		expect(result.updateAvailable).toBe(true);
		expect(result.message).toContain("2.0.0");
	});

	it("reports no update when already on latest", () => {
		const result: UpdateCheckResult = checkForUpdates({
			currentVersion: "1.0.0",
			latestVersion: "1.0.0",
		});
		expect(result.updateAvailable).toBe(false);
	});

	it("suggests explicit user action for version changes", () => {
		const result: UpdateCheckResult = checkForUpdates({
			currentVersion: "1.0.0",
			latestVersion: "2.0.0",
		});
		expect(result.upgradeInstructions).toBeTruthy();
		// Must mention either opencode.json or npm update
		const instructions = result.upgradeInstructions as string;
		expect(
			instructions.includes("opencode.json") ||
				instructions.includes("npm update"),
		).toBe(true);
	});

	it("respects the once-per-day check frequency", () => {
		const result: UpdateCheckResult = checkForUpdates({
			currentVersion: "1.0.0",
			latestVersion: "2.0.0",
			lastCheckTimestamp: Date.now() - 1000, // 1 second ago
		});
		expect(result.skippedDueToFrequency).toBe(true);
	});

	it("allows a check when more than 24 hours have passed", () => {
		const result: UpdateCheckResult = checkForUpdates({
			currentVersion: "1.0.0",
			latestVersion: "2.0.0",
			lastCheckTimestamp: Date.now() - 25 * 60 * 60 * 1000, // 25 hours ago
		});
		expect(result.skippedDueToFrequency).toBe(false);
	});

	it("handles missing lastCheckTimestamp as first-ever check", () => {
		const result: UpdateCheckResult = checkForUpdates({
			currentVersion: "1.0.0",
			latestVersion: "2.0.0",
		});
		expect(result.skippedDueToFrequency).toBe(false);
	});
});

// ===========================================================================
// Integration: supply-chain config shape
// ===========================================================================

describe("supply-chain config shape", () => {
	it("accepts a SupplyChainConfig with all optional fields", () => {
		const config: SupplyChainConfig = {
			expectedPackageName: "opencode-axiom",
			updateCheckFrequencyMs: 24 * 60 * 60 * 1000,
			enableProvenanceCheck: true,
		};
		expect(config.expectedPackageName).toBe("opencode-axiom");
		expect(config.updateCheckFrequencyMs).toBe(86400000);
		expect(config.enableProvenanceCheck).toBe(true);
	});
});
