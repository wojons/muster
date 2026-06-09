/**
 * BM25 full-text search index for L8 hybrid search (Pillar 4).
 *
 * Implements an in-memory inverted index with BM25 scoring for fast
 * full-text search over workspace files.
 *
 * REQ-FAC-060: background search server with BM25 full-text index.
 * REQ-FAC-062: text search on warm index < 200ms for repos up to 5,000 files.
 * REQ-FAC-067: grep-then-vector pipeline — text search narrows candidates first.
 * REQ-FAC-068: index invalidation within 5s of file change.
 * REQ-FAC-070: zero CPU overhead when idle.
 * REQ-FAC-080–088: exclusion list for sensitive files.
 *
 * Spec: specs/87-MCP-File-Agent-Communication.md#7
 * axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-060 plan=phase-100/task-100-4/step-100-4-2 jira_ref=DEX-458
 */

import { createHash } from "node:crypto";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { extname, join, relative, resolve } from "node:path";

// ── Constants ────────────────────────────────────────────────────────────

/** BM25 tuning parameters (standard defaults). */
const BM25_K1 = 1.5;
const BM25_B = 0.75;

/**
 * Default file extensions to include in the search index.
 * REQ-FAC-084
 */
export const DEFAULT_INCLUDE_EXTENSIONS = new Set([
	".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs",
	".py", ".go", ".rs", ".java", ".kt", ".swift",
	".c", ".cpp", ".h", ".hpp",
	".md", ".mdx", ".txt", ".yaml", ".yml", ".json", ".toml",
	".sh", ".bash", ".zsh",
	".html", ".css", ".scss",
	".sql",
]);

/**
 * Default exclusion patterns — files matching these are NEVER indexed or sent to Bedrock.
 * REQ-FAC-080
 */
export const DEFAULT_EXCLUDE_PATTERNS = [
	"*.env", "*.env.*", ".env", ".env.*",
	"*.pem", "*.p12", "*.pfx", "*.key",
	"*secret*", "*credential*", "*password*",
	"*.npmrc", "*.pypirc", "*.netrc",
	"**/.git/**",
	"**/node_modules/**",
	"**/_tmp/**",
	"**/.axiom/logs/**",
];

// ── Types ────────────────────────────────────────────────────────────────

/** A single indexed document. */
export interface IndexedDocument {
	/** Workspace-relative file path. */
	path: string;
	/** SHA-256 hash of the file content (for change detection). */
	contentHash: string;
	/** Tokenized terms. */
	terms: string[];
	/** Term frequency map: term → count. */
	tf: Map<string, number>;
	/** Last modified time (ms). */
	mtime: number;
}

/** A search result entry. */
export interface TextSearchResult {
	/** Workspace-relative file path. */
	path: string;
	/** BM25 score. */
	score: number;
	/** Snippet of matching content (first 200 chars around first match). */
	snippet?: string;
}

/** Options for building/updating the index. */
export interface IndexOptions {
	/** Workspace root directory (absolute path). */
	workspaceRoot: string;
	/** Additional exclusion glob patterns. REQ-FAC-082 */
	excludePatterns?: string[];
	/** File extensions to include. REQ-FAC-084 */
	includeExtensions?: Set<string>;
	/** Maximum file size to index in bytes (default: 500KB). */
	maxFileSizeBytes?: number;
}

// ── Tokenizer ────────────────────────────────────────────────────────────

/**
 * Simple whitespace + punctuation tokenizer.
 * Lowercases and splits on non-alphanumeric characters.
 */
export function tokenize(text: string): string[] {
	return text
		.toLowerCase()
		.split(/[^a-z0-9_]+/)
		.filter((t) => t.length >= 2 && t.length <= 50);
}

// ── Simple glob matcher ──────────────────────────────────────────────────

/**
 * Convert a glob pattern to a RegExp.
 * Supports: * (any chars except /), ** (any chars including /), ? (single char),
 * {a,b} (brace expansion).
 */
function globToRegex(pattern: string): RegExp {
	// Step 1: expand brace alternatives {a,b} → (a|b) BEFORE any escaping
	// REQ-FAC-080: user patterns like *.{env,key} must match .env and .key files
	// axiom:trace work_item=mcp-agent-comm-01 spec=specs/87-MCP-File-Agent-Communication.md#REQ-FAC-080 plan=phase-100/task-100-backlog/fb-004 jira_ref=DEX-458
	const BRACE_START = "\u0001BRACE_START\u0001";
	const BRACE_SEP = "\u0001BRACE_SEP\u0001";
	const BRACE_END = "\u0001BRACE_END\u0001";

	let expanded = pattern.replace(/\{([^}]+)\}/g, (_match, group: string) => {
		const alternatives = group.split(",").map((s) => s.trim());
		return `${BRACE_START}${alternatives.join(BRACE_SEP)}${BRACE_END}`;
	});

	// Step 2: escape regex special chars (the placeholders use \u0001 which is not a regex special char)
	expanded = expanded.replace(/[.+^${}()|[\]\\]/g, "\\$&");

	// Step 3: restore brace alternatives as regex alternation groups
	expanded = expanded
		.split(BRACE_START).join("(")
		.split(BRACE_SEP).join("|")
		.split(BRACE_END).join(")");

	// Step 4: glob wildcards
	const result = expanded
		.replace(/\*\*/g, "§GLOBSTAR§")
		.replace(/\*/g, "[^/]*")
		.replace(/§GLOBSTAR§/g, ".*")
		.replace(/\?/g, "[^/]");

	return new RegExp(`^${result}$`, "i");
}

/**
 * Check whether a path matches a glob pattern.
 * Supports leading double-star for matching anywhere in the path.
 */
function matchesGlob(filePath: string, pattern: string): boolean {
	// Normalize separators
	const normalPath = filePath.replace(/\\/g, "/");
	const normalPattern = pattern.replace(/\\/g, "/");

	try {
		const regex = globToRegex(normalPattern);
		if (regex.test(normalPath)) return true;

		// Also test just the basename for patterns without path separators
		if (!normalPattern.includes("/")) {
			const basename = normalPath.split("/").pop() ?? normalPath;
			return regex.test(basename);
		}

		// For patterns like **/node_modules/**, also check if any path segment matches
		// by testing each suffix of the path
		if (normalPattern.startsWith("**/")) {
			const suffix = normalPattern.slice(3); // remove leading **/
			const suffixRegex = globToRegex(suffix);
			// Check if any trailing portion of the path matches
			const parts = normalPath.split("/");
			for (let i = 0; i < parts.length; i++) {
				const sub = parts.slice(i).join("/");
				if (suffixRegex.test(sub)) return true;
			}
		}
	} catch {
		// Malformed pattern — skip
	}
	return false;
}

// ── Exclusion list ───────────────────────────────────────────────────────

/**
 * Check whether a file path should be excluded from indexing.
 * REQ-FAC-080–088: exclusion takes precedence over allowlist.
 */
export function isExcluded(
	filePath: string,
	excludePatterns: string[],
): boolean {
	for (const pattern of excludePatterns) {
		if (matchesGlob(filePath, pattern)) return true;
	}
	return false;
}

/**
 * Check whether a file should be included based on extension.
 * REQ-FAC-083: binary files excluded by default.
 * REQ-FAC-084: configurable include extensions.
 */
export function isIncluded(
	filePath: string,
	includeExtensions: Set<string>,
): boolean {
	const ext = extname(filePath).toLowerCase();
	return includeExtensions.has(ext);
}

// ── Content hash ─────────────────────────────────────────────────────────

function hashContent(content: string): string {
	return createHash("sha256").update(content).digest("hex").slice(0, 16);
}

// ── BM25 index ───────────────────────────────────────────────────────────

/**
 * In-memory BM25 full-text search index.
 *
 * REQ-FAC-060: BM25-based full-text inverted index.
 * REQ-FAC-062: text search < 200ms for 5K files.
 * REQ-FAC-070: zero CPU overhead when idle (no background polling).
 */
export class BM25Index {
	/** Documents in the index, keyed by workspace-relative path. */
	private docs = new Map<string, IndexedDocument>();

	/** Inverted index: term → set of document paths. */
	private invertedIndex = new Map<string, Set<string>>();

	/** Average document length (in terms). */
	private avgDocLength = 0;

	/** Total number of documents. */
	get size(): number {
		return this.docs.size;
	}

	/**
	 * Add or update a document in the index.
	 * REQ-FAC-068: invalidates stale entries.
	 */
	addDocument(path: string, content: string, mtime: number): void {
		const contentHash = hashContent(content);

		// Skip if unchanged
		const existing = this.docs.get(path);
		if (existing && existing.contentHash === contentHash) return;

		// Remove old entry from inverted index
		if (existing) {
			for (const term of existing.terms) {
				this.invertedIndex.get(term)?.delete(path);
			}
		}

		const terms = tokenize(content);
		const tf = new Map<string, number>();
		for (const term of terms) {
			tf.set(term, (tf.get(term) ?? 0) + 1);
		}

		const doc: IndexedDocument = { path, contentHash, terms, tf, mtime };
		this.docs.set(path, doc);

		// Update inverted index
		for (const term of new Set(terms)) {
			if (!this.invertedIndex.has(term)) {
				this.invertedIndex.set(term, new Set());
			}
			this.invertedIndex.get(term)!.add(path);
		}

		this.recomputeAvgDocLength();
	}

	/**
	 * Remove a document from the index.
	 */
	removeDocument(path: string): void {
		const doc = this.docs.get(path);
		if (!doc) return;
		for (const term of doc.terms) {
			this.invertedIndex.get(term)?.delete(path);
		}
		this.docs.delete(path);
		this.recomputeAvgDocLength();
	}

	private recomputeAvgDocLength(): void {
		if (this.docs.size === 0) {
			this.avgDocLength = 0;
			return;
		}
		let total = 0;
		for (const doc of this.docs.values()) {
			total += doc.terms.length;
		}
		this.avgDocLength = total / this.docs.size;
	}

	/**
	 * Search the index using BM25 scoring.
	 *
	 * REQ-FAC-062: < 200ms for 5K files on warm index.
	 *
	 * @param query    Search query string.
	 * @param topK     Maximum number of results to return (default: 20).
	 * @returns        Ranked results sorted by BM25 score descending.
	 */
	search(query: string, topK = 20): TextSearchResult[] {
		const queryTerms = tokenize(query);
		if (queryTerms.length === 0) return [];

		const N = this.docs.size;
		if (N === 0) return [];

		const scores = new Map<string, number>();

		for (const term of queryTerms) {
			const matchingPaths = this.invertedIndex.get(term);
			if (!matchingPaths || matchingPaths.size === 0) continue;

			// IDF: log((N - df + 0.5) / (df + 0.5) + 1)
			const df = matchingPaths.size;
			const idf = Math.log((N - df + 0.5) / (df + 0.5) + 1);

			for (const path of matchingPaths) {
				const doc = this.docs.get(path)!;
				const tf = doc.tf.get(term) ?? 0;
				const docLen = doc.terms.length;

				// BM25 TF component
				const tfNorm =
					(tf * (BM25_K1 + 1)) /
					(tf + BM25_K1 * (1 - BM25_B + BM25_B * (docLen / this.avgDocLength)));

				scores.set(path, (scores.get(path) ?? 0) + idf * tfNorm);
			}
		}

		// Sort by score descending, take topK
		const sorted = Array.from(scores.entries())
			.sort((a, b) => b[1] - a[1])
			.slice(0, topK);

		return sorted.map(([path, score]) => ({ path, score }));
	}

	/**
	 * Check whether a document needs re-indexing based on mtime.
	 * REQ-FAC-068
	 */
	needsUpdate(path: string, mtime: number): boolean {
		const doc = this.docs.get(path);
		return !doc || doc.mtime < mtime;
	}

	/** Get all indexed paths. */
	getPaths(): string[] {
		return Array.from(this.docs.keys());
	}

	/** Clear the entire index. */
	clear(): void {
		this.docs.clear();
		this.invertedIndex.clear();
		this.avgDocLength = 0;
	}
}

// ── Index builder ────────────────────────────────────────────────────────

/**
 * Read .gitignore patterns from the workspace root.
 * REQ-FAC-081: .gitignore patterns MUST also be excluded from the search index.
 */
function readGitignorePatterns(workspaceRoot: string): string[] {
	const gitignorePath = join(resolve(workspaceRoot), ".gitignore");
	if (!existsSync(gitignorePath)) return [];
	try {
		return readFileSync(gitignorePath, "utf8")
			.split("\n")
			.map((line) => line.trim())
			.filter((line) => line.length > 0 && !line.startsWith("#"))
			.flatMap((line) => {
				// Normalize trailing slash (directory patterns) to glob patterns
				// e.g. "dist/" → ["dist/**", "**/dist/**"]
				if (line.endsWith("/")) {
					const dir = line.slice(0, -1);
					return [`${dir}/**`, `**/${dir}/**`];
				}
				return [line];
			});
	} catch {
		return [];
	}
}

/**
 * Build or incrementally update the BM25 index from workspace files.
 *
 * REQ-FAC-065: index build < 30s for 1,000 files.
 * REQ-FAC-068: incremental update — only re-index changed files.
 * REQ-FAC-081: .gitignore patterns excluded.
 *
 * @returns Number of files indexed (added or updated).
 */
export function buildIndex(
	index: BM25Index,
	opts: IndexOptions,
): number {
	const {
		workspaceRoot,
		excludePatterns = DEFAULT_EXCLUDE_PATTERNS,
		includeExtensions = DEFAULT_INCLUDE_EXTENSIONS,
		maxFileSizeBytes = 500 * 1024,
	} = opts;

	// REQ-FAC-081: merge .gitignore patterns
	const gitignorePatterns = readGitignorePatterns(workspaceRoot);
	const allPatterns = [...DEFAULT_EXCLUDE_PATTERNS, ...excludePatterns, ...gitignorePatterns];
	const absRoot = resolve(workspaceRoot);

	let indexed = 0;

	function walk(dir: string): void {
		let entries: import("node:fs").Dirent[];
		try {
			entries = readdirSync(dir, { withFileTypes: true });
		} catch {
			return;
		}

		for (const entry of entries) {
			const absPath = join(dir, entry.name);
			const relPath = relative(absRoot, absPath);

			if (isExcluded(relPath, allPatterns)) continue;

			if (entry.isDirectory()) {
				walk(absPath);
				continue;
			}

			if (!entry.isFile()) continue;
			if (!isIncluded(relPath, includeExtensions)) continue;

			try {
				const stat = statSync(absPath);
				if (stat.size > maxFileSizeBytes) continue;

				if (!index.needsUpdate(relPath, stat.mtimeMs)) continue;

				const content = readFileSync(absPath, "utf8");
				index.addDocument(relPath, content, stat.mtimeMs);
				indexed++;
			} catch {
				// Unreadable file — skip
			}
		}
	}

	if (existsSync(absRoot)) {
		walk(absRoot);
	}

	return indexed;
}
