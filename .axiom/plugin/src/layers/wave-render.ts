/**
 * Wave /axiom-render command handler.
 *
 * REQ-WEAVE-100: /axiom-render creates a rich_render item in the current
 *   Space using one of the pre-built HTML/CSS templates.
 *
 * Spec: specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-2.8/task-2.8.2/step-2.8.2
 */

/**
 * Allowed template names — validated against this allowlist to prevent
 * path traversal or injection attacks.
 *
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-2.8/task-2.8.2/step-2.8.2
 */
export const ALLOWED_TEMPLATES = ["report", "dashboard", "timeline", "kanban"] as const
export type TemplateName = (typeof ALLOWED_TEMPLATES)[number]

export interface RenderCommandOptions {
  backendUrl: string
  authToken?: string
  spaceId?: string
  /** Override fetch for testing */
  _mockFetch?: (url: string, opts?: RequestInit) => Promise<Response>
}

export interface RenderResult {
  itemId: string
  template: TemplateName
  spaceId: string
}

/**
 * Validate that a template name is in the allowlist.
 * Returns true if valid, false otherwise.
 *
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-2.8/task-2.8.2/step-2.8.2
 */
export function isValidTemplate(name: string): name is TemplateName {
  return (ALLOWED_TEMPLATES as readonly string[]).includes(name)
}

/**
 * Handle /axiom-render <template> [data_json]
 *
 * 1. Validates template name against the allowlist.
 * 2. Parses optional data_json argument.
 * 3. Creates a rich_render item in the current Space.
 * 4. Returns confirmation message.
 *
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-2.8/task-2.8.2/step-2.8.2
 */
export async function handleRenderCommand(
  templateName: string,
  dataJson: string | undefined,
  opts: RenderCommandOptions,
): Promise<RenderResult> {
  // Validate template name against allowlist (prevents path traversal)
  if (!isValidTemplate(templateName)) {
    throw new Error(
      `Unknown template "${templateName}". Allowed templates: ${ALLOWED_TEMPLATES.join(", ")}`,
    )
  }

  // Parse optional data JSON
  let data: unknown = undefined
  if (dataJson && dataJson.trim() !== "") {
    try {
      data = JSON.parse(dataJson)
    } catch {
      throw new Error(`Invalid JSON in data argument: ${dataJson}`)
    }
  }

  if (!opts.spaceId) {
    throw new Error("spaceId is required for /axiom-render — open a Wave Space first")
  }

  const fetch = opts._mockFetch ?? globalThis.fetch
  const authHeaders: Record<string, string> = opts.authToken
    ? { Authorization: `Bearer ${opts.authToken}` }
    : {}

  // Create a rich_render item in the current Space
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-2.8/task-2.8.2/step-2.8.2
  const response = await fetch(
    `${opts.backendUrl}/api/v1/spaces/${opts.spaceId}/items`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...authHeaders,
      },
      body: JSON.stringify({
        type: "rich_render",
        content: {
          template: templateName,
          data,
        },
      }),
    },
  )

  if (!response.ok) {
    throw new Error(
      `Failed to create rich_render item: ${response.status} ${response.statusText}`,
    )
  }

  const item = (await response.json()) as { item_id: string }

  return {
    itemId: item.item_id,
    template: templateName,
    spaceId: opts.spaceId,
  }
}
