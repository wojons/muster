/**
 * User identity resolver — 6-step email identity chain.
 *
 * REQ-PLG-126: The plugin MUST resolve the session user's email address using
 *   this precedence chain:
 *   1. AXIOM_USER_EMAIL env var
 *   2. git config user.email
 *   3. GitHub API user email (if GitHub MCP available)
 *   4. Atlassian MCP user email (if Jira integration configured)
 *   5. metrics.user_email in .axiom/plugin.yaml
 *   6. "anonymous@unknown" (fallback)
 *
 * REQ-PLG-126: Noreply detection — if git email is noreply, emit WARN and
 *   proceed to step 3.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-126
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-134 plan=phase-70-fixes/task-f2-ext/step-f2-ext-1
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-126 jira_ref=DEX-392
 */

import { execSync } from "child_process"
import type { EmailSource } from "./sink-interface"
import { createFallbackLogger } from "../logger"

// Module-level fallback logger — routes to stderr, never stdout (REQ-PLG-134)
const _log = createFallbackLogger()

/** Result of email identity resolution. */
export interface EmailIdentityResult {
  email: string
  source: EmailSource
}

/** Options for resolving email identity. */
export interface EmailIdentityOptions {
  /** Repo root directory for git config lookup. */
  repoRoot: string
  /** Explicit email override from metrics.user_email config. */
  configEmail?: string | null
  /** GitHub MCP email lookup function (optional). */
  githubEmailLookup?: () => Promise<string | null>
  /** Atlassian MCP email lookup function (optional). */
  atlassianEmailLookup?: () => Promise<string | null>
}

/** Noreply email patterns. */
const NOREPLY_PATTERNS = [
  /@users\.noreply\.github\.com$/i,
  /@noreply\./i,
]

/** Check if an email is a noreply address. */
export function isNoreplyEmail(email: string): boolean {
  return NOREPLY_PATTERNS.some((pattern) => pattern.test(email))
}

/**
 * Resolve the session user's email address using the 6-step identity chain.
 *
 * Steps 3 and 4 (GitHub API and Atlassian MCP) are async and optional.
 * If no async lookups are provided, they are skipped.
 */
export async function resolveEmailIdentity(
  options: EmailIdentityOptions,
): Promise<EmailIdentityResult> {
  const { repoRoot, configEmail, githubEmailLookup, atlassianEmailLookup } = options

  // Step 1: AXIOM_USER_EMAIL env var (highest priority)
  const envEmail = process.env.AXIOM_USER_EMAIL
  if (envEmail && envEmail.trim()) {
    return { email: envEmail.trim(), source: "env_var" }
  }

  // Step 2: git config user.email
  const gitEmail = resolveGitEmail(repoRoot)
  if (gitEmail) {
    // Noreply detection — warn and continue to step 3
    if (isNoreplyEmail(gitEmail)) {
      _log.warn(
        "Git email is a noreply address — attempting GitHub API lookup for real identity.",
      )
    } else {
      return { email: gitEmail, source: "git_config" }
    }
  }

  // Step 3: GitHub API user email (if available)
  if (githubEmailLookup) {
    try {
      const githubEmail = await githubEmailLookup()
      if (githubEmail && githubEmail.trim()) {
        return { email: githubEmail.trim(), source: "github_api" }
      }
    } catch {
      // GitHub lookup failed — continue to next step
    }
  }

  // Step 4: Atlassian MCP user email (if available)
  if (atlassianEmailLookup) {
    try {
      const atlassianEmail = await atlassianEmailLookup()
      if (atlassianEmail && atlassianEmail.trim()) {
        return { email: atlassianEmail.trim(), source: "atlassian_mcp" }
      }
    } catch {
      // Atlassian lookup failed — continue to next step
    }
  }

  // If git email was noreply and API lookups failed, use the noreply address
  // (better than anonymous per spec)
  if (gitEmail && isNoreplyEmail(gitEmail)) {
    return { email: gitEmail, source: "git_config" }
  }

  // Step 5: metrics.user_email in config
  if (configEmail && configEmail.trim()) {
    return { email: configEmail.trim(), source: "config_file" }
  }

  // Step 6: Fallback — MUST NOT be silently omitted
  return { email: "anonymous@unknown", source: "anonymous" }
}

/**
 * Synchronous version for cases where async lookups are not available.
 * Skips steps 3 and 4 (GitHub API and Atlassian MCP).
 */
export function resolveEmailIdentitySync(
  options: Omit<EmailIdentityOptions, "githubEmailLookup" | "atlassianEmailLookup">,
): EmailIdentityResult {
  const { repoRoot, configEmail } = options

  // Step 1: AXIOM_USER_EMAIL env var
  const envEmail = process.env.AXIOM_USER_EMAIL
  if (envEmail && envEmail.trim()) {
    return { email: envEmail.trim(), source: "env_var" }
  }

  // Step 2: git config user.email
  const gitEmail = resolveGitEmail(repoRoot)
  if (gitEmail && !isNoreplyEmail(gitEmail)) {
    return { email: gitEmail, source: "git_config" }
  }

  // If noreply, use it (better than anonymous)
  if (gitEmail) {
    _log.warn(
      "Git email is a noreply address — no API lookups available, using noreply address.",
    )
    return { email: gitEmail, source: "git_config" }
  }

  // Step 5: config email
  if (configEmail && configEmail.trim()) {
    return { email: configEmail.trim(), source: "config_file" }
  }

  // Step 6: Fallback
  return { email: "anonymous@unknown", source: "anonymous" }
}

/** Resolve git config user.email from the repo. */
function resolveGitEmail(repoRoot: string): string | null {
  try {
    const result = execSync("git config user.email", {
      cwd: repoRoot,
      encoding: "utf-8",
      timeout: 3000,
      stdio: ["pipe", "pipe", "pipe"],
    }).trim()
    return result || null
  } catch {
    return null
  }
}

/**
 * Resolve the current git branch name.
 *
 * REQ-PLG-133: Returns the branch name, or null if:
 * - Not in a git repo
 * - In detached HEAD state (returns "HEAD" from git — we suppress this)
 * - Command fails for any reason
 *
 * @param repoRoot - Absolute path to the repository root
 */
export function resolveGitBranch(repoRoot: string): string | null {
  try {
    const result = execSync("git rev-parse --abbrev-ref HEAD", {
      cwd: repoRoot,
      encoding: "utf-8",
      timeout: 3000,
      stdio: ["pipe", "pipe", "pipe"],
    }).trim()
    // "HEAD" means detached HEAD state — omit per REQ-PLG-133
    if (!result || result === "HEAD") return null
    return result
  } catch {
    return null
  }
}
