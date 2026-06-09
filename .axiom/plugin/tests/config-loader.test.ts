/**
 * Config-loader tests — verifies .axiom/plugin.yaml and .axiom/plugin.json
 * loading, schema validation, version gating, and fallback behavior per
 * REQ-PLG-400, REQ-PLG-401, REQ-PLG-402, and REQ-PLG-403.
 *
 * These tests are written BEFORE the config-loader module exists.
 * They MUST fail initially and drive the implementation.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-400, #REQ-PLG-401, #REQ-PLG-402, #REQ-PLG-403
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-400 plan=phase-70-4/task-70-4-1/step-70-4-1-1
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { existsSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { PluginConfig } from "../src/config-loader";
// ── Config-loader module import (will fail until implemented) ────────────
// REQ-PLG-400: load automation config from .axiom/plugin.yaml or .axiom/plugin.json
// REQ-PLG-401: schema with version, layers, metrics, hooks, watches, scripts, tools
// REQ-PLG-402: validate on load; invalid config -> conversational error + defaults
// REQ-PLG-403: version: 1 for forward compat; unknown version -> warning + defaults
import {
	DEFAULT_PLUGIN_CONFIG,
	loadPluginConfig,
	validatePluginConfig,
} from "../src/config-loader";

/** Create a unique temp directory for each test */
function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-config-loader-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
	);
	mkdirSync(dir, { recursive: true });
	return dir;
}

/** Helper to write a file, creating parent dirs as needed */
function writeFile(filePath: string, content: string): void {
	mkdirSync(join(filePath, ".."), { recursive: true });
	writeFileSync(filePath, content);
}

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-400: Load automation config from .axiom/plugin.yaml or .json
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-400: load plugin config from .axiom/plugin.yaml or .json", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("loads config from .axiom/plugin.yaml when present", () => {
		const yamlContent = `version: 1
layers:
  metrics: true
  automation: true
`;
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), yamlContent);

		const result = loadPluginConfig(tempDir);
		expect(result).toBeDefined();
		expect(result.loaded).toBe(true);
		expect(result.source).toBe("plugin.yaml");
		expect(result.config).toBeDefined();
		expect(result.config?.version).toBe(1);
	});

	it("loads config from .axiom/plugin.json when present", () => {
		const jsonContent = JSON.stringify({
			version: 1,
			layers: { metrics: true, automation: false },
		});
		writeFile(join(tempDir, ".axiom", "plugin.json"), jsonContent);

		const result = loadPluginConfig(tempDir);
		expect(result).toBeDefined();
		expect(result.loaded).toBe(true);
		expect(result.source).toBe("plugin.json");
		expect(result.config).toBeDefined();
		expect(result.config?.version).toBe(1);
	});

	it("prefers .axiom/plugin.yaml over .axiom/plugin.json when both exist", () => {
		writeFile(
			join(tempDir, ".axiom", "plugin.yaml"),
			"version: 1\nlayers:\n  metrics: true\n",
		);
		writeFile(
			join(tempDir, ".axiom", "plugin.json"),
			JSON.stringify({ version: 1, layers: { metrics: false } }),
		);

		const result = loadPluginConfig(tempDir);
		expect(result.loaded).toBe(true);
		expect(result.source).toBe("plugin.yaml");
		// YAML version wins — metrics should be true
		expect(result.config?.layers?.metrics).toBe(true);
	});

	it("returns loaded=false with no error when neither file exists (L5 disabled silently)", () => {
		// REQ-PLG-400: "If the file does not exist, L5 is disabled silently."
		const result = loadPluginConfig(tempDir);
		expect(result.loaded).toBe(false);
		expect(result.config).toBeUndefined();
		expect(result.error).toBeUndefined();
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-401: Config schema — top-level sections
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-401: config schema top-level sections", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("parses the version field", () => {
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), "version: 1\n");
		const result = loadPluginConfig(tempDir);
		expect(result.loaded).toBe(true);
		expect(result.config?.version).toBe(1);
	});

	it("parses the layers section with boolean toggles", () => {
		const yaml = `version: 1
layers:
  metrics: true
  session_sharing: false
  network: false
  automation: true
  tools: true
`;
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), yaml);
		const result = loadPluginConfig(tempDir);
		expect(result.loaded).toBe(true);
		const layers = (result.config as PluginConfig).layers as NonNullable<
			PluginConfig["layers"]
		>;
		expect(layers.metrics).toBe(true);
		expect(layers.session_sharing).toBe(false);
		expect(layers.network).toBe(false);
		expect(layers.automation).toBe(true);
		expect(layers.tools).toBe(true);
	});

	it("parses the metrics section with budget and pricing overrides", () => {
		const yaml = `version: 1
metrics:
  session_budget_usd: 5.00
  warning_threshold_pct: 80
  persist_sessions: true
`;
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), yaml);
		const result = loadPluginConfig(tempDir);
		expect(result.loaded).toBe(true);
		const metrics = (result.config as PluginConfig).metrics as NonNullable<
			PluginConfig["metrics"]
		>;
		expect(metrics.session_budget_usd).toBe(5.0);
		expect(metrics.warning_threshold_pct).toBe(80);
		expect(metrics.persist_sessions).toBe(true);
	});

	it("parses the hooks section with lifecycle event arrays", () => {
		const yaml = `version: 1
hooks:
  on_session_start:
    - name: "load-context"
      run: "cat .axiom/context.md"
      description: "Load project context"
`;
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), yaml);
		const result = loadPluginConfig(tempDir);
		expect(result.loaded).toBe(true);
		const hooks = (result.config as PluginConfig).hooks as NonNullable<
			PluginConfig["hooks"]
		>;
		expect(hooks.on_session_start).toBeDefined();
		expect(Array.isArray(hooks.on_session_start)).toBe(true);
		expect(hooks.on_session_start?.length).toBeGreaterThanOrEqual(1);
		expect(hooks.on_session_start?.[0].name).toBe("load-context");
		expect(hooks.on_session_start?.[0].run).toBe("cat .axiom/context.md");
	});

	it("parses the scripts section with named script entries", () => {
		const yaml = `version: 1
scripts:
  lint:
    run: "npm run lint"
    description: "Run project linter"
  test:
    run: "npm test"
    description: "Run project tests"
`;
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), yaml);
		const result = loadPluginConfig(tempDir);
		expect(result.loaded).toBe(true);
		const scripts = (result.config as PluginConfig).scripts as NonNullable<
			PluginConfig["scripts"]
		>;
		expect(scripts.lint).toBeDefined();
		expect(scripts.lint?.run).toBe("npm run lint");
		expect(scripts.test).toBeDefined();
		expect(scripts.test?.run).toBe("npm test");
	});

	it("parses the tools section with custom tool definitions", () => {
		const yaml = `version: 1
tools:
  - name: "project_lint"
    description: "Run the project linter"
    run: "npm run lint 2>&1"
`;
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), yaml);
		const result = loadPluginConfig(tempDir);
		expect(result.loaded).toBe(true);
		const tools = (result.config as PluginConfig).tools as NonNullable<
			PluginConfig["tools"]
		>;
		expect(Array.isArray(tools)).toBe(true);
		expect(tools.length).toBeGreaterThanOrEqual(1);
		expect(tools[0].name).toBe("project_lint");
		expect(tools[0].run).toBe("npm run lint 2>&1");
	});

	// ── REQ-PLG-401: watches top-level section ──────────────────────────────
	// Spec example (specs/70-OpenCode-Plugin.md lines 692-707):
	//   watches:
	//     - name: "spec-change-guard"
	//       trigger: "tool.write"
	//       pattern: "specs/*.md"
	//       run: "echo 'Spec changed — remember to update tests'"
	//       description: "Remind about test updates when specs change"
	//     - name: "security-file-guard"
	//       trigger: "tool.read"
	//       pattern: "*.env*"
	//       action: "block"
	//       message: "Reading .env files is blocked by security policy"
	//     - name: "large-file-warning"
	//       trigger: "tool.write"
	//       condition: "file_size > 10000"
	//       run: "echo 'Warning: large file write detected'"

	it("parses the watches section as an array of watch entries", () => {
		const yaml = `version: 1
watches:
  - name: "spec-change-guard"
    trigger: "tool.write"
    pattern: "specs/*.md"
    run: "echo 'Spec changed'"
    description: "Remind about test updates when specs change"
`;
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), yaml);
		const result = loadPluginConfig(tempDir);
		expect(result.loaded).toBe(true);
		const watches = (result.config as PluginConfig).watches as NonNullable<
			PluginConfig["watches"]
		>;
		expect(Array.isArray(watches)).toBe(true);
		expect(watches.length).toBe(1);
		expect(watches[0].name).toBe("spec-change-guard");
		expect(watches[0].trigger).toBe("tool.write");
		expect(watches[0].pattern).toBe("specs/*.md");
		expect(watches[0].run).toBe("echo 'Spec changed'");
		expect(watches[0].description).toBe(
			"Remind about test updates when specs change",
		);
	});

	it("parses a watch entry with action and message fields (block mode)", () => {
		const yaml = `version: 1
watches:
  - name: "security-file-guard"
    trigger: "tool.read"
    pattern: "*.env*"
    action: "block"
    message: "Reading .env files is blocked by security policy"
`;
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), yaml);
		const result = loadPluginConfig(tempDir);
		expect(result.loaded).toBe(true);
		const watches = (result.config as PluginConfig).watches as NonNullable<
			PluginConfig["watches"]
		>;
		expect(watches.length).toBe(1);
		expect(watches[0].name).toBe("security-file-guard");
		expect(watches[0].trigger).toBe("tool.read");
		expect(watches[0].pattern).toBe("*.env*");
		expect(watches[0].action).toBe("block");
		expect(watches[0].message).toBe(
			"Reading .env files is blocked by security policy",
		);
	});

	it("parses a watch entry with condition field (no pattern)", () => {
		const yaml = `version: 1
watches:
  - name: "large-file-warning"
    trigger: "tool.write"
    condition: "file_size > 10000"
    run: "echo 'Warning: large file write detected'"
`;
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), yaml);
		const result = loadPluginConfig(tempDir);
		expect(result.loaded).toBe(true);
		const watches = (result.config as PluginConfig).watches as NonNullable<
			PluginConfig["watches"]
		>;
		expect(watches.length).toBe(1);
		expect(watches[0].name).toBe("large-file-warning");
		expect(watches[0].trigger).toBe("tool.write");
		expect(watches[0].condition).toBe("file_size > 10000");
		expect(watches[0].run).toBe("echo 'Warning: large file write detected'");
		// pattern is optional when condition is used
		expect(watches[0].pattern).toBeUndefined();
	});

	it("parses multiple watch entries in the same config", () => {
		const yaml = `version: 1
watches:
  - name: "spec-change-guard"
    trigger: "tool.write"
    pattern: "specs/*.md"
    run: "echo 'Spec changed'"
    description: "Remind about test updates"
  - name: "security-file-guard"
    trigger: "tool.read"
    pattern: "*.env*"
    action: "block"
    message: "Blocked by security policy"
  - name: "large-file-warning"
    trigger: "tool.write"
    condition: "file_size > 10000"
    run: "echo 'Large file'"
`;
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), yaml);
		const result = loadPluginConfig(tempDir);
		expect(result.loaded).toBe(true);
		const watches = (result.config as PluginConfig).watches as NonNullable<
			PluginConfig["watches"]
		>;
		expect(Array.isArray(watches)).toBe(true);
		expect(watches.length).toBe(3);
		expect(watches[0].name).toBe("spec-change-guard");
		expect(watches[1].name).toBe("security-file-guard");
		expect(watches[2].name).toBe("large-file-warning");
	});

	it("handles config with empty watches array", () => {
		const yaml = `version: 1
watches: []
`;
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), yaml);
		const result = loadPluginConfig(tempDir);
		expect(result.loaded).toBe(true);
		const watches = (result.config as PluginConfig).watches as NonNullable<
			PluginConfig["watches"]
		>;
		expect(Array.isArray(watches)).toBe(true);
		expect(watches.length).toBe(0);
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-402: Validate on load — invalid config -> conversational fallback
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-402: invalid config produces conversational error and falls back to defaults", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("returns error with conversational message for malformed YAML", () => {
		// Completely broken YAML
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), "{{{{not yaml at all");
		const result = loadPluginConfig(tempDir);
		expect(result.loaded).toBe(false);
		expect(result.error).toBeDefined();
		expect(typeof result.error).toBe("string");
		// Error should be conversational, not a raw stack trace
		expect(result.error?.length).toBeGreaterThan(10);
		expect(result.error?.length).toBeLessThan(500);
	});

	it("returns error with conversational message for malformed JSON", () => {
		writeFile(join(tempDir, ".axiom", "plugin.json"), "{not json}");
		const result = loadPluginConfig(tempDir);
		expect(result.loaded).toBe(false);
		expect(result.error).toBeDefined();
		expect(typeof result.error).toBe("string");
	});

	it("falls back to defaults when config is invalid (L5/L6 disabled)", () => {
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), "{{{{broken");
		const result = loadPluginConfig(tempDir);
		// When invalid, the result should indicate fallback to defaults
		expect(result.loaded).toBe(false);
		// No config object returned — caller uses defaults
		expect(result.config).toBeUndefined();
	});

	it("validatePluginConfig rejects config missing version field", () => {
		const config = { layers: { metrics: true } } as unknown as PluginConfig;
		const validation = validatePluginConfig(config);
		expect(validation.valid).toBe(false);
		expect(validation.errors).toBeDefined();
		expect(validation.errors?.length).toBeGreaterThan(0);
	});

	it("validatePluginConfig rejects config with non-numeric version", () => {
		const config = { version: "one" } as unknown as PluginConfig;
		const validation = validatePluginConfig(config);
		expect(validation.valid).toBe(false);
		expect(validation.errors).toBeDefined();
	});

	it("validatePluginConfig accepts minimal valid config (version: 1 only)", () => {
		const config: PluginConfig = { version: 1 };
		const validation = validatePluginConfig(config);
		expect(validation.valid).toBe(true);
		expect(validation.errors).toBeUndefined();
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-403: version: 1 for forward compat; unknown version -> warning
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-403: version gating and forward compatibility", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("accepts version: 1 without warnings", () => {
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), "version: 1\n");
		const result = loadPluginConfig(tempDir);
		expect(result.loaded).toBe(true);
		expect(result.warning).toBeUndefined();
		expect(result.config?.version).toBe(1);
	});

	it("produces a warning for unknown version (e.g., version: 99)", () => {
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), "version: 99\n");
		const result = loadPluginConfig(tempDir);
		// REQ-PLG-403: unknown versions produce a warning and fall back to defaults
		expect(result.warning).toBeDefined();
		expect(typeof result.warning).toBe("string");
		expect(result.warning as string).toContain("99");
	});

	it("falls back to defaults for unknown version (L5/L6 disabled)", () => {
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), "version: 99\n");
		const result = loadPluginConfig(tempDir);
		// Config is loaded but with defaults applied (unknown version fallback)
		// The loaded flag may be true (file was read) but the config should be defaults
		expect(result.loaded).toBe(true);
		expect(result.config).toBeDefined();
		// Layers should be defaults (L5/L6 disabled)
		expect(result.config?.layers?.automation).toBeFalsy();
		expect(result.config?.layers?.tools).toBeFalsy();
	});

	it("produces a warning for version: 0 (below minimum)", () => {
		writeFile(join(tempDir, ".axiom", "plugin.yaml"), "version: 0\n");
		const result = loadPluginConfig(tempDir);
		expect(result.warning).toBeDefined();
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// DEFAULT_PLUGIN_CONFIG export — callers need a known-good default shape
// ═══════════════════════════════════════════════════════════════════════════
describe("DEFAULT_PLUGIN_CONFIG export", () => {
	it("exports a DEFAULT_PLUGIN_CONFIG object", () => {
		expect(DEFAULT_PLUGIN_CONFIG).toBeDefined();
		expect(typeof DEFAULT_PLUGIN_CONFIG).toBe("object");
	});

	it("default config has version: 1", () => {
		expect(DEFAULT_PLUGIN_CONFIG.version).toBe(1);
	});

	it("default config has L5 automation disabled", () => {
		expect(DEFAULT_PLUGIN_CONFIG.layers?.automation).toBeFalsy();
	});

	it("default config has L6 tools disabled", () => {
		expect(DEFAULT_PLUGIN_CONFIG.layers?.tools).toBeFalsy();
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-400a: Profile-based config scoping
// ═══════════════════════════════════════════════════════════════════════════

import { resolveActiveProfile } from "../src/config-loader";

describe("REQ-PLG-400a: profile-based config scoping", () => {
	it("returns base config when AXIOM_PROFILE is not set", () => {
		const config = {
			version: 1,
			hooks: { on_session_start: [{ name: "base", run: "echo base" }] },
		};
		const result = resolveActiveProfile(config, {});
		expect(result.profileName).toBeNull();
		expect(result.config).toBe(config);
	});

	it("merges profile hooks when AXIOM_PROFILE matches", () => {
		const config = {
			version: 1,
			hooks: { on_session_start: [{ name: "base-hook", run: "echo base" }] },
			profiles: {
				local: {
					hooks: {
						on_session_start: [{ name: "local-hook", run: "echo local" }],
					},
				},
			},
		};
		const result = resolveActiveProfile(config, { AXIOM_PROFILE: "local" });
		expect(result.profileName).toBe("local");
		expect(result.config.hooks?.on_session_start?.length).toBe(2);
		expect(result.config.hooks?.on_session_start?.[0].name).toBe("base-hook");
		expect(result.config.hooks?.on_session_start?.[1].name).toBe("local-hook");
	});

	it("returns warning for unknown profile and falls back to default", () => {
		const config = { version: 1, profiles: { ci: {} } };
		const result = resolveActiveProfile(config, {
			AXIOM_PROFILE: "nonexistent",
		});
		expect(result.profileName).toBe("nonexistent");
		expect(result.warning).toBeDefined();
		expect(result.warning).toContain("nonexistent");
		expect(result.config).toBe(config);
	});

	it("profile scripts are merged with base scripts", () => {
		const config = {
			version: 1,
			scripts: { lint: { run: "npm run lint" } },
			profiles: {
				ci: { scripts: { test: { run: "npm test" } } },
			},
		};
		const result = resolveActiveProfile(config, { AXIOM_PROFILE: "ci" });
		expect(result.config.scripts?.lint).toBeDefined();
		expect(result.config.scripts?.test).toBeDefined();
	});

	it("profile watches are appended to base watches", () => {
		const config = {
			version: 1,
			watches: [
				{ name: "base-watch", trigger: "tool.write" as const, pattern: "*.ts" },
			],
			profiles: {
				dev: {
					watches: [
						{
							name: "dev-watch",
							trigger: "tool.write" as const,
							pattern: "*.md",
						},
					],
				},
			},
		};
		const result = resolveActiveProfile(config, { AXIOM_PROFILE: "dev" });
		expect(result.config.watches?.length).toBe(2);
	});
});
