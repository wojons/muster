/**
 * Tests for REQ-PLG-708 (HTTPS enforcement) and REQ-PLG-709 (URL allowlist).
 *
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-708,REQ-PLG-709 plan=phase-70-6/task-70-6-5/step-70-6-5-1 jira_ref=DEX-64
 */

import { describe, it, expect } from "bun:test";
import { validateOutboundUrl } from "../src/layers/network";

// ── REQ-PLG-708: HTTPS enforcement ───────────────────────────────────────

describe("REQ-PLG-708: HTTPS enforcement", () => {
	it("accepts HTTPS URLs", () => {
		expect(() =>
			validateOutboundUrl("https://api.axiom.dev/v1/sessions"),
		).not.toThrow();
	});

	it("rejects HTTP URLs with clear error", () => {
		expect(() =>
			validateOutboundUrl("http://api.axiom.dev/v1/sessions"),
		).toThrow("REQ-PLG-708");
	});

	it("rejects non-HTTP/HTTPS URLs (ftp)", () => {
		expect(() =>
			validateOutboundUrl("ftp://api.axiom.dev/file"),
		).toThrow("REQ-PLG-708");
	});

	it("rejects empty string", () => {
		expect(() => validateOutboundUrl("")).toThrow("REQ-PLG-708");
	});

	it("rejects URL with no scheme", () => {
		expect(() => validateOutboundUrl("api.axiom.dev/v1/sessions")).toThrow(
			"REQ-PLG-708",
		);
	});

	it("error message includes the offending URL prefix", () => {
		let msg = "";
		try {
			validateOutboundUrl("http://evil.example.com/steal");
		} catch (e) {
			msg = (e as Error).message;
		}
		expect(msg).toContain("http://evil.example.com");
	});
});

// ── REQ-PLG-709: URL allowlist ────────────────────────────────────────────

describe("REQ-PLG-709: URL allowlist", () => {
	it("accepts URL matching allowed domain (full https:// prefix)", () => {
		expect(() =>
			validateOutboundUrl(
				"https://api.axiom.dev/v1/sessions",
				"https://api.axiom.dev",
			),
		).not.toThrow();
	});

	it("accepts URL matching allowed domain (bare hostname)", () => {
		expect(() =>
			validateOutboundUrl(
				"https://api.axiom.dev/v1/sessions",
				"api.axiom.dev",
			),
		).not.toThrow();
	});

	it("accepts URL with path/query when domain matches", () => {
		expect(() =>
			validateOutboundUrl(
				"https://api.axiom.dev/api/v1/heartbeat?foo=bar",
				"https://api.axiom.dev",
			),
		).not.toThrow();
	});

	it("rejects URL not matching allowed domain", () => {
		expect(() =>
			validateOutboundUrl(
				"https://evil.example.com/steal",
				"https://api.axiom.dev",
			),
		).toThrow("REQ-PLG-709");
	});

	it("error message names the expected and actual domain", () => {
		let msg = "";
		try {
			validateOutboundUrl(
				"https://evil.example.com/steal",
				"https://api.axiom.dev",
			);
		} catch (e) {
			msg = (e as Error).message;
		}
		expect(msg).toContain("api.axiom.dev");
		expect(msg).toContain("evil.example.com");
	});

	it("no allowlist = any HTTPS URL accepted", () => {
		expect(() =>
			validateOutboundUrl("https://any.domain.com/path"),
		).not.toThrow();
	});

	it("subdomain of allowed domain is rejected (exact hostname match)", () => {
		expect(() =>
			validateOutboundUrl(
				"https://sub.api.axiom.dev/v1/sessions",
				"https://api.axiom.dev",
			),
		).toThrow("REQ-PLG-709");
	});
});
