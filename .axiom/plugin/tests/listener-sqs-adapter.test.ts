/**
 * Tests for the SQS adapter.
 *
 * REQ-LNP-001 through REQ-LNP-005.
 * Uses a mock SQS client to avoid real AWS calls.
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#3.1
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-6/task-6.7
 */
import { describe, expect, it } from "bun:test";
import { createSqsAdapter } from "../src/layers/listeners/adapters/sqs";
import type { SqsClient, SqsMessage } from "../src/layers/listeners/adapters/sqs";
import type { EventEnvelope } from "../src/layers/listeners/types";

// ── Mock SQS Client ──────────────────────────────────────────────────────

function makeMockSqsClient(messages: SqsMessage[]): SqsClient & { deletedHandles: string[] } {
	const deletedHandles: string[] = [];
	let callCount = 0;

	return {
		deletedHandles,
		async receiveMessages(_params) {
			callCount++;
			// Return messages only on first call, then empty
			if (callCount === 1) return messages;
			return [];
		},
		async deleteMessage(params) {
			if (params.ReceiptHandle) {
				deletedHandles.push(params.ReceiptHandle);
			}
		},
	};
}

// ── Tests ────────────────────────────────────────────────────────────────

describe("createSqsAdapter — REQ-LNP-001 through REQ-LNP-005", () => {
	it("starts and stops cleanly (REQ-LNP-001)", async () => {
		const mockClient = makeMockSqsClient([]);
		const adapter = createSqsAdapter(
			{
				listenerName: "test-sqs",
				config: {
					queue_url: "https://sqs.us-east-1.amazonaws.com/123/test",
					region: "us-east-1",
				},
			},
			mockClient,
		);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });
		await adapter.stop();

		expect(adapter.isHealthy()).toBe(false); // stopped
	});

	it("receives and delivers SQS messages (REQ-LNP-003)", async () => {
		const messages: SqsMessage[] = [
			{
				MessageId: "msg-1",
				ReceiptHandle: "handle-1",
				Body: JSON.stringify({ event_type: "jira:issue_updated", key: "ABC-123" }),
			},
		];
		const mockClient = makeMockSqsClient(messages);

		const adapter = createSqsAdapter(
			{
				listenerName: "test-sqs",
				config: {
					queue_url: "https://sqs.us-east-1.amazonaws.com/123/test",
					region: "us-east-1",
					poll_interval_seconds: 60, // long interval so we only poll once
				},
			},
			mockClient,
		);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });
		await adapter.stop();

		expect(events).toHaveLength(1);
		const payload = events[0].payload as Record<string, unknown>;
		expect(payload.event_type).toBe("jira:issue_updated");
	});

	it("deletes message on successful processing (REQ-LNP-003)", async () => {
		const messages: SqsMessage[] = [
			{
				MessageId: "msg-1",
				ReceiptHandle: "handle-1",
				Body: JSON.stringify({ test: true }),
			},
		];
		const mockClient = makeMockSqsClient(messages);

		const adapter = createSqsAdapter(
			{
				listenerName: "test-sqs",
				config: {
					queue_url: "https://sqs.us-east-1.amazonaws.com/123/test",
					region: "us-east-1",
				},
			},
			mockClient,
		);

		await adapter.start(async (_e) => {});
		await adapter.stop();

		expect(mockClient.deletedHandles).toContain("handle-1");
	});

	it("does not delete message when processing fails (REQ-LNP-003)", async () => {
		const messages: SqsMessage[] = [
			{
				MessageId: "msg-1",
				ReceiptHandle: "handle-1",
				Body: JSON.stringify({ test: true }),
			},
		];
		const mockClient = makeMockSqsClient(messages);

		const adapter = createSqsAdapter(
			{
				listenerName: "test-sqs",
				config: {
					queue_url: "https://sqs.us-east-1.amazonaws.com/123/test",
					region: "us-east-1",
				},
			},
			mockClient,
		);

		await adapter.start(async (_e) => {
			throw new Error("Processing failed");
		});
		await adapter.stop();

		// Message should NOT be deleted when processing fails
		expect(mockClient.deletedHandles).not.toContain("handle-1");
	});

	it("handles non-JSON message body gracefully", async () => {
		const messages: SqsMessage[] = [
			{
				MessageId: "msg-1",
				ReceiptHandle: "handle-1",
				Body: "plain text message",
			},
		];
		const mockClient = makeMockSqsClient(messages);

		const adapter = createSqsAdapter(
			{
				listenerName: "test-sqs",
				config: {
					queue_url: "https://sqs.us-east-1.amazonaws.com/123/test",
					region: "us-east-1",
				},
			},
			mockClient,
		);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });
		await adapter.stop();

		expect(events).toHaveLength(1);
		const payload = events[0].payload as Record<string, unknown>;
		expect(payload.raw).toBe("plain text message");
	});

	it("sets source_type to 'sqs'", async () => {
		const messages: SqsMessage[] = [
			{
				MessageId: "msg-1",
				ReceiptHandle: "handle-1",
				Body: JSON.stringify({ test: true }),
			},
		];
		const mockClient = makeMockSqsClient(messages);

		const adapter = createSqsAdapter(
			{
				listenerName: "test-sqs",
				config: {
					queue_url: "https://sqs.us-east-1.amazonaws.com/123/test",
					region: "us-east-1",
				},
			},
			mockClient,
		);

		const events: EventEnvelope[] = [];
		await adapter.start(async (e) => { events.push(e); });
		await adapter.stop();

		expect(events[0].source_type).toBe("sqs");
		expect(events[0].listener_name).toBe("test-sqs");
	});
});
