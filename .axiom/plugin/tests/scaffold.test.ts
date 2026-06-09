/**
 * Scaffold verification test — proves the plugin module exports the expected shape.
 *
 * axiom:trace work_item=opencode-plugin-entrypoints-01 spec=specs/70-OpenCode-Plugin.md plan=phase-70-1/task-70-1-1/step-70-1-1-1
 */
import { describe, expect, it } from "bun:test";
import { CodeOpsPlugin } from "../src/index";

describe("Plugin scaffold", () => {
	it("exports CodeOpsPlugin as a function", () => {
		expect(typeof CodeOpsPlugin).toBe("function");
	});

	it("exports default as CodeOpsPlugin", async () => {
		const mod = await import("../src/index");
		expect(mod.default).toBe(mod.CodeOpsPlugin);
	});

	it("plugin returns an object with event and tool properties", async () => {
		// Minimal mock of the plugin context
		const result = await CodeOpsPlugin({
			directory: "/tmp/test-repo",
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			project: {} as any,
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			client: {} as any,
			// biome-ignore lint/suspicious/noExplicitAny: intentional test mock
			$: (() => {}) as any,
			worktree: "/tmp/test-repo",
		});

		expect(result).toBeDefined();
		expect(typeof result.event).toBe("function");
		expect(typeof result.tool).toBe("object");
	});
});
