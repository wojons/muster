// Tests for the shared MODEL_DISPLAY_ALIASES module
// axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-012 plan=step-verify-H004

import { describe, it, expect } from "bun:test"
import { MODEL_DISPLAY_ALIASES, resolveModelDisplayName } from "../src/dashboard/model-aliases"

describe("MODEL_DISPLAY_ALIASES (shared module)", () => {
  it("has entries for all major providers", () => {
    // Anthropic (Bedrock + direct), OpenAI, Moonshot, Minimax, ZhipuAI, Google, OpenCode
    expect(Object.keys(MODEL_DISPLAY_ALIASES).length).toBeGreaterThanOrEqual(20)
  })

  it("resolves Anthropic Bedrock-prefixed model IDs", () => {
    expect(resolveModelDisplayName("anthropic.claude-opus-4-6-v1")).toBe("Claude Opus 4.6")
    expect(resolveModelDisplayName("anthropic.claude-sonnet-4-6")).toBe("Claude Sonnet 4.6")
  })

  it("resolves Anthropic direct API model IDs", () => {
    expect(resolveModelDisplayName("claude-3-5-sonnet-20241022")).toBe("Claude Sonnet 3.5")
    expect(resolveModelDisplayName("claude-sonnet-4-20250514")).toBe("Claude Sonnet 4")
    expect(resolveModelDisplayName("claude-haiku-4-20250414")).toBe("Claude Haiku 4")
  })

  it("resolves Anthropic Bedrock full ARN-style IDs", () => {
    expect(resolveModelDisplayName("amazon-bedrock/anthropic.claude-sonnet-4-6")).toBe("Claude Sonnet 4.6")
    expect(resolveModelDisplayName("amazon-bedrock/anthropic.claude-opus-4-6-v1")).toBe("Claude Opus 4.6")
  })

  it("resolves OpenAI models", () => {
    expect(resolveModelDisplayName("gpt-5.4")).toBe("GPT-5.4")
    expect(resolveModelDisplayName("gpt-5.4-mini")).toBe("GPT-5.4 Mini")
  })

  it("resolves Moonshot/Kimi models", () => {
    expect(resolveModelDisplayName("moonshotai.kimi-k2.5")).toBe("Kimi K2.5")
  })

  it("resolves Minimax models", () => {
    expect(resolveModelDisplayName("minimax.minimax-m2.5")).toBe("Minimax M2.5")
  })

  it("resolves ZhipuAI/GLM models", () => {
    expect(resolveModelDisplayName("zai.glm-5")).toBe("GLM-5")
  })

  it("resolves OpenCode built-in free models", () => {
    expect(resolveModelDisplayName("opencode/big-pickle")).toBe("Big Pickle")
    expect(resolveModelDisplayName("opencode/glm-5-free")).toBe("GLM-5 Free")
  })

  it("returns full raw ID for unknown models (no truncation)", () => {
    expect(resolveModelDisplayName("some-unknown-model-xyz-v99")).toBe("some-unknown-model-xyz-v99")
  })

  it("returns short unknown models unchanged", () => {
    expect(resolveModelDisplayName("gpt-4")).toBe("gpt-4")
  })

  it("returns empty string for empty input", () => {
    expect(resolveModelDisplayName("")).toBe("")
  })
})
