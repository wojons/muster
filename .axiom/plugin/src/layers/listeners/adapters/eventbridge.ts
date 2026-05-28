/**
 * EventBridge source adapter for the Listen/Notify Plugin (L7).
 *
 * Implements EventBridge → SQS fan-out pattern (OD-LNP-08 resolved):
 * - When managed=true: provisions an EventBridge rule + SQS queue, then delegates
 *   all polling to the SQS adapter (REQ-LNP-126).
 * - When managed=false or target_queue_url provided: uses the given queue directly.
 *
 * REQ-LNP-120: EventBridge listener via SQS fan-out.
 * REQ-LNP-121: Configuration parameters (event_bus_name, rule_name, event_pattern, etc.).
 * REQ-LNP-122: Managed provisioning — create SQS queue + EventBridge rule + policy.
 * REQ-LNP-123: Inherit AWS credentials from ambient environment.
 * REQ-LNP-124: Normalize EventBridge envelope — extract detail as primary payload.
 * REQ-LNP-125: IAM error handling — AccessDenied → AUTH_FAILED + degraded state.
 * REQ-LNP-126: Thin wrapper over SQS adapter — reuse all SQS polling logic.
 * REQ-LNP-127: Emit listener.eventbridge_rule_created / listener.eventbridge_rule_deleted.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#13.3
 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-120 plan=phase-3/task-3.2
 */

import type { EventBridgeListenerConfig } from "../../../config-loader";
import { createSqsAdapter } from "./sqs";
import type { EventCallback, ListenerAdapter } from "../types";
import { emitListenerLog } from "../types";

// ── Types ────────────────────────────────────────────────────────────────

/** Options for creating an EventBridge adapter. */
export interface EventBridgeAdapterOptions {
	listenerName: string;
	config: EventBridgeListenerConfig;
}

// ── AWS Client Interfaces (for testability) ──────────────────────────────

/** Minimal SQS management client interface (allows mocking in tests). */
export interface SqsManagementClient {
	createQueue(params: { QueueName: string }): Promise<{ QueueUrl: string }>;
	getQueueAttributes(params: {
		QueueUrl: string;
		AttributeNames: string[];
	}): Promise<{ Attributes?: Record<string, string> }>;
	setQueueAttributes(params: {
		QueueUrl: string;
		Attributes: Record<string, string>;
	}): Promise<void>;
	deleteQueue(params: { QueueUrl: string }): Promise<void>;
}

/** Minimal EventBridge management client interface (allows mocking in tests). */
export interface EventBridgeManagementClient {
	putRule(params: {
		Name: string;
		EventBusName: string;
		EventPattern: string;
		State: string;
		Description?: string;
	}): Promise<void>;
	putTargets(params: {
		Rule: string;
		EventBusName: string;
		Targets: Array<{ Id: string; Arn: string }>;
	}): Promise<void>;
	removeTargets(params: {
		Rule: string;
		EventBusName: string;
		Ids: string[];
	}): Promise<void>;
	deleteRule(params: {
		Name: string;
		EventBusName: string;
	}): Promise<void>;
}

// ── Implementation ───────────────────────────────────────────────────────

/**
 * Create an EventBridge listener adapter.
 *
 * REQ-LNP-120: EventBridge → SQS fan-out. Provisions managed resources when
 * managed=true, then delegates all polling to the SQS adapter (REQ-LNP-126).
 *
 * @param options - Adapter options (listenerName + config).
 * @param sqsMgmtClientOverride - Optional SQS management client override for testing.
 * @param ebClientOverride - Optional EventBridge management client override for testing.
 */
export function createEventBridgeAdapter(
	options: EventBridgeAdapterOptions,
	/** Optional SQS management client override for testing (REQ-LNP-122). */
	sqsMgmtClientOverride?: SqsManagementClient,
	/** Optional EventBridge management client override for testing (REQ-LNP-122). */
	ebClientOverride?: EventBridgeManagementClient,
): ListenerAdapter {
	const { listenerName, config } = options;
	const {
		event_bus_name,
		rule_name,
		event_pattern,
		target_queue_url,
		region,
		managed = true,
	} = config;

	let managedQueueUrl: string | null = null;
	let sqsAdapter: ListenerAdapter | null = null;

	// ── AWS Client Factories ─────────────────────────────────────────────

	/** REQ-LNP-123: Inherit AWS credentials from ambient environment. */
	async function getSqsMgmtClient(): Promise<SqsManagementClient> {
		if (sqsMgmtClientOverride) return sqsMgmtClientOverride;

		try {
			const sqsPkg = "@aws-sdk/client-sqs";
			const { SQSClient, CreateQueueCommand, GetQueueAttributesCommand, SetQueueAttributesCommand, DeleteQueueCommand } =
				await import(sqsPkg);

			const client = new SQSClient({ region });

			return {
				async createQueue(params) {
					const result = await client.send(new CreateQueueCommand({ QueueName: params.QueueName }));
					return { QueueUrl: result.QueueUrl! };
				},
				async getQueueAttributes(params) {
					const result = await client.send(new GetQueueAttributesCommand({
						QueueUrl: params.QueueUrl,
						AttributeNames: params.AttributeNames,
					}));
					return { Attributes: result.Attributes as Record<string, string> | undefined };
				},
				async setQueueAttributes(params) {
					await client.send(new SetQueueAttributesCommand({
						QueueUrl: params.QueueUrl,
						Attributes: params.Attributes,
					}));
				},
				async deleteQueue(params) {
					await client.send(new DeleteQueueCommand({ QueueUrl: params.QueueUrl }));
				},
			};
		} catch (err) {
			const msg = err instanceof Error ? err.message : String(err);
			throw new Error(`@aws-sdk/client-sqs not available: ${msg}. Install it to use EventBridge listeners.`);
		}
	}

	/** REQ-LNP-123: Inherit AWS credentials from ambient environment. */
	async function getEbClient(): Promise<EventBridgeManagementClient> {
		if (ebClientOverride) return ebClientOverride;

		try {
			const ebPkg = "@aws-sdk/client-eventbridge";
			const { EventBridgeClient, PutRuleCommand, PutTargetsCommand, RemoveTargetsCommand, DeleteRuleCommand } =
				await import(ebPkg);

			const client = new EventBridgeClient({ region });

			return {
				async putRule(params) {
					await client.send(new PutRuleCommand({
						Name: params.Name,
						EventBusName: params.EventBusName,
						EventPattern: params.EventPattern,
						State: params.State as "ENABLED" | "DISABLED",
						Description: params.Description,
					}));
				},
				async putTargets(params) {
					await client.send(new PutTargetsCommand({
						Rule: params.Rule,
						EventBusName: params.EventBusName,
						Targets: params.Targets,
					}));
				},
				async removeTargets(params) {
					await client.send(new RemoveTargetsCommand({
						Rule: params.Rule,
						EventBusName: params.EventBusName,
						Ids: params.Ids,
					}));
				},
				async deleteRule(params) {
					await client.send(new DeleteRuleCommand({
						Name: params.Name,
						EventBusName: params.EventBusName,
					}));
				},
			};
		} catch (err) {
			const msg = err instanceof Error ? err.message : String(err);
			throw new Error(`@aws-sdk/client-eventbridge not available: ${msg}. Install it to use EventBridge listeners.`);
		}
	}

	// ── Provisioning ─────────────────────────────────────────────────────

	/**
	 * Provision managed resources (REQ-LNP-122).
	 * Creates SQS queue + EventBridge rule + queue policy when managed=true.
	 * Returns the queue URL to use for SQS polling.
	 *
	 * REQ-LNP-125: AccessDenied → AUTH_FAILED error.
	 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-122 plan=phase-3/task-3.2
	 */
	async function provision(): Promise<string> {
		// If a target queue URL is explicitly provided, skip provisioning entirely
		if (target_queue_url) return target_queue_url;
		// If managed=false and no target_queue_url, we cannot proceed
		if (!managed) {
			throw new Error(
				`EventBridge adapter "${listenerName}": managed=false requires target_queue_url to be set.`,
			);
		}

		try {
			const sqsMgmt = await getSqsMgmtClient();
			const ebMgmt = await getEbClient();

		// BL-006: Use crypto.randomUUID() for collision-resistant queue name suffix
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-122 plan=phase-1/task-1.4 jira_ref=DEX-434
		const queueName = `axiom-eventbridge-${rule_name}-${crypto.randomUUID().slice(0, 8)}`;
			const { QueueUrl: queueUrl } = await sqsMgmt.createQueue({ QueueName: queueName });

			// Get queue ARN for policy + EventBridge target
			const attrResult = await sqsMgmt.getQueueAttributes({
				QueueUrl: queueUrl,
				AttributeNames: ["QueueArn"],
			});
			const queueArn = attrResult.Attributes?.QueueArn;
			if (!queueArn) {
				throw new Error(`Could not retrieve ARN for queue ${queueName}`);
			}

			// REQ-LNP-122: Set queue policy to allow EventBridge to send messages
			const policy = JSON.stringify({
				Version: "2012-10-17",
				Statement: [
					{
						Effect: "Allow",
						Principal: { Service: "events.amazonaws.com" },
						Action: "sqs:SendMessage",
						Resource: queueArn,
					},
				],
			});
			await sqsMgmt.setQueueAttributes({
				QueueUrl: queueUrl,
				Attributes: { Policy: policy },
			});

			// REQ-LNP-122: Create EventBridge rule
			await ebMgmt.putRule({
				Name: rule_name,
				EventBusName: event_bus_name,
				EventPattern: event_pattern,
				State: "ENABLED",
				Description: config.rule_description,
			});

			// REQ-LNP-122: Add SQS queue as target
			await ebMgmt.putTargets({
				Rule: rule_name,
				EventBusName: event_bus_name,
				Targets: [{ Id: "axiom-target", Arn: queueArn }],
			});

			// REQ-LNP-127: Emit rule created event
			emitListenerLog({
				event: "listener.eventbridge_rule_created",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
			});

			return queueUrl;
		} catch (err) {
			const msg = err instanceof Error ? err.message : String(err);
			// REQ-LNP-125: IAM AccessDenied → AUTH_FAILED error class
			if (msg.includes("AccessDenied") || msg.includes("AccessDeniedException")) {
				throw new Error(`AUTH_FAILED: ${msg}`);
			}
			throw err;
		}
	}

	/**
	 * Clean up managed resources on stop (REQ-LNP-122).
	 * Removes EventBridge rule + target + SQS queue.
	 *
	 * REQ-LNP-127: Emit listener.eventbridge_rule_deleted.
	 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-122 plan=phase-3/task-3.2
	 */
	async function cleanup(queueUrl: string): Promise<void> {
		if (!managed || target_queue_url) {
			// Unmanaged — do not delete resources we didn't create
			return;
		}
		try {
			const sqsMgmt = await getSqsMgmtClient();
			const ebMgmt = await getEbClient();

			// Remove target first, then delete rule, then delete queue
			await ebMgmt.removeTargets({
				Rule: rule_name,
				EventBusName: event_bus_name,
				Ids: ["axiom-target"],
			});
			await ebMgmt.deleteRule({
				Name: rule_name,
				EventBusName: event_bus_name,
			});
			await sqsMgmt.deleteQueue({ QueueUrl: queueUrl });

			// REQ-LNP-127: Emit rule deleted event
			emitListenerLog({
				event: "listener.eventbridge_rule_deleted",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
			});
		} catch (err) {
			// REQ-LNP-122: Non-fatal cleanup failure — log for operator visibility
			// BL-003: Log cleanup failures so operators can detect orphaned resources
			// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-122 plan=phase-1/task-1.3 jira_ref=DEX-434
			const msg = err instanceof Error ? err.message : String(err);
			emitListenerLog({
				event: "listener.error",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
				error: `EventBridge cleanup failed (best-effort): ${msg}`,
			});
		}
	}

	// ── Adapter ──────────────────────────────────────────────────────────

	return {
		/**
		 * Start the EventBridge adapter.
		 * Provisions managed resources (if needed), then delegates to SQS adapter.
		 *
		 * REQ-LNP-120: EventBridge → SQS fan-out.
		 * REQ-LNP-124: Normalize EventBridge envelope — extract detail as primary payload.
		 * REQ-LNP-125: AUTH_FAILED on AccessDenied.
		 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-120 plan=phase-3/task-3.2
		 */
		async start(onEvent: EventCallback): Promise<void> {
			try {
				const queueUrl = await provision();
				managedQueueUrl = queueUrl;

			/**
			 * REQ-LNP-124: Normalize EventBridge event envelope.
			 * Extract `detail` as primary payload; preserve full envelope in event.metadata.
			 * Non-EventBridge payloads (no `detail` key) pass through with source_type override.
			 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-124 plan=step-fix-eb-001
			 */
			const wrappedOnEvent: EventCallback = async (event) => {
				const payload = event.payload as Record<string, unknown> | null | undefined;
				if (payload !== null && payload !== undefined && typeof payload === "object" && "detail" in payload) {
					// REQ-LNP-124: Override source_type to 'eventbridge' for downstream routing
					const normalized = {
						...event,
						source_type: "eventbridge" as const,
						payload: payload.detail,
						metadata: payload,
					};
					return onEvent(normalized);
				}
				// Non-EventBridge payload — still override source_type for consistent adapter identity.
				// REQ-LNP-124: Downstream consumers MUST handle event.metadata being undefined
				// when source_type === "eventbridge" and no `detail` field was present.
				// axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-124 plan=step-da-002
				const passThrough = {
					...event,
					source_type: "eventbridge" as const,
					// metadata is intentionally NOT set for non-EventBridge payloads
				};
				return onEvent(passThrough);
			};

				// REQ-LNP-126: Delegate all SQS polling to the SQS adapter
				sqsAdapter = createSqsAdapter({
					listenerName,
					config: { queue_url: queueUrl, region },
				});
				await sqsAdapter.start(wrappedOnEvent);
			} catch (err) {
				const msg = err instanceof Error ? err.message : String(err);
				// REQ-LNP-125: Emit AUTH_FAILED error event
				emitListenerLog({
					event: "listener.error",
					listener_name: listenerName,
					timestamp: new Date().toISOString(),
					error: msg,
				});
				throw err;
			}
		},

		/**
		 * Stop the EventBridge adapter.
		 * Stops SQS polling, then cleans up managed resources.
		 *
		 * REQ-LNP-122: Clean up rule + queue on listener stop.
		 * axiom:trace work_item=listen-notify-02 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-122 plan=phase-3/task-3.2
		 */
		async stop(): Promise<void> {
			await sqsAdapter?.stop();
			if (managedQueueUrl) {
				await cleanup(managedQueueUrl);
				managedQueueUrl = null;
			}
		},

		/** Health check — delegates to underlying SQS adapter. */
		isHealthy(): boolean {
			return sqsAdapter?.isHealthy() ?? false;
		},

		/** Status description — delegates to underlying SQS adapter. */
		statusDescription(): string {
			return sqsAdapter?.statusDescription() ?? "stopped";
		},
	};
}
