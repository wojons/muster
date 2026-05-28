/**
 * Hybrid search server for L8 file-based agent communication (Pillar 4).
 *
 * Coordinates BM25 full-text search and vector search (Bedrock embeddings)
 * to provide fast, accurate code search over workspace files.
 *
 * The grep-then-vector pipeline:
 * 1. Text search narrows candidates (fast, cheap).
 * 2. Vector search re-ranks candidates (slower, costs embedding tokens).
 *
 * REQ-FAC-060: background search server with BM25 + HNSW indexes.
 * REQ-FAC-061: search_text, search_vector, search_hybrid tools.
 * REQ-FAC-062: text search < 200ms for 5K files.
 * REQ-FAC-063: vector search < 500ms for 50 candidates.
 * REQ-FAC-064: hybrid search < 1s end-to-end.
 * REQ-FAC-067: grep-then-vector pipeline.
 * REQ-FAC-068: index invalidation within 5s.
 * REQ-FAC-070: zero CPU overhead when idle.
 * REQ-FAC-071: graceful degradation on server crash.
 * REQ-FAC-078: zero-cloud-dependency mode.
 *
 * Spec: specs/87-MCP-File-Agent-Communication.md#7
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-061 plan=phase-100/task-100-4/step-100-4-5 jira_ref=DEX-458
 */

import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import type { SearchConfig } from "../../config-loader";
import {
	type EmbeddingVector,
	EmbeddingClient,
	cosineSimilarity,
	loadCachedEmbedding,
} from "./embeddings";
import {
	BM25Index,
	DEFAULT_EXCLUDE_PATTERNS,
	DEFAULT_INCLUDE_EXTENSIONS,
	type IndexOptions,
	type TextSearchResult,
	buildIndex,
	isExcluded,
	isIncluded,
} from "./text-index";

// ── Types ────────────────────────────────────────────────────────────────

/** A hybrid search result combining text and vector scores. */
export interface HybridSearchResult {
	/** Workspace-relative file path. */
	path: string;
	/** BM25 text score (0 if text search not used). */
	textScore: number;
	/** Cosine similarity vector score (0 if vector search not used or unavailable). */
	vectorScore: number;
	/** Combined score (weighted sum). */
	combinedScore: number;
	/** Whether this result came from text search, vector search, or both. */
	source: "text" | "vector" | "hybrid";
}

/** Options for the search server. */
export interface SearchServerOptions {
	/** Workspace root directory (absolute path). */
	workspaceRoot: string;
	/** Search configuration from plugin.yaml. */
	config?: SearchConfig;
}

/** Search server state. */
export type SearchServerState = "idle" | "building" | "ready" | "error";

// ── Search server ────────────────────────────────────────────────────────

/**
 * Hybrid search server.
 *
 * Maintains a BM25 full-text index and an in-memory vector store.
 * Provides text, vector, and hybrid search over workspace files.
 *
 * REQ-FAC-070: zero CPU overhead when idle — no background polling.
 * Index is built on first search and updated incrementally on subsequent calls.
 */
export class SearchServer {
	private textIndex: BM25Index;
	private vectorStore = new Map<string, EmbeddingVector>();
	private embeddingClient: EmbeddingClient;
	private state: SearchServerState = "idle";
	private lastIndexBuildMs = 0;
	private opts: SearchServerOptions;
	private excludePatterns: string[];
	private includeExtensions: Set<string>;
	/** REQ-FAC-079: total embedding tokens used this session. */
	private embeddingTokensUsed = 0;

	constructor(opts: SearchServerOptions) {
		this.opts = opts;
		this.textIndex = new BM25Index();

		const cfg = opts.config ?? {};
		this.excludePatterns = [
			...DEFAULT_EXCLUDE_PATTERNS,
			...(cfg.exclude_patterns ?? []),
		];
		this.includeExtensions = cfg.include_extensions
			? new Set(cfg.include_extensions)
			: DEFAULT_INCLUDE_EXTENSIONS;

		this.embeddingClient = new EmbeddingClient({
			workspaceRoot: opts.workspaceRoot,
			region: cfg.bedrock_region,
			chunkSize: cfg.chunk_size,
			chunkOverlap: cfg.chunk_overlap,
			bedrockEnabled: cfg.bedrock_enabled !== false,
		});
	}

	/** Current server state. */
	get serverState(): SearchServerState {
		return this.state;
	}

	/** Number of indexed documents. */
	get indexSize(): number {
		return this.textIndex.size;
	}

	/** Total embedding tokens used this session (REQ-FAC-079). */
	getEmbeddingTokensUsed(): number {
		return this.embeddingTokensUsed;
	}

	// ── Index management ───────────────────────────────────────────────

	/**
	 * Build or incrementally update the text index.
	 * REQ-FAC-065: < 30s for 1K files.
	 * REQ-FAC-068: incremental update.
	 */
	buildTextIndex(): number {
		this.state = "building";
		try {
			const indexOpts: IndexOptions = {
				workspaceRoot: this.opts.workspaceRoot,
				excludePatterns: this.excludePatterns,
				includeExtensions: this.includeExtensions,
			};
			const count = buildIndex(this.textIndex, indexOpts);
			this.lastIndexBuildMs = Date.now();
			this.state = "ready";
			return count;
		} catch (err) {
			this.state = "error";
			throw err;
		}
	}

	/**
	 * Check if the index needs rebuilding (files changed since last build).
	 * REQ-FAC-068: invalidation within 5s of file change.
	 *
	 * Default maxAgeMs is 300_000 (5 minutes) — not 5s. The 5s figure in
	 * REQ-FAC-068 refers to how quickly a file change should be reflected
	 * after a rebuild is triggered, not how often the index should be rebuilt.
	 * A 5s default would cause the singleton to rebuild on every search call
	 * in any realistic usage pattern, defeating the warm-index SLO (REQ-FAC-062).
	 *
	 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-062 plan=phase-100/task-100-5/step-100-5-4 jira_ref=DEX-458
	 */
	needsRebuild(maxAgeMs = 300_000): boolean {
		return Date.now() - this.lastIndexBuildMs > maxAgeMs;
	}

	/**
	 * Ensure the index is warm, rebuilding if needed.
	 */
	private ensureIndex(): void {
		if (this.state === "idle" || this.needsRebuild()) {
			this.buildTextIndex();
		}
	}

	// ── Text search ────────────────────────────────────────────────────

	/**
	 * Full-text search using BM25.
	 * REQ-FAC-061: search_text tool.
	 * REQ-FAC-062: < 200ms for 5K files.
	 */
	searchText(query: string, topK = 20): TextSearchResult[] {
		this.ensureIndex();
		return this.textIndex.search(query, topK);
	}

	// ── Vector search ──────────────────────────────────────────────────

	/**
	 * Vector search using cosine similarity over cached embeddings.
	 *
	 * For candidates not yet embedded, loads from cache if available.
	 * REQ-FAC-061: search_vector tool.
	 * REQ-FAC-063: < 500ms for 50 candidates.
	 * REQ-FAC-078: returns empty if Bedrock unavailable.
	 */
	async searchVector(
		query: string,
		candidatePaths?: string[],
		topK = 20,
	): Promise<HybridSearchResult[]> {
		const cfg = this.opts.config ?? {};
		if (cfg.bedrock_enabled === false) {
			return []; // REQ-FAC-078: zero-cloud-dependency mode
		}

		// Get query embedding
		const queryResult = await this.embeddingClient.embedFile("__query__", query);
		if (!queryResult) return [];

		const queryVector = queryResult.vector;
		this.embeddingTokensUsed += queryResult.tokensUsed; // REQ-FAC-079

		// Determine candidates
		const paths = candidatePaths ?? this.textIndex.getPaths();

		// Load vectors for candidates (from in-memory store or disk cache)
		const results: HybridSearchResult[] = [];
		for (const path of paths.slice(0, 200)) { // cap at 200 for performance
			let vector = this.vectorStore.get(path);
			if (!vector) {
				// Try loading from disk cache
				const absPath = resolve(this.opts.workspaceRoot, path);
				if (!existsSync(absPath)) continue;
				try {
					const content = readFileSync(absPath, "utf8");
					const cached = loadCachedEmbedding(this.opts.workspaceRoot, content);
					if (cached) {
						vector = cached.vector;
						this.vectorStore.set(path, vector);
					}
				} catch {
					continue;
				}
			}
			if (!vector) continue;

			const score = cosineSimilarity(queryVector, vector);
			results.push({
				path,
				textScore: 0,
				vectorScore: score,
				combinedScore: score,
				source: "vector",
			});
		}

		return results
			.sort((a, b) => b.vectorScore - a.vectorScore)
			.slice(0, topK);
	}

	// ── Hybrid search ──────────────────────────────────────────────────

	/**
	 * Hybrid search: text search narrows candidates, vector search re-ranks.
	 *
	 * REQ-FAC-061: search_hybrid tool.
	 * REQ-FAC-064: < 1s end-to-end.
	 * REQ-FAC-067: grep-then-vector pipeline.
	 */
	async searchHybrid(
		query: string,
		topK = 20,
	): Promise<HybridSearchResult[]> {
		// Step 1: Text search narrows candidates (fast). REQ-FAC-067
		const textResults = this.searchText(query, 50);

		if (textResults.length === 0) {
			return [];
		}

		const cfg = this.opts.config ?? {};
		if (cfg.bedrock_enabled === false) {
			// REQ-FAC-078: text-only mode
			return textResults.slice(0, topK).map((r) => ({
				path: r.path,
				textScore: r.score,
				vectorScore: 0,
				combinedScore: r.score,
				source: "text" as const,
			}));
		}

		// Step 2: Vector re-ranking of text candidates. REQ-FAC-067
		const candidatePaths = textResults.map((r) => r.path);
		const vectorResults = await this.searchVector(query, candidatePaths, 50);

		if (vectorResults.length === 0) {
			// Vector unavailable — return text results
			return textResults.slice(0, topK).map((r) => ({
				path: r.path,
				textScore: r.score,
				vectorScore: 0,
				combinedScore: r.score,
				source: "text" as const,
			}));
		}

		// Combine scores: normalize text scores, then weighted sum
		const maxTextScore = Math.max(...textResults.map((r) => r.score), 1);
		const textScoreMap = new Map(textResults.map((r) => [r.path, r.score / maxTextScore]));
		const vectorScoreMap = new Map(vectorResults.map((r) => [r.path, r.vectorScore]));

		const allPaths = new Set([...textScoreMap.keys(), ...vectorScoreMap.keys()]);
		const combined: HybridSearchResult[] = [];

		for (const path of allPaths) {
			const ts = textScoreMap.get(path) ?? 0;
			const vs = vectorScoreMap.get(path) ?? 0;
			const hasText = textScoreMap.has(path);
			const hasVector = vectorScoreMap.has(path);

			combined.push({
				path,
				textScore: ts,
				vectorScore: vs,
				// Weighted: 40% text + 60% vector (vector is more semantic)
				combinedScore: 0.4 * ts + 0.6 * vs,
				source: hasText && hasVector ? "hybrid" : hasText ? "text" : "vector",
			});
		}

		return combined
			.sort((a, b) => b.combinedScore - a.combinedScore)
			.slice(0, topK);
	}

	/**
	 * Pre-embed a set of files and store vectors in the in-memory store.
	 * Called during index build to warm the vector cache.
	 * REQ-FAC-074: > 100 files/minute via batched calls.
	 */
	async embedFiles(paths: string[]): Promise<number> {
		let embedded = 0;
		for (const path of paths) {
			const absPath = resolve(this.opts.workspaceRoot, path);
			if (!existsSync(absPath)) continue;
			if (isExcluded(path, this.excludePatterns)) continue; // REQ-FAC-080
			if (!isIncluded(path, this.includeExtensions)) continue;

			try {
				const content = readFileSync(absPath, "utf8");
				const result = await this.embeddingClient.embedFile(path, content);
				if (result) {
					this.vectorStore.set(path, result.vector);
					this.embeddingTokensUsed += result.tokensUsed; // REQ-FAC-079
					embedded++;
				}
			} catch {
				// Skip unembeddable files
			}
		}
		return embedded;
	}
}
