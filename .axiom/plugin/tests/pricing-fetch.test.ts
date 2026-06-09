/**
 * Pricing fetch tests — cache hit, cache miss, offline fallback, user overrides.
 *
 * REQ-PLG-104b: fetch with 24h cache, graceful offline fallback, user overrides win.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-104b
 * axiom:trace work_item=plugin-v2-enhancements-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-104b plan=phase-70-9/task-70-9-2/step-70-9-2-1
 */
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { existsSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
	EMBEDDED_PRICING,
	fetchPricingNonBlocking,
	fetchPricingWithCache,
	type PricingTable,
} from "../src/pricing-fetch";

function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-pricing-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
	);
	mkdirSync(dir, { recursive: true });
	return dir;
}

describe("REQ-PLG-104b: live pricing from models.dev", () => {
	let tempDir: string;

	beforeEach(() => {
		tempDir = makeTempDir();
	});

	afterEach(() => {
		if (existsSync(tempDir)) {
			rmSync(tempDir, { recursive: true, force: true });
		}
	});

	it("EMBEDDED_PRICING has entries for common models", () => {
		expect(Object.keys(EMBEDDED_PRICING).length).toBeGreaterThan(0);
		expect(EMBEDDED_PRICING["claude-3-5-sonnet-20241022"]).toBeDefined();
		expect(EMBEDDED_PRICING["gpt-4o"]).toBeDefined();
	});

	it("falls back to embedded table when offline (no network)", async () => {
		// Use a non-existent URL to simulate offline
		const result = await fetchPricingWithCache({
			cacheDir: tempDir,
			fetchTimeoutMs: 100,
		});
		// Should return embedded pricing (fallback)
		expect(result).toBeDefined();
		expect(Object.keys(result).length).toBeGreaterThan(0);
		// At least some embedded models should be present
		const hasEmbedded = Object.keys(EMBEDDED_PRICING).some(
			(k) => result[k] !== undefined,
		);
		expect(hasEmbedded).toBe(true);
	});

	it("cache hit: returns cached pricing without fetching", async () => {
		// Write a valid cache file
		const cachedPricing: PricingTable = {
			"test-model": { prompt_per_1m: 1.0, completion_per_1m: 2.0 },
		};
		const cacheData = {
			fetchedAt: Date.now(),
			pricing: cachedPricing,
		};
		writeFileSync(
			join(tempDir, ".pricing-cache.json"),
			JSON.stringify(cacheData),
		);

		const result = await fetchPricingWithCache({ cacheDir: tempDir });
		expect(result["test-model"]).toBeDefined();
		expect(result["test-model"].prompt_per_1m).toBe(1.0);
	});

	it("cache miss: expired cache triggers re-fetch (falls back to embedded)", async () => {
		// Write an expired cache file
		const expiredData = {
			fetchedAt: Date.now() - 25 * 60 * 60 * 1000, // 25 hours ago
			pricing: {
				"old-model": { prompt_per_1m: 99.0, completion_per_1m: 99.0 },
			},
		};
		writeFileSync(
			join(tempDir, ".pricing-cache.json"),
			JSON.stringify(expiredData),
		);

		const result = await fetchPricingWithCache({
			cacheDir: tempDir,
			fetchTimeoutMs: 100, // short timeout → offline fallback
		});
		// Should NOT return the expired cache data
		expect(result["old-model"]).toBeUndefined();
		// Should return embedded pricing
		const hasEmbedded = Object.keys(EMBEDDED_PRICING).some(
			(k) => result[k] !== undefined,
		);
		expect(hasEmbedded).toBe(true);
	});

	it("user pricingOverrides always win over cached and embedded prices", async () => {
		const overrides: PricingTable = {
			"claude-3-5-sonnet-20241022": {
				prompt_per_1m: 0.01,
				completion_per_1m: 0.02,
			},
		};
		const result = await fetchPricingWithCache({
			cacheDir: tempDir,
			fetchTimeoutMs: 100,
			pricingOverrides: overrides,
		});
		expect(result["claude-3-5-sonnet-20241022"].prompt_per_1m).toBe(0.01);
		expect(result["claude-3-5-sonnet-20241022"].completion_per_1m).toBe(0.02);
	});

	it("fetchPricingNonBlocking returns immediately with embedded pricing", () => {
		const result = fetchPricingNonBlocking({ cacheDir: tempDir });
		expect(result).toBeDefined();
		expect(Object.keys(result).length).toBeGreaterThan(0);
	});

	it("fetchPricingNonBlocking returns cached pricing when cache is fresh", () => {
		const cachedPricing: PricingTable = {
			"cached-model": { prompt_per_1m: 5.0, completion_per_1m: 10.0 },
		};
		writeFileSync(
			join(tempDir, ".pricing-cache.json"),
			JSON.stringify({ fetchedAt: Date.now(), pricing: cachedPricing }),
		);
		const result = fetchPricingNonBlocking({ cacheDir: tempDir });
		expect(result["cached-model"]).toBeDefined();
		expect(result["cached-model"].prompt_per_1m).toBe(5.0);
	});
});
