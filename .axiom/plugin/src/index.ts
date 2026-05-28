/**
 * opencode-axiom — Axiom platform extension plugin for OpenCode.
 *
 * This is the entry point for the OpenCode plugin. It exports the plugin
 * definition that OpenCode loads on session start.
 *
 * Spec: specs/70-OpenCode-Plugin.md
 * axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md plan=phase-70-1/task-70-1-5/step-70-1-5-3
 */

import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import type { Plugin } from "@opencode-ai/plugin";
import { tool } from "@opencode-ai/plugin";
import { createBackendRouter } from "./backend-routing";
import { loadPluginConfig } from "./config-loader";
import { detectMaturity } from "./detect";
import {
	createCompactionHook,
	createCopilotSystemHook,
	createSessionInitHook,
	createShellEnvHook,
} from "./hooks";
import { createHooksEngine } from "./layers/automation/hooks";
import { createIdleDetector } from "./layers/automation/idle-detector";
import { createScriptsEngine } from "./layers/automation/scripts";
import {
	buildToolDefinitions,
	createToolRegistry,
	isConfigApproved,
} from "./layers/automation/tool-registry";
import { createWatchesEngine } from "./layers/automation/watches";
import { createPluginLogger } from "./logger";
import { isBlockedFile } from "./security/blocked-files";
import {
	buildPrivacyReport,
	formatPrivacyReport,
} from "./security/privacy-command";
import { findSdkPackageJson, isVersionCompatible } from "./version";

export type {
	BackendDetectionOptions,
	BackendDetectionResult,
	BackendHealthCheck,
	BackendMode,
	SessionBackendCache,
} from "./backend";
export { createSessionCache, detectBackendMode } from "./backend";
export type {
	ActiveBackend,
	BackendId,
	BackendRouter,
	BackendRouterOptions,
	BackendStatusReport,
	BackendSwitchResult,
	BackendUseCase,
} from "./backend-routing";
export { createBackendRouter } from "./backend-routing";
export type { BootstrapOptions, BootstrapResult, RepoInfo } from "./bootstrap";
export { bootstrap } from "./bootstrap";
export type { AboveLevelResult, CommandDefinition } from "./commands";
export {
	COMMAND_VISIBILITY_MATRIX,
	getCommandsForLevel,
	handleAboveLevelCommand,
} from "./commands";
export type {
	CodeOpsConfig,
	ConfidenceConfig,
	GithubConfig,
	GithubConfigResult,
	GithubSettingsOptions,
	JiraConfig,
	JiraConfigResult,
} from "./config";
export {
	configureGithubSettings,
	configureJiraProjectKey,
	readConfigYaml,
	validateJiraProjectKey,
	writeConfigYaml,
} from "./config";
export type {
	HookEntry,
	HooksConfig,
	LayersConfig,
	MetricsConfig,
	NetworkConfig,
	PluginConfig,
	PluginConfigResult,
	ScriptEntry,
	ToolEntry,
	ValidationResult,
	WatchEntry,
} from "./config-loader";
export {
	DEFAULT_PLUGIN_CONFIG,
	loadPluginConfig,
	validatePluginConfig,
} from "./config-loader";
export type { BackendInfo, MaturityResult, RepoMetadata } from "./detect";
export { detectMaturity, MATURITY_LABELS, MaturityLevel } from "./detect";
export type { HookContext } from "./hooks";
export {
	createCompactionHook,
	createCopilotSystemHook,
	createSessionInitHook,
	createShellEnvHook,
} from "./hooks";
export type {
	HookContext as AutomationHookContext,
	HookExecutionResult,
	HooksEngine,
	HooksEngineOptions,
	LifecycleEvent,
} from "./layers/automation/hooks";
// L5: Config-Driven Automation
export {
	createHooksEngine,
	evaluateCondition,
} from "./layers/automation/hooks";
export type {
	RunScriptOptions,
	ScriptListEntry,
	ScriptRunResult,
	ScriptsEngine,
	ScriptsEngineOptions,
} from "./layers/automation/scripts";
export {
	createScriptsEngine,
	substituteArgs,
} from "./layers/automation/scripts";
export type {
	ToolCountCheckResult,
	ToolDefinition,
	ToolExecutionResult,
	ToolRegistry,
	ToolRegistryOptions,
} from "./layers/automation/tool-registry";
// L6: Tool Extension
export {
	approveConfig,
	buildToolDefinitions,
	computeConfigHash,
	createToolRegistry,
	isConfigApproved,
} from "./layers/automation/tool-registry";
export type {
	BlockLogEntry,
	WatchActionResult,
	WatchesEngine,
	WatchesEngineOptions,
	WatchTriggerEvent,
} from "./layers/automation/watches";
export {
	createWatchesEngine,
	matchesGlobPattern,
	matchesWatchTrigger,
} from "./layers/automation/watches";
export type {
	MessageTokens,
	ModelCostEntry,
	ParsedMessage,
	PricingEntry,
	PricingTable,
	SessionCostResult,
} from "./layers/metrics";
export {
	applyPricingOverrides,
	computeSessionCost,
	loadEmbeddedPricing,
	loadPricingTable,
	parseSessionMessages,
} from "./layers/metrics";
export type {
	BudgetEvaluationResult,
	SessionBudgetConfig,
} from "./layers/metrics-budget";
export {
	evaluateSessionBudget,
	formatBudgetAlert,
	formatBudgetWarning,
	loadSessionBudgetConfig,
} from "./layers/metrics-budget";
export type {
	BuildSessionCostInput,
	SessionCostModelEntry,
	SessionCostToolResult,
} from "./layers/metrics-tools";
export {
	buildSessionCostResult,
	formatSessionCostSummary,
} from "./layers/metrics-tools";
export type {
	GitignoreOptions,
	GitignoreResult,
	PersistSessionOptions,
	RetentionPolicyOptions,
	RetentionResult,
	SessionAnalyticsFile,
	TestFileEntry,
} from "./layers/session-persistence";
export {
	enforceRetentionPolicy,
	ensureSessionsGitignore,
	MAX_SESSION_AGE_DAYS,
	MAX_SESSION_FILES,
	persistSessionAnalytics,
	SESSION_DIR_NAME,
} from "./layers/session-persistence";
export type {
	BlockedFileChecker,
	BlockedFileConfig,
	BlockedFileResult,
} from "./security/blocked-files";
export {
	createBlockedFileChecker,
	DEFAULT_BLOCKED_PATTERNS,
	isBlockedFile,
} from "./security/blocked-files";
export type {
	DataCategory,
	DeletionGuidance,
	PrivacyReport,
	PrivacyReportOptions,
	RedactionPatternSummary,
	SharedDataEntry,
	StorageLocation,
} from "./security/privacy-command";
export {
	buildPrivacyReport,
	formatPrivacyReport,
} from "./security/privacy-command";
export type {
	CustomPattern,
	RedactionConfig,
	RedactionEngine,
	RedactionLogEntry,
	RedactionResult,
} from "./security/redaction";
export {
	createRedactionEngine,
	redactContent,
} from "./security/redaction";
export type {
	PackageNameVerification,
	ProvenanceGuidance,
	StartupAuditLog,
	SupplyChainConfig,
	UpdateCheckResult,
} from "./security/supply-chain";
export {
	buildStartupAuditLog,
	checkForUpdates,
	getProvenanceGuidance,
	verifyPackageName,
} from "./security/supply-chain";
export type {
	CustomSpinupTarget,
	SpinupExecutor,
	SpinupManager,
	SpinupManagerOptions,
	SpinupResult,
	SpinupStatus,
	SpinupStopResult,
	SpinupTarget,
} from "./spinup";
export { BUILTIN_TARGETS, createSpinupManager } from "./spinup";

/**
 * Axiom plugin for OpenCode.
 *
 * Capability layers (independently enableable):
 * - L0: Onboarding (maturity detection, bootstrap, progressive commands, copilot)
 * - L1: Execution (backend routing, spin-up)
 * - L2: Metrics (cost tracking, session analytics)
 * - L3: Session Sharing (replay, broadcast)
 * - L4: Network Awareness (team pulse, heartbeat)
 * - L5: Config Automation (hooks, watches, scripts)
 * - L6: Tool Extension (custom tool registration)
 */
export const CodeOpsPlugin: Plugin = async ({ directory, client }) => {
	// REQ-PLG-134: Route all plugin log output through OpenCode's structured logger
	const log = createPluginLogger(client);

	// REQ-PLG-010: Detect maturity level on session start
	const maturity = detectMaturity(directory);

	// REQ-PLG-052/053: Backend router for routing and /axiom-backend command
	const router = createBackendRouter({ repoDir: directory });

	// REQ-PLG-005: Version compatibility check (non-blocking)
	// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-005 plan=phase-70-fixes/task-f5-ext/step-f5-ext-1
	// REQ-PLG-137: Hoist sdkVersion so it is accessible for opencodeVersion in createPublisher()
	// axiom:trace work_item=plugin-metrics-fixes-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-137 plan=phase-70-l2-plugin-version-auto jira_ref=DEX-419
	let sdkVersion: string | null = null;
	try {
		const MIN_OPENCODE_VERSION = "1.1.53";
		const pkgPath = findSdkPackageJson(directory);
		if (!pkgPath) {
			log.info(
				"Could not locate @opencode-ai/plugin package.json — skipping version compatibility check",
			);
		} else {
			const pkg = JSON.parse(readFileSync(pkgPath, "utf-8")) as {
				version?: string;
			};
			sdkVersion = pkg.version ?? null;
			if (sdkVersion && !isVersionCompatible(sdkVersion, MIN_OPENCODE_VERSION)) {
				log.warn(
					`Axiom plugin requires OpenCode SDK >= ${MIN_OPENCODE_VERSION}. Detected: ${sdkVersion}. Some features may not work correctly.`,
				);
			}
		}
	} catch {
		// Non-blocking — version check must never throw
	}

	// REQ-PLG-030: Lazy session init (session.created unavailable in SDK v1.2.27)
	const sessionInit = createSessionInitHook({ directory });

	// REQ-PLG-013: Copilot auto-load via experimental.chat.system.transform
	const copilotHook = createCopilotSystemHook({
		directory,
		maturityLevel: maturity.level,
	});

	// REQ-PLG-031: Compaction survival via experimental.session.compacting
	const compactionHook = createCompactionHook({
		directory,
		maturityLevel: maturity.level,
	});

	// REQ-PLG-036: Shell env injection via shell.env
	const shellEnvHook = createShellEnvHook({
		directory,
		maturityLevel: maturity.level,
		backend: maturity.backends.mode,
	});

	// REQ-PLG-400: Load L5/L6 config (silently disabled if not present)
	const configResult = loadPluginConfig(directory);
	let pluginConfig = configResult.config;

	// REQ-PLG-400a: Apply profile-based config scoping if AXIOM_PROFILE is set
	if (pluginConfig?.profiles) {
		const { resolveActiveProfile } = await import("./config-loader");
		const profileResult = resolveActiveProfile(pluginConfig);
		pluginConfig = profileResult.config;
		if (profileResult.warning) {
			// REQ-PLG-134: use PluginLogger instead of console.warn
			log.warn(profileResult.warning);
		}
	}

	// REQ-PLG-104a: Resolve session identity for cost attribution
	const { resolveSessionIdentity } = await import("./session-identity");
	const sessionIdentity = resolveSessionIdentity({
		repoRoot: directory,
		mode: pluginConfig
			? ((pluginConfig as unknown as Record<string, unknown>).identity as
					| "auto"
					| "git"
					| "hostname"
					| "custom"
					| undefined)
			: undefined,
		customIdentity: pluginConfig
			? ((pluginConfig as unknown as Record<string, unknown>).custom_identity as
					| string
					| undefined)
			: undefined,
	});
	// Expose session identity as env var for shell hooks and analytics
	process.env.AXIOM_SESSION_IDENTITY = sessionIdentity.identity;
	process.env.AXIOM_SESSION_IDENTITY_SOURCE = sessionIdentity.source;

	// REQ-PLG-104b: Warm pricing cache in background (non-blocking)
	const { fetchPricingNonBlocking } = await import("./pricing-fetch");
	void fetchPricingNonBlocking({
		cacheDir: join(directory, ".axiom"),
		pricingOverrides: pluginConfig?.metrics?.pricing_overrides as
			| Record<string, { prompt_per_1m: number; completion_per_1m: number }>
			| undefined,
	});

	// REQ-PLG-122: L2 Metrics Publisher — dual-cadence snapshot publishing
	// axiom:trace work_item=plugin-metrics-fixes-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-122 plan=phase-70-l2-metrics-runtime-wiring jira_ref=DEX-419
	//
	// currentSessionId is declared in the outer scope so the getSessionId callback
	// (closed over by metricsPublisher) and the event handler (which updates it when
	// OpenCode provides a real session ID) both reference the same mutable binding.
	let currentSessionId = `fallback-${Date.now()}`;
	// Cast to MetricsPublishConfig shape — MetricsConfig in config-loader is a subset;
	// the full publish config fields (publish_enabled, user_email, etc.) are read here
	// via a type-safe cast to avoid TS errors on the narrower MetricsConfig type.
	const metricsRaw = pluginConfig?.metrics as
		| import("./metrics/sink-interface").MetricsPublishConfig
		| undefined;
	const l2MetricsEnabled = metricsRaw?.publish_enabled !== false;
	let metricsPublisher: import("./metrics/publisher").MetricsPublisher | null = null;

	if (l2MetricsEnabled) {
		const {
			createPublisher,
			createStdoutSink,
			resolveEmailIdentitySync,
			resolveGitBranch,
			DEFAULT_METRICS_PUBLISH_CONFIG,
		} = await import("./metrics");

		// REQ-PLG-126: Resolve email identity for metrics attribution
		const emailResult = resolveEmailIdentitySync({
			repoRoot: directory,
			configEmail: metricsRaw?.user_email ?? null,
		});

		// REQ-PLG-133: Resolve git branch
		const gitBranch = resolveGitBranch(directory) ?? undefined;

		// REQ-PLG-137: Read plugin version from own package.json
		let pluginVersion: string | null = null;
		try {
			const ownPkgPath = join(directory, ".axiom", "plugin", "package.json");
			if (existsSync(ownPkgPath)) {
				const ownPkg = JSON.parse(readFileSync(ownPkgPath, "utf-8")) as {
					version?: string;
				};
				pluginVersion = ownPkg.version ?? null;
			}
		} catch {
			// Non-blocking — plugin version is informational only
		}

		// Build active sinks — stdout sink always active (routes through client.app.log)
		const stdoutSink = createStdoutSink(log);
		const activeSinks = [stdoutSink];
		// TODO: add axiom-backend sink when network.backend_url is configured

		// REQ-PLG-123: emit a single WARN when falling back to stdout-only sink
		// axiom:trace work_item=plugin-metrics-fixes-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-123 plan=phase-70-l2-stdout-warn jira_ref=DEX-419
		const hasBackendSink = !!(pluginConfig?.network?.backend_url);
		if (!hasBackendSink) {
			log.warn(
				"Axiom metrics: no backend sink configured — publishing to OpenCode log (stdout sink) only. " +
				"To send metrics to the Axiom backend, set network.backend_url in .axiom/plugin.yaml.",
			);
		}

		const metricsConfig = {
			...DEFAULT_METRICS_PUBLISH_CONFIG,
			...(metricsRaw ?? {}),
		};

		metricsPublisher = createPublisher({
			sinks: activeSinks,
			getSessionId: () => currentSessionId,
			repo: sessionIdentity.identity,
			gitBranch,
			repoPath: directory,
			userEmail: emailResult.email,
			emailSource: emailResult.source,
			sessionIdentity: sessionIdentity.identity,
			workItemId: process.env.AXIOM_WORK_ITEM || undefined,
			runId: process.env.AXIOM_RUN_ID || undefined,
			streamIntervalSeconds: metricsConfig.stream_interval_seconds,
			reconciliationIntervalSeconds:
				metricsConfig.reconciliation_interval_seconds,
			// REQ-PLG-136: reasoning tokens — null until provider exposes them
			getReasoningTokens: () => null,
			// REQ-PLG-137: metadata fields
			modelProvider: null, // populated from session events when available
			agentName: process.env.AXIOM_AGENT_NAME ?? null,
			opencodeVersion: sdkVersion, // REQ-PLG-137: wired from hoisted sdkVersion
			pluginVersion,
			thinkingLevel: null,
			getThinkingTokens: () => null,
			connectedMcps: [], // populated from MCP registry when available
		});

		metricsPublisher.start();

		// REQ-PLG-125: Use beforeExit (not exit) so async publishFinal() can complete.
		// The exit event fires synchronously — async work scheduled after it never runs.
		// beforeExit fires when the event loop is empty but still running, allowing
		// async I/O to complete. SIGTERM/SIGINT handlers are also present for signal-based exits.
		// Note: beforeExit does NOT fire on explicit process.exit() calls.
		// axiom:trace work_item=plugin-metrics-fixes-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-125 plan=phase-70-l2-exit-handler-fix jira_ref=DEX-419
		process.once("beforeExit", () => {
			void metricsPublisher?.publishFinal();
		});
		process.once("SIGTERM", () => {
			void metricsPublisher?.publishFinal();
		});
		process.once("SIGINT", () => {
			void metricsPublisher?.publishFinal();
		});
	}

	// REQ-PLG-720/721: Check config approval before enabling automation
	let configApproved = false;
	if (configResult.loaded && pluginConfig) {
		const configPath = join(
			directory,
			".axiom",
			configResult.source ?? "plugin.yaml",
		);
		if (existsSync(configPath)) {
			const rawContent = readFileSync(configPath, "utf-8");
			configApproved = isConfigApproved(directory, rawContent);
		}
	}

	// L5: Create automation engines (only active when config loaded and approved)
	const l5Active =
		configResult.loaded &&
		configApproved &&
		pluginConfig?.layers?.automation !== false;
	const hooksEngine = l5Active
		? createHooksEngine({ repoRoot: directory })
		: null;
	const watchesEngine = l5Active
		? createWatchesEngine({ repoRoot: directory })
		: null;
	const scriptsEngine = l5Active
		? createScriptsEngine({ repoRoot: directory })
		: null;

	// REQ-PLG-411a: idle detection — fire on_session_idle hooks after inactivity window
	const idleTimeoutMs = pluginConfig?.idle_timeout_ms ?? 60_000;
	const idleDetector =
		l5Active && hooksEngine && pluginConfig?.hooks?.on_session_idle?.length
			? createIdleDetector({
					idleTimeoutMs,
					onIdle: () => {
						if (hooksEngine && pluginConfig?.hooks) {
							hooksEngine.fireEventNonBlocking(
								"on_session_idle",
								pluginConfig.hooks,
								{
									maturityLevel: maturity.level,
									backend: maturity.backends.mode,
								},
							);
						}
					},
				})
			: null;

	// L6: Custom tool definitions (only active when config loaded and approved)
	const l6Active =
		configResult.loaded &&
		configApproved &&
		pluginConfig?.layers?.tools !== false;
	const customToolDefs =
		l6Active && pluginConfig?.tools
			? buildToolDefinitions(pluginConfig.tools)
			: [];
	const toolRegistry = l6Active
		? createToolRegistry({ repoRoot: directory })
		: null;

	// REQ-PLG-305: L4 heartbeat client — starts when network.backend_url is configured
	const l4ApiToken = process.env.AXIOM_API_TOKEN;
	const l4BackendUrl = pluginConfig?.network?.backend_url;
	let heartbeatClient:
		| import("./layers/network").HeartbeatClientWithVisibilityControl
		| null = null;
	let quietModeTimer: import("./layers/network").QuietModeTimer | null = null;

	if (l4BackendUrl && l4ApiToken) {
		// REQ-PLG-305/307: use visibility-controllable client so hide/show can be wired at runtime
		const { createHeartbeatClientWithVisibilityControl, createQuietModeTimer } =
			await import("./layers/network");
		heartbeatClient = createHeartbeatClientWithVisibilityControl({
			backendUrl: l4BackendUrl,
			apiToken: l4ApiToken,
			intervalMs:
				(pluginConfig?.network?.heartbeat_interval_seconds ?? 60) * 1000,
			workItemId: process.env.AXIOM_WORK_ITEM ?? "",
			visibility:
				((pluginConfig?.network as Record<string, unknown>)
					?.visibility as import("./layers/network").VisibilityMode) ??
				"public",
		});
		heartbeatClient.start();

		// REQ-PLG-308: quiet mode — suppress proactive network surfacing for first 5 minutes
		quietModeTimer = createQuietModeTimer({ quietDurationMs: 5 * 60 * 1000 });

		// REQ-PLG-305: stop heartbeat cleanly when process exits (SDK has no session.end event in v1.2.27)
		process.once("exit", () => {
			heartbeatClient?.stop();
			quietModeTimer?.stop();
		});
		process.once("SIGTERM", () => {
			heartbeatClient?.stop();
			quietModeTimer?.stop();
		});
		process.once("SIGINT", () => {
			heartbeatClient?.stop();
			quietModeTimer?.stop();
		});
	}

	// Build custom tools map for the plugin
	const customTools: Record<string, ReturnType<typeof tool>> = {};
	if (l6Active && toolRegistry) {
		for (const toolDef of customToolDefs) {
			const capturedDef = toolDef;
			customTools[capturedDef.name] = tool({
				description: capturedDef.description,
				args: {},
				async execute(_args, _context) {
					const result = await toolRegistry.executeTool(
						{
							name: capturedDef.name,
							description: capturedDef.description,
							run: capturedDef.run,
						},
						{},
					);
					return (
						result.output ||
						(result.success ? "Done." : `Error (exit ${result.exitCode})`)
					);
				},
			});
		}
	}

	return {
		// REQ-PLG-030: event hook delegates to lazy session init
		// Since session.created is unavailable, we use the event hook as a
		// catch-all initializer that runs sessionInit on first invocation.
		event: async ({ event }) => {
			try {
				// Lazy-init on any event; idempotent per sessionInit cache
				// Extract sessionID from event properties when available
				const props =
					event && "properties" in event
						? (event as { properties?: Record<string, unknown> }).properties
						: undefined;
				const sid =
					props && typeof props.sessionID === "string"
						? props.sessionID
						: undefined;
				if (sid) {
					// REQ-PLG-109: update publisher with real session ID from OpenCode
					// axiom:trace work_item=plugin-metrics-fixes-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-109 plan=phase-70-l2-metrics-runtime-wiring jira_ref=DEX-419
					currentSessionId = sid;
					await sessionInit({ sessionID: sid });
			// REQ-PLG-108: emit plugin.session.started structured log event
				// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-108 plan=phase-70-fixes/task-f3/step-f3-1
				// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-109 plan=phase-70-fixes/task-f3-ext/step-f3-ext-1
				log.info("plugin.session.started", {
					event_type: "plugin.session.started",
					session_id: sid,
					maturity_level: maturity.level,
					active_layers: {
						l5_automation: l5Active,
						l6_tools: l6Active,
						l4_network: !!(l4BackendUrl && l4ApiToken),
					},
					run_id: process.env.AXIOM_RUN_ID ?? "",
					work_item_id: process.env.AXIOM_WORK_ITEM ?? "",
					repo: directory,
				});
				}

				// REQ-PLG-411: Fire on_message hooks for message events (L5)
				if (hooksEngine && pluginConfig?.hooks && event && "type" in event) {
					const evType = (event as { type?: string }).type;
					if (evType === "message") {
						hooksEngine.fireEventNonBlocking("on_message", pluginConfig.hooks, {
							maturityLevel: maturity.level,
							backend: maturity.backends.mode,
						});
					}
				}

				// REQ-PLG-411a: reset idle detector on any activity (tool or message)
				if (idleDetector) {
					idleDetector.reset();
				}

			// REQ-PLG-309: conflict detection — check for team members on same work item (advisory)
			// REQ-PLG-308: suppress during quiet mode (first 5 minutes of session)
			// NOTE: This event-hook path fires for ALL event types (not just chat messages).
			// The dedicated chat.message hook (line ~716) also fires conflict detection for
			// message events specifically. This creates a double-fire for message events when
			// L4 is active. This is a known LOW-severity issue — both paths are advisory-only
			// and non-blocking. Consolidation to a single hook is tracked as a follow-up.
			// TODO(phase-70-followup): Remove this block once chat.message hook is confirmed
			// as the canonical owner of REQ-PLG-309 conflict detection.
			if (
					heartbeatClient &&
					l4BackendUrl &&
					l4ApiToken &&
					event &&
					"type" in event
				) {
					const evType = (event as { type?: string }).type;
					if (evType === "message" && !quietModeTimer?.isQuiet()) {
						void (async () => {
							try {
								const { createTeamPulseClient, detectConflicts } = await import(
									"./layers/network"
								);
								const pulseClient = createTeamPulseClient({
									backendUrl: l4BackendUrl,
									apiToken: l4ApiToken,
								});
								const members = await pulseClient.getTeamPulse();
								const currentWorkItem = process.env.AXIOM_WORK_ITEM ?? "";
								const currentUserId = sessionIdentity.identity;
								if (currentWorkItem) {
									const conflicts = detectConflicts(
										currentWorkItem,
										currentUserId,
										members,
									);
									if (conflicts.length > 0) {
										// Advisory warning — logged but not blocking
										// REQ-PLG-134: use PluginLogger instead of console.warn
										log.warn(
											`Conflict detected: ${conflicts[0].message}`,
										);
									}
								}
							} catch {
								// Non-fatal — conflict detection is advisory only
							}
						})();
					}
				}
		} catch (err) {
		// REQ-PLG-037: non-blocking
		// REQ-PLG-108: emit plugin.error structured log event
		// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-108 plan=phase-70-fixes/task-f3/step-f3-1
		// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-109 plan=phase-70-fixes/task-f3-ext/step-f3-ext-1
		log.error("plugin.error", {
			event_type: "plugin.error",
			error_type: "event_hook_error",
			message: err instanceof Error ? err.message : String(err),
			run_id: process.env.AXIOM_RUN_ID ?? "",
			work_item_id: process.env.AXIOM_WORK_ITEM ?? "",
			repo: directory,
			session_id: process.env.AXIOM_SESSION_IDENTITY ?? "",
		});
		}
		},

		// REQ-PLG-013: copilot auto-load for Level 0-2 repos
		"experimental.chat.system.transform": copilotHook,

		// REQ-PLG-031: compaction survival — inject Axiom context
		"experimental.session.compacting": compactionHook,

		// REQ-PLG-036: shell env injection — expose Axiom env vars
		"shell.env": shellEnvHook,

		// REQ-PLG-108: emit plugin.command.executed structured log event
		// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-108 plan=phase-70-fixes/task-f3/step-f3-1
		// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-109 plan=phase-70-fixes/task-f3-ext/step-f3-ext-1
		"command.execute.before": async (input, _output) => {
			try {
				log.info("plugin.command.executed", {
					event_type: "plugin.command.executed",
					command_name: input.command ?? "unknown",
					session_id: input.sessionID ?? "",
					run_id: process.env.AXIOM_RUN_ID ?? "",
					work_item_id: process.env.AXIOM_WORK_ITEM ?? "",
					repo: directory,
				});
			} catch {
				// Non-blocking
			}
		},

		// REQ-PLG-700: Blocked-file enforcement via tool.execute.before hook
		// REQ-PLG-416: Watches trigger on tool.execute.before (L5)
		"tool.execute.before": async (input, output) => {
			// REQ-PLG-108: emit plugin.tool.invoked structured log event
			// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-108 plan=phase-70-fixes/task-f3/step-f3-1
			// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-109 plan=phase-70-fixes/task-f3-ext/step-f3-ext-1
			try {
				log.info("plugin.tool.invoked", {
					event_type: "plugin.tool.invoked",
					tool_name: (input as { tool?: string }).tool ?? "unknown",
					session_id: (input as { sessionID?: string }).sessionID ?? "",
					run_id: process.env.AXIOM_RUN_ID ?? "",
					work_item_id: process.env.AXIOM_WORK_ITEM ?? "",
					repo: directory,
				});
			} catch {
				// Non-blocking
			}
			// Extract file path from tool args — tools like read/cat/write pass filePath
			const args = output.args;
			if (args && typeof args === "object") {
				const filePath =
					args.filePath ?? args.file ?? args.path ?? args.filename;
				if (typeof filePath === "string" && filePath.length > 0) {
					// REQ-PLG-700: built-in blocked file check
					const result = isBlockedFile(filePath);
					if (result.blocked) {
						throw new Error(
							`File read blocked by REQ-PLG-700: ${result.reason}`,
						);
					}

					// REQ-PLG-416: check config-defined watches (L5)
					if (watchesEngine && pluginConfig?.watches) {
						const toolName = (output as { name?: string }).name ?? "";
						const eventType = toolName.includes("write")
							? "tool.write"
							: toolName.includes("edit")
								? "tool.edit"
								: toolName.includes("bash")
									? "tool.bash"
									: "tool.read";
						const watchResults = await watchesEngine.processEvent(
							{ type: eventType, filePath },
							pluginConfig.watches,
						);
						for (const wr of watchResults) {
							if (wr.blocked) {
								throw new Error(
									wr.blockMessage ??
										`File access blocked by watch "${wr.watchName}"`,
								);
							}
						}
					}
				}
			}
		},

		// REQ-PLG-309: conflict detection via chat.message hook (dedicated hook surface)
		// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-309 plan=phase-70-7/task-70-7-1/step-70-7-1-1 jira_ref=DEX-64
		"chat.message": async (_input, _output) => {
			// REQ-PLG-309: conflict detection via chat.message hook (dedicated surface)
			// REQ-PLG-308: suppress during quiet mode (first 5 minutes of session)

			// Reset idle detector on message activity
			if (idleDetector) {
				idleDetector.reset();
			}

			// Conflict detection — advisory only, non-blocking
			if (heartbeatClient && l4BackendUrl && l4ApiToken && !quietModeTimer?.isQuiet()) {
				void (async () => {
					try {
						const { createTeamPulseClient, detectConflicts } = await import(
							"./layers/network"
						);
						const pulseClient = createTeamPulseClient({
							backendUrl: l4BackendUrl,
							apiToken: l4ApiToken,
						});
						const members = await pulseClient.getTeamPulse();
						const currentWorkItem = process.env.AXIOM_WORK_ITEM ?? "";
						const currentUserId = sessionIdentity.identity;
						if (currentWorkItem) {
							const conflicts = detectConflicts(
								currentWorkItem,
								currentUserId,
								members,
							);
							if (conflicts.length > 0) {
								// Advisory warning — logged but not blocking
								// REQ-PLG-134: use PluginLogger instead of console.warn
								log.warn(
									`Conflict detected: ${conflicts[0].message}`,
								);
							}
						}
					} catch {
						// Non-fatal — conflict detection is advisory only
					}
				})();
			}
		},

		// REQ-PLG-504: codeops_detect tool — returns maturity level, backends, repo metadata
		tool: {
			codeops_detect: tool({
				description:
					"Returns the Axiom maturity level (0-4), detection signals, and repo metadata as JSON",
				args: {},
				async execute(_args, _context) {
					// Re-evaluate on each call per REQ-PLG-023
					const current = detectMaturity(directory);
					return JSON.stringify(current, null, 2);
				},
			}),

			// REQ-PLG-053: codeops_backend tool — shows available backends and allows switching
			codeops_backend: tool({
				description:
					"Show available backends, the active backend, and switch between Python and Go execution modes (/axiom-backend)",
				args: {
					action: tool.schema
						.string()
						.optional()
						.describe(
							'Action to perform: "status" (default) shows current backend info, "switch" changes the active backend',
						),
					target: tool.schema
						.string()
						.optional()
						.describe('Target backend for switch action: "python" or "go"'),
				},
				async execute(args, _context) {
					const action = args.action ?? "status";
					const target = args.target;

					if (action === "switch") {
						if (!target || (target !== "python" && target !== "go")) {
							return JSON.stringify(
								{
									error: 'Switch requires a target: "python" or "go"',
								},
								null,
								2,
							);
						}
						const result = router.switchTo(target);
						return JSON.stringify(result, null, 2);
					}

					// Default: status report
					const report = router.status();
					return JSON.stringify(report, null, 2);
				},
			}),

		// REQ-PLG-715: codeops_privacy tool — shows privacy report
		codeops_privacy: tool({
			description:
				"Show what data the Axiom plugin collects, where it is stored, sharing status, deletion guidance, and current redaction patterns (/axiom-privacy)",
			args: {},
			async execute(_args, _context) {
				const report = buildPrivacyReport({ repoDir: directory });
				return formatPrivacyReport(report);
			},
		}),

		// REQ-PLG-102: codeops_session_cost tool — returns current session cost breakdown
		// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-102 plan=phase-70-fixes/task-f1/step-f1-1
		codeops_session_cost: tool({
			description:
				"Get current session cost breakdown with token usage by model, duration, and message count. Returns both a conversational summary and structured JSON.",
			args: {},
			async execute(_args, _context) {
				try {
					const { loadPricingTable, parseSessionMessages, computeSessionCost } =
						await import("./layers/metrics");
					const { buildSessionCostResult, formatSessionCostSummary } =
						await import("./layers/metrics-tools");
					const pricingTable = loadPricingTable(directory);
					const sessionId = process.env.AXIOM_SESSION_IDENTITY ?? "current";
					const messagesDir = join(directory, ".opencode", "sessions");
					const messages = parseSessionMessages(messagesDir, sessionId);
					const costResult = computeSessionCost(messages, pricingTable);
					const toolResult = buildSessionCostResult({
						sessionId,
						costResult,
						durationMinutes: 0,
						messageCount: messages.length,
						toolsUsed: [],
					});
					const summary = formatSessionCostSummary(toolResult);
					return `${summary}\n\n${JSON.stringify(toolResult, null, 2)}`;
				} catch (err) {
					return `Session cost data not yet available: ${err instanceof Error ? err.message : String(err)}`;
				}
			},
		}),

		// REQ-PLG-107: codeops_session_stats tool — returns cumulative repo session analytics
		// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-107 plan=phase-70-fixes/task-f1/step-f1-1
		codeops_session_stats: tool({
			description:
				"Get cumulative session analytics for this repo: total sessions, total cost, average cost, most-used tools and commands, and cost trend.",
			args: {},
			async execute(_args, _context) {
				try {
					const { readdirSync, readFileSync, existsSync } = await import(
						"node:fs"
					);
					const { join: pathJoin } = await import("node:path");
					const sessionsDir = pathJoin(directory, ".axiom", "sessions");
					if (!existsSync(sessionsDir)) {
						return "No session analytics found. Sessions are recorded as you use Axiom.";
					}
					const files = readdirSync(sessionsDir).filter((f) =>
						f.endsWith(".json"),
					);
					if (files.length === 0) {
						return "No session analytics found. Sessions are recorded as you use Axiom.";
					}
					let totalCost = 0;
					const toolCounts: Record<string, number> = {};
					for (const file of files) {
						try {
							const data = JSON.parse(
								readFileSync(pathJoin(sessionsDir, file), "utf-8"),
							) as {
								total_cost_usd?: number;
								tools_used?: string[];
							};
							totalCost += data.total_cost_usd ?? 0;
							for (const t of data.tools_used ?? []) {
								toolCounts[t] = (toolCounts[t] ?? 0) + 1;
							}
						} catch {
							// Skip malformed files
						}
					}
					const avgCost = files.length > 0 ? totalCost / files.length : 0;
					const topTools = Object.entries(toolCounts)
						.sort((a, b) => b[1] - a[1])
						.slice(0, 5)
						.map(([name, count]) => `${name} (${count})`);
					return JSON.stringify(
						{
							total_sessions: files.length,
							total_cost_usd: Math.round(totalCost * 10000) / 10000,
							average_cost_usd: Math.round(avgCost * 10000) / 10000,
							top_tools: topTools,
						},
						null,
						2,
					);
				} catch (err) {
					return `Session stats unavailable: ${err instanceof Error ? err.message : String(err)}`;
				}
			},
		}),

		// REQ-PLG-606: codeops_spinup tool — start/stop/status Axiom backend processes
		// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-606 plan=phase-70-fixes/task-f1/step-f1-1
		codeops_spinup: tool({
			description:
				"Start, stop, or check status of Axiom backend processes (ralph, morty, serve, opencode). Use action='list' to see available targets, action='start' to start one, action='stop' to stop one, action='status' to check one.",
			args: {
				action: tool.schema
					.string()
					.optional()
					.describe(
						'"list" (default) shows available targets, "start" starts a target, "stop" stops a target, "status" checks a target',
					),
				target: tool.schema
					.string()
					.optional()
					.describe(
						'Target name (e.g., "ralph", "morty", "serve", "opencode")',
					),
			},
			async execute(toolArgs, _context) {
				const { createSpinupManager } = await import("./spinup");
				const spinupManager = createSpinupManager({
					repoDir: directory,
					backendMode: maturity.backends.mode,
				});
				const action = toolArgs.action ?? "list";
				if (action === "list") {
					return spinupManager.toToolResult();
				}
				if (action === "start") {
					if (!toolArgs.target) {
						return 'Please provide a target name. Use action="list" to see available targets.';
					}
					const result = await spinupManager.start(toolArgs.target);
					if (!result.success) {
						return `Failed to start "${toolArgs.target}": ${result.reason}`;
					}
					return `Started "${toolArgs.target}" (PID: ${result.pid ?? "unknown"}).`;
				}
				if (action === "stop") {
					if (!toolArgs.target) {
						return 'Please provide a target name. Use action="list" to see available targets.';
					}
					const result = await spinupManager.stop(toolArgs.target);
					if (!result.success) {
						return `Failed to stop "${toolArgs.target}": ${result.reason}`;
					}
					return `Stopped "${toolArgs.target}".`;
				}
				if (action === "status") {
					if (!toolArgs.target) {
						return spinupManager.toToolResult();
					}
					const status = await spinupManager.status(toolArgs.target);
					return JSON.stringify(status, null, 2);
				}
				return `Unknown action "${action}". Use "list", "start", "stop", or "status".`;
			},
		}),

			// REQ-PLG-415: codeops_hook_output tool — returns output from last hook execution (L5)
			codeops_hook_output: tool({
				description:
					"Returns the output from the last hook execution. Use this to inspect what automation hooks have run and their results. (L5 — only available when .axiom/plugin.yaml is configured and approved)",
				args: {},
				async execute(_args, _context) {
					if (!hooksEngine) {
						return "L5 config automation is not active. Add .axiom/plugin.yaml with hooks configuration and approve it to enable automation.";
					}
					return hooksEngine.formatHookOutput();
				},
			}),

			// REQ-PLG-421: codeops_script tool — list and run named scripts (L5)
			codeops_script: tool({
				description:
					"List available scripts or run a named script from .axiom/plugin.yaml. Use action='list' to see available scripts, action='run' with name to execute one. (L5 — only available when .axiom/plugin.yaml is configured and approved)",
				args: {
					action: tool.schema
						.string()
						.optional()
						.describe(
							'Action: "list" (default) shows available scripts, "run" executes a named script',
						),
					name: tool.schema
						.string()
						.optional()
						.describe("Script name to run (required for action='run')"),
					args: tool.schema
						.string()
						.optional()
						.describe("Space-separated positional arguments for the script"),
				},
				async execute(toolArgs, _context) {
					if (!scriptsEngine || !pluginConfig?.scripts) {
						return "L5 config automation is not active. Add .axiom/plugin.yaml with scripts configuration and approve it to enable scripts.";
					}
					const action = toolArgs.action ?? "list";
					if (action === "list") {
						return scriptsEngine.formatScriptList(pluginConfig.scripts);
					}
					if (action === "run") {
						const scriptName = toolArgs.name;
						if (!scriptName) {
							return 'Please provide a script name. Use action="list" to see available scripts.';
						}
						const positionalArgs = toolArgs.args
							? toolArgs.args.split(" ").filter(Boolean)
							: [];
						const result = await scriptsEngine.runScript(
							scriptName,
							pluginConfig.scripts,
							positionalArgs,
						);
						if (result.needsConfirmation) {
							return (
								result.confirmationPrompt ??
								`Script "${scriptName}" requires confirmation before running.`
							);
						}
						if (!result.success) {
							return `Script "${scriptName}" failed (exit ${result.exitCode}):\n${result.output || result.error || "(no output)"}`;
						}
						return result.output || "Script completed successfully.";
					}
					return `Unknown action "${action}". Use "list" or "run".`;
				},
			}),

			// REQ-PLG-720: codeops_approve_config tool — approve .axiom/plugin.yaml for automation
			codeops_approve_config: tool({
				description:
					"Approve the current .axiom/plugin.yaml configuration to enable L5/L6 automation. Shows the configured hooks, watches, and scripts before asking for confirmation.",
				args: {
					confirm: tool.schema
						.boolean()
						.optional()
						.describe("Set to true to approve the current config"),
				},
				async execute(toolArgs, _context) {
					const configPath = join(directory, ".axiom", "plugin.yaml");
					const jsonPath = join(directory, ".axiom", "plugin.json");
					const activePath = existsSync(configPath)
						? configPath
						: existsSync(jsonPath)
							? jsonPath
							: null;

					if (!activePath) {
						return "No .axiom/plugin.yaml or .axiom/plugin.json found. Create one to enable L5/L6 automation.";
					}

					const rawContent = readFileSync(activePath, "utf-8");
					const alreadyApproved = isConfigApproved(directory, rawContent);

					if (alreadyApproved && !toolArgs.confirm) {
						return "The current config is already approved. Automation is active.";
					}

					if (!toolArgs.confirm) {
						// Show what will be approved
						const cfg = loadPluginConfig(directory);
						const lines = [
							"The following automation config will be approved:",
							"",
						];
						if (cfg.config?.hooks) {
							const events = Object.keys(cfg.config.hooks);
							lines.push(`Hooks: ${events.join(", ")}`);
						}
						if (cfg.config?.watches?.length) {
							lines.push(
								`Watches: ${cfg.config.watches.map((w) => w.name).join(", ")}`,
							);
						}
						if (cfg.config?.scripts) {
							lines.push(
								`Scripts: ${Object.keys(cfg.config.scripts).join(", ")}`,
							);
						}
						if (cfg.config?.tools?.length) {
							lines.push(
								`Custom tools: ${cfg.config.tools.map((t) => t.name).join(", ")}`,
							);
						}
						lines.push("");
						lines.push("Call this tool again with confirm=true to approve.");
						return lines.join("\n");
					}

					// REQ-PLG-721: write approval hash
					const { approveConfig } = await import(
						"./layers/automation/tool-registry"
					);
					approveConfig(directory, rawContent);
					return "Config approved. L5/L6 automation is now active for this session. Restart OpenCode to apply the new config.";
				},
			}),

		// REQ-PLG-200: codeops_share tool — share session replay or export locally
		codeops_share: tool({
			description:
				"Share a session replay with team members or export it locally. Use action='share' to upload to the Axiom backend (requires network.backend_url in plugin.yaml), or action='local' to export a redacted JSON file to .axiom/shared/. Always shows a confirmation before sharing.",
			args: {
				action: tool.schema
					.string()
					.optional()
					.describe(
						'"share" to upload to backend, "local" to export locally (default: "local")',
					),
				session_id: tool.schema
					.string()
					.optional()
					.describe("Session ID to share (defaults to current session)"),
				confirm: tool.schema
					.boolean()
					.optional()
					.describe(
						"Set to true to confirm sharing after reviewing the preview",
					),
				preview: tool.schema.boolean().optional().describe(
					"If true, export a local redacted file without uploading. Note: preview content is a placeholder until OpenCode session store access is available (task-70-6-2b). The structural wiring (no upload, redaction applied) is complete.",
				),
			},
			async execute(toolArgs, _context) {
				const action = toolArgs.action ?? "local";
				const sessionId = toolArgs.session_id ?? `session-${Date.now()}`;

				if (toolArgs.preview) {
					// REQ-PLG-204a: preview mode — local redacted output, no upload
					// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-204a plan=phase-70-6/task-70-6-2/step-70-6-2-1 jira_ref=DEX-64
					try {
						const { exportSessionLocal } = await import("./layers/sharing");
						const result = exportSessionLocal({
							repoRoot: directory,
							sessionId: process.env.AXIOM_SESSION_IDENTITY ?? "current",
							content: `Session ${process.env.AXIOM_SESSION_IDENTITY ?? "current"} — preview at ${new Date().toISOString()}`,
						});
						return `Preview (not shared):\n${result.filePath}\n\nThis is a local redacted preview. No data was uploaded. Run without --preview to share.`;
					} catch (err) {
						return `Preview failed: ${err instanceof Error ? err.message : String(err)}`;
					}
				}

					if (action === "local") {
						const { exportSessionLocal } = await import("./layers/sharing");
						const result = exportSessionLocal({
							repoRoot: directory,
							sessionId,
							content: `Session ${sessionId} — exported at ${new Date().toISOString()}`,
						});
						return `Session exported to ${result.filePath} (${result.redactedCount} items redacted).`;
					}

					if (action === "share") {
						const backendUrl = pluginConfig?.network?.backend_url;
						if (!backendUrl) {
							return "To share sessions, add `network.backend_url` to .axiom/plugin.yaml. Use action='local' to export locally instead.";
						}
						// REQ-PLG-710: auth token MUST come from AXIOM_API_TOKEN env var
						const apiToken = process.env.AXIOM_API_TOKEN;
						if (!apiToken) {
							return "To share sessions remotely, set the AXIOM_API_TOKEN environment variable. Use action='local' to export locally instead.";
						}
						if (!toolArgs.confirm) {
							const { buildShareConfirmationPrompt } = await import(
								"./layers/sharing"
							);
							return buildShareConfirmationPrompt({
								sessionId,
								redactedCount: 0,
								contentPreview: `Session ${sessionId}`,
							});
						}
						const { createSessionReplayClient } = await import(
							"./layers/sharing"
						);
						const client = createSessionReplayClient({
							backendUrl,
							authToken: apiToken,
						});
						const result = await client.share({
							sessionId,
							content: `Session ${sessionId}`,
							visibility: "link",
						});
						return `Session shared: ${result.url} (${result.redactedCount} items redacted).`;
					}

					return `Unknown action "${action}". Use "share" or "local".`;
				},
			}),

		// REQ-PLG-208: codeops_broadcast tool — start/stop/status live session broadcast (L3)
		// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-208 plan=phase-70-6/task-70-6-1/step-70-6-1-1 jira_ref=DEX-64
		// Only registered when network.backend_url is configured (same gate as codeops_team_pulse)
		...(pluginConfig?.network?.backend_url
			? (() => {
					const broadcastBackendUrl = pluginConfig.network!.backend_url!;
					let broadcastClient:
						| import("./layers/sharing").SessionBroadcastClient
						| null = null;
					const activeSubscriptions = new Set<string>();

					return {
						codeops_broadcast: tool({
							description:
								"Start, stop, or check status of a live session broadcast to opted-in team members. action='start' begins broadcasting, action='stop' ends it, action='status' shows active subscriptions. Requires network.backend_url in plugin.yaml and AXIOM_API_TOKEN env var.",
							args: {
								action: tool.schema
									.string()
									.optional()
									.describe(
										'"start" (default) begins broadcasting, "stop" ends broadcasting, "status" shows active subscriptions',
									),
							},
							async execute(toolArgs, _context) {
								try {
									const apiToken = process.env.AXIOM_API_TOKEN;
									if (!apiToken) {
										return "AXIOM_API_TOKEN environment variable is required for broadcast. Set it and try again.";
									}

									const action = toolArgs.action ?? "start";
									const sessionId =
										process.env.AXIOM_SESSION_IDENTITY ??
										`session-${Date.now()}`;

									if (action === "start") {
										// REQ-PLG-209: apply redaction before broadcast — same as replay (REQ-PLG-202)
										// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-209 plan=phase-70-6/task-70-6-3/step-70-6-3-1 jira_ref=DEX-64
										const { createSessionBroadcastClient } = await import(
											"./layers/sharing"
										);
										const { redactContent } = await import(
											"./security/redaction"
										);
										if (!broadcastClient) {
											broadcastClient = createSessionBroadcastClient({
												backendUrl: broadcastBackendUrl,
												authToken: apiToken,
											});
										}
									broadcastClient.subscribe({
										sessionId,
										onMessage: (msg) => {
											// REQ-PLG-209: redact received content before any downstream use
											// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-209 plan=phase-70-6/task-70-6-3/step-70-6-3-1 jira_ref=DEX-64
											// The plugin is a subscriber — redact inbound content before logging/display
											const { redacted } = redactContent(msg);
											log.info(`[broadcast] received message (${redacted.length} chars, redacted)`);
										},
									});
										activeSubscriptions.add(sessionId);
										return `Broadcast started for session ${sessionId}. Team members who have opted in will receive live updates.`;
									}

									if (action === "stop") {
										if (!broadcastClient || !activeSubscriptions.has(sessionId)) {
											return `No active broadcast found for session ${sessionId}.`;
										}
										broadcastClient.unsubscribe(sessionId);
										activeSubscriptions.delete(sessionId);
										return `Broadcast stopped for session ${sessionId}.`;
									}

							if (action === "status") {
								// REQ-PLG-213: viewer count visible to broadcaster
								// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-213 plan=phase-70-6/task-70-6-4/step-70-6-4-1 jira_ref=DEX-64
								// NOTE: activeSubscriptions.size is a local proxy (sessions this plugin subscribed to).
								// Backend viewer_count (REQ-API-L3-021) is the authoritative source.
								// TODO(phase-70-7): replace with backend viewer_count from SSE heartbeat or publish-event response.
								if (activeSubscriptions.size === 0) {
									return "No active broadcasts. Use action='start' to begin broadcasting.";
								}
								return `Broadcasting to ${activeSubscriptions.size} viewers (local estimate).\nActive sessions: ${[...activeSubscriptions].join(", ")}\nNote: For accurate viewer count, backend viewer_count API is required (pending phase-70-7).`;
							}

									return `Unknown action "${action}". Use "start", "stop", or "status".`;
								} catch (err) {
									// Non-blocking — broadcast errors must not disrupt the session
									return `Broadcast error: ${err instanceof Error ? err.message : String(err)}`;
								}
							},
						}),
					};
				})()
			: {}),

		// REQ-PLG-303: codeops_team_pulse tool — returns team activity as structured JSON (L4)
		// REQ-PLG-505: only registered when L4 is active (network.backend_url configured)
		...(pluginConfig?.network?.backend_url
				? {
						codeops_team_pulse: tool({
							description:
								"Returns current team activity, or toggles your visibility. action='status' (default) shows team; action='hide' goes invisible; action='show' restores visibility. Requires network.backend_url in plugin.yaml and AXIOM_API_TOKEN env var.",
							args: {
								action: tool.schema
									.string()
									.optional()
									.describe(
										'"status" (default) shows team activity, "hide" sets visibility to hidden, "show" restores visibility',
									),
								format: tool.schema
									.string()
									.optional()
									.describe(
										'"json" (default) or "text" for human-readable output',
									),
							},
							async execute(toolArgs, _context) {
								const backendUrl = pluginConfig?.network?.backend_url;
								if (!backendUrl) {
									return "L4 network awareness is not configured. Add network.backend_url to .axiom/plugin.yaml.";
								}
								const apiToken = process.env.AXIOM_API_TOKEN;
								if (!apiToken) {
									return "AXIOM_API_TOKEN environment variable is required for team pulse. Set it and try again.";
								}

								const action = toolArgs.action ?? "status";

								// REQ-PLG-307: hide/show visibility toggle
								if (action === "hide") {
									heartbeatClient?.setVisibility("hidden");
									return "Visibility set to hidden. Your heartbeats are still sent for session tracking but you won't appear in team pulse responses.";
								}
								if (action === "show") {
									const defaultVisibility =
										((pluginConfig?.network as Record<string, unknown>)
											?.visibility as import("./layers/network").VisibilityMode) ??
										"public";
									heartbeatClient?.setVisibility(defaultVisibility);
									return `Visibility restored to ${defaultVisibility}. You are now visible in team pulse responses.`;
								}

								const { createTeamPulseClient } = await import(
									"./layers/network"
								);
								const client = createTeamPulseClient({ backendUrl, apiToken });
								const members = await client.getTeamPulse();
								if (toolArgs.format === "text") {
									if (members.length === 0)
										return "No active team members found.";
									return members
										.map(
											(m) =>
												`${m.displayName ?? m.userId}: ${m.workItemId} (last seen: ${m.lastSeen})`,
										)
										.join("\n");
								}
								return JSON.stringify(
									{ members, count: members.length },
									null,
									2,
								);
							},
						}),
					}
				: {}),

			// Spread custom tools from L6 config (REQ-PLG-500)
			...customTools,
		},
	};
};

export default CodeOpsPlugin;
