/**
 * Version compatibility helper for the Axiom plugin.
 *
 * REQ-PLG-005: The plugin MUST check the OpenCode SDK version on startup and
 * emit a non-blocking warning if the version is below the minimum required.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-005
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-005 plan=phase-70-fixes/task-f5/step-f5-1
 */

import { existsSync } from "node:fs";
import { join } from "node:path";
import { homedir } from "node:os";

/**
 * Parse a semver string into [major, minor, patch] integers.
 * Returns null if the string is not a valid semver.
 */
function parseSemver(version: string): [number, number, number] | null {
	// Strip leading "v" if present
	const clean = version.replace(/^v/, "");
	const parts = clean.split(".");
	if (parts.length < 2) return null;
	const major = parseInt(parts[0], 10);
	const minor = parseInt(parts[1], 10);
	// patch may have pre-release suffix — take only the numeric part
	const patchStr = (parts[2] ?? "0").replace(/[^0-9].*$/, "");
	const patch = parseInt(patchStr || "0", 10);
	if (isNaN(major) || isNaN(minor) || isNaN(patch)) return null;
	return [major, minor, patch];
}

/**
 * Returns true if `current` satisfies `>= minimum`.
 *
 * Both arguments must be semver strings (e.g., "1.2.27").
 * Returns false if either string cannot be parsed, so callers should
 * treat a false result as "unknown / possibly incompatible".
 *
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-005 plan=phase-70-fixes/task-f5/step-f5-1
 */
export function isVersionCompatible(
	current: string,
	minimum: string,
): boolean {
	const cur = parseSemver(current);
	const min = parseSemver(minimum);
	if (!cur || !min) return false;

	const [curMaj, curMin, curPatch] = cur;
	const [minMaj, minMin, minPatch] = min;

	if (curMaj !== minMaj) return curMaj > minMaj;
	if (curMin !== minMin) return curMin > minMin;
	return curPatch >= minPatch;
}

/**
 * Locate the @opencode-ai/plugin package.json by trying multiple candidate
 * paths in priority order:
 *   1. Repo-local install  ({repoDir}/.opencode/node_modules/…)
 *   2. Global OpenCode cache (~/.cache/opencode/node_modules/…)
 *   3. Global npm prefix   (~/.npm/node_modules/…)
 *
 * Returns the first path that exists on disk, or null if none are found.
 *
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-005 plan=phase-70-fixes/task-f5-ext/step-f5-ext-1
 */
export function findSdkPackageJson(repoDir: string): string | null {
	const SDK_PKG_SUBPATH = join(
		"node_modules",
		"@opencode-ai",
		"plugin",
		"package.json",
	);

	const candidates = [
		// 1. Repo-local (current behaviour — highest priority)
		join(repoDir, ".opencode", SDK_PKG_SUBPATH),
		// 2. Global OpenCode cache (npm-installed mode)
		join(homedir(), ".cache", "opencode", SDK_PKG_SUBPATH),
		// 3. Global npm prefix
		join(homedir(), ".npm", SDK_PKG_SUBPATH),
	];

	for (const candidate of candidates) {
		if (existsSync(candidate)) return candidate;
	}
	return null;
}
