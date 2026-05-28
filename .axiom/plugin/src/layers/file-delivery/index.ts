/**
 * L8: File-based agent communication — file-delivery layer barrel.
 *
 * Exports the public API for Pillar 1 (file-path delivery) of the
 * MCP file-based agent communication system.
 *
 * Spec: specs/87-MCP-File-Agent-Communication.md
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md plan=phase-100/task-100-1 jira_ref=DEX-458
 */

export type {
	ArtifactCipher,
	SessionCipher,
} from "./cipher";
export {
	GCM_NONCE_BYTES,
	GCM_TAG_BYTES,
	deriveArtifactCipher,
	doesNotContainCipher,
	generateNonce,
	generateSessionCipher,
} from "./cipher";

export type {
	FileRef,
	ReadFileRefOptions,
	ReadFileRefResult,
	WriteFileRefOptions,
} from "./file-ref";
export {
	DEFAULT_THRESHOLD_BYTES,
	FILE_REF_DIR,
	SUBAGENT_OUTPUT_DIR,
	cleanupFileRefs,
	cleanupOrphanedFileRefs,
	readFileRef,
	shouldUseFileRef,
	wrapUntrustedData,
	writeFileRef,
	writeFileRefSync,
} from "./file-ref";
