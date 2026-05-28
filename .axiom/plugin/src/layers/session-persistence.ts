/**
 * L2 Session Persistence — persist session analytics to
 * `.axiom/sessions/<session-id>.json` on session end, enforce a 90-day /
 * 1000-file retention policy on session start, and ensure `.axiom/sessions/`
 * is in `.gitignore`.
 *
 * REQ-PLG-106: Session analytics MUST be persistable to
 *   `.axiom/sessions/<session-id>.json` when the session ends. This file
 *   follows the schema in REQ-PLG-102 with additional fields.
 * REQ-PLG-106a: The `.axiom/sessions/` directory MUST have a local retention
 *   policy: files older than 90 days or exceeding 1000 files (whichever is
 *   reached first) MUST be automatically cleaned up on session start. The
 *   `/axiom-setup` command MUST add `.axiom/sessions/` to `.gitignore` to
 *   prevent repo bloat.
 *
 * Out of scope:
 * - REQ-PLG-107: codeops_session_stats tool (repo-history aggregation)
 * - REQ-PLG-113: Per-repo cost budgets with session history aggregation
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-106, specs/70-OpenCode-Plugin.md#REQ-PLG-106a
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-106 plan=phase-70-3/task-70-3-4/step-70-3-4-2
 */

import {
	existsSync,
	mkdirSync,
	readdirSync,
	readFileSync,
	statSync,
	unlinkSync,
	writeFileSync,
} from "node:fs";
import { join, resolve } from "node:path";
import type { SessionCostToolResult } from "./metrics-tools";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

/** Name of the sessions directory under .axiom/. */
export const SESSION_DIR_NAME = "sessions";

/** Maximum number of session files to retain. */
export const MAX_SESSION_FILES = 1000;

/** Maximum age in days for session files. */
export const MAX_SESSION_AGE_DAYS = 90;

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/**
 * The persisted session analytics file shape.
 * Extends SessionCostToolResult (REQ-PLG-102) with additional fields.
 */
export interface SessionAnalyticsFile extends SessionCostToolResult {
	/** ISO 8601 timestamp when the session started. */
	started_at: string;
	/** ISO 8601 timestamp when the session ended. */
	ended_at: string;
	/** Absolute path where the file was (or would be) written. */
	file_path: string;
	/** Whether this was a dry-run (file not actually written). */
	dry_run?: boolean;
	/** Maturity level of the repo at session time, if available. */
	maturity_level?: number;
}

/** Options for persistSessionAnalytics. */
export interface PersistSessionOptions {
	/** Root directory of the repository. */
	repoDir: string;
	/** The session cost tool result to persist. */
	toolResult: SessionCostToolResult;
	/** When the session started. */
	startedAt: Date;
	/** When the session ended. */
	endedAt: Date;
	/** If true, compute the result but do not write to disk. */
	dryRun?: boolean;
	/** Optional maturity level to include. */
	maturityLevel?: number;
}

/** Test-injectable file descriptor for retention policy testing. */
export interface TestFileEntry {
	name: string;
	ageDays: number;
}

/** Options for enforceRetentionPolicy. */
export interface RetentionPolicyOptions {
	/** Path to the .axiom/sessions/ directory. */
	sessionsDir: string;
	/** If true, compute removals but do not delete files. */
	dryRun?: boolean;
	/** Injected test files for deterministic testing. */
	_testFiles?: TestFileEntry[];
}

/** Result of retention policy enforcement. */
export interface RetentionResult {
	/** Number of files removed (or that would be removed in dry-run). */
	files_removed: number;
	/** Number of files remaining after cleanup. */
	files_remaining: number;
	/** Human-readable reason for the cleanup action. */
	reason: string;
	/** Names of files that were (or would be) removed. */
	removed_files: string[];
}

/** Options for ensureSessionsGitignore. */
export interface GitignoreOptions {
	/** Root directory of the repository. */
	repoDir: string;
	/** If true, compute the action but do not write to disk. */
	dryRun?: boolean;
	/** Injected existing .gitignore content for testing. null = file does not exist. */
	_existingGitignore?: string | null;
}

/** Result of gitignore handling. */
export interface GitignoreResult {
	/** Action taken: "added", "already_present", "created", or "skipped". */
	action: "added" | "already_present" | "created" | "skipped";
	/** Path to the .gitignore file. */
	gitignore_path: string;
	/** The entry that was added or checked. */
	entry?: string;
}

// ---------------------------------------------------------------------------
// REQ-PLG-106: Session persistence
// ---------------------------------------------------------------------------

/**
 * Sanitize a session ID to prevent path traversal attacks.
 * Strips directory separators and ".." sequences, keeping only safe characters.
 */
function sanitizeSessionId(sessionId: string): string {
	// Remove any path separators and ".." sequences
	return sessionId
		.replace(/\.\./g, "_")
		.replace(/[/\\]/g, "_")
		.replace(/[^a-zA-Z0-9_-]/g, "_");
}

/**
 * Persist session analytics to `.axiom/sessions/<session-id>.json`.
 *
 * REQ-PLG-106: The file follows the schema in REQ-PLG-102 with additional
 * fields (started_at, ended_at, file_path, maturity_level).
 */
export function persistSessionAnalytics(
	options: PersistSessionOptions,
): SessionAnalyticsFile {
	const { repoDir, toolResult, startedAt, endedAt, dryRun, maturityLevel } =
		options;

	const safeId = sanitizeSessionId(toolResult.session_id);
	const sessionsDir = join(resolve(repoDir), ".axiom", SESSION_DIR_NAME);
	const filePath = join(sessionsDir, `${safeId}.json`);

	const result: SessionAnalyticsFile = {
		...toolResult,
		started_at: startedAt.toISOString(),
		ended_at: endedAt.toISOString(),
		file_path: filePath,
	};

	// REQ-PLG-104a: include session_identity in all analytics events
	const identity = process.env.AXIOM_SESSION_IDENTITY;
	const identitySource = process.env.AXIOM_SESSION_IDENTITY_SOURCE;
	if (identity) {
		result.session_identity = identity;
		result.session_identity_source = identitySource;
	}

	if (maturityLevel !== undefined) {
		result.maturity_level = maturityLevel;
	}

	if (dryRun) {
		result.dry_run = true;
		return result;
	}

	// Ensure the sessions directory exists
	if (!existsSync(sessionsDir)) {
		mkdirSync(sessionsDir, { recursive: true });
	}

	// Write the session file
	writeFileSync(filePath, JSON.stringify(result, null, 2), "utf-8");

	return result;
}

// ---------------------------------------------------------------------------
// REQ-PLG-106a: Retention policy
// ---------------------------------------------------------------------------

/**
 * Enforce the retention policy on the sessions directory.
 *
 * REQ-PLG-106a: Files older than 90 days or exceeding 1000 files (whichever
 * is reached first) MUST be automatically cleaned up on session start.
 *
 * Policy application order:
 * 1. Remove files older than MAX_SESSION_AGE_DAYS (> 90 days, not >=)
 * 2. If remaining count still exceeds MAX_SESSION_FILES, remove oldest until at limit
 * 3. Both policies are applied; whichever removes more files wins
 */
export function enforceRetentionPolicy(
	options: RetentionPolicyOptions,
): RetentionResult {
	const { sessionsDir, dryRun, _testFiles } = options;

	// If test files are injected, use them for deterministic testing
	if (_testFiles !== undefined) {
		return enforceRetentionOnTestFiles(_testFiles, dryRun);
	}

	// Real filesystem path
	if (!existsSync(sessionsDir)) {
		return {
			files_removed: 0,
			files_remaining: 0,
			reason: "Sessions directory not found",
			removed_files: [],
		};
	}

	const now = Date.now();
	const maxAgeMs = MAX_SESSION_AGE_DAYS * 24 * 60 * 60 * 1000;

	// Read all .json files with their stats
	const entries: Array<{ name: string; mtime: number }> = [];
	for (const name of readdirSync(sessionsDir)) {
		if (!name.endsWith(".json")) continue;
		try {
			const stat = statSync(join(sessionsDir, name));
			entries.push({ name, mtime: stat.mtimeMs });
		} catch {
			// Skip files we can't stat
		}
	}

	// Compute age-based and count-based removal sets independently,
	// then apply whichever removes more files.

	// 1. Age-based removal: files older than 90 days (strictly greater than)
	const ageRemoval = new Set<string>();
	for (const entry of entries) {
		const ageMs = now - entry.mtime;
		if (ageMs > maxAgeMs) {
			ageRemoval.add(entry.name);
		}
	}

	// 2. Count-based removal: if total exceeds MAX_SESSION_FILES, remove oldest
	const countRemoval = new Set<string>();
	if (entries.length > MAX_SESSION_FILES) {
		// Sort by mtime ascending (oldest first)
		const sorted = [...entries].sort((a, b) => a.mtime - b.mtime);
		const excess = entries.length - MAX_SESSION_FILES;
		for (let i = 0; i < excess; i++) {
			countRemoval.add(sorted[i].name);
		}
	}

	// Apply whichever policy removes more files
	const toRemove =
		ageRemoval.size >= countRemoval.size ? ageRemoval : countRemoval;

	// Execute removals
	const removedFiles = Array.from(toRemove);
	if (!dryRun) {
		for (const name of removedFiles) {
			try {
				unlinkSync(join(sessionsDir, name));
			} catch {
				// Best-effort removal
			}
		}
	}

	const reasons: string[] = [];
	if (removedFiles.length > 0) {
		reasons.push(`Removed ${removedFiles.length} file(s) by retention policy`);
	} else {
		reasons.push("No files exceeded retention limits");
	}

	return {
		files_removed: removedFiles.length,
		files_remaining: entries.length - removedFiles.length,
		reason: reasons.join("; "),
		removed_files: removedFiles,
	};
}

/**
 * Apply retention policy to injected test files (deterministic testing path).
 */
function enforceRetentionOnTestFiles(
	testFiles: TestFileEntry[],
	_dryRun?: boolean,
): RetentionResult {
	// Filter to only .json files
	const jsonFiles = testFiles.filter((f) => f.name.endsWith(".json"));

	// Compute age-based and count-based removal sets independently,
	// then apply whichever removes more files.

	// 1. Age-based: remove files older than 90 days (strictly > 90)
	const ageRemoval = new Set<string>();
	for (const file of jsonFiles) {
		if (file.ageDays > MAX_SESSION_AGE_DAYS) {
			ageRemoval.add(file.name);
		}
	}

	// 2. Count-based: if total exceeds MAX_SESSION_FILES, remove oldest
	const countRemoval = new Set<string>();
	if (jsonFiles.length > MAX_SESSION_FILES) {
		// Sort by ageDays descending (oldest first = highest ageDays)
		const sorted = [...jsonFiles].sort((a, b) => b.ageDays - a.ageDays);
		const excess = jsonFiles.length - MAX_SESSION_FILES;
		for (let i = 0; i < excess; i++) {
			countRemoval.add(sorted[i].name);
		}
	}

	// Apply whichever policy removes more files
	const toRemove =
		ageRemoval.size >= countRemoval.size ? ageRemoval : countRemoval;

	const removedFiles = Array.from(toRemove);
	const reasons: string[] = [];
	if (removedFiles.length > 0) {
		reasons.push(`Removed ${removedFiles.length} file(s) by retention policy`);
	} else if (jsonFiles.length === 0) {
		reasons.push("No session files found");
	} else {
		reasons.push("No files exceeded retention limits");
	}

	return {
		files_removed: removedFiles.length,
		files_remaining: jsonFiles.length - removedFiles.length,
		reason: reasons.join("; "),
		removed_files: removedFiles,
	};
}

// ---------------------------------------------------------------------------
// REQ-PLG-106a: .gitignore handling
// ---------------------------------------------------------------------------

const GITIGNORE_ENTRY = ".axiom/sessions/";

/**
 * Ensure `.axiom/sessions/` is in the repo's `.gitignore`.
 *
 * REQ-PLG-106a: The `/axiom-setup` command MUST add `.axiom/sessions/`
 * to `.gitignore` to prevent repo bloat.
 */
export function ensureSessionsGitignore(
	options: GitignoreOptions,
): GitignoreResult {
	const { repoDir, dryRun, _existingGitignore } = options;
	const gitignorePath = join(resolve(repoDir), ".gitignore");

	// Determine existing content
	let existingContent: string | null;
	if (_existingGitignore !== undefined) {
		existingContent = _existingGitignore;
	} else {
		try {
			existingContent = readFileSync(gitignorePath, "utf-8");
		} catch {
			existingContent = null;
		}
	}

	// File does not exist — create it with the entry
	if (existingContent === null) {
		if (!dryRun) {
			writeFileSync(gitignorePath, `${GITIGNORE_ENTRY}\n`, "utf-8");
		}
		return {
			action: "created",
			gitignore_path: gitignorePath,
			entry: GITIGNORE_ENTRY,
		};
	}

	// Check if the entry already exists (with or without trailing slash)
	const lines = existingContent.split("\n");
	const entryWithoutSlash = GITIGNORE_ENTRY.replace(/\/$/, "");
	const alreadyPresent = lines.some((line) => {
		const trimmed = line.trim();
		return trimmed === GITIGNORE_ENTRY || trimmed === entryWithoutSlash;
	});

	if (alreadyPresent) {
		return {
			action: "already_present",
			gitignore_path: gitignorePath,
			entry: GITIGNORE_ENTRY,
		};
	}

	// Add the entry
	if (!dryRun) {
		// Ensure we add on a new line
		const needsNewline =
			existingContent.length > 0 && !existingContent.endsWith("\n");
		const newContent = needsNewline
			? `${existingContent}\n${GITIGNORE_ENTRY}\n`
			: `${existingContent}${GITIGNORE_ENTRY}\n`;
		writeFileSync(gitignorePath, newContent, "utf-8");
	}

	return {
		action: "added",
		gitignore_path: gitignorePath,
		entry: GITIGNORE_ENTRY,
	};
}
