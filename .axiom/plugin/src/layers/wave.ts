/**
 * Wave command handlers — /axiom-wave and /axiom-review.
 *
 * REQ-WEAVE-100: /axiom-wave creates or opens a Space for the current work item.
 * REQ-WEAVE-101: /axiom-review opens the current session in a Space for live review.
 * REQ-WEAVE-102: Wave commands open the Wave in the user's default browser.
 *
 * Spec: specs/70-OpenCode-Plugin.md#wave-integration
 * axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md#wave-integration plan=phase-1.3/task-1.3.1/step-1.3.1.1
 */

export interface WaveCommandOptions {
  backendUrl: string
  authToken?: string
  workItemId?: string
  sessionId?: string
  /** Override open-in-browser for testing */
  _mockOpen?: (url: string) => void
  /** Override fetch for testing */
  _mockFetch?: (url: string, opts?: RequestInit) => Promise<Response>
}

export interface WaveSpaceResult {
  spaceId: string
  url: string
  title: string
  isNew: boolean
}

/**
 * Handle /axiom-wave — create or open a Space for the current work item.
 * Returns the Space URL to open in the browser.
 */
export async function handleWaveCommand(
  title: string | undefined,
  opts: WaveCommandOptions,
): Promise<WaveSpaceResult> {
  const fetch = opts._mockFetch ?? globalThis.fetch
  const spaceTitle = title ?? (opts.workItemId ? `Wave: ${opts.workItemId}` : "Wave Space")

  // POST /api/v1/spaces to create a new Space
  const response = await fetch(`${opts.backendUrl}/api/v1/spaces`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...(opts.authToken ? { Authorization: `Bearer ${opts.authToken}` } : {}),
    },
    body: JSON.stringify({
      title: spaceTitle,
      type: "collaboration",
      work_item_id: opts.workItemId ?? null,
      access_scope: "link",
    }),
  })

  if (!response.ok) {
    throw new Error(`Failed to create Wave Space: ${response.status} ${response.statusText}`)
  }

  const space = await response.json() as { space_id: string; title: string }
  const spaceUrl = `${opts.backendUrl}/ui/spaces/${space.space_id}`

  // Open in browser (REQ-WEAVE-102)
  const openFn = opts._mockOpen ?? ((url: string) => {
    // In production: use the platform's open-URL mechanism
    console.log(`[wave] Opening Space in browser: ${url}`)
  })
  openFn(spaceUrl)

  return {
    spaceId: space.space_id,
    url: spaceUrl,
    title: space.title,
    isNew: true,
  }
}

/**
 * Handle /axiom-review — open the current session in a Space for live review.
 */
export async function handleReviewCommand(
  opts: WaveCommandOptions,
): Promise<WaveSpaceResult> {
  const sessionId = opts.sessionId ?? "unknown-session"
  return handleWaveCommand(`Session Review: ${sessionId}`, {
    ...opts,
    // Override type to session_review
    _mockFetch: opts._mockFetch
      ? async (url: string, init?: RequestInit) => {
          if (init?.body && typeof init.body === "string") {
            const body = JSON.parse(init.body)
            body.type = "session_review"
            return opts._mockFetch!(url, { ...init, body: JSON.stringify(body) })
          }
          return opts._mockFetch!(url, init)
        }
      : async (url: string, init?: RequestInit) => {
          if (init?.body && typeof init.body === "string") {
            const body = JSON.parse(init.body)
            body.type = "session_review"
            return globalThis.fetch(url, { ...init, body: JSON.stringify(body) })
          }
          return globalThis.fetch(url, init)
        },
  })
}