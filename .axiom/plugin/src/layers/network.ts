/**
 * L4 Network Awareness — HeartbeatClient, TeamPulseClient, conflict detection.
 *
 * REQ-PLG-300: /axiom-pulse command shows team activity
 * REQ-PLG-303: codeops_team_pulse tool returns structured JSON
 * REQ-PLG-305: heartbeat published every 60s with HMAC-SHA256 signing
 * REQ-PLG-306: heartbeat MUST NOT include session content, PII beyond user_id
 * REQ-PLG-309: conflict detection (advisory, opt-in)
 * REQ-PLG-718: user_id derived from token, never from config
 * REQ-PLG-719: visibility modes: public, team_only, hidden
 *
 * Spec: specs/70-OpenCode-Plugin.md §7
 * axiom:trace work_item=plugin-l4-wiring-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-305 plan=phase-70-7/task-70-7-1/step-70-7-1-1
 */

import { createHash, createHmac } from "node:crypto";

// ── HTTPS / allowlist enforcement ─────────────────────────────────────────

/**
 * Validate that an outbound URL is safe to fetch.
 *
 * REQ-PLG-708: HTTPS enforcement — all outbound requests must use HTTPS.
 * REQ-PLG-709: URL allowlist — only configured backend_url domain permitted.
 *
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-708,REQ-PLG-709 plan=phase-70-6/task-70-6-5/step-70-6-5-1 jira_ref=DEX-64
 *
 * @param url          The URL about to be fetched.
 * @param allowedDomain Optional: the configured backend_url (or its hostname).
 *                      When provided, the URL's hostname must match exactly.
 * @throws Error with REQ-PLG-708 prefix if the scheme is not https.
 * @throws Error with REQ-PLG-709 prefix if the hostname is not in the allowlist.
 */
export function validateOutboundUrl(url: string, allowedDomain?: string): void {
	if (!url.startsWith("https://")) {
		throw new Error(
			`REQ-PLG-708: Outbound URL must use HTTPS. Received: ${url.substring(0, 50)}`,
		);
	}
	if (allowedDomain) {
		const urlObj = new URL(url);
		const allowedObj = new URL(
			allowedDomain.startsWith("https://") || allowedDomain.startsWith("http://")
				? allowedDomain
				: `https://${allowedDomain}`,
		);
		if (urlObj.hostname !== allowedObj.hostname) {
			throw new Error(
				`REQ-PLG-709: URL not in allowlist. Expected domain: ${allowedObj.hostname}, got: ${urlObj.hostname}`,
			);
		}
	}
}

// ── Types ────────────────────────────────────────────────────────────────

export type VisibilityMode = "public" | "team_only" | "hidden";

/** Heartbeat payload sent to the backend. */
export interface HeartbeatPayload {
	user_id: string;
	work_item_id: string;
	timestamp: string;
	visibility: VisibilityMode;
	/** HMAC-SHA256 signature of the payload (REQ-PLG-305). */
	signature: string;
}

/** A team member entry from the pulse response. */
export interface TeamMember {
	userId: string;
	displayName: string;
	workItemId: string;
	lastSeen: string;
	visibility: VisibilityMode;
}

/** A conflict warning when another member is on the same work item. */
export interface ConflictWarning {
	conflictingUserId: string;
	conflictingDisplayName?: string;
	workItemId: string;
	message: string;
}

/** Options for createHeartbeatClient. */
export interface HeartbeatClientOptions {
	backendUrl: string;
	apiToken: string;
	intervalMs?: number;
	workItemId?: string;
	visibility?: VisibilityMode;
	/** Internal: override send for testing. */
	_mockSend?: (payload: HeartbeatPayload) => Promise<{ ok: boolean }>;
}

/** The heartbeat client interface. */
export interface HeartbeatClient {
	start(): void;
	stop(): void;
	isRunning(): boolean;
}

/** Options for buildHeartbeatPayload. */
export interface BuildHeartbeatOptions {
	apiToken: string;
	workItemId: string;
	visibility?: VisibilityMode;
}

/** Options for createTeamPulseClient. */
export interface TeamPulseClientOptions {
	backendUrl: string;
	apiToken: string;
	/** Internal: override fetch for testing. */
	_mockFetch?: () => Promise<{ members: TeamMember[] }>;
}

/** The team pulse client interface. */
export interface TeamPulseClient {
	getTeamPulse(): Promise<TeamMember[]>;
}

// ── Heartbeat payload builder ─────────────────────────────────────────────

/**
 * Build a heartbeat payload.
 *
 * REQ-PLG-718: user_id derived from apiToken via SHA-256 hash (first 16 chars).
 * This ensures user_id is stable per token but never exposes the raw token.
 * REQ-PLG-306: payload MUST NOT include session content, file contents, or cost data.
 */
export function buildHeartbeatPayload(
	options: BuildHeartbeatOptions,
): HeartbeatPayload {
	const { apiToken, workItemId, visibility = "public" } = options;

	// REQ-PLG-718: derive user_id from token hash, never from config
	const user_id = createHash("sha256")
		.update(apiToken)
		.digest("hex")
		.slice(0, 16);

	const timestamp = new Date().toISOString();

	// Build the signable payload (without signature)
	const signable = JSON.stringify({
		user_id,
		work_item_id: workItemId,
		timestamp,
		visibility,
	});

	// HMAC-SHA256 signature using the API token as the key
	const signature = createHmac("sha256", apiToken)
		.update(signable)
		.digest("hex");

	return {
		user_id,
		work_item_id: workItemId,
		timestamp,
		visibility,
		signature,
	};
}

// ── HeartbeatClient ───────────────────────────────────────────────────────

/**
 * Create a heartbeat client that sends periodic heartbeats to the backend.
 *
 * REQ-PLG-305: POST to /heartbeat every intervalMs (default 60s).
 * Non-blocking background timer. Stop cleanly when session ends.
 */
export function createHeartbeatClient(
	options: HeartbeatClientOptions,
): HeartbeatClient {
	const {
		backendUrl,
		apiToken,
		intervalMs = 60_000,
		workItemId = "",
		visibility = "public",
		_mockSend,
	} = options;

	let timer: ReturnType<typeof setInterval> | null = null;
	let running = false;

	async function sendHeartbeat(): Promise<void> {
		const payload = buildHeartbeatPayload({ apiToken, workItemId, visibility });

		if (_mockSend) {
			await _mockSend(payload).catch(() => {
				// Non-fatal — heartbeat failure should not crash the session
			});
			return;
		}

		// Real HTTP POST — REQ-PLG-708/709: validate URL before fetch
		try {
			const heartbeatUrl = `${backendUrl}/api/v1/heartbeat`;
			validateOutboundUrl(heartbeatUrl, backendUrl);
			await fetch(heartbeatUrl, {
				method: "POST",
				headers: {
					Authorization: `Bearer ${apiToken}`,
					"Content-Type": "application/json",
				},
				body: JSON.stringify(payload),
			});
		} catch {
			// Non-fatal — heartbeat failure should not crash the session
		}
	}

	function start(): void {
		if (running) return;
		running = true;
		// Send immediately, then on interval
		void sendHeartbeat();
		timer = setInterval(() => {
			void sendHeartbeat();
		}, intervalMs);
		// Allow the process to exit even if this timer is still active
		// (important for test environments to avoid hanging after tests complete)
		if (timer && typeof timer.unref === "function") timer.unref();
	}

	function stop(): void {
		running = false;
		if (timer !== null) {
			clearInterval(timer);
			timer = null;
		}
	}

	function isRunning(): boolean {
		return running;
	}

	return { start, stop, isRunning };
}

// ── TeamPulseClient ───────────────────────────────────────────────────────

/**
 * Create a team pulse client for fetching active team members.
 *
 * REQ-PLG-303: GET /team/pulse, return active team members as structured JSON.
 * REQ-PLG-719: hidden members are filtered from pulse responses.
 */
export function createTeamPulseClient(
	options: TeamPulseClientOptions,
): TeamPulseClient {
	const { backendUrl, apiToken, _mockFetch } = options;

	async function getTeamPulse(): Promise<TeamMember[]> {
		try {
			let data: { members: TeamMember[] };

			if (_mockFetch) {
				data = await _mockFetch();
			} else {
				// REQ-PLG-708/709: validate URL before fetch
				const pulseUrl = `${backendUrl}/api/v1/team/pulse`;
				validateOutboundUrl(pulseUrl, backendUrl);
				const response = await fetch(pulseUrl, {
					headers: { Authorization: `Bearer ${apiToken}` },
				});
				if (!response.ok) return [];
				data = (await response.json()) as { members: TeamMember[] };
			}

			// REQ-PLG-719: filter out hidden members from pulse responses
			return (data.members ?? []).filter((m) => m.visibility !== "hidden");
		} catch {
			return [];
		}
	}

	return { getTeamPulse };
}

// ── Conflict detection ────────────────────────────────────────────────────

/**
 * Detect conflicts — other team members working on the same work item.
 *
 * REQ-PLG-309: advisory only, opt-in. Returns empty array if no conflicts.
 */
export function detectConflicts(
	currentWorkItemId: string,
	currentUserId: string,
	teamMembers: TeamMember[],
): ConflictWarning[] {
	return teamMembers
		.filter(
			(m) => m.workItemId === currentWorkItemId && m.userId !== currentUserId,
		)
		.map((m) => ({
			conflictingUserId: m.userId,
			conflictingDisplayName: m.displayName,
			workItemId: currentWorkItemId,
			message: `${m.displayName ?? m.userId} is also working on ${currentWorkItemId}. Coordinate to avoid conflicts.`,
		}));
}

// ── REQ-PLG-307: Visibility-controllable HeartbeatClient ─────────────────

/** Extended heartbeat client with runtime visibility control. */
export interface HeartbeatClientWithVisibilityControl extends HeartbeatClient {
	/** Get the current visibility mode. */
	getVisibility(): VisibilityMode;
	/** Set the visibility mode for subsequent heartbeats. */
	setVisibility(mode: VisibilityMode): void;
}

/**
 * Create a heartbeat client with runtime visibility control.
 *
 * REQ-PLG-307: /axiom-pulse hide sets visibility to 'hidden';
 * /axiom-pulse show restores the configured default.
 */
export function createHeartbeatClientWithVisibilityControl(
	options: HeartbeatClientOptions,
): HeartbeatClientWithVisibilityControl {
	const {
		backendUrl,
		apiToken,
		intervalMs = 60_000,
		workItemId = "",
		visibility: initialVisibility = "public",
		_mockSend,
	} = options;

	let currentVisibility: VisibilityMode = initialVisibility;
	let timer: ReturnType<typeof setInterval> | null = null;
	let running = false;

	async function sendHeartbeat(): Promise<void> {
		const payload = buildHeartbeatPayload({
			apiToken,
			workItemId,
			visibility: currentVisibility,
		});
		if (_mockSend) {
			await _mockSend(payload).catch(() => {});
			return;
		}
		// REQ-PLG-708/709: validate URL before fetch
		try {
			const heartbeatUrl = `${backendUrl}/api/v1/heartbeat`;
			validateOutboundUrl(heartbeatUrl, backendUrl);
			await fetch(heartbeatUrl, {
				method: "POST",
				headers: {
					Authorization: `Bearer ${apiToken}`,
					"Content-Type": "application/json",
				},
				body: JSON.stringify(payload),
			});
		} catch {
			// Non-fatal
		}
	}

	function start(): void {
		if (running) return;
		running = true;
		void sendHeartbeat();
		timer = setInterval(() => {
			void sendHeartbeat();
		}, intervalMs);
		if (timer && typeof timer.unref === "function") timer.unref();
	}

	function stop(): void {
		running = false;
		if (timer !== null) {
			clearInterval(timer);
			timer = null;
		}
	}

	function isRunning(): boolean {
		return running;
	}
	function getVisibility(): VisibilityMode {
		return currentVisibility;
	}
	function setVisibility(mode: VisibilityMode): void {
		currentVisibility = mode;
	}

	return { start, stop, isRunning, getVisibility, setVisibility };
}

// ── REQ-PLG-308: Quiet mode timer ────────────────────────────────────────

/** Options for createQuietModeTimer. */
export interface QuietModeTimerOptions {
	/** How long (ms) the quiet window lasts (default: 5 minutes). */
	quietDurationMs?: number;
}

/** A quiet mode timer that suppresses proactive network feature surfacing. */
export interface QuietModeTimer {
	/** Returns true if currently in quiet mode. */
	isQuiet(): boolean;
	/** Reset the quiet window (restart the timer). */
	reset(): void;
	/** Stop the timer and exit quiet mode. */
	stop(): void;
}

/**
 * Create a quiet mode timer.
 *
 * REQ-PLG-308: network features are available but not proactively surfaced
 * for the first quietDurationMs (default: 5 minutes) after session start.
 */
export function createQuietModeTimer(
	options: QuietModeTimerOptions = {},
): QuietModeTimer {
	const { quietDurationMs = 5 * 60 * 1000 } = options;
	let quiet = true;
	let timer: ReturnType<typeof setTimeout> | null = null;

	function scheduleEnd(): void {
		if (timer !== null) clearTimeout(timer);
		timer = setTimeout(() => {
			quiet = false;
			timer = null;
		}, quietDurationMs);
	}

	scheduleEnd();

	function isQuiet(): boolean {
		return quiet;
	}

	function reset(): void {
		quiet = true;
		scheduleEnd();
	}

	function stop(): void {
		quiet = false;
		if (timer !== null) {
			clearTimeout(timer);
			timer = null;
		}
	}

	return { isQuiet, reset, stop };
}
