/**
 * Maturity detection — detects the Axiom maturity level (0-4) of a repository.
 *
 * REQ-PLG-010: On session.created, the plugin MUST detect the repo's Axiom maturity level.
 *
 * | Level | Detection                                                        |
 * |-------|------------------------------------------------------------------|
 * | 0     | No `.opencode/`, no `specs/`, no `.memory-bank/`                 |
 * | 1     | `.opencode/` exists but no `specs/` or empty specs               |
 * | 2     | `specs/` has content but no `.memory-bank/` or no plans          |
 * | 3     | Specs + plans + memory bank exist                                |
 * | 4     | All of above + evidence bundles + CI integration                 |
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-010
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-010 plan=phase-70-1/task-70-1-2/step-70-1-2-1
 */

import { execSync } from "node:child_process";
import { existsSync, readdirSync, statSync } from "node:fs";
import { basename, join } from "node:path";

/** Axiom maturity levels per REQ-PLG-010 */
export enum MaturityLevel {
	/** No `.opencode/`, no `specs/`, no `.memory-bank/` */
	New = 0,
	/** `.opencode/` exists but no usable `specs/` */
	Scaffolded = 1,
	/** `specs/` has content but no `.memory-bank/` or no plans */
	Specced = 2,
	/** Specs + plans + memory bank exist */
	Active = 3,
	/** All of above + evidence bundles + CI integration */
	Mature = 4,
}

/** Human-readable labels for each maturity level */
export const MATURITY_LABELS: Record<MaturityLevel, string> = {
	[MaturityLevel.New]: "New",
	[MaturityLevel.Scaffolded]: "Scaffolded",
	[MaturityLevel.Specced]: "Spec'd",
	[MaturityLevel.Active]: "Active",
	[MaturityLevel.Mature]: "Mature",
};

/** Detected backend availability per REQ-PLG-050 */
export interface BackendInfo {
	/** Python backend (.axiom/ + axiom CLI) available */
	python: boolean;
	/** Go backend (morty CLI) available */
	go: boolean;
	/** Resolved mode: "spec-only" | "python" | "go" | "hybrid" */
	mode: "spec-only" | "python" | "go" | "hybrid";
}

/** Basic repo metadata for codeops_detect JSON output per REQ-PLG-504 */
export interface RepoMetadata {
	/** Directory name of the repo root */
	name: string;
	/** Whether a .git directory exists */
	hasGit: boolean;
	/** Whether a .axiom/ directory exists */
	hasCodeops: boolean;
}

/** Result of maturity detection — includes backends and repo metadata per REQ-PLG-504 */
export interface MaturityResult {
	level: MaturityLevel;
	label: string;
	signals: {
		hasOpencode: boolean;
		hasSpecs: boolean;
		specsHasContent: boolean;
		hasMemoryBank: boolean;
		hasPlans: boolean;
		hasEvidenceBundles: boolean;
		hasCiIntegration: boolean;
	};
	/** Detected backend availability per REQ-PLG-050 */
	backends: BackendInfo;
	/** Basic repo metadata per REQ-PLG-504 */
	repoMetadata: RepoMetadata;
}

/**
 * Check if a directory exists and is non-empty.
 */
function dirExistsAndNonEmpty(dirPath: string): boolean {
	try {
		if (!existsSync(dirPath)) return false;
		const stat = statSync(dirPath);
		if (!stat.isDirectory()) return false;
		const entries = readdirSync(dirPath);
		return entries.length > 0;
	} catch {
		return false;
	}
}

/**
 * Check if specs/ has meaningful content (not just meta files).
 * Meta files: README.md, _prompt.md, _index.md
 */
function specsHasContent(specsDir: string): boolean {
	try {
		if (!existsSync(specsDir)) return false;
		const entries = readdirSync(specsDir);
		const metaFiles = new Set(["readme.md", "_prompt.md", "_index.md"]);
		const contentFiles = entries.filter(
			(e) =>
				e.endsWith(".md") &&
				!metaFiles.has(e.toLowerCase()) &&
				!e.startsWith("_"),
		);
		return contentFiles.length > 0;
	} catch {
		return false;
	}
}

/**
 * Check if implementation plans exist under .memory-bank/.
 * Looks for .memory-bank/implementation-plans/ with at least one P-*.md file,
 * or .memory-bank/work-items/ with at least one work item directory containing plan.yaml.
 */
function hasPlans(repoRoot: string): boolean {
	try {
		const plansDir = join(repoRoot, ".memory-bank", "implementation-plans");
		if (existsSync(plansDir)) {
			const entries = readdirSync(plansDir);
			if (entries.some((e) => e.startsWith("P-") && e.endsWith(".md"))) {
				return true;
			}
		}

		const workItemsDir = join(repoRoot, ".memory-bank", "work-items");
		if (existsSync(workItemsDir)) {
			const entries = readdirSync(workItemsDir);
			for (const entry of entries) {
				if (entry.startsWith("_") || entry.startsWith(".")) continue;
				const planYaml = join(workItemsDir, entry, "plan.yaml");
				if (existsSync(planYaml)) return true;
			}
		}

		return false;
	} catch {
		return false;
	}
}

/**
 * Check if evidence bundles exist.
 * Looks for work-item run directories containing verification.md files.
 */
function hasEvidenceBundles(repoRoot: string): boolean {
	try {
		const workItemsDir = join(repoRoot, ".memory-bank", "work-items");
		if (!existsSync(workItemsDir)) return false;

		const entries = readdirSync(workItemsDir);
		for (const entry of entries) {
			if (entry.startsWith("_") || entry.startsWith(".")) continue;
			const runsDir = join(workItemsDir, entry, "runs");
			if (!existsSync(runsDir)) continue;
			const runs = readdirSync(runsDir);
			for (const run of runs) {
				const verificationMd = join(runsDir, run, "verification.md");
				if (existsSync(verificationMd)) return true;
			}
		}
		return false;
	} catch {
		return false;
	}
}

/**
 * Check if CI integration exists.
 * Looks for .github/workflows/, .gitlab-ci.yml, Jenkinsfile, or .axiom/ci/ presence.
 */
function hasCiIntegration(repoRoot: string): boolean {
	const ciPaths = [
		join(repoRoot, ".github", "workflows"),
		join(repoRoot, ".gitlab-ci.yml"),
		join(repoRoot, "Jenkinsfile"),
		join(repoRoot, ".axiom", "ci"),
	];

	for (const ciPath of ciPaths) {
		try {
			if (existsSync(ciPath)) return true;
		} catch {}
	}
	return false;
}

/**
 * Detect available backends per REQ-PLG-050 and REQ-PLG-051.
 * Checks for Python (.axiom/ + axiom CLI) and Go (morty CLI) backends.
 */
export function detectBackends(repoRoot: string): BackendInfo {
	const hasCodeopsDir = dirExistsAndNonEmpty(join(repoRoot, ".axiom"));

	let hasPythonCli = false;
	try {
		execSync("axiom run --help", { stdio: "ignore", timeout: 1500 });
		hasPythonCli = true;
	} catch {
		hasPythonCli = false;
	}

	let hasGoCli = false;
	try {
		execSync("morty --version", { stdio: "ignore", timeout: 1500 });
		hasGoCli = true;
	} catch {
		hasGoCli = false;
	}

	const python = hasCodeopsDir && hasPythonCli;
	const go = hasGoCli;

	let mode: BackendInfo["mode"];
	if (python && go) {
		mode = "hybrid";
	} else if (python) {
		mode = "python";
	} else if (go) {
		mode = "go";
	} else {
		mode = "spec-only";
	}

	return { python, go, mode };
}

/**
 * Collect basic repo metadata per REQ-PLG-504.
 */
function collectRepoMetadata(repoRoot: string): RepoMetadata {
	return {
		name: basename(repoRoot),
		hasGit: existsSync(join(repoRoot, ".git")),
		hasCodeops: dirExistsAndNonEmpty(join(repoRoot, ".axiom")),
	};
}

/**
 * Detect the Axiom maturity level of a repository.
 *
 * REQ-PLG-010: Maturity detection is re-evaluated on each session start (REQ-PLG-023).
 *
 * @param repoRoot - Absolute path to the repository root directory
 * @returns MaturityResult with level, label, and detection signals
 */
export function detectMaturity(repoRoot: string): MaturityResult {
	const signals = {
		hasOpencode: dirExistsAndNonEmpty(join(repoRoot, ".opencode")),
		hasSpecs: existsSync(join(repoRoot, "specs")),
		specsHasContent: specsHasContent(join(repoRoot, "specs")),
		hasMemoryBank: dirExistsAndNonEmpty(join(repoRoot, ".memory-bank")),
		hasPlans: hasPlans(repoRoot),
		hasEvidenceBundles: hasEvidenceBundles(repoRoot),
		hasCiIntegration: hasCiIntegration(repoRoot),
	};

	let level: MaturityLevel;

	if (!signals.hasOpencode && !signals.hasSpecs && !signals.hasMemoryBank) {
		// Level 0: No .opencode/, no specs/, no .memory-bank/
		level = MaturityLevel.New;
	} else if (!signals.specsHasContent) {
		// Level 1: .opencode/ exists but no usable specs/
		level = MaturityLevel.Scaffolded;
	} else if (!signals.hasMemoryBank || !signals.hasPlans) {
		// Level 2: specs/ has content but no .memory-bank/ or no plans
		level = MaturityLevel.Specced;
	} else if (signals.hasEvidenceBundles && signals.hasCiIntegration) {
		// Level 4: All of above + evidence bundles + CI integration
		level = MaturityLevel.Mature;
	} else {
		// Level 3: Specs + plans + memory bank exist
		level = MaturityLevel.Active;
	}

	return {
		level,
		label: MATURITY_LABELS[level],
		signals,
		backends: detectBackends(repoRoot),
		repoMetadata: collectRepoMetadata(repoRoot),
	};
}
