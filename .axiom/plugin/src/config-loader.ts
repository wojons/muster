/**
 * Config-loader — loads and validates .axiom/plugin.yaml or .axiom/plugin.json
 * for Layer 5 (Config-Driven Automation) and Layer 6 (Tool Extension).
 *
 * REQ-PLG-400: Load automation config from .axiom/plugin.yaml (or .axiom/plugin.json).
 *              If the file does not exist, L5 is disabled silently.
 * REQ-PLG-401: Config schema supports version, layers, metrics, hooks, watches, scripts, tools.
 * REQ-PLG-402: Validate on load; invalid config -> conversational error + defaults.
 * REQ-PLG-403: version: 1 for forward compat; unknown version -> warning + defaults.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-400, #REQ-PLG-401, #REQ-PLG-402, #REQ-PLG-403
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-400 plan=phase-70-4/task-70-4-1/step-70-4-1-2
 */

import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { parse as parseYaml } from "yaml";

// ── Types ───────────────────────────────────────────────────────────────

/** Layer toggle configuration. */
export interface LayersConfig {
	metrics?: boolean;
	session_sharing?: boolean;
	network?: boolean;
	automation?: boolean;
	tools?: boolean;
	/**
	 * L8: File-based agent communication (file-ref delivery, pipes, chaining, search).
	 * Default: true when any l8 config section is present.
	 * REQ-FAC-001 through REQ-FAC-112
	 */
	file_comm?: boolean;
}

/** Metrics configuration section. */
export interface MetricsConfig {
	session_budget_usd?: number;
	warning_threshold_pct?: number;
	persist_sessions?: boolean;
	pricing_overrides?: Record<
		string,
		{ prompt_per_1m?: number; completion_per_1m?: number }
	>;
}

/** A single hook entry. */
export interface HookEntry {
	name: string;
	run: string;
	description?: string;
	condition?: string;
}

/** Hooks configuration section — lifecycle event arrays. */
export interface HooksConfig {
	on_session_start?: HookEntry[];
	on_session_end?: HookEntry[];
	on_session_idle?: HookEntry[];
	on_message?: HookEntry[];
	on_command?: HookEntry[];
	on_error?: HookEntry[];
}

/** A single watch entry. */
export interface WatchEntry {
	name: string;
	trigger: string;
	pattern?: string;
	condition?: string;
	run?: string;
	action?: string;
	message?: string;
	description?: string;
}

/** A named script entry. */
export interface ScriptEntry {
	run: string;
	description?: string;
	requires_confirmation?: boolean;
	args?: string[];
}

/** A custom tool definition. */
export interface ToolEntry {
	name: string;
	description?: string;
	run: string;
	parameters?: Record<
		string,
		{ type?: string; description?: string; default?: unknown }
	>;
}

/** Network configuration section. */
export interface NetworkConfig {
	enabled?: boolean;
	team_id?: string;
	backend_url?: string;
	heartbeat_interval_seconds?: number;
	user_identifier?: string;
}

// ── Listener types (REQ-LNP-001 through REQ-LNP-078) ────────────────────

/**
 * Source type for a listener.
 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-100 plan=phase-1/task-1.1
 */
export type ListenerSourceType = "sqs" | "http" | "script" | "nats" | "kafka" | "eventbridge";

/** SQS-specific listener configuration (REQ-LNP-002). */
export interface SqsListenerConfig {
	queue_url: string;
	region: string;
	poll_interval_seconds?: number;
	max_messages_per_poll?: number;
	visibility_timeout_seconds?: number;
	/** Explicit credentials — MUST NOT be logged (REQ-LNP-004). */
	access_key_id?: string;
	secret_access_key?: string;
}

/** HTTP webhook listener configuration (REQ-LNP-011). */
export interface HttpListenerConfig {
	port: number;
	path: string;
	host?: string;
	/** Optional webhook verification (REQ-LNP-014). */
	secret_token?: string;
	signature?: string;
}

/** Script-based listener configuration (REQ-LNP-021). */
export interface ScriptListenerConfig {
	command: string;
	args: string[];
	timeout_seconds?: number;
	working_directory?: string;
	poll_interval_seconds?: number;
}

/**
 * NATS listener configuration (REQ-LNP-101).
 * Supports both core NATS (at-most-once) and JetStream (at-least-once).
 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-101 plan=phase-1/task-1.1
 */
export interface NatsListenerConfig {
	/** Array of NATS server URLs (required). */
	servers: string[];
	/** NATS subject to subscribe to; supports wildcards * and > (REQ-LNP-105). */
	subject: string;
	/** NATS queue group for load-balanced delivery (optional). */
	queue_group?: string;
	/** Enable JetStream consumer (default: false). OD-LNP-06 resolved: core NATS is default. */
	jetstream?: boolean;
	/** JetStream stream name (required when jetstream: true). */
	stream?: string;
	/** JetStream durable consumer name (optional). */
	consumer?: string;
	/** JetStream deliver policy (default: "new"). */
	deliver_policy?: "new" | "all" | "last" | "by_start_sequence" | "by_start_time";
	/** JetStream ack wait timeout in seconds (default: 30). */
	ack_wait_seconds?: number;
	/** Path to NATS credentials file — MUST NOT be logged (REQ-LNP-106). */
	credentials_file?: string;
	/** TLS certificate path (optional). */
	tls_cert?: string;
	/** TLS key path (optional). */
	tls_key?: string;
	/** TLS CA certificate path (optional). */
	tls_ca?: string;
}

/**
 * Kafka listener configuration (REQ-LNP-111).
 * OD-LNP-07 resolved: auto_commit: false (manual ack after successful processing).
 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-111 plan=phase-2/task-2.2
 */
export interface KafkaListenerConfig {
	/** Array of broker addresses (required). */
	brokers: string[];
	/** Kafka topic to consume from (required). */
	topic: string;
	/** Consumer group ID (required). */
	group_id: string;
	/** Kafka client identifier (default: "axiom-listener"). */
	client_id?: string;
	/** Start from earliest offset on first run (default: false). */
	from_beginning?: boolean;
	/** Auto-commit offsets — NOT recommended; manual commit preferred (default: false). */
	auto_commit?: boolean;
	/** Session timeout in ms (default: 30000). */
	session_timeout_ms?: number;
	/** Heartbeat interval in ms (default: 3000). */
	heartbeat_interval_ms?: number;
	/** Max bytes per partition (default: 1048576). */
	max_bytes_per_partition?: number;
	/** SASL authentication — credentials MUST NOT be logged (REQ-LNP-115). */
	sasl?: {
		mechanism: "plain" | "scram-sha-256" | "scram-sha-512";
		username: string;
		password: string;
	};
	/** SSL/TLS configuration. */
	ssl?: {
		ca_cert?: string;
		client_cert?: string;
		client_key?: string;
	};
	/**
	 * Consumer lag threshold for listener.kafka_lag emission (default: 1000).
	 * REQ-LNP-116: emit listener.kafka_lag when consumer lag exceeds this threshold.
	 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-116 plan=step-fix-kafka-001
	 */
	lag_threshold?: number;
	/**
	 * Confluent Schema Registry URL for Avro/Protobuf deserialization (REQ-LNP-118).
	 * When configured, messages are deserialized before routing.
	 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-118 plan=step-fix-kafka-002
	 */
	schema_registry_url?: string;
	/** Schema registry credentials — MUST NOT be logged (REQ-LNP-118). */
	schema_registry_credentials?: { username: string; password: string };
}

/**
 * EventBridge listener configuration (REQ-LNP-121).
 * EventBridge → SQS fan-out: adapter creates/manages an EventBridge rule + SQS queue.
 *
 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-121 plan=phase-3/task-3.2
 */
export interface EventBridgeListenerConfig {
	/** EventBridge event bus name (required). */
	event_bus_name: string;
	/** EventBridge rule name (required). */
	rule_name: string;
	/** EventBridge event pattern JSON string (required). */
	event_pattern: string;
	/** Existing SQS queue URL; if omitted, adapter creates a managed queue (REQ-LNP-122). */
	target_queue_url?: string;
	/** AWS region (required). */
	region: string;
	/** If true (default), adapter creates/manages rule + queue (REQ-LNP-122). */
	managed?: boolean;
	/** Optional description for the EventBridge rule. */
	rule_description?: string;
}

/** Event filter configuration (REQ-LNP-036, REQ-LNP-037). */
export interface ListenerFilter {
	[field: string]: string | { operator: "exact" | "prefix" | "contains" | "regex" | "exists"; value?: string };
}

/** Routing configuration for a listener (REQ-LNP-031, REQ-LNP-032). */
export interface ListenerRouting {
	match?: Record<string, string>;
	match_all?: boolean;
	fallback_session?: string;
}

/** Lifecycle configuration for a listener. */
export interface ListenerLifecycle {
	auto_start?: boolean;
	auto_stop_on_session_end?: boolean;
	max_events?: number;
	ttl_seconds?: number;
}

/** Rubix sanitization configuration (REQ-LNP-065). */
export interface RubixConfig {
	enabled?: boolean;
	endpoint?: string;
	timeout_ms?: number;
}

/** Execution surface for the run action (REQ-LNP-141). */
export type RunSurface = "cli" | "http";

/**
 * Configuration for the run action (REQ-LNP-142).
 * Used when ListenerEntry.action is "run" or "inject_and_run".
 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-142 plan=phase-5/task-5.1 jira_ref=DEX-434
 */
export interface RunActionConfig {
	/** Work item ID to run. Supports $event.* variable substitution. */
	work_item: string;
	/** Repo path for the runner (default: "."). */
	repo?: string;
	/** Optional prompt override. Supports $event.* variable substitution. */
	prompt?: string;
	/** Max runner execution time in seconds (default: 300). */
	timeout_seconds?: number;
	/** Execution surface: "cli" (default) or "http". */
	surface?: RunSurface;
	/** HTTP API base URL (required when surface="http"). e.g. "http://127.0.0.1:8100" */
	api_url?: string;
}

/** A single listener configuration entry (Appendix B). */
export interface ListenerEntry {
	name: string;
	enabled?: boolean;
	source: ListenerSourceType;
	config: SqsListenerConfig | HttpListenerConfig | ScriptListenerConfig | NatsListenerConfig | KafkaListenerConfig | EventBridgeListenerConfig;
	filter?: ListenerFilter;
	routing?: ListenerRouting;
	/** Delivery mode: interruptive or non-interruptive (REQ-LNP-050). */
	delivery?: "interruptive" | "non-interruptive";
	lifecycle?: ListenerLifecycle;
	/** Rubix sanitization config (REQ-LNP-065). */
	rubix?: RubixConfig;
	/**
	 * Action to take when an event is received (REQ-LNP-140).
	 * - "inject" (default): push event into agent context window (existing behavior)
	 * - "run": trigger a Axiom execution with event as context; do NOT inject
	 * - "inject_and_run": both inject into context AND trigger execution
	 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-140 plan=phase-5/task-5.1 jira_ref=DEX-434
	 */
	action?: "inject" | "run" | "inject_and_run";
	/** Run action configuration (required when action is "run" or "inject_and_run"). */
	run?: RunActionConfig;
}

// ── L8: File-Based Agent Communication (REQ-FAC-001 through REQ-FAC-112) ─

/**
 * File-path delivery configuration (Pillar 1).
 * Controls when MCP tool output is written to a file-ref instead of returned inline.
 *
 * REQ-FAC-002: threshold_bytes configurable; default 4096.
 * Spec: specs/87-MCP-File-Agent-Communication.md
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-002 plan=phase-100/task-100-1/step-100-1-1 jira_ref=DEX-458
 */
export interface FileDeliveryConfig {
	/**
	 * Result size threshold in bytes above which output is written to a file-ref
	 * instead of returned inline. Default: 4096 (4KB).
	 * REQ-FAC-002
	 */
	threshold_bytes?: number;
	/**
	 * Enable/disable file-path delivery entirely. Default: true.
	 * When false, all output is returned inline regardless of size.
	 */
	enabled?: boolean;
	/**
	 * Disable at-rest encryption for file-ref files. Default: false (encryption on).
	 * When true, a warning is logged: "At-rest encryption disabled — MCP output files will be plaintext on disk."
	 * REQ-FAC-037
	 */
	disable_encryption?: boolean;
}

/**
 * One-time-read pipe configuration (Pillar 2).
 * Controls encrypted ephemeral FIFO pipes for sensitive data transfer.
 *
 * REQ-FAC-022: ttl_seconds configurable; default 60, max 300.
 * REQ-FAC-024: encryption configurable; default true.
 * REQ-FAC-026/027: write/read timeout configurable; default 30s.
 * Spec: specs/87-MCP-File-Agent-Communication.md
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-022 plan=phase-100/task-100-2/step-100-2-1 jira_ref=DEX-458
 */
export interface PipeConfig {
	/**
	 * Enable/disable pipe encryption. Default: true (AES-256-GCM).
	 * REQ-FAC-024
	 */
	encryption?: boolean;
	/**
	 * Default TTL in seconds for new pipes. Default: 60, max: 300.
	 * REQ-FAC-022
	 */
	default_ttl_seconds?: number;
	/**
	 * Write/read timeout in seconds. Default: 30.
	 * REQ-FAC-026/027
	 */
	timeout_seconds?: number;
	/**
	 * Audit log path for pipe operations. Default: ".axiom/logs/pipe-audit.jsonl".
	 * REQ-FAC-108
	 */
	audit_log_path?: string;
}

/**
 * Tool chaining configuration (Pillar 3).
 * Controls declarative multi-step tool workflows.
 *
 * REQ-FAC-040: all tools chainable by default.
 * REQ-FAC-043: max chain depth; default 10.
 * Spec: specs/87-MCP-File-Agent-Communication.md
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-040 plan=phase-100/task-100-3/step-100-3-1 jira_ref=DEX-458
 */
export interface ChainStepConfig {
	/** Tool name to invoke. */
	tool: string;
	/** Input mapping: key = tool param name, value = literal or "$prev.fieldName" context ref. */
	input?: Record<string, string>;
	/**
	 * Optional condition expression evaluated against the local context.
	 * Step is skipped when condition evaluates to false.
	 * REQ-FAC-051
	 */
	if?: string;
}

export interface ChainConfig {
	/** Chain name (used in tool result and logs). */
	name: string;
	/** Ordered list of steps. */
	steps: ChainStepConfig[];
	/** Human-readable description. */
	description?: string;
}

export interface ChainingConfig {
	/**
	 * Maximum chain depth before cycle-detection error. Default: 10.
	 * REQ-FAC-043
	 */
	max_depth?: number;
	/** Declarative chain definitions. REQ-FAC-041 */
	chains?: ChainConfig[];
}

/**
 * Hybrid search server configuration (Pillar 4).
 * Controls BM25 full-text + HNSW vector search over workspace files.
 *
 * REQ-FAC-060: background search server with BM25 + HNSW indexes.
 * REQ-FAC-073: Bedrock Titan Embeddings V2 for vector search.
 * Spec: specs/87-MCP-File-Agent-Communication.md
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-060 plan=phase-100/task-100-4/step-100-4-1 jira_ref=DEX-458
 */
export interface SearchConfig {
	/**
	 * Enable/disable Bedrock embedding for vector search. Default: true.
	 * When false, only text search is available (zero cloud dependencies).
	 * REQ-FAC-100
	 */
	bedrock_enabled?: boolean;
	/**
	 * AWS region for Bedrock Titan Embeddings V2. Default: "us-east-1".
	 * REQ-FAC-073
	 */
	bedrock_region?: string;
	/**
	 * Chunk size in characters for large-file embedding. Default: 8000.
	 * REQ-FAC-076
	 */
	chunk_size?: number;
	/**
	 * Chunk overlap in characters for sliding-window chunking. Default: 200.
	 * REQ-FAC-076
	 */
	chunk_overlap?: number;
	/**
	 * Additional glob patterns to exclude from indexing (extends built-in exclusion list).
	 * REQ-FAC-082
	 */
	exclude_patterns?: string[];
	/**
	 * File extensions to include in the search index.
	 * Default: common source code extensions (.ts, .js, .py, .go, .md, etc.).
	 * REQ-FAC-084
	 */
	include_extensions?: string[];
}

/** Profile-specific config overrides (hooks are appended to default). */
export interface ProfileConfig {
	hooks?: HooksConfig;
	watches?: WatchEntry[];
	scripts?: Record<string, ScriptEntry>;
}

/** Full plugin config shape per REQ-PLG-401. */
export interface PluginConfig {
	version: number;
	layers?: LayersConfig;
	metrics?: MetricsConfig;
	network?: NetworkConfig;
	hooks?: HooksConfig;
	watches?: WatchEntry[];
	scripts?: Record<string, ScriptEntry>;
	tools?: ToolEntry[];
	/** REQ-PLG-411a: idle detection window in ms before on_session_idle fires (default: 60000). */
	idle_timeout_ms?: number;
	/** REQ-PLG-400a: profile-based config scoping. Active profile selected via AXIOM_PROFILE env var. */
	profiles?: Record<string, ProfileConfig>;
	/**
	 * L7: Event listener configurations (REQ-LNP-001 through REQ-LNP-078).
	 * Spec: specs/88-Listen-Notify-Plugin.md
	 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md
	 */
	listeners?: ListenerEntry[];
	/**
	 * L8: File-path delivery configuration (Pillar 1).
	 * Controls file-ref result type and at-rest encryption.
	 * REQ-FAC-001 through REQ-FAC-012, REQ-FAC-030 through REQ-FAC-039
	 * Spec: specs/87-MCP-File-Agent-Communication.md
	 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-002 plan=phase-100/task-100-1/step-100-1-1 jira_ref=DEX-458
	 */
	file_delivery?: FileDeliveryConfig;
	/**
	 * L8: One-time-read pipe configuration (Pillar 2).
	 * Controls encrypted ephemeral FIFO pipes.
	 * REQ-FAC-020 through REQ-FAC-029, REQ-FAC-095 through REQ-FAC-109
	 * Spec: specs/87-MCP-File-Agent-Communication.md
	 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-022 plan=phase-100/task-100-2/step-100-2-1 jira_ref=DEX-458
	 */
	pipe?: PipeConfig;
	/**
	 * L8: Tool chaining configuration (Pillar 3).
	 * Controls declarative multi-step tool workflows.
	 * REQ-FAC-040 through REQ-FAC-051
	 * Spec: specs/87-MCP-File-Agent-Communication.md
	 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-040 plan=phase-100/task-100-3/step-100-3-1 jira_ref=DEX-458
	 */
	chaining?: ChainingConfig;
	/**
	 * L8: Hybrid search server configuration (Pillar 4).
	 * Controls BM25 full-text + HNSW vector search.
	 * REQ-FAC-060 through REQ-FAC-088
	 * Spec: specs/87-MCP-File-Agent-Communication.md
	 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-060 plan=phase-100/task-100-4/step-100-4-1 jira_ref=DEX-458
	 */
	search?: SearchConfig;
}

/** Result of loadPluginConfig(). */
export interface PluginConfigResult {
	/** Whether a config file was successfully loaded and parsed. */
	loaded: boolean;
	/** The source file that was loaded ("plugin.yaml" or "plugin.json"). */
	source?: string;
	/** The parsed and validated config, or undefined if not loaded. */
	config?: PluginConfig;
	/** Conversational error message if loading/parsing failed. */
	error?: string;
	/** Warning message (e.g., unknown version). */
	warning?: string;
}

/** Result of validatePluginConfig(). */
export interface ValidationResult {
	valid: boolean;
	errors?: string[];
}

// ── Constants ───────────────────────────────────────────────────────────

/** Default plugin config — L5/L6 disabled, version 1. */
export const DEFAULT_PLUGIN_CONFIG: PluginConfig = {
	version: 1,
	layers: {
		metrics: true,
		session_sharing: false,
		network: false,
		automation: false,
		tools: false,
	},
};

/** The only currently supported config version. */
const SUPPORTED_VERSION = 1;

// ── Validation ──────────────────────────────────────────────────────────

/**
 * Validate a parsed plugin config object.
 *
 * REQ-PLG-402: Invalid config produces clear errors.
 * REQ-PLG-403: version: 1 required; unknown versions are warned separately.
 */
export function validatePluginConfig(config: PluginConfig): ValidationResult {
	const errors: string[] = [];

	// version is required and must be a number
	if (config.version === undefined || config.version === null) {
		errors.push("Missing required field: version");
	} else if (typeof config.version !== "number") {
		errors.push(
			`Field "version" must be a number, got ${typeof config.version}`,
		);
	}

	if (errors.length > 0) {
		return { valid: false, errors };
	}

	// REQ-FAC-086: validate exclusion patterns at config load time
	// Invalid patterns produce warnings (not errors) — they are skipped, not blocking.
	if (config.search?.exclude_patterns) {
		for (const pattern of config.search.exclude_patterns) {
			try {
				// Attempt to compile the pattern — invalid patterns throw
				const escaped = pattern
					.replace(/[.+^${}()|[\]\\]/g, "\\$&")
					.replace(/\*\*/g, "§")
					.replace(/\*/g, "[^/]*")
					.replace(/§/g, ".*")
					.replace(/\?/g, "[^/]");
				new RegExp(`^${escaped}$`);
			} catch {
				errors.push(`WARNING: search.exclude_patterns: invalid glob pattern "${pattern}" — will be skipped`);
			}
		}
	}

	return { valid: true, errors: errors.length > 0 ? errors : undefined };
}

// ── Loader ──────────────────────────────────────────────────────────────

/**
 * Load plugin config from .axiom/plugin.yaml or .axiom/plugin.json.
 *
 * REQ-PLG-400: Load from .axiom/plugin.yaml (preferred) or .axiom/plugin.json.
 *              If neither exists, return loaded=false (L5 disabled silently).
 * REQ-PLG-402: Invalid config -> conversational error + fallback to defaults.
 * REQ-PLG-403: Unknown version -> warning + fallback to defaults.
 *
 * @param repoRoot - Absolute path to the repository root
 * @returns PluginConfigResult
 */
export function loadPluginConfig(repoRoot: string): PluginConfigResult {
	const yamlPath = join(repoRoot, ".axiom", "plugin.yaml");
	const jsonPath = join(repoRoot, ".axiom", "plugin.json");

	// REQ-PLG-400: prefer YAML over JSON
	let filePath: string | null = null;
	let source: string | null = null;

	if (existsSync(yamlPath)) {
		filePath = yamlPath;
		source = "plugin.yaml";
	} else if (existsSync(jsonPath)) {
		filePath = jsonPath;
		source = "plugin.json";
	}

	// REQ-PLG-400: no config file -> L5 disabled silently
	if (filePath === null || source === null) {
		return { loaded: false };
	}

	// Read the file
	let rawContent: string;
	try {
		rawContent = readFileSync(filePath, "utf-8");
	} catch (err) {
		const msg = err instanceof Error ? err.message : String(err);
		return {
			loaded: false,
			error: `Could not read ${source}: ${msg}. L5/L6 automation is disabled. Fix the file and restart your session.`,
		};
	}

	// Parse the file
	let parsed: unknown;
	try {
		if (source === "plugin.yaml") {
			parsed = parseYaml(rawContent);
		} else {
			parsed = JSON.parse(rawContent);
		}
	} catch (err) {
		const msg = err instanceof Error ? err.message : String(err);
		return {
			loaded: false,
			error: `Could not parse ${source}: ${msg}. Check the file syntax and try again. L5/L6 automation is disabled until the config is valid.`,
		};
	}

	// Must be an object
	if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
		return {
			loaded: false,
			error: `${source} must contain a YAML/JSON object (not ${parsed === null ? "null" : Array.isArray(parsed) ? "array" : typeof parsed}). L5/L6 automation is disabled.`,
		};
	}

	const config = parsed as PluginConfig;

	// REQ-PLG-402: validate
	const validation = validatePluginConfig(config);
	if (!validation.valid) {
		return {
			loaded: false,
			error: `Invalid ${source}: ${validation.errors?.join("; ")}. L5/L6 automation is disabled until the config is valid.`,
		};
	}

	// REQ-PLG-403: version gating
	if (config.version !== SUPPORTED_VERSION) {
		// Unknown version -> warning + fallback to defaults
		return {
			loaded: true,
			source,
			config: {
				...DEFAULT_PLUGIN_CONFIG,
				version: config.version,
			},
			warning: `Unknown config version ${config.version} in ${source}. Only version 1 is supported. Falling back to defaults (L5/L6 disabled).`,
		};
	}

	// Valid config — return as-is
	return {
		loaded: true,
		source,
		config,
	};
}

// ── REQ-PLG-400a: Profile-based config scoping ───────────────────────────

/**
 * Resolve the active profile and merge it with the base config.
 *
 * REQ-PLG-400a: Active profile selected via AXIOM_PROFILE env var.
 * Merge strategy: default config first, then profile hooks appended.
 * Unknown profiles produce a warning and fall back to default.
 */
export function resolveActiveProfile(
	config: PluginConfig,
	env: Record<string, string | undefined> = process.env as Record<
		string,
		string | undefined
	>,
): { config: PluginConfig; profileName: string | null; warning?: string } {
	const profileName = env.AXIOM_PROFILE ?? null;

	if (!profileName) {
		return { config, profileName: null };
	}

	const profiles = config.profiles ?? {};
	const profile = profiles[profileName];

	if (!profile) {
		return {
			config,
			profileName,
			warning: `Unknown profile "${profileName}" (AXIOM_PROFILE). Available profiles: ${Object.keys(profiles).join(", ") || "(none)"}. Falling back to default config.`,
		};
	}

	// Merge: default config first, then profile overrides appended
	const merged: PluginConfig = {
		...config,
		hooks: _mergeHooks(config.hooks, profile.hooks),
		watches: [...(config.watches ?? []), ...(profile.watches ?? [])],
		scripts: { ...(config.scripts ?? {}), ...(profile.scripts ?? {}) },
	};

	return { config: merged, profileName };
}

/** Merge two HooksConfig objects by appending profile hooks to default hooks. */
function _mergeHooks(
	base: HooksConfig | undefined,
	override: HooksConfig | undefined,
): HooksConfig | undefined {
	if (!base && !override) return undefined;
	if (!base) return override;
	if (!override) return base;

	const events: (keyof HooksConfig)[] = [
		"on_session_start",
		"on_session_end",
		"on_session_idle",
		"on_message",
		"on_command",
		"on_error",
	];

	const merged: HooksConfig = { ...base };
	for (const event of events) {
		const baseHooks = base[event] ?? [];
		const overrideHooks = override[event] ?? [];
		if (baseHooks.length > 0 || overrideHooks.length > 0) {
			merged[event] = [...baseHooks, ...overrideHooks];
		}
	}
	return merged;
}
