/**
 * Shared-content redaction engine — applies automatic redaction to all content
 * that may leave the local session boundary (session replay, broadcast, hook
 * output).
 *
 * REQ-PLG-701: Automatic redaction of AWS keys, JWTs, generic API keys in
 *   key-value contexts, and KEY=VALUE env vars. Supports custom patterns from
 *   `.axiom/plugin.yaml` security.redaction_patterns.
 *
 * REQ-PLG-702: Conservative — over-redact rather than leak.
 *
 * REQ-PLG-703: Audit logging — "Redacted N items: X AWS key, Y JWT token, ..."
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-701
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-701 plan=phase-70-5/task-70-5-1/step-70-5-1-2
 */

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Result of a redaction pass over a string. */
export interface RedactionResult {
	/** The redacted content string. */
	redacted: string;
	/** Total number of redacted items. */
	count: number;
	/** Audit log entry (undefined when count === 0). */
	log?: RedactionLogEntry;
}

/** Audit log entry produced by a redaction pass. */
export interface RedactionLogEntry {
	/** Total redacted items. */
	totalCount: number;
	/** Per-category counts (e.g. { "aws-key": 1, "jwt": 2 }). */
	categories: Record<string, number>;
	/** Human-readable audit message per REQ-PLG-703. */
	message: string;
}

/** Configuration for the redaction engine. */
export interface RedactionConfig {
	/** Additional custom patterns to apply alongside built-in patterns. */
	customPatterns?: CustomPattern[];
}

/** A custom redaction pattern from plugin.yaml. */
export interface CustomPattern {
	/** Regex pattern string (will be compiled with global flag). */
	pattern: string;
	/** Label used in the replacement tag, e.g. "stripe-key" → [REDACTED:stripe-key]. */
	label: string;
}

/** A compiled redaction engine instance. */
export interface RedactionEngine {
	/** Apply redaction to the given content string. */
	redact(content: string): RedactionResult;
}

// ---------------------------------------------------------------------------
// Internal: pattern definition
// ---------------------------------------------------------------------------

interface InternalPattern {
	regex: RegExp;
	label: string;
	/** If true, the replacement replaces only the value portion (for KEY=VALUE). */
	keyValueReplace?: boolean;
}

// ---------------------------------------------------------------------------
// Built-in patterns (REQ-PLG-701)
// ---------------------------------------------------------------------------

/**
 * Build the ordered list of built-in redaction patterns.
 *
 * Order matters: more specific patterns (AWS, JWT) run before generic ones
 * (API key, env var) to avoid double-redaction.
 */
function builtinPatterns(): InternalPattern[] {
	return [
		// AWS access key IDs: AKIA followed by exactly 16 uppercase alphanumeric chars
		{
			regex: /AKIA[0-9A-Z]{16}/g,
			label: "aws-key",
		},

		// JWT tokens: three base64url segments separated by dots, each starting with eyJ
		{
			regex: /eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/g,
			label: "jwt",
		},

		// Generic API keys: 32+ alphanumeric chars in key-value contexts
		// Matches patterns like: key=<value>, key: <value>, Authorization: Bearer <value>
		{
			regex:
				/(?:(?:[a-zA-Z_][a-zA-Z0-9_]*\s*[=:]\s*)|(?:Bearer\s+))([a-zA-Z0-9]{32,})/g,
			label: "api-key",
		},

		// Environment variable KEY=VALUE: uppercase key with underscores, followed by = and a value
		// KEY must be at least 2 uppercase chars with underscores, value is everything to EOL or whitespace
		{
			regex: /^([A-Z][A-Z0-9_]{1,})=(.+)$/gm,
			label: "env",
			keyValueReplace: true,
		},
	];
}

// ---------------------------------------------------------------------------
// Category label → human-readable name mapping for audit messages
// ---------------------------------------------------------------------------

const CATEGORY_DISPLAY_NAMES: Record<string, string> = {
	"aws-key": "AWS key",
	jwt: "JWT token",
	"api-key": "API key",
	env: "env variable",
};

// ---------------------------------------------------------------------------
// Core redaction logic
// ---------------------------------------------------------------------------

function applyPatterns(
	content: string,
	patterns: InternalPattern[],
): RedactionResult {
	let result = content;
	const categories: Record<string, number> = {};
	let totalCount = 0;

	for (const pat of patterns) {
		// Reset regex lastIndex for global patterns
		pat.regex.lastIndex = 0;

		if (pat.keyValueReplace) {
			// For KEY=VALUE patterns, replace only the value portion
			result = result.replace(
				pat.regex,
				(_match, key: string, _value: string) => {
					categories[pat.label] = (categories[pat.label] ?? 0) + 1;
					totalCount++;
					return `${key}=[REDACTED:${pat.label}]`;
				},
			);
		} else if (pat.label === "api-key") {
			// For generic API key pattern, we need to capture the prefix and replace only the key
			result = result.replace(pat.regex, (match, captured: string) => {
				categories[pat.label] = (categories[pat.label] ?? 0) + 1;
				totalCount++;
				const prefix = match.slice(0, match.length - captured.length);
				return `${prefix}[REDACTED:${pat.label}]`;
			});
		} else {
			// Standard full-match replacement
			result = result.replace(pat.regex, () => {
				categories[pat.label] = (categories[pat.label] ?? 0) + 1;
				totalCount++;
				return `[REDACTED:${pat.label}]`;
			});
		}
	}

	if (totalCount === 0) {
		return { redacted: result, count: 0 };
	}

	// Build audit message per REQ-PLG-703
	const parts: string[] = [];
	for (const [cat, count] of Object.entries(categories)) {
		const displayName = CATEGORY_DISPLAY_NAMES[cat] ?? cat;
		parts.push(`${count} ${displayName}`);
	}
	const itemWord = totalCount === 1 ? "item" : "items";
	const message = `Redacted ${totalCount} ${itemWord}: ${parts.join(", ")}.`;

	return {
		redacted: result,
		count: totalCount,
		log: {
			totalCount,
			categories,
			message,
		},
	};
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Create a redaction engine with optional custom patterns.
 *
 * The engine applies all built-in patterns (AWS keys, JWTs, generic API keys,
 * KEY=VALUE env vars) plus any custom patterns from the provided config.
 */
export function createRedactionEngine(
	config?: RedactionConfig,
): RedactionEngine {
	const patterns: InternalPattern[] = builtinPatterns();

	// Append custom patterns from config
	if (config?.customPatterns) {
		for (const cp of config.customPatterns) {
			patterns.push({
				regex: new RegExp(cp.pattern, "g"),
				label: cp.label,
			});
		}
	}

	return {
		redact(content: string): RedactionResult {
			// Create fresh regex instances to avoid stale lastIndex state
			const freshPatterns = patterns.map((p) => ({
				...p,
				regex: new RegExp(p.regex.source, p.regex.flags),
			}));
			return applyPatterns(content, freshPatterns);
		},
	};
}

/**
 * Convenience function: redact content using the default engine (built-in
 * patterns only, no custom config).
 */
const defaultEngine = createRedactionEngine();

export function redactContent(content: string): RedactionResult {
	return defaultEngine.redact(content);
}
