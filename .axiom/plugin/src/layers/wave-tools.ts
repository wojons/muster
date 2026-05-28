/**
 * Wave plugin tools — codeops_wave_read, codeops_wave_write, codeops_wave_propose_action,
 * codeops_wave_automation, codeops_wave_comments, codeops_wave_execute_action.
 *
 * These tools allow the agent to interact with Wave Spaces programmatically.
 *
 * REQ-WEAVE-112: codeops_wave_write — write rich content to a Wave Space.
 * REQ-WEAVE-113: codeops_wave_read — read Wave Space state.
 * REQ-WEAVE-114: codeops_wave_propose_action — propose an action item.
 * REQ-WEAVE-064: codeops_wave_execute_action — execute an approved Wave action.
 *
 * Spec: specs/70-OpenCode-Plugin.md#wave-integration Section 20.2
 * axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md#wave-integration plan=phase-1.3/task-1.3.4/step-1.3.4.1
 */

/**
 * All registered Wave tools. All 6 tools are now real implementations.
 * - codeops_wave_read: Read Space state, comments, action list
 * - codeops_wave_write: Write content to a Space panel
 * - codeops_wave_propose_action: Propose an action item for human approval
 * - codeops_wave_automation: Manage automations
 * - codeops_wave_comments: Read comments on session items
 * - codeops_wave_execute_action: Execute an approved action via OpenCode session (Phase 4.7)
 *
 * axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md plan=phase-4.7/task-4.7.execution/step-4.7.3
 */
export const WAVE_TOOL_NAMES = [
  "codeops_wave_read",
  "codeops_wave_write",
  "codeops_wave_propose_action",
  "codeops_wave_automation",
  "codeops_wave_comments",
  "codeops_wave_execute_action",  // Phase 4.7: now calls real OpenCode API
] as const

export interface WaveToolConfig {
  backendUrl: string
  authToken?: string
  /** OpenCode HTTP API base URL. Defaults to AXIOM_OPENCODE_BASE_URL env var or http://127.0.0.1:4096 */
  openCodeUrl?: string
  _mockFetch?: (url: string, opts?: RequestInit) => Promise<Response>
}

// ── codeops_wave_read ────────────────────────────────────────────────────

export interface WaveReadParams {
  space_id: string
  panel_id?: string
  include_comments?: boolean   // default true
  include_action_list?: boolean // default true
  include_chat?: boolean        // default false — when true, fetches recent_chat from /chat endpoint
}

export interface WaveReadResult {
  space_id: string
  panels: object[]
  comments: object[]
  actions: object[]
  panel_items?: object[]
  recent_chat?: object[]  // populated when include_chat=true (REQ-WEAVE-065)
}

// axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md plan=phase-3.5/task-3.5.mentions/step-3.5.4

export async function waveRead(
  params: WaveReadParams,
  config: WaveToolConfig,
): Promise<WaveReadResult> {
  const fetch = config._mockFetch ?? globalThis.fetch
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(config.authToken ? { Authorization: `Bearer ${config.authToken}` } : {}),
  }

  // Fetch panels
  const panelsResp = await fetch(
    `${config.backendUrl}/api/v1/spaces/${params.space_id}/panels`,
    { headers },
  )
  if (!panelsResp.ok) throw new Error(`waveRead: panels fetch failed ${panelsResp.status}`)
  const panelsData = await panelsResp.json() as { panels?: object[] } | object[]
  const panels = Array.isArray(panelsData) ? panelsData : (panelsData as { panels?: object[] }).panels ?? []

  // Fetch comments (if requested)
  let comments: object[] = []
  if (params.include_comments !== false) {
    const commentsResp = await fetch(
      `${config.backendUrl}/api/v1/spaces/${params.space_id}/comments`,
      { headers },
    )
    if (commentsResp.ok) {
      const cd = await commentsResp.json() as { comments?: object[] } | object[]
      comments = Array.isArray(cd) ? cd : (cd as { comments?: object[] }).comments ?? []
    }
  }

  // Fetch action items (if requested)
  let actions: object[] = []
  if (params.include_action_list !== false) {
    const actionsResp = await fetch(
      `${config.backendUrl}/api/v1/spaces/${params.space_id}/actions`,
      { headers },
    )
    if (actionsResp.ok) {
      const ad = await actionsResp.json() as { actions?: object[] } | object[]
      actions = Array.isArray(ad) ? ad : (ad as { actions?: object[] }).actions ?? []
    }
  }

  const result: WaveReadResult = { space_id: params.space_id, panels, comments, actions }

  // Fetch recent chat messages (if requested)
  // axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md plan=phase-3.5/task-3.5.mentions/step-3.5.4
  if (params.include_chat) {
    const chatResp = await fetch(
      `${config.backendUrl}/api/v1/spaces/${params.space_id}/chat?limit=20`,
      { headers },
    )
    if (chatResp.ok) {
      const chatData = await chatResp.json() as { items?: object[] } | object[]
      result.recent_chat = Array.isArray(chatData)
        ? chatData
        : (chatData as { items?: object[] }).items ?? []
    }
  }

  return result
}

// ── codeops_wave_propose_action ──────────────────────────────────────────

export interface WaveProposeActionParams {
  space_id: string
  title: string
  description: string
  priority?: "high" | "medium" | "low"
  source_item_id?: string
}

export interface WaveProposeActionResult {
  action_id: string
  status: string
  title: string
}

export async function waveProposeAction(
  params: WaveProposeActionParams,
  config: WaveToolConfig,
): Promise<WaveProposeActionResult> {
  const fetch = config._mockFetch ?? globalThis.fetch
  const resp = await fetch(
    `${config.backendUrl}/api/v1/spaces/${params.space_id}/actions`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(config.authToken ? { Authorization: `Bearer ${config.authToken}` } : {}),
      },
      body: JSON.stringify({
        title: params.title,
        description: params.description,
        priority: params.priority ?? "medium",
        source: params.source_item_id ? { type: "item", item_id: params.source_item_id } : { type: "agent" },
      }),
    },
  )
  if (!resp.ok) throw new Error(`waveProposeAction failed: ${resp.status}`)
  return resp.json() as Promise<WaveProposeActionResult>
}

// ── codeops_wave_write ────────────────────────────────────────────────────

export interface WaveWriteParams {
  space_id: string
  panel_id?: string
  content_type: "markdown" | "html" | "json" | "template"
  content: string
  template?: string
  data?: Record<string, unknown>
}

export interface WaveWriteResult {
  item_id: string
  panel_id: string
  space_id: string
}

export async function waveWrite(
  params: WaveWriteParams,
  config: WaveToolConfig,
): Promise<WaveWriteResult> {
  const fetch = config._mockFetch ?? globalThis.fetch
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(config.authToken ? { Authorization: `Bearer ${config.authToken}` } : {}),
  }

  let panelId = params.panel_id

  // If no panel_id, create a new panel of type "rendered_output"
  if (!panelId) {
    const createResp = await fetch(
      `${config.backendUrl}/api/v1/spaces/${params.space_id}/panels`,
      {
        method: "POST",
        headers,
        body: JSON.stringify({ type: "rendered_output" }),
      },
    )
    if (!createResp.ok) throw new Error(`waveWrite: panel creation failed ${createResp.status}`)
    const createData = await createResp.json() as { panel_id: string }
    panelId = createData.panel_id
  }

  // Post the item to the panel
  const itemResp = await fetch(
    `${config.backendUrl}/api/v1/spaces/${params.space_id}/panels/${panelId}/items`,
    {
      method: "POST",
      headers,
      body: JSON.stringify({
        type: "rich_render",
        author: "agent",
        content: {
          content_type: params.content_type,
          content: params.content,
          template: params.template,
          data: params.data,
        },
      }),
    },
  )
  if (!itemResp.ok) throw new Error(`waveWrite: item creation failed ${itemResp.status}`)
  const itemData = await itemResp.json() as { item_id: string }

  return {
    item_id: itemData.item_id,
    panel_id: panelId,
    space_id: params.space_id,
  }
}

// axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md#wave-integration plan=phase-1.3/task-1.3.7/step-1.3.7.1

// ── codeops_wave_automation ─────────────────────────────────────────────────

export interface WaveAutomationParams {
  space_id: string
  action: "create" | "update" | "delete" | "enable" | "disable" | "list"
  automation_id?: string
  automation?: Record<string, unknown>
}

export interface WaveAutomationResult {
  automation_id?: string
  automations?: object[]
  status: string
}

export async function waveAutomation(
  params: WaveAutomationParams,
  config: WaveToolConfig,
): Promise<WaveAutomationResult> {
  const fetch = config._mockFetch ?? globalThis.fetch
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(config.authToken ? { Authorization: `Bearer ${config.authToken}` } : {}),
  }

  const baseUrl = `${config.backendUrl}/api/v1/spaces/${params.space_id}/automations`

  switch (params.action) {
    case "list": {
      const resp = await fetch(baseUrl, { headers })
      if (!resp.ok) throw new Error(`waveAutomation list failed: ${resp.status}`)
      const data = await resp.json() as { automations?: object[] } | object[]
      const automations = Array.isArray(data) ? data : (data as { automations?: object[] }).automations ?? []
      return { automations, status: "ok" }
    }
    case "create": {
      const resp = await fetch(baseUrl, {
        method: "POST",
        headers,
        body: JSON.stringify(params.automation ?? {}),
      })
      if (!resp.ok) throw new Error(`waveAutomation create failed: ${resp.status}`)
      const data = await resp.json() as { automation_id: string }
      return { automation_id: data.automation_id, status: "ok" }
    }
    case "update":
    case "enable":
    case "disable": {
      if (!params.automation_id) throw new Error("automation_id required for update/enable/disable")
      const resp = await fetch(`${baseUrl}/${params.automation_id}`, {
        method: "PATCH",
        headers,
        body: JSON.stringify(params.automation ?? {}),
      })
      if (!resp.ok) throw new Error(`waveAutomation ${params.action} failed: ${resp.status}`)
      return { status: "ok" }
    }
    case "delete": {
      if (!params.automation_id) throw new Error("automation_id required for delete")
      const resp = await fetch(`${baseUrl}/${params.automation_id}`, {
        method: "DELETE",
        headers,
      })
      if (!resp.ok) throw new Error(`waveAutomation delete failed: ${resp.status}`)
      return { status: "ok" }
    }
    default:
      throw new Error(`Unknown action: ${(params as WaveAutomationParams).action}`)
  }
}

// axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md#wave-integration plan=phase-1.3/task-1.3.8/step-1.3.8.1

// ── codeops_wave_comments ───────────────────────────────────────────────────

export interface WaveCommentsParams {
  space_id: string
  item_id?: string
  status?: "open" | "resolved" | "all"
}

export interface WaveCommentsResult {
  comments: object[]
  total: number
}

export async function waveComments(
  params: WaveCommentsParams,
  config: WaveToolConfig,
): Promise<WaveCommentsResult> {
  const fetch = config._mockFetch ?? globalThis.fetch
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(config.authToken ? { Authorization: `Bearer ${config.authToken}` } : {}),
  }

  const query = new URLSearchParams()
  if (params.item_id) query.set("item_id", params.item_id)
  if (params.status) query.set("status", params.status)
  const queryStr = query.toString()
  const url = `${config.backendUrl}/api/v1/spaces/${params.space_id}/comments${queryStr ? "?" + queryStr : ""}`

  const resp = await fetch(url, { headers })
  if (!resp.ok) throw new Error(`waveComments failed: ${resp.status}`)
  const data = await resp.json() as { comments?: object[] } | object[]
  const comments = Array.isArray(data) ? data : (data as { comments?: object[] }).comments ?? []

  return { comments, total: comments.length }
}

// axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md#wave-integration plan=phase-1.3/task-1.3.9/step-1.3.9.1

// ── codeops_wave_execute_action ─────────────────────────────────────────────
// Executes an approved Wave action by creating a real OpenCode session via the
// OpenCode HTTP API (POST /message). Session progress is reported back to the
// Wave backend so it is visible in the Space as a live session panel.
//
// REQ-WEAVE-062: spin up an OpenCode session when a human approves an action.
// REQ-WEAVE-064: session progress visible in Space.
// axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-062,REQ-WEAVE-064 plan=phase-4.7/task-4.7.execution/step-4.7.1

export interface WaveExecuteActionParams {
  space_id: string
  action_id: string
  /** Optional work item ID to associate with the session */
  work_item?: string
}

export interface WaveExecuteActionResult {
  session_id: string
  session_link_item_id: string
  status: "starting" | "running" | "completed" | "failed"
}

/**
 * waveExecuteAction — executes an approved Wave action by creating a real OpenCode session.
 *
 * Steps:
 *   1. Fetch action details from the Wave backend.
 *   2. POST to the OpenCode HTTP API (/message) to create a session.
 *   3. Report session progress back to the Wave backend (non-critical).
 *
 * The OpenCode URL is resolved from (in priority order):
 *   config.openCodeUrl → AXIOM_OPENCODE_BASE_URL env var → http://127.0.0.1:4096
 *
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-062,REQ-WEAVE-064 plan=phase-4.7/task-4.7.execution/step-4.7.1
 */
export async function waveExecuteAction(
  params: WaveExecuteActionParams,
  config: WaveToolConfig,
): Promise<WaveExecuteActionResult> {
  const fetch = config._mockFetch ?? globalThis.fetch
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(config.authToken ? { Authorization: `Bearer ${config.authToken}` } : {}),
  }

  // 1. Fetch the action details from the Wave backend
  const actionResp = await fetch(
    `${config.backendUrl}/api/v1/spaces/${params.space_id}/items/${params.action_id}`,
    { headers },
  )
  if (!actionResp.ok) {
    throw new Error(`waveExecuteAction: action fetch failed ${actionResp.status}`)
  }
  const actionData = await actionResp.json() as {
    item_id?: string
    content?: { title?: string; description?: string; action_type?: string }
  }

  // 2. Create a real OpenCode session via the HTTP API
  // URL resolution: config.openCodeUrl → env var → default
  const openCodeUrl =
    config.openCodeUrl ??
    (typeof process !== "undefined" ? process.env.AXIOM_OPENCODE_BASE_URL : undefined) ??
    "http://127.0.0.1:4096"

  const actionTitle = actionData.content?.title ?? params.action_id
  const actionDescription = actionData.content?.description ?? ""
  const workItemClause = params.work_item ? `\nWork item: ${params.work_item}` : ""

  const sessionResp = await fetch(`${openCodeUrl}/message`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      role: "user",
      content: `Execute Wave action: ${actionTitle}\n${actionDescription}${workItemClause}`,
    }),
  })
  if (!sessionResp.ok) {
    throw new Error(`waveExecuteAction: OpenCode session creation failed ${sessionResp.status}`)
  }
  const sessionData = await sessionResp.json() as {
    session_id?: string
    id?: string
    session_link_item_id?: string
  }
  const sessionId = sessionData.session_id ?? sessionData.id ?? `sess-${Date.now().toString(36)}`

  // 3. Report initial "running" progress to the Wave backend (non-critical — do not fail on error)
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-064 plan=phase-4.7/task-4.7.execution/step-4.7.2
  try {
    await fetch(
      `${config.backendUrl}/api/v1/spaces/${params.space_id}/sessions/${sessionId}/progress`,
      {
        method: "POST",
        headers,
        body: JSON.stringify({
          step_name: "Session started",
          status: "running",
          output_preview: `OpenCode session ${sessionId} created`,
        }),
      },
    )
  } catch {
    // Non-critical: progress update failure does not abort execution
  }

  // 4. Poll OpenCode for session status and post progress updates at each transition.
  //    Polling is non-critical — failures do not abort execution.
  //    Stops when terminal state (completed/failed) is reached or after 10 minutes.
  //
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-064 plan=phase-4.7/task-4.7.execution/step-4.7.2
  const pollInterval = setInterval(() => {
    void (async () => {
      try {
        // Check OpenCode session status
        const statusRes = await fetch(`${openCodeUrl}/session/${sessionId}`, {
          headers: { "Content-Type": "application/json" },
        })
        if (!statusRes.ok) return

        const statusData = await statusRes.json() as {
          status?: string
          current_step?: string
          output_preview?: string
        }
        const sessionStatus = statusData.status ?? "running"

        // Post progress update to Wave backend
        await fetch(
          `${config.backendUrl}/api/v1/spaces/${params.space_id}/sessions/${sessionId}/progress`,
          {
            method: "POST",
            headers,
            body: JSON.stringify({
              step_name: statusData.current_step ?? "Processing",
              status:
                sessionStatus === "completed"
                  ? "completed"
                  : sessionStatus === "failed"
                    ? "failed"
                    : "running",
              output_preview: statusData.output_preview ?? "",
            }),
          },
        )

        // Stop polling when terminal state is reached
        if (sessionStatus === "completed" || sessionStatus === "failed") {
          clearInterval(pollInterval)
        }
      } catch {
        // Non-critical: polling failure does not abort execution
      }
    })()
  }, 2000) // Poll every 2 seconds

  // Hard stop after 10 minutes to prevent runaway polling
  const hardStop = setTimeout(() => clearInterval(pollInterval), 600_000)
  if (hardStop && typeof hardStop.unref === "function") hardStop.unref()

  return {
    session_id: sessionId,
    session_link_item_id: sessionData.session_link_item_id ?? actionData.item_id ?? "",
    status: "running",
  }
}

// ── waveApplyMerge ───────────────────────────────────────────────────────────
// Applies a Wave share's content to the current OpenCode session by calling the
// auto-merge endpoint.  The backend returns the share payload; the plugin then
// formats it as structured feedback and sends it to the active OpenCode session
// via POST /session/{sessionId}/message.
//
// REQ-WEAVE-203: Auto-merge shares — step 4: apply to active session.
// axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-203 plan=phase-7.0/task-7.0.merge-apply/step-7.0.1

export interface WaveApplyMergeResult {
  success: boolean
  itemsApplied: number
  message: string
}

/**
 * Apply a Wave share's content to the current OpenCode session.
 *
 * Step 1: POST /api/v1/spaces/{spaceId}/shares/{shareId}/merge on the Wave
 *         backend to fetch the merge_payload.
 * Step 2: Format the payload as structured feedback and POST it to the active
 *         OpenCode session via {openCodeUrl}/session/{sessionId}/message.
 *         Session apply failure is non-fatal — the merge data was still fetched.
 *
 * The OpenCode URL is resolved from (in priority order):
 *   config.openCodeUrl → AXIOM_OPENCODE_BASE_URL env var → http://127.0.0.1:4096
 *
 * @param spaceId   - The Space that owns the share.
 * @param shareId   - The share to merge.
 * @param sessionId - The active OpenCode session ID to apply feedback to.
 * @param config    - WaveToolConfig (backendUrl, optional authToken / openCodeUrl / _mockFetch).
 *
 * REQ-WEAVE-203: Auto-merge shares — step 4: apply to active session.
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-203 plan=phase-7.0/task-7.0.merge-apply/step-7.0.1
 */
export async function waveApplyMerge(
  spaceId: string,
  shareId: string,
  sessionId: string,
  config: WaveToolConfig,
): Promise<WaveApplyMergeResult> {
  const fetch = config._mockFetch ?? globalThis.fetch
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(config.authToken ? { Authorization: `Bearer ${config.authToken}` } : {}),
  }

  // Step 1: Fetch merge payload from Wave backend
  const mergeResponse = await fetch(
    `${config.backendUrl}/api/v1/spaces/${spaceId}/shares/${shareId}/merge`,
    { method: "POST", headers },
  )

  if (!mergeResponse.ok) {
    const err = await mergeResponse.json().catch(() => ({})) as { error?: { message?: string } }
    return {
      success: false,
      itemsApplied: 0,
      message: err?.error?.message ?? "Merge failed",
    }
  }

  const mergeData = await mergeResponse.json() as {
    item_count?: number
    share_id?: string
    merge_payload?: {
      comments?: Record<string, unknown>[]
      items?: { type?: string; content?: unknown }[]
    }
  }
  const itemCount = mergeData.item_count ?? 0
  const mergePayload = mergeData.merge_payload

  // Step 2: Apply merge payload to active OpenCode session (non-fatal if session unavailable)
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-203 plan=phase-7.0/task-7.0.merge-apply/step-7.0.1
  if (itemCount > 0 && mergePayload) {
    const openCodeUrl =
      config.openCodeUrl ??
      (typeof process !== "undefined" ? process.env.AXIOM_OPENCODE_BASE_URL : undefined) ??
      "http://127.0.0.1:4096"

    // Format the merge payload as structured feedback for the session
    const feedbackItems: string[] = [
      ...(mergePayload.comments ?? []).map((c) =>
        `Comment from Wave Space ${spaceId}: ${JSON.stringify(c)}`
      ),
      ...(mergePayload.items ?? []).map((item) =>
        `Wave item (${item.type ?? "unknown"}): ${JSON.stringify(item.content)}`
      ),
    ]

    if (feedbackItems.length > 0) {
      const MAX_ITEMS = 10
      const feedbackMessage = [
        `Applying ${itemCount} item(s) from Wave share ${shareId}:`,
        ...feedbackItems.slice(0, MAX_ITEMS),
        feedbackItems.length > MAX_ITEMS
          ? `... and ${feedbackItems.length - MAX_ITEMS} more items`
          : "",
      ]
        .filter(Boolean)
        .join("\n")

      // Send structured feedback to the active OpenCode session
      // API contract: POST /session/{id}/message with {parts: [{type: "text", text: string}]}
      // per specs/31-OpenCode-Integration-Contract.md#2.2.3
      // axiom:trace work_item=wave-01 spec=specs/31-OpenCode-Integration-Contract.md#2.2.3 plan=phase-7.0/task-7.0.merge-apply/step-7.0.1
      const sessionResponse = await fetch(
        `${openCodeUrl}/session/${sessionId}/message`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ parts: [{ type: "text", text: feedbackMessage }] }),
        },
      ).catch(() => null)

      if (!sessionResponse?.ok) {
        // Non-fatal: merge data was fetched successfully; session apply failed
        return {
          success: true,
          itemsApplied: itemCount,
          message: `Fetched ${itemCount} item(s) but could not apply to session (session may not be active)`,
        }
      }
    }
  }

  return {
    success: true,
    itemsApplied: itemCount,
    message: `Applied ${itemCount} item(s) from share ${shareId} to session`,
  }
}