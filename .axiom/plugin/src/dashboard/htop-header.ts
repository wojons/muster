/**
 * htop-style header meters for the TUI Dashboard.
 *
 * Renders ANSI-colored progress bars showing per-model token usage,
 * provider breakdown, and session activity indicators.
 *
 * Color scheme (matches htop CPU bar convention):
 *   Green  = output tokens (work produced)
 *   Cyan   = cache read tokens (reused context)
 *   Blue   = input tokens (new context sent)
 *   Yellow = reasoning tokens (chain-of-thought)
 *   Red    = cache write tokens (context stored)
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-071
 */

import type { ModelUsage, ProviderUsage, SessionRow, DashboardAggregates } from "./types"

// ---------------------------------------------------------------------------
// WorktreeGroup — aggregated session group for --group worktree mode
// ---------------------------------------------------------------------------

/**
 * Aggregated group of sessions sharing the same worktree.
 * Used by renderWorktreeGroups() in renderer.ts.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-110 plan=phase-10/task-10.6
 */
export interface WorktreeGroup {
  worktree: string
  sessions: SessionRow[]
  totalCostUsd: number
  totalOutputTokens: number
  totalToolCalls: number
  sessionCount: number
}

/**
 * Group sessions by worktree name, sorted by total cost descending.
 * Sessions without a directory are grouped under "—".
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-110 plan=phase-10/task-10.6
 */
export function groupByWorktree(sessions: SessionRow[]): WorktreeGroup[] {
  const map = new Map<string, WorktreeGroup>()
  for (const s of sessions) {
    const wt = extractWorktree(s.directory ?? "")
    const existing = map.get(wt)
    if (existing) {
      existing.sessions.push(s)
      existing.totalCostUsd += s.costUsd
      existing.totalOutputTokens += s.outputTokens ?? 0
      existing.totalToolCalls += s.toolCalls ?? 0
      existing.sessionCount++
    } else {
      map.set(wt, {
        worktree: wt,
        sessions: [s],
        totalCostUsd: s.costUsd,
        totalOutputTokens: s.outputTokens ?? 0,
        totalToolCalls: s.toolCalls ?? 0,
        sessionCount: 1,
      })
    }
  }
  return Array.from(map.values()).sort((a, b) => b.totalCostUsd - a.totalCostUsd)
}

// ---------------------------------------------------------------------------
// ANSI color codes
// ---------------------------------------------------------------------------

const C = {
  reset: "\x1b[0m",
  bold: "\x1b[1m",
  dim: "\x1b[2m",
  // Foreground
  green: "\x1b[32m",
  cyan: "\x1b[36m",
  blue: "\x1b[34m",
  yellow: "\x1b[33m",
  red: "\x1b[31m",
  white: "\x1b[37m",
  gray: "\x1b[90m",
  magenta: "\x1b[35m",
  // Background (for filled bar segments)
  bgGreen: "\x1b[42m",
  bgCyan: "\x1b[46m",
  bgBlue: "\x1b[44m",
  bgYellow: "\x1b[43m",
  bgRed: "\x1b[41m",
  bgGray: "\x1b[100m",
} as const

// ---------------------------------------------------------------------------
// Progress bar rendering
// ---------------------------------------------------------------------------

/**
 * Render a stacked progress bar with colored segments.
 * Each segment is a { fraction: 0-1, color: ANSI bg color } pair.
 * The bar is `width` characters wide.
 */
function renderStackedBar(
  segments: Array<{ fraction: number; color: string }>,
  width: number
): string {
  const chars: string[] = []
  let used = 0

  for (const seg of segments) {
    const segWidth = Math.round(seg.fraction * width)
    if (segWidth > 0 && used + segWidth <= width) {
      chars.push(seg.color + " ".repeat(segWidth) + C.reset)
      used += segWidth
    }
  }

  // Fill remaining with dim background
  if (used < width) {
    chars.push(C.bgGray + " ".repeat(width - used) + C.reset)
  }

  return chars.join("")
}

/**
 * Format a number with K/M suffix for compact display.
 */
function fmtNum(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${Math.round(n / 1_000)}K`
  return String(n)
}

// ---------------------------------------------------------------------------
// Sparkline for tok/s
// ---------------------------------------------------------------------------

const SPARK_CHARS = ["▏", "▎", "▍", "▌", "▋", "▊", "▉", "█"]

/**
 * Render a single-character sparkline bar for a value relative to a max.
 * Returns a colored Unicode block character.
 */
export function renderSparkline(value: number, maxValue: number): string {
  if (maxValue <= 0 || value <= 0) return C.dim + "▏" + C.reset
  const ratio = Math.min(value / maxValue, 1)
  const idx = Math.min(Math.floor(ratio * SPARK_CHARS.length), SPARK_CHARS.length - 1)
  const char = SPARK_CHARS[idx]!
  // Color: green for high throughput, yellow for medium, red for low
  let color: string = C.green
  if (ratio < 0.3) color = C.red
  else if (ratio < 0.6) color = C.yellow
  return color + char + C.reset
}

/**
 * Render a multi-character sparkline history from an array of samples.
 * Uses 8-level Unicode blocks, color-coded by each value's level relative to maxValue.
 * The last sample is bolded to highlight the current value.
 * Width = samples.length (up to 20 chars).
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-120 plan=phase-10/task-10.5
 */
export function renderSparklineHistory(samples: number[], maxValue: number): string {
  if (samples.length === 0) return ""
  const HISTORY_SPARK_CHARS = ["▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"]
  const effectiveMax = Math.max(maxValue, 1)

  return samples.map((v, i) => {
    const ratio = Math.min(v / effectiveMax, 1)
    const idx = Math.min(Math.floor(ratio * HISTORY_SPARK_CHARS.length), HISTORY_SPARK_CHARS.length - 1)
    const char = HISTORY_SPARK_CHARS[idx]!
    // Color based on value level
    const color = ratio < 0.3 ? C.red : ratio < 0.6 ? C.yellow : C.green
    // Last sample gets bold to highlight current value
    const bold = i === samples.length - 1 ? C.bold : ""
    return `${bold}${color}${char}${C.reset}`
  }).join("")
}

// ---------------------------------------------------------------------------
// Worktree extraction
// ---------------------------------------------------------------------------

/**
 * Extract worktree name from a session directory path.
 * e.g., "/Users/foo/code/Axiom/.worktrees/tui-dashboard" → "tui-dashboard"
 *       "/Users/foo/code/Axiom" → "main"
 *       "/Users/foo/code/recall" → "recall"
 */
export function extractWorktree(directory: string): string {
  if (!directory) return "—"
  const wtMatch = directory.match(/\.worktrees\/([^/]+)/)
  if (wtMatch) return wtMatch[1]!
  // Not a worktree — use the last directory segment as the project name
  const parts = directory.split("/").filter(Boolean)
  return parts[parts.length - 1] ?? "—"
}

// ---------------------------------------------------------------------------
// Provider display names
// ---------------------------------------------------------------------------

/**
 * Short display names for known provider IDs.
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-100 plan=phase-10/task-10.3
 */
const PROVIDER_DISPLAY_NAMES: Record<string, string> = {
  "amazon-bedrock": "Bedrock",
  "openai": "OpenAI",
  "anthropic": "Anthropic",
  "google": "Google",
  "mistral": "Mistral",
}

/**
 * Aggregate ModelUsage entries by provider.
 * Sessions are max-aggregated (a session belongs to one provider); all other
 * numeric fields are summed. Results are sorted by costUsd descending.
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-100 plan=phase-10/task-10.3
 */
export function aggregateByProvider(modelUsage: ModelUsage[]): ProviderUsage[] {
  const map = new Map<string, ProviderUsage>()
  for (const m of modelUsage) {
    const key = m.provider || "unknown"
    const existing = map.get(key)
    if (existing) {
      existing.sessions = Math.max(existing.sessions, m.sessions)
      existing.calls += m.calls
      existing.costUsd += m.costUsd
      existing.outputTokens += m.outputTokens
      existing.inputTokens += m.inputTokens
      existing.cacheReadTokens += m.cacheReadTokens
      existing.reasoningTokens += m.reasoningTokens
    } else {
      map.set(key, { ...m, provider: key })
    }
  }
  return Array.from(map.values()).sort((a, b) => b.costUsd - a.costUsd)
}

// ---------------------------------------------------------------------------
// htop-style header
// ---------------------------------------------------------------------------

/**
 * Render the htop-style header with per-model colored progress bars.
 *
 * Layout:
 *   ┌─ Models ──────────────────────────────────────────────────────────────┐
 *   │ Claude Opus 4.6   [████████████░░░░░░░░░░░░░░░░] 2813 calls  $673.97 │
 *   │ Claude Sonnet 4.6 [██████████████████░░░░░░░░░░░] 3981 calls  $878.22 │
 *   │ GPT-5.4           [██░░░░░░░░░░░░░░░░░░░░░░░░░░]   30 calls    $0.00 │
 *   └──────────────────────────────────────────────────────────────────────┘
 *
 * Bar colors: green=output, cyan=cache_read, blue=input, yellow=reasoning
 */
export function renderHtopHeader(
  modelUsage: ModelUsage[],
  aggregates: DashboardAggregates,
  sessions: SessionRow[],
): string {
  if (!modelUsage || modelUsage.length === 0) return ""

  const barWidth = 30
  const lines: string[] = []

  // Find the max total tokens across all models for proportional bars
  const maxTokens = Math.max(...modelUsage.map((m) => m.outputTokens + m.inputTokens + m.cacheReadTokens + (m.reasoningTokens ?? 0)))

  // Running/stopped counts
  const running = sessions.filter((s) => s.status === "running").length
  const stopped = sessions.filter((s) => s.status === "stopped").length
  const idle = sessions.filter((s) => s.status === "idle").length

  // Header line
  lines.push("")
  lines.push(
    `  ${C.bold}Axiom Dashboard${C.reset}  ` +
    `${C.green}● ${running} running${C.reset}  ` +
    (idle > 0 ? `${C.yellow}○ ${idle} idle${C.reset}  ` : "") +
    `${C.gray}■ ${stopped} stopped${C.reset}  ` +
    `│  ${C.bold}$${aggregates.totalCostUsd.toFixed(2)}${C.reset} total  ` +
    `${fmtNum(aggregates.totalTokens)} tokens  ` +
    `${aggregates.avgSpeedTokensPerSec.toFixed(1)} tok/s avg`
  )

  // Age alert: warn if any session has been running > 24h (atop-style saturation indicator)
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-072 plan=phase-10/task-10.7
  const oldSessions = sessions.filter(s => s.durationSeconds > 24 * 3600).length
  if (oldSessions > 0) {
    lines.push(`  ${C.red}⚠ ${oldSessions} session${oldSessions > 1 ? 's' : ''} running > 24h${C.reset}`)
  }

  lines.push("")

  // Per-model bars
  const maxModelLen = Math.max(...modelUsage.map((m) => m.model.length), 10)

  for (const m of modelUsage) {
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-100 plan=F-003
    const totalTok = m.outputTokens + m.inputTokens + m.cacheReadTokens + (m.reasoningTokens ?? 0)
    if (totalTok === 0) continue

    // Compute segment fractions relative to this model's total
    const segments = [
      { fraction: m.outputTokens / totalTok, color: C.bgGreen },
      { fraction: m.cacheReadTokens / totalTok, color: C.bgCyan },
      { fraction: m.inputTokens / totalTok, color: C.bgBlue },
      { fraction: (m.reasoningTokens ?? 0) / totalTok, color: C.bgYellow },
    ]

    // Scale the bar width proportional to max tokens across all models
    const scaledWidth = Math.max(1, Math.round((totalTok / maxTokens) * barWidth))

    const bar = renderStackedBar(segments, scaledWidth)
    // Pad remaining bar space with dots
    const padBar = scaledWidth < barWidth ? C.dim + "·".repeat(barWidth - scaledWidth) + C.reset : ""

    const label = m.model.padEnd(maxModelLen)
    const calls = String(m.calls).padStart(5)
    const cost = `$${m.costUsd.toFixed(2)}`.padStart(9)
    const out = fmtNum(m.outputTokens).padStart(6) + " out"

    lines.push(`  ${label}  ${bar}${padBar}  ${calls} calls ${cost}  ${out}`)
  }

  // Provider breakdown bars (iftop-style, task-10.3)
  // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-100 plan=phase-10/task-10.3
  const providers = aggregateByProvider(modelUsage)
  if (providers.length > 0) {
    const maxProviderTokens = Math.max(
      ...providers.map((p) => p.outputTokens + p.inputTokens + p.cacheReadTokens + (p.reasoningTokens ?? 0))
    )
    const maxProviderNameLen = Math.max(
      ...providers.map((p) => (PROVIDER_DISPLAY_NAMES[p.provider] ?? p.provider.slice(0, 12)).length),
      7
    )

    lines.push(`  ${C.dim}─ Providers ${"─".repeat(Math.max(0, barWidth + maxProviderNameLen + 20))}${C.reset}`)

    for (const p of providers) {
      const totalTok = p.outputTokens + p.inputTokens + p.cacheReadTokens + (p.reasoningTokens ?? 0)
      if (totalTok === 0) continue

      const segments = [
        { fraction: p.outputTokens / totalTok, color: C.bgGreen },
        { fraction: p.cacheReadTokens / totalTok, color: C.bgCyan },
        { fraction: p.inputTokens / totalTok, color: C.bgBlue },
        { fraction: (p.reasoningTokens ?? 0) / totalTok, color: C.bgYellow },
      ]

      const scaledWidth = Math.max(1, Math.round((totalTok / maxProviderTokens) * barWidth))
      const bar = renderStackedBar(segments, scaledWidth)
      const padBar = scaledWidth < barWidth ? C.dim + "·".repeat(barWidth - scaledWidth) + C.reset : ""

      const displayName = PROVIDER_DISPLAY_NAMES[p.provider] ?? p.provider.slice(0, 12)
      const label = displayName.padEnd(maxProviderNameLen)
      const sessions = String(p.sessions).padStart(4) + " sessions"
      const cost = `$${p.costUsd.toFixed(2)}`.padStart(9)

      lines.push(`  ${label}  ${bar}${padBar}  ${sessions} ${cost}`)
    }

    lines.push("")
  }

  // Legend
  lines.push("")
  lines.push(
    `  ${C.dim}Legend: ${C.reset}` +
    `${C.bgGreen} ${C.reset} output  ` +
    `${C.bgCyan} ${C.reset} cache  ` +
    `${C.bgBlue} ${C.reset} input  ` +
    `${C.bgYellow} ${C.reset} reasoning`
  )

  return lines.join("\n")
}
