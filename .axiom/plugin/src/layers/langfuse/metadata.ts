/**
 * Axiom Metadata Span Processor — injects Axiom-specific attributes into every OTEL span.
 *
 * REQ-LF-003: Inject Axiom-specific attributes into every span before it reaches
 * the LangfuseSpanProcessor.
 *
 * Attributes injected:
 * - axiom.work_item_id: from AXIOM_WORK_ITEM env var
 * - axiom.session_identity: from AXIOM_SESSION_IDENTITY env var
 * - axiom.user_email: from AXIOM_USER_EMAIL env var
 * - axiom.repo_root: from AXIOM_REPO_ROOT env var
 * - axiom.maturity_level: from AXIOM_MATURITY_LEVEL env var
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-LF-003
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-LF-003 plan=phase-3/task-3-3/step-3-3-1 jira_ref=DEX-403
 */

import { SpanProcessor, ReadableSpan, Span } from "@opentelemetry/sdk-trace-base"
import type { Context } from "@opentelemetry/api"

// ── Types ───────────────────────────────────────────────────────────────────

/** Axiom metadata injected into every span. */
export interface CodeOpsMetadata {
  work_item_id?: string
  session_identity?: string
  user_email?: string
  repo_root?: string
  maturity_level?: number
}

// ── Constants ───────────────────────────────────────────────────────────────

const AXIOM_ATTR_PREFIX = "axiom."

// ── Metadata Resolution ───────────────────────────────────────────────────

/**
 * Resolve Axiom metadata from environment variables.
 *
 * @param repoRoot - Optional repo root (can be used for additional resolution if needed)
 * @returns CodeOpsMetadata object with values from env vars (undefined for missing vars)
 */
export function resolveCodeOpsMetadata(repoRoot?: string): CodeOpsMetadata {
  const metadata: CodeOpsMetadata = {}

  // Work item ID
  if (process.env.AXIOM_WORK_ITEM) {
    metadata.work_item_id = process.env.AXIOM_WORK_ITEM
  }

  // Session identity (e.g., "github.com/dexdat/Axiom")
  if (process.env.AXIOM_SESSION_IDENTITY) {
    metadata.session_identity = process.env.AXIOM_SESSION_IDENTITY
  }

  // User email
  if (process.env.AXIOM_USER_EMAIL) {
    metadata.user_email = process.env.AXIOM_USER_EMAIL
  }

  // Repo root
  if (process.env.AXIOM_REPO_ROOT) {
    metadata.repo_root = process.env.AXIOM_REPO_ROOT
  } else if (repoRoot) {
    // Fallback to provided repoRoot if env var not set
    metadata.repo_root = repoRoot
  }

  // Maturity level (numeric)
  if (process.env.AXIOM_MATURITY_LEVEL) {
    const level = parseInt(process.env.AXIOM_MATURITY_LEVEL, 10)
    if (!isNaN(level)) {
      metadata.maturity_level = level
    }
  }

  return metadata
}

// ── Span Processor ─────────────────────────────────────────────────────────

/**
 * Span processor that wraps a delegate SpanProcessor and injects
 * Axiom metadata attributes into every span before passing through.
 *
 * Implements the SpanProcessor interface from @opentelemetry/sdk-trace-base.
 */
export class CodeOpsMetadataProcessor implements SpanProcessor {
  private readonly delegate: SpanProcessor
  private readonly metadata: CodeOpsMetadata

  /**
   * Create a new CodeOpsMetadataProcessor.
   *
   * @param delegate - The underlying SpanProcessor to delegate to (e.g., LangfuseSpanProcessor)
   * @param metadata - Axiom metadata to inject into spans
   */
  constructor(delegate: SpanProcessor, metadata: CodeOpsMetadata) {
    this.delegate = delegate
    this.metadata = metadata
  }

  /**
   * Called when a span starts. Injects Axiom metadata attributes into the span,
   * then delegates to the wrapped processor.
   */
  onStart(span: Span, parentContext: Context): void {
    // Inject all defined metadata attributes (omit-not-null pattern)
    if (this.metadata.work_item_id !== undefined) {
      span.setAttribute(`${AXIOM_ATTR_PREFIX}work_item_id`, this.metadata.work_item_id)
    }

    if (this.metadata.session_identity !== undefined) {
      span.setAttribute(`${AXIOM_ATTR_PREFIX}session_identity`, this.metadata.session_identity)
    }

    if (this.metadata.user_email !== undefined) {
      span.setAttribute(`${AXIOM_ATTR_PREFIX}user_email`, this.metadata.user_email)
    }

    if (this.metadata.repo_root !== undefined) {
      span.setAttribute(`${AXIOM_ATTR_PREFIX}repo_root`, this.metadata.repo_root)
    }

    if (this.metadata.maturity_level !== undefined) {
      span.setAttribute(`${AXIOM_ATTR_PREFIX}maturity_level`, this.metadata.maturity_level)
    }

    // Delegate to wrapped processor
    this.delegate.onStart(span, parentContext)
  }

  /**
   * Called when a span ends. Delegates to the wrapped processor.
   */
  onEnd(span: ReadableSpan): void {
    this.delegate.onEnd(span)
  }

  /**
   * Shutdown the processor. Delegates to the wrapped processor.
   */
  shutdown(): Promise<void> {
    return this.delegate.shutdown()
  }

  /**
   * Force flush pending spans. Delegates to the wrapped processor.
   */
  forceFlush(): Promise<void> {
    return this.delegate.forceFlush()
  }
}

// ── Factory ─────────────────────────────────────────────────────────────────

/**
 * Create a CodeOpsMetadataProcessor wrapping the given delegate.
 *
 * @param delegate - The underlying SpanProcessor to wrap
 * @param metadata - Axiom metadata to inject into spans
 * @returns CodeOpsMetadataProcessor instance
 */
export function createCodeOpsMetadataProcessor(
  delegate: SpanProcessor,
  metadata: CodeOpsMetadata,
): CodeOpsMetadataProcessor {
  return new CodeOpsMetadataProcessor(delegate, metadata)
}