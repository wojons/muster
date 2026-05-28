#!/usr/bin/env bun
/**
 * Standalone TUI Dashboard CLI
 * Usage: axiom-dashboard [options]
 *
 * Options:
 *   --json          Output JSON instead of table
 *   --plain         Output tab-separated values (grep/awk friendly)
 *   --watch         Auto-refresh every 5 seconds (REQ-TUI-023)
 *   --team          Include team sessions (requires AXIOM_API_TOKEN + AXIOM_BACKEND_URL)
 *   --max-rows N    Maximum rows to show (default: 20)
 *   --repo PATH     Path to repo root (default: current directory)
 *   --help          Show this help
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-023 plan=phase-5/task-5.1
 */

import { resolve } from "node:path"
import { createInterface } from "node:readline"
import { FileSource } from "./sources/file"
import { SessionDataCollector } from "./collector"
import {
  renderDashboard,
  renderDashboardExtended,
  renderDashboardJson,
  renderWorktreeGroups,
} from "./renderer"
import type { DashboardData, SessionRow } from "./types"

// ---------------------------------------------------------------------------
// Help text
// ---------------------------------------------------------------------------

const HELP_TEXT = `
axiom-dashboard — htop-style session dashboard for OpenCode

Usage: axiom-dashboard [options]

Display:
  --extended, -x     Extended mode: input/output/cache tokens, tok/s, tools, messages
  --max-rows N       Maximum sessions to show (default: 20)
  --sort FIELD       Sort by: updated (default), cost, tokens, duration, name, status
  --reverse, -r      Reverse sort order (ascending instead of descending)
  --group worktree   Group sessions by worktree with summary rows (glances-style)

Filtering:
  --since DURATION   Only show sessions from the last duration (e.g., 1h, 24h, 7d, 2w)
  --repo PATH        Only show sessions for this repo directory

Output:
  --json             Output JSON (pipe to jq)
  --plain            Tab-separated values (pipe to awk/grep)

Live:
  --watch            Auto-refresh (default: every 5 seconds)
  --interval N       Refresh interval in seconds for --watch (default: 5)

Other:
  --team             Include team sessions (requires AXIOM_API_TOKEN + AXIOM_BACKEND_URL)
  --help, -h         Show this help

Examples:
  axiom-dashboard                       Show compact table
  axiom-dashboard -x                    Extended mode with full token breakdown
  axiom-dashboard --since 24h           Sessions from the last 24 hours
  axiom-dashboard --since 7d -x         Last 7 days, extended view
  axiom-dashboard --sort cost           Sort by cost (highest first)
  axiom-dashboard --sort tokens -r      Sort by tokens (lowest first)
  axiom-dashboard --watch -x            Live extended mode, refresh every 5s
  axiom-dashboard --watch --interval 2  Live mode, refresh every 2s
  axiom-dashboard --max-rows 50         Show up to 50 sessions
  axiom-dashboard --group worktree      Group by worktree with summary rows
  axiom-dashboard --json | jq '.sessions[0]'
  axiom-dashboard --plain | awk '{print $3}'
`.trim()

// ---------------------------------------------------------------------------
// Arg parsing
// ---------------------------------------------------------------------------

/**
 * Parsed CLI arguments.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-023 plan=phase-5/task-5.1
 */
export interface CliArgs {
  json: boolean
  plain: boolean
  watch: boolean
  team: boolean
  extended: boolean
  maxRows: number
  repoRoot: string
  help: boolean
  sort: "updated" | "cost" | "tokens" | "duration" | "name" | "status"
  reverse: boolean
  since: string | undefined  // e.g., "24h", "7d", "2w"
  interval: number           // watch refresh interval in seconds
  /** Group sessions by worktree with summary rows. Default: "none". */
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-110 plan=phase-10/task-10.6
  group: "worktree" | "none"
}

/**
 * Parse raw argv array into CliArgs.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-023 plan=phase-5/task-5.1
 */
export function parseArgs(argv: string[], defaultRepoRoot?: string): CliArgs {
  const args: CliArgs = {
    json: false,
    plain: false,
    watch: false,
    team: false,
    extended: false,
    maxRows: 20,
    repoRoot: defaultRepoRoot ?? process.cwd(),
    help: false,
    sort: "updated",
    reverse: false,
    since: undefined,
    interval: 5,
    group: "none",
  }

  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i]
    switch (arg) {
      case "--json":
        args.json = true
        break
      case "--plain":
        args.plain = true
        break
      case "--watch":
        args.watch = true
        break
      case "--team":
        args.team = true
        break
      case "--extended":
      case "-x":
        args.extended = true
        break
      case "--help":
      case "-h":
        args.help = true
        break
      case "--max-rows": {
        const next = argv[i + 1]
        if (next !== undefined && /^\d+$/.test(next)) {
          args.maxRows = parseInt(next, 10)
          i++
        }
        break
      }
      case "--repo": {
        const next = argv[i + 1]
        if (next !== undefined && !next.startsWith("--")) {
          args.repoRoot = resolve(next)
          i++
        }
        break
      }
      case "--sort": {
        const next = argv[i + 1]
        if (next && ["updated", "cost", "tokens", "duration", "name", "status"].includes(next)) {
          args.sort = next as CliArgs["sort"]
          i++
        }
        break
      }
      case "--reverse":
      case "-r":
        args.reverse = true
        break
      case "--since": {
        const next = argv[i + 1]
        if (next && /^\d+[hdwm]$/.test(next)) {
          args.since = next
          i++
        }
        break
      }
      case "--interval": {
        const next = argv[i + 1]
        if (next !== undefined && /^\d+$/.test(next)) {
          args.interval = Math.max(1, parseInt(next, 10))
          i++
        }
        break
      }
      case "--group": {
        const next = argv[i + 1]
        // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-110 plan=phase-10/task-10.6
        if (next === "worktree" || next === "none") {
          args.group = next
          i++
        }
        break
      }
      default:
        // Unknown flags are silently ignored
        break
    }
  }

  return args
}

// ---------------------------------------------------------------------------
// Data collection
// ---------------------------------------------------------------------------

/**
 * Collect dashboard data using SQLiteSource (primary) + FileSource (fallback).
 *
 * SQLiteSource reads from OpenCode's SQLite DB (~/.local/share/opencode/opencode.db)
 * which has real cost, tokens, model, title, and duration data.
 * FileSource reads from .axiom/sessions/*.json as a fallback.
 *
 * Exported so main() can call it once and share the result with both
 * renderDashboard and the interactive selector — avoiding double I/O and
 * TOCTOU inconsistency (NEW-006).
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-023 plan=step-final-002
 */
export async function collectData(args: CliArgs, _sqliteDbPath?: string): Promise<DashboardData> {
  const { SQLiteSource } = await import("./sources/sqlite")

  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-082 plan=phase-10/task-10.1
  // Parse --since into epoch-ms cutoff and pass it to SQLiteSource so filtering
  // happens at the DB level BEFORE the LIMIT (fixes --since on large databases).
  const sinceMs = args.since ? parseSinceDuration(args.since) : undefined

  const sqliteSource = new SQLiteSource({
    filterDirectory: args.repoRoot !== process.cwd() ? args.repoRoot : undefined,
    dbPath: _sqliteDbPath,  // undefined = use default; "/nonexistent" = skip DB in tests
    sinceMs,                // DB-level time-range filter (REQ-TUI-082)
  })

  const fileSource = new FileSource({ repoRoot: args.repoRoot })

  // SQLiteSource is primary (priority=4), FileSource is fallback (priority=2)
  // Collector deduplicates by sessionId, keeping the higher-priority source
  const sources: import("./types").SessionDataSource[] = [sqliteSource, fileSource]

  // NetworkSource: only if --team and env vars are set
  // (NetworkSource is not yet implemented; this is a placeholder for future use)
  if (args.team) {
    const apiToken = process.env["AXIOM_API_TOKEN"]
    const backendUrl = process.env["AXIOM_BACKEND_URL"]
    if (!apiToken || !backendUrl) {
      // Warn but continue — team data will simply be absent
      process.stderr.write(
        "Warning: --team requires AXIOM_API_TOKEN and AXIOM_BACKEND_URL env vars.\n"
      )
    }
    // NetworkSource would be added here when implemented
  }

  const collector = new SessionDataCollector()
  const data = await collector.collect({
    repoRoot: args.repoRoot,
    includeTeam: args.team,
    sources,
  })

  // Fetch model usage breakdown for htop-style header meters
  // Only needed in extended mode (--extended / -x) — skip for --plain, --json, compact mode
  if (args.extended && sqliteSource.isAvailable()) {
    try {
      data.modelUsage = await sqliteSource.getModelUsage()
    } catch { /* non-critical */ }
  }

  // --since: filter sessions by time range
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-082 plan=phase-10/step-p10-M003
  // Reuse sinceMs computed above (line 234) — eliminates second parseSinceDuration() call
  // and the TOCTOU drift between the two Date.now() invocations.
  if (sinceMs) {
    data.sessions = data.sessions.filter((s) => s.updatedAt.getTime() >= sinceMs)
    // Recompute aggregates after filtering
    data.aggregates = computeAggregates(data.sessions)
  }

  // --sort: sort sessions
  const sortFn = getSortFunction(args.sort, args.reverse)
  if (sortFn) {
    data.sessions.sort(sortFn)
  }

  return data
}

/**
 * Parse a --since duration string (e.g., "24h", "7d", "2w", "3m") into a
 * cutoff timestamp (epoch ms). Returns undefined if the format is invalid.
 */
function parseSinceDuration(since: string): number | undefined {
  const match = since.match(/^(\d+)([hdwm])$/)
  if (!match) return undefined
  const value = parseInt(match[1]!, 10)
  const unit = match[2]!
  const now = Date.now()
  switch (unit) {
    case "h": return now - value * 60 * 60 * 1000
    case "d": return now - value * 24 * 60 * 60 * 1000
    case "w": return now - value * 7 * 24 * 60 * 60 * 1000
    case "m": return now - value * 30 * 24 * 60 * 60 * 1000
    default: return undefined
  }
}

/**
 * Recompute aggregates from a filtered session list.
 */
function computeAggregates(sessions: import("./types").SessionRow[]): import("./types").DashboardAggregates {
  const nonTeam = sessions.filter((s) => !s.isTeam)
  const totalCostUsd = nonTeam.reduce((sum, s) => sum + s.costUsd, 0)
  const totalTokens = nonTeam.reduce((sum, s) => sum + s.totalTokens, 0)
  const totalDuration = nonTeam.reduce((sum, s) => sum + s.durationSeconds, 0)
  return {
    totalSessions: nonTeam.length,
    totalCostUsd,
    totalTokens,
    avgSpeedTokensPerSec: totalDuration > 0 ? totalTokens / totalDuration : 0,
  }
}

/**
 * Return a sort comparator for the given field and direction.
 */
function getSortFunction(
  field: CliArgs["sort"],
  reverse: boolean
): ((a: import("./types").SessionRow, b: import("./types").SessionRow) => number) | undefined {
  const dir = reverse ? -1 : 1  // default descending (highest first); --reverse flips to ascending
  switch (field) {
    case "updated":
      return (a, b) => dir * (b.updatedAt.getTime() - a.updatedAt.getTime())
    case "cost":
      return (a, b) => dir * (b.costUsd - a.costUsd)
    case "tokens":
      return (a, b) => dir * (b.totalTokens - a.totalTokens)
    case "duration":
      return (a, b) => dir * (b.durationSeconds - a.durationSeconds)
    case "name":
      return (a, b) => dir * (a.title ?? "").localeCompare(b.title ?? "")
    case "status": {
      // running=0 (top), idle=1, stopped=2 — so running sessions float to top by default
      const statusOrder: Record<string, number> = { running: 0, idle: 1, stopped: 2 }
      return (a, b) => dir * ((statusOrder[a.status] ?? 9) - (statusOrder[b.status] ?? 9))
    }
    default:
      return undefined
  }
}

// ---------------------------------------------------------------------------
// Current tok/s helper (task-10.2)
// ---------------------------------------------------------------------------

/**
 * Compute current tok/s from delta between two snapshots.
 * Returns undefined if delta is negative (session reset) or elapsed time is 0.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-073 plan=phase-10/task-10.2
 */
export function computeCurrentTps(
  currentOutput: number,
  prevOutput: number,
  elapsedMs: number
): number | undefined {
  if (elapsedMs <= 0) return undefined
  const delta = currentOutput - prevOutput
  if (delta < 0) return undefined  // session reset or data inconsistency
  return delta / (elapsedMs / 1000)
}

// ---------------------------------------------------------------------------
// Output formatters
// ---------------------------------------------------------------------------

/**
 * Render plain tab-separated output (one line per session, no header).
 * Format: sessionId\tmodel\tcostUsd\ttotalTokens\tdurationSeconds\tstatus
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-023 plan=phase-5/task-5.4
 */
export function renderPlain(sessions: SessionRow[]): string {
  if (sessions.length === 0) return ""
  return sessions
    .map((s) =>
      [
        s.sessionId,
        s.model,
        s.costUsd.toFixed(2),
        String(s.totalTokens),
        String(s.durationSeconds),
        s.status,
      ].join("\t")
    )
    .join("\n")
}

// ---------------------------------------------------------------------------
// Interactive selector (task-5.2)
// ---------------------------------------------------------------------------

/**
 * Prompt the user to enter a session number and print jump instructions.
 * Only runs when stdout is a TTY and not in watch mode.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-031 plan=phase-5/task-5.2
 */
async function runInteractiveSelector(sessions: SessionRow[]): Promise<void> {
  if (sessions.length === 0) return

  const rl = createInterface({
    input: process.stdin,
    output: process.stdout,
    terminal: false,
  })

  return new Promise((resolve) => {
    process.stdout.write("\nEnter session number to jump into (or press Enter to exit): ")

    rl.once("line", (line) => {
      rl.close()
      const trimmed = line.trim()
      if (trimmed === "") {
        resolve()
        return
      }
      const num = parseInt(trimmed, 10)
      if (!isNaN(num) && num >= 1 && num <= sessions.length) {
        const session = sessions[num - 1]
        if (session) {
          // Show full session ID (not truncated) and attach command
          const lines = [
            "",
            `Session ${num}: ${session.sessionId}`,
            `Title: ${session.title ?? "(untitled)"}`,
            "",
            "To jump into this session, run:",
            "",
            `  opencode attach http://localhost:4096 --session ${session.sessionId}`,
            "",
            "Or copy the session ID:",
            `  ${session.sessionId}`,
            "",
          ]
          process.stdout.write(lines.join("\n") + "\n")
        }
      }
      resolve()
    })

    rl.once("close", () => resolve())
  })
}

// ---------------------------------------------------------------------------
// Watch mode (task-5.3)
// ---------------------------------------------------------------------------

/**
 * Format current time as HH:MM:SS.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-023 plan=phase-5/task-5.3
 */
function formatTime(date: Date): string {
  const hh = String(date.getHours()).padStart(2, "0")
  const mm = String(date.getMinutes()).padStart(2, "0")
  const ss = String(date.getSeconds()).padStart(2, "0")
  return `${hh}:${mm}:${ss}`
}

/**
 * Clear the terminal screen.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-023 plan=phase-5/task-5.3
 */
function clearScreen(): void {
  process.stdout.write("\x1b[2J\x1b[H")
}

// ---------------------------------------------------------------------------
// Core runCli — exported for testability
// ---------------------------------------------------------------------------

/**
 * Run the CLI with the given args and return the output as a string.
 *
 * This function is exported for unit testing without spawning a subprocess.
 * When `opts.repoRoot` is provided, it overrides the `--repo` flag default.
 * When `opts.data` is provided, it is used directly instead of calling
 * collectData() — this allows main() to collect once and share the result
 * with both the renderer and the interactive selector (fixes NEW-006).
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-023 plan=step-final-002
 */
export async function runCli(
  args: string[],
  opts?: { repoRoot?: string; data?: DashboardData; _sqliteDbPath?: string }
): Promise<string> {
  const parsed = parseArgs(args, opts?.repoRoot)

  // --help: return help text
  if (parsed.help) {
    return HELP_TEXT
  }

  // Collect data — reuse pre-collected data if provided (avoids double I/O)
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-023 plan=step-final-002
  const data = opts?.data ?? (await collectData(parsed, opts?._sqliteDbPath))

  // --json: return JSON output
  if (parsed.json) {
    return renderDashboardJson(data)
  }

  // --plain: return tab-separated output (respects --max-rows)
  if (parsed.plain) {
    const sessions = data.sessions.slice(0, parsed.maxRows)
    return renderPlain(sessions)
  }

  // --group worktree: render sessions grouped by worktree with summary rows
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-110 plan=phase-10/task-10.6
  if (parsed.group === "worktree") {
    const { groupByWorktree } = await import("./htop-header")
    const groups = groupByWorktree(data.sessions)
    return renderWorktreeGroups(groups, { maxRows: parsed.maxRows })
  }

  // Default: render table
  if (parsed.extended) {
    // Extended mode: wider table with full token breakdown
    const result = renderDashboardExtended(data, {
      maxRows: parsed.maxRows,
      includeTeam: parsed.team,
    })
    return result.output
  }

  const width = process.stdout.columns ?? 88
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-004 plan=step-reverify-001
  const result = renderDashboard(data, {
    width,
    maxRows: parsed.maxRows,
    includeTeam: parsed.team,
    isInteractive: process.stdout.isTTY ?? false,
  })
  return result.output
}

// ---------------------------------------------------------------------------
// Watch mode runner
// ---------------------------------------------------------------------------

/**
 * Run the dashboard in watch mode (auto-refresh every 5 seconds).
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-023 plan=phase-5/task-5.3
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-004 plan=phase-10/task-10.8
 */
async function runWatch(parsed: CliArgs): Promise<void> {
  // Last rendered sessions — used by keyboard shortcut 1-9 to show session details.
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-004 plan=phase-10/task-10.8
  let lastRenderSessions: SessionRow[] = []

  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-073 plan=phase-10/task-10.2
  // Per-session history for computing current (delta-based) tok/s in watch mode.
  // First render: no history → lifetime average is used (existing behavior).
  // Subsequent renders: delta tokens / delta seconds gives the current rate.
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-120 plan=phase-10/task-10.5
  // `samples` stores the last 20 tok/s values for the multi-char sparkline history.
  const tpsHistory = new Map<string, { prevOutput: number; prevTime: number; samples: number[] }>()

  const render = async () => {
    // Collect data BEFORE clearing screen to avoid flicker (blank screen while loading)
    const data = await collectData(parsed)

    // Compute current tok/s from delta since last refresh (task-10.2)
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-073 plan=phase-10/task-10.2
    const now = Date.now()
    for (const session of data.sessions) {
      const prev = tpsHistory.get(session.sessionId)
      let currentTps: number | undefined
      if (prev && session.outputTokens !== undefined) {
        currentTps = computeCurrentTps(
          session.outputTokens ?? 0,
          prev.prevOutput,
          now - prev.prevTime
        )
        if (currentTps !== undefined) {
          session.tokensPerSecond = currentTps
        }
      }
      // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-120 plan=phase-10/task-10.5
      // Accumulate tok/s samples for multi-char sparkline history (last 20 values).
      const entry = tpsHistory.get(session.sessionId) ?? { prevOutput: 0, prevTime: now, samples: [] }
      const sampleValue = currentTps ?? session.tokensPerSecond ?? 0
      entry.samples.push(sampleValue)
      if (entry.samples.length > 20) entry.samples.shift()
      session.tpsSamples = entry.samples.slice()  // snapshot for renderer
      // Update history for next refresh
      tpsHistory.set(session.sessionId, {
        ...entry,
        prevOutput: session.outputTokens ?? 0,
        prevTime: now,
      })
    }

    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-052 plan=phase-10/step-p10-H002
    // Evict stale sessions from tpsHistory (sessions no longer in data.sessions)
    // Bounds the Map to at most the number of currently visible sessions (max 50 by default),
    // preventing unbounded memory growth during long-running watch sessions with high churn.
    const currentSessionIds = new Set(data.sessions.map(s => s.sessionId))
    for (const id of tpsHistory.keys()) {
      if (!currentSessionIds.has(id)) {
        tpsHistory.delete(id)
      }
    }

    let output: string
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-160 plan=phase-10/step-p10-H001
    if (parsed.group === "worktree") {
      const { groupByWorktree } = await import("./htop-header")
      const { renderWorktreeGroups } = await import("./renderer")
      const groups = groupByWorktree(data.sessions)
      output = renderWorktreeGroups(groups, { maxRows: parsed.maxRows })
    } else if (parsed.extended) {
      const result = renderDashboardExtended(data, {
        maxRows: parsed.maxRows,
        includeTeam: parsed.team,
      })
      output = result.output
    } else {
      const width = process.stdout.columns ?? 88
      const result = renderDashboard(data, {
        width,
        maxRows: parsed.maxRows,
        includeTeam: parsed.team,
        isInteractive: true,
      })
      output = result.output
    }

    // Only clear and redraw once the new frame is ready
    clearScreen()
    process.stdout.write(output + "\n")
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-004 plan=phase-10/task-10.8
    process.stdout.write(`Last updated: ${formatTime(new Date())}  │  Refreshing every ${parsed.interval}s  │  q=quit  r=refresh  s=sort  x=extended  t=team  1-9=jump\n`)
    // Store sessions for keyboard shortcut 1-9 (task-10.8)
    lastRenderSessions = data.sessions
  }

  // Cleanup: stop interval and restore stdin
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-004 plan=phase-10/task-10.8
  // `interval` is assigned below; cleanup captures it via closure (let, not const).
  let interval: ReturnType<typeof setInterval>
  const cleanup = () => {
    clearInterval(interval)
    if (process.stdin.isTTY) {
      process.stdin.setRawMode(false)
      process.stdin.pause()
    }
  }

  // Handle SIGINT (Ctrl+C) — stop cleanly
  process.on("SIGINT", () => {
    cleanup()
    process.stdout.write("\n")
    process.exit(0)
  })

  // Initial render
  await render()

  // Refresh at configurable interval (default 5s)
  const intervalMs = parsed.interval * 1000
  interval = setInterval(async () => {
    await render()
  }, intervalMs)

  // Set up raw-mode keyboard input (htop conventions) — only when stdin is a TTY.
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-004 plan=phase-10/task-10.8
  if (process.stdin.isTTY) {
    process.stdin.setRawMode(true)
    process.stdin.resume()
    process.stdin.setEncoding("utf8")

    process.stdin.on("data", (key: string) => {
      // Ctrl+C
      if (key === "\u0003") {
        cleanup()
        process.exit(0)
      }
      // q: quit
      if (key === "q" || key === "Q") {
        cleanup()
        process.exit(0)
      }
      // r: force refresh
      if (key === "r" || key === "R") {
        render().catch(() => {})
      }
      // s: cycle sort (updated → cost → tokens → duration → name → status → updated)
      if (key === "s" || key === "S") {
        const sorts: CliArgs["sort"][] = ["updated", "cost", "tokens", "duration", "name", "status"]
        const currentIdx = sorts.indexOf(parsed.sort)
        parsed.sort = sorts[(currentIdx + 1) % sorts.length]!
        render().catch(() => {})
      }
      // x: toggle extended mode
      if (key === "x" || key === "X") {
        parsed.extended = !parsed.extended
        render().catch(() => {})
      }
      // t: toggle team view
      if (key === "t" || key === "T") {
        parsed.team = !parsed.team
        render().catch(() => {})
      }
      // 1-9: jump to session — show attach command for session N
      const num = parseInt(key, 10)
      if (!isNaN(num) && num >= 1 && num <= 9) {
        const session = lastRenderSessions[num - 1]
        if (session) {
          process.stdout.write("\n")
          process.stdout.write(`Session ${num}: ${session.sessionId}\n`)
          process.stdout.write(`Title: ${session.title ?? "(untitled)"}\n`)
          process.stdout.write(`\nopencode attach http://localhost:4096 --session ${session.sessionId}\n\n`)
        }
      }
    })
  }

  // Keep the process alive
  // The interval itself keeps the event loop running; SIGINT/q will exit.
  // Suppress the "interval" unused warning — it IS used to keep the loop alive.
  void (interval)
}

// ---------------------------------------------------------------------------
// Main entry point (only runs when executed directly, not when imported)
// ---------------------------------------------------------------------------

/**
 * Main entry point for the CLI binary.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-023 plan=phase-5/task-5.1
 */
async function main(): Promise<void> {
  const argv = process.argv.slice(2)
  const parsed = parseArgs(argv)

  // --help
  if (parsed.help) {
    process.stdout.write(HELP_TEXT + "\n")
    process.exit(0)
  }

  // --watch mode
  if (parsed.watch) {
    await runWatch(parsed)
    return
  }

  // One-shot output — collect data once, share with interactive selector
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-023 plan=step-final-002
  const data = await collectData(parsed)
  const output = await runCli(argv, { data })
  process.stdout.write(output + "\n")

  // Interactive selector: only when stdout is a TTY and not --json/--plain
  // Reuses the already-collected data — no second collectData() call (fixes NEW-006)
  if (process.stdout.isTTY && !parsed.json && !parsed.plain) {
    await runInteractiveSelector(data.sessions)
  }
}

// Run main only when this file is the entry point (not when imported as a module)
// Bun sets import.meta.main = true when the file is run directly.
if (import.meta.main) {
  main().catch((err) => {
    process.stderr.write(`Error: ${err instanceof Error ? err.message : String(err)}\n`)
    process.exit(1)
  })
}
