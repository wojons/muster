/**
 * Template inventory and parameterization tests — proves the plugin exposes
 * the required scaffold asset inventory, renders repo-specific placeholders
 * correctly, skips existing files, and records template version metadata.
 *
 * REQ-PLG-040: Embedded scaffold template inventory
 * REQ-PLG-041: Parameterized templates with repo-specific values
 * REQ-PLG-044: Detect existing files and skip during scaffolding
 * REQ-PLG-045: Template versioning and upgrade tracking
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-040, #REQ-PLG-041, #REQ-PLG-044, #REQ-PLG-045
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md plan=phase-70-1/task-70-1-6/step-70-1-6-1
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import {
	mkdirSync,
	mkdtempSync,
	readFileSync,
	rmSync,
	writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// This import will fail until the templates module is implemented
import {
	getTemplateManifest,
	renderTemplate,
	TEMPLATE_INVENTORY,
	TEMPLATE_VERSION,
	type TemplateEntry,
	type TemplateParams,
} from "../src/templates";

// ── Test helpers ────────────────────────────────────────────────────────

function makeTempRepo(): string {
	return mkdtempSync(join(tmpdir(), "axiom-template-test-"));
}

function cleanupTempRepo(dir: string): void {
	try {
		rmSync(dir, { recursive: true, force: true });
	} catch {
		// best-effort cleanup
	}
}

// ── REQ-PLG-040: Template inventory ─────────────────────────────────────

describe("REQ-PLG-040: Scaffold template inventory", () => {
	it("TEMPLATE_INVENTORY contains opencode.jsonc", () => {
		const entry = TEMPLATE_INVENTORY.find(
			(t: TemplateEntry) => t.path === "opencode.jsonc",
		);
		expect(entry).toBeDefined();
		expect(entry?.path).toBe("opencode.jsonc");
	});

	it("TEMPLATE_INVENTORY contains .opencode/agents/ entries", () => {
		const agentEntries = TEMPLATE_INVENTORY.filter((t: TemplateEntry) =>
			t.path.startsWith(".opencode/agents/"),
		);
		// At minimum: tower, dev, qa, specwriter
		expect(agentEntries.length).toBeGreaterThanOrEqual(4);
		const names = agentEntries.map((e: TemplateEntry) => e.path);
		expect(names.some((n: string) => n.includes("tower"))).toBe(true);
		expect(names.some((n: string) => n.includes("dev"))).toBe(true);
		expect(names.some((n: string) => n.includes("qa"))).toBe(true);
		expect(names.some((n: string) => n.includes("specwriter"))).toBe(true);
	});

	it("TEMPLATE_INVENTORY contains .opencode/commands/ entries", () => {
		const cmdEntries = TEMPLATE_INVENTORY.filter((t: TemplateEntry) =>
			t.path.startsWith(".opencode/commands/"),
		);
		expect(cmdEntries.length).toBeGreaterThanOrEqual(1);
	});

	it("TEMPLATE_INVENTORY contains .opencode/skills/ entries", () => {
		const skillEntries = TEMPLATE_INVENTORY.filter((t: TemplateEntry) =>
			t.path.startsWith(".opencode/skills/"),
		);
		// At minimum: copilot, install, onboarding
		expect(skillEntries.length).toBeGreaterThanOrEqual(3);
		const names = skillEntries.map((e: TemplateEntry) => e.path);
		expect(names.some((n: string) => n.includes("copilot"))).toBe(true);
		expect(names.some((n: string) => n.includes("install"))).toBe(true);
		expect(names.some((n: string) => n.includes("onboarding"))).toBe(true);
	});

	it("TEMPLATE_INVENTORY contains specs/ skeleton entries", () => {
		const specEntries = TEMPLATE_INVENTORY.filter((t: TemplateEntry) =>
			t.path.startsWith("specs/"),
		);
		const names = specEntries.map((e: TemplateEntry) => e.path);
		expect(names).toContain("specs/README.md");
		expect(names).toContain("specs/_prompt.md");
		expect(names).toContain("specs/_index.md");
		expect(names).toContain("specs/00-PRD.md");
	});

	it("TEMPLATE_INVENTORY contains .memory-bank/ skeleton entries", () => {
		const mbEntries = TEMPLATE_INVENTORY.filter((t: TemplateEntry) =>
			t.path.startsWith(".memory-bank/"),
		);
		const names = mbEntries.map((e: TemplateEntry) => e.path);
		expect(names).toContain(".memory-bank/_prompt.md");
		expect(names).toContain(".memory-bank/_index.md");
	});

	it("TEMPLATE_INVENTORY contains .axiom/axiom.config.yaml", () => {
		const entry = TEMPLATE_INVENTORY.find(
			(t: TemplateEntry) => t.path === ".axiom/axiom.config.yaml",
		);
		expect(entry).toBeDefined();
	});

	it("TEMPLATE_INVENTORY contains .axiom/plugin.yaml", () => {
		const entry = TEMPLATE_INVENTORY.find(
			(t: TemplateEntry) => t.path === ".axiom/plugin.yaml",
		);
		expect(entry).toBeDefined();
	});

	it("every template entry has a non-empty content string", () => {
		for (const entry of TEMPLATE_INVENTORY) {
			expect(typeof entry.content).toBe("string");
			expect(entry.content.length).toBeGreaterThan(0);
		}
	});
});

// ── REQ-PLG-041: Parameterized templates ────────────────────────────────

describe("REQ-PLG-041: Template parameterization", () => {
	it("renderTemplate replaces {{repo_name}} with provided value", () => {
		const template = "Project: {{repo_name}}";
		const result = renderTemplate(template, {
			repo_name: "my-app",
		} as TemplateParams);
		expect(result).toBe("Project: my-app");
	});

	it("renderTemplate replaces {{repo_id}} with org/repo format", () => {
		const template = "repo_id: {{repo_id}}";
		const result = renderTemplate(template, {
			repo_id: "acme/my-app",
		} as TemplateParams);
		expect(result).toBe("repo_id: acme/my-app");
	});

	it("renderTemplate replaces {{jira_project_key}} with provided key", () => {
		const template = "jira_project_key: {{jira_project_key}}";
		const result = renderTemplate(template, {
			jira_project_key: "PROJ",
		} as TemplateParams);
		expect(result).toBe("jira_project_key: PROJ");
	});

	it("renderTemplate replaces {{jira_project_key}} with null when not provided", () => {
		const template = "jira_project_key: {{jira_project_key}}";
		const result = renderTemplate(template, {
			jira_project_key: null,
		} as TemplateParams);
		expect(result).toBe("jira_project_key: null");
	});

	it("renderTemplate replaces {{primary_language}} with detected language", () => {
		const template = "language: {{primary_language}}";
		const result = renderTemplate(template, {
			primary_language: "typescript",
		} as TemplateParams);
		expect(result).toBe("language: typescript");
	});

	it("renderTemplate replaces {{framework}} with detected framework", () => {
		const template = "framework: {{framework}}";
		const result = renderTemplate(template, {
			framework: "nextjs",
		} as TemplateParams);
		expect(result).toBe("framework: nextjs");
	});

	it("renderTemplate replaces {{date}} with current date", () => {
		const template = "created: {{date}}";
		const today = new Date().toISOString().slice(0, 10);
		const result = renderTemplate(template, { date: today } as TemplateParams);
		expect(result).toContain(today);
	});

	it("renderTemplate handles multiple placeholders in one template", () => {
		const template =
			"# {{repo_name}}\n\nrepo_id: {{repo_id}}\nlanguage: {{primary_language}}";
		const result = renderTemplate(template, {
			repo_name: "my-app",
			repo_id: "acme/my-app",
			primary_language: "typescript",
		} as TemplateParams);
		expect(result).toContain("my-app");
		expect(result).toContain("acme/my-app");
		expect(result).toContain("typescript");
	});

	it("renderTemplate leaves unknown placeholders untouched", () => {
		const template = "value: {{unknown_param}}";
		const result = renderTemplate(template, {} as TemplateParams);
		expect(result).toBe("value: {{unknown_param}}");
	});

	it("axiom.config.yaml template uses parameterized repo_id and jira_project_key", () => {
		const configEntry = TEMPLATE_INVENTORY.find(
			(t: TemplateEntry) => t.path === ".axiom/axiom.config.yaml",
		);
		expect(configEntry).toBeDefined();
		expect(configEntry?.content).toContain("{{repo_id}}");
		expect(configEntry?.content).toContain("{{jira_project_key}}");
	});
});

// ── REQ-PLG-044: Skip existing files ────────────────────────────────────

describe("REQ-PLG-044: Skip existing files during scaffolding", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempRepo();
	});

	afterEach(() => {
		cleanupTempRepo(tempDir);
	});

	it("getTemplateManifest reports files to create when repo is empty", () => {
		const manifest = getTemplateManifest(tempDir, {} as TemplateParams);
		expect(manifest.toCreate.length).toBeGreaterThan(0);
		expect(manifest.toSkip.length).toBe(0);
	});

	it("getTemplateManifest reports skipped files when they already exist", () => {
		// Pre-create opencode.jsonc
		writeFileSync(join(tempDir, "opencode.jsonc"), '{ "existing": true }');

		const manifest = getTemplateManifest(tempDir, {} as TemplateParams);
		const skipped = manifest.toSkip.find((s) => s.path === "opencode.jsonc");
		expect(skipped).toBeDefined();
		expect(skipped?.reason).toContain("already exists");
	});

	it("getTemplateManifest does not include skipped files in toCreate", () => {
		// Pre-create specs/README.md
		mkdirSync(join(tempDir, "specs"), { recursive: true });
		writeFileSync(join(tempDir, "specs", "README.md"), "# Existing specs");

		const manifest = getTemplateManifest(tempDir, {} as TemplateParams);
		const inCreate = manifest.toCreate.find(
			(c) => c.path === "specs/README.md",
		);
		expect(inCreate).toBeUndefined();
	});

	it("getTemplateManifest preserves user-authored content in existing files", () => {
		const userContent = "# My custom specs README\n\nThis is my project.";
		mkdirSync(join(tempDir, "specs"), { recursive: true });
		writeFileSync(join(tempDir, "specs", "README.md"), userContent);

		getTemplateManifest(tempDir, {} as TemplateParams);

		// Verify the file was not modified
		const afterContent = readFileSync(
			join(tempDir, "specs", "README.md"),
			"utf-8",
		);
		expect(afterContent).toBe(userContent);
	});

	it("getTemplateManifest skips .axiom/axiom.config.yaml when it exists", () => {
		mkdirSync(join(tempDir, ".axiom"), { recursive: true });
		writeFileSync(
			join(tempDir, ".axiom", "axiom.config.yaml"),
			"repo_id: existing",
		);

		const manifest = getTemplateManifest(tempDir, {} as TemplateParams);
		const skipped = manifest.toSkip.find(
			(s) => s.path === ".axiom/axiom.config.yaml",
		);
		expect(skipped).toBeDefined();
	});
});

// ── REQ-PLG-045: Template versioning ────────────────────────────────────

describe("REQ-PLG-045: Template versioning and upgrade tracking", () => {
	it("TEMPLATE_VERSION is a non-empty semver-like string", () => {
		expect(typeof TEMPLATE_VERSION).toBe("string");
		expect(TEMPLATE_VERSION.length).toBeGreaterThan(0);
		// Should match semver pattern (e.g., "1.0.0" or "0.1.0")
		expect(TEMPLATE_VERSION).toMatch(/^\d+\.\d+\.\d+$/);
	});

	it("every template entry includes a version field", () => {
		for (const entry of TEMPLATE_INVENTORY) {
			expect(entry.version).toBe(TEMPLATE_VERSION);
		}
	});

	it("getTemplateManifest includes version metadata in output", () => {
		const tempDir = makeTempRepo();
		try {
			const manifest = getTemplateManifest(tempDir, {} as TemplateParams);
			expect(manifest.templateVersion).toBe(TEMPLATE_VERSION);
		} finally {
			cleanupTempRepo(tempDir);
		}
	});

	it("getTemplateManifest records scaffolded template version for each created file", () => {
		const tempDir = makeTempRepo();
		try {
			const manifest = getTemplateManifest(tempDir, {} as TemplateParams);
			for (const entry of manifest.toCreate) {
				expect(entry.version).toBe(TEMPLATE_VERSION);
			}
		} finally {
			cleanupTempRepo(tempDir);
		}
	});
});
