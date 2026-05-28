/**
 * Tests for Axiom Metadata Span Processor.
 *
 * REQ-LF-003: Inject Axiom-specific attributes into every span
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-LF-003
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-LF-003 plan=phase-3/task-3-3/step-3-3-1 jira_ref=DEX-403
 */

import { describe, it, expect, afterEach, vi } from "bun:test"
import {
  resolveCodeOpsMetadata,
  CodeOpsMetadataProcessor,
  createCodeOpsMetadataProcessor,
  type CodeOpsMetadata,
} from "../src/layers/langfuse/metadata"
import { SpanProcessor } from "@opentelemetry/sdk-trace-base"
import { Span, SpanContext, Context } from "@opentelemetry/api"

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeMockSpan(): Span {
  const attributes: Record<string, unknown> = {}
  return {
    setAttribute: (key: string, value: unknown) => {
      attributes[key] = value
    },
    setAttributes: (attrs: Record<string, unknown>) => {
      Object.assign(attributes, attrs)
    },
    getAttribute: (key: string) => attributes[key],
    addEvent: () => {},
    addException: () => {},
    updateName: () => {},
    setStatus: () => {},
    end: () => {},
    spanContext: () => ({ traceId: "abc123", spanId: "def456", traceFlags: 0 } as SpanContext),
  } as unknown as Span
}

/**
 * createMockSpan returns a mock Span with a publicly accessible `attributes`
 * map so tests can assert on the exact values written via setAttribute().
 *
 * Use this helper (instead of makeMockSpan) whenever a test needs to verify
 * that specific attribute values were set on the span.
 */
function createMockSpan(): Span & { attributes: Record<string, unknown> } {
  const attributes: Record<string, unknown> = {}
  const span = {
    attributes,
    setAttribute: (key: string, value: unknown) => {
      attributes[key] = value
    },
    setAttributes: (attrs: Record<string, unknown>) => {
      Object.assign(attributes, attrs)
    },
    getAttribute: (key: string) => attributes[key],
    addEvent: () => {},
    addException: () => {},
    updateName: () => {},
    setStatus: () => {},
    end: () => {},
    spanContext: () => ({ traceId: "abc123", spanId: "def456", traceFlags: 0 } as SpanContext),
  }
  return span as unknown as Span & { attributes: Record<string, unknown> }
}

function makeMockDelegate(): SpanProcessor {
  return {
    onStart: vi.fn(),
    onEnd: vi.fn(),
    shutdown: vi.fn().mockResolvedValue(undefined),
    forceFlush: vi.fn().mockResolvedValue(undefined),
  }
}

// ---------------------------------------------------------------------------
// resolveCodeOpsMetadata
// ---------------------------------------------------------------------------

describe("resolveCodeOpsMetadata", () => {
  const savedEnv: Record<string, string | undefined> = {}

  afterEach(() => {
    // Restore env vars
    for (const [key, val] of Object.entries(savedEnv)) {
      if (val === undefined) {
        delete process.env[key]
      } else {
        process.env[key] = val
      }
    }
  })

  it("returns empty object when no env vars set", () => {
    // Save and clear relevant env vars
    savedEnv.AXIOM_WORK_ITEM = process.env.AXIOM_WORK_ITEM
    savedEnv.AXIOM_SESSION_IDENTITY = process.env.AXIOM_SESSION_IDENTITY
    savedEnv.AXIOM_USER_EMAIL = process.env.AXIOM_USER_EMAIL
    savedEnv.AXIOM_REPO_ROOT = process.env.AXIOM_REPO_ROOT
    savedEnv.AXIOM_MATURITY_LEVEL = process.env.AXIOM_MATURITY_LEVEL

    delete process.env.AXIOM_WORK_ITEM
    delete process.env.AXIOM_SESSION_IDENTITY
    delete process.env.AXIOM_USER_EMAIL
    delete process.env.AXIOM_REPO_ROOT
    delete process.env.AXIOM_MATURITY_LEVEL

    const result = resolveCodeOpsMetadata()
    expect(result).toEqual({})
  })

  it("reads AXIOM_WORK_ITEM from env var", () => {
    savedEnv.AXIOM_WORK_ITEM = process.env.AXIOM_WORK_ITEM
    process.env.AXIOM_WORK_ITEM = "DEX-387"

    const result = resolveCodeOpsMetadata()
    expect(result.work_item_id).toBe("DEX-387")
  })

  it("reads AXIOM_SESSION_IDENTITY from env var", () => {
    savedEnv.AXIOM_SESSION_IDENTITY = process.env.AXIOM_SESSION_IDENTITY
    process.env.AXIOM_SESSION_IDENTITY = "github.com/dexdat/Axiom"

    const result = resolveCodeOpsMetadata()
    expect(result.session_identity).toBe("github.com/dexdat/Axiom")
  })

  it("reads AXIOM_USER_EMAIL from env var", () => {
    savedEnv.AXIOM_USER_EMAIL = process.env.AXIOM_USER_EMAIL
    process.env.AXIOM_USER_EMAIL = "alice@example.com"

    const result = resolveCodeOpsMetadata()
    expect(result.user_email).toBe("alice@example.com")
  })

  it("reads AXIOM_REPO_ROOT from env var", () => {
    savedEnv.AXIOM_REPO_ROOT = process.env.AXIOM_REPO_ROOT
    process.env.AXIOM_REPO_ROOT = "/Users/alice/code/backend"

    const result = resolveCodeOpsMetadata()
    expect(result.repo_root).toBe("/Users/alice/code/backend")
  })

  it("falls back to repoRoot parameter when AXIOM_REPO_ROOT not set", () => {
    savedEnv.AXIOM_REPO_ROOT = process.env.AXIOM_REPO_ROOT
    delete process.env.AXIOM_REPO_ROOT

    const result = resolveCodeOpsMetadata("/fallback/repo")
    expect(result.repo_root).toBe("/fallback/repo")
  })

  it("prefers env var over repoRoot parameter", () => {
    savedEnv.AXIOM_REPO_ROOT = process.env.AXIOM_REPO_ROOT
    process.env.AXIOM_REPO_ROOT = "/env/repo"
    const fallback = "/fallback/repo"

    const result = resolveCodeOpsMetadata(fallback)
    expect(result.repo_root).toBe("/env/repo")
  })

  it("reads AXIOM_MATURITY_LEVEL from env var as number", () => {
    savedEnv.AXIOM_MATURITY_LEVEL = process.env.AXIOM_MATURITY_LEVEL
    process.env.AXIOM_MATURITY_LEVEL = "3"

    const result = resolveCodeOpsMetadata()
    expect(result.maturity_level).toBe(3)
  })

  it("ignores non-numeric maturity level", () => {
    savedEnv.AXIOM_MATURITY_LEVEL = process.env.AXIOM_MATURITY_LEVEL
    process.env.AXIOM_MATURITY_LEVEL = "not-a-number"

    const result = resolveCodeOpsMetadata()
    expect(result.maturity_level).toBeUndefined()
  })

  it("returns all metadata when all env vars set", () => {
    savedEnv.AXIOM_WORK_ITEM = process.env.AXIOM_WORK_ITEM
    savedEnv.AXIOM_SESSION_IDENTITY = process.env.AXIOM_SESSION_IDENTITY
    savedEnv.AXIOM_USER_EMAIL = process.env.AXIOM_USER_EMAIL
    savedEnv.AXIOM_REPO_ROOT = process.env.AXIOM_REPO_ROOT
    savedEnv.AXIOM_MATURITY_LEVEL = process.env.AXIOM_MATURITY_LEVEL

    process.env.AXIOM_WORK_ITEM = "DEX-387"
    process.env.AXIOM_SESSION_IDENTITY = "github.com/user/repo"
    process.env.AXIOM_USER_EMAIL = "alice@example.com"
    process.env.AXIOM_REPO_ROOT = "/Users/alice/code/repo"
    process.env.AXIOM_MATURITY_LEVEL = "5"

    const result = resolveCodeOpsMetadata()
    expect(result).toEqual({
      work_item_id: "DEX-387",
      session_identity: "github.com/user/repo",
      user_email: "alice@example.com",
      repo_root: "/Users/alice/code/repo",
      maturity_level: 5,
    })
  })
})

// ---------------------------------------------------------------------------
// CodeOpsMetadataProcessor
// ---------------------------------------------------------------------------

describe("CodeOpsMetadataProcessor", () => {
  it("onStart sets all 5 attributes on the span", () => {
    const delegate = makeMockDelegate()
    const metadata: CodeOpsMetadata = {
      work_item_id: "DEX-387",
      session_identity: "github.com/user/repo",
      user_email: "alice@example.com",
      repo_root: "/Users/alice/code/repo",
      maturity_level: 3,
    }

    const processor = new CodeOpsMetadataProcessor(delegate, metadata)
    const span = createMockSpan()

    processor.onStart(span, undefined as unknown as Context)

    // Verify delegate was called
    expect(delegate.onStart).toHaveBeenCalledWith(span, undefined)
    expect(delegate.onStart).toHaveBeenCalledTimes(1)

    // Verify the exact attribute VALUES were written to the span
    expect(span.attributes["axiom.work_item_id"]).toBe(metadata.work_item_id)
    expect(span.attributes["axiom.session_identity"]).toBe(metadata.session_identity)
    expect(span.attributes["axiom.user_email"]).toBe(metadata.user_email)
    expect(span.attributes["axiom.repo_root"]).toBe(metadata.repo_root)
    expect(span.attributes["axiom.maturity_level"]).toBe(metadata.maturity_level)
  })

  it("onStart sets only defined attributes (omit-not-null)", () => {
    const delegate = makeMockDelegate()
    const metadata: CodeOpsMetadata = {
      work_item_id: "DEX-387",
      // session_identity, user_email, repo_root, maturity_level are undefined
    }

    const processor = new CodeOpsMetadataProcessor(delegate, metadata)
    const span = createMockSpan()

    processor.onStart(span, undefined as unknown as Context)

    expect(delegate.onStart).toHaveBeenCalledTimes(1)

    // Defined attribute must be present with correct value
    expect(span.attributes["axiom.work_item_id"]).toBe("DEX-387")

    // Undefined metadata fields must NOT produce span attributes (omit-not-null)
    expect(span.attributes["axiom.session_identity"]).toBeUndefined()
    expect(span.attributes["axiom.user_email"]).toBeUndefined()
    expect(span.attributes["axiom.repo_root"]).toBeUndefined()
    expect(span.attributes["axiom.maturity_level"]).toBeUndefined()
  })

  it("onEnd delegates to wrapped processor", () => {
    const delegate = makeMockDelegate()
    const metadata: CodeOpsMetadata = { work_item_id: "DEX-387" }

    const processor = new CodeOpsMetadataProcessor(delegate, metadata)
    const span = makeMockSpan()

    processor.onEnd(span)

    expect(delegate.onEnd).toHaveBeenCalledWith(span)
    expect(delegate.onEnd).toHaveBeenCalledTimes(1)
  })

  it("shutdown delegates to wrapped processor", async () => {
    const delegate = makeMockDelegate()
    const metadata: CodeOpsMetadata = { work_item_id: "DEX-387" }

    const processor = new CodeOpsMetadataProcessor(delegate, metadata)

    await processor.shutdown()

    expect(delegate.shutdown).toHaveBeenCalledTimes(1)
  })

  it("forceFlush delegates to wrapped processor", async () => {
    const delegate = makeMockDelegate()
    const metadata: CodeOpsMetadata = { work_item_id: "DEX-387" }

    const processor = new CodeOpsMetadataProcessor(delegate, metadata)

    await processor.forceFlush()

    expect(delegate.forceFlush).toHaveBeenCalledTimes(1)
  })
})

// ---------------------------------------------------------------------------
// createCodeOpsMetadataProcessor factory
// ---------------------------------------------------------------------------

describe("createCodeOpsMetadataProcessor", () => {
  it("creates a CodeOpsMetadataProcessor instance", () => {
    const delegate = makeMockDelegate()
    const metadata: CodeOpsMetadata = { work_item_id: "DEX-387" }

    const processor = createCodeOpsMetadataProcessor(delegate, metadata)

    expect(processor).toBeInstanceOf(CodeOpsMetadataProcessor)
  })

  it("works with empty metadata", () => {
    const delegate = makeMockDelegate()
    const metadata: CodeOpsMetadata = {}

    const processor = createCodeOpsMetadataProcessor(delegate, metadata)
    const span = makeMockSpan()

    // Should not throw
    processor.onStart(span, undefined)
    processor.onEnd(span)

    expect(delegate.onStart).toHaveBeenCalledTimes(1)
    expect(delegate.onEnd).toHaveBeenCalledTimes(1)
  })

  it("does NOT set attributes for undefined metadata fields", () => {
    const processor = createCodeOpsMetadataProcessor(makeMockDelegate(), {
      work_item_id: "DEX-392",
      // session_identity, user_email, repo_root, maturity_level are undefined
    })
    const span = createMockSpan()
    processor.onStart(span, undefined as unknown as Context)

    expect(span.attributes["axiom.work_item_id"]).toBe("DEX-392")
    expect(span.attributes["axiom.session_identity"]).toBeUndefined()
    expect(span.attributes["axiom.user_email"]).toBeUndefined()
    expect(span.attributes["axiom.repo_root"]).toBeUndefined()
    expect(span.attributes["axiom.maturity_level"]).toBeUndefined()
  })
})
