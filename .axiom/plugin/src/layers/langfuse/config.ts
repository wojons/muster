/**
 * Langfuse Configuration — fetch from backend, cache to disk, fallback to env vars.
 *
 * REQ-LF-001: Proxy mode — plugin fetches config from backend
 * REQ-LF-002: Direct mode — plugin reads from env vars
 *
 * Config resolution order:
 * 1. Backend fetch (if network.backend_url set and langfuse.proxy_mode: true)
 * 2. Cache file (.axiom/langfuse-config.json) if fresh (TTL: 3600s default)
 * 3. Env vars (LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY, LANGFUSE_BASEURL, LANGFUSE_ENVIRONMENT)
 * 4. Defaults (base_url: "https://cloud.langfuse.com", environment: "development")
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-LF-001, #REQ-LF-002
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-LF-001,REQ-LF-002 plan=phase-3/task-3-2/step-3-2-1 jira_ref=DEX-403
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "fs"
import { join } from "path"

// ── Types ───────────────────────────────────────────────────────────────────

/** Langfuse configuration returned by the backend. */
export interface LangfuseConfig {
  public_key: string
  secret_key?: string
  base_url: string
  environment: string
}

/** Source of the resolved config. */
export type LangfuseConfigSource = "backend" | "cache" | "env_vars" | "defaults"

/** Result of config resolution. */
export interface LangfuseConfigResult {
  config: LangfuseConfig
  source: LangfuseConfigSource
  cached: boolean
}

/**
 * Plugin-level Langfuse configuration (from .axiom/plugin.yaml).
 * Used by config-loader.ts to type the langfuse field in PluginConfig.
 */
export interface LangfusePluginConfig {
  /** Whether Langfuse integration is enabled (opt-in, default: false). */
  enabled?: boolean
  /** Whether to use backend proxy mode (default: true). */
  proxy_mode?: boolean
  /** Cache TTL in seconds (default: 3600). */
  cache_ttl_seconds?: number
  /** Override otelEnabled detection (default: auto-detect from OTEL env vars). */
  otel_enabled?: boolean
}

/** Options for fetchLangfuseConfig(). */
export interface FetchLangfuseConfigOptions {
  /** Path to the repo root (for .axiom/ directory). */
  repoRoot: string
  /** Backend URL (e.g., "http://localhost:3000"). */
  backendUrl?: string
  /** API token for backend auth. */
  apiToken?: string
  /** Whether proxy mode is enabled. */
  proxyMode?: boolean
  /** Cache TTL in seconds (default: 3600). */
  cacheTtlSeconds?: number
  /** Internal: override fetch for testing. */
  _mockFetch?: () => Promise<{ public_key: string; base_url: string; environment: string }>
  /** Internal: override fs for testing. */
  _mockFs?: {
    existsSync?: typeof existsSync
    readFileSync?: typeof readFileSync
    writeFileSync?: typeof writeFileSync
    mkdirSync?: typeof mkdirSync
  }
}

// ── Constants ───────────────────────────────────────────────────────────────

const CACHE_FILE_NAME = "langfuse-config.json"
const DEFAULT_CACHE_TTL_SECONDS = 3600
const DEFAULT_BASE_URL = "https://cloud.langfuse.com"
const DEFAULT_ENVIRONMENT = "development"

// ── Cache helpers ───────────────────────────────────────────────────────────

interface CachedConfig {
  config: LangfuseConfig
  cached_at: number
  ttl_seconds: number
}

/**
 * Load cached config from disk.
 * Returns null if file doesn't exist, is invalid, or is stale.
 */
export function loadCachedConfig(
  cacheFilePath: string,
  fsImpl?: FetchLangfuseConfigOptions["_mockFs"],
): { config: LangfuseConfig; cached: boolean } | null {
  const fs = fsImpl ?? {
    existsSync,
    readFileSync,
  }

  if (!fs.existsSync!(cacheFilePath)) {
    return null
  }

  try {
    const content = fs.readFileSync!(cacheFilePath, "utf-8")
    const cached: CachedConfig = JSON.parse(content)

    if (!cached.config || !cached.cached_at || !cached.ttl_seconds) {
      return null
    }

    const now = Date.now()
    const ageSeconds = (now - cached.cached_at) / 1000

    if (ageSeconds > cached.ttl_seconds) {
      // Cache is stale
      return null
    }

    return { config: cached.config, cached: true }
  } catch {
    return null
  }
}

/**
 * Save config to cache file with timestamp and TTL.
 */
export function saveCachedConfig(
  cacheFilePath: string,
  config: LangfuseConfig,
  ttlSeconds: number,
  fsImpl?: FetchLangfuseConfigOptions["_mockFs"],
): void {
  const fs = fsImpl ?? {
    existsSync,
    mkdirSync,
    writeFileSync,
  }

  const dir = join(cacheFilePath, "..")
  fs.mkdirSync!(dir, { recursive: true })

  const cached: CachedConfig = {
    config,
    cached_at: Date.now(),
    ttl_seconds: ttlSeconds,
  }

  fs.writeFileSync!(cacheFilePath, JSON.stringify(cached, null, 2))
}

// ── Env var helpers ─────────────────────────────────────────────────────────

/**
 * Load config from environment variables.
 * Returns null if required env vars are missing.
 */
export function loadEnvVarConfig(): LangfuseConfig | null {
  const publicKey = process.env.LANGFUSE_PUBLIC_KEY
  const secretKey = process.env.LANGFUSE_SECRET_KEY

  if (!publicKey) {
    return null
  }

  return {
    public_key: publicKey,
    secret_key: secretKey,
    base_url: process.env.LANGFUSE_BASEURL ?? DEFAULT_BASE_URL,
    environment: process.env.LANGFUSE_ENVIRONMENT ?? DEFAULT_ENVIRONMENT,
  }
}

// ── Backend fetch ───────────────────────────────────────────────────────────

/**
 * Fetch config from the Axiom backend.
 * Returns null if fetch fails or returns non-OK.
 */
export async function fetchFromBackend(
  backendUrl: string,
  apiToken: string,
  mockFetch?: FetchLangfuseConfigOptions["_mockFetch"],
): Promise<LangfuseConfig | null> {
  try {
    let data: { public_key: string; base_url: string; environment: string }

    if (mockFetch) {
      data = await mockFetch()
    } else {
      const response = await fetch(`${backendUrl}/api/v1/langfuse/config`, {
        method: "GET",
        headers: {
          "Authorization": `Bearer ${apiToken}`,
          "Content-Type": "application/json",
        },
      })

      if (!response.ok) {
        return null
      }

      data = await response.json() as { public_key: string; base_url: string; environment: string }
    }

    return {
      public_key: data.public_key,
      base_url: data.base_url,
      environment: data.environment,
    }
  } catch {
    return null
  }
}

// ── Main API ───────────────────────────────────────────────────────────────

/**
 * Resolve Langfuse config with the following priority:
 * 1. Backend fetch (if proxyMode enabled and backendUrl provided)
 * 2. Cache file (if fresh)
 * 3. Environment variables
 * 4. Defaults
 *
 * @param options - Configuration options
 * @returns LangfuseConfigResult with resolved config and source
 */
export async function fetchLangfuseConfig(
  options: FetchLangfuseConfigOptions,
): Promise<LangfuseConfigResult> {
  const {
    repoRoot,
    backendUrl,
    apiToken,
    proxyMode = false,
    cacheTtlSeconds = DEFAULT_CACHE_TTL_SECONDS,
    _mockFetch,
    _mockFs,
  } = options

  const cacheFilePath = join(repoRoot, ".axiom", CACHE_FILE_NAME)

  // Step 1: Try backend fetch (if proxy mode enabled)
  if (proxyMode && backendUrl && apiToken) {
    const backendConfig = await fetchFromBackend(backendUrl, apiToken, _mockFetch)

    if (backendConfig) {
      // Save to cache
      saveCachedConfig(cacheFilePath, backendConfig, cacheTtlSeconds, _mockFs)

      return {
        config: backendConfig,
        source: "backend",
        cached: false,
      }
    }
  }

  // Step 2: Try cache
  const cachedResult = loadCachedConfig(cacheFilePath, _mockFs)
  if (cachedResult) {
    return {
      config: cachedResult.config,
      source: "cache",
      cached: true,
    }
  }

  // Step 3: Try env vars
  const envConfig = loadEnvVarConfig()
  if (envConfig) {
    return {
      config: envConfig,
      source: "env_vars",
      cached: false,
    }
  }

  // Step 4: Fall back to defaults
  return {
    config: {
      public_key: "",
      base_url: DEFAULT_BASE_URL,
      environment: DEFAULT_ENVIRONMENT,
    },
    source: "defaults",
    cached: false,
  }
}