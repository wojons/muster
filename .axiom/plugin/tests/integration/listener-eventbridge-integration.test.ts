/**
 * Tier 3 EventBridge integration tests — requires LocalStack.
 * Run: docker compose -f docker-compose.listen-notify-test.yml up -d --wait
 * Then: AWS_ENDPOINT_URL=http://localhost:4566 bun test tests/integration/listener-eventbridge-integration.test.ts
 *
 * These tests use skipIfServicesUnavailable() and will SKIP (not fail) when
 * Docker services are not running. Safe to include in CI without Docker.
 *
 * axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-120 plan=phase-2/task-2.4 jira_ref=DEX-434
 */
import { describe, it, expect, beforeAll } from "bun:test";
import { createEventBridgeAdapter } from "../../src/layers/listeners/adapters/eventbridge";
import {
	AWS_ENDPOINT_URL,
	AWS_REGION,
	skipIfServicesUnavailable,
	skipIfPackageUnavailable,
} from "./helpers/wait-for-services";
import type { EventEnvelope } from "../../src/layers/listeners/types";

// Configure AWS SDK to use LocalStack
// REQ-LNP-123: Credentials inherited from ambient environment; test uses LocalStack dummy creds
process.env.AWS_ACCESS_KEY_ID = "test";
process.env.AWS_SECRET_ACCESS_KEY = "test";
process.env.AWS_ENDPOINT_URL = AWS_ENDPOINT_URL;

describe("EventBridge Tier 3 integration tests (REQ-LNP-120)", () => {
	let skip = false;

	beforeAll(async () => {
		skip = await skipIfServicesUnavailable([
			{ name: "LocalStack", host: "localhost", port: 4566 },
		]) || await skipIfPackageUnavailable("@aws-sdk/client-sqs");
	});

	it("managed EventBridge rule: create rule → send event → verify SQS delivery (REQ-LNP-120)", async () => {
		if (skip) return;
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-120 plan=phase-2/task-2.4 test=listener-eventbridge-integration.test.ts jira_ref=DEX-434

		const ruleName = `test-rule-${Date.now()}`;
		const events: EventEnvelope[] = [];

		const adapter = createEventBridgeAdapter({
			listenerName: "eb-integration-test",
			config: {
				event_bus_name: "default",
				rule_name: ruleName,
				event_pattern: JSON.stringify({ source: ["test.integration"] }),
				region: AWS_REGION,
				managed: true,
			},
		});

		await adapter.start(async (e) => {
			events.push(e);
		});
		expect(adapter.isHealthy()).toBe(true);

		// Send a test event via EventBridge
		const { EventBridgeClient, PutEventsCommand } = await import(
			"@aws-sdk/client-eventbridge"
		);
		const ebClient = new EventBridgeClient({
			region: AWS_REGION,
			endpoint: AWS_ENDPOINT_URL,
			credentials: { accessKeyId: "test", secretAccessKey: "test" },
		});

		await ebClient.send(
			new PutEventsCommand({
				Entries: [
					{
						EventBusName: "default",
						Source: "test.integration",
						DetailType: "TestEvent",
						Detail: JSON.stringify({
							message: "hello from eventbridge",
							value: 123,
						}),
					},
				],
			}),
		);

		// Wait for SQS polling to pick up the event (SQS long-poll interval)
		await new Promise((r) => setTimeout(r, 8000));

		expect(events.length).toBeGreaterThan(0);
		expect(events[0].source_type).toBe("eventbridge");
		// REQ-LNP-124: EventBridge envelope normalization — detail is the primary payload
		expect(events[0].payload).toMatchObject({
			message: "hello from eventbridge",
			value: 123,
		});
		expect(events[0].listener_name).toBe("eb-integration-test");
		expect(events[0].id).toMatch(/^evt_/);
		expect(events[0].received_at).toBeTruthy();

		await adapter.stop();
	}, 30_000);

	it("EventBridge cleanup removes rule and queue on stop (REQ-LNP-122)", async () => {
		if (skip) return;
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-122 plan=phase-2/task-2.4 test=listener-eventbridge-integration.test.ts jira_ref=DEX-434

		const ruleName = `test-cleanup-${Date.now()}`;

		const adapter = createEventBridgeAdapter({
			listenerName: "eb-cleanup-test",
			config: {
				event_bus_name: "default",
				rule_name: ruleName,
				event_pattern: JSON.stringify({ source: ["test.cleanup"] }),
				region: AWS_REGION,
				managed: true,
			},
		});

		await adapter.start(async () => {});
		expect(adapter.isHealthy()).toBe(true);

		// Stop — should clean up rule and queue (REQ-LNP-122)
		await adapter.stop();

		// Verify rule no longer exists
		const { EventBridgeClient, DescribeRuleCommand } = await import(
			"@aws-sdk/client-eventbridge"
		);
		const ebClient = new EventBridgeClient({
			region: AWS_REGION,
			endpoint: AWS_ENDPOINT_URL,
			credentials: { accessKeyId: "test", secretAccessKey: "test" },
		});

		try {
			await ebClient.send(
				new DescribeRuleCommand({
					Name: ruleName,
					EventBusName: "default",
				}),
			);
			// If we get here, the rule still exists — test fails
			expect(false).toBe(true); // force failure: rule should have been deleted
		} catch (err) {
			// Expected: rule should not exist after cleanup (REQ-LNP-122)
			const msg = err instanceof Error ? err.message : String(err);
			expect(msg).toMatch(
				/ResourceNotFoundException|does not exist|not found/i,
			);
		}
	}, 30_000);

	it("EventBridge: isHealthy() returns false after stop (REQ-LNP-120)", async () => {
		if (skip) return;
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-120 plan=phase-2/task-2.4 test=listener-eventbridge-integration.test.ts jira_ref=DEX-434

		const ruleName = `test-health-${Date.now()}`;

		const adapter = createEventBridgeAdapter({
			listenerName: "eb-health-test",
			config: {
				event_bus_name: "default",
				rule_name: ruleName,
				event_pattern: JSON.stringify({ source: ["test.health"] }),
				region: AWS_REGION,
				managed: true,
			},
		});

		await adapter.start(async () => {});
		expect(adapter.isHealthy()).toBe(true);

		await adapter.stop();
		expect(adapter.isHealthy()).toBe(false);
	}, 30_000);
});
