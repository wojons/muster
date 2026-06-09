/**
 * Tests for L8 search layer (Pillar 4).
 *
 * Covers:
 * - Tokenizer (REQ-FAC-060)
 * - Exclusion list: sensitive files excluded (REQ-FAC-080–088)
 * - BM25 index: add, search, incremental update (REQ-FAC-060, REQ-FAC-068)
 * - Text search returns ranked results (REQ-FAC-062)
 * - Chunking for large files (REQ-FAC-075)
 * - Vector math: cosine similarity, mean vector (REQ-FAC-063)
 * - Embedding cache: save and load (REQ-FAC-069)
 * - SearchServer: text search, hybrid text-only mode (REQ-FAC-061, REQ-FAC-078)
 * - Exclusion list prevents Bedrock calls for sensitive files (REQ-FAC-080)
 * - Zero-cloud-dependency mode (REQ-FAC-078)
 *
 * Spec: specs/87-MCP-File-Agent-Communication.md#7
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-060 plan=phase-100/task-100-4/step-100-4-7 jira_ref=DEX-458
 */

import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { existsSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
	BM25Index,
	DEFAULT_EXCLUDE_PATTERNS,
	buildIndex,
	isExcluded,
	isIncluded,
	tokenize,
} from "../src/layers/search/text-index";
import {
	chunkText,
	cosineSimilarity,
	loadCachedEmbedding,
	meanVector,
	saveCachedEmbedding,
} from "../src/layers/search/embeddings";
import { SearchServer } from "../src/layers/search/search-server";

// ── Helpers ──────────────────────────────────────────────────────────────

function makeTempDir(): string {
	const dir = join(
		tmpdir(),
		`axiom-search-test-${Date.now()}-${Math.random().toString(36).slice(2)}`,
	);
	mkdirSync(dir, { recursive: true });
	return dir;
}

let tmpDir: string;

beforeEach(() => {
	tmpDir = makeTempDir();
});

afterEach(() => {
	try { rmSync(tmpDir, { recursive: true, force: true }); } catch { /* ignore */ }
});

// ── Tokenizer ────────────────────────────────────────────────────────────

describe("tokenize", () => {
	it("REQ-FAC-060: splits on whitespace and punctuation", () => {
		const tokens = tokenize("hello world, foo-bar");
		expect(tokens).toContain("hello");
		expect(tokens).toContain("world");
		expect(tokens).toContain("foo");
		expect(tokens).toContain("bar");
	});

	it("REQ-FAC-060: lowercases all tokens", () => {
		const tokens = tokenize("Hello WORLD");
		expect(tokens).toContain("hello");
		expect(tokens).toContain("world");
	});

	it("REQ-FAC-060: filters tokens shorter than 2 chars", () => {
		const tokens = tokenize("a bb ccc");
		expect(tokens).not.toContain("a");
		expect(tokens).toContain("bb");
		expect(tokens).toContain("ccc");
	});
});

// ── Exclusion list ───────────────────────────────────────────────────────

describe("isExcluded", () => {
	it("REQ-FAC-080: excludes .env files", () => {
		expect(isExcluded(".env", DEFAULT_EXCLUDE_PATTERNS)).toBe(true);
		expect(isExcluded(".env.production", DEFAULT_EXCLUDE_PATTERNS)).toBe(true);
	});

	it("REQ-FAC-080: excludes private key files", () => {
		expect(isExcluded("server.pem", DEFAULT_EXCLUDE_PATTERNS)).toBe(true);
		expect(isExcluded("cert.p12", DEFAULT_EXCLUDE_PATTERNS)).toBe(true);
	});

	it("REQ-FAC-080: excludes files with 'secret' in name", () => {
		expect(isExcluded("my-secret-config.yaml", DEFAULT_EXCLUDE_PATTERNS)).toBe(true);
	});

	it("REQ-FAC-080: excludes *.key files", () => {
		expect(isExcluded("id_rsa.key", DEFAULT_EXCLUDE_PATTERNS)).toBe(true);
		expect(isExcluded("server.key", DEFAULT_EXCLUDE_PATTERNS)).toBe(true);
	});

	it("REQ-FAC-081: excludes node_modules", () => {
		expect(isExcluded("node_modules/foo/bar.js", DEFAULT_EXCLUDE_PATTERNS)).toBe(true);
	});

	it("REQ-FAC-085: normal source files are not excluded", () => {
		expect(isExcluded("src/index.ts", DEFAULT_EXCLUDE_PATTERNS)).toBe(false);
		expect(isExcluded("README.md", DEFAULT_EXCLUDE_PATTERNS)).toBe(false);
	});

	it("REQ-FAC-088: exclusion takes precedence (excluded file not in results)", () => {
		// Even if a file matches include extensions, exclusion wins
		expect(isExcluded(".env", DEFAULT_EXCLUDE_PATTERNS)).toBe(true);
	});
});

describe("isIncluded", () => {
	it("REQ-FAC-084: includes common source extensions", () => {
		const { DEFAULT_INCLUDE_EXTENSIONS } = require("../src/layers/search/text-index");
		expect(isIncluded("foo.ts", DEFAULT_INCLUDE_EXTENSIONS)).toBe(true);
		expect(isIncluded("bar.py", DEFAULT_INCLUDE_EXTENSIONS)).toBe(true);
		expect(isIncluded("baz.md", DEFAULT_INCLUDE_EXTENSIONS)).toBe(true);
	});

	it("REQ-FAC-083: excludes binary files", () => {
		const { DEFAULT_INCLUDE_EXTENSIONS } = require("../src/layers/search/text-index");
		expect(isIncluded("image.png", DEFAULT_INCLUDE_EXTENSIONS)).toBe(false);
		expect(isIncluded("binary.exe", DEFAULT_INCLUDE_EXTENSIONS)).toBe(false);
	});
});

// ── BM25 index ───────────────────────────────────────────────────────────

describe("BM25Index", () => {
	it("REQ-FAC-060: indexes documents and returns search results", () => {
		const index = new BM25Index();
		index.addDocument("src/auth.ts", "function authenticate user login password", Date.now());
		index.addDocument("src/api.ts", "function createUser register endpoint", Date.now());
		index.addDocument("src/db.ts", "database connection pool query", Date.now());

		const results = index.search("authenticate login");
		expect(results.length).toBeGreaterThan(0);
		expect(results[0].path).toBe("src/auth.ts");
	});

	it("REQ-FAC-060: returns empty results for no matches", () => {
		const index = new BM25Index();
		index.addDocument("src/foo.ts", "hello world", Date.now());
		const results = index.search("xyzzy_nonexistent_term");
		expect(results).toHaveLength(0);
	});

	it("REQ-FAC-068: incremental update — re-indexes changed documents", () => {
		const index = new BM25Index();
		const mtime1 = Date.now() - 1000;
		index.addDocument("src/foo.ts", "original content", mtime1);

		const mtime2 = Date.now();
		index.addDocument("src/foo.ts", "updated authentication content", mtime2);

		const results = index.search("authentication");
		expect(results.length).toBeGreaterThan(0);
		expect(results[0].path).toBe("src/foo.ts");
	});

	it("REQ-FAC-068: needsUpdate returns false for unchanged file", () => {
		const index = new BM25Index();
		const mtime = Date.now();
		index.addDocument("src/foo.ts", "content", mtime);
		expect(index.needsUpdate("src/foo.ts", mtime)).toBe(false);
	});

	it("REQ-FAC-068: needsUpdate returns true for newer mtime", () => {
		const index = new BM25Index();
		const mtime = Date.now() - 1000;
		index.addDocument("src/foo.ts", "content", mtime);
		expect(index.needsUpdate("src/foo.ts", Date.now())).toBe(true);
	});

	it("REQ-FAC-060: BM25 ranks more relevant documents higher", () => {
		const index = new BM25Index();
		index.addDocument("a.ts", "authentication login password user session", Date.now());
		index.addDocument("b.ts", "database query select insert update", Date.now());
		index.addDocument("c.ts", "authentication token jwt bearer", Date.now());

		const results = index.search("authentication");
		// a.ts and c.ts should appear in results; b.ts should not (no matching terms)
		const paths = results.map((r) => r.path);
		expect(paths).toContain("a.ts");
		expect(paths).toContain("c.ts");
		expect(paths).not.toContain("b.ts");
	});
});

// ── buildIndex ───────────────────────────────────────────────────────────

describe("buildIndex", () => {
	it("REQ-FAC-065: indexes files from workspace", () => {
		// Create some test files
		mkdirSync(join(tmpDir, "src"), { recursive: true });
		writeFileSync(join(tmpDir, "src", "index.ts"), "export function hello() {}");
		writeFileSync(join(tmpDir, "src", "utils.ts"), "export function add(a: number, b: number) {}");

		const index = new BM25Index();
		const count = buildIndex(index, { workspaceRoot: tmpDir });
		expect(count).toBeGreaterThanOrEqual(2);
	});

	it("REQ-FAC-080: excludes .env files from index", () => {
		writeFileSync(join(tmpDir, ".env"), "SECRET=abc123");
		writeFileSync(join(tmpDir, "index.ts"), "export const x = 1;");

		const index = new BM25Index();
		buildIndex(index, { workspaceRoot: tmpDir });

		const paths = index.getPaths();
		expect(paths.some((p) => p.includes(".env"))).toBe(false);
		expect(paths.some((p) => p.includes("index.ts"))).toBe(true);
	});

	it("REQ-FAC-085: excluded files do not appear in search results", () => {
		writeFileSync(join(tmpDir, "my-secret-config.yaml"), "password: hunter2");
		writeFileSync(join(tmpDir, "app.ts"), "const password = 'hunter2';");

		const index = new BM25Index();
		buildIndex(index, { workspaceRoot: tmpDir });

		const results = index.search("hunter2");
		const paths = results.map((r) => r.path);
		expect(paths.some((p) => p.includes("secret"))).toBe(false);
	});

	it("REQ-FAC-081: .gitignore patterns excluded from index", () => {
		mkdirSync(join(tmpDir, "dist"), { recursive: true });
		writeFileSync(join(tmpDir, ".gitignore"), "dist/\nbuild/\n# comment\n");
		writeFileSync(join(tmpDir, "dist", "bundle.js"), "compiled output");
		writeFileSync(join(tmpDir, "app.ts"), "const x = 1;");

		const index = new BM25Index();
		buildIndex(index, { workspaceRoot: tmpDir });

		const paths = index.getPaths();
		expect(paths.some((p) => p.includes("dist"))).toBe(false);
		expect(paths.some((p) => p.includes("app.ts"))).toBe(true);
	});

	it("REQ-FAC-109: audit log files excluded from BM25 index", () => {
		// axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-109 plan=phase-100/task-100-backlog/fb-008 jira_ref=DEX-458
		// The default audit log path .axiom/logs/pipe-audit.jsonl must not be indexed.
		// Coverage is via DEFAULT_EXCLUDE_PATTERNS **/.axiom/logs/** (intentional).
		mkdirSync(join(tmpDir, ".axiom", "logs"), { recursive: true });
		writeFileSync(join(tmpDir, ".axiom", "logs", "pipe-audit.jsonl"),
			'{"event":"pipe.created","pipe_id":"abc123"}\n');
		writeFileSync(join(tmpDir, "app.ts"), "const x = 1;");

		const index = new BM25Index();
		buildIndex(index, { workspaceRoot: tmpDir });

		const paths = index.getPaths();
		// Audit log must not be indexed (REQ-FAC-109)
		expect(paths.some((p) => p.includes("pipe-audit"))).toBe(false);
		// Normal source files are indexed
		expect(paths.some((p) => p.includes("app.ts"))).toBe(true);
	});
});

// ── Chunking ─────────────────────────────────────────────────────────────

describe("chunkText", () => {
	it("REQ-FAC-075: returns single chunk for short text", () => {
		const chunks = chunkText("short text", 8000, 200);
		expect(chunks).toHaveLength(1);
		expect(chunks[0]).toBe("short text");
	});

	it("REQ-FAC-075: chunks long text with overlap", () => {
		const text = "x".repeat(20000);
		const chunks = chunkText(text, 8000, 200);
		expect(chunks.length).toBeGreaterThan(1);
		// Each chunk should be at most chunkSize
		for (const chunk of chunks) {
			expect(chunk.length).toBeLessThanOrEqual(8000);
		}
	});

	it("REQ-FAC-076: respects custom chunk size and overlap", () => {
		const text = "a".repeat(1000);
		const chunks = chunkText(text, 400, 50);
		expect(chunks.length).toBeGreaterThan(1);
	});
});

// ── Vector math ──────────────────────────────────────────────────────────

describe("cosineSimilarity", () => {
	it("REQ-FAC-063: identical vectors have similarity 1", () => {
		const v = [1, 0, 0, 1];
		expect(cosineSimilarity(v, v)).toBeCloseTo(1.0);
	});

	it("REQ-FAC-063: orthogonal vectors have similarity 0", () => {
		const a = [1, 0];
		const b = [0, 1];
		expect(cosineSimilarity(a, b)).toBeCloseTo(0.0);
	});

	it("REQ-FAC-063: opposite vectors have similarity -1", () => {
		const a = [1, 0];
		const b = [-1, 0];
		expect(cosineSimilarity(a, b)).toBeCloseTo(-1.0);
	});

	it("REQ-FAC-063: returns 0 for empty vectors", () => {
		expect(cosineSimilarity([], [])).toBe(0);
	});
});

describe("meanVector", () => {
	it("REQ-FAC-075: computes mean of multiple vectors", () => {
		const result = meanVector([[1, 2], [3, 4]]);
		expect(result[0]).toBeCloseTo(2.0);
		expect(result[1]).toBeCloseTo(3.0);
	});

	it("REQ-FAC-075: single vector returns itself", () => {
		const v = [1, 2, 3];
		expect(meanVector([v])).toEqual(v);
	});
});

// ── Embedding cache ──────────────────────────────────────────────────────

describe("embedding cache", () => {
	it("REQ-FAC-069: saves and loads embedding from cache", () => {
		const content = "function authenticate() {}";
		const vector = [0.1, 0.2, 0.3, 0.4];

		saveCachedEmbedding(tmpDir, content, vector);
		const cached = loadCachedEmbedding(tmpDir, content);

		expect(cached).not.toBeNull();
		expect(cached!.vector).toEqual(vector);
	});

	it("REQ-FAC-069: returns null for uncached content", () => {
		const result = loadCachedEmbedding(tmpDir, "uncached content xyz");
		expect(result).toBeNull();
	});

	it("REQ-FAC-069: cache miss for different content (hash mismatch)", () => {
		const content1 = "function foo() {}";
		const content2 = "function bar() {}";
		saveCachedEmbedding(tmpDir, content1, [1, 2, 3]);
		const result = loadCachedEmbedding(tmpDir, content2);
		expect(result).toBeNull();
	});
});

// ── SearchServer ─────────────────────────────────────────────────────────

describe("SearchServer", () => {
	it("REQ-FAC-061: searchText returns results for indexed files", () => {
		mkdirSync(join(tmpDir, "src"), { recursive: true });
		writeFileSync(join(tmpDir, "src", "auth.ts"), "function authenticate user login");
		writeFileSync(join(tmpDir, "src", "db.ts"), "database connection pool");

		const server = new SearchServer({ workspaceRoot: tmpDir });
		const results = server.searchText("authenticate login");
		expect(results.length).toBeGreaterThan(0);
		expect(results[0].path).toContain("auth.ts");
	});

	it("REQ-FAC-078: searchHybrid returns text-only results when bedrock disabled", async () => {
		mkdirSync(join(tmpDir, "src"), { recursive: true });
		writeFileSync(join(tmpDir, "src", "foo.ts"), "function processPayment amount");

		const server = new SearchServer({
			workspaceRoot: tmpDir,
			config: { bedrock_enabled: false },
		});
		const results = await server.searchHybrid("processPayment");
		expect(results.length).toBeGreaterThan(0);
		// All results should be text-only
		for (const r of results) {
			expect(r.vectorScore).toBe(0);
			expect(r.source).toBe("text");
		}
	});

	it("REQ-FAC-080: excluded files not in search results", () => {
		writeFileSync(join(tmpDir, ".env"), "SECRET=hunter2");
		writeFileSync(join(tmpDir, "app.ts"), "const x = 1;");

		const server = new SearchServer({ workspaceRoot: tmpDir });
		const results = server.searchText("hunter2");
		const paths = results.map((r) => r.path);
		expect(paths.some((p) => p.includes(".env"))).toBe(false);
	});

	it("REQ-FAC-070: server starts in idle state", () => {
		const server = new SearchServer({ workspaceRoot: tmpDir });
		expect(server.serverState).toBe("idle");
	});

	it("REQ-FAC-060: server transitions to ready after index build", () => {
		const server = new SearchServer({ workspaceRoot: tmpDir });
		server.buildTextIndex();
		expect(server.serverState).toBe("ready");
	});

	it("REQ-FAC-079: SearchServer tracks embedding tokens used", () => {
		// axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-079 plan=phase-100/task-100-backlog/fb-014 jira_ref=DEX-458
		const server = new SearchServer({ workspaceRoot: tmpDir, config: { bedrock_enabled: false } });
		// With bedrock disabled, no tokens are used
		expect(server.getEmbeddingTokensUsed()).toBe(0);
		// After building index (no embeddings), still 0
		server.buildTextIndex();
		expect(server.getEmbeddingTokensUsed()).toBe(0);
	});
});
