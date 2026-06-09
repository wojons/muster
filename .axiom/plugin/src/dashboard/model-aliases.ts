/**
 * Canonical model display alias map for the TUI Dashboard.
 *
 * Single source of truth for model ID → display name mapping.
 * All dashboard data sources and the renderer import from here.
 *
 * When a new model is released, update ONLY this file.
 *
 * REQ-TUI-012: Model names MUST use display aliases for known models.
 * Unknown models show the full raw ID (no truncation).
 *
 * axiom:trace work_item=tui-dashboard-01 spec=specs/89-TUI-Dashboard-Plugin.md#REQ-TUI-012 plan=step-verify-H004
 */

export const MODEL_DISPLAY_ALIASES: Record<string, string> = {
  // Anthropic — via Bedrock (provider-prefixed IDs from OpenCode)
  "anthropic.claude-opus-4-6-v1": "Claude Opus 4.6",
  "anthropic.claude-sonnet-4-6": "Claude Sonnet 4.6",
  "anthropic.claude-sonnet-4-5-v2": "Claude Sonnet 4.5v2",
  "anthropic.claude-haiku-4-5-v1": "Claude Haiku 4.5",
  // Anthropic — direct API model IDs
  "claude-opus-4-6-20250514": "Claude Opus 4.6",
  "claude-sonnet-4-6-20250514": "Claude Sonnet 4.6",
  "claude-sonnet-4-20250514": "Claude Sonnet 4",
  "claude-3-5-sonnet-20241022": "Claude Sonnet 3.5",
  "claude-3-5-haiku-20241022": "Claude Haiku 3.5",
  "claude-haiku-4-20250414": "Claude Haiku 4",
  // Anthropic — Bedrock full ARN-style IDs
  "amazon-bedrock/anthropic.claude-sonnet-4-6": "Claude Sonnet 4.6",
  "amazon-bedrock/anthropic.claude-opus-4-6-v1": "Claude Opus 4.6",
  // OpenAI
  "gpt-5.4": "GPT-5.4",
  "gpt-5.4-mini": "GPT-5.4 Mini",
  "gpt-4.1": "GPT-4.1",
  "gpt-4.1-mini": "GPT-4.1 Mini",
  "o3": "o3",
  "o4-mini": "o4 Mini",
  // Moonshot (Kimi)
  "moonshotai.kimi-k2.5": "Kimi K2.5",
  "kimi-k2.5": "Kimi K2.5",
  // Minimax
  "minimax.minimax-m2.5": "Minimax M2.5",
  "minimax-m2.5": "Minimax M2.5",
  // ZhipuAI (GLM)
  "zai.glm-5": "GLM-5",
  "glm-5": "GLM-5",
  // Google
  "gemini-2.5-pro": "Gemini 2.5 Pro",
  "gemini-2.5-flash": "Gemini 2.5 Flash",
  // OpenCode built-in free models
  "opencode/big-pickle": "Big Pickle",
  "opencode/glm-5-free": "GLM-5 Free",
  "opencode/gpt-5-nano": "GPT-5 Nano",
  "opencode/minimax-m2.5-free": "Minimax M2.5 Free",
  "opencode/trinity-large-preview-free": "Trinity Large",
}

/**
 * Resolve a raw model ID to its display name.
 * Falls back to the full raw ID (no truncation) for unknown models.
 */
export function resolveModelDisplayName(rawModelId: string): string {
  if (!rawModelId) return rawModelId
  const alias = MODEL_DISPLAY_ALIASES[rawModelId]
  if (alias) return alias
  // No truncation — show the full model ID so users can identify it
  return rawModelId
}
