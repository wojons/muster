/**
 * NetworkSource — reads team session data from the L4 pulse API.
 *
 * REQ-TUI-040: When L4 network awareness is enabled, the dashboard MUST show
 *   team members' sessions as additional rows with visual distinction.
 * REQ-TUI-041: Team rows MUST display name, work item, model, duration, status.
 *   Cost and token data MUST NOT be shown for team sessions.
 * REQ-TUI-042: Team visibility MUST respect the `network.visibility` setting:
 *   users set to `hidden` do not appear in the dashboard's team view.
 * REQ-TUI-043: Hidden team members removed gracefully on next pulse refresh.
 * REQ-TUI-044: Team data refresh MUST use GET <backend_url>/api/v1/pulse and
 *   MUST handle API failures gracefully — show local sessions only with a
 *   subtle "Team data unavailable" indicator.
 *
 * Spec: specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-040
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-040 plan=phase-4/task-4.1
 */

import type { SessionDataSource, SessionRow } from "../types"
import { resolveModelDisplayName } from "../model-aliases"

// ---------------------------------------------------------------------------
// TeamMember — pulse API response shape
// ---------------------------------------------------------------------------

/**
 * A single team member entry from the pulse API response.
 *
 * GET <backendUrl>/api/v1/pulse returns TeamMember[].
 *
 * REQ-TUI-042: members with visibility="hidden" MUST be filtered out.
 */
export interface TeamMember {
  user_id: string
  work_item?: string
  model?: string
  status: "active" | "idle" | "offline"
  last_seen: string // ISO 8601
  visibility: "public" | "hidden"
}

// ---------------------------------------------------------------------------
// NetworkSourceOptions
// ---------------------------------------------------------------------------

/**
 * Options for constructing a NetworkSource.
 *
 * `_fetch` is an injection point for testing — when provided, no real HTTP
 * requests are made. The injected function receives the full URL and token
 * and returns a TeamMember array directly.
 */
export interface NetworkSourceOptions {
  /** Base URL of the Axiom backend (e.g., "https://backend.example.com"). */
  backendUrl: string
  /** API token for Authorization: Bearer header. */
  apiToken: string
  /**
   * For testing: inject a custom fetch function.
   * Signature: (url: string, token: string) => Promise<TeamMember[]>
   */
  _fetch?: (url: string, token: string) => Promise<TeamMember[]>
}

// ---------------------------------------------------------------------------
// Model display aliases (re-exported from shared module for backward compat)
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-012 plan=step-verify-H004
// ---------------------------------------------------------------------------

/** @deprecated Import from `../model-aliases` directly. Re-exported for backward compatibility. */
export { MODEL_DISPLAY_ALIASES as MODEL_ALIASES } from "../model-aliases"

// ---------------------------------------------------------------------------
// Status mapping
// ---------------------------------------------------------------------------

/**
 * Map TeamMember.status to SessionRow.status (SessionState).
 *
 * "active"  → "running"
 * "idle"    → "idle"
 * "offline" → "stopped"
 */
function mapStatus(memberStatus: TeamMember["status"]): "running" | "idle" | "stopped" {
  switch (memberStatus) {
    case "active":
      return "running"
    case "idle":
      return "idle"
    case "offline":
      return "stopped"
    default:
      return "stopped"
  }
}

// ---------------------------------------------------------------------------
// NetworkSource
// ---------------------------------------------------------------------------

/**
 * NetworkSource — lowest-priority SessionDataSource (priority=1).
 *
 * Fetches team member sessions from the L4 pulse API endpoint:
 *   GET <backendUrl>/api/v1/pulse
 *
 * - Filters out members with visibility="hidden" (REQ-TUI-042).
 * - Sets costUsd=0 and totalTokens=0 for all team sessions (REQ-TUI-041).
 * - Handles network errors gracefully: returns [] and stores error message.
 * - Exposes getLastError() for the renderer to show "Team data unavailable".
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-040 plan=phase-4/task-4.1
 */
export class NetworkSource implements SessionDataSource {
  readonly id = "network" as const
  readonly priority = 1

  private readonly backendUrl: string
  private readonly apiToken: string
  private readonly fetchFn: (url: string, token: string) => Promise<TeamMember[]>

  private lastUpdate: Date | null = null
  private lastError: string | null = null

  constructor(options: NetworkSourceOptions) {
    this.backendUrl = options.backendUrl
    this.apiToken = options.apiToken

    // Use injected fetch or real HTTP fetch
    // axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-055 plan=step-verify-C001
    this.fetchFn =
      options._fetch ??
      (async (url: string, token: string): Promise<TeamMember[]> => {
        const response = await fetch(url, {
          headers: { Authorization: `Bearer ${token}` },
          signal: AbortSignal.timeout(500), // REQ-TUI-055: <500ms SLO
        })
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}: ${response.statusText}`)
        }
        return (await response.json()) as TeamMember[]
      })
  }

  /**
   * Returns true when backendUrl is configured (non-empty).
   *
   * REQ-TUI-044: source is only active when backend URL is set.
   */
  isAvailable(): boolean {
    return this.backendUrl.trim().length > 0
  }

  /**
   * Returns the timestamp of the most recent successful fetch, or null.
   */
  getLastUpdate(): Date | null {
    return this.lastUpdate
  }

  /**
   * Returns the last error message from a failed fetch, or null.
   *
   * Used by the renderer to show "Team data unavailable: <error>".
   * REQ-TUI-044: failures MUST be surfaced gracefully.
   */
  getLastError(): string | null {
    return this.lastError
  }

  /**
   * Fetch team sessions from the pulse API.
   *
   * - Returns [] when not available (backendUrl empty).
   * - Filters out hidden members (REQ-TUI-042).
   * - Sets costUsd=0 and totalTokens=0 (REQ-TUI-041).
   * - On network error: stores error, returns [] (REQ-TUI-044).
   *
   * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-040 plan=phase-4/task-4.1
   */
  async getSessions(): Promise<SessionRow[]> {
    if (!this.isAvailable()) {
      return []
    }

    const url = `${this.backendUrl}/api/v1/pulse`

    let members: TeamMember[]
    try {
      members = await this.fetchFn(url, this.apiToken)
      // Clear any previous error on success
      this.lastError = null
      this.lastUpdate = new Date()
    } catch (err: unknown) {
      // REQ-TUI-044: handle failures gracefully — store error, return []
      const message = err instanceof Error ? err.message : String(err)
      this.lastError = message
      return []
    }

    const now = new Date()
    const rows: SessionRow[] = []

    for (const member of members) {
      // REQ-TUI-042: filter out hidden members
      if (member.visibility === "hidden") {
        continue
      }

      // Compute duration from last_seen to now
      let durationSeconds = 0
      try {
        const lastSeenMs = new Date(member.last_seen).getTime()
        if (!isNaN(lastSeenMs)) {
          durationSeconds = Math.max(0, Math.floor((now.getTime() - lastSeenMs) / 1000))
        }
      } catch {
        durationSeconds = 0
      }

      // Parse updatedAt from last_seen
      let updatedAt: Date
      try {
        const parsed = new Date(member.last_seen)
        updatedAt = isNaN(parsed.getTime()) ? now : parsed
      } catch {
        updatedAt = now
      }

      const rawModel = member.model ?? ""

      const row: SessionRow = {
        sessionId: `team-${member.user_id}`,
        workItemId: member.work_item,
        // REQ-TUI-012: resolve display alias for model; undefined model → "Unknown"
        model: member.model ? resolveModelDisplayName(member.model) : "Unknown",
        modelRaw: rawModel,
        // REQ-TUI-041: cost and token data MUST NOT be shown for team sessions
        costUsd: 0,
        totalTokens: 0,
        durationSeconds,
        status: mapStatus(member.status),
        source: "network",
        updatedAt,
        isTeam: true,
        teamMemberId: member.user_id,
      }

      rows.push(row)
    }

    return rows
  }
}

// ---------------------------------------------------------------------------
// Factory helper
// ---------------------------------------------------------------------------

/**
 * Convenience factory for constructing a NetworkSource.
 *
 * @example
 * ```typescript
 * const source = createNetworkSource({
 *   backendUrl: config.network.backendUrl,
 *   apiToken: config.network.apiToken,
 * })
 * ```
 */
export function createNetworkSource(options: NetworkSourceOptions): NetworkSource {
  return new NetworkSource(options)
}
