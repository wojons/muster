/**
 * L8: Search layer barrel.
 *
 * Spec: specs/87-MCP-File-Agent-Communication.md#7
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md plan=phase-100/task-100-4 jira_ref=DEX-458
 */

export type {
	CachedEmbedding,
	EmbedFileResult,
	EmbeddingClientOptions,
	EmbeddingVector,
} from "./embeddings";
export {
	BEDROCK_EMBEDDING_MODEL,
	DEFAULT_CHUNK_OVERLAP,
	DEFAULT_CHUNK_SIZE,
	EMBEDDING_CACHE_DIR,
	EMBEDDING_DIM,
	EmbeddingClient,
	chunkText,
	cosineSimilarity,
	loadCachedEmbedding,
	meanVector,
	saveCachedEmbedding,
} from "./embeddings";

export type {
	HybridSearchResult,
	SearchServerOptions,
	SearchServerState,
} from "./search-server";
export { SearchServer } from "./search-server";

export type {
	IndexOptions,
	IndexedDocument,
	TextSearchResult,
} from "./text-index";
export {
	BM25Index,
	DEFAULT_EXCLUDE_PATTERNS,
	DEFAULT_INCLUDE_EXTENSIONS,
	buildIndex,
	isExcluded,
	isIncluded,
	tokenize,
} from "./text-index";
