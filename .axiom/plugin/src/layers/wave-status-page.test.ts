/**
 * Tests for /axiom-status-page command handler.
 *
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1 test=.axiom/plugin/src/layers/wave-status-page.test.ts
 */

import { beforeEach, describe, expect, it, vi } from "vitest"
import { handleStatusPageCommand, formatStatusPageMessage } from "./wave-status-page"

// Clear all mocks before each test to prevent vi.fn() state leakage in bun vitest compat.
beforeEach(() => { vi.clearAllMocks() })

const BACKEND = "http://localhost:8100"

/** Build a mock fetch that returns a sequence of responses, cycling on the last one. */
function mockFetchSequence(
  responses: Array<{
    ok: boolean
    status?: number
    statusText?: string
    json?: () => Promise<unknown>
  }>,
) {
  let call = 0
  const fn = vi.fn().mockImplementation(async () => {
    const r = responses[call++] ?? responses[responses.length - 1]
    return {
      ok: r.ok,
      status: r.status ?? (r.ok ? 201 : 500),
      statusText: r.statusText ?? (r.ok ? "Created" : "Server Error"),
      json: r.json ?? (async () => ({})),
    } as unknown as Response
  })
  return fn as unknown as typeof fetch & { mock: (typeof fn)["mock"] }
}

describe("handleStatusPageCommand", () => {
  // test_status_page_creates_space
  it("test_status_page_creates_space — creates space with type=status_page", async () => {
    // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
    const mockFetch = mockFetchSequence([
      { ok: true, json: async () => ({ space_id: "sp-status-1", title: "Status Page" }) },
      { ok: true, json: async () => ({ automation_id: "auto-1" }) },
    ])
    const result = await handleStatusPageCommand(
      {},
      { backendUrl: BACKEND, _mockFetch: mockFetch },
    )
    expect(result.spaceId).toBe("sp-status-1")
    // Verify the space creation call used type=status_page
    const spaceBody = JSON.parse(mockFetch.mock.calls[0][1].body as string)
    expect(spaceBody.type).toBe("status_page")
    expect(spaceBody.access_scope).toBe("team")
    expect(spaceBody.refresh_trigger).toBe("manual")
  })

  // test_status_page_uses_default_title
  it("test_status_page_uses_default_title — no title arg → title='Status Page'", async () => {
    // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
    const mockFetch = mockFetchSequence([
      { ok: true, json: async () => ({ space_id: "sp-default-title", title: "Status Page" }) },
      { ok: true, json: async () => ({ automation_id: "auto-2" }) },
    ])
    await handleStatusPageCommand(
      {},
      { backendUrl: BACKEND, _mockFetch: mockFetch },
    )
    const spaceBody = JSON.parse(mockFetch.mock.calls[0][1].body as string)
    expect(spaceBody.title).toBe("Status Page")
  })

  // test_status_page_uses_custom_title
  it("test_status_page_uses_custom_title — title='My Status' → space created with that title", async () => {
    // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
    const mockFetch = mockFetchSequence([
      { ok: true, json: async () => ({ space_id: "sp-custom-title", title: "My Status" }) },
      { ok: true, json: async () => ({ automation_id: "auto-3" }) },
    ])
    await handleStatusPageCommand(
      { title: "My Status" },
      { backendUrl: BACKEND, _mockFetch: mockFetch },
    )
    const spaceBody = JSON.parse(mockFetch.mock.calls[0][1].body as string)
    expect(spaceBody.title).toBe("My Status")
  })

  // test_status_page_creates_automation
  it("test_status_page_creates_automation — POST /automations called after space creation", async () => {
    // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
    const mockFetch = mockFetchSequence([
      { ok: true, json: async () => ({ space_id: "sp-auto-test", title: "Status Page" }) },
      { ok: true, json: async () => ({ automation_id: "auto-4" }) },
    ])
    const result = await handleStatusPageCommand(
      {},
      { backendUrl: BACKEND, _mockFetch: mockFetch },
    )
    // Two calls: POST /spaces, then POST /spaces/{id}/automations
    expect(mockFetch.mock.calls).toHaveLength(2)
    const autoUrl = mockFetch.mock.calls[1][0] as string
    expect(autoUrl).toContain(`/api/v1/spaces/sp-auto-test/automations`)
    const autoBody = JSON.parse(mockFetch.mock.calls[1][1].body as string)
    expect(autoBody.automation_type).toBe("manual")
    expect(autoBody.action_type).toBe("run_prompt")
    expect(result.automationCreated).toBe(true)
  })

  // test_status_page_returns_url
  it("test_status_page_returns_url — return value contains space URL", async () => {
    // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
    const mockFetch = mockFetchSequence([
      { ok: true, json: async () => ({ space_id: "sp-url-check", title: "Status Page" }) },
      { ok: true, json: async () => ({ automation_id: "auto-5" }) },
    ])
    const result = await handleStatusPageCommand(
      {},
      { backendUrl: BACKEND, _mockFetch: mockFetch },
    )
    expect(result.url).toContain("sp-url-check")
    expect(result.url).toContain(BACKEND)
    expect(result.url).toContain("/ui/spaces/")
  })

  // test_status_page_handles_space_creation_failure
  it("test_status_page_handles_space_creation_failure — POST /spaces returns 500 → throws", async () => {
    // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
    const mockFetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      statusText: "Internal Server Error",
    } as Response) as unknown as typeof fetch
    await expect(
      handleStatusPageCommand(
        {},
        { backendUrl: BACKEND, _mockFetch: mockFetch },
      ),
    ).rejects.toThrow("Failed to create status page Space: 500")
  })

  // test_status_page_automation_failure_is_non_critical
  it("test_status_page_automation_failure_is_non_critical — automation 500 does not throw", async () => {
    // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
    const mockFetch = mockFetchSequence([
      { ok: true, json: async () => ({ space_id: "sp-auto-fail", title: "Status Page" }) },
      { ok: false, status: 500, statusText: "Server Error" },
    ])
    // Should NOT throw — automation failure is tolerated
    const result = await handleStatusPageCommand(
      {},
      { backendUrl: BACKEND, _mockFetch: mockFetch },
    )
    expect(result.spaceId).toBe("sp-auto-fail")
    expect(result.automationCreated).toBe(false)
  })

  // test_status_page_uses_custom_prompt
  it("test_status_page_uses_custom_prompt — custom prompt is forwarded to space and automation", async () => {
    const mockFetch = mockFetchSequence([
      { ok: true, json: async () => ({ space_id: "sp-prompt", title: "Status Page" }) },
      { ok: true, json: async () => ({ automation_id: "auto-6" }) },
    ])
    await handleStatusPageCommand(
      { prompt: "Summarize CI status" },
      { backendUrl: BACKEND, _mockFetch: mockFetch },
    )
    const spaceBody = JSON.parse(mockFetch.mock.calls[0][1].body as string)
    expect(spaceBody.prompt).toBe("Summarize CI status")
    const autoBody = JSON.parse(mockFetch.mock.calls[1][1].body as string)
    expect(autoBody.config.prompt).toBe("Summarize CI status")
  })
})

describe("formatStatusPageMessage", () => {
  it("includes the space URL in the success message", () => {
    const msg = formatStatusPageMessage({
      spaceId: "sp-fmt-1",
      url: "http://localhost:8100/ui/spaces/sp-fmt-1",
      title: "Status Page",
      automationCreated: true,
    })
    expect(msg).toContain("http://localhost:8100/ui/spaces/sp-fmt-1")
    expect(msg).toContain("✅")
  })

  it("includes automation warning when automationCreated=false", () => {
    const msg = formatStatusPageMessage({
      spaceId: "sp-fmt-2",
      url: "http://localhost:8100/ui/spaces/sp-fmt-2",
      title: "Status Page",
      automationCreated: false,
    })
    expect(msg).toContain("⚠️")
    expect(msg).toContain("automation")
  })

  it("does not include warning when automationCreated=true", () => {
    const msg = formatStatusPageMessage({
      spaceId: "sp-fmt-3",
      url: "http://localhost:8100/ui/spaces/sp-fmt-3",
      title: "Status Page",
      automationCreated: true,
    })
    expect(msg).not.toContain("⚠️")
  })
})
