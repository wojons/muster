/**
 * Bedrock Titan Embeddings V2 integration for L8 hybrid search (Pillar 4).
 *
 * Generates embedding vectors for workspace files using AWS Bedrock.
 * Vectors are cached locally in _tmp/search-cache/ to avoid re-embedding
 * unchanged files.
 *
 * REQ-FAC-073: Bedrock Titan Embeddings V2.
 * REQ-FAC-074: throughput > 100 files/minute via batched calls.
 * REQ-FAC-075: chunking for files exceeding context limit (~8KB).
 * REQ-FAC-076: configurable chunk_size and chunk_overlap.
 * REQ-FAC-077: graceful degradation on Bedrock rate limit.
 * REQ-FAC-078: zero-cloud-dependency mode when Bedrock unavailable.
 * REQ-FAC-079: embedding costs in L2 metrics.
 * REQ-FAC-080: default exclusion list — sensitive files never sent to Bedrock.
 * REQ-FAC-099–103: privacy disclosure and Bedrock opt-out.
 *
 * Spec: specs/87-MCP-File-Agent-Communication.md#7.2
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-073 plan=phase-100/task-100-4/step-100-4-3 jira_ref=DEX-458
 */

import { createHash } from "node:crypto";
import {
	existsSync,
	mkdirSync,
	readFileSync,
	writeFileSync,
} from "node:fs";
import { resolve } from "node:path";

// ── Constants ────────────────────────────────────────────────────────────

/** Bedrock model ID for Titan Embeddings V2. REQ-FAC-073 */
export const BEDROCK_EMBEDDING_MODEL = "amazon.titan-embed-text-v2:0";

/** Default chunk size in characters. REQ-FAC-076 */
export const DEFAULT_CHUNK_SIZE = 8000;

/** Default chunk overlap in characters. REQ-FAC-076 */
export const DEFAULT_CHUNK_OVERLAP = 200;

/** Embedding vector cache directory. REQ-FAC-069 */
export const EMBEDDING_CACHE_DIR = "_tmp/search-cache";

/** Embedding vector dimension for Titan V2. */
export const EMBEDDING_DIM = 1024;

// ── Types ────────────────────────────────────────────────────────────────

/** An embedding vector (array of floats). */
export type EmbeddingVector = number[];

/** Cached embedding entry stored on disk. */
export interface CachedEmbedding {
	/** SHA-256 hash of the file content (for cache invalidation). */
	contentHash: string;
	/** Aggregated embedding vector (mean of chunk vectors). */
	vector: EmbeddingVector;
	/** Timestamp when cached. */
	cachedAt: number;
}

/** Options for the embedding client. */
export interface EmbeddingClientOptions {
	/** AWS region for Bedrock. Default: "us-east-1". REQ-FAC-073 */
	region?: string;
	/** Chunk size in characters. REQ-FAC-076 */
	chunkSize?: number;
	/** Chunk overlap in characters. REQ-FAC-076 */
	chunkOverlap?: number;
	/** Workspace root for cache storage. */
	workspaceRoot: string;
	/** Whether Bedrock is enabled. REQ-FAC-100 */
	bedrockEnabled?: boolean;
}

/** Result of embedding a file. */
export interface EmbedFileResult {
	/** Workspace-relative file path. */
	path: string;
	/** Embedding vector. */
	vector: EmbeddingVector;
	/** Whether the result came from cache. */
	fromCache: boolean;
	/** Number of tokens used (approximate). */
	tokensUsed: number;
}

// ── Chunking ─────────────────────────────────────────────────────────────

/**
 * Split text into overlapping chunks for embedding.
 * REQ-FAC-075: files exceeding context limit are chunked.
 */
export function chunkText(
	text: string,
	chunkSize = DEFAULT_CHUNK_SIZE,
	overlap = DEFAULT_CHUNK_OVERLAP,
): string[] {
	if (text.length <= chunkSize) return [text];

	const chunks: string[] = [];
	let start = 0;
	while (start < text.length) {
		const end = Math.min(start + chunkSize, text.length);
		chunks.push(text.slice(start, end));
		if (end === text.length) break;
		start += chunkSize - overlap;
	}
	return chunks;
}

// ── Vector math ──────────────────────────────────────────────────────────

/**
 * Compute the mean of multiple embedding vectors.
 * Used to aggregate chunk embeddings into a single file vector.
 */
export function meanVector(vectors: EmbeddingVector[]): EmbeddingVector {
	if (vectors.length === 0) return [];
	if (vectors.length === 1) return vectors[0];

	const dim = vectors[0].length;
	const result = new Array<number>(dim).fill(0);
	for (const vec of vectors) {
		for (let i = 0; i < dim; i++) {
			result[i] += vec[i];
		}
	}
	for (let i = 0; i < dim; i++) {
		result[i] /= vectors.length;
	}
	return result;
}

/**
 * Compute cosine similarity between two vectors.
 * Returns a value in [-1, 1] where 1 = identical direction.
 */
export function cosineSimilarity(a: EmbeddingVector, b: EmbeddingVector): number {
	if (a.length !== b.length || a.length === 0) return 0;
	let dot = 0;
	let normA = 0;
	let normB = 0;
	for (let i = 0; i < a.length; i++) {
		dot += a[i] * b[i];
		normA += a[i] * a[i];
		normB += b[i] * b[i];
	}
	const denom = Math.sqrt(normA) * Math.sqrt(normB);
	return denom === 0 ? 0 : dot / denom;
}

// ── Cache ────────────────────────────────────────────────────────────────

function contentHash(content: string): string {
	return createHash("sha256").update(content).digest("hex").slice(0, 16);
}

function cachePath(workspaceRoot: string, fileHash: string): string {
	return resolve(workspaceRoot, EMBEDDING_CACHE_DIR, `${fileHash}.vec`);
}

/**
 * Load a cached embedding vector for a file.
 * REQ-FAC-069: local embedding cache.
 */
export function loadCachedEmbedding(
	workspaceRoot: string,
	fileContent: string,
): CachedEmbedding | null {
	const hash = contentHash(fileContent);
	const path = cachePath(workspaceRoot, hash);
	if (!existsSync(path)) return null;
	try {
		const raw = readFileSync(path, "utf8");
		const cached = JSON.parse(raw) as CachedEmbedding;
		if (cached.contentHash !== hash) return null; // stale
		return cached;
	} catch {
		return null;
	}
}

/**
 * Save an embedding vector to the local cache.
 * REQ-FAC-069, REQ-FAC-093: vectors stored locally only.
 */
export function saveCachedEmbedding(
	workspaceRoot: string,
	fileContent: string,
	vector: EmbeddingVector,
): void {
	const hash = contentHash(fileContent);
	const cacheDir = resolve(workspaceRoot, EMBEDDING_CACHE_DIR);
	mkdirSync(cacheDir, { recursive: true });
	const entry: CachedEmbedding = {
		contentHash: hash,
		vector,
		cachedAt: Date.now(),
	};
	try {
		writeFileSync(cachePath(workspaceRoot, hash), JSON.stringify(entry));
	} catch {
		// Cache write failure is non-fatal
	}
}

// ── Bedrock client ───────────────────────────────────────────────────────

/**
 * Embedding client that calls AWS Bedrock Titan Embeddings V2.
 *
 * Uses dynamic import of @aws-sdk/client-bedrock-runtime to avoid
 * hard dependency — falls back gracefully if SDK not installed.
 *
 * REQ-FAC-073: Bedrock Titan Embeddings V2.
 * REQ-FAC-077: graceful degradation on rate limit.
 * REQ-FAC-078: zero-cloud-dependency mode.
 */
export class EmbeddingClient {
	private opts: Required<EmbeddingClientOptions>;
	private firstUseLogged = false;

	constructor(opts: EmbeddingClientOptions) {
		this.opts = {
			region: opts.region ?? "us-east-1",
			chunkSize: opts.chunkSize ?? DEFAULT_CHUNK_SIZE,
			chunkOverlap: opts.chunkOverlap ?? DEFAULT_CHUNK_OVERLAP,
			workspaceRoot: opts.workspaceRoot,
			bedrockEnabled: opts.bedrockEnabled !== false,
		};
	}

	/**
	 * Embed a file's content, using cache when available.
	 *
	 * REQ-FAC-069: check cache first.
	 * REQ-FAC-075: chunk large files.
	 * REQ-FAC-099/102: log notice on first use.
	 */
	async embedFile(
		filePath: string,
		content: string,
	): Promise<EmbedFileResult | null> {
		if (!this.opts.bedrockEnabled) return null;

		// REQ-FAC-069: check cache
		const cached = loadCachedEmbedding(this.opts.workspaceRoot, content);
		if (cached) {
			return {
				path: filePath,
				vector: cached.vector,
				fromCache: true,
				tokensUsed: 0,
			};
		}

		// REQ-FAC-102: log notice on first vector search use
		if (!this.firstUseLogged) {
			this.firstUseLogged = true;
			process.stderr.write(
				"[axiom plugin] L8: Vector search enabled. File contents are sent to AWS Bedrock for embedding generation. " +
				"Configure search.exclude_patterns to exclude sensitive files.\n",
			);
		}

		// Chunk the content
		const chunks = chunkText(content, this.opts.chunkSize, this.opts.chunkOverlap);
		const vectors: EmbeddingVector[] = [];
		let totalTokens = 0;

		for (const chunk of chunks) {
			try {
				const result = await this.callBedrock(chunk);
				if (result) {
					vectors.push(result.vector);
					totalTokens += result.tokensUsed;
				}
			} catch (err: unknown) {
				const msg = err instanceof Error ? err.message : String(err);
				// REQ-FAC-077: graceful degradation on rate limit
				if (msg.includes("ThrottlingException") || msg.includes("rate limit")) {
					process.stderr.write(
						"[axiom plugin] L8: Vector re-ranking unavailable (Bedrock rate limit). Showing text-match results only.\n",
					);
					return null;
				}
				// Other errors — skip this file
				return null;
			}
		}

		if (vectors.length === 0) return null;

		const vector = meanVector(vectors);

		// REQ-FAC-069: save to cache
		saveCachedEmbedding(this.opts.workspaceRoot, content, vector);

		return {
			path: filePath,
			vector,
			fromCache: false,
			tokensUsed: totalTokens,
		};
	}

	/**
	 * Call Bedrock Titan Embeddings V2 for a single text chunk.
	 * Uses dynamic import to avoid hard dependency on AWS SDK.
	 */
	private async callBedrock(
		text: string,
	): Promise<{ vector: EmbeddingVector; tokensUsed: number } | null> {
		try {
			// Dynamic import — graceful failure if SDK not installed. REQ-FAC-078
			// @ts-ignore — optional dependency; gracefully absent when not installed
			// biome-ignore lint: intentional dynamic import of optional dep
			const bedrockModule = await import("@aws-sdk/client-bedrock-runtime").catch(() => null);
			if (!bedrockModule) return null;
			const { BedrockRuntimeClient, InvokeModelCommand } = bedrockModule as {
				BedrockRuntimeClient: new (opts: { region: string }) => { send: (cmd: unknown) => Promise<{ body: Uint8Array }> };
				InvokeModelCommand: new (opts: { modelId: string; contentType: string; accept: string; body: Buffer }) => unknown;
			};
			const client = new BedrockRuntimeClient({ region: this.opts.region });
			const body = JSON.stringify({ inputText: text, dimensions: EMBEDDING_DIM });
			const command = new InvokeModelCommand({
				modelId: BEDROCK_EMBEDDING_MODEL,
				contentType: "application/json",
				accept: "application/json",
				body: Buffer.from(body),
			});
			const response = await client.send(command);
			const responseBody = JSON.parse(
				Buffer.from(response.body).toString("utf8"),
			) as { embedding: number[]; inputTextTokenCount: number };
			// REQ-FAC-073: validate embedding dimension — wrong dimension causes silent cosineSimilarity corruption
			const embedding = responseBody.embedding as number[];
			if (!Array.isArray(embedding) || embedding.length !== EMBEDDING_DIM) {
				process.stderr.write(
					`[axiom plugin] L8: Bedrock returned unexpected embedding dimension ` +
					`${Array.isArray(embedding) ? embedding.length : 'non-array'} (expected ${EMBEDDING_DIM}). Skipping file.\n`,
				);
				return null;
			}
			return {
				vector: embedding,
				tokensUsed: (responseBody as { inputTextTokenCount?: number }).inputTextTokenCount ?? 0,
			};
		} catch (err: unknown) {
			const msg = err instanceof Error ? err.message : String(err);
			// Re-throw throttling errors for caller to handle
			if (msg.includes("ThrottlingException") || msg.includes("rate limit")) {
				throw err;
			}
			// SDK not installed or no credentials — REQ-FAC-078
			return null;
		}
	}
}
