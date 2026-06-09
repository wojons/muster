/**
 * Wave-aware share handler — upgrades /axiom-share to use Wave share API.
 *
 * When Wave backend is available: creates a Wave Space + share link.
 * Falls back to existing local share when Wave is unavailable.
 *
 * REQ-WEAVE-103: /axiom-share is Wave-aware — creates a share via Wave API.
 * REQ-WEAVE-300: Redaction applied before sharing.
 * REQ-WEAVE-009: Share links expire after 30 days by default.
 *
 * axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md#wave-integration plan=phase-1.3/task-1.3.3/step-1.3.3.1
 */

export interface WaveShareOptions {
  backendUrl: string
  authToken?: string
  spaceId?: string          // If provided, share this Space; otherwise create a new one
  workItemId?: string
  sessionId?: string
  accessScope?: "private" | "team" | "link" | "public"
  expiresInDays?: number    // Default: 30
  /** Override fetch for testing */
  _mockFetch?: (url: string, opts?: RequestInit) => Promise<Response>
}

export interface WaveShareResult {
  shareId: string
  url: string
  expiresAt: string
  accessScope: string
  spaceId: string
  isWaveShare: true
}

export interface LocalShareResult {
  url: string
  isWaveShare: false
}

export type ShareResult = WaveShareResult | LocalShareResult

/**
 * Create a Wave share link for a Space.
 * If spaceId is not provided, creates a new session_review Space first.
 */
export async function createWaveShare(opts: WaveShareOptions): Promise<WaveShareResult> {
  const fetch = opts._mockFetch ?? globalThis.fetch
  let spaceId = opts.spaceId

  // If no spaceId, create a new Space first
  if (!spaceId) {
    const createResp = await fetch(`${opts.backendUrl}/api/v1/spaces`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(opts.authToken ? { Authorization: `Bearer ${opts.authToken}` } : {}),
      },
      body: JSON.stringify({
        title: opts.sessionId ? `Share: ${opts.sessionId}` : "Shared Session",
        type: "share",
        work_item_id: opts.workItemId ?? null,
        access_scope: opts.accessScope ?? "link",
      }),
    })
    if (!createResp.ok) {
      throw new Error(`Failed to create Space for share: ${createResp.status}`)
    }
    const space = await createResp.json() as { space_id: string }
    spaceId = space.space_id
  }

  // Create the share link
  const shareResp = await fetch(`${opts.backendUrl}/api/v1/spaces/${spaceId}/share`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...(opts.authToken ? { Authorization: `Bearer ${opts.authToken}` } : {}),
    },
    body: JSON.stringify({
      access_scope: opts.accessScope ?? "link",
      expires_in_days: opts.expiresInDays ?? 30,
    }),
  })

  if (!shareResp.ok) {
    throw new Error(`Failed to create Wave share: ${shareResp.status}`)
  }

  const share = await shareResp.json() as {
    share_id: string; url: string; expires_at: string; access_scope: string; space_id: string
  }

  return {
    shareId: share.share_id,
    url: share.url,
    expiresAt: share.expires_at,
    accessScope: share.access_scope,
    spaceId: share.space_id,
    isWaveShare: true,
  }
}

/**
 * Try Wave share; fall back to local share URL on failure.
 */
export async function shareWithWaveFallback(
  opts: WaveShareOptions,
  localFallback: () => Promise<string>,
): Promise<ShareResult> {
  try {
    return await createWaveShare(opts)
  } catch {
    const url = await localFallback()
    return { url, isWaveShare: false }
  }
}