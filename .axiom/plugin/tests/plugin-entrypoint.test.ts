/**
 * Plugin-only entrypoint safety test.
 *
 * Asserts that src/plugin.ts (→ dist/plugin.js) exports ONLY function values.
 * This is the invariant that makes the opencode-axiom/plugin subpath safe for
 * OpenCode's getLegacyPlugins loader, which iterates Object.values(module) and
 * throws "Plugin export is not a function" for any non-function export.
 *
 * WHY THIS TEST EXISTS:
 * The main entrypoint (src/index.ts → dist/index.js) exports ~66 symbols including
 * objects, numbers, and strings — unsafe for getLegacyPlugins. The plugin-only
 * entrypoint (src/plugin.ts → dist/plugin.js) must export ONLY functions.
 * This test enforces that invariant automatically so CI catches any regression.
 *
 * axiom:trace work_item=opencode-plugin-entrypoints-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-001 plan=phase-1/task-1-1/step-1-1-1 test=.axiom/plugin/tests/plugin-entrypoint.test.ts
 */
import { describe, expect, it } from "bun:test";
import * as pluginEntry from "../src/plugin";

describe("Plugin-only entrypoint (src/plugin.ts → dist/plugin.js)", () => {
	it("exports exactly 2 symbols: CodeOpsPlugin and default", () => {
		const keys = Object.keys(pluginEntry);
		expect(keys).toContain("CodeOpsPlugin");
		expect(keys).toContain("default");
		expect(keys.length).toBe(2);
	});

	it("all exports are functions — safe for getLegacyPlugins", () => {
		// OpenCode's getLegacyPlugins iterates Object.values(module) and calls
		// each value as a plugin function. Any non-function export causes:
		// "Plugin export is not a function"
		const entries = Object.entries(pluginEntry);
		for (const [key, val] of entries) {
			expect(typeof val, `export "${key}" must be a function (got ${typeof val})`).toBe("function");
		}
	});

	it("CodeOpsPlugin is an async function", () => {
		const { CodeOpsPlugin } = pluginEntry;
		expect(typeof CodeOpsPlugin).toBe("function");
		expect(CodeOpsPlugin.constructor.name).toBe("AsyncFunction");
	});

	it("default export is the same reference as CodeOpsPlugin", () => {
		expect(pluginEntry.default).toBe(pluginEntry.CodeOpsPlugin);
	});
});
