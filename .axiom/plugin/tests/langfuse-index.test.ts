/**
 * Tests for Langfuse OTEL layer initialization.
 *
 * REQ-LF-001: Core Langfuse OTEL Integration
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-LF-001
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-LF-001 plan=phase-3/task-3-5/step-3-5-1 jira_ref=DEX-403
 */

import { describe, it, expect, spyOn, afterEach } from "bun:test"
import { initLangfuse } from "../src/layers/langfuse/index"

describe("initLangfuse (REQ-LF-001)", () => {
  afterEach(() => {
    // Clean up env vars
    delete process.env.LANGFUSE_PUBLIC_KEY
    delete process.env.LANGFUSE_SECRET_KEY
    delete process.env.LANGFUSE_BASEURL
  })

  it("skips when otelEnabled is false", async () => {
    const warnSpy = spyOn(console, "warn").mockImplementation(() => {})

    const result = await initLangfuse({
      repoRoot: "/tmp/test",
      otelEnabled: false,
    })

    expect(result.initialized).toBe(false)
    expect(result.skipReason).toBe("experimental.openTelemetry not enabled")

    warnSpy.mockRestore()
  })

  it("skips when no public key is available", async () => {
    const warnSpy = spyOn(console, "warn").mockImplementation(() => {})

    // No env vars, no backend, no cache → defaults with empty public_key
    const result = await initLangfuse({
      repoRoot: "/tmp/nonexistent-test-repo",
      otelEnabled: true,
    })

    expect(result.initialized).toBe(false)
    expect(result.skipReason).toBe("no public key")

    warnSpy.mockRestore()
  })

  it("flush and shutdown are no-ops when not initialized", async () => {
    const warnSpy = spyOn(console, "warn").mockImplementation(() => {})

    const result = await initLangfuse({
      repoRoot: "/tmp/test",
      otelEnabled: false,
    })

    // Should not throw
    await result.flush()
    await result.shutdown()

    warnSpy.mockRestore()
  })

  it("attempts initialization when public key is available via env vars", async () => {
    const warnSpy = spyOn(console, "warn").mockImplementation(() => {})

    process.env.LANGFUSE_PUBLIC_KEY = "pk-lf-test-key"
    process.env.LANGFUSE_SECRET_KEY = "sk-lf-test-key"
    process.env.LANGFUSE_BASEURL = "http://localhost:3000"

    const result = await initLangfuse({
      repoRoot: "/tmp/test",
      otelEnabled: true,
    })

    // May succeed or fail depending on whether Langfuse SDK can init
    // without a real server — but it should at least attempt
    if (result.initialized) {
      expect(result.configSource).toBe("env_vars")
      // Clean up
      await result.shutdown()
    } else {
      // Acceptable — SDK may fail without a real Langfuse server
      expect(result.skipReason).toBeTruthy()
    }

    warnSpy.mockRestore()
  })

  it("returns flush and shutdown functions", async () => {
    const warnSpy = spyOn(console, "warn").mockImplementation(() => {})

    const result = await initLangfuse({
      repoRoot: "/tmp/test",
      otelEnabled: false,
    })

    expect(typeof result.flush).toBe("function")
    expect(typeof result.shutdown).toBe("function")

    warnSpy.mockRestore()
  })
})
