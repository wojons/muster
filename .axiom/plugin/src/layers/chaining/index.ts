/**
 * L8: Chaining layer barrel.
 *
 * Spec: specs/87-MCP-File-Agent-Communication.md#6
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md plan=phase-100/task-100-3 jira_ref=DEX-458
 */

export type {
	ChainContext,
	ChainResult,
	ChainStepResult,
	ChainValidationResult,
	ExecuteChainOptions,
	ToolExecutor,
} from "./chain-executor";
export {
	DEFAULT_MAX_CHAIN_DEPTH,
	detectChainCycles,
	evaluateChainCondition,
	executeChain,
	resolveStepInput,
	validateAllChains,
	validateChain,
} from "./chain-executor";
