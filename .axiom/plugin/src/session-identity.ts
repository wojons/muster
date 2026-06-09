/**
 * Session identity chain — resolves a stable identity for cost attribution.
 *
 * REQ-PLG-104a: Session identity chain: git remote → hostname → UUID → custom.
 * Precedence: user-configured custom > git remote URL > machine hostname > stable UUID.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-104a
 * axiom:trace work_item=plugin-v2-enhancements-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-104a plan=phase-70-9/task-70-9-1/step-70-9-1-1
 * axiom:trace work_item=plugin-metrics-fixes-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-124,REQ-PLG-133 plan=phase-70-l2-repo-hostname-prefix jira_ref=DEX-419
 */

import { execSync } from "node:child_process";
import { randomUUID } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { hostname } from "node:os";
import { join } from "node:path";

/** Identity resolution mode. */
export type IdentityMode = "auto" | "git" | "hostname" | "custom";

/** Options for resolveSessionIdentity(). */
export interface SessionIdentityOptions {
	repoRoot: string;
	mode?: IdentityMode;
	customIdentity?: string;
}

/** Result of resolveSessionIdentity(). */
export interface SessionIdentityResult {
	identity: string;
	source: "custom" | "git" | "hostname" | "uuid";
}

/**
 * Resolve a stable session identity for cost attribution.
 *
 * REQ-PLG-104a: Precedence chain:
 * 1. custom — user-configured value (wins over all auto-derived)
 * 2. git — extract owner/repo from git remote origin URL
 * 3. hostname — machine hostname
 * 4. uuid — stable UUID persisted to .axiom/.identity
 */
export function resolveSessionIdentity(
	options: SessionIdentityOptions,
): SessionIdentityResult {
	const { repoRoot, mode = "auto", customIdentity } = options;

	// 1. Custom always wins
	if (customIdentity?.trim()) {
		return { identity: customIdentity.trim(), source: "custom" };
	}
	if (mode === "custom") {
		// custom mode with no value — fall through to auto
	}

	// 2. Git remote URL (auto or git mode)
	if (mode === "auto" || mode === "git") {
		const gitIdentity = _resolveGitIdentity(repoRoot);
		if (gitIdentity) {
			return { identity: gitIdentity, source: "git" };
		}
		if (mode === "git") {
			// git mode but no remote — fall through to uuid
		}
	}

	// 3. Hostname (auto or hostname mode)
	// REQ-PLG-124 + REQ-PLG-133: fallback MUST be "hostname:<name>" (prefixed), not raw hostname.
	// The prefix makes the value self-describing and distinguishable from a git remote
	// (e.g., "github.com/org/repo"). A raw hostname like "mymachine" is ambiguous.
	if (mode === "auto" || mode === "hostname") {
		const host = hostname();
		if (host?.trim()) {
			return { identity: `hostname:${host.trim()}`, source: "hostname" };
		}
		if (mode === "hostname") {
			// hostname mode but empty — fall through to uuid
		}
	}

	// 4. Stable UUID (last resort)
	const uuid = _resolveOrCreateUuid(repoRoot);
	return { identity: uuid, source: "uuid" };
}

/** Extract owner/repo from git remote origin URL. */
function _resolveGitIdentity(repoRoot: string): string | null {
	try {
		const raw = execSync("git remote get-url origin", {
			cwd: repoRoot,
			encoding: "utf-8",
			timeout: 2000,
			stdio: ["ignore", "pipe", "ignore"],
		}).trim();

		if (!raw) return null;

		// Parse various git URL formats:
		// https://github.com/org/repo.git → github.com/org/repo
		// git@github.com:org/repo.git → github.com/org/repo
		// ssh://git@github.com/org/repo → github.com/org/repo
		const httpsMatch = raw.match(/https?:\/\/([^/]+)\/(.+?)(?:\.git)?$/);
		if (httpsMatch) {
			return `${httpsMatch[1]}/${httpsMatch[2]}`;
		}

		const sshMatch = raw.match(/git@([^:]+):(.+?)(?:\.git)?$/);
		if (sshMatch) {
			return `${sshMatch[1]}/${sshMatch[2]}`;
		}

		const sshUrlMatch = raw.match(/ssh:\/\/[^@]+@([^/]+)\/(.+?)(?:\.git)?$/);
		if (sshUrlMatch) {
			return `${sshUrlMatch[1]}/${sshUrlMatch[2]}`;
		}

		return null;
	} catch {
		return null;
	}
}

/** Load or create a stable UUID persisted to .axiom/.identity. */
function _resolveOrCreateUuid(repoRoot: string): string {
	const identityPath = join(repoRoot, ".axiom", ".identity");

	if (existsSync(identityPath)) {
		const stored = readFileSync(identityPath, "utf-8").trim();
		if (stored && stored.length > 0) {
			return stored;
		}
	}

	// Create a new UUID and persist it
	const uuid = randomUUID();
	try {
		mkdirSync(join(repoRoot, ".axiom"), { recursive: true });
		writeFileSync(identityPath, uuid, "utf-8");
	} catch {
		// If we can't persist, return the UUID anyway (ephemeral)
	}
	return uuid;
}
