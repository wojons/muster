/**
 * Tests for Langfuse config module — fetch/cache/fallback chain.
 *
 * REQ-LF-001: Core Langfuse OTEL Integration
 * REQ-LF-002: Backend Proxy Mode
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-LF-001, #REQ-LF-002
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-LF-001,REQ-LF-002 plan=phase-3/task-3-2/step-3-2-1 jira_ref=DEX-403
 */

import { describe, it, expect, afterEach } from "bun:test"
import {
  fetchLangfuseConfig,
  loadCachedConfig,
  saveCachedConfig,
  loadEnvVarConfig,
  fetchFromBackend,
  type LangfuseConfig,
} from "../src/layers/langfuse/config"

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeFreshCacheFs(config: LangfuseConfig, ttlSeconds = 3600) {
  const cached = JSON.stringify({
    config,
    cached_at: Date.now(),
    ttl_seconds: ttlSeconds,
  })
  return {
    existsSync: () => true,
    readFileSync: () => cached,
    writeFileSync: () => {},
    mkdirSync: () => {},
  }
}

function makeStaleCacheFs(config: LangfuseConfig) {
  const cached = JSON.stringify({
    config,
    cached_at: Date.now() - 7200 * 1000, // 2 hours ago
    ttl_seconds: 3600,
  })
  return {
    existsSync: () => true,
    readFileSync: () => cached,
    writeFileSync: () => {},
    mkdirSync: () => {},
  }
}

function makeEmptyCacheFs() {
  return {
    existsSync: () => false,
    readFileSync: () => { throw new Error("not found") },
    writeFileSync: () => {},
    mkdirSync: () => {},
  }
}

const sampleConfig: LangfuseConfig = {
  public_key: "pk-lf-test",
  base_url: "http://localhost:3000",
  environment: "test",
}

// ---------------------------------------------------------------------------
// loadCachedConfig
// ---------------------------------------------------------------------------

describe("loadCachedConfig", () => {
  it("returns null when file does not exist", () => {
    const result = loadCachedConfig("/tmp/nonexistent.json", makeEmptyCacheFs())
    expect(result).toBeNull()
  })

  it("returns config when cache is fresh", () => {
    const result = loadCachedConfig("/tmp/cache.json", makeFreshCacheFs(sampleConfig))
    expect(result).not.toBeNull()
    expect(result!.config.public_key).toBe("pk-lf-test")
    expect(result!.cached).toBe(true)
  })

  it("returns null when cache is stale", () => {
    const result = loadCachedConfig("/tmp/cache.json", makeStaleCacheFs(sampleConfig))
    expect(result).toBeNull()
  })

  it("returns null for invalid JSON", () => {
    const badFs = {
      existsSync: () => true,
      readFileSync: () => "not-json",
    }
    const result = loadCachedConfig("/tmp/cache.json", badFs)
    expect(result).toBeNull()
  })

  it("returns null for missing required fields", () => {
    const badFs = {
      existsSync: () => true,
      readFileSync: () => JSON.stringify({ config: null, cached_at: null }),
    }
    const result = loadCachedConfig("/tmp/cache.json", badFs)
    expect(result).toBeNull()
  })
})

// ---------------------------------------------------------------------------
// saveCachedConfig
// ---------------------------------------------------------------------------

describe("saveCachedConfig", () => {
  it("writes config with timestamp and TTL", () => {
    let written = ""
    const mockFs = {
      existsSync: () => false,
      mkdirSync: () => {},
      writeFileSync: (_path: string, content: string) => { written = content },
    }

    saveCachedConfig("/tmp/.axiom/langfuse-config.json", sampleConfig, 3600, mockFs)

    const parsed = JSON.parse(written)
    expect(parsed.config.public_key).toBe("pk-lf-test")
    expect(parsed.ttl_seconds).toBe(3600)
    expect(parsed.cached_at).toBeGreaterThan(0)
  })
})

// ---------------------------------------------------------------------------
// loadEnvVarConfig
// ---------------------------------------------------------------------------

describe("loadEnvVarConfig", () => {
  const savedEnv: Record<string, string | undefined> = {}

  afterEach(() => {
    for (const [key, val] of Object.entries(savedEnv)) {
      if (val === undefined) {
        delete process.env[key]
      } else {
        process.env[key] = val
      }
    }
  })

  it("returns null when LANGFUSE_PUBLIC_KEY is not set", () => {
    savedEnv.LANGFUSE_PUBLIC_KEY = process.env.LANGFUSE_PUBLIC_KEY
    delete process.env.LANGFUSE_PUBLIC_KEY
    expect(loadEnvVarConfig()).toBeNull()
  })

  it("returns config from env vars", () => {
    savedEnv.LANGFUSE_PUBLIC_KEY = process.env.LANGFUSE_PUBLIC_KEY
    savedEnv.LANGFUSE_SECRET_KEY = process.env.LANGFUSE_SECRET_KEY
    savedEnv.LANGFUSE_BASEURL = process.env.LANGFUSE_BASEURL
    savedEnv.LANGFUSE_ENVIRONMENT = process.env.LANGFUSE_ENVIRONMENT

    process.env.LANGFUSE_PUBLIC_KEY = "pk-env-test"
    process.env.LANGFUSE_SECRET_KEY = "sk-env-test"
    process.env.LANGFUSE_BASEURL = "http://localhost:3000"
    process.env.LANGFUSE_ENVIRONMENT = "staging"

    const result = loadEnvVarConfig()
    expect(result).not.toBeNull()
    expect(result!.public_key).toBe("pk-env-test")
    expect(result!.secret_key).toBe("sk-env-test")
    expect(result!.base_url).toBe("http://localhost:3000")
    expect(result!.environment).toBe("staging")
  })

  it("uses default base_url and environment when not set", () => {
    savedEnv.LANGFUSE_PUBLIC_KEY = process.env.LANGFUSE_PUBLIC_KEY
    savedEnv.LANGFUSE_BASEURL = process.env.LANGFUSE_BASEURL
    savedEnv.LANGFUSE_ENVIRONMENT = process.env.LANGFUSE_ENVIRONMENT

    process.env.LANGFUSE_PUBLIC_KEY = "pk-test"
    delete process.env.LANGFUSE_BASEURL
    delete process.env.LANGFUSE_ENVIRONMENT

    const result = loadEnvVarConfig()
    expect(result!.base_url).toBe("https://cloud.langfuse.com")
    expect(result!.environment).toBe("development")
  })
})

// ---------------------------------------------------------------------------
// fetchFromBackend
// ---------------------------------------------------------------------------

describe("fetchFromBackend", () => {
  it("returns config from successful backend response", async () => {
    const mockFetch = async () => ({
      public_key: "pk-backend",
      base_url: "http://langfuse.internal",
      environment: "production",
    })

    const result = await fetchFromBackend("http://backend", "token", mockFetch)
    expect(result).not.toBeNull()
    expect(result!.public_key).toBe("pk-backend")
    expect(result!.base_url).toBe("http://langfuse.internal")
  })

  it("returns null when mock fetch throws", async () => {
    const mockFetch = async () => { throw new Error("network error") }
    const result = await fetchFromBackend("http://backend", "token", mockFetch)
    expect(result).toBeNull()
  })
})

// ---------------------------------------------------------------------------
// fetchLangfuseConfig — full resolution chain
// ---------------------------------------------------------------------------

describe("fetchLangfuseConfig", () => {
  const savedEnv: Record<string, string | undefined> = {}

  afterEach(() => {
    for (const [key, val] of Object.entries(savedEnv)) {
      if (val === undefined) {
        delete process.env[key]
      } else {
        process.env[key] = val
      }
    }
  })

  it("returns backend config when proxy mode enabled and fetch succeeds", async () => {
    const result = await fetchLangfuseConfig({
      repoRoot: "/tmp/test-repo",
      backendUrl: "http://backend",
      apiToken: "token",
      proxyMode: true,
      _mockFetch: async () => ({
        public_key: "pk-from-backend",
        base_url: "http://langfuse.internal",
        environment: "production",
      }),
      _mockFs: makeEmptyCacheFs(),
    })

    expect(result.source).toBe("backend")
    expect(result.config.public_key).toBe("pk-from-backend")
    expect(result.cached).toBe(false)
  })

  it("returns cached config when cache is fresh", async () => {
    const result = await fetchLangfuseConfig({
      repoRoot: "/tmp/test-repo",
      proxyMode: false,
      _mockFs: makeFreshCacheFs(sampleConfig),
    })

    expect(result.source).toBe("cache")
    expect(result.config.public_key).toBe("pk-lf-test")
    expect(result.cached).toBe(true)
  })

  it("falls through stale cache to env vars", async () => {
    savedEnv.LANGFUSE_PUBLIC_KEY = process.env.LANGFUSE_PUBLIC_KEY
    process.env.LANGFUSE_PUBLIC_KEY = "pk-from-env"

    const result = await fetchLangfuseConfig({
      repoRoot: "/tmp/test-repo",
      proxyMode: false,
      _mockFs: makeStaleCacheFs(sampleConfig),
    })

    expect(result.source).toBe("env_vars")
    expect(result.config.public_key).toBe("pk-from-env")
  })

  it("falls back to defaults when nothing is configured", async () => {
    savedEnv.LANGFUSE_PUBLIC_KEY = process.env.LANGFUSE_PUBLIC_KEY
    delete process.env.LANGFUSE_PUBLIC_KEY

    const result = await fetchLangfuseConfig({
      repoRoot: "/tmp/test-repo",
      proxyMode: false,
      _mockFs: makeEmptyCacheFs(),
    })

    expect(result.source).toBe("defaults")
    expect(result.config.base_url).toBe("https://cloud.langfuse.com")
    expect(result.config.environment).toBe("development")
    expect(result.config.public_key).toBe("")
  })

  it("falls back to cache when backend fetch fails", async () => {
    const result = await fetchLangfuseConfig({
      repoRoot: "/tmp/test-repo",
      backendUrl: "http://backend",
      apiToken: "token",
      proxyMode: true,
      _mockFetch: async () => { throw new Error("backend down") },
      _mockFs: makeFreshCacheFs(sampleConfig),
    })

    expect(result.source).toBe("cache")
    expect(result.config.public_key).toBe("pk-lf-test")
  })

  it("skips backend when proxy_mode is false", async () => {
    let backendCalled = false
    const result = await fetchLangfuseConfig({
      repoRoot: "/tmp/test-repo",
      backendUrl: "http://backend",
      apiToken: "token",
      proxyMode: false,
      _mockFetch: async () => {
        backendCalled = true
        return { public_key: "pk-backend", base_url: "http://x", environment: "prod" }
      },
      _mockFs: makeFreshCacheFs(sampleConfig),
    })

    expect(backendCalled).toBe(false)
    expect(result.source).toBe("cache")
  })
})
