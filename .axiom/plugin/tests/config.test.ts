/**
 * Integration configuration tests — verifies /axiom-setup Jira project key
 * prompt, validation, skip, persistence, and idempotent behavior per
 * REQ-PLG-046 and REQ-PLG-047.
 *
 * These tests are written BEFORE the config module implementation exists.
 * They MUST fail initially and drive the implementation.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-046, #REQ-PLG-047
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-046 plan=phase-70-1/task-70-1-8/step-70-1-8-1
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import {
	existsSync,
	mkdirSync,
	readFileSync,
	rmSync,
	writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { CodeOpsConfig } from "../src/config";
// ── Config module import (will fail until implemented) ──────────────────
// REQ-PLG-046: scaffold .axiom/axiom.config.yaml
// REQ-PLG-047: integration configuration step
import {
	configureGithubSettings,
	configureJiraProjectKey,
	readConfigYaml,
	validateJiraProjectKey,
	writeConfigYaml,
} from "../src/config";

/** Create a unique temp directory for each test */
function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-config-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
	);
	mkdirSync(dir, { recursive: true });
	return dir;
}

/** Helper to create a file with content */
function touch(filePath: string, content = ""): void {
	mkdirSync(join(filePath, ".."), { recursive: true });
	writeFileSync(filePath, content);
}

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-047 §2: Jira project key validation
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-047 §2: Jira project key validation", () => {
	it("accepts valid uppercase alphabetic keys (2-10 chars)", () => {
		// Standard Jira project key format: ^[A-Z][A-Z0-9_]{1,9}$
		expect(validateJiraProjectKey("AB")).toBe(true);
		expect(validateJiraProjectKey("PROJ")).toBe(true);
		expect(validateJiraProjectKey("BACKEND")).toBe(true);
		expect(validateJiraProjectKey("FE")).toBe(true);
		expect(validateJiraProjectKey("ABCDEFGHIJ")).toBe(true); // 10 chars max
	});

	it("accepts keys with digits after the first character", () => {
		expect(validateJiraProjectKey("A1")).toBe(true);
		expect(validateJiraProjectKey("PROJ123")).toBe(true);
		expect(validateJiraProjectKey("BE2")).toBe(true);
	});

	it("accepts keys with underscores after the first character", () => {
		expect(validateJiraProjectKey("A_B")).toBe(true);
		expect(validateJiraProjectKey("MY_PROJ")).toBe(true);
	});

	it("rejects single-character keys", () => {
		expect(validateJiraProjectKey("A")).toBe(false);
	});

	it("rejects keys longer than 10 characters", () => {
		expect(validateJiraProjectKey("ABCDEFGHIJK")).toBe(false); // 11 chars
	});

	it("rejects lowercase keys", () => {
		expect(validateJiraProjectKey("proj")).toBe(false);
		expect(validateJiraProjectKey("Proj")).toBe(false); // mixed case
	});

	it("rejects keys starting with a digit", () => {
		expect(validateJiraProjectKey("1PROJ")).toBe(false);
	});

	it("rejects empty string", () => {
		expect(validateJiraProjectKey("")).toBe(false);
	});

	it("rejects keys with spaces or special characters", () => {
		expect(validateJiraProjectKey("MY PROJ")).toBe(false);
		expect(validateJiraProjectKey("MY-PROJ")).toBe(false);
		expect(validateJiraProjectKey("MY.PROJ")).toBe(false);
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-046: Scaffold .axiom/axiom.config.yaml
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-046: scaffold axiom.config.yaml", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("configureJiraProjectKey() creates .axiom/axiom.config.yaml with provided key", async () => {
		const result = await configureJiraProjectKey(tempDir, "PROJ");
		expect(result).toBeDefined();
		expect(result.configured).toBe(true);
		expect(result.projectKey).toBe("PROJ");

		// File must exist
		const configPath = join(tempDir, ".axiom", "axiom.config.yaml");
		expect(existsSync(configPath)).toBe(true);

		// File must contain the Jira project key
		const content = readFileSync(configPath, "utf-8");
		expect(content).toContain("project_key");
		expect(content).toContain("PROJ");
	});

	it("configureJiraProjectKey() with null key writes null to config", async () => {
		// REQ-PLG-047 §3: If the user skips, leave jira.project_key: null
		const result = await configureJiraProjectKey(tempDir, null);
		expect(result).toBeDefined();
		expect(result.configured).toBe(false);
		expect(result.projectKey).toBeNull();

		const configPath = join(tempDir, ".axiom", "axiom.config.yaml");
		expect(existsSync(configPath)).toBe(true);

		const content = readFileSync(configPath, "utf-8");
		expect(content).toContain("project_key");
		// Must contain null (not empty string, not missing)
		expect(content).toMatch(/project_key:\s*null/);
	});

	it("config includes repo_id auto-derived field", async () => {
		// REQ-PLG-046: repo_id auto-derived from git remote
		await configureJiraProjectKey(tempDir, "PROJ");

		const configPath = join(tempDir, ".axiom", "axiom.config.yaml");
		const content = readFileSync(configPath, "utf-8");
		expect(content).toContain("repo_id");
	});

	it("config includes default confidence weights and thresholds", async () => {
		// REQ-PLG-046: Default confidence weights and thresholds
		await configureJiraProjectKey(tempDir, "PROJ");

		const configPath = join(tempDir, ".axiom", "axiom.config.yaml");
		const content = readFileSync(configPath, "utf-8");
		expect(content).toContain("confidence");
		expect(content).toContain("weights");
		expect(content).toContain("thresholds");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-046: Idempotent config — do not overwrite existing
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-046: idempotent config (no overwrite)", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("does NOT overwrite existing .axiom/axiom.config.yaml", async () => {
		// Pre-create a config with user content
		const configPath = join(tempDir, ".axiom", "axiom.config.yaml");
		const userContent = `# My custom config
repo_id: "myorg/myrepo"
jira:
  project_key: CUSTOM
custom_setting: true
`;
		touch(configPath, userContent);

		// configureJiraProjectKey must NOT overwrite
		const result = await configureJiraProjectKey(tempDir, "NEWKEY");
		expect(result.skipped).toBe(true);

		const afterContent = readFileSync(configPath, "utf-8");
		expect(afterContent).toBe(userContent);
	});

	it("offers to set jira.project_key when existing config has null key", async () => {
		// REQ-PLG-046: check whether jira.project_key is null and offer to set it
		const configPath = join(tempDir, ".axiom", "axiom.config.yaml");
		const existingContent = `repo_id: "myorg/myrepo"
jira:
  project_key: null
`;
		touch(configPath, existingContent);

		// When existing config has null key and user provides a new key,
		// it should update ONLY the jira.project_key field
		const result = await configureJiraProjectKey(tempDir, "NEWKEY");
		expect(result.configured).toBe(true);
		expect(result.projectKey).toBe("NEWKEY");
		expect(result.updatedExisting).toBe(true);

		const afterContent = readFileSync(configPath, "utf-8");
		expect(afterContent).toContain("NEWKEY");
		// Must preserve other settings
		expect(afterContent).toContain("repo_id");
	});

	it("does NOT update jira.project_key when existing config already has a non-null key", async () => {
		const configPath = join(tempDir, ".axiom", "axiom.config.yaml");
		const existingContent = `repo_id: "myorg/myrepo"
jira:
  project_key: EXISTING
`;
		touch(configPath, existingContent);

		// Must not overwrite an existing non-null key
		const result = await configureJiraProjectKey(tempDir, "NEWKEY");
		expect(result.skipped).toBe(true);

		const afterContent = readFileSync(configPath, "utf-8");
		expect(afterContent).toContain("EXISTING");
		expect(afterContent).not.toContain("NEWKEY");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-047: Integration configuration step behavior
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-047: integration configuration step", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("rejects invalid Jira project key and returns validation error", async () => {
		// REQ-PLG-047 §2: validate against ^[A-Z][A-Z0-9_]{1,9}$
		const result = await configureJiraProjectKey(tempDir, "invalid-key");
		expect(result.configured).toBe(false);
		expect(result.validationError).toBeDefined();
		expect(typeof result.validationError).toBe("string");

		// Config file should NOT be created with invalid key
		const configPath = join(tempDir, ".axiom", "axiom.config.yaml");
		// Either file doesn't exist, or if it does, it has null key
		if (existsSync(configPath)) {
			const content = readFileSync(configPath, "utf-8");
			expect(content).not.toContain("invalid-key");
		}
	});

	it("skippable: null key means user skipped Jira configuration", async () => {
		// REQ-PLG-047 §3 + §6: skippable — user can skip
		const result = await configureJiraProjectKey(tempDir, null);
		expect(result.configured).toBe(false);
		expect(result.projectKey).toBeNull();
		// No validation error — skip is intentional
		expect(result.validationError).toBeUndefined();
	});

	it("persists key to .axiom/axiom.config.yaml under jira.project_key", async () => {
		// REQ-PLG-047 §2: write to .axiom/axiom.config.yaml under jira.project_key
		await configureJiraProjectKey(tempDir, "BACKEND");

		const configPath = join(tempDir, ".axiom", "axiom.config.yaml");
		expect(existsSync(configPath)).toBe(true);

		const content = readFileSync(configPath, "utf-8");
		// Must be under jira: section with project_key field
		expect(content).toMatch(/jira:\s*\n\s+project_key:\s*BACKEND/);
	});

	it("does not overwrite other integration settings when updating null key", async () => {
		// REQ-PLG-046: must not overwrite other integration settings
		const configPath = join(tempDir, ".axiom", "axiom.config.yaml");
		const existingContent = `repo_id: "myorg/myrepo"
jira:
  project_key: null
github:
  default_base_branch: develop
custom:
  my_setting: 42
`;
		touch(configPath, existingContent);

		await configureJiraProjectKey(tempDir, "FE");

		const afterContent = readFileSync(configPath, "utf-8");
		// Jira key updated
		expect(afterContent).toContain("FE");
		// Other settings preserved
		expect(afterContent).toContain("default_base_branch");
		expect(afterContent).toContain("develop");
		expect(afterContent).toContain("my_setting");
		expect(afterContent).toContain("42");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-047 §4: Multi-repo workspace — each repo gets its own config
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-047 §4: multi-repo workspace", () => {
	let workspaceDir: string;
	let repoA: string;
	let repoB: string;

	beforeEach(() => {
		workspaceDir = makeTempDir();
		repoA = join(workspaceDir, "frontend");
		repoB = join(workspaceDir, "backend");
		mkdirSync(repoA, { recursive: true });
		mkdirSync(repoB, { recursive: true });
	});

	afterEach(() => {
		if (existsSync(workspaceDir)) {
			rmSync(workspaceDir, { recursive: true, force: true });
		}
	});

	it("different repos can have different Jira project keys", async () => {
		// REQ-PLG-047 §4: Different repos MAY use different Jira projects
		await configureJiraProjectKey(repoA, "FE");
		await configureJiraProjectKey(repoB, "BE");

		const configA = readFileSync(
			join(repoA, ".axiom", "axiom.config.yaml"),
			"utf-8",
		);
		const configB = readFileSync(
			join(repoB, ".axiom", "axiom.config.yaml"),
			"utf-8",
		);

		expect(configA).toContain("FE");
		expect(configA).not.toContain("BE");
		expect(configB).toContain("BE");
		expect(configB).not.toContain("FE");
	});

	it("each repo config is independent (updating one does not affect the other)", async () => {
		await configureJiraProjectKey(repoA, "FE");
		await configureJiraProjectKey(repoB, "BE");

		// Update repo A's config to null
		// First, manually set it to null to simulate re-setup
		const configPathA = join(repoA, ".axiom", "axiom.config.yaml");
		const contentA = readFileSync(configPathA, "utf-8");
		writeFileSync(configPathA, contentA.replace("FE", "null"));

		// Now update repo A with a new key
		const result = await configureJiraProjectKey(repoA, "FRONTEND");
		expect(result.configured).toBe(true);

		// Repo B must be unchanged
		const configB = readFileSync(
			join(repoB, ".axiom", "axiom.config.yaml"),
			"utf-8",
		);
		expect(configB).toContain("BE");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// readConfigYaml / writeConfigYaml helpers
// ═══════════════════════════════════════════════════════════════════════════
describe("config YAML read/write helpers", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("readConfigYaml returns null when config does not exist", () => {
		const config = readConfigYaml(tempDir);
		expect(config).toBeNull();
	});

	it("readConfigYaml parses existing config", () => {
		const configPath = join(tempDir, ".axiom", "axiom.config.yaml");
		touch(
			configPath,
			`repo_id: "myorg/myrepo"
jira:
  project_key: PROJ
`,
		);

		const config = readConfigYaml(tempDir);
		expect(config).toBeDefined();
		expect(config?.repo_id).toBe("myorg/myrepo");
		expect(config?.jira?.project_key).toBe("PROJ");
	});

	it("writeConfigYaml creates .axiom/ directory if missing", () => {
		const config: CodeOpsConfig = {
			repo_id: "myorg/myrepo",
			jira: { project_key: "TEST" },
		};
		writeConfigYaml(tempDir, config);

		expect(existsSync(join(tempDir, ".axiom"))).toBe(true);
		expect(existsSync(join(tempDir, ".axiom", "axiom.config.yaml"))).toBe(
			true,
		);
	});

	it("writeConfigYaml round-trips through readConfigYaml", () => {
		const config: CodeOpsConfig = {
			repo_id: "myorg/myrepo",
			jira: { project_key: "ROUND" },
		};
		writeConfigYaml(tempDir, config);

		const read = readConfigYaml(tempDir);
		expect(read).toBeDefined();
		expect(read?.repo_id).toBe("myorg/myrepo");
		expect(read?.jira?.project_key).toBe("ROUND");
	});
});

// ═══════════════════════════════════════════════════════════════════════════
// REQ-PLG-047 §5: GitHub default_base_branch configuration
// ═══════════════════════════════════════════════════════════════════════════
describe("REQ-PLG-047 §5: GitHub default_base_branch configuration", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("configureGithubSettings() offers to set default_base_branch when branch differs from main", async () => {
		// REQ-PLG-047 §5: MUST check for GitHub settings and offer to configure
		// github.default_base_branch if it differs from "main"
		const result = await configureGithubSettings(tempDir, {
			defaultBranch: "develop",
		});
		expect(result).toBeDefined();
		expect(result.configured).toBe(true);
		expect(result.defaultBaseBranch).toBe("develop");

		const configPath = join(tempDir, ".axiom", "axiom.config.yaml");
		expect(existsSync(configPath)).toBe(true);

		const content = readFileSync(configPath, "utf-8");
		expect(content).toContain("github");
		expect(content).toContain("default_base_branch");
		expect(content).toContain("develop");
	});

	it("configureGithubSettings() does NOT offer when branch is main (default)", async () => {
		// When the branch is "main", no configuration is needed
		const result = await configureGithubSettings(tempDir, {
			defaultBranch: "main",
		});
		expect(result).toBeDefined();
		expect(result.configured).toBe(false);
		expect(result.skipped).toBe(true);
		expect(result.reason).toContain("main");
	});

	it("configureGithubSettings() writes github section to existing config without overwriting other fields", async () => {
		// Pre-create a config with Jira settings
		const configPath = join(tempDir, ".axiom", "axiom.config.yaml");
		const existingContent = `repo_id: "myorg/myrepo"
jira:
  project_key: PROJ
`;
		touch(configPath, existingContent);

		const result = await configureGithubSettings(tempDir, {
			defaultBranch: "develop",
		});
		expect(result.configured).toBe(true);

		const afterContent = readFileSync(configPath, "utf-8");
		// GitHub settings added
		expect(afterContent).toContain("default_base_branch");
		expect(afterContent).toContain("develop");
		// Jira settings preserved
		expect(afterContent).toContain("project_key");
		expect(afterContent).toContain("PROJ");
		expect(afterContent).toContain("repo_id");
	});

	it("configureGithubSettings() does NOT overwrite existing github.default_base_branch", async () => {
		// Pre-create a config with existing GitHub settings
		const configPath = join(tempDir, ".axiom", "axiom.config.yaml");
		const existingContent = `repo_id: "myorg/myrepo"
jira:
  project_key: PROJ
github:
  default_base_branch: master
`;
		touch(configPath, existingContent);

		const result = await configureGithubSettings(tempDir, {
			defaultBranch: "develop",
		});
		expect(result.configured).toBe(false);
		expect(result.skipped).toBe(true);

		const afterContent = readFileSync(configPath, "utf-8");
		// Original value preserved
		expect(afterContent).toContain("master");
		expect(afterContent).not.toContain("develop");
	});

	it("configureGithubSettings() with null defaultBranch skips configuration", async () => {
		// User skips GitHub configuration entirely
		const result = await configureGithubSettings(tempDir, {
			defaultBranch: null,
		});
		expect(result.configured).toBe(false);
		expect(result.skipped).toBe(true);
	});
});
