/**
 * L8: Pipes layer barrel.
 *
 * Spec: specs/87-MCP-File-Agent-Communication.md#4
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md plan=phase-100/task-100-2 jira_ref=DEX-458
 */

export type {
	CreatePipeOptions,
	PipeDescriptor,
	PipeMeta,
	PipeTransport,
	ReadPipeOptions,
	ReadPipeResult,
	WritePipeOptions,
} from "./pipe";
export {
	DEFAULT_PIPE_TIMEOUT_SECONDS,
	DEFAULT_PIPE_TTL_SECONDS,
	MAX_ACTIVE_PIPES,
	MAX_PIPE_TTL_SECONDS,
	PIPE_DIR,
	cleanupAllPipes,
	cleanupOrphanedPipes,
	createPipe,
	getActivePipeCount,
	reapExpiredPipes,
	readPipe,
	writePipe,
} from "./pipe";
