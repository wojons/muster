/**
 * TUI Dashboard — Unicode box-drawing table renderer.
 *
 * Takes a `DashboardData` payload and returns a fully-formatted Unicode
 * box-drawing table string matching the spec example output.
 *
 * ## Key exports
 * - `renderDashboard(data, opts?)` — renders the full TUI table as a string.
 * - `renderDashboardJson(data, opts?)` — serializes `DashboardData` to JSON for CI/headless use.
 * - `renderSessionJumpInstructions(sessionId, rowNumber)` — manual session-switch fallback text.
 * - `MODEL_DISPLAY_ALIASES` — map of raw model IDs to short display names.
 * - `resolveModelDisplayName(rawModelId)` — resolves a raw model ID to its display alias.
 *
 * ## SLO targets (specs/89-TUI-Dashboard-Plugin.md)
 * - REQ-TUI-050: Dashboard open latency <1s (collect + first render).
 * - REQ-TUI-051: Refresh cycle latency <200ms (data collection + rendering for one frame).
 * - REQ-TUI-052: Memory overhead <5MB (dashboard state including session list).
 * - REQ-TUI-054: File read latency <10ms per file.
 * - REQ-TUI-056: Zero CPU when not visible — renderDashboard() is a pure function with no timers.
 *
 * ## Requirements implemented
 * - REQ-TUI-003: Aggregate stats bar + session table.
 * - REQ-TUI-010: Per-session column formatting.
 * - REQ-TUI-011: Budget alert indicators (⚠ at ≥80%, 🔴 at ≥100% of budgetUsd).
 * - REQ-TUI-013: Sessions with no cost data MUST display $0.00 / 0 tokens / 0s.
 * - REQ-TUI-014: The table MUST paginate at a configurable max rows (default: 20).
 *
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-003
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-003 plan=phase-2/task-2.1
 */

import type { DashboardData, SessionRow, SourceStatus } from "./types"
import { renderHtopHeader, renderSparkline, renderSparklineHistory, extractWorktree } from "./htop-header"
import type { WorktreeGroup } from "./htop-header"

// ---------------------------------------------------------------------------
// Model display aliases (re-exported from shared module — REQ-TUI-012)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-012 plan=step-verify-H004
// ---------------------------------------------------------------------------

export { MODEL_DISPLAY_ALIASES, resolveModelDisplayName } from "./model-aliases"

// ---------------------------------------------------------------------------
// Public interfaces
// ---------------------------------------------------------------------------

/**
 * Options controlling how the dashboard table is rendered.
 *
 * REQ-TUI-014: maxRows defaults to 20 for pagination.
 *
 * Edge-case flags (spec §10):
 *   dataDelayed       — show "⚠ Data may be delayed" indicator (SQLite locked / stale)
 *   permissionWarning — show a permission warning line when set
 *   showFreshnessAge  — show "last updated X min ago" per session (stale session files)
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#edge-cases plan=phase-2/task-2.5
 */
export interface RenderOptions {
  /** Terminal width in characters. Default: 90 (matches spec example). */
  width: number
  /** Max session rows before pagination. Default: 20 (REQ-TUI-014). */
  maxRows?: number
  /** Whether to include team rows in the output. Default: false. */
  includeTeam?: boolean
  /**
   * When true, show "⚠ Data may be delayed" indicator below the stats bar.
   * Set by the data layer when the SQLite DB was locked or data is stale.
   * Spec §10 edge case 2 (SQLite DB locked).
   */
  dataDelayed?: boolean
  /**
   * When set, show a "⚠ <permissionWarning>" line below the stats bar.
   * Set by the data layer when file-system permission errors are encountered.
   * Spec §10 edge case 9 (file system permissions).
   */
  permissionWarning?: string
  /**
   * When true, append "last updated X min ago" to each session row's status cell.
   * Spec §10 edge case 3 (stale session files).
   */
  showFreshnessAge?: boolean
  /**
   * When true, render the full interactive keyboard hints in the footer
   * ([r] Refresh │ [q] Close │ [t] Toggle team view).
   *
   * When false (default), render a non-interactive hint pointing to the
   * standalone CLI, since keyboard shortcuts do nothing in one-shot plugin
   * tool context.
   *
   * REQ-TUI-004: Plugin tool output must not show misleading keyboard hints.
   *
   * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-004 plan=step-verify-H002
   */
  isInteractive?: boolean
  /**
   * When true, render extended columns: Input/Output/Cache/Reasoning tokens,
   * tok/s, tool calls, messages. Uses wider layout.
   */
  extended?: boolean
}

/**
 * Result returned by `renderDashboard()`.
 *
 * `truncated` is true when the session list was cut at `maxRows`.
 */
export interface RenderResult {
  /** The full rendered table as a single string (newline-separated lines). */
  output: string
  /** True when sessions were truncated due to maxRows. */
  truncated: boolean
  /** Total sessions before any truncation. */
  totalSessions: number
}

// ---------------------------------------------------------------------------
// Column layout (fixed for v1 — REQ-TUI-003 spec example)
// ---------------------------------------------------------------------------

/**
 * Fixed column content widths for v1 (characters of actual content, excluding
 * the │ separators and the single space padding on each side).
 *
 * Derived from the spec example separator line:
 *   ├───┬──────────────┬──────────────┬──────────┬────────┬─────────┬──────────┬─────────────┤
 *
 * Each cell occupies: 1 (space) + contentWidth + 1 (space) = contentWidth + 2.
 * Total table width = sum(contentWidth + 2) + (numCols - 1 dividers) + 2 borders = 90.
 *
 * # (1) | Session (12) | Work Item (12) | Model (8) | Cost (6) | Tokens (7) | Duration (8) | Status (11)
 */
const COL = {
  num: 1,
  session: 12,
  workItem: 12,
  model: 8,
  cost: 6,
  tokens: 7,
  duration: 8,
  status: 11,
} as const

/**
 * Default terminal width matching the spec example (90 characters).
 *
 * Computed as: sum((colWidth + 2) for each col) + (numCols - 1) + 2 borders
 * = (3+14+14+10+8+9+10+13) + 7 + 2 = 88 + 7 + 2 = 90 (but inner = 88).
 *
 * Note: The spec example lines are 90 chars wide. The default width option
 * is set to 90 to match.
 */
const DEFAULT_WIDTH = 90

// ---------------------------------------------------------------------------
// Unicode box-drawing characters
// ---------------------------------------------------------------------------

const BOX = {
  topLeft: "┌",
  topRight: "┐",
  bottomLeft: "└",
  bottomRight: "┘",
  horizontal: "─",
  vertical: "│",
  teeDown: "┬",
  teeUp: "┴",
  teeRight: "├",
  teeLeft: "┤",
  cross: "┼",
} as const

// ---------------------------------------------------------------------------
// Formatting helpers
// ---------------------------------------------------------------------------

/**
 * Pad a string to exactly `width` characters.
 * Truncates with `…` if longer; pads with spaces if shorter.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-010 plan=phase-2/task-2.1
 */
function padCell(value: string, width: number): string {
  if (value.length > width) {
    // Truncate: replace last char with ellipsis
    return value.slice(0, width - 1) + "…"
  }
  return value.padEnd(width, " ")
}

/**
 * Format cost as `$X.XX` (plain, no budget indicator).
 *
 * Used for the aggregate stats bar where no per-session budget applies.
 *
 * REQ-TUI-010: Cost formatted as `$X.XX`.
 * REQ-TUI-013: Zero cost shows `$0.00`.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-010 plan=phase-2/task-2.1
 */
function formatCost(costUsd: number): string {
  return `$${costUsd.toFixed(2)}`
}

/**
 * Format cost as `$X.XX`, with optional budget alert indicator prefix.
 *
 * REQ-TUI-010: Cost formatted as `$X.XX`.
 * REQ-TUI-011: Budget alert indicators:
 *   - `⚠ $X.XX` when costUsd / budgetUsd >= 0.8 (approaching budget)
 *   - `🔴 $X.XX` when costUsd / budgetUsd >= 1.0 (over budget)
 * REQ-TUI-013: Zero cost shows `$0.00`.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-011 plan=phase-6/task-6.3
 */
function formatCostWithBudget(costUsd: number, budgetUsd?: number): string {
  const base = `$${costUsd.toFixed(2)}`
  if (budgetUsd !== undefined && budgetUsd > 0) {
    const ratio = costUsd / budgetUsd
    if (ratio >= 1.0) {
      return `🔴 ${base}`
    }
    if (ratio >= 0.8) {
      return `⚠ ${base}`
    }
  }
  return base
}

/**
 * Format token count as `XXK` for thousands, raw number for < 1000.
 *
 * REQ-TUI-010: Token count formatted as `XXK` for thousands.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-010 plan=phase-2/task-2.1
 */
function formatTokens(tokens: number): string {
  if (tokens >= 1000) {
    return `${Math.round(tokens / 1000)}K`
  }
  return String(tokens)
}

/**
 * Format duration in seconds as `Xm` for < 60 min, `Xh Ym` for >= 60 min.
 *
 * REQ-TUI-010: Duration formatted as `Xm` or `Xh Ym`.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-010 plan=phase-2/task-2.1
 */
function formatDuration(seconds: number): string {
  const totalMinutes = Math.floor(seconds / 60)
  if (totalMinutes < 60) {
    return `${totalMinutes}m`
  }
  const hours = Math.floor(totalMinutes / 60)
  const mins = totalMinutes % 60
  return `${hours}h ${mins}m`
}

/**
 * Format duration with ANSI age-alert color coding (extended renderer only).
 *
 * Inspired by atop's resource saturation coloring:
 *   - < 1 hour  : no color (normal)
 *   - 1–24 hours: yellow (\x1b[33m) — session may be forgotten
 *   - > 24 hours: red    (\x1b[31m) — session likely stuck or abandoned
 *
 * IMPORTANT: Only use in `renderDashboardExtended()`. The compact renderer
 * (`renderDashboard`) uses fixed-width box-drawing columns where ANSI codes
 * would break column alignment.
 *
 * REQ-TUI-072: Age alerts for long-running sessions.
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-072 plan=phase-10/task-10.7
 */
function formatDurationWithAge(seconds: number): string {
  const formatted = formatDuration(seconds)
  if (seconds >= 24 * 3600) {
    return `\x1b[31m${formatted}\x1b[0m`  // red: > 24h
  }
  if (seconds >= 3600) {
    return `\x1b[33m${formatted}\x1b[0m`  // yellow: 1–24h
  }
  return formatted  // normal: < 1h
}

/**
 * Format session status as a symbol + label string.
 *
 * REQ-TUI-010: Status indicator: "● Running", "○ Idle", "■ Stopped".
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-010 plan=phase-2/task-2.1
 */
function formatStatus(status: SessionRow["status"]): string {
  switch (status) {
    case "running":
      return "● Running"
    case "idle":
      return "○ Idle"
    case "stopped":
      return "■ Stopped"
    default:
      return "○ Idle"
  }
}

/**
 * Format aggregate token count for the stats bar (same K-rounding as cell).
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-003 plan=phase-2/task-2.1
 */
function formatAggTokens(tokens: number): string {
  if (tokens >= 1000) {
    return `${Math.round(tokens / 1000)}K`
  }
  return String(tokens)
}

// ---------------------------------------------------------------------------
// Box-drawing line builders
// ---------------------------------------------------------------------------

/**
 * Build a horizontal separator line using the given junction characters.
 *
 * `junctions` is an array of [position, char] pairs where position is the
 * 0-based index within the inner content area (between the left and right
 * border characters).
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-003 plan=phase-2/task-2.1
 */
function buildSeparatorLine(
  width: number,
  leftChar: string,
  rightChar: string,
  fillChar: string,
  junctions: Array<{ pos: number; char: string }>
): string {
  // Inner width = total width - 2 (for left and right border chars)
  const innerWidth = width - 2
  const chars = Array(innerWidth).fill(fillChar) as string[]

  for (const { pos, char } of junctions) {
    if (pos >= 0 && pos < innerWidth) {
      chars[pos] = char
    }
  }

  return leftChar + chars.join("") + rightChar
}

/**
 * Compute the junction positions for column separators.
 *
 * Each column occupies: 1 (space) + colWidth + 1 (space) = colWidth + 2 chars.
 * The separator between columns is at the cumulative position.
 *
 * Returns positions (0-based within inner content area) of each column divider.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-003 plan=phase-2/task-2.1
 */
function columnJunctionPositions(): number[] {
  // Each cell is: " " + content(colWidth) + " " = colWidth + 2
  // The divider is placed AFTER the trailing space of each column.
  const colWidths = [
    COL.num,
    COL.session,
    COL.workItem,
    COL.model,
    COL.cost,
    COL.tokens,
    COL.duration,
    COL.status,
  ]

  const positions: number[] = []
  let pos = 0
  for (let i = 0; i < colWidths.length - 1; i++) {
    // Each column cell: 1 space + colWidth + 1 space = colWidth + 2
    pos += colWidths[i] + 2
    positions.push(pos)
    // The divider char itself occupies 1 position; next column starts after it
    pos += 1
  }
  return positions
}

/**
 * Build a data row line with column values.
 *
 * Format: │ val1 │ val2 │ ... │
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-003 plan=phase-2/task-2.1
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-041 plan=phase-4/task-4.2
 */
function buildDataRow(cells: string[]): string {
  const colWidths = [
    COL.num,
    COL.session,
    COL.workItem,
    COL.model,
    COL.cost,
    COL.tokens,
    COL.duration,
    COL.status,
  ]

  const parts = cells.map((cell, i) => {
    const w = colWidths[i] ?? 0
    return ` ${padCell(cell, w)} `
  })

  return BOX.vertical + parts.join(BOX.vertical) + BOX.vertical
}

// ---------------------------------------------------------------------------
// Main renderer
// ---------------------------------------------------------------------------

/**
 * Render the dashboard table from a `DashboardData` payload.
 *
 * Returns a `RenderResult` with the full table string, truncation flag,
 * and total session count before truncation.
 *
 * REQ-TUI-003: Aggregate stats bar + session table.
 * REQ-TUI-010: Per-session column formatting.
 * REQ-TUI-013: Zero-cost sessions show $0.00 / 0 tokens / 0s.
 * REQ-TUI-014: Pagination via maxRows (default: 20).
 * Spec §10: Edge-case indicators (dataDelayed, permissionWarning, network error, truncation hint).
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#edge-cases plan=phase-2/task-2.5
 */
export function renderDashboard(
  data: DashboardData,
  opts?: Partial<RenderOptions>
): RenderResult {
  const width = opts?.width ?? DEFAULT_WIDTH
  const maxRows = opts?.maxRows ?? 20
  const includeTeam = opts?.includeTeam ?? false
  const dataDelayed = opts?.dataDelayed ?? false
  const permissionWarning = opts?.permissionWarning
  // Network error indicator: only shown when team view is enabled and network source has an error
  const networkError =
    includeTeam && data.sources.network.error ? data.sources.network.error : undefined

  // Filter sessions: exclude team rows unless includeTeam is set
  const allSessions = includeTeam
    ? data.sessions
    : data.sessions.filter((s) => !s.isTeam)

  const totalSessions = allSessions.length
  const truncated = totalSessions > maxRows
  const visibleSessions = truncated ? allSessions.slice(0, maxRows) : allSessions

  // -------------------------------------------------------------------------
  // Empty state
  // -------------------------------------------------------------------------
  if (totalSessions === 0) {
    return {
      output: renderEmptyState(width),
      truncated: false,
      totalSessions: 0,
    }
  }

  // -------------------------------------------------------------------------
  // Compute junction positions for column separators
  // -------------------------------------------------------------------------
  const junctionPositions = columnJunctionPositions()

  // -------------------------------------------------------------------------
  // Line 1: Top border with title
  // -------------------------------------------------------------------------
  const title = " Axiom Dashboard "
  const innerWidth = width - 2
  const titlePad = Math.max(0, innerWidth - title.length)
  const leftPad = Math.floor(titlePad / 2)
  const rightPad = titlePad - leftPad
  const topLine =
    BOX.topLeft +
    BOX.horizontal.repeat(leftPad) +
    title +
    BOX.horizontal.repeat(rightPad) +
    BOX.topRight

  // -------------------------------------------------------------------------
  // Line 2: Aggregate stats bar
  // REQ-TUI-072: Show running/idle/stopped counts in compact mode stats bar.
  // Use compact symbols (●=running, ○=idle, ■=stopped) to fit within innerWidth.
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-072 plan=F-002
  // -------------------------------------------------------------------------
  const agg = data.aggregates
  const runningCount = allSessions.filter((s) => s.status === "running").length
  const stoppedCount = allSessions.filter((s) => s.status === "stopped").length
  const idleCount = allSessions.filter((s) => s.status === "idle").length
  // Compact format: "●3 ○1 ■2" — only non-zero counts shown
  const statusParts = [
    ...(runningCount > 0 ? [`●${runningCount}`] : []),
    ...(idleCount > 0 ? [`○${idleCount}`] : []),
    ...(stoppedCount > 0 ? [`■${stoppedCount}`] : []),
  ]
  const statusSummary = statusParts.length > 0 ? statusParts.join(" ") : "●0"
  const statsContent =
    ` Sessions: ${agg.totalSessions} (${statusSummary})` +
    ` │ Total Cost: ${formatCost(agg.totalCostUsd)}` +
    ` │ Total Tokens: ${formatAggTokens(agg.totalTokens)}` +
    ` │ Avg Speed: ${Math.round(agg.avgSpeedTokensPerSec)} tok/s`
  // Clamp to innerWidth to prevent box-frame overflow (REQ-TUI-072)
  const statsContentClamped =
    statsContent.length > innerWidth
      ? statsContent.slice(0, innerWidth - 1) + "…"
      : statsContent
  const statsLine =
    BOX.vertical +
    statsContentClamped.padEnd(innerWidth, " ") +
    BOX.vertical

  // -------------------------------------------------------------------------
  // Lines 2a-2c: Optional warning indicator lines (spec §10 edge cases)
  //
  // Each warning line is a full-width box row: │ ⚠ <message>  │
  // Rendered immediately after the stats bar, before the column headers.
  //
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#edge-cases plan=phase-2/task-2.5
  // -------------------------------------------------------------------------
  const warningLines: string[] = []

  if (dataDelayed) {
    const msg = " ⚠ Data may be delayed"
    warningLines.push(BOX.vertical + msg.padEnd(innerWidth, " ") + BOX.vertical)
  }

  if (permissionWarning) {
    const msg = ` ⚠ ${permissionWarning}`
    warningLines.push(BOX.vertical + msg.padEnd(innerWidth, " ") + BOX.vertical)
  }

  if (networkError) {
    const msg = ` ⚠ Team data unavailable: ${networkError}`
    warningLines.push(BOX.vertical + msg.padEnd(innerWidth, " ") + BOX.vertical)
  }

  // -------------------------------------------------------------------------
  // Line 3: Column header top separator (├───┬──────────────┬ ... ┤)
  // -------------------------------------------------------------------------
  const headerTopJunctions = junctionPositions.map((pos) => ({
    pos: pos - 1, // -1 because buildSeparatorLine inner area starts at 0
    char: BOX.teeDown,
  }))
  const headerTopLine = buildSeparatorLine(
    width,
    BOX.teeRight,
    BOX.teeLeft,
    BOX.horizontal,
    headerTopJunctions
  )

  // -------------------------------------------------------------------------
  // Line 4: Column header row
  // -------------------------------------------------------------------------
  const headerRow = buildDataRow([
    "#",
    "Session",
    "Work Item",
    "Model",
    "Cost",
    "Tokens",
    "Duration",
    "Status",
  ])

  // -------------------------------------------------------------------------
  // Line 5: Header/data separator (├───┼──────────────┼ ... ┤)
  // -------------------------------------------------------------------------
  const headerDataJunctions = junctionPositions.map((pos) => ({
    pos: pos - 1,
    char: BOX.cross,
  }))
  const headerDataLine = buildSeparatorLine(
    width,
    BOX.teeRight,
    BOX.teeLeft,
    BOX.horizontal,
    headerDataJunctions
  )

  // -------------------------------------------------------------------------
  // Lines 6..N: Session data rows
  // REQ-TUI-011: Budget alert indicators (⚠ at ≥80%, 🔴 at ≥100% of budgetUsd).
  // REQ-TUI-041: Team rows show teamMemberId, "—" for cost/tokens, "~" prefix in # column.
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-011 plan=phase-6/task-6.3
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-041 plan=phase-4/task-4.2
  // -------------------------------------------------------------------------
  const dataRows = visibleSessions.map((session, idx) => {
    const isTeam = session.isTeam === true
    // REQ-TUI-041: Team rows use "~" prefix in # column; local rows use plain number
    // Note: COL.num=1 so only 1 char fits — "~" is used as the team indicator
    const num = isTeam ? "~" : String(idx + 1)
    // REQ-TUI-041: Team rows show teamMemberId (not session title)
    const sessionTitle = isTeam
      ? (session.teamMemberId ?? session.sessionId)
      : (session.title ?? session.sessionId)
    const workItem = session.workItemId ?? "—"
    const model = session.model
    // REQ-TUI-041: Team rows show "—" for cost and tokens (hidden per spec)
    // REQ-TUI-011: Local rows show budget alert indicator when budgetUsd is set
    const cost = isTeam ? "—" : formatCostWithBudget(session.costUsd, session.budgetUsd)
    const tokens = isTeam ? "—" : formatTokens(session.totalTokens)
    const duration = formatDuration(session.durationSeconds)
    const status = formatStatus(session.status)

    return buildDataRow([num, sessionTitle, workItem, model, cost, tokens, duration, status])
  })

  // -------------------------------------------------------------------------
  // Footer separator (├───┴──────────────┴ ... ┤)
  // -------------------------------------------------------------------------
  const footerSepJunctions = junctionPositions.map((pos) => ({
    pos: pos - 1,
    char: BOX.teeUp,
  }))
  const footerSepLine = buildSeparatorLine(
    width,
    BOX.teeRight,
    BOX.teeLeft,
    BOX.horizontal,
    footerSepJunctions
  )

  // -------------------------------------------------------------------------
  // Footer keyboard hints
  //
  // In non-interactive (plugin tool) mode, keyboard shortcuts do nothing, so
  // we show a helpful note pointing to the standalone CLI instead of
  // misleading hints. In interactive (--watch) mode, the full hint set is shown.
  //
  // REQ-TUI-004: Plugin tool output must not show misleading keyboard hints.
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-004 plan=step-verify-H002
  // -------------------------------------------------------------------------
  const maxRowNum = Math.min(visibleSessions.length, 9)
  const jumpHint = maxRowNum > 0 ? `[1-${maxRowNum}] Jump to session` : "[1-9] Jump to session"
  const footerContent = opts?.isInteractive
    ? ` ${jumpHint} │ [r] Refresh │ [q] Close │ [t] Toggle team view`
    : ` ${jumpHint} │ Use --watch for live refresh │ axiom-dashboard --help`
  const footerLine =
    BOX.vertical +
    footerContent.padEnd(innerWidth, " ") +
    BOX.vertical

  // -------------------------------------------------------------------------
  // Optional truncation hint line (spec §10 edge case 6: high session count)
  //
  // Shown when sessions were truncated at maxRows.
  // Format: │ Showing N of M sessions. Add [a] to show all.  │
  //
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#edge-cases plan=phase-2/task-2.5
  // -------------------------------------------------------------------------
  const truncationLines: string[] = []
  if (truncated) {
    const truncMsg = ` Showing ${visibleSessions.length} of ${totalSessions} sessions. Add [a] to show all.`
    truncationLines.push(BOX.vertical + truncMsg.padEnd(innerWidth, " ") + BOX.vertical)
  }

  // -------------------------------------------------------------------------
  // Bottom border
  // -------------------------------------------------------------------------
  const bottomLine =
    BOX.bottomLeft + BOX.horizontal.repeat(innerWidth) + BOX.bottomRight

  // -------------------------------------------------------------------------
  // Assemble
  // -------------------------------------------------------------------------
  const lines = [
    topLine,
    statsLine,
    ...warningLines,
    headerTopLine,
    headerRow,
    headerDataLine,
    ...dataRows,
    footerSepLine,
    footerLine,
    ...truncationLines,
    bottomLine,
  ]

  return {
    output: lines.join("\n"),
    truncated,
    totalSessions,
  }
}

// ---------------------------------------------------------------------------
// JSON output mode (REQ-TUI-005)
// ---------------------------------------------------------------------------

/**
 * Options for `renderDashboardJson()`.
 *
 * REQ-TUI-005: JSON output mode for non-interactive (CI/headless) contexts.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-005 plan=phase-2/task-2.4
 */
export interface JsonOutputOptions {
  /** When true (default), output is pretty-printed with 2-space indentation. */
  pretty?: boolean
}

/**
 * Serialize a `SourceStatus` object, converting `lastUpdate` Date → ISO 8601
 * string or null.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-005 plan=phase-2/task-2.4
 */
function serializeSourceStatus(s: SourceStatus): object {
  return {
    available: s.available,
    lastUpdate: s.lastUpdate !== null ? s.lastUpdate.toISOString() : null,
    ...(s.error !== undefined ? { error: s.error } : {}),
  }
}

/**
 * Render `DashboardData` as a JSON string suitable for piping to `jq`.
 *
 * Transformations applied:
 * - `sessions[].updatedAt` → ISO 8601 string
 * - `sources.*.lastUpdate` → ISO 8601 string or null
 * - `generatedAt` → ISO 8601 string
 *
 * REQ-TUI-005: JSON output mode for non-interactive (CI/headless) contexts.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-005 plan=phase-2/task-2.4
 */
export function renderDashboardJson(data: DashboardData, opts?: JsonOutputOptions): string {
  const pretty = opts?.pretty ?? true

  const payload = {
    sessions: data.sessions.map((s) => ({
      sessionId: s.sessionId,
      title: s.title,
      workItemId: s.workItemId,
      model: s.model,
      modelRaw: s.modelRaw,
      costUsd: s.costUsd,
      totalTokens: s.totalTokens,
      durationSeconds: s.durationSeconds,
      status: s.status,
      source: s.source,
      updatedAt: s.updatedAt.toISOString(),
      isTeam: s.isTeam,
      teamMemberId: s.teamMemberId,
      budgetUsd: s.budgetUsd,
    })),
    aggregates: {
      totalSessions: data.aggregates.totalSessions,
      totalCostUsd: data.aggregates.totalCostUsd,
      totalTokens: data.aggregates.totalTokens,
      avgSpeedTokensPerSec: data.aggregates.avgSpeedTokensPerSec,
    },
    sources: {
      inMemory: serializeSourceStatus(data.sources.inMemory),
      file: serializeSourceStatus(data.sources.file),
      network: serializeSourceStatus(data.sources.network),
    },
    generatedAt: data.generatedAt.toISOString(),
  }

  return pretty ? JSON.stringify(payload, null, 2) : JSON.stringify(payload)
}

// ---------------------------------------------------------------------------
// Empty state renderer
// ---------------------------------------------------------------------------

/**
 * Render an empty-state table when there are no sessions.
 *
 * Returns a minimal box with the spec §10 "No active sessions" message and
 * a `/axiom-run` hint so users know how to start a session.
 *
 * Spec §10 edge case 1: Zero sessions — show helpful message, no broken table.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#edge-cases plan=phase-2/task-2.5
 */
function renderEmptyState(width: number): string {
  const innerWidth = width - 2
  const title = " Axiom Dashboard "
  const titlePad = Math.max(0, innerWidth - title.length)
  const leftPad = Math.floor(titlePad / 2)
  const rightPad = titlePad - leftPad

  const topLine =
    BOX.topLeft +
    BOX.horizontal.repeat(leftPad) +
    title +
    BOX.horizontal.repeat(rightPad) +
    BOX.topRight

  // Spec §10 edge case 1: exact message text required
  const emptyMsg = " No active sessions. Start one with `/axiom-run` or open a new OpenCode instance."
  const emptyLine =
    BOX.vertical + emptyMsg.padEnd(innerWidth, " ") + BOX.vertical

  const bottomLine =
    BOX.bottomLeft + BOX.horizontal.repeat(innerWidth) + BOX.bottomRight

  return [topLine, emptyLine, bottomLine].join("\n")
}

// ---------------------------------------------------------------------------
// Session jump instructions (REQ-TUI-031)
// ---------------------------------------------------------------------------

/**
 * Return manual session-switching instructions for a given session.
 *
 * Because the plugin SDK does not support programmatic session switching
 * (REQ-TUI-030), the dashboard surfaces the session ID and guides the user
 * through the manual "File → Open Session" flow instead.
 *
 * REQ-TUI-031: If programmatic session switching is not available, display
 * the session ID clearly and provide instructions for manual switching.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-031 plan=phase-3/task-3.3
 *
 * @param sessionId  The raw session ID to display.
 * @param rowNumber  The 1-based row number shown in the dashboard # column.
 * @returns          A multi-line string with the session ID and switch instructions.
 */
export function renderSessionJumpInstructions(sessionId: string, rowNumber: number): string {
  return [
    `Session ${rowNumber}: ${sessionId}`,
    "To switch: in OpenCode, use File → Open Session and paste the session ID above.",
    "(Programmatic session switching is not available in the current SDK version.)",
  ].join("\n")
}

// ---------------------------------------------------------------------------
// Extended mode renderer
// ---------------------------------------------------------------------------

/**
 * Render a compact tool call bar using Unicode block elements.
 * Scaled to maxToolCalls across all visible sessions.
 * Width: 4 chars max (fits in the Tools column alongside the number).
 *
 * Inspired by iotop's per-process I/O bars — lets users see at a glance
 * which sessions are tool-heavy without reading raw numbers.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-071 plan=phase-10/task-10.4
 */
export function renderToolBar(toolCalls: number, maxToolCalls: number): string {
  if (maxToolCalls <= 0 || toolCalls <= 0) return "    "  // 4 spaces
  const BAR_WIDTH = 4
  const BLOCKS = ["▏", "▎", "▍", "▌", "▋", "▊", "▉", "█"]
  const ratio = Math.min(toolCalls / maxToolCalls, 1)
  const filled = Math.floor(ratio * BAR_WIDTH)
  const partialIdx = Math.floor((ratio * BAR_WIDTH - filled) * BLOCKS.length)

  let bar = ""
  for (let i = 0; i < BAR_WIDTH; i++) {
    if (i < filled) {
      bar += "\x1b[35m█\x1b[0m"  // magenta for filled blocks
    } else if (i === filled && partialIdx > 0) {
      bar += "\x1b[35m" + BLOCKS[partialIdx]! + "\x1b[0m"  // magenta partial block
    } else {
      bar += "\x1b[2m·\x1b[0m"  // dim dot for empty
    }
  }
  return bar
}

/**
 * Format a token count for extended display (more precise than formatTokens).
 * Shows exact K/M values: 167K, 79.6M, 6.2M, 327
 */
function formatTokensExtended(tokens: number): string {
  if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(1)}M`
  if (tokens >= 1_000) return `${Math.round(tokens / 1_000)}K`
  return String(tokens)
}

/**
 * Render the dashboard in extended mode — wider table with full token breakdown.
 *
 * Shows: #, Session Title, Model, Cost, Output, Input, Cache, Reasoning, Tools, Msgs, tok/s, Duration, Status
 *
 * Uses the full terminal width and doesn't truncate session titles aggressively.
 */
export function renderDashboardExtended(
  data: DashboardData,
  opts?: Partial<RenderOptions>
): RenderResult {
  const maxRows = opts?.maxRows ?? 20
  const includeTeam = opts?.includeTeam ?? false

  const allSessions = includeTeam
    ? data.sessions
    : data.sessions.filter((s) => !s.isTeam)

  const totalSessions = allSessions.length
  const truncated = totalSessions > maxRows
  const visibleSessions = truncated ? allSessions.slice(0, maxRows) : allSessions

  if (totalSessions === 0) {
    return {
      output: "No active sessions. Start one with opencode or /axiom-run.",
      truncated: false,
      totalSessions: 0,
    }
  }

  // Compute max tok/s for sparkline scaling
  const maxTps = Math.max(...allSessions.map((s) => s.tokensPerSecond ?? 0), 1)

  // Compute max tool calls for bar scaling (task-10.4)
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-071 plan=phase-10/task-10.4
  const maxToolCalls = Math.max(...visibleSessions.map((s) => s.toolCalls ?? 0), 1)

  // Build rows as arrays of strings for column alignment
  const header = [
    "#", "⚡", "Session", "Worktree", "Model", "Cost", "Output", "Cache R",
    "Tools", "Msgs", "tok/s", "Duration", "Status"
  ]

  const rows: string[][] = []
  const sparklines: string[] = []  // Separate because ANSI codes break width calc
  const toolBars: string[] = []    // Separate because ANSI codes break width calc

  for (let i = 0; i < visibleSessions.length; i++) {
    const s = visibleSessions[i]!
    const num = String(i + 1)
    const title = s.title ?? s.sessionId
    const titleDisplay = title.length > 38 ? title.slice(0, 37) + "…" : title
    const worktree = extractWorktree(s.directory ?? "")
    const wtDisplay = worktree.length > 16 ? worktree.slice(0, 15) + "…" : worktree
    const model = s.model || s.modelRaw || "—"
    const modelDisplay = model.length > 20 ? model.slice(0, 19) + "…" : model
    const cost = s.isTeam ? "—" : `$${s.costUsd.toFixed(2)}`
    const output = s.isTeam ? "—" : formatTokensExtended(s.outputTokens ?? 0)
    const cacheR = s.isTeam ? "—" : formatTokensExtended(s.cacheReadTokens ?? 0)
    // Tool calls: bar + number (task-10.4)
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-071 plan=phase-10/task-10.4
    const toolsNum = s.toolCalls ?? 0
    const tools = s.isTeam ? "—" : String(toolsNum).padStart(4)
    const msgs = s.isTeam ? "—" : String(s.messageCount ?? 0)
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-073 plan=F-001
    const tps = s.isTeam ? "—" : (s.tokensPerSecond ? `${Number(s.tokensPerSecond).toFixed(3)}` : "0.000")
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-072 plan=phase-10/task-10.7
    const dur = formatDurationWithAge(s.durationSeconds)
    const status = formatStatus(s.status)

    // Sparkline is rendered separately (ANSI codes break width calc)
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-120 plan=phase-10/task-10.5
    // Use multi-char history sparkline when tpsSamples is available (watch mode); fall back to single-char.
    const sparkChar = s.tpsSamples && s.tpsSamples.length > 1
      ? renderSparklineHistory(s.tpsSamples, maxTps)
      : renderSparkline(s.tokensPerSecond ?? 0, maxTps)
    sparklines.push(sparkChar)
    // Tool bar is rendered separately (ANSI codes break width calc)
    toolBars.push(s.isTeam ? "    " : renderToolBar(toolsNum, maxToolCalls))

    rows.push([num, " ", titleDisplay, wtDisplay, modelDisplay, cost, output, cacheR, tools, msgs, tps, dur, status])
  }

  // Calculate column widths (max of header and all rows per column)
  const colWidths = header.map((h, ci) => {
    let max = h.length
    for (const row of rows) {
      if (row[ci]!.length > max) max = row[ci]!.length
    }
    return max
  })

  // Build the table
  const pad = (s: string, w: number) => s + " ".repeat(Math.max(0, w - s.length))
  const padR = (s: string, w: number) => " ".repeat(Math.max(0, w - s.length)) + s

  // Right-align numeric columns (5-12), left-align text (0-4, 13)
  const rightAlignCols = new Set([5, 6, 7, 8, 9, 10, 11])

  const formatRow = (cells: string[], sparkline?: string, toolBar?: string) => {
    const formatted = cells.map((c, i) => {
      if (i === 1) return sparkline ?? " "  // Replace placeholder with sparkline
      // Column 8 = Tools: prepend the tool bar (4 chars + space) before the right-aligned number
      if (i === 8) return (toolBar ?? "    ") + " " + (rightAlignCols.has(i) ? padR(c, colWidths[i]!) : pad(c, colWidths[i]!))
      return rightAlignCols.has(i) ? padR(c, colWidths[i]!) : pad(c, colWidths[i]!)
    })
    return formatted.join("  ")
  }

  const lines: string[] = []

  // htop-style header with colored model bars
  if (data.modelUsage && data.modelUsage.length > 0) {
    lines.push(renderHtopHeader(data.modelUsage, data.aggregates, allSessions))
  } else {
    // Fallback: plain stats bar
    const agg = data.aggregates
    const running = allSessions.filter((s) => s.status === "running").length
    const stopped = allSessions.filter((s) => s.status === "stopped").length
    lines.push("")
    lines.push(`  Axiom Dashboard — ${totalSessions} sessions (${running} running, ${stopped} stopped)`)
    lines.push(`  $${agg.totalCostUsd.toFixed(2)} total │ ${formatTokensExtended(agg.totalTokens)} tokens │ ${Number(agg.avgSpeedTokensPerSec).toFixed(3)} tok/s avg`)
  }

  lines.push("")

  // Table header
  lines.push("  " + formatRow(header))
  lines.push("  " + colWidths.map(w => "─".repeat(w)).join("──"))

  // Data rows with sparklines and tool bars
  for (let i = 0; i < rows.length; i++) {
    lines.push("  " + formatRow(rows[i]!, sparklines[i], toolBars[i]))
  }

  // Footer
  lines.push("")
  if (truncated) {
    lines.push(`  Showing ${maxRows} of ${totalSessions} sessions. Use --max-rows ${totalSessions} to show all.`)
  }
  lines.push(`  [1-${Math.min(visibleSessions.length, 9)}] Jump to session │ --watch for live refresh │ --help for options`)
  lines.push("")

  return {
    output: lines.join("\n"),
    truncated,
    totalSessions,
  }
}

// ---------------------------------------------------------------------------
// Worktree group renderer (task-10.6)
// ---------------------------------------------------------------------------

/**
 * Render sessions grouped by worktree with summary header rows.
 * Used when --group worktree is specified.
 *
 * Each group shows a dim summary row (worktree name, session count, total cost,
 * total output tokens, total tool calls) followed by compact per-session rows.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-110 plan=phase-10/task-10.6
 */
export function renderWorktreeGroups(
  groups: WorktreeGroup[],
  opts?: Partial<RenderOptions>
): string {
  const maxRows = opts?.maxRows ?? 20
  const lines: string[] = []
  let rowsShown = 0

  for (const group of groups) {
    if (rowsShown >= maxRows) break

    // Summary row with dim styling
    lines.push(
      `\x1b[2m  ▶ ${group.worktree.padEnd(20)} ` +
      `${group.sessionCount} sessions  ` +
      `$${group.totalCostUsd.toFixed(2)}  ` +
      `${formatTokensExtended(group.totalOutputTokens)} out  ` +
      `${group.totalToolCalls} tools\x1b[0m`
    )

    // Individual session rows (compact format)
    for (const s of group.sessions) {
      if (rowsShown >= maxRows) break
      const title = (s.title ?? s.sessionId).slice(0, 30)
      const model = s.model.slice(0, 15)
      const cost = `$${s.costUsd.toFixed(2)}`
      const status =
        s.status === "running" ? "\x1b[32m●\x1b[0m" :
        s.status === "idle"    ? "\x1b[33m○\x1b[0m" :
                                 "\x1b[90m■\x1b[0m"
      lines.push(`    ${status} ${title.padEnd(32)} ${model.padEnd(16)} ${cost.padStart(8)}`)
      rowsShown++
    }

    lines.push("")
  }

  return lines.join("\n")
}
