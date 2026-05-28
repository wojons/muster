/**
 * Tests for /axiom-annotate command handler.
 *
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-041 plan=phase-2.10/task-2.10.annotate-test-fix/step-1 test=.axiom/plugin/src/layers/wave-annotate.test.ts
 */

import { beforeEach, describe, expect, it, vi } from "vitest"
import { handleAnnotateCommand } from "./wave-annotate"

// Clear all mocks before each test to prevent vi.fn() state leakage in bun vitest compat.
beforeEach(() => { vi.clearAllMocks() })

const BACKEND = "http://localhost:8100"

/** Stable fake repo root used in tests that need repoPath */
const FAKE_REPO = "/fake-repo"

function mockFetchSequence(responses: Array<{ ok: boolean; status?: number; statusText?: string; json?: () => Promise<unknown> }>) {
  let call = 0
  return vi.fn().mockImplementation(async () => {
    const r = responses[call++] ?? responses[responses.length - 1]
    return {
      ok: r.ok,
      status: r.status ?? (r.ok ? 201 : 500),
      statusText: r.statusText ?? (r.ok ? "Created" : "Server Error"),
      json: r.json ?? (async () => ({})),
    } as unknown as Response
  })
}

/**
 * Identity realpath mock: returns the path unchanged (no symlink resolution).
 * Used in tests that don't need symlink-specific behaviour.
 */
const identityRealpath = (p: string) => p

describe("handleAnnotateCommand", () => {
  // test_annotate_command_creates_artifact_review_space
  it("test_annotate_command_creates_artifact_review_space — creates space + item", async () => {
    const mockFetch = mockFetchSequence([
      { ok: true, json: async () => ({ space_id: "sp-ann-1", title: "Artifact Review: README.md" }) },
      { ok: true, json: async () => ({ item_id: "item-ann-1" }) },
    ])
    const result = await handleAnnotateCommand("README.md", {
      backendUrl: BACKEND,
      authToken: "tok-123",
      workItemId: "wave-01",
      _mockFetch: mockFetch,
      _mockReadFile: () => "# Hello World",
      _mockStat: () => ({ size: 100 }),
      _mockRealpath: identityRealpath,
      repoPath: FAKE_REPO,
    })
    expect(result.spaceId).toBe("sp-ann-1")
    expect(result.itemId).toBe("item-ann-1")
    expect(result.filePath).toBe("README.md")
    // Verify space creation call
    const spaceBody = JSON.parse(mockFetch.mock.calls[0][1].body)
    expect(spaceBody.type).toBe("artifact_review")
    expect(spaceBody.work_item_id).toBe("wave-01")
    // Verify item creation call
    const itemBody = JSON.parse(mockFetch.mock.calls[1][1].body)
    expect(itemBody.type).toBe("artifact_review")
    expect(itemBody.content.artifact_type).toBe("markdown")
    expect(itemBody.content.file_path).toBe("README.md")
    expect(itemBody.content.text).toBe("# Hello World")
  })

  // test_annotate_command_returns_url
  it("test_annotate_command_returns_url — returns space URL containing space_id", async () => {
    const mockFetch = mockFetchSequence([
      { ok: true, json: async () => ({ space_id: "sp-url-test", title: "Artifact Review" }) },
      { ok: true, json: async () => ({ item_id: "item-url-test" }) },
    ])
    const result = await handleAnnotateCommand("src/main.ts", {
      backendUrl: BACKEND,
      _mockFetch: mockFetch,
      _mockReadFile: () => "export {}",
      _mockStat: () => ({ size: 100 }),
      _mockRealpath: identityRealpath,
      repoPath: FAKE_REPO,
    })
    expect(result.url).toContain("sp-url-test")
    expect(result.url).toContain(BACKEND)
  })

  it("throws when file_path is empty", async () => {
    await expect(
      handleAnnotateCommand("", {
        backendUrl: BACKEND,
        _mockFetch: vi.fn(),
        _mockReadFile: () => "",
        _mockRealpath: identityRealpath,
        repoPath: FAKE_REPO,
      }),
    ).rejects.toThrow("file_path is required")
  })

  it("throws when file cannot be read", async () => {
    // _mockStat passes (size check ok); _mockReadFile throws a generic read error
    // (no .code property) → implementation wraps as "Error reading file: ..."
    await expect(
      handleAnnotateCommand("nonexistent.ts", {
        backendUrl: BACKEND,
        _mockFetch: vi.fn(),
        _mockReadFile: () => { throw new Error("permission denied") },
        _mockStat: () => ({ size: 100 }),
        _mockRealpath: identityRealpath,
        repoPath: FAKE_REPO,
      }),
    ).rejects.toThrow("Error reading file")
  })

  it("throws when space creation fails", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: false, status: 500, statusText: "Server Error",
    } as Response)
    await expect(
      handleAnnotateCommand("README.md", {
        backendUrl: BACKEND,
        _mockFetch: mockFetch,
        _mockReadFile: () => "content",
        _mockStat: () => ({ size: 100 }),
        _mockRealpath: identityRealpath,
        repoPath: FAKE_REPO,
      }),
    ).rejects.toThrow("Failed to create artifact_review Space")
  })

  it("throws when item creation fails", async () => {
    const mockFetch = mockFetchSequence([
      { ok: true, json: async () => ({ space_id: "sp-1", title: "t" }) },
      { ok: false, status: 422, statusText: "Unprocessable Entity" },
    ])
    await expect(
      handleAnnotateCommand("README.md", {
        backendUrl: BACKEND,
        _mockFetch: mockFetch,
        _mockReadFile: () => "content",
        _mockStat: () => ({ size: 100 }),
        _mockRealpath: identityRealpath,
        repoPath: FAKE_REPO,
      }),
    ).rejects.toThrow("Failed to create artifact_review item")
  })

  it("sends Authorization header when authToken provided", async () => {
    const mockFetch = mockFetchSequence([
      { ok: true, json: async () => ({ space_id: "sp-auth", title: "t" }) },
      { ok: true, json: async () => ({ item_id: "item-auth" }) },
    ])
    await handleAnnotateCommand("file.ts", {
      backendUrl: BACKEND,
      authToken: "my-token",
      _mockFetch: mockFetch,
      _mockReadFile: () => "code",
      _mockStat: () => ({ size: 100 }),
      _mockRealpath: identityRealpath,
      repoPath: FAKE_REPO,
    })
    expect(mockFetch.mock.calls[0][1].headers["Authorization"]).toBe("Bearer my-token")
    expect(mockFetch.mock.calls[1][1].headers["Authorization"]).toBe("Bearer my-token")
  })

  // test_annotate_rejects_oversized_file
  it("test_annotate_rejects_oversized_file — returns error for files > 1MB", async () => {
    const mockFetch = mockFetchSequence([
      { ok: true, json: async () => ({ space_id: "sp-large", title: "t" }) },
      { ok: true, json: async () => ({ item_id: "item-large" }) },
    ])
    await expect(
      handleAnnotateCommand("large.bin", {
        backendUrl: BACKEND,
        _mockFetch: mockFetch,
        _mockReadFile: () => "content",
        _mockStat: () => ({ size: 2 * 1024 * 1024 }), // 2MB
        _mockRealpath: identityRealpath,
        repoPath: FAKE_REPO,
      }),
    ).rejects.toThrow("File too large")
  })

  // test_annotate_rejects_path_traversal
  it("test_annotate_rejects_path_traversal — returns error for paths outside repo", async () => {
    const mockFetch = mockFetchSequence([
      { ok: true, json: async () => ({ space_id: "sp-traversal", title: "t" }) },
      { ok: true, json: async () => ({ item_id: "item-traversal" }) },
    ])
    await expect(
      handleAnnotateCommand("../../etc/passwd", {
        backendUrl: BACKEND,
        _mockFetch: mockFetch,
        _mockReadFile: () => "content",
        _mockRealpath: identityRealpath,
        repoPath: FAKE_REPO,
      }),
    ).rejects.toThrow("outside the repository boundary")
  })

  // test_annotate_file_not_found
  it("test_annotate_file_not_found — returns specific error for ENOENT", async () => {
    const mockFetch = mockFetchSequence([
      { ok: true, json: async () => ({ space_id: "sp-enoent", title: "t" }) },
      { ok: true, json: async () => ({ item_id: "item-enoent" }) },
    ])
    const error = new Error("ENOENT: no such file or directory")
    ;(error as NodeJS.ErrnoException).code = "ENOENT"
    await expect(
      handleAnnotateCommand("nonexistent.ts", {
        backendUrl: BACKEND,
        _mockFetch: mockFetch,
        _mockReadFile: () => { throw error },
        _mockStat: () => ({ size: 100 }),
        _mockRealpath: identityRealpath,
        repoPath: FAKE_REPO,
      }),
    ).rejects.toThrow("File not found")
  })

  // test_annotate_allows_symlink_inside_repo
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-041 plan=phase-2.10/task-2.10.symlink-intra-repo-test/step-1
  it("test_annotate_allows_symlink_inside_repo — allows valid symlink within repo", async () => {
    // Simulate: /fake-repo/link.md -> /fake-repo/real.md (both inside repo)
    // realpath is called twice:
    //   1) realpath(resolve(repoPath))          → "/fake-repo"  (identity)
    //   2) realpath(resolve("/fake-repo", "link.md")) → "/fake-repo/real.md" (symlink resolved)
    // The boundary check: "/fake-repo/real.md".startsWith("/fake-repo/") → ALLOWED
    const mockRealpath = (p: string) => {
      if (p.includes("link.md")) return "/fake-repo/real.md"
      return p
    }
    const mockFetch = mockFetchSequence([
      { ok: true, json: async () => ({ space_id: "sp-symlink", title: "Artifact Review: link.md" }) },
      { ok: true, json: async () => ({ item_id: "item-symlink" }) },
    ])
    const result = await handleAnnotateCommand("link.md", {
      backendUrl: BACKEND,
      _mockFetch: mockFetch,
      _mockReadFile: () => "file content via symlink",
      _mockStat: () => ({ size: 100 }),
      _mockRealpath: mockRealpath,
      repoPath: FAKE_REPO,
    })
    // Should succeed — symlink target /fake-repo/real.md is inside the repo
    expect(result.spaceId).toBe("sp-symlink")
    expect(result.itemId).toBe("item-symlink")
    expect(result.url).toContain("sp-symlink")
    expect(result.filePath).toBe("link.md")
  })
})
