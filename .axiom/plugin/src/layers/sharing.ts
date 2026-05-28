/**
 * L3 Session Sharing — SessionReplayClient, SessionBroadcastClient, local export.
 *
 * REQ-PLG-200: /axiom-share command creates shareable session replay
 * REQ-PLG-204: opt-in confirmation before sharing
 * REQ-PLG-218: --local fallback exports to .axiom/shared/<session-id>.json
 * REQ-PLG-701: redaction applied before sharing
 * REQ-PLG-716: SSE reconnection with Last-Event-ID
 *
 * Spec: specs/70-OpenCode-Plugin.md §6
 * axiom:trace work_item=plugin-l3-wiring-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-200 plan=P-70-opencode-plugin.md#phase-706
 */

import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { redactContent } from "../security/redaction";
import { validateOutboundUrl } from "./network";

// ── Types ────────────────────────────────────────────────────────────────

export type SessionVisibility = "link" | "team" | "private";

export interface ShareOptions {
	sessionId: string;
	content: string;
	visibility: SessionVisibility;
	/** Internal: override upload for testing */
	_mockUpload?: (
		content: string,
		headers?: Record<string, string>,
	) => Promise<{ url: string; sessionId: string }>;
}

export interface ShareResult {
	url: string;
	sessionId: string;
	redactedCount: number;
}

export interface SessionReplayClientOptions {
	backendUrl: string;
	authToken: string;
}

export interface SessionReplayClient {
	share(options: ShareOptions): Promise<ShareResult>;
	list(): Promise<{ sessionId: string; url: string; createdAt: string }[]>;
	delete(sessionId: string): Promise<void>;
}

export interface SubscribeOptions {
	sessionId: string;
	lastEventId?: string;
	onMessage: (message: string) => void;
	/** Internal: override SSE connect for testing */
	_mockConnect?: (options: Record<string, unknown>) => { close: () => void };
}

export interface SessionBroadcastClientOptions {
	backendUrl: string;
	authToken: string;
}

export interface SessionBroadcastClient {
	subscribe(options: SubscribeOptions): void;
	unsubscribe(sessionId: string): void;
}

export interface ShareConfirmationOptions {
	sessionId: string;
	redactedCount: number;
	contentPreview: string;
}

export interface LocalExportOptions {
	repoRoot: string;
	sessionId: string;
	content: string;
}

export interface LocalExportResult {
	filePath: string;
	redactedCount: number;
}

// ── SessionReplayClient ───────────────────────────────────────────────────

/**
 * Create a session replay client for sharing sessions to the Axiom backend.
 *
 * REQ-PLG-200: POST to /sessions/{id}/replay with bearer auth.
 * REQ-PLG-701: redaction applied before upload.
 */
export function createSessionReplayClient(
	options: SessionReplayClientOptions,
): SessionReplayClient {
	const { backendUrl, authToken } = options;

	async function share(shareOptions: ShareOptions): Promise<ShareResult> {
		const { sessionId, content, visibility, _mockUpload } = shareOptions;

		// REQ-PLG-701: apply redaction before sharing
		const redactionResult = redactContent(content);
		const redactedContent = redactionResult.redacted;
		const redactedCount = redactionResult.count;

		const headers: Record<string, string> = {
			Authorization: `Bearer ${authToken}`,
			"Content-Type": "application/json",
		};

		const payload = JSON.stringify({
			session_id: sessionId,
			content: redactedContent,
			visibility,
		});

		if (_mockUpload) {
			const result = await _mockUpload(redactedContent, headers);
			return { ...result, redactedCount };
		}

		// Real HTTP upload to backend
		// REQ-PLG-708/709: validate URL before fetch
		const shareUrl = `${backendUrl}/api/v1/sessions/share`;
		validateOutboundUrl(shareUrl, backendUrl);
		const response = await fetch(shareUrl, {
			method: "POST",
			headers,
			body: payload,
		});

		if (!response.ok) {
			throw new Error(
				`Share failed: ${response.status} ${response.statusText}`,
			);
		}

		const data = (await response.json()) as { url: string; session_id: string };
		return {
			url: data.url,
			sessionId: data.session_id,
			redactedCount,
		};
	}

	async function list(): Promise<
		{ sessionId: string; url: string; createdAt: string }[]
	> {
		// REQ-PLG-708/709: validate URL before fetch
		const listUrl = `${backendUrl}/api/v1/sessions/shared`;
		validateOutboundUrl(listUrl, backendUrl);
		const response = await fetch(listUrl, {
			headers: { Authorization: `Bearer ${authToken}` },
		});
		if (!response.ok) return [];
		const data = (await response.json()) as {
			sessions: Array<{ session_id: string; url: string; created_at: string }>;
		};
		return (data.sessions ?? []).map((s) => ({
			sessionId: s.session_id,
			url: s.url,
			createdAt: s.created_at,
		}));
	}

	async function deleteSession(sessionId: string): Promise<void> {
		// REQ-PLG-708/709: validate URL before fetch
		const deleteUrl = `${backendUrl}/api/v1/sessions/shared/${sessionId}`;
		validateOutboundUrl(deleteUrl, backendUrl);
		await fetch(deleteUrl, {
			method: "DELETE",
			headers: { Authorization: `Bearer ${authToken}` },
		});
	}

	return { share, list, delete: deleteSession };
}

// ── SessionBroadcastClient ────────────────────────────────────────────────

/**
 * Create a session broadcast client for subscribing to live session streams.
 *
 * REQ-PLG-716: SSE subscription with Last-Event-ID reconnection.
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-208 plan=phase-70-6/task-70-6-1/step-70-6-1-1
 */
export function createSessionBroadcastClient(
	options: SessionBroadcastClientOptions,
): SessionBroadcastClient {
	const { backendUrl, authToken } = options;
	const connections = new Map<string, { close: () => void }>();

	function subscribe(subscribeOptions: SubscribeOptions): void {
		const { sessionId, lastEventId, onMessage, _mockConnect } =
			subscribeOptions;

		const connectOptions: Record<string, unknown> = {
			url: `${backendUrl}/api/v1/sessions/shared/${sessionId}/stream`,
			headers: {
				Authorization: `Bearer ${authToken}`,
				...(lastEventId ? { "Last-Event-ID": lastEventId } : {}),
			},
			lastEventId,
		};

		if (_mockConnect) {
			const conn = _mockConnect(connectOptions);
			connections.set(sessionId, conn);
			return;
		}

		// Real SSE connection (browser EventSource or fetch-based SSE)
		// In Node/Bun environment, use fetch with streaming
		const controller = new AbortController();
		connections.set(sessionId, { close: () => controller.abort() });

		void (async () => {
			try {
				const headers: Record<string, string> = {
					Authorization: `Bearer ${authToken}`,
					Accept: "text/event-stream",
				};
				if (lastEventId) {
					headers["Last-Event-ID"] = lastEventId;
				}
				// REQ-PLG-708/709: validate URL before fetch
				validateOutboundUrl(connectOptions.url as string, backendUrl);
				const response = await fetch(connectOptions.url as string, {
					headers,
					signal: controller.signal,
				});
				if (!response.ok || !response.body) return;
				const reader = response.body.getReader();
				const decoder = new TextDecoder();
				while (true) {
					const { done, value } = await reader.read();
					if (done) break;
					const chunk = decoder.decode(value);
					// Parse SSE data lines
					for (const line of chunk.split("\n")) {
						if (line.startsWith("data: ")) {
							onMessage(line.slice(6));
						}
					}
				}
			} catch {
				// Connection closed or aborted — non-fatal
			}
		})();
	}

	function unsubscribe(sessionId: string): void {
		const conn = connections.get(sessionId);
		if (conn) {
			conn.close();
			connections.delete(sessionId);
		}
	}

	return { subscribe, unsubscribe };
}

// ── Opt-in confirmation ───────────────────────────────────────────────────

/**
 * Build the opt-in confirmation prompt shown before sharing.
 *
 * REQ-PLG-204: show redaction summary and ask for confirmation.
 */
export function buildShareConfirmationPrompt(
	options: ShareConfirmationOptions,
): string {
	const { redactedCount, contentPreview } = options;
	const lines: string[] = [];

	if (redactedCount > 0) {
		lines.push(
			`I redacted ${redactedCount} sensitive item${redactedCount === 1 ? "" : "s"} from this session.`,
		);
	}

	lines.push(
		`Preview: "${contentPreview.slice(0, 80)}${contentPreview.length > 80 ? "..." : ""}"`,
	);
	lines.push("Share this session replay? [yes/no]");

	return lines.join("\n");
}

// ── Local export fallback ─────────────────────────────────────────────────

/**
 * Export a session locally to .axiom/shared/<session-id>.json.
 *
 * REQ-PLG-218: --local fallback when no backend is available.
 * REQ-PLG-701: redaction applied before export.
 */
export function exportSessionLocal(
	options: LocalExportOptions,
): LocalExportResult {
	const { repoRoot, sessionId, content } = options;

	// REQ-PLG-701: apply redaction
	const redactionResult = redactContent(content);
	const redactedContent = redactionResult.redacted;
	const redactedCount = redactionResult.count;

	const sharedDir = join(repoRoot, ".axiom", "shared");
	if (!existsSync(sharedDir)) {
		mkdirSync(sharedDir, { recursive: true });
	}

	const filePath = join(sharedDir, `${sessionId}.json`);
	const exportData = {
		session_id: sessionId,
		content: redactedContent,
		exported_at: new Date().toISOString(),
		redacted_count: redactedCount,
	};

	writeFileSync(filePath, JSON.stringify(exportData, null, 2), "utf-8");

	return { filePath, redactedCount };
}
