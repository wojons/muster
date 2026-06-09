/**
 * Live pricing fetch from models.dev API with 24h cache and offline fallback.
 *
 * REQ-PLG-104b: Fetch pricing from models.dev API; cache 24h; fallback to embedded table.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-104b
 * axiom:trace work_item=plugin-v2-enhancements-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-104b plan=phase-70-9/task-70-9-2/step-70-9-2-1
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

/** Pricing entry for a single model. */
export interface ModelPricing {
	prompt_per_1m: number;
	completion_per_1m: number;
}

/** Pricing table keyed by model ID. */
export type PricingTable = Record<string, ModelPricing>;

/** Cached pricing data with TTL. */
interface PricingCache {
	fetchedAt: number;
	pricing: PricingTable;
}

/** Options for fetchPricingWithCache(). */
export interface PricingFetchOptions {
	cacheDir: string;
	/** Cache TTL in ms (default: 24h). */
	cacheTtlMs?: number;
	/** Fetch timeout in ms (default: 2000). */
	fetchTimeoutMs?: number;
	/** User-configured overrides (always win). */
	pricingOverrides?: PricingTable;
}

const CACHE_FILE = ".pricing-cache.json";
const DEFAULT_TTL_MS = 24 * 60 * 60 * 1000; // 24 hours
const DEFAULT_TIMEOUT_MS = 2000;
const MODELS_DEV_URL = "https://models.dev/api/models";

/** Embedded fallback pricing table (updated with each plugin release). */
export const EMBEDDED_PRICING: PricingTable = {
	"claude-3-5-sonnet-20241022": { prompt_per_1m: 3.0, completion_per_1m: 15.0 },
	"claude-3-5-haiku-20241022": { prompt_per_1m: 0.8, completion_per_1m: 4.0 },
	"claude-3-opus-20240229": { prompt_per_1m: 15.0, completion_per_1m: 75.0 },
	"gpt-4o": { prompt_per_1m: 2.5, completion_per_1m: 10.0 },
	"gpt-4o-mini": { prompt_per_1m: 0.15, completion_per_1m: 0.6 },
	"gpt-4-turbo": { prompt_per_1m: 10.0, completion_per_1m: 30.0 },
	"gemini-1.5-pro": { prompt_per_1m: 1.25, completion_per_1m: 5.0 },
	"gemini-1.5-flash": { prompt_per_1m: 0.075, completion_per_1m: 0.3 },
};

/**
 * Fetch pricing from models.dev with 24h cache and offline fallback.
 *
 * REQ-PLG-104b:
 * - Fetch from models.dev API; cache in .axiom/.pricing-cache.json
 * - 24h TTL; re-fetch on expiry
 * - Graceful fallback to embedded table on any failure (offline, timeout, error)
 * - Non-blocking: returns immediately with cached/embedded data; fetches in background
 * - User pricingOverrides always win over fetched and embedded prices
 */
export async function fetchPricingWithCache(
	options: PricingFetchOptions,
): Promise<PricingTable> {
	const {
		cacheDir,
		cacheTtlMs = DEFAULT_TTL_MS,
		fetchTimeoutMs = DEFAULT_TIMEOUT_MS,
		pricingOverrides = {},
	} = options;

	const cachePath = join(cacheDir, CACHE_FILE);

	// Try to load from cache first
	const cached = _loadCache(cachePath, cacheTtlMs);
	if (cached) {
		return _applyOverrides(cached, pricingOverrides);
	}

	// Cache miss or expired — try to fetch from models.dev
	const fetched = await _fetchFromModelsDev(MODELS_DEV_URL, fetchTimeoutMs);
	if (fetched) {
		_saveCache(cachePath, fetched);
		return _applyOverrides(fetched, pricingOverrides);
	}

	// Fallback to embedded table
	return _applyOverrides(EMBEDDED_PRICING, pricingOverrides);
}

/**
 * Non-blocking version — returns embedded/cached pricing immediately,
 * triggers a background refresh if cache is stale.
 */
export function fetchPricingNonBlocking(
	options: PricingFetchOptions,
): PricingTable {
	const {
		cacheDir,
		cacheTtlMs = DEFAULT_TTL_MS,
		pricingOverrides = {},
	} = options;
	const cachePath = join(cacheDir, CACHE_FILE);

	// Return cached or embedded immediately
	const cached = _loadCache(cachePath, cacheTtlMs);
	const immediate = _applyOverrides(
		cached ?? EMBEDDED_PRICING,
		pricingOverrides,
	);

	// Trigger background refresh if cache is stale
	if (!cached) {
		void fetchPricingWithCache(options).catch(() => {
			// Swallow — background fetch failure is non-fatal
		});
	}

	return immediate;
}

// ── Internal helpers ──────────────────────────────────────────────────────

function _loadCache(cachePath: string, ttlMs: number): PricingTable | null {
	if (!existsSync(cachePath)) return null;
	try {
		const raw = readFileSync(cachePath, "utf-8");
		const cache: PricingCache = JSON.parse(raw);
		const age = Date.now() - cache.fetchedAt;
		if (age > ttlMs) return null; // expired
		return cache.pricing;
	} catch {
		return null;
	}
}

function _saveCache(cachePath: string, pricing: PricingTable): void {
	try {
		mkdirSync(require("node:path").dirname(cachePath), { recursive: true });
		const cache: PricingCache = { fetchedAt: Date.now(), pricing };
		writeFileSync(cachePath, JSON.stringify(cache, null, 2), "utf-8");
	} catch {
		// Non-fatal — cache write failure just means next call re-fetches
	}
}

async function _fetchFromModelsDev(
	url: string,
	timeoutMs: number,
): Promise<PricingTable | null> {
	try {
		const controller = new AbortController();
		const timer = setTimeout(() => controller.abort(), timeoutMs);
		const response = await fetch(url, { signal: controller.signal });
		clearTimeout(timer);
		if (!response.ok) return null;
		const data = (await response.json()) as unknown;
		return _parsePricingResponse(data);
	} catch {
		return null;
	}
}

function _parsePricingResponse(data: unknown): PricingTable | null {
	if (!data || typeof data !== "object") return null;
	const table: PricingTable = {};
	// models.dev returns an array or object of model entries
	const entries = Array.isArray(data)
		? data
		: Object.values(data as Record<string, unknown>);
	for (const entry of entries) {
		if (!entry || typeof entry !== "object") continue;
		const e = entry as Record<string, unknown>;
		const id =
			typeof e.id === "string"
				? e.id
				: typeof e.name === "string"
					? e.name
					: null;
		if (!id) continue;
		const pricing = e.pricing as Record<string, unknown> | undefined;
		if (!pricing) continue;
		const prompt =
			typeof pricing.input === "number"
				? pricing.input
				: typeof pricing.prompt === "number"
					? pricing.prompt
					: null;
		const completion =
			typeof pricing.output === "number"
				? pricing.output
				: typeof pricing.completion === "number"
					? pricing.completion
					: null;
		if (prompt !== null && completion !== null) {
			table[id] = { prompt_per_1m: prompt, completion_per_1m: completion };
		}
	}
	return Object.keys(table).length > 0 ? table : null;
}

function _applyOverrides(
	base: PricingTable,
	overrides: PricingTable,
): PricingTable {
	return { ...base, ...overrides };
}
