/**
 * Agent listener API tools — tools for agents to create, modify, and tear down
 * listeners during a conversation.
 *
 * REQ-LNP-020: Script-based listeners for agent self-tracking.
 * REQ-LNP-048: Self-routing for agent-built listeners.
 * REQ-LNP-057: Dynamic delivery mode switching via codeops_update_listener.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#2 (Agent listener API)
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-4/task-4.1
 */

import type { ListenerEntry } from "../../config-loader";
import type { ListenerManager } from "./lifecycle";
import type { ListenerState } from "./types";

// ── Types ────────────────────────────────────────────────────────────────

/** Result of a create listener operation. */
export interface CreateListenerResult {
	success: boolean;
	name?: string;
	error?: string;
}

/** Result of an update listener operation. */
export interface UpdateListenerResult {
	success: boolean;
	error?: string;
}

/** Result of a delete listener operation. */
export interface DeleteListenerResult {
	success: boolean;
	error?: string;
}

// ── Tool Implementations ─────────────────────────────────────────────────

/**
 * Create a new listener from agent-provided config.
 * Returns a formatted string result for the agent.
 */
export async function executeCreateListener(
	manager: ListenerManager,
	sessionId: string,
	args: {
		name: string;
		source: string;
		config: string;
		delivery?: string;
		routing?: string;
	},
): Promise<string> {
	// Validate source type
	const validSources = ["sqs", "http", "script"];
	if (!validSources.includes(args.source)) {
		return `Invalid source type "${args.source}". Valid types: ${validSources.join(", ")}`;
	}

	// Parse config JSON
	let listenerConfig: unknown;
	try {
		listenerConfig = JSON.parse(args.config);
	} catch {
		return `Invalid config JSON: ${args.config}. Please provide valid JSON.`;
	}

	// Parse routing JSON if provided
	let routing: ListenerEntry["routing"] | undefined;
	if (args.routing) {
		try {
			routing = JSON.parse(args.routing);
		} catch {
			return `Invalid routing JSON: ${args.routing}. Please provide valid JSON.`;
		}
	}

	// Default routing to "self" for agent-built listeners (REQ-LNP-048)
	if (!routing) {
		routing = { match: { self: "true" } };
	}

	const entry: ListenerEntry = {
		name: args.name,
		enabled: true,
		source: args.source as ListenerEntry["source"],
		config: listenerConfig as ListenerEntry["config"],
		delivery: (args.delivery as ListenerEntry["delivery"]) ?? "non-interruptive",
		routing,
		lifecycle: {
			auto_start: true,
			auto_stop_on_session_end: true,
		},
	};

	const result = await manager.startListener(entry, sessionId);

	if (!result.success) {
		return `Failed to create listener "${args.name}": ${result.error}`;
	}

	return [
		`Listener "${args.name}" created and started.`,
		`Source: ${args.source}`,
		`Delivery mode: ${entry.delivery}`,
		`Routing: ${JSON.stringify(routing)}`,
		`The listener will automatically stop when this session ends.`,
	].join("\n");
}

/**
 * List all active listeners.
 * Returns a formatted string result for the agent.
 */
export function executeListListeners(
	manager: ListenerManager,
	format: "json" | "text" = "text",
): string {
	const states = manager.getAllStates();

	if (states.length === 0) {
		return "No active listeners.";
	}

	if (format === "json") {
		return JSON.stringify(
			states.map((s) => ({
				name: s.name,
				status: s.status,
				source: s.config.source,
				delivery: s.config.delivery ?? "non-interruptive",
				events_received: s.events_received,
				events_delivered: s.events_delivered,
				events_dropped: s.events_dropped,
				started_at: s.started_at,
				creator_session_id: s.creator_session_id,
			})),
			null,
			2,
		);
	}

	return states
		.map((s) => formatListenerState(s))
		.join("\n---\n");
}

/**
 * Update a listener's delivery mode at runtime.
 * REQ-LNP-057: Dynamic delivery mode switching.
 */
export function executeUpdateListener(
	manager: ListenerManager,
	args: {
		name: string;
		delivery?: string;
	},
): string {
	if (!args.delivery) {
		return `No updates specified for listener "${args.name}".`;
	}

	const validModes = ["interruptive", "non-interruptive"];
	if (!validModes.includes(args.delivery)) {
		return `Invalid delivery mode "${args.delivery}". Valid modes: ${validModes.join(", ")}`;
	}

	const success = manager.updateDeliveryMode(
		args.name,
		args.delivery as "interruptive" | "non-interruptive",
	);

	if (!success) {
		return `Listener "${args.name}" not found.`;
	}

	return `Listener "${args.name}" updated: delivery mode set to "${args.delivery}".`;
}

/**
 * Delete (stop) a listener.
 */
export async function executeDeleteListener(
	manager: ListenerManager,
	name: string,
): Promise<string> {
	const state = manager.getState(name);
	if (!state) {
		return `Listener "${name}" not found.`;
	}

	await manager.stopListener(name);
	return `Listener "${name}" stopped and removed.`;
}

// ── Helpers ──────────────────────────────────────────────────────────────

/** Format a listener state for human-readable output. */
function formatListenerState(state: ListenerState): string {
	const lines = [
		`Name: ${state.name}`,
		`Status: ${state.status}`,
		`Source: ${state.config.source}`,
		`Delivery: ${state.config.delivery ?? "non-interruptive"}`,
		`Events: received=${state.events_received} delivered=${state.events_delivered} dropped=${state.events_dropped}`,
	];
	if (state.started_at) lines.push(`Started: ${state.started_at}`);
	if (state.last_error) lines.push(`Last error: ${state.last_error}`);
	return lines.join("\n");
}
