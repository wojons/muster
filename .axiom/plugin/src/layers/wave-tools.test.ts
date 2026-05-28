/**
 * Tests for Wave plugin tools.
 *
 * axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md#wave-integration plan=phase-1.3/task-1.3.4/step-1.3.4.1
 * axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md plan=phase-3.5/task-3.5.mentions/step-3.5.4
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-064 plan=phase-3.7/task-3.7.execution/step-3.7.2
 */

import { afterEach, describe, expect, it, vi } from "vitest"
import { waveRead, waveProposeAction, waveWrite, waveAutomation, waveComments, waveExecuteAction, WAVE_TOOL_NAMES } from "./wave-tools"

// Reset mocks after each test so queued mockResolvedValueOnce values
// do not leak into other test files (bun vitest compat issue on Linux).
afterEach(() => { vi.clearAllMocks() })

const BACKEND = "http://localhost:8100"
const CONFIG = { backendUrl: BACKEND }

function okJson(data: object) {
  return { ok: true, status: 200, json: async () => data } as unknown as Response
}

describe("waveRead", () => {
  it("fetches panels, comments, and actions", async () => {
    const mockFetch = vi.fn()
      .mockResolvedValueOnce(okJson([{ panel_id: "pn-1" }]))   // panels
      .mockResolvedValueOnce(okJson([{ comment_id: "cm-1" }])) // comments
      .mockResolvedValueOnce(okJson([{ action_id: "ac-1" }]))  // actions
    const result = await waveRead({ space_id: "sp-1" }, { ...CONFIG, _mockFetch: mockFetch })
    expect(result.space_id).toBe("sp-1")
    expect(result.panels).toHaveLength(1)
    expect(result.comments).toHaveLength(1)
    expect(result.actions).toHaveLength(1)
  })

  it("skips comments when include_comments=false", async () => {
    const mockFetch = vi.fn()
      .mockResolvedValueOnce(okJson([]))  // panels
      .mockResolvedValueOnce(okJson([]))  // actions (no comments call)
    const result = await waveRead(
      { space_id: "sp-2", include_comments: false },
      { ...CONFIG, _mockFetch: mockFetch },
    )
    expect(mockFetch).toHaveBeenCalledTimes(2) // panels + actions only
    expect(result.comments).toHaveLength(0)
  })

  it("skips actions when include_action_list=false", async () => {
    const mockFetch = vi.fn()
      .mockResolvedValueOnce(okJson([]))  // panels
      .mockResolvedValueOnce(okJson([]))  // comments (no actions call)
    await waveRead(
      { space_id: "sp-3", include_action_list: false },
      { ...CONFIG, _mockFetch: mockFetch },
    )
    expect(mockFetch).toHaveBeenCalledTimes(2) // panels + comments only
  })

  it("throws on panels fetch failure", async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: false, status: 404 } as Response)
    await expect(waveRead({ space_id: "sp-x" }, { ...CONFIG, _mockFetch: mockFetch }))
      .rejects.toThrow("404")
  })

  it("returns empty arrays when comments/actions endpoints fail gracefully", async () => {
    const mockFetch = vi.fn()
      .mockResolvedValueOnce(okJson([]))                        // panels ok
      .mockResolvedValueOnce({ ok: false, status: 500 } as Response) // comments fail
      .mockResolvedValueOnce({ ok: false, status: 500 } as Response) // actions fail
    const result = await waveRead({ space_id: "sp-4" }, { ...CONFIG, _mockFetch: mockFetch })
    expect(result.comments).toHaveLength(0)
    expect(result.actions).toHaveLength(0)
  })

  // -------------------------------------------------------------------------
  // test_wave_read_includes_chat_when_flag_set
  // REQ: include_chat=true → recent_chat array in response
  // axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md plan=phase-3.5/task-3.5.mentions/step-3.5.4
  // -------------------------------------------------------------------------
  it("test_wave_read_includes_chat_when_flag_set — include_chat=true fetches chat and returns recent_chat array", async () => {
    const chatItems = [
      { item_id: "ch-1", author: "alice", content: { text: "Hello @bob" } },
      { item_id: "ch-2", author: "bob",   content: { text: "Hey @alice" } },
    ]
    const mockFetch = vi.fn()
      .mockResolvedValueOnce(okJson([]))          // panels
      .mockResolvedValueOnce(okJson([]))          // comments
      .mockResolvedValueOnce(okJson([]))          // actions
      .mockResolvedValueOnce(okJson({ items: chatItems })) // chat
    const result = await waveRead(
      { space_id: "sp-chat", include_chat: true },
      { ...CONFIG, _mockFetch: mockFetch },
    )
    // Chat endpoint must be called with correct URL
    const chatCall = mockFetch.mock.calls[3]
    expect(chatCall[0]).toBe("http://localhost:8100/api/v1/spaces/sp-chat/chat?limit=20")
    // recent_chat must be present and populated
    expect(result.recent_chat).toBeDefined()
    expect(result.recent_chat).toHaveLength(2)
    expect((result.recent_chat?.[0] as { item_id: string }).item_id).toBe("ch-1")
  })

  // -------------------------------------------------------------------------
  // test_wave_read_excludes_chat_by_default
  // REQ: no include_chat flag → no recent_chat in response, no chat fetch
  // axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md plan=phase-3.5/task-3.5.mentions/step-3.5.4
  // -------------------------------------------------------------------------
  it("test_wave_read_excludes_chat_by_default — omitting include_chat returns no recent_chat and skips chat fetch", async () => {
    const mockFetch = vi.fn()
      .mockResolvedValueOnce(okJson([]))  // panels
      .mockResolvedValueOnce(okJson([]))  // comments
      .mockResolvedValueOnce(okJson([]))  // actions
    const result = await waveRead(
      { space_id: "sp-no-chat" },
      { ...CONFIG, _mockFetch: mockFetch },
    )
    // Only 3 fetches (panels + comments + actions) — no chat fetch
    expect(mockFetch).toHaveBeenCalledTimes(3)
    // recent_chat must be absent
    expect(result.recent_chat).toBeUndefined()
  })
})

describe("waveProposeAction", () => {
  it("posts action and returns result", async () => {
    const mockFetch = vi.fn().mockResolvedValue(
      okJson({ action_id: "ac-new", status: "proposed", title: "Fix the bug" })
    )
    const result = await waveProposeAction(
      { space_id: "sp-1", title: "Fix the bug", description: "Details here" },
      { ...CONFIG, _mockFetch: mockFetch },
    )
    expect(result.action_id).toBe("ac-new")
    expect(result.status).toBe("proposed")
  })

  it("uses medium priority by default", async () => {
    const mockFetch = vi.fn().mockResolvedValue(
      okJson({ action_id: "ac-2", status: "proposed", title: "T" })
    )
    await waveProposeAction(
      { space_id: "sp-1", title: "T", description: "D" },
      { ...CONFIG, _mockFetch: mockFetch },
    )
    const body = JSON.parse(mockFetch.mock.calls[0][1].body)
    expect(body.priority).toBe("medium")
  })

  it("throws on non-ok response", async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: false, status: 422 } as Response)
    await expect(
      waveProposeAction({ space_id: "sp-1", title: "T", description: "D" }, { ...CONFIG, _mockFetch: mockFetch })
    ).rejects.toThrow("422")
  })
})

describe("waveWrite", () => {
  it("test_wave_write_creates_panel_and_item: no panel_id, mockFetch called twice, result has item_id", async () => {
    const mockFetch = vi.fn()
      .mockResolvedValueOnce(okJson({ panel_id: "pn-new" }))           // create panel
      .mockResolvedValueOnce(okJson({ item_id: "it-123" }))            // create item
    const result = await waveWrite(
      { space_id: "sp-1", content_type: "markdown", content: "# Hello" },
      { ...CONFIG, _mockFetch: mockFetch },
    )
    expect(mockFetch).toHaveBeenCalledTimes(2)
    expect(result.item_id).toBe("it-123")
    expect(result.panel_id).toBe("pn-new")
    expect(result.space_id).toBe("sp-1")
  })

  it("test_wave_write_uses_existing_panel: panel_id given, mockFetch called once", async () => {
    const mockFetch = vi.fn()
      .mockResolvedValueOnce(okJson({ item_id: "it-456" }))            // create item only
    const result = await waveWrite(
      { space_id: "sp-1", panel_id: "pn-existing", content_type: "html", content: "<b>Hi</b>" },
      { ...CONFIG, _mockFetch: mockFetch },
    )
    expect(mockFetch).toHaveBeenCalledTimes(1)
    expect(result.item_id).toBe("it-456")
    expect(result.panel_id).toBe("pn-existing")
  })

  it("test_wave_write_throws_on_error: non-ok fetch throws", async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: false, status: 500 } as Response)
    await expect(
      waveWrite({ space_id: "sp-1", content_type: "json", content: "{}" }, { ...CONFIG, _mockFetch: mockFetch })
    ).rejects.toThrow("500")
  })
})

describe("waveAutomation", () => {
  it("test_wave_automation_list: action=list, GET called, returns automations array", async () => {
    const mockFetch = vi.fn().mockResolvedValue(
      okJson([{ automation_id: "auto-1", name: "Test Automation" }])
    )
    const result = await waveAutomation(
      { space_id: "sp-1", action: "list" },
      { ...CONFIG, _mockFetch: mockFetch },
    )
    expect(mockFetch).toHaveBeenCalledTimes(1)
    expect(mockFetch.mock.calls[0][0]).toBe("http://localhost:8100/api/v1/spaces/sp-1/automations")
    expect(mockFetch.mock.calls[0][1]?.method).toBeUndefined() // GET
    expect(result.automations).toHaveLength(1)
    expect(result.automations?.[0]).toEqual({ automation_id: "auto-1", name: "Test Automation" })
    expect(result.status).toBe("ok")
  })

  it("test_wave_automation_create: action=create, POST called, returns automation_id", async () => {
    const mockFetch = vi.fn().mockResolvedValue(
      okJson({ automation_id: "auto-new", status: "created" })
    )
    const result = await waveAutomation(
      { space_id: "sp-1", action: "create", automation: { name: "New Auto", trigger: "on_event" } },
      { ...CONFIG, _mockFetch: mockFetch },
    )
    expect(mockFetch).toHaveBeenCalledTimes(1)
    expect(mockFetch.mock.calls[0][0]).toBe("http://localhost:8100/api/v1/spaces/sp-1/automations")
    expect(mockFetch.mock.calls[0][1]?.method).toBe("POST")
    expect(result.automation_id).toBe("auto-new")
    expect(result.status).toBe("ok")
  })

  it("test_wave_automation_enable: action=enable, PATCH called", async () => {
    const mockFetch = vi.fn().mockResolvedValue(okJson({ status: "ok" }))
    const result = await waveAutomation(
      { space_id: "sp-1", action: "enable", automation_id: "auto-1", automation: { enabled: true } },
      { ...CONFIG, _mockFetch: mockFetch },
    )
    expect(mockFetch).toHaveBeenCalledTimes(1)
    expect(mockFetch.mock.calls[0][0]).toBe("http://localhost:8100/api/v1/spaces/sp-1/automations/auto-1")
    expect(mockFetch.mock.calls[0][1]?.method).toBe("PATCH")
    expect(result.status).toBe("ok")
  })

  it("test_wave_automation_throws_on_error: non-ok throws", async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: false, status: 403 } as Response)
    await expect(
      waveAutomation({ space_id: "sp-1", action: "list" }, { ...CONFIG, _mockFetch: mockFetch })
    ).rejects.toThrow("403")
  })
})

describe("waveComments", () => {
  it("test_wave_comments_all: no filters, GET called, returns comments array", async () => {
    const mockFetch = vi.fn().mockResolvedValue(
      okJson([{ comment_id: "cm-1" }, { comment_id: "cm-2" }])
    )
    const result = await waveComments(
      { space_id: "sp-1" },
      { ...CONFIG, _mockFetch: mockFetch },
    )
    expect(mockFetch).toHaveBeenCalledTimes(1)
    expect(mockFetch.mock.calls[0][0]).toBe("http://localhost:8100/api/v1/spaces/sp-1/comments")
    expect(mockFetch.mock.calls[0][1]?.method).toBeUndefined() // GET
    expect(result.comments).toHaveLength(2)
    expect(result.total).toBe(2)
  })

  it("test_wave_comments_filter_by_item: item_id in URL query string", async () => {
    const mockFetch = vi.fn().mockResolvedValue(okJson([{ comment_id: "cm-1", item_id: "it-123" }]))
    const result = await waveComments(
      { space_id: "sp-1", item_id: "it-123" },
      { ...CONFIG, _mockFetch: mockFetch },
    )
    expect(mockFetch.mock.calls[0][0]).toContain("item_id=it-123")
    expect(result.comments).toHaveLength(1)
  })

  it("test_wave_comments_filter_by_status: status=open in URL query string", async () => {
    const mockFetch = vi.fn().mockResolvedValue(okJson([{ comment_id: "cm-1", status: "open" }]))
    const result = await waveComments(
      { space_id: "sp-1", status: "open" },
      { ...CONFIG, _mockFetch: mockFetch },
    )
    expect(mockFetch.mock.calls[0][0]).toContain("status=open")
    expect(result.comments).toHaveLength(1)
  })

  it("test_wave_comments_throws_on_error: non-ok throws", async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: false, status: 500 } as Response)
    await expect(
      waveComments({ space_id: "sp-1" }, { ...CONFIG, _mockFetch: mockFetch })
    ).rejects.toThrow("500")
  })
})

// ---------------------------------------------------------------------------
// waveExecuteAction tests (REQ-WEAVE-062, REQ-WEAVE-064)
// axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-062,REQ-WEAVE-064 plan=phase-4.7/task-4.7.execution/step-4.7.1
// ---------------------------------------------------------------------------

describe("waveExecuteAction", () => {
  // -------------------------------------------------------------------------
  // test_wave_execute_action_calls_opencode_api
  // REQ-WEAVE-062: waveExecuteAction must POST to OpenCode /message with correct content
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-062 plan=phase-4.7/task-4.7.execution/step-4.7.1
  // -------------------------------------------------------------------------
  it("test_wave_execute_action_calls_opencode_api — POSTs to OpenCode /message with action title in content", async () => {
    const mockFetch = vi.fn()
      // 1st call: fetch action details from Wave backend
      .mockResolvedValueOnce(okJson({
        item_id: "it-action-001",
        content: { title: "Deploy to staging", description: "Run deploy pipeline", action_type: "deploy" },
      }))
      // 2nd call: POST to OpenCode /message
      .mockResolvedValueOnce(okJson({ session_id: "oc-sess-abc123" }))
      // 3rd call: progress update (non-critical)
      .mockResolvedValueOnce(okJson({ ok: true }))

    await waveExecuteAction(
      { space_id: "sp-1", action_id: "it-action-001" },
      { ...CONFIG, openCodeUrl: "http://127.0.0.1:4096", _mockFetch: mockFetch },
    )

    // Verify the OpenCode /message call
    const openCodeCall = mockFetch.mock.calls[1]
    expect(openCodeCall[0]).toBe("http://127.0.0.1:4096/message")
    expect(openCodeCall[1]?.method).toBe("POST")
    const body = JSON.parse(openCodeCall[1]?.body as string)
    expect(body.role).toBe("user")
    expect(body.content).toContain("Deploy to staging")
  })

  // -------------------------------------------------------------------------
  // test_wave_execute_action_returns_real_session_id
  // REQ-WEAVE-062: session_id from OpenCode response must be returned
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-062 plan=phase-4.7/task-4.7.execution/step-4.7.1
  // -------------------------------------------------------------------------
  it("test_wave_execute_action_returns_real_session_id — session_id from OpenCode response is returned", async () => {
    const mockFetch = vi.fn()
      .mockResolvedValueOnce(okJson({ item_id: "it-action-002", content: { title: "Run tests" } }))
      .mockResolvedValueOnce(okJson({ session_id: "oc-real-session-xyz" }))
      .mockResolvedValueOnce(okJson({ ok: true }))

    const result = await waveExecuteAction(
      { space_id: "sp-1", action_id: "it-action-002" },
      { ...CONFIG, openCodeUrl: "http://127.0.0.1:4096", _mockFetch: mockFetch },
    )

    // Must return the real session_id from OpenCode, not a generated stub
    expect(result.session_id).toBe("oc-real-session-xyz")
    // Status must be 'running' (real session created)
    expect(result.status).toBe("running")
  })

  // -------------------------------------------------------------------------
  // test_wave_execute_action_handles_opencode_failure
  // REQ: graceful error when OpenCode is not running or returns 500
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-062 plan=phase-4.7/task-4.7.execution/step-4.7.1
  // -------------------------------------------------------------------------
  it("test_wave_execute_action_handles_opencode_failure — throws when OpenCode /message returns 500", async () => {
    const mockFetch = vi.fn()
      .mockResolvedValueOnce(okJson({ item_id: "it-action-003", content: { title: "Failing action" } }))
      // OpenCode returns 500
      .mockResolvedValueOnce({ ok: false, status: 500 } as Response)

    await expect(
      waveExecuteAction(
        { space_id: "sp-1", action_id: "it-action-003" },
        { ...CONFIG, openCodeUrl: "http://127.0.0.1:4096", _mockFetch: mockFetch },
      )
    ).rejects.toThrow("500")
  })

  // -------------------------------------------------------------------------
  // test_wave_execute_action_fetches_action_details
  // REQ: waveExecuteAction fetches action details from the correct URL
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-062 plan=phase-4.7/task-4.7.execution/step-4.7.1
  // -------------------------------------------------------------------------
  it("test_wave_execute_action_fetches_action_details — fetch called with correct items URL", async () => {
    const mockFetch = vi.fn()
      .mockResolvedValueOnce(okJson({ item_id: "it-action-004", content: {} }))
      .mockResolvedValueOnce(okJson({ session_id: "oc-sess-check" }))
      .mockResolvedValueOnce(okJson({ ok: true }))

    await waveExecuteAction(
      { space_id: "sp-exec", action_id: "it-action-004" },
      { ...CONFIG, openCodeUrl: "http://127.0.0.1:4096", _mockFetch: mockFetch },
    )

    // First fetch must be the action details from the Wave backend
    expect(mockFetch.mock.calls[0][0]).toBe(
      "http://localhost:8100/api/v1/spaces/sp-exec/items/it-action-004"
    )
  })

  it("test_wave_execute_action_throws_on_fetch_error — non-ok action fetch throws", async () => {
    const mockFetch = vi.fn().mockResolvedValue({ ok: false, status: 404 } as Response)
    await expect(
      waveExecuteAction(
        { space_id: "sp-1", action_id: "it-missing" },
        { ...CONFIG, _mockFetch: mockFetch },
      )
    ).rejects.toThrow("404")
  })

  // -------------------------------------------------------------------------
  // test_wave_execute_action_uses_config_opencode_url
  // REQ: OpenCode URL must be configurable via config.openCodeUrl
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-062 plan=phase-4.7/task-4.7.execution/step-4.7.1
  // -------------------------------------------------------------------------
  it("test_wave_execute_action_uses_config_opencode_url — uses config.openCodeUrl when provided", async () => {
    const mockFetch = vi.fn()
      .mockResolvedValueOnce(okJson({ item_id: "it-action-005", content: { title: "Custom URL test" } }))
      .mockResolvedValueOnce(okJson({ session_id: "oc-custom-url-sess" }))
      .mockResolvedValueOnce(okJson({ ok: true }))

    await waveExecuteAction(
      { space_id: "sp-1", action_id: "it-action-005" },
      { ...CONFIG, openCodeUrl: "http://custom-opencode:9999", _mockFetch: mockFetch },
    )

    // The /message call must use the custom URL
    expect(mockFetch.mock.calls[1][0]).toBe("http://custom-opencode:9999/message")
  })

  // -------------------------------------------------------------------------
  // test_wave_execute_action_progress_failure_non_critical
  // REQ: progress update failure must not abort execution
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-064 plan=phase-4.7/task-4.7.execution/step-4.7.1
  // -------------------------------------------------------------------------
  it("test_wave_execute_action_progress_failure_non_critical — still returns result when progress update fails", async () => {
    const mockFetch = vi.fn()
      .mockResolvedValueOnce(okJson({ item_id: "it-action-006", content: { title: "Progress fail test" } }))
      .mockResolvedValueOnce(okJson({ session_id: "oc-sess-progress-fail" }))
      // Progress update returns 500 — must not throw
      .mockResolvedValueOnce({ ok: false, status: 500 } as Response)

    const result = await waveExecuteAction(
      { space_id: "sp-1", action_id: "it-action-006" },
      { ...CONFIG, openCodeUrl: "http://127.0.0.1:4096", _mockFetch: mockFetch },
    )

    // Must still return successfully despite progress update failure
    expect(result.session_id).toBe("oc-sess-progress-fail")
    expect(result.status).toBe("running")
  })

  // -------------------------------------------------------------------------
  // test_wave_execute_action_posts_initial_progress
  // REQ-WEAVE-064: initial progress POST is called immediately after session creation
  // Verifies step_name="Session started", status="running", correct URL shape
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-064 plan=phase-4.7/task-4.7.execution/step-4.7.2
  // -------------------------------------------------------------------------
  it("test_wave_execute_action_posts_initial_progress — initial progress POST sent with status=running after session creation", async () => {
    const mockFetch = vi.fn()
      // 1st: fetch action details
      .mockResolvedValueOnce(okJson({ item_id: "it-action-007", content: { title: "Initial progress test" } }))
      // 2nd: POST to OpenCode /message
      .mockResolvedValueOnce(okJson({ session_id: "oc-sess-init-progress" }))
      // 3rd: initial progress POST
      .mockResolvedValueOnce(okJson({ ok: true }))

    const result = await waveExecuteAction(
      { space_id: "sp-progress", action_id: "it-action-007" },
      { ...CONFIG, openCodeUrl: "http://127.0.0.1:4096", _mockFetch: mockFetch },
    )

    // Function must return successfully
    expect(result.session_id).toBe("oc-sess-init-progress")
    expect(result.status).toBe("running")

    // 3rd call must be the initial progress POST to the correct URL
    const progressCall = mockFetch.mock.calls[2]
    expect(progressCall[0]).toBe(
      "http://localhost:8100/api/v1/spaces/sp-progress/sessions/oc-sess-init-progress/progress"
    )
    expect(progressCall[1]?.method).toBe("POST")
    const progressBody = JSON.parse(progressCall[1]?.body as string)
    expect(progressBody.step_name).toBe("Session started")
    expect(progressBody.status).toBe("running")
    expect(progressBody.output_preview).toContain("oc-sess-init-progress")
  })

  // -------------------------------------------------------------------------
  // test_wave_execute_action_polls_for_completion
  // REQ-WEAVE-064: polling mechanism checks session status and posts progress updates;
  // stops when terminal state (completed) is reached.
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-064 plan=phase-4.7/task-4.7.execution/step-4.7.2
  // -------------------------------------------------------------------------
  it("test_wave_execute_action_polls_for_completion — polls OpenCode status and posts progress; stops on completed", async () => {
    vi.useFakeTimers()

    const mockFetch = vi.fn()
      // 1st: fetch action details
      .mockResolvedValueOnce(okJson({ item_id: "it-action-008", content: { title: "Polling test" } }))
      // 2nd: POST to OpenCode /message → returns session_id
      .mockResolvedValueOnce(okJson({ session_id: "oc-sess-poll-test" }))
      // 3rd: initial progress POST (status=running)
      .mockResolvedValueOnce(okJson({ ok: true }))
      // 4th: poll tick 1 — GET /session/:id → still running
      .mockResolvedValueOnce(okJson({ status: "running", current_step: "Analyzing", output_preview: "..." }))
      // 5th: progress POST for tick 1
      .mockResolvedValueOnce(okJson({ ok: true }))
      // 6th: poll tick 2 — GET /session/:id → completed
      .mockResolvedValueOnce(okJson({ status: "completed", current_step: "Done", output_preview: "All done" }))
      // 7th: progress POST for tick 2 (status=completed)
      .mockResolvedValueOnce(okJson({ ok: true }))

    const result = await waveExecuteAction(
      { space_id: "sp-poll", action_id: "it-action-008" },
      { ...CONFIG, openCodeUrl: "http://127.0.0.1:4096", _mockFetch: mockFetch },
    )

    expect(result.session_id).toBe("oc-sess-poll-test")
    expect(result.status).toBe("running")

    // Advance 2s → first poll tick fires
    vi.advanceTimersByTime(2000)
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()

    // After tick 1: status endpoint called + progress POST
    const tick1StatusCall = mockFetch.mock.calls[3]
    expect(tick1StatusCall[0]).toBe("http://127.0.0.1:4096/session/oc-sess-poll-test")
    const tick1ProgressCall = mockFetch.mock.calls[4]
    expect(tick1ProgressCall[0]).toBe(
      "http://localhost:8100/api/v1/spaces/sp-poll/sessions/oc-sess-poll-test/progress"
    )
    const tick1Body = JSON.parse(tick1ProgressCall[1]?.body as string)
    expect(tick1Body.status).toBe("running")
    expect(tick1Body.step_name).toBe("Analyzing")

    // Advance another 2s → second poll tick fires (completed)
    vi.advanceTimersByTime(2000)
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()

    const tick2StatusCall = mockFetch.mock.calls[5]
    expect(tick2StatusCall[0]).toBe("http://127.0.0.1:4096/session/oc-sess-poll-test")
    const tick2ProgressCall = mockFetch.mock.calls[6]
    const tick2Body = JSON.parse(tick2ProgressCall[1]?.body as string)
    expect(tick2Body.status).toBe("completed")
    expect(tick2Body.step_name).toBe("Done")
    expect(tick2Body.output_preview).toBe("All done")

    // Advance another 2s → polling should have stopped (no more calls)
    vi.advanceTimersByTime(2000)
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()

    // Total calls: 3 (setup) + 2 (tick1) + 2 (tick2) = 7; no tick3
    expect(mockFetch).toHaveBeenCalledTimes(7)

    vi.useRealTimers()
  })
})

// ---------------------------------------------------------------------------
// Tool registration tests
// axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-064 plan=phase-3.7/task-3.7.spec-align/step-1
// ---------------------------------------------------------------------------

describe("Wave tool registration", () => {
  // -------------------------------------------------------------------------
  // test_wave_execute_action_tool_registered
  // REQ: codeops_wave_execute_action must be in the WAVE_TOOL_NAMES registry
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-064 plan=phase-3.7/task-3.7.spec-align/step-1
  // -------------------------------------------------------------------------
  it("test_wave_execute_action_tool_registered — codeops_wave_execute_action is in WAVE_TOOL_NAMES", () => {
    expect(WAVE_TOOL_NAMES).toContain("codeops_wave_execute_action")
  })

  it("WAVE_TOOL_NAMES contains all 6 expected Wave tools", () => {
    const expected = [
      "codeops_wave_read",
      "codeops_wave_write",
      "codeops_wave_propose_action",
      "codeops_wave_automation",
      "codeops_wave_comments",
      "codeops_wave_execute_action",
    ]
    for (const name of expected) {
      expect(WAVE_TOOL_NAMES).toContain(name)
    }
    expect(WAVE_TOOL_NAMES).toHaveLength(6)
  })

  // -------------------------------------------------------------------------
  // test_wave_tool_names_includes_execute_action
  // REQ: WAVE_TOOL_NAMES must include codeops_wave_execute_action
  // axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md plan=phase-4.7/task-4.7.execution/step-4.7.3
  // -------------------------------------------------------------------------
  it("test_wave_tool_names_includes_execute_action — WAVE_TOOL_NAMES includes codeops_wave_execute_action", () => {
    expect(WAVE_TOOL_NAMES).toContain("codeops_wave_execute_action")
    expect(WAVE_TOOL_NAMES).toHaveLength(6)
  })

  // -------------------------------------------------------------------------
  // test_wave_execute_action_is_not_stub
  // REQ: waveExecuteAction must make real API calls, not just return a fake session_id
  // axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md plan=phase-4.7/task-4.7.execution/step-4.7.3
  // -------------------------------------------------------------------------
  it("test_wave_execute_action_is_not_stub — makes real API calls to OpenCode /message", async () => {
    const mockFetch = vi.fn()
      .mockResolvedValueOnce(okJson({ item_id: "it-action-real", content: { title: "Real action" } }))
      .mockResolvedValueOnce(okJson({ session_id: "oc-real-session-123" }))
      .mockResolvedValueOnce(okJson({ ok: true }))

    const result = await waveExecuteAction(
      { space_id: "sp-real", action_id: "it-action-real" },
      { ...CONFIG, openCodeUrl: "http://127.0.0.1:4096", _mockFetch: mockFetch },
    )

    // Must have called OpenCode /message endpoint (not just returned a fake ID)
    const openCodeCall = mockFetch.mock.calls.find(
      (call) => call[0] === "http://127.0.0.1:4096/message"
    )
    expect(openCodeCall).toBeDefined()
    expect(openCodeCall?.[1]?.method).toBe("POST")

    // Must return the real session_id from OpenCode response
    expect(result.session_id).toBe("oc-real-session-123")
    expect(result.status).toBe("running")
  })
})