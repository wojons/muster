/**
 * Progressive command surface — maturity-based command visibility and gating.
 *
 * REQ-PLG-020: Commands are progressively unlocked based on the repo's Axiom
 *   maturity level. Each level is a superset of the previous (with one exception:
 *   /axiom-next is not available at Level 4 — already at max maturity).
 *
 * REQ-PLG-021: Commands not yet available for the current maturity level MUST NOT
 *   appear in the command list (hidden, not just disabled).
 *
 * REQ-PLG-022: Above-level command invocations return conversational guidance
 *   (implemented in task-70-1-4-2 step 2).
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-020, #REQ-PLG-021, #REQ-PLG-022
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-020,specs/70-OpenCode-Plugin.md#REQ-PLG-021 plan=phase-70-1/task-70-1-4/step-70-1-4-2
 */

import { MaturityLevel } from "./detect"

/**
 * A single command definition in the progressive command surface.
 *
 * - `minLevel`: The minimum maturity level at which this command is visible.
 * - `maxLevel`: Optional upper bound. If set, the command is hidden above this level.
 *   Used for /axiom-next which is not available at Level 4 (Mature).
 */
export interface CommandDefinition {
  /** Slash-command name, e.g. "/axiom-setup" */
  name: string
  /** Human-readable description shown in the command palette */
  description: string
  /** Minimum maturity level at which this command is visible (inclusive) */
  minLevel: MaturityLevel
  /** Optional maximum maturity level (inclusive). Absent means no upper bound. */
  maxLevel?: MaturityLevel
}

/**
 * The full progressive command visibility matrix per REQ-PLG-020.
 *
 * Level 0 (New):        setup, help, status, next, glossary, privacy  → 6 commands
 * Level 1 (Scaffolded): + spec                                        → 7 commands
 * Level 2 (Specced):    + plan                                        → 8 commands
 * Level 3 (Active):     + run, verify, loop, share, pulse, spinup, backend → 15 commands
 * Level 4 (Mature):     + release, - next                            → 15 commands
 */
export const COMMAND_VISIBILITY_MATRIX: CommandDefinition[] = [
  // ── Level 0 commands (available from the very first session) ────────────
  {
    name: "/axiom-setup",
    description: "Bootstrap Axiom in this repository — scaffold specs/, .memory-bank/, and .opencode/ structure",
    minLevel: MaturityLevel.New,
  },
  {
    name: "/axiom-help",
    description: "Show available Axiom commands and a quick-start guide for the current maturity level",
    minLevel: MaturityLevel.New,
  },
  {
    name: "/axiom-status",
    description: "Show the current Axiom maturity level, active work items, and open gaps",
    minLevel: MaturityLevel.New,
  },
  {
    name: "/axiom-next",
    description: "Show the recommended next action to advance to the next maturity level",
    minLevel: MaturityLevel.New,
    // REQ-PLG-020: /axiom-next is not available at Level 4 (already at max maturity)
    maxLevel: MaturityLevel.Active, // 3 — hidden at Level 4
  },
  {
    name: "/axiom-glossary",
    description: "Show Axiom terminology and concept definitions",
    minLevel: MaturityLevel.New,
  },
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-305 plan=phase-5.0/task-5.0.privacy-command/step-5.0.1
  {
    name: "/axiom-privacy",
    description: "Show what Wave data is stored, where it is stored, and how to delete it",
    minLevel: MaturityLevel.New,
  },

  // ── Level 1 commands (unlocked when .opencode/ scaffold exists) ──────────
  {
    name: "/axiom-spec",
    description: "Create, update, or review product specs in specs/",
    minLevel: MaturityLevel.Scaffolded,
  },

  // ── Level 2 commands (unlocked when specs/ has content) ─────────────────
  {
    name: "/axiom-plan",
    description: "Generate or update an implementation plan aligned to specs",
    minLevel: MaturityLevel.Specced,
  },

  // ── Level 3 commands (unlocked when specs + plans + memory bank exist) ───
  {
    name: "/axiom-run",
    description: "Execute a work item plan step using the Axiom runner",
    minLevel: MaturityLevel.Active,
  },
  {
    name: "/axiom-verify",
    description: "Run the verifier-captain to assess evidence and issue a verdict",
    minLevel: MaturityLevel.Active,
  },
  {
    name: "/axiom-loop",
    description: "Start a Ralph loop — iterative builder/verifier cycles until acceptance",
    minLevel: MaturityLevel.Active,
  },
  {
    name: "/axiom-share",
    description: "Share a session replay or broadcast to team members",
    minLevel: MaturityLevel.Active,
  },
  {
    name: "/axiom-pulse",
    description: "Show team activity pulse — who is working on what right now",
    minLevel: MaturityLevel.Active,
  },
  {
    name: "/axiom-spinup",
    description: "Spin up a backend (Python/Go) for the current repository",
    minLevel: MaturityLevel.Active,
  },
  {
    name: "/axiom-backend",
    description: "Show available backends and switch between Python and Go execution modes",
    minLevel: MaturityLevel.Active,
  },
  {
    name: "/axiom-wave",
    description: "Create or open a Wave Space for the current work item — interactive review and collaboration portal",
    minLevel: MaturityLevel.Active, // Level 3+
  },
  {
    name: "/axiom-review",
    description: "Open the current session in a Wave Space for live review — share with colleagues in real time",
    minLevel: MaturityLevel.Active, // Level 3+
  },
  // ── Wave annotation + render commands (Level 2+ and 3+) ─────────────────
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-2.8/task-2.8.1/step-2.8.1
  {
    name: "/axiom-annotate",
    description: "Annotate a file in a Wave artifact_review Space — /axiom-annotate <file_path>",
    minLevel: MaturityLevel.Specced, // Level 2+
  },
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-2.8/task-2.8.2/step-2.8.2
  {
    name: "/axiom-render",
    description: "Render a pre-built template in the current Wave Space — /axiom-render <template> [data_json]",
    minLevel: MaturityLevel.Active, // Level 3+
  },

  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
  {
    name: "/axiom-status-page",
    description: "Create a status_page Space with a default automation and open it in the browser",
    minLevel: MaturityLevel.Active, // Level 3+
  },

  // ── Level 4 commands (unlocked when evidence bundles + CI integration exist) ─
  {
    name: "/axiom-release",
    description: "Prepare and publish a release — changelog, version bump, and publish pipeline",
    minLevel: MaturityLevel.Mature,
  },
]

/**
 * Returns the list of commands visible at the given maturity level.
 *
 * REQ-PLG-020: Commands are progressively unlocked — each level is a superset
 *   of the previous (with the /axiom-next exception at Level 4).
 * REQ-PLG-021: Commands not yet available MUST NOT appear (hidden, not disabled).
 *
 * @param level - The current repo maturity level
 * @returns Array of CommandDefinition objects visible at this level
 */
export function getCommandsForLevel(level: MaturityLevel): CommandDefinition[] {
  return COMMAND_VISIBILITY_MATRIX.filter((cmd) => {
    // Must meet minimum level requirement
    if (level < cmd.minLevel) return false
    // Must not exceed maximum level (if set)
    if (cmd.maxLevel !== undefined && level > cmd.maxLevel) return false
    return true
  })
}

/**
 * Result of attempting to handle an above-level command.
 *
 * - `allowed`: The command is available at the current level; no guidance needed.
 * - `above_level`: The command requires a higher maturity level; `guidance` contains
 *   a conversational message per REQ-PLG-022.
 * - `unknown`: The command is not in the visibility matrix at all.
 */
export interface AboveLevelResult {
  status: "allowed" | "above_level" | "unknown"
  /** Conversational guidance message (only set when status is "above_level") */
  guidance?: string
  /** The command definition (set when status is "allowed" or "above_level") */
  command?: CommandDefinition
  /** The minimum level required (set when status is "above_level") */
  requiredLevel?: MaturityLevel
}

/**
 * Guidance templates per command for above-level attempts.
 *
 * REQ-PLG-022: The plugin MUST respond conversationally when a user tries to
 * use a command above their maturity level.
 *
 * REQ-PLG-061: All plugin messages use Day 1 vocabulary until Level 3.
 */
const ABOVE_LEVEL_GUIDANCE: Record<string, string> = {
  "/axiom-spec":
    'To use `/axiom-spec`, you\'ll need a scaffolded repo first. Want me to run `/axiom-setup` to get you started?',
  "/axiom-plan":
    'To use `/axiom-plan`, you\'ll need specs first. Want me to help you create your first spec?',
  "/axiom-run":
    'To use `/axiom-run`, you\'ll need specs and a plan first. Want me to help you create your first spec?',
  "/axiom-verify":
    'To use `/axiom-verify`, you\'ll need specs, a plan, and an active run first. Want me to help you get started with `/axiom-setup`?',
  "/axiom-loop":
    'To use `/axiom-loop`, you\'ll need specs, a plan, and a memory bank first. Want me to help you get started?',
  "/axiom-share":
    'To use `/axiom-share`, you\'ll need an active Axiom project first. Want me to help you set one up?',
  "/axiom-pulse":
    'To use `/axiom-pulse`, you\'ll need an active Axiom project first. Want me to help you get started?',
  "/axiom-spinup":
    'To use `/axiom-spinup`, you\'ll need specs, a plan, and a memory bank first. Want me to help you get started?',
  "/axiom-backend":
    'To use `/axiom-backend`, you\'ll need an active Axiom project with at least one backend installed. Want me to help you get started?',
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
  "/axiom-status-page":
    'To use `/axiom-status-page`, you\'ll need specs, a plan, and a memory bank first. Want me to help you get started?',
  "/axiom-release":
    'To use `/axiom-release`, you\'ll need evidence bundles and CI integration first. Want me to help you get there?',
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-2.8/task-2.8.1/step-2.8.1
  "/axiom-annotate":
    'To use `/axiom-annotate`, you\'ll need specs first. Want me to help you create your first spec with `/axiom-spec`?',
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-2.8/task-2.8.2/step-2.8.2
  "/axiom-render":
    'To use `/axiom-render`, you\'ll need specs, a plan, and a memory bank first. Want me to help you get started?',
}

/**
 * Checks whether a command is available at the given maturity level and returns
 * conversational guidance if it is above-level.
 *
 * REQ-PLG-022: When a user tries to use a command above their maturity level,
 * the plugin MUST respond conversationally.
 *
 * @param commandName - The slash-command name (e.g. "/axiom-run")
 * @param currentLevel - The current repo maturity level
 * @returns AboveLevelResult with status, optional guidance, and command info
 */
export function handleAboveLevelCommand(
  commandName: string,
  currentLevel: MaturityLevel,
): AboveLevelResult {
  const cmd = COMMAND_VISIBILITY_MATRIX.find((c) => c.name === commandName)

  if (!cmd) {
    return { status: "unknown" }
  }

  // Check if the command is available at the current level
  const isVisible =
    currentLevel >= cmd.minLevel &&
    (cmd.maxLevel === undefined || currentLevel <= cmd.maxLevel)

  if (isVisible) {
    return { status: "allowed", command: cmd }
  }

  // Command is above the current level — generate conversational guidance
  const guidance =
    ABOVE_LEVEL_GUIDANCE[commandName] ??
    `\`${commandName}\` isn't available yet at your current maturity level. Want me to help you advance?`

  return {
    status: "above_level",
    guidance,
    command: cmd,
    requiredLevel: cmd.minLevel,
  }
}
