/**
 * Privacy command contract tests — verifies that the `/axiom-privacy`
 * command shows what data the plugin collects, where it is stored, what
 * has been shared, how to delete shared data, and current redaction patterns.
 *
 * This is a TEST-FIRST contract: the tests import from
 * ../src/security/privacy-command which does not exist yet. All tests are
 * expected to fail at the module-resolution level until the privacy command
 * is implemented.
 *
 * REQ-PLG-715: The plugin MUST provide a `/axiom-privacy` command that shows:
 *   - What data the plugin collects
 *   - Where data is stored (local vs remote)
 *   - What has been shared and with whom
 *   - How to delete shared data
 *   - Current redaction patterns
 *
 * Out of scope for this slice:
 * - REQ-PLG-720/721: First-run trust prompt (deferred v2)
 * - REQ-PLG-700: Blocked file patterns (already done)
 * - REQ-PLG-701/702/703: Redaction engine (already done)
 * - Phase 70.4 config automation (deferred per spec section 1.2)
 * - Implementation of src/security/privacy-command.ts (step-70-5-4-2)
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-715
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-715 plan=phase-70-5/task-70-5-4/step-70-5-4-1
 */
import { describe, expect, it } from "bun:test";

// Import from the not-yet-existing security/privacy-command module — this WILL
// fail at module resolution, which is the expected test-first signal.
import {
	buildPrivacyReport,
	type DataCategory,
	formatPrivacyReport,
	type PrivacyReport,
	type RedactionPatternSummary,
	type StorageLocation,
} from "../src/security/privacy-command";

// ===========================================================================
// REQ-PLG-715 §1: What data the plugin collects
// ===========================================================================

describe("REQ-PLG-715: data collection categories", () => {
	it("reports session analytics as a collected data category", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		const categories = report.dataCategories;
		const sessionAnalytics = categories.find(
			(c: DataCategory) => c.id === "session-analytics",
		);
		expect(sessionAnalytics).toBeDefined();
		expect(sessionAnalytics?.description).toContain("cost");
	});

	it("reports maturity detection signals as a collected data category", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		const categories = report.dataCategories;
		const maturity = categories.find(
			(c: DataCategory) => c.id === "maturity-detection",
		);
		expect(maturity).toBeDefined();
		expect(maturity?.description).toContain("maturity");
	});

	it("reports configuration data as a collected data category", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		const categories = report.dataCategories;
		const config = categories.find(
			(c: DataCategory) => c.id === "configuration",
		);
		expect(config).toBeDefined();
	});

	it("does NOT report file contents or conversation text as collected", () => {
		// Per REQ-PLG-714: session analytics MUST NOT contain file contents,
		// conversation text, or tool outputs — only aggregate metrics.
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		const categories = report.dataCategories;
		const fileContents = categories.find(
			(c: DataCategory) => c.id === "file-contents",
		);
		const conversationText = categories.find(
			(c: DataCategory) => c.id === "conversation-text",
		);
		expect(fileContents).toBeUndefined();
		expect(conversationText).toBeUndefined();
	});

	it("includes at least 2 data categories", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		expect(report.dataCategories.length).toBeGreaterThanOrEqual(2);
	});
});

// ===========================================================================
// REQ-PLG-715 §2: Where data is stored (local vs remote)
// ===========================================================================

describe("REQ-PLG-715: storage locations", () => {
	it("reports .axiom/sessions/ as a local storage location", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		const locations = report.storageLocations;
		const sessions = locations.find((l: StorageLocation) =>
			l.path.includes("sessions"),
		);
		expect(sessions).toBeDefined();
		expect(sessions?.type).toBe("local");
	});

	it("reports .axiom/axiom.config.yaml as a local storage location", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		const locations = report.storageLocations;
		const config = locations.find((l: StorageLocation) =>
			l.path.includes("axiom.config.yaml"),
		);
		expect(config).toBeDefined();
		expect(config?.type).toBe("local");
	});

	it("reports .axiom/plugin.yaml as a local storage location", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		const locations = report.storageLocations;
		const pluginConfig = locations.find((l: StorageLocation) =>
			l.path.includes("plugin.yaml"),
		);
		expect(pluginConfig).toBeDefined();
		expect(pluginConfig?.type).toBe("local");
	});

	it("marks all v1 storage as local (no remote in v1)", () => {
		// v1 scope: L3 session sharing is deferred, so no remote storage exists
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		const remoteLocations = report.storageLocations.filter(
			(l: StorageLocation) => l.type === "remote",
		);
		expect(remoteLocations.length).toBe(0);
	});

	it("includes at least 2 storage locations", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		expect(report.storageLocations.length).toBeGreaterThanOrEqual(2);
	});
});

// ===========================================================================
// REQ-PLG-715 §3: What has been shared and with whom
// ===========================================================================

describe("REQ-PLG-715: sharing status", () => {
	it("reports no shared data when no sessions have been shared", () => {
		// v1 has no L3 sharing, so this should always be empty
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		expect(report.sharedData).toEqual([]);
	});

	it("sharedData is an array of SharedDataEntry", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		expect(Array.isArray(report.sharedData)).toBe(true);
	});

	it("includes a human-readable sharing summary", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		expect(report.sharingSummary).toBeDefined();
		expect(typeof report.sharingSummary).toBe("string");
		// Should indicate sharing status (either nothing shared or count of shared sessions)
		expect(report.sharingSummary.length).toBeGreaterThan(0);
		expect(report.sharingSummary.toLowerCase()).toMatch(/session|share|no/);
	});
});

// ===========================================================================
// REQ-PLG-715 §4: How to delete shared data
// ===========================================================================

describe("REQ-PLG-715: data deletion guidance", () => {
	it("provides deletion instructions for local session data", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		expect(report.deletionGuidance).toBeDefined();
		expect(report.deletionGuidance.localSessions).toBeDefined();
		expect(typeof report.deletionGuidance.localSessions).toBe("string");
		expect(report.deletionGuidance.localSessions).toContain(
			".axiom/sessions",
		);
	});

	it("provides deletion instructions for configuration data", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		expect(report.deletionGuidance.configuration).toBeDefined();
		expect(typeof report.deletionGuidance.configuration).toBe("string");
	});

	it("references /axiom-share delete for shared data when sharing exists", () => {
		// Per REQ-PLG-712: shared data deletable via /axiom-share delete <session-id>
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		expect(report.deletionGuidance.sharedData).toBeDefined();
		expect(typeof report.deletionGuidance.sharedData).toBe("string");
		// v1: should mention the command even though sharing is not yet available
		expect(report.deletionGuidance.sharedData).toContain("axiom-share");
	});
});

// ===========================================================================
// REQ-PLG-715 §5: Current redaction patterns
// ===========================================================================

describe("REQ-PLG-715: redaction pattern summary", () => {
	it("lists built-in redaction patterns", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		const patterns = report.redactionPatterns;
		expect(patterns.builtIn.length).toBeGreaterThanOrEqual(4);
		// Must include the REQ-PLG-701 categories
		const ids = patterns.builtIn.map((p: RedactionPatternSummary) => p.id);
		expect(ids).toContain("aws-key");
		expect(ids).toContain("jwt");
		expect(ids).toContain("api-key");
		expect(ids).toContain("env-var");
	});

	it("reports custom redaction patterns when none are configured", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		expect(report.redactionPatterns.custom).toEqual([]);
	});

	it("lists blocked file patterns", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		const blocked = report.redactionPatterns.blockedFilePatterns;
		expect(blocked.length).toBeGreaterThanOrEqual(3);
		// Must include core REQ-PLG-700 patterns
		expect(blocked.some((p: string) => p.includes(".env"))).toBe(true);
		expect(blocked.some((p: string) => p.includes(".pem"))).toBe(true);
	});
});

// ===========================================================================
// formatPrivacyReport: human-readable output
// ===========================================================================

describe("REQ-PLG-715: formatted privacy report", () => {
	it("produces a non-empty string", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		const formatted = formatPrivacyReport(report);
		expect(typeof formatted).toBe("string");
		expect(formatted.length).toBeGreaterThan(0);
	});

	it("includes a section about collected data", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		const formatted = formatPrivacyReport(report);
		expect(formatted.toLowerCase()).toContain("collect");
	});

	it("includes a section about storage locations", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		const formatted = formatPrivacyReport(report);
		expect(formatted.toLowerCase()).toContain("storage");
	});

	it("includes a section about sharing status", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		const formatted = formatPrivacyReport(report);
		expect(formatted.toLowerCase()).toContain("shar");
	});

	it("includes a section about deletion", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		const formatted = formatPrivacyReport(report);
		expect(formatted.toLowerCase()).toContain("delet");
	});

	it("includes a section about redaction patterns", () => {
		const report: PrivacyReport = buildPrivacyReport({
			repoDir: "/tmp/test-repo",
		});
		const formatted = formatPrivacyReport(report);
		expect(formatted.toLowerCase()).toContain("redact");
	});
});
