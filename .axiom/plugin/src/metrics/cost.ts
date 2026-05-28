/**
 * Cost module — integer microdollars arithmetic.
 *
 * REQ-PLG-131: All cost fields MUST use integer microdollars
 *   (1 USD = 1,000,000 microdollars) to avoid floating-point rounding errors.
 *   Field name: `estimated_cost_microdollars` (int).
 *   Display layers convert: `cost_usd = microdollars / 1_000_000`.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-131
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-131 jira_ref=DEX-392
 */

/** 1 USD = 1,000,000 microdollars. */
export const MICRODOLLARS_PER_USD = 1_000_000

/**
 * Convert USD (float) to integer microdollars.
 *
 * Uses Math.round to avoid floating-point drift.
 */
export function usdToMicrodollars(usd: number): number {
  return Math.round(usd * MICRODOLLARS_PER_USD)
}

/**
 * Convert integer microdollars to USD (float).
 *
 * For display purposes only — all internal arithmetic uses microdollars.
 */
export function microdollarsToUsd(microdollars: number): number {
  return microdollars / MICRODOLLARS_PER_USD
}

/** Pricing entry in microdollars per 1M tokens. */
export interface MicrodollarPricing {
  input_per_1m_microdollars: number
  output_per_1m_microdollars: number
  cache_read_per_1m_microdollars: number
  cache_write_per_1m_microdollars: number
  reasoning_per_1m_microdollars: number
}

/**
 * Convert a USD-based pricing entry to microdollar pricing.
 */
export function pricingToMicrodollars(pricing: {
  price_per_1m_input: number
  price_per_1m_output: number
  price_per_1m_cache_read: number
  price_per_1m_cache_write: number
  price_per_1m_reasoning: number
}): MicrodollarPricing {
  return {
    input_per_1m_microdollars: usdToMicrodollars(pricing.price_per_1m_input),
    output_per_1m_microdollars: usdToMicrodollars(pricing.price_per_1m_output),
    cache_read_per_1m_microdollars: usdToMicrodollars(pricing.price_per_1m_cache_read),
    cache_write_per_1m_microdollars: usdToMicrodollars(pricing.price_per_1m_cache_write),
    reasoning_per_1m_microdollars: usdToMicrodollars(pricing.price_per_1m_reasoning),
  }
}

/**
 * Compute cost in microdollars for a set of token counts.
 *
 * Formula (integer arithmetic):
 *   cost_microdollars = sum of (price_per_1m_microdollars * tokens / 1_000_000)
 *   for each token type.
 *
 * Uses Math.round on each term to maintain integer precision.
 */
export function computeCostMicrodollars(
  tokens: {
    input: number
    output: number
    cache_read: number
    cache_write: number
    reasoning?: number
  },
  pricing: MicrodollarPricing,
): number {
  const inputCost = Math.round((pricing.input_per_1m_microdollars * tokens.input) / 1_000_000)
  const outputCost = Math.round((pricing.output_per_1m_microdollars * tokens.output) / 1_000_000)
  const cacheReadCost = Math.round((pricing.cache_read_per_1m_microdollars * tokens.cache_read) / 1_000_000)
  const cacheWriteCost = Math.round((pricing.cache_write_per_1m_microdollars * tokens.cache_write) / 1_000_000)
  const reasoningCost = Math.round(
    (pricing.reasoning_per_1m_microdollars * (tokens.reasoning ?? 0)) / 1_000_000,
  )

  return inputCost + outputCost + cacheReadCost + cacheWriteCost + reasoningCost
}
