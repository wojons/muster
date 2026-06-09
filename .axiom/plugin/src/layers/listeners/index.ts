/**
 * Listen/Notify Plugin subsystem (L7) — public API.
 *
 * Exports all types, factories, and utilities for the event listener subsystem.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-5/task-5.1
 */

// Config-loader types re-exported for consumers of this subsystem
export type { RunActionConfig, RunSurface } from "../../config-loader";

// Core types
export type {
	DeliveryMode,
	DeliveryStatus,
	EventCallback,
	EventEnvelope,
	EventId,
	ListenerAdapter,
	ListenerLogEntry,
	ListenerLogEvent,
	ListenerState,
	ListenerStatus,
} from "./types";
export {
	emitListenerLog,
	generateEventId,
	HEALTH_CHECK_INTERVAL_MS,
	INTERRUPTIVE_QUEUE_MAX,
	INTERRUPTIVE_RATE_LIMIT_MS,
	MAX_EVENTS_PER_INJECT,
	MAX_LISTENERS_PER_MACHINE,
	MAX_LISTENERS_PER_SESSION,
	MAX_PAYLOAD_BYTES,
	NON_INTERRUPTIVE_RATE_LIMIT_PER_MINUTE,
	ORPHAN_DETECTION_INTERVAL_MS,
	truncatePayload,
} from "./types";

// Session router
export type {
	RoutingResult,
	SessionInfo,
	SessionRouter,
	SessionRouterOptions,
} from "./router";
export {
	createSessionRouter,
	evaluateFilter,
	getNestedField,
	matchesRoutingRule,
	resolveVariables,
} from "./router";

// Context injection
export type {
	InjectionEngine,
	InjectionEngineOptions,
	InjectionResult,
	PendingInjection,
} from "./injection";
export { createInjectionEngine } from "./injection";

// Delivery queue
export type {
	DeliveryAttemptResult,
	DeliveryQueue,
	DeliveryQueueOptions,
	QueuedDelivery,
} from "./delivery";
export { createDeliveryQueue } from "./delivery";

// Lifecycle manager
export type {
	ListenerManager,
	ListenerManagerOptions,
	ListenerMetrics,
} from "./lifecycle";
export { createListenerManager, removeActiveSession, writeActiveSessionsFile, readActiveSessionsForRouter } from "./lifecycle";

// Adapters
export type { SqsAdapterOptions, SqsClient, SqsMessage } from "./adapters/sqs";
export { createSqsAdapter } from "./adapters/sqs";

export type { HttpAdapterOptions } from "./adapters/http";
export { createHttpAdapter } from "./adapters/http";

export type { ScriptAdapterOptions, ScriptValidationResult } from "./adapters/script";
export {
	buildSafeEnv,
	createScriptAdapter,
	validateScript,
} from "./adapters/script";

// Agent tools
export type {
	CreateListenerResult,
	DeleteListenerResult,
	UpdateListenerResult,
} from "./tools";
export {
	executeCreateListener,
	executeDeleteListener,
	executeListListeners,
	executeUpdateListener,
} from "./tools";
