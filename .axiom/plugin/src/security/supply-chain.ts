/**
 * Supply-chain security controls for the Axiom OpenCode plugin.
 *
 * Implements:
 * - REQ-PLG-800: npm provenance attestation guidance
 * - REQ-PLG-801: Exact package name verification and typosquat warnings
 * - REQ-PLG-802: Startup audit logging of version and source path
 * - REQ-PLG-803: No auto-update rule with once-per-day version check
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-800 through REQ-PLG-803
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-800 plan=phase-70-5/task-70-5-5/step-70-5-5-2
 */

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Configuration for supply-chain security controls. */
export interface SupplyChainConfig {
	/** The canonical npm package name. Defaults to "opencode-axiom". */
	expectedPackageName?: string;
	/** How often to check for updates, in milliseconds. Defaults to 24h. */
	updateCheckFrequencyMs?: number;
	/** Whether to include provenance check guidance. Defaults to true. */
	enableProvenanceCheck?: boolean;
}

/** Result of verifying the loaded package name against the canonical name. */
export interface PackageNameVerification {
	/** True if the loaded package matches the canonical name. */
	isExpectedPackage: boolean;
	/** Warning message if the package name does not match. Undefined when ok. */
	warning?: string;
	/** The canonical package name (always "opencode-axiom"). */
	canonicalPackageName: string;
	/** The package name extracted from the load path. */
	detectedPackageName: string;
}

/** Structured startup audit log entry per REQ-PLG-802. */
export interface StartupAuditLog {
	/** Human-readable log message in the spec-defined format. */
	message: string;
	/** Plugin version string (e.g. "1.0.0"). */
	version: string;
	/** Filesystem path the plugin was loaded from. */
	sourcePath: string;
	/** ISO-8601 timestamp of when the log was created. */
	timestamp: string;
}

/** Result of checking for available updates per REQ-PLG-803. */
export interface UpdateCheckResult {
	/** Always false — the plugin MUST NOT auto-update. */
	autoUpdateTriggered: false;
	/** True if a newer version is available. */
	updateAvailable: boolean;
	/** Human-readable message about the update status. */
	message?: string;
	/** Instructions for the user to upgrade manually. */
	upgradeInstructions?: string;
	/** True if the check was skipped due to frequency throttling. */
	skippedDueToFrequency: boolean;
}

/** Provenance attestation guidance per REQ-PLG-800. */
export interface ProvenanceGuidance {
	/** The npm publish command including --provenance. */
	publishCommand: string;
	/** Description of the expected build source. */
	expectedBuildSource: string;
	/** Command consumers can run to verify provenance. */
	verificationCommand: string;
	/** The attestation format used (sigstore). */
	attestationFormat: string;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

/** The canonical npm package name for the Axiom plugin. */
const CANONICAL_PACKAGE_NAME = "opencode-axiom";

/** Default update check frequency: 24 hours in milliseconds. */
const DEFAULT_CHECK_FREQUENCY_MS = 24 * 60 * 60 * 1000;

// ---------------------------------------------------------------------------
// REQ-PLG-800: npm provenance attestation guidance
// ---------------------------------------------------------------------------

/**
 * Returns guidance for publishing with npm provenance attestations.
 * The plugin itself does not perform attestation — it provides the
 * commands and documentation for CI pipelines and consumers.
 */
export function getProvenanceGuidance(): ProvenanceGuidance {
	return {
		publishCommand: "npm publish --provenance",
		expectedBuildSource: "Axiom repo CI pipeline",
		verificationCommand: "npm audit signatures",
		attestationFormat: "sigstore",
	};
}

// ---------------------------------------------------------------------------
// REQ-PLG-801: exact package name verification and typosquat warnings
// ---------------------------------------------------------------------------

/**
 * Extracts the package name from a node_modules load path.
 * Handles both scoped (@scope/pkg) and unscoped package paths.
 */
function extractPackageName(loadPath: string): string {
	const nmIndex = loadPath.lastIndexOf("node_modules/");
	if (nmIndex === -1) {
		// No node_modules in path — return the last path segment before dist/
		const parts = loadPath.split("/").filter(Boolean);
		const distIdx = parts.indexOf("dist");
		if (distIdx > 0) {
			return parts[distIdx - 1];
		}
		return parts[parts.length - 1] ?? "";
	}

	const afterNm = loadPath.slice(nmIndex + "node_modules/".length);
	const segments = afterNm.split("/");

	// Scoped package: @scope/name
	if (segments[0]?.startsWith("@") && segments.length >= 2) {
		return `${segments[0]}/${segments[1]}`;
	}

	return segments[0] ?? "";
}

/**
 * Verifies the plugin was loaded from the expected package name.
 * Returns a warning if the detected name differs from the canonical name,
 * including a typosquat alert for similar-looking names.
 */
export function verifyPackageName(opts: {
	loadedFrom: string;
}): PackageNameVerification {
	const detected = extractPackageName(opts.loadedFrom);
	const isExpected = detected === CANONICAL_PACKAGE_NAME;

	if (isExpected) {
		return {
			isExpectedPackage: true,
			canonicalPackageName: CANONICAL_PACKAGE_NAME,
			detectedPackageName: detected,
		};
	}

	// Build warning — check if it looks like a typosquat
	const isTyposquat = isLikelyTyposquat(detected);
	const warningParts = [
		`Loaded from unexpected package "${detected}".`,
		`Expected "${CANONICAL_PACKAGE_NAME}".`,
	];
	if (isTyposquat) {
		warningParts.push(
			`This may be a typosquat of the official package. Please verify you installed the correct package: npm install ${CANONICAL_PACKAGE_NAME}`,
		);
	}

	return {
		isExpectedPackage: false,
		warning: warningParts.join(" "),
		canonicalPackageName: CANONICAL_PACKAGE_NAME,
		detectedPackageName: detected,
	};
}

/**
 * Checks if a package name looks like a typosquat of the canonical name.
 * Uses simple heuristics: edit distance, common substitutions, missing/extra chars.
 */
function isLikelyTyposquat(name: string): boolean {
	if (name === CANONICAL_PACKAGE_NAME) return false;

	// Normalize for comparison
	const canonical = CANONICAL_PACKAGE_NAME.toLowerCase();
	const candidate = name.toLowerCase();

	// Common typosquat patterns:
	// 1. Missing/swapped characters (opencode-codops, opencodeops)
	// 2. Different separators (opencode_codeops, opencode-code-ops)
	// 3. Very similar names within edit distance threshold

	// Strip all separators and compare
	const canonicalStripped = canonical.replace(/[-_]/g, "");
	const candidateStripped = candidate.replace(/[-_]/g, "");

	// Any package containing both "opencode" and "ops" is suspicious —
	// it's clearly trying to look like the canonical name
	if (
		candidateStripped.includes("opencode") &&
		candidateStripped.includes("ops")
	) {
		return true;
	}

	// Check Levenshtein distance on the raw name (with separators)
	if (levenshtein(canonical, candidate) <= 4) {
		return true;
	}

	// Check Levenshtein distance on the stripped form
	if (levenshtein(canonicalStripped, candidateStripped) <= 4) {
		return true;
	}

	return false;
}

/** Simple Levenshtein distance for typosquat detection. */
function levenshtein(a: string, b: string): number {
	const m = a.length;
	const n = b.length;
	const dp: number[][] = Array.from({ length: m + 1 }, () =>
		Array.from({ length: n + 1 }, () => 0),
	);

	for (let i = 0; i <= m; i++) dp[i][0] = i;
	for (let j = 0; j <= n; j++) dp[0][j] = j;

	for (let i = 1; i <= m; i++) {
		for (let j = 1; j <= n; j++) {
			const cost = a[i - 1] === b[j - 1] ? 0 : 1;
			dp[i][j] = Math.min(
				dp[i - 1][j] + 1,
				dp[i][j - 1] + 1,
				dp[i - 1][j - 1] + cost,
			);
		}
	}

	return dp[m][n];
}

// ---------------------------------------------------------------------------
// REQ-PLG-802: startup audit logging
// ---------------------------------------------------------------------------

/**
 * Builds a structured startup audit log entry.
 * Format: "Axiom plugin vX.Y.Z loaded from <path>"
 */
export function buildStartupAuditLog(opts: {
	version: string;
	sourcePath: string;
}): StartupAuditLog {
	return {
		message: `Axiom plugin v${opts.version} loaded from ${opts.sourcePath}`,
		version: opts.version,
		sourcePath: opts.sourcePath,
		timestamp: new Date().toISOString(),
	};
}

// ---------------------------------------------------------------------------
// REQ-PLG-803: no auto-update rule
// ---------------------------------------------------------------------------

/**
 * Checks whether a newer version is available, respecting the once-per-day
 * frequency limit. Never triggers automatic updates.
 *
 * This is a pure data function — the actual npm registry check is the
 * caller's responsibility. This function evaluates the result and enforces
 * the no-auto-update and frequency rules.
 */
export function checkForUpdates(opts: {
	currentVersion: string;
	latestVersion: string;
	lastCheckTimestamp?: number;
}): UpdateCheckResult {
	const checkFrequency = DEFAULT_CHECK_FREQUENCY_MS;

	// Frequency throttle: skip if last check was less than 24h ago
	if (opts.lastCheckTimestamp !== undefined) {
		const elapsed = Date.now() - opts.lastCheckTimestamp;
		if (elapsed < checkFrequency) {
			return {
				autoUpdateTriggered: false,
				updateAvailable: false,
				skippedDueToFrequency: true,
			};
		}
	}

	// Compare versions
	const updateAvailable =
		opts.latestVersion !== opts.currentVersion &&
		isNewerVersion(opts.latestVersion, opts.currentVersion);

	if (!updateAvailable) {
		return {
			autoUpdateTriggered: false,
			updateAvailable: false,
			skippedDueToFrequency: false,
		};
	}

	return {
		autoUpdateTriggered: false,
		updateAvailable: true,
		message: `A newer version of the Axiom plugin is available: ${opts.latestVersion} (current: ${opts.currentVersion})`,
		upgradeInstructions: `To upgrade, update the plugin version in your opencode.json or run: npm update ${CANONICAL_PACKAGE_NAME}`,
		skippedDueToFrequency: false,
	};
}

/**
 * Simple semver comparison: returns true if `a` is newer than `b`.
 * Handles basic X.Y.Z format.
 */
function isNewerVersion(a: string, b: string): boolean {
	const partsA = a.split(".").map(Number);
	const partsB = b.split(".").map(Number);

	for (let i = 0; i < Math.max(partsA.length, partsB.length); i++) {
		const va = partsA[i] ?? 0;
		const vb = partsB[i] ?? 0;
		if (va > vb) return true;
		if (va < vb) return false;
	}

	return false;
}
