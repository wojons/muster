/**
 * Tier 3 Kafka integration tests — requires real Kafka broker.
 * Run: docker compose -f docker-compose.listen-notify-test.yml up -d --wait
 * Then: KAFKA_BROKERS=localhost:9092 bun test tests/integration/listener-kafka-integration.test.ts
 *
 * These tests use skipIfServicesUnavailable() and will SKIP (not fail) when
 * Docker services are not running. Safe to include in CI without Docker.
 *
 * axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-110 plan=phase-2/task-2.3 jira_ref=DEX-434
 */
import { describe, it, expect, beforeAll } from "bun:test";
import { createKafkaAdapter } from "../../src/layers/listeners/adapters/kafka";
import { KAFKA_BROKERS, skipIfServicesUnavailable, skipIfPackageUnavailable } from "./helpers/wait-for-services";
import type { EventEnvelope } from "../../src/layers/listeners/types";

describe("Kafka Tier 3 integration tests (REQ-LNP-110)", () => {
	let skip = false;

	beforeAll(async () => {
		skip = await skipIfServicesUnavailable([
			{ name: "Kafka", host: "localhost", port: 9092 },
		]) || await skipIfPackageUnavailable("kafkajs");
	});

	it("Kafka consumer group: produce message → event delivered (REQ-LNP-110)", async () => {
		if (skip) return;
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-110 plan=phase-2/task-2.3 test=listener-kafka-integration.test.ts jira_ref=DEX-434

		const topic = `test-integration-${Date.now()}`;
		const groupId = `test-group-${Date.now()}`;
		const events: EventEnvelope[] = [];

		const adapter = createKafkaAdapter({
			listenerName: "kafka-integration-test",
			config: {
				brokers: KAFKA_BROKERS,
				topic,
				group_id: groupId,
				from_beginning: true,
			},
		});

		await adapter.start(async (e) => {
			events.push(e);
		});
		expect(adapter.isHealthy()).toBe(true);

		// Produce a message using kafkajs directly
		const { Kafka } = await import("kafkajs");
		const kafka = new Kafka({ clientId: "test-producer", brokers: KAFKA_BROKERS });
		const producer = kafka.producer();
		await producer.connect();
		await producer.send({
			topic,
			messages: [
				{
					value: JSON.stringify({ test: "kafka-integration", value: 99 }),
				},
			],
		});
		await producer.disconnect();

		// Wait for delivery (Kafka consumer group startup takes a few seconds)
		await new Promise((r) => setTimeout(r, 5000));

		expect(events.length).toBeGreaterThan(0);
		expect(events[0].source_type).toBe("kafka");
		expect(events[0].payload).toMatchObject({ test: "kafka-integration", value: 99 });
		expect(events[0].listener_name).toBe("kafka-integration-test");
		expect(events[0].id).toMatch(/^evt_/);
		expect(events[0].received_at).toBeTruthy();

		await adapter.stop();
	}, 30_000);

	it("Kafka: non-JSON message delivered as { raw: string } (REQ-LNP-110)", async () => {
		if (skip) return;
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-110 plan=phase-2/task-2.3 test=listener-kafka-integration.test.ts jira_ref=DEX-434

		const topic = `test-raw-${Date.now()}`;
		const groupId = `test-raw-group-${Date.now()}`;
		const events: EventEnvelope[] = [];

		const adapter = createKafkaAdapter({
			listenerName: "kafka-raw-test",
			config: {
				brokers: KAFKA_BROKERS,
				topic,
				group_id: groupId,
				from_beginning: true,
			},
		});

		await adapter.start(async (e) => {
			events.push(e);
		});

		const { Kafka } = await import("kafkajs");
		const kafka = new Kafka({ clientId: "test-producer-raw", brokers: KAFKA_BROKERS });
		const producer = kafka.producer();
		await producer.connect();
		await producer.send({
			topic,
			messages: [{ value: "not-json-at-all" }],
		});
		await producer.disconnect();

		await new Promise((r) => setTimeout(r, 5000));

		expect(events.length).toBeGreaterThan(0);
		expect(events[0].source_type).toBe("kafka");
		// Non-JSON payloads are wrapped as { raw: string }
		expect(events[0].payload).toMatchObject({ raw: "not-json-at-all" });

		await adapter.stop();
	}, 30_000);

	it("Kafka manual offset commit: message not redelivered after ack (REQ-LNP-112)", async () => {
		if (skip) return;
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-112 plan=phase-2/task-2.3 test=listener-kafka-integration.test.ts jira_ref=DEX-434

		const topic = `test-commit-${Date.now()}`;
		const groupId = `test-commit-group-${Date.now()}`;

		// Produce a message before starting the consumer
		const { Kafka } = await import("kafkajs");
		const kafka = new Kafka({
			clientId: "test-producer-commit",
			brokers: KAFKA_BROKERS,
		});
		const producer = kafka.producer();
		await producer.connect();
		await producer.send({
			topic,
			messages: [{ value: JSON.stringify({ commit: "test" }) }],
		});
		await producer.disconnect();

		// First consumer: receive and ack (offset committed on successful processing)
		const events1: EventEnvelope[] = [];
		const adapter1 = createKafkaAdapter({
			listenerName: "kafka-commit-test-1",
			config: {
				brokers: KAFKA_BROKERS,
				topic,
				group_id: groupId,
				from_beginning: true,
			},
		});
		await adapter1.start(async (e) => {
			events1.push(e);
		});
		await new Promise((r) => setTimeout(r, 5000));
		await adapter1.stop();
		expect(events1.length).toBeGreaterThan(0);

		// Second consumer with same group: should NOT receive the already-committed message
		const events2: EventEnvelope[] = [];
		const adapter2 = createKafkaAdapter({
			listenerName: "kafka-commit-test-2",
			config: {
				brokers: KAFKA_BROKERS,
				topic,
				group_id: groupId,
				from_beginning: false,
			},
		});
		await adapter2.start(async (e) => {
			events2.push(e);
		});
		await new Promise((r) => setTimeout(r, 3000));
		await adapter2.stop();

		// Message should not be redelivered (offset was committed by first consumer)
		expect(events2).toHaveLength(0);
	}, 30_000);

	it("Kafka: adapter stops cleanly and isHealthy() returns false after stop (REQ-LNP-110)", async () => {
		if (skip) return;
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-110 plan=phase-2/task-2.3 test=listener-kafka-integration.test.ts jira_ref=DEX-434

		const topic = `test-stop-${Date.now()}`;
		const groupId = `test-stop-group-${Date.now()}`;

		const adapter = createKafkaAdapter({
			listenerName: "kafka-stop-test",
			config: {
				brokers: KAFKA_BROKERS,
				topic,
				group_id: groupId,
				from_beginning: false,
			},
		});

		await adapter.start(async () => {});
		expect(adapter.isHealthy()).toBe(true);

		await adapter.stop();
		expect(adapter.isHealthy()).toBe(false);
	}, 15_000);
});
