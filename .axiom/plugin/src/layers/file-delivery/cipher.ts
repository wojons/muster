/**
 * Session encryption context for L8 file-based agent communication.
 *
 * Generates a random AES-256 session key at session start and derives
 * per-file/per-pipe keys using HKDF. The session key is held in memory
 * only — it never appears in tool results, logs, context windows, or
 * any artifact visible to the model or user.
 *
 * REQ-FAC-030: Random AES-256 session key generated at session start.
 * REQ-FAC-031: CRITICAL — key MUST NOT appear in tool results, logs, or context.
 * REQ-FAC-033: Per-file/pipe keys derived via HKDF(session_key, salt=uuid, info="axiom-at-rest-v1").
 * REQ-FAC-034: Key generated using CSPRNG (crypto.randomBytes(32)).
 *
 * Spec: specs/87-MCP-File-Agent-Communication.md#5.1
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-030 plan=phase-100/task-100-1/step-100-1-2 jira_ref=DEX-458
 */

import { createHmac, randomBytes } from "node:crypto";

// ── Constants ────────────────────────────────────────────────────────────

/** HKDF info string — stable identifier for this derivation context. REQ-FAC-033 */
const HKDF_INFO = "axiom-at-rest-v1";

/** AES-256 key length in bytes. */
const KEY_BYTES = 32;

/** GCM nonce (IV) length in bytes. AES-GCM standard is 12 bytes. */
export const GCM_NONCE_BYTES = 12;

/** GCM authentication tag length in bytes. */
export const GCM_TAG_BYTES = 16;

// ── Types ────────────────────────────────────────────────────────────────

/**
 * Opaque session cipher material — never serialized, never logged.
 * REQ-FAC-031: must not appear in tool results, context, or logs.
 */
export type SessionCipher = Buffer & { readonly __brand: "SessionCipher" };

/** Derived per-artifact cipher material — never serialized, never logged. */
export type ArtifactCipher = Buffer & { readonly __brand: "ArtifactCipher" };

// ── Session cipher generation ────────────────────────────────────────────

/**
 * Generate a new random AES-256 session cipher using CSPRNG.
 *
 * Call once at plugin session start. Store the result in memory only.
 * Never pass this value to tool results, logs, or any serialized output.
 *
 * REQ-FAC-030, REQ-FAC-034
 */
export function generateSessionCipher(): SessionCipher {
	return randomBytes(KEY_BYTES) as SessionCipher;
}

// ── HKDF derivation ──────────────────────────────────────────────────────

/**
 * Derive a per-artifact cipher from the session cipher using HKDF-SHA256.
 *
 * Each file-ref or pipe gets its own derived cipher so that:
 * - Compromise of one file's cipher does not expose others.
 * - Cross-session reads fail (different session cipher → different derived cipher).
 *
 * Implementation: HKDF(session_cipher, salt=artifactId, info="axiom-at-rest-v1")
 * using HMAC-SHA256 as the PRF (RFC 5869 two-step: extract then expand).
 *
 * REQ-FAC-033
 *
 * @param sessionCipher  The session cipher (32 bytes, from generateSessionCipher).
 * @param artifactId     The file UUID or pipe UUID used as the HKDF salt.
 * @returns              A 32-byte derived cipher for AES-256-GCM.
 */
export function deriveArtifactCipher(
	sessionCipher: SessionCipher,
	artifactId: string,
): ArtifactCipher {
	// HKDF-Extract: PRK = HMAC-SHA256(salt=artifactId, IKM=sessionCipher)
	const salt = Buffer.from(artifactId, "utf8");
	const prk = createHmac("sha256", salt).update(sessionCipher).digest();

	// HKDF-Expand: OKM = HMAC-SHA256(PRK, info || 0x01)
	const info = Buffer.from(HKDF_INFO, "utf8");
	const counter = Buffer.from([0x01]);
	const okm = createHmac("sha256", prk)
		.update(info)
		.update(counter)
		.digest();

	// sha256 output is 32 bytes = KEY_BYTES
	return okm.subarray(0, KEY_BYTES) as ArtifactCipher;
}

// ── Nonce generation ─────────────────────────────────────────────────────

/**
 * Generate a random 12-byte nonce for AES-256-GCM.
 *
 * A fresh nonce MUST be generated for every encryption operation.
 * Nonce reuse with the same cipher breaks GCM's authentication guarantee.
 */
export function generateNonce(): Buffer {
	return randomBytes(GCM_NONCE_BYTES);
}

// ── Safety helpers ───────────────────────────────────────────────────────

/**
 * Verify that a value does NOT contain session cipher material.
 * Used in tests to assert REQ-FAC-031.
 *
 * @param value          The string/object to check (serialized to JSON if object).
 * @param sessionCipher  The session cipher to search for.
 * @returns              true if the value is safe (cipher not found).
 */
export function doesNotContainCipher(
	value: unknown,
	sessionCipher: SessionCipher,
): boolean {
	const serialized =
		typeof value === "string" ? value : JSON.stringify(value) ?? "";
	const cipherHex = sessionCipher.toString("hex");
	const cipherBase64 = sessionCipher.toString("base64");
	return !serialized.includes(cipherHex) && !serialized.includes(cipherBase64);
}
