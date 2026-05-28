/**
 * Tests for Wave /axiom-privacy command handler.
 *
 * REQ-WEAVE-305
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-305 plan=phase-5.0/task-5.0.privacy-command/step-5.0.1
 */

import { describe, expect, it } from "bun:test"
import { formatPrivacyMessage, handlePrivacyCommand } from "./wave-privacy"

describe("wave-privacy: handlePrivacyCommand", () => {
  it("test_privacy_returns_string — handlePrivacyCommand() returns a non-empty string", () => {
    const result = handlePrivacyCommand()
    expect(typeof result).toBe("string")
    expect(result.length).toBeGreaterThan(0)
  })

  it("test_privacy_mentions_spaces — output mentions wave_spaces", () => {
    const result = handlePrivacyCommand()
    expect(result).toContain("wave_spaces")
  })

  it("test_privacy_mentions_delete — output mentions DELETE", () => {
    const result = handlePrivacyCommand()
    expect(result).toContain("DELETE")
  })

  it("test_privacy_mentions_encryption — output mentions AES-256-GCM", () => {
    const result = handlePrivacyCommand()
    expect(result).toContain("AES-256-GCM")
  })

  it("test_privacy_format_message — formatPrivacyMessage() returns markdown with ## heading", () => {
    const result = formatPrivacyMessage()
    expect(result).toContain("## Wave Privacy Information")
  })
})
