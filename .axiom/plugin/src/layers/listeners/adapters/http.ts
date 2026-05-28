/**
 * HTTP webhook source adapter for the Listen/Notify Plugin.
 *
 * REQ-LNP-010: HTTP webhook listener support.
 * REQ-LNP-011: HTTP webhook configuration parameters.
 * REQ-LNP-012: Content-Type validation (must be application/json).
 * REQ-LNP-013: Async processing — HTTP 200 returned immediately.
 * REQ-LNP-014: Optional webhook verification (secret_token, HMAC-SHA256 signature).
 * REQ-LNP-015: Request body size limit (64KB).
 *
 * Spec: specs/88-Listen-Notify-Plugin.md#3.2
 * axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md plan=phase-3/task-3.2
 */

import { createHmac } from "node:crypto";
import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import type { HttpListenerConfig } from "../../../config-loader";
import type { EventCallback, ListenerAdapter } from "../types";
import {
	emitListenerLog,
	generateEventId,
	MAX_PAYLOAD_BYTES,
	truncatePayload,
} from "../types";

// ── Types ────────────────────────────────────────────────────────────────

/** Options for creating an HTTP webhook adapter. */
export interface HttpAdapterOptions {
	listenerName: string;
	config: HttpListenerConfig;
}

// ── Implementation ───────────────────────────────────────────────────────

/**
 * Create an HTTP webhook listener adapter.
 * REQ-LNP-010: Exposes an HTTP endpoint that accepts POST requests.
 */
export function createHttpAdapter(options: HttpAdapterOptions): ListenerAdapter {
	const { listenerName, config } = options;
	const {
		port,
		path,
		host = "127.0.0.1",
		secret_token,
		signature: signatureSecret,
	} = config;

	let server: Server | null = null;
	let healthy = false;
	let statusDesc = "stopped";
	let onEventCallback: EventCallback | null = null;

	function handleRequest(req: IncomingMessage, res: ServerResponse): void {
		// Only handle POST to the configured path
		if (req.method !== "POST" || req.url !== path) {
			res.writeHead(404);
			res.end("Not Found");
			return;
		}

		// REQ-LNP-012: Validate Content-Type
		const contentType = req.headers["content-type"] ?? "";
		if (!contentType.includes("application/json")) {
			emitListenerLog({
				event: "listener.error",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
				error: `Invalid Content-Type: ${contentType}`,
			});
			res.writeHead(415, { "Content-Type": "application/json" });
			res.end(JSON.stringify({ error: "Unsupported Media Type — expected application/json" }));
			return;
		}

		// Collect body BEFORE sending response to avoid race condition
		// REQ-LNP-013: Return 200 immediately after queueing for processing
		const chunks: Buffer[] = [];
		let bodySize = 0;
		let tooLarge = false;

		req.on("data", (chunk: Buffer) => {
			bodySize += chunk.length;
			// REQ-LNP-015: Limit request body to 64KB.
			// axiom:trace work_item=listen-notify-01 spec=specs/88-Listen-Notify-Plugin.md#REQ-LNP-015 plan=step-fix-004 test=listener-http-adapter.test.ts
			// Send 413 and destroy the socket immediately on first overflow chunk —
			// this prevents buffering the full body in memory before rejecting.
			if (bodySize > MAX_PAYLOAD_BYTES) {
				if (!tooLarge) {
					tooLarge = true;
					emitListenerLog({
						event: "listener.error",
						listener_name: listenerName,
						timestamp: new Date().toISOString(),
						error: `Request body exceeds 64KB limit — rejecting`,
					});
					res.writeHead(413, { "Content-Type": "application/json" });
					res.end(JSON.stringify({ error: "Payload Too Large" }));
					req.socket?.destroy();
				}
				return;
			}
			chunks.push(chunk);
		});

		req.on("end", () => {
			// REQ-LNP-013: Return 200 immediately after body is received.
			// 413 is already sent in the data handler; just bail out here.
			if (tooLarge) return;

			// Send 200 immediately — processing happens asynchronously below
			res.writeHead(200, { "Content-Type": "application/json" });
			res.end(JSON.stringify({ status: "accepted" }));

			const rawBody = Buffer.concat(chunks).toString("utf-8");

			// REQ-LNP-014: Webhook verification
			if (secret_token) {
				const tokenHeader = req.headers["x-webhook-token"];
				if (tokenHeader !== secret_token) {
					emitListenerLog({
						event: "listener.error",
						listener_name: listenerName,
						timestamp: new Date().toISOString(),
						error: "Webhook token validation failed — dropping event",
					});
					return;
				}
			}

			if (signatureSecret) {
				const sigHeader = req.headers["x-signature"];
				const expectedSig = createHmac("sha256", signatureSecret)
					.update(rawBody)
					.digest("hex");
				if (sigHeader !== expectedSig) {
					emitListenerLog({
						event: "listener.error",
						listener_name: listenerName,
						timestamp: new Date().toISOString(),
						error: "Webhook signature validation failed — dropping event",
					});
					return;
				}
			}

			// Parse JSON body
			let payload: unknown;
			try {
				payload = JSON.parse(rawBody);
			} catch {
				payload = { raw: rawBody };
			}

			// REQ-LNP-045: Truncate payload to 64KB
			const { payload: truncated, truncated: wasTruncated } =
				truncatePayload(payload);

			const event = {
				id: generateEventId(),
				listener_name: listenerName,
				source_type: "http" as const,
				source_ref: `http://${host}:${port}${path}`,
				received_at: new Date().toISOString(),
				payload: truncated,
				truncated: wasTruncated,
				delivery: {
					mode: "non-interruptive" as const,
					status: "pending" as const,
					attempts: 0,
					last_attempt_at: null,
				},
			};

			emitListenerLog({
				event: "listener.event_received",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
				event_id: event.id,
				truncated: wasTruncated,
			});

			// Process event asynchronously
			if (onEventCallback) {
				void onEventCallback(event).catch((err) => {
					const msg = err instanceof Error ? err.message : String(err);
					emitListenerLog({
						event: "listener.error",
						listener_name: listenerName,
						timestamp: new Date().toISOString(),
						event_id: event.id,
						error: `Event processing failed: ${msg}`,
					});
				});
			}
		});

		req.on("error", (err) => {
			emitListenerLog({
				event: "listener.error",
				listener_name: listenerName,
				timestamp: new Date().toISOString(),
				error: `Request error: ${err.message}`,
			});
		});
	}

	return {
		async start(onEvent: EventCallback): Promise<void> {
			if (server) return;
			onEventCallback = onEvent;

			return new Promise((resolve, reject) => {
				server = createServer(handleRequest);

				server.on("error", (err) => {
					healthy = false;
					statusDesc = `error: ${err.message}`;
					emitListenerLog({
						event: "listener.error",
						listener_name: listenerName,
						timestamp: new Date().toISOString(),
						error: err.message,
					});
					reject(err);
				});

				server.listen(port, host, () => {
					// unref() allows the process to exit even if this server is still
					// listening (important for test environments to avoid hanging).
					if (server && typeof server.unref === "function") server.unref();
					healthy = true;
					statusDesc = `running on ${host}:${port}${path}`;
					emitListenerLog({
						event: "listener.started",
						listener_name: listenerName,
						timestamp: new Date().toISOString(),
					});
					resolve();
				});
			});
		},

		async stop(): Promise<void> {
			if (!server) return;
			return new Promise((resolve) => {
				server!.close(() => {
					server = null;
					healthy = false;
					statusDesc = "stopped";
					onEventCallback = null;
					emitListenerLog({
						event: "listener.stopped",
						listener_name: listenerName,
						timestamp: new Date().toISOString(),
					});
					resolve();
				});
			});
		},

		isHealthy(): boolean {
			return healthy && server !== null;
		},

		statusDescription(): string {
			return statusDesc;
		},
	};
}
