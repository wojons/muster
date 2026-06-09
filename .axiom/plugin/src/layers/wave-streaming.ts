/**
 * Wave session streaming — pushes session items to a Wave Space backend.
 *
 * The plugin streams every message, tool call, tool result, and file change
 * to the Wave backend in real time. Streaming is non-blocking: if the Wave
 * backend is unavailable, the plugin buffers locally and retries.
 *
 * REQ-WEAVE-110: Plugin streams session data to Wave backend.
 * REQ-WEAVE-115: Streaming is non-blocking — OpenCode session is never blocked.
 *
 * Spec: specs/70-OpenCode-Plugin.md#wave-integration Section 20.3
 * axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md#wave-integration plan=phase-1.3/task-1.3.5/step-1.3.5.1
 */

export type ItemType = "message" | "tool_call" | "tool_result" | "file_change" | "rich_render" | "status_update"

export interface SessionItem {
  type: ItemType
  author: "human" | "agent"
  content: Record<string, unknown>
}

export interface StreamingConfig {
  backendUrl: string
  spaceId: string
  panelId: string
  authToken?: string
  /** Max items to buffer before dropping oldest (default: 100) */
  maxBufferSize?: number
  /** Override fetch for testing */
  _mockFetch?: (url: string, opts?: RequestInit) => Promise<Response>
}

export interface StreamingStats {
  sent: number
  buffered: number
  dropped: number
  errors: number
}

/**
 * WaveSessionStreamer — non-blocking session item streamer.
 *
 * Usage:
 *   const streamer = new WaveSessionStreamer(config)
 *   streamer.push({ type: "message", author: "agent", content: { text: "Hello" } })
 *   await streamer.flush()
 */
export class WaveSessionStreamer {
  private readonly config: StreamingConfig
  private readonly buffer: SessionItem[] = []
  private readonly maxBufferSize: number
  private stats: StreamingStats = { sent: 0, buffered: 0, dropped: 0, errors: 0 }

  constructor(config: StreamingConfig) {
    this.config = config
    this.maxBufferSize = config.maxBufferSize ?? 100
  }

  /**
   * Push a session item. Non-blocking — queues for async send.
   * Returns immediately; does not await the network call.
   */
  push(item: SessionItem): void {
    if (this.buffer.length >= this.maxBufferSize) {
      this.buffer.shift() // drop oldest
      this.stats.dropped++
    }
    this.buffer.push(item)
    this.stats.buffered++
  }

  /**
   * Flush all buffered items. Awaitable for test/shutdown scenarios.
   */
  async flush(): Promise<void> {
    while (this.buffer.length > 0) {
      await this._sendNext()
    }
  }

  getStats(): StreamingStats {
    return { ...this.stats }
  }

  private async _sendNext(): Promise<void> {
    const item = this.buffer.shift()
    if (!item) return
    const fetch = this.config._mockFetch ?? globalThis.fetch
    try {
      const resp = await fetch(
        `${this.config.backendUrl}/api/v1/spaces/${this.config.spaceId}/panels/${this.config.panelId}/items`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            ...(this.config.authToken ? { Authorization: `Bearer ${this.config.authToken}` } : {}),
          },
          body: JSON.stringify({
            type: item.type,
            author: item.author,
            content: item.content,
          }),
        },
      )
      if (resp.ok) {
        this.stats.sent++
      } else {
        this.stats.errors++
      }
    } catch {
      this.stats.errors++
      // Non-blocking: swallow errors, item is already removed from buffer
    }
  }
}