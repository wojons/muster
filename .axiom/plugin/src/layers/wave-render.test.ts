/**
 * Tests for /axiom-render command handler.
 *
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-2.8/task-2.8.2/step-2.8.2 test=.axiom/plugin/src/layers/wave-render.test.ts
 */

import { beforeEach, describe, expect, it, vi } from "vitest"
import { handleRenderCommand, isValidTemplate, ALLOWED_TEMPLATES } from "./wave-render"

// Clear all mocks before each test to prevent vi.fn() state leakage in bun vitest compat.
beforeEach(() => { vi.clearAllMocks() })

const BACKEND = "http://localhost:8100"
const SPACE_ID = "sp-render-1"

function mockFetchSuccess(itemId = "item-render-1") {
  return vi.fn().mockResolvedValue({
    ok: true,
    status: 201,
    json: async () => ({ item_id: itemId }),
  } as unknown as Response)
}

describe("isValidTemplate", () => {
  it("returns true for all allowed templates", () => {
    for (const t of ALLOWED_TEMPLATES) {
      expect(isValidTemplate(t)).toBe(true)
    }
  })

  it("returns false for unknown template names", () => {
    expect(isValidTemplate("unknown")).toBe(false)
    expect(isValidTemplate("../etc/passwd")).toBe(false)
    expect(isValidTemplate("")).toBe(false)
    expect(isValidTemplate("REPORT")).toBe(false)
  })
})

describe("handleRenderCommand", () => {
  // test_render_command_creates_rich_render_item
  it("test_render_command_creates_rich_render_item — /axiom-render report creates item", async () => {
    const mockFetch = mockFetchSuccess("item-report-1")
    const result = await handleRenderCommand("report", undefined, {
      backendUrl: BACKEND,
      spaceId: SPACE_ID,
      _mockFetch: mockFetch,
    })
    expect(result.itemId).toBe("item-report-1")
    expect(result.template).toBe("report")
    expect(result.spaceId).toBe(SPACE_ID)
    // Verify request body
    const body = JSON.parse(mockFetch.mock.calls[0][1].body)
    expect(body.type).toBe("rich_render")
    expect(body.content.template).toBe("report")
  })

  // test_render_command_validates_template_name
  it("test_render_command_validates_template_name — unknown template → error message", async () => {
    await expect(
      handleRenderCommand("unknown-template", undefined, {
        backendUrl: BACKEND,
        spaceId: SPACE_ID,
        _mockFetch: vi.fn(),
      }),
    ).rejects.toThrow('Unknown template "unknown-template"')
  })

  it("test_render_command_validates_template_name — path traversal attempt → error", async () => {
    await expect(
      handleRenderCommand("../etc/passwd", undefined, {
        backendUrl: BACKEND,
        spaceId: SPACE_ID,
        _mockFetch: vi.fn(),
      }),
    ).rejects.toThrow("Unknown template")
  })

  // test_render_command_parses_data_json
  it("test_render_command_parses_data_json — data JSON is parsed and included in item content", async () => {
    const mockFetch = mockFetchSuccess()
    const data = { title: "My Report", metrics: [{ label: "Score", value: 95 }] }
    await handleRenderCommand("report", JSON.stringify(data), {
      backendUrl: BACKEND,
      spaceId: SPACE_ID,
      _mockFetch: mockFetch,
    })
    const body = JSON.parse(mockFetch.mock.calls[0][1].body)
    expect(body.content.data).toEqual(data)
  })

  it("throws when data_json is invalid JSON", async () => {
    await expect(
      handleRenderCommand("report", "{ invalid json }", {
        backendUrl: BACKEND,
        spaceId: SPACE_ID,
        _mockFetch: vi.fn(),
      }),
    ).rejects.toThrow("Invalid JSON")
  })

  it("throws when spaceId is missing", async () => {
    await expect(
      handleRenderCommand("report", undefined, {
        backendUrl: BACKEND,
        _mockFetch: vi.fn(),
      }),
    ).rejects.toThrow("spaceId is required")
  })

  it("throws when item creation fails", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: false, status: 500, statusText: "Server Error",
    } as Response)
    await expect(
      handleRenderCommand("dashboard", undefined, {
        backendUrl: BACKEND,
        spaceId: SPACE_ID,
        _mockFetch: mockFetch,
      }),
    ).rejects.toThrow("Failed to create rich_render item")
  })

  it("sends Authorization header when authToken provided", async () => {
    const mockFetch = mockFetchSuccess()
    await handleRenderCommand("kanban", undefined, {
      backendUrl: BACKEND,
      spaceId: SPACE_ID,
      authToken: "tok-render",
      _mockFetch: mockFetch,
    })
    expect(mockFetch.mock.calls[0][1].headers["Authorization"]).toBe("Bearer tok-render")
  })

  it("all 4 template names are accepted", async () => {
    for (const template of ALLOWED_TEMPLATES) {
      const mockFetch = mockFetchSuccess()
      const result = await handleRenderCommand(template, undefined, {
        backendUrl: BACKEND,
        spaceId: SPACE_ID,
        _mockFetch: mockFetch,
      })
      expect(result.template).toBe(template)
    }
  })

  it("data is undefined when no data_json provided", async () => {
    const mockFetch = mockFetchSuccess()
    await handleRenderCommand("timeline", undefined, {
      backendUrl: BACKEND,
      spaceId: SPACE_ID,
      _mockFetch: mockFetch,
    })
    const body = JSON.parse(mockFetch.mock.calls[0][1].body)
    expect(body.content.data).toBeUndefined()
  })
})
