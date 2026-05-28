/**
 * Wave /axiom-status-page command handler.
 *
 * Creates a status_page Space with a default manual automation and returns
 * the Space URL for the user to open in the browser.
 *
 * REQ-WEAVE-100: Plugin commands create typed Spaces via the Wave backend.
 *
 * Spec: specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
 */

export interface StatusPageOptions {
  /** Display title for the status page Space (default: "Status Page") */
  title?: string
  /** Prompt used by the automation to generate content (default: "Generate a status summary for this project") */
  prompt?: string
}

export interface StatusPageConfig {
  backendUrl: string
  /** Override fetch for testing */
  _mockFetch?: typeof fetch
}

export interface StatusPageResult {
  spaceId: string
  url: string
  title: string
  automationCreated: boolean
}

/**
 * Handle /axiom-status-page [--title <title>] [--prompt <prompt>]
 *
 * 1. Creates a status_page Space via POST /api/v1/spaces.
 * 2. Creates a default manual automation via POST /api/v1/spaces/{id}/automations.
 *    Automation failure is non-critical — the Space is still returned.
 * 3. Returns the Space URL and a success message.
 *
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
 */
export async function handleStatusPageCommand(
  opts: StatusPageOptions,
  config: StatusPageConfig,
): Promise<StatusPageResult> {
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
  const fetchFn = config._mockFetch ?? globalThis.fetch
  const title = opts.title ?? "Status Page"
  const prompt = opts.prompt ?? "Generate a status summary for this project"

  // Step 1: Create the status_page Space
  const spaceRes = await fetchFn(`${config.backendUrl}/api/v1/spaces`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      title,
      type: "status_page",
      access_scope: "team",
      prompt,
      refresh_trigger: "manual",
    }),
  })

  if (!spaceRes.ok) {
    throw new Error(`Failed to create status page Space: ${spaceRes.status} ${spaceRes.statusText}`)
  }

  const space = (await spaceRes.json()) as { space_id: string; title: string }
  const spaceId = space.space_id

  // Step 2: Create a default manual automation (non-critical — failure is tolerated)
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
  let automationCreated = false
  try {
    const autoRes = await fetchFn(
      `${config.backendUrl}/api/v1/spaces/${spaceId}/automations`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: "Manual Refresh",
          automation_type: "manual",
          action_type: "run_prompt",
          config: { prompt },
        }),
      },
    )
    automationCreated = autoRes.ok
  } catch {
    // Non-critical: automation creation failure does not block Space creation
    automationCreated = false
  }

  // Step 3: Build the Space URL
  const spaceUrl = `${config.backendUrl}/ui/spaces/${spaceId}`

  return {
    spaceId,
    url: spaceUrl,
    title: space.title ?? title,
    automationCreated,
  }
}

/**
 * Format the user-facing success message for /axiom-status-page.
 *
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
 */
export function formatStatusPageMessage(result: StatusPageResult): string {
  const lines = [
    `✅ Status page created: ${result.url}`,
    ``,
    `Run /axiom-status-page to refresh it.`,
  ]
  if (!result.automationCreated) {
    lines.push(`⚠️  Default automation could not be created — you can add one manually in the Space settings.`)
  }
  return lines.join("\n")
}
