// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-005 plan=phase-70-ts-tests/task-ts-2/step-ts-2-1
import { describe, it, expect } from "bun:test";
import { isVersionCompatible, findSdkPackageJson } from "../src/version";
import { existsSync } from "node:fs";
import { join } from "node:path";

// ---------------------------------------------------------------------------
// isVersionCompatible
// ---------------------------------------------------------------------------

describe("isVersionCompatible", () => {
  it("returns true when current >= minimum", () => {
    expect(isVersionCompatible("1.2.0", "1.1.53")).toBe(true);
    expect(isVersionCompatible("1.1.53", "1.1.53")).toBe(true);
    expect(isVersionCompatible("2.0.0", "1.1.53")).toBe(true);
  });

  it("returns false when current < minimum", () => {
    expect(isVersionCompatible("1.1.52", "1.1.53")).toBe(false);
    expect(isVersionCompatible("1.0.0", "1.1.53")).toBe(false);
    expect(isVersionCompatible("0.9.0", "1.1.53")).toBe(false);
  });

  it("handles v-prefix", () => {
    expect(isVersionCompatible("v1.2.0", "1.1.53")).toBe(true);
    expect(isVersionCompatible("v1.1.52", "1.1.53")).toBe(false);
  });

  it("handles pre-release suffix", () => {
    expect(isVersionCompatible("1.2.0-beta.1", "1.1.53")).toBe(true);
  });

  it("returns false for empty or unparseable version", () => {
    expect(isVersionCompatible("", "1.1.53")).toBe(false);
    expect(isVersionCompatible("not-a-version", "1.1.53")).toBe(false);
  });

  it("compares patch correctly at boundary", () => {
    expect(isVersionCompatible("1.1.53", "1.1.53")).toBe(true);
    expect(isVersionCompatible("1.1.54", "1.1.53")).toBe(true);
    expect(isVersionCompatible("1.1.52", "1.1.53")).toBe(false);
  });

  it("major version dominates minor/patch", () => {
    // major 2 > major 1 regardless of minor/patch
    expect(isVersionCompatible("2.0.0", "1.9.99")).toBe(true);
    // major 1 < major 2 regardless of minor/patch
    expect(isVersionCompatible("1.99.99", "2.0.0")).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// findSdkPackageJson — real filesystem tests
//
// version.ts uses `import { existsSync } from "node:fs"` (destructured), so
// module-level spying cannot intercept the already-bound reference.
// We test with real paths instead:
//   • A repo dir that is known to have the SDK installed (this plugin's own
//     worktree root, which has .opencode/node_modules) → non-null path.
//   • A clearly non-existent directory → repo-local candidate is absent.
// ---------------------------------------------------------------------------

describe("findSdkPackageJson", () => {
  // The worktree root has .opencode/node_modules/@opencode-ai/plugin/package.json
  const WORKTREE_ROOT = join(import.meta.dir, "../../..");
  // Skip real-filesystem tests when .opencode/node_modules is not installed
  // (e.g., CI jobs that only install .axiom/plugin/node_modules)
  const OPENCODE_SDK_PRESENT = existsSync(
    join(WORKTREE_ROOT, ".opencode", "node_modules", "@opencode-ai", "plugin", "package.json"),
  );

  it("returns a non-null path when the SDK is installed under .opencode/node_modules", () => {
    if (!OPENCODE_SDK_PRESENT) return; // skip: .opencode/node_modules not installed in this env
    const result = findSdkPackageJson(WORKTREE_ROOT);
    expect(result).not.toBeNull();
    expect(result).toContain("package.json");
  });

  it("returned path contains .opencode (repo-local candidate wins)", () => {
    if (!OPENCODE_SDK_PRESENT) return;
    const result = findSdkPackageJson(WORKTREE_ROOT);
    expect(result).not.toBeNull();
    expect(result!).toContain(".opencode");
  });

  it("returned path is a real file that exists on disk", () => {
    if (!OPENCODE_SDK_PRESENT) return;
    const result = findSdkPackageJson(WORKTREE_ROOT);
    expect(result).not.toBeNull();
    expect(existsSync(result!)).toBe(true);
  });

  it("tries repo-local path first — returned path starts with repoDir", () => {
    if (!OPENCODE_SDK_PRESENT) return;
    const result = findSdkPackageJson(WORKTREE_ROOT);
    expect(result).not.toBeNull();
    expect(result!.startsWith(WORKTREE_ROOT)).toBe(true);
  });

  it("repo-local candidate is absent for a non-existent repoDir", () => {
    const bogusDir = "/nonexistent-axiom-test-dir-xyzzy-12345";
    const repoLocalCandidate = join(
      bogusDir,
      ".opencode",
      "node_modules",
      "@opencode-ai",
      "plugin",
      "package.json",
    );
    // The repo-local candidate must not exist for a bogus dir
    expect(existsSync(repoLocalCandidate)).toBe(false);
    // If findSdkPackageJson returns non-null for a bogus dir, it must be
    // a real file (e.g. a global cache hit) — we don't assert null because
    // the developer's machine may have a global install
    const result = findSdkPackageJson(bogusDir);
    if (result !== null) {
      expect(existsSync(result)).toBe(true);
    }
  });
});
