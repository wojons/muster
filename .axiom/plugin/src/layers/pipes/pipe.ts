/**
 * One-time-read pipe implementation for L8 file-based agent communication (Pillar 2).
 *
 * Provides encrypted ephemeral pipes for sensitive data transfer between agents.
 * Data is written once, read once, then immediately deleted. No trace remains.
 *
 * Two transport modes:
 * - FIFO (primary): Unix named pipe via mkfifo. True streaming semantics.
 * - TempFile (fallback): Regular file with immediate deletion after read.
 *   Used on macOS when FIFO poll/select behavior is unreliable (OD-FAC-7).
 *
 * REQ-FAC-020: UUID-based FIFO path in _tmp/pipes/.
 * REQ-FAC-021: Optional name parameter (metadata only, not in path).
 * REQ-FAC-022: TTL parameter (default 60s, max 300s).
 * REQ-FAC-023: Rate limit: max 10 active pipes per session.
 * REQ-FAC-024: AES-256-GCM encryption by default.
 * REQ-FAC-025: One-time-read semantics — second read returns error.
 * REQ-FAC-026: Write timeout (default 30s).
 * REQ-FAC-027: Read timeout (default 30s).
 * REQ-FAC-028: Immediate deletion after successful read.
 * REQ-FAC-029: Reaper process for TTL-based cleanup.
 * REQ-FAC-095: Pipe contents deleted immediately after read.
 * REQ-FAC-096: TTL-based cleanup by reaper.
 * REQ-FAC-097: Orphan cleanup on session start (pipes > 1 hour old).
 * REQ-FAC-098: Metadata file (.meta) deleted with pipe.
 * REQ-FAC-104–109: Audit logging for pipe operations.
 * REQ-FAC-110: Decrypted output wrapped in untrusted-data boundary.
 * REQ-FAC-111: Metadata includes session_id and creating_tool_name.
 * REQ-FAC-112: Cross-session read fails with clear error.
 *
 * Spec: specs/87-MCP-File-Agent-Communication.md#4
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-020 plan=phase-100/task-100-2/step-100-2-2 jira_ref=DEX-458
 */

import {
	createCipheriv,
	createDecipheriv,
	randomUUID,
} from "node:crypto";
import {
	appendFileSync,
	existsSync,
	mkdirSync,
	readdirSync,
	readFileSync,
	renameSync,
	statSync,
	unlinkSync,
	writeFileSync,
} from "node:fs";
import { readFile, writeFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import { execSync } from "node:child_process";
import {
	type ArtifactCipher,
	type SessionCipher,
	GCM_NONCE_BYTES,
	GCM_TAG_BYTES,
	deriveArtifactCipher,
	generateNonce,
} from "../file-delivery/cipher";
import { wrapUntrustedData } from "../file-delivery/file-ref";

// ── Constants ────────────────────────────────────────────────────────────

/** Pipe directory relative to workspace root. REQ-FAC-020 */
export const PIPE_DIR = "_tmp/pipes";

/** Default TTL in seconds. REQ-FAC-022 */
export const DEFAULT_PIPE_TTL_SECONDS = 60;

/** Maximum TTL in seconds. REQ-FAC-022 */
export const MAX_PIPE_TTL_SECONDS = 300;

/** Maximum active pipes per session. REQ-FAC-023 */
export const MAX_ACTIVE_PIPES = 10;

/** Default write/read timeout in seconds. REQ-FAC-026/027 */
export const DEFAULT_PIPE_TIMEOUT_SECONDS = 30;

/** Orphan age threshold: pipes older than 1 hour are cleaned up. REQ-FAC-097 */
const ORPHAN_AGE_MS = 60 * 60 * 1000;

/** Magic bytes for encrypted pipe files. */
const PIPE_MAGIC = Buffer.from("CODEPIPE", "utf8"); // 8 bytes

// ── Types ────────────────────────────────────────────────────────────────

/** Transport mode for a pipe. */
export type PipeTransport = "fifo" | "tempfile";

/**
 * Pipe descriptor returned to the caller after creation.
 * REQ-FAC-020, REQ-FAC-021
 */
export interface PipeDescriptor {
	/** UUID-based pipe identifier. REQ-FAC-020 */
	pipe_id: string;
	/** Workspace-relative path to the FIFO or temp file. */
	pipe_path: string;
	/** Workspace-relative path to the metadata file. REQ-FAC-098 */
	meta_path: string;
	/** Transport mode in use. */
	transport: PipeTransport;
	/** TTL in seconds. REQ-FAC-022 */
	ttl_seconds: number;
	/** Creation timestamp (Unix ms). */
	created_at: number;
	/** Whether encryption is enabled. REQ-FAC-024 */
	encrypted: boolean;
}

/**
 * Pipe metadata stored in the .meta file alongside the pipe.
 * REQ-FAC-098, REQ-FAC-111
 */
export interface PipeMeta {
	pipe_id: string;
	session_id: string;
	creating_tool_name: string;
	optional_name?: string;
	ttl_seconds: number;
	created_at: number;
	transport: PipeTransport;
	encrypted: boolean;
}

/** Options for createPipe(). */
export interface CreatePipeOptions {
	/** Workspace root directory (absolute path). */
	workspaceRoot: string;
	/** Session cipher for encryption. REQ-FAC-024 */
	sessionCipher: SessionCipher | undefined;
	/** Session ID for metadata and cross-session detection. REQ-FAC-111 */
	sessionId: string;
	/** Tool name creating the pipe. REQ-FAC-111 */
	creatingToolName: string;
	/** Optional human-readable name (metadata only, not in path). REQ-FAC-021 */
	name?: string;
	/** TTL in seconds (default 60, max 300). REQ-FAC-022 */
	ttlSeconds?: number;
	/** Encryption enabled (default true). REQ-FAC-024 */
	encryption?: boolean;
	/** Audit log path. REQ-FAC-108 */
	auditLogPath?: string;
}

/** Options for writePipe(). */
export interface WritePipeOptions {
	/** Workspace root directory (absolute path). */
	workspaceRoot: string;
	/** Session cipher for encryption. */
	sessionCipher: SessionCipher | undefined;
	/** Write timeout in seconds (default 30). REQ-FAC-026 */
	timeoutSeconds?: number;
	/** Audit log path. REQ-FAC-108 */
	auditLogPath?: string;
}

/** Options for readPipe(). */
export interface ReadPipeOptions {
	/** Workspace root directory (absolute path). */
	workspaceRoot: string;
	/** Session cipher for decryption. */
	sessionCipher: SessionCipher | undefined;
	/** Session ID for cross-session detection. REQ-FAC-112 */
	sessionId: string;
	/** Read timeout in seconds (default 30). REQ-FAC-027 */
	timeoutSeconds?: number;
	/**
	 * Wrap decrypted output in untrusted-data boundary. Default: true.
	 * REQ-FAC-110
	 */
	wrapUntrusted?: boolean;
	/** Audit log path. REQ-FAC-108 */
	auditLogPath?: string;
}

/** Result of readPipe(). */
export interface ReadPipeResult {
	/** Plaintext content (decrypted if encrypted). */
	content: string;
	/** Whether the content was wrapped in an untrusted-data boundary. */
	wrapped: boolean;
	/** Pipe metadata at time of read. */
	meta: PipeMeta;
}

// ── In-memory active pipe registry (per session) ─────────────────────────

/** Tracks active pipes for rate limiting. REQ-FAC-023 */
const activePipes = new Set<string>();

/** Tracks consumed pipes for one-time-read enforcement. REQ-FAC-025 */
const consumedPipes = new Set<string>();

// ── Encryption helpers ───────────────────────────────────────────────────

function encryptPipeContent(
	plaintext: string,
	artifactCipher: ArtifactCipher,
): Buffer {
	const nonce = generateNonce();
	const cipher = createCipheriv("aes-256-gcm", artifactCipher, nonce);
	const encrypted = Buffer.concat([
		cipher.update(plaintext, "utf8"),
		cipher.final(),
	]);
	const tag = cipher.getAuthTag();
	return Buffer.concat([PIPE_MAGIC, nonce, encrypted, tag]);
}

function decryptPipeContent(
	ciphertext: Buffer,
	artifactCipher: ArtifactCipher,
): string {
	const magic = ciphertext.subarray(0, PIPE_MAGIC.length);
	if (!magic.equals(PIPE_MAGIC)) {
		throw new Error("File belongs to a different session.");
	}
	const offset = PIPE_MAGIC.length;
	const nonce = ciphertext.subarray(offset, offset + GCM_NONCE_BYTES);
	const tagStart = ciphertext.length - GCM_TAG_BYTES;
	const encrypted = ciphertext.subarray(offset + GCM_NONCE_BYTES, tagStart);
	const tag = ciphertext.subarray(tagStart);

	const decipher = createDecipheriv("aes-256-gcm", artifactCipher, nonce);
	decipher.setAuthTag(tag);
	try {
		return Buffer.concat([decipher.update(encrypted), decipher.final()]).toString("utf8");
	} catch {
		throw new Error("File belongs to a different session.");
	}
}

// ── Workspace boundary ───────────────────────────────────────────────────

function resolveInsideWorkspace(workspaceRoot: string, rel: string): string {
	const abs = resolve(workspaceRoot, rel);
	const root = resolve(workspaceRoot);
	if (!abs.startsWith(root + "/") && abs !== root) {
		throw new Error(`Pipe path escapes workspace boundary: ${rel}`);
	}
	return abs;
}

// ── Audit logging ────────────────────────────────────────────────────────

/**
 * Append a structured audit log entry for pipe operations.
 * REQ-FAC-104–109: pipe contents MUST NOT be logged; only metadata.
 * REQ-FAC-108: max 10MB per file, max 5 rotation files.
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-108 plan=phase-100/task-100-backlog/fb-013 jira_ref=DEX-458
 */
function auditLog(
	workspaceRoot: string,
	auditLogPath: string | undefined,
	entry: Record<string, unknown>,
): void {
	const logPath = auditLogPath ?? join(".axiom", "logs", "pipe-audit.jsonl");
	const absLog = resolveInsideWorkspace(workspaceRoot, logPath);
	try {
		mkdirSync(join(absLog, ".."), { recursive: true });

		// REQ-FAC-108: rotate if file exceeds 10MB
		const MAX_LOG_BYTES = 10 * 1024 * 1024; // 10MB
		const MAX_ROTATIONS = 5;

		if (existsSync(absLog)) {
			const size = statSync(absLog).size;
			if (size >= MAX_LOG_BYTES) {
				// Rotate: shift existing rotations down, rename current to .1
				for (let i = MAX_ROTATIONS - 1; i >= 1; i--) {
					const older = `${absLog}.${i}`;
					const newer = `${absLog}.${i + 1}`;
					if (existsSync(older)) {
						if (i === MAX_ROTATIONS - 1) {
							// Delete the oldest rotation to make room
							try { unlinkSync(newer); } catch { /* ignore */ }
						}
						renameSync(older, newer);
					}
				}
				renameSync(absLog, `${absLog}.1`);
			}
		}

		appendFileSync(absLog, JSON.stringify({ ...entry, timestamp: new Date().toISOString() }) + "\n");
	} catch {
		// Best-effort — audit log failure must not break pipe operations
	}
}

// ── Transport detection ──────────────────────────────────────────────────

/**
 * Detect whether mkfifo is available and reliable on this platform.
 *
 * OD-FAC-7: macOS FIFO behavior may differ from Linux (poll/select semantics).
 * On macOS, we default to tempfile transport for reliability.
 * On Linux, we use FIFO if mkfifo is available.
 *
 * The tempfile transport provides the same one-time-read semantics:
 * - Write: atomic temp→rename.
 * - Read: read file, then immediately delete.
 * - One-time-read: enforced by the consumed-pipes registry.
 */
function detectTransport(): PipeTransport {
	// OD-FAC-7: macOS defaults to tempfile for reliability
	if (process.platform === "darwin") {
		return "tempfile";
	}
	try {
		execSync("which mkfifo", { stdio: "ignore" });
		return "fifo";
	} catch {
		return "tempfile";
	}
}

// ── Create ───────────────────────────────────────────────────────────────

/**
 * Create a new one-time-read pipe.
 *
 * Returns a PipeDescriptor the writer uses to write data and the reader
 * uses to read it. The pipe_id is the stable identifier for both operations.
 *
 * REQ-FAC-020: UUID path, not guessable.
 * REQ-FAC-021: Optional name stored in metadata only.
 * REQ-FAC-022: TTL clamped to [1, MAX_PIPE_TTL_SECONDS].
 * REQ-FAC-023: Rate limit — max 10 active pipes per session.
 * REQ-FAC-104: Audit log on creation.
 */
export function createPipe(opts: CreatePipeOptions): PipeDescriptor {
	// REQ-FAC-023: rate limit
	if (activePipes.size >= MAX_ACTIVE_PIPES) {
		throw new Error(
			`Pipe rate limit exceeded — maximum ${MAX_ACTIVE_PIPES} active pipes per session.`,
		);
	}

	const pipeId = randomUUID();
	const ttl = Math.min(
		Math.max(opts.ttlSeconds ?? DEFAULT_PIPE_TTL_SECONDS, 1),
		MAX_PIPE_TTL_SECONDS,
	);
	const encrypted = opts.encryption !== false && opts.sessionCipher !== undefined;
	const transport = detectTransport();

	const absDir = resolveInsideWorkspace(opts.workspaceRoot, PIPE_DIR);
	mkdirSync(absDir, { recursive: true });

	const pipePath = join(PIPE_DIR, pipeId);
	const metaPath = join(PIPE_DIR, `${pipeId}.meta`);
	const absPipePath = join(absDir, pipeId);
	const absMetaPath = join(absDir, `${pipeId}.meta`);

	// Create the transport file
	if (transport === "fifo") {
		try {
			execSync(`mkfifo "${absPipePath}"`, { stdio: "ignore" });
		} catch {
			// mkfifo failed — fall back to tempfile
			// (file will be created on write)
		}
	}
	// For tempfile transport, the file is created on write

	// Write metadata. REQ-FAC-098, REQ-FAC-111
	const meta: PipeMeta = {
		pipe_id: pipeId,
		session_id: opts.sessionId,
		creating_tool_name: opts.creatingToolName,
		optional_name: opts.name,
		ttl_seconds: ttl,
		created_at: Date.now(),
		transport,
		encrypted,
	};
	writeFileSync(absMetaPath, JSON.stringify(meta, null, 2));

	activePipes.add(pipeId);

	// REQ-FAC-104: audit log creation
	auditLog(opts.workspaceRoot, opts.auditLogPath, {
		event: "pipe.created",
		pipe_id: pipeId,
		ttl_seconds: ttl,
		encrypted,
		creating_tool_name: opts.creatingToolName,
		transport,
	});

	return {
		pipe_id: pipeId,
		pipe_path: pipePath,
		meta_path: metaPath,
		transport,
		ttl_seconds: ttl,
		created_at: meta.created_at,
		encrypted,
	};
}

// ── Write ────────────────────────────────────────────────────────────────

/**
 * Write data to a pipe.
 *
 * For FIFO transport: writes to the named pipe (blocks until reader connects,
 * up to timeoutSeconds). For tempfile transport: writes atomically to a temp
 * file then renames.
 *
 * REQ-FAC-024: encrypts with AES-256-GCM if encryption enabled.
 * REQ-FAC-026: write timeout.
 */
export async function writePipe(
	descriptor: PipeDescriptor,
	content: string,
	opts: WritePipeOptions,
): Promise<void> {
	const absPipePath = resolveInsideWorkspace(opts.workspaceRoot, descriptor.pipe_path);
	const timeoutMs = (opts.timeoutSeconds ?? DEFAULT_PIPE_TIMEOUT_SECONDS) * 1000;

	let payload: Buffer;
	if (descriptor.encrypted && opts.sessionCipher) {
		const artifactCipher = deriveArtifactCipher(opts.sessionCipher, descriptor.pipe_id);
		payload = encryptPipeContent(content, artifactCipher);
	} else {
		payload = Buffer.from(content, "utf8");
	}

	if (descriptor.transport === "fifo") {
		// For FIFO: write with timeout. If no reader connects within timeout, fail.
		await Promise.race([
			writeFile(absPipePath, payload),
			new Promise<never>((_, reject) =>
				setTimeout(() => {
					// REQ-FAC-107: log failed pipe operations
					auditLog(opts.workspaceRoot, opts.auditLogPath, {
						event: "pipe.failed",
						pipe_id: descriptor.pipe_id,
						reason: "write_timeout",
					});
					reject(new Error("Pipe write timed out — no reader connected."));
				}, timeoutMs),
			),
		]);
	} else {
		// Tempfile: atomic write (temp → rename)
		const tmpPath = `${absPipePath}.tmp`;
		await writeFile(tmpPath, payload);
		renameSync(tmpPath, absPipePath);
	}
}

// ── Read ─────────────────────────────────────────────────────────────────

/**
 * Read data from a pipe (one-time-read).
 *
 * Decrypts the content, wraps in untrusted-data boundary, deletes the pipe
 * and metadata files immediately. A second read attempt returns an error.
 *
 * REQ-FAC-025: one-time-read — second read returns "Pipe already consumed."
 * REQ-FAC-027: read timeout.
 * REQ-FAC-028: immediate deletion after read.
 * REQ-FAC-095: pipe contents deleted immediately.
 * REQ-FAC-110: untrusted-data boundary.
 * REQ-FAC-112: cross-session read fails.
 * REQ-FAC-105: audit log on read.
 */
export async function readPipe(
	descriptor: PipeDescriptor,
	opts: ReadPipeOptions,
): Promise<ReadPipeResult> {
	const startMs = Date.now();

	// REQ-FAC-025: one-time-read enforcement
	if (consumedPipes.has(descriptor.pipe_id)) {
		// REQ-FAC-107: log failed pipe operations
		auditLog(opts.workspaceRoot, opts.auditLogPath, {
			event: "pipe.failed",
			pipe_id: descriptor.pipe_id,
			reason: "already_consumed",
		});
		throw new Error("Pipe already consumed.");
	}

	// Load metadata for cross-session check. REQ-FAC-112
	const absMetaPath = resolveInsideWorkspace(opts.workspaceRoot, descriptor.meta_path);
	let meta: PipeMeta;
	try {
		meta = JSON.parse(readFileSync(absMetaPath, "utf8")) as PipeMeta;
	} catch {
		throw new Error("Pipe already consumed.");
	}

	// REQ-FAC-112: cross-session check
	if (meta.session_id !== opts.sessionId) {
		throw new Error("File belongs to a different session.");
	}

	const absPipePath = resolveInsideWorkspace(opts.workspaceRoot, descriptor.pipe_path);
	const timeoutMs = (opts.timeoutSeconds ?? DEFAULT_PIPE_TIMEOUT_SECONDS) * 1000;

	// REQ-FAC-025: mark consumed BEFORE reading to prevent concurrent read race.
	// Moving add() before waitForFile() narrows the race window to near-zero.
	// axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-025 plan=phase-100/task-100-5/step-100-5-8 jira_ref=DEX-458
	consumedPipes.add(descriptor.pipe_id);
	activePipes.delete(descriptor.pipe_id);

	// Wait for the pipe file to exist (FIFO: writer must write first)
	const waitForFile = async (): Promise<Buffer> => {
		const deadline = Date.now() + timeoutMs;
		while (!existsSync(absPipePath)) {
			if (Date.now() > deadline) {
				throw new Error("Pipe read timed out — writer may not have sent data.");
			}
			await new Promise((r) => setTimeout(r, 50));
		}
		return readFile(absPipePath);
	};

	const raw = await Promise.race([
		waitForFile(),
		new Promise<never>((_, reject) =>
			setTimeout(() => {
				// REQ-FAC-107: log failed pipe operations
				auditLog(opts.workspaceRoot, opts.auditLogPath, {
					event: "pipe.failed",
					pipe_id: descriptor.pipe_id,
					reason: "read_timeout",
				});
				reject(new Error("Pipe read timed out — writer may not have sent data."));
			}, timeoutMs),
		),
	]);

	// REQ-FAC-028/095: delete pipe and metadata immediately
	try { unlinkSync(absPipePath); } catch { /* already gone */ }
	try { unlinkSync(absMetaPath); } catch { /* already gone */ }

	// Decrypt
	let content: string;
	if (descriptor.encrypted && opts.sessionCipher) {
		const artifactCipher = deriveArtifactCipher(opts.sessionCipher, descriptor.pipe_id);
		content = decryptPipeContent(raw, artifactCipher);
	} else {
		content = raw.toString("utf8");
	}

	const latencyMs = Date.now() - startMs;

	// REQ-FAC-105: audit log read (content NOT logged)
	auditLog(opts.workspaceRoot, opts.auditLogPath, {
		event: "pipe.read",
		pipe_id: descriptor.pipe_id,
		reader_tool_name: "codeops_read_pipe",
		latency_ms: latencyMs,
	});

	// REQ-FAC-110: wrap in untrusted-data boundary
	const wrap = opts.wrapUntrusted !== false;
	return {
		content: wrap ? wrapUntrustedData(content) : content,
		wrapped: wrap,
		meta,
	};
}

// ── Reaper ───────────────────────────────────────────────────────────────

/**
 * Delete pipes that have exceeded their TTL without being read.
 * REQ-FAC-029, REQ-FAC-096
 *
 * Should be called periodically (at most once per minute).
 * Returns the number of pipes reaped.
 */
export function reapExpiredPipes(workspaceRoot: string): number {
	const absDir = resolveInsideWorkspace(workspaceRoot, PIPE_DIR);
	if (!existsSync(absDir)) return 0;

	const now = Date.now();
	let reaped = 0;

	try {
		const entries = readdirSync(absDir, { withFileTypes: true });
		for (const entry of entries) {
			if (!entry.isFile() || entry.name.endsWith(".meta")) continue;
			const pipeId = entry.name;
			const metaPath = join(absDir, `${pipeId}.meta`);
			const pipePath = join(absDir, pipeId);

			try {
				if (!existsSync(metaPath)) {
					// No metadata — orphan, delete
					unlinkSync(pipePath);
					reaped++;
					continue;
				}
				const meta = JSON.parse(readFileSync(metaPath, "utf8")) as PipeMeta;
				const ageMs = now - meta.created_at;
				const ttlMs = meta.ttl_seconds * 1000;
				if (ageMs > ttlMs) {
					try { unlinkSync(pipePath); } catch { /* ignore */ }
					try { unlinkSync(metaPath); } catch { /* ignore */ }
					activePipes.delete(pipeId);
					reaped++;
				}
			} catch {
				// Malformed metadata or concurrent deletion — skip
			}
		}
	} catch {
		// Best-effort
	}

	// REQ-FAC-023: evict stale consumed pipe IDs to prevent unbounded growth.
	// consumedPipes tracks IDs to enforce one-time-read. After MAX_PIPE_TTL_SECONDS * 2
	// (600s), a consumed pipe ID can safely be evicted — no legitimate retry would occur
	// that far after consumption.
	// Cap: if consumedPipes grows beyond MAX_ACTIVE_PIPES * 100 (1000 entries), clear all.
	// axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-023 plan=phase-100/task-100-backlog/fb-007 jira_ref=DEX-458
	if (consumedPipes.size > MAX_ACTIVE_PIPES * 100) {
		consumedPipes.clear();
	}

	return reaped;
}

/**
 * Clean up orphaned pipes from previous sessions (creation time > 1 hour).
 * Called on session start. REQ-FAC-097
 */
export function cleanupOrphanedPipes(workspaceRoot: string): void {
	const absDir = resolveInsideWorkspace(workspaceRoot, PIPE_DIR);
	if (!existsSync(absDir)) return;

	const now = Date.now();
	try {
		const entries = readdirSync(absDir, { withFileTypes: true });
		for (const entry of entries) {
			if (!entry.isFile()) continue;
			const absFile = join(absDir, entry.name);
			try {
				const stat = statSync(absFile);
				if (now - stat.mtimeMs > ORPHAN_AGE_MS) {
					unlinkSync(absFile);
				}
			} catch { /* ignore */ }
		}
	} catch { /* best-effort */ }
}

/**
 * Clean up all pipes for the current session.
 * Called on session end.
 */
export function cleanupAllPipes(workspaceRoot: string): void {
	const absDir = resolveInsideWorkspace(workspaceRoot, PIPE_DIR);
	if (!existsSync(absDir)) return;
	try {
		const entries = readdirSync(absDir, { withFileTypes: true });
		for (const entry of entries) {
			if (!entry.isFile()) continue;
			try { unlinkSync(join(absDir, entry.name)); } catch { /* ignore */ }
		}
	} catch { /* best-effort */ }
	// Clear consumed pipe registry on session end
	consumedPipes.clear();
	activePipes.clear();
}

// ── Active pipe count ────────────────────────────────────────────────────

/** Returns the number of currently active (not yet consumed) pipes. REQ-FAC-023 */
export function getActivePipeCount(): number {
	return activePipes.size;
}

/** Reset active/consumed pipe registries (for testing only). */
export function _resetPipeRegistriesForTest(): void {
	activePipes.clear();
	consumedPipes.clear();
}
