/**
 * Tier 3 NATS integration tests — requires real NATS server.
 * Run: docker compose -f docker-compose.listen-notify-test.yml up -d --wait
 * Then: NATS_URL=nats://localhost:4222 bun test tests/integration/listener-nats-integration.test.ts
 *
 * These tests use skipIfServicesUnavailable() and will SKIP (not fail) when
 * Docker services are not running. Safe to include in CI without Docker.
 *
 * axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-100 plan=phase-2/task-2.2 jira_ref=DEX-434
 */
import { describe, it, expect, beforeAll } from "bun:test";
import { createNatsAdapter } from "../../src/layers/listeners/adapters/nats";
import { NATS_URL, skipIfServicesUnavailable, skipIfPackageUnavailable } from "./helpers/wait-for-services";
import type { EventEnvelope } from "../../src/layers/listeners/types";

describe("NATS Tier 3 integration tests (REQ-LNP-100)", () => {
	let skip = false;

	beforeAll(async () => {
		skip = await skipIfServicesUnavailable([
			{ name: "NATS", host: "localhost", port: 4222 },
		]) || await skipIfPackageUnavailable("nats");
	});

	it("core NATS: publish message → event delivered to adapter (REQ-LNP-100)", async () => {
		if (skip) return;
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-100 plan=phase-2/task-2.2 test=listener-nats-integration.test.ts jira_ref=DEX-434

		const subject = `test.integration.${Date.now()}`;
		const events: EventEnvelope[] = [];

		const adapter = createNatsAdapter({
			listenerName: "nats-integration-test",
			config: {
				servers: [NATS_URL],
				subject,
			},
		});

		await adapter.start(async (e) => {
			events.push(e);
		});
		expect(adapter.isHealthy()).toBe(true);

		// Publish a message using the NATS client directly
		const { connect } = await import("nats");
		const nc = await connect({ servers: NATS_URL });
		nc.publish(
			subject,
			new TextEncoder().encode(JSON.stringify({ test: "integration", value: 42 })),
		);
		await nc.flush();
		await nc.close();

		// Wait for delivery
		await new Promise((r) => setTimeout(r, 500));

		expect(events).toHaveLength(1);
		expect(events[0].source_type).toBe("nats");
		expect(events[0].payload).toMatchObject({ test: "integration", value: 42 });
		expect(events[0].listener_name).toBe("nats-integration-test");
		expect(events[0].id).toMatch(/^evt_/);
		expect(events[0].received_at).toBeTruthy();

		await adapter.stop();
	}, 10_000);

	it("core NATS: non-JSON message delivered as { raw: string } (REQ-LNP-100)", async () => {
		if (skip) return;
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-100 plan=phase-2/task-2.2 test=listener-nats-integration.test.ts jira_ref=DEX-434

		const subject = `test.raw.${Date.now()}`;
		const events: EventEnvelope[] = [];

		const adapter = createNatsAdapter({
			listenerName: "nats-raw-test",
			config: {
				servers: [NATS_URL],
				subject,
			},
		});

		await adapter.start(async (e) => {
			events.push(e);
		});

		const { connect } = await import("nats");
		const nc = await connect({ servers: NATS_URL });
		nc.publish(subject, new TextEncoder().encode("not-json-at-all"));
		await nc.flush();
		await nc.close();

		await new Promise((r) => setTimeout(r, 500));

		expect(events).toHaveLength(1);
		expect(events[0].source_type).toBe("nats");
		// Non-JSON payloads are wrapped as { raw: string }
		expect(events[0].payload).toMatchObject({ raw: "not-json-at-all" });

		await adapter.stop();
	}, 10_000);

	it("NATS JetStream: consumer resumes from last offset after restart (REQ-LNP-102)", async () => {
		if (skip) return;
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-102 plan=phase-2/task-2.2 test=listener-nats-integration.test.ts jira_ref=DEX-434

		const streamName = `TEST_STREAM_${Date.now()}`;
		const subject = `test.js.${Date.now()}`;
		const consumerName = `test-consumer-${Date.now()}`;

		// Create stream and publish 3 messages
		const { connect } = await import("nats");
		const nc = await connect({ servers: NATS_URL });
		const js = nc.jetstream();
		const jsm = await nc.jetstreamManager();
		await jsm.streams.add({ name: streamName, subjects: [subject] });

		for (let i = 1; i <= 3; i++) {
			await js.publish(
				subject,
				new TextEncoder().encode(JSON.stringify({ seq: i })),
			);
		}

		// Start adapter, receive messages, then stop
		const events1: EventEnvelope[] = [];
		const adapter1 = createNatsAdapter({
			listenerName: "nats-js-resume-test",
			config: {
				servers: [NATS_URL],
				subject,
				jetstream: true,
				stream: streamName,
				consumer: consumerName,
				deliver_policy: "all",
			},
		});

		await adapter1.start(async (e) => {
			events1.push(e);
		});
		await new Promise((r) => setTimeout(r, 1500));
		await adapter1.stop();

		// Should have received all 3 messages
		expect(events1.length).toBeGreaterThanOrEqual(3);

		// Publish 2 more messages
		await js.publish(
			subject,
			new TextEncoder().encode(JSON.stringify({ seq: 4 })),
		);
		await js.publish(
			subject,
			new TextEncoder().encode(JSON.stringify({ seq: 5 })),
		);

		// Restart adapter with same consumer name — should resume from where it left off
		const events2: EventEnvelope[] = [];
		const adapter2 = createNatsAdapter({
			listenerName: "nats-js-resume-test",
			config: {
				servers: [NATS_URL],
				subject,
				jetstream: true,
				stream: streamName,
				consumer: consumerName,
				deliver_policy: "all",
			},
		});

		await adapter2.start(async (e) => {
			events2.push(e);
		});
		await new Promise((r) => setTimeout(r, 1500));
		await adapter2.stop();

		// Should have received the 2 new messages (not re-delivered the first 3)
		const seqs2 = events2.map(
			(e) => (e.payload as { seq: number }).seq,
		);
		expect(seqs2).toContain(4);
		expect(seqs2).toContain(5);
		// Must NOT re-deliver already-acked messages
		expect(seqs2).not.toContain(1);
		expect(seqs2).not.toContain(2);
		expect(seqs2).not.toContain(3);

		// Cleanup
		await jsm.streams.delete(streamName);
		await nc.close();
	}, 30_000);

	it("NATS: adapter stops cleanly and isHealthy() returns false after stop (REQ-LNP-100)", async () => {
		if (skip) return;
		// axiom:trace work_item=listen-notify-03 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-100 plan=phase-2/task-2.2 test=listener-nats-integration.test.ts jira_ref=DEX-434

		const subject = `test.stop.${Date.now()}`;

		const adapter = createNatsAdapter({
			listenerName: "nats-stop-test",
			config: {
				servers: [NATS_URL],
				subject,
			},
		});

		await adapter.start(async () => {});
		expect(adapter.isHealthy()).toBe(true);

		await adapter.stop();
		expect(adapter.isHealthy()).toBe(false);
	}, 10_000);
});
