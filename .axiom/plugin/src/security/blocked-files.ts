/**
 * Blocked-file pattern enforcement — prevents reading secrets files via
 * the `tool.execute.before` hook by matching file paths against a set of
 * built-in and custom glob-like patterns.
 *
 * REQ-PLG-700: The plugin MUST prevent reading secrets files. Blocked patterns:
 *   - `.env`, `.env.*`
 *   - `credentials*`, `*credential*`
 *   - `*secret*`, `*token*`, `*key*` (in filenames, not content)
 *   - `*.pem`, `*.p12`, `*.pfx`
 *   - Custom patterns from `.axiom/plugin.yaml` `security.blocked_patterns`
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-700
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-700 plan=phase-70-5/task-70-5-2/step-70-5-2-2
 */

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Result of checking whether a file path is blocked. */
export interface BlockedFileResult {
	/** Whether the file is blocked. */
	blocked: boolean;
	/** The pattern that matched (only present when blocked). */
	pattern?: string;
	/** Human-readable reason (only present when blocked). */
	reason?: string;
}

/** Configuration for the blocked-file checker. */
export interface BlockedFileConfig {
	/** Additional custom patterns to apply alongside built-in patterns. */
	customPatterns?: string[];
}

/** A checker instance that combines default + custom patterns. */
export interface BlockedFileChecker {
	/** Check whether a file path is blocked. */
	check: (filePath: string) => BlockedFileResult;
}

// ---------------------------------------------------------------------------
// Default blocked patterns (REQ-PLG-700)
// ---------------------------------------------------------------------------

/**
 * Default blocked patterns per REQ-PLG-700.
 *
 * Each entry is a simplified glob pattern:
 * - `*` matches any sequence of non-separator characters
 * - Patterns are matched against the basename of the file path
 * - `.env` and `.env.*` are special-cased for exact prefix matching
 */
export const DEFAULT_BLOCKED_PATTERNS: string[] = [
	// .env and .env.*
	".env",
	".env.*",
	// credentials* and *credential*
	"credentials*",
	"*credential*",
	// *secret*, *token*, *key* (filename patterns)
	"*secret*",
	"*token*",
	"*key*",
	// Certificate files
	"*.pem",
	"*.p12",
	"*.pfx",
];

// ---------------------------------------------------------------------------
// Pattern matching
// ---------------------------------------------------------------------------

/**
 * Convert a simplified glob pattern to a RegExp.
 *
 * Rules:
 * - `*` matches zero or more non-separator characters
 * - `.` is escaped to match literal dots
 * - The pattern is anchored to match the full basename
 */
function globToRegex(pattern: string): RegExp {
	// Escape regex-special characters except `*`
	let escaped = pattern.replace(/[.+?^${}()|[\]\\]/g, "\\$&");
	// Replace `*` with `.*` (match any characters)
	escaped = escaped.replace(/\*/g, ".*");
	return new RegExp(`^${escaped}$`, "i");
}

/**
 * Extract the basename (last path segment) from a file path.
 */
function basename(filePath: string): string {
	const parts = filePath.replace(/\\/g, "/").split("/");
	return parts[parts.length - 1] || filePath;
}

/**
 * Check whether a file path matches a single pattern.
 */
function matchesPattern(filePath: string, pattern: string): boolean {
	const name = basename(filePath);
	const regex = globToRegex(pattern);
	return regex.test(name);
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Check whether a file path is blocked by the default REQ-PLG-700 patterns.
 *
 * This is a convenience function that uses only the built-in default patterns.
 * For custom patterns, use `createBlockedFileChecker()`.
 */
export function isBlockedFile(filePath: string): BlockedFileResult {
	for (const pattern of DEFAULT_BLOCKED_PATTERNS) {
		if (matchesPattern(filePath, pattern)) {
			return {
				blocked: true,
				pattern,
				reason: `File "${basename(filePath)}" matches blocked pattern "${pattern}" (REQ-PLG-700)`,
			};
		}
	}
	return { blocked: false };
}

/**
 * Create a blocked-file checker that combines default patterns with
 * custom patterns from `.axiom/plugin.yaml` `security.blocked_patterns`.
 */
export function createBlockedFileChecker(
	config?: BlockedFileConfig,
): BlockedFileChecker {
	const allPatterns = [
		...DEFAULT_BLOCKED_PATTERNS,
		...(config?.customPatterns ?? []),
	];

	return {
		check(filePath: string): BlockedFileResult {
			for (const pattern of allPatterns) {
				if (matchesPattern(filePath, pattern)) {
					return {
						blocked: true,
						pattern,
						reason: `File "${basename(filePath)}" matches blocked pattern "${pattern}" (REQ-PLG-700)`,
					};
				}
			}
			return { blocked: false };
		},
	};
}
