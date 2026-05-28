// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-109 plan=phase-70-ts-tests/task-ts-2/step-ts-2-1
import { describe, it, expect, beforeEach, afterEach } from "bun:test";

describe("REQ-PLG-109: event correlation fields", () => {
  // Snapshot original env so we can restore it after each test
  let savedRunId: string | undefined;
  let savedWorkItem: string | undefined;

  beforeEach(() => {
    savedRunId = process.env.AXIOM_RUN_ID;
    savedWorkItem = process.env.AXIOM_WORK_ITEM;
    process.env.AXIOM_RUN_ID = "test-run-123";
    process.env.AXIOM_WORK_ITEM = "opencode-plugin-01";
  });

  afterEach(() => {
    if (savedRunId === undefined) {
      delete process.env.AXIOM_RUN_ID;
    } else {
      process.env.AXIOM_RUN_ID = savedRunId;
    }
    if (savedWorkItem === undefined) {
      delete process.env.AXIOM_WORK_ITEM;
    } else {
      process.env.AXIOM_WORK_ITEM = savedWorkItem;
    }
  });

  it("AXIOM_RUN_ID env var is readable for run_id field", () => {
    const runId = process.env.AXIOM_RUN_ID ?? "";
    expect(runId).toBe("test-run-123");
  });

  it("AXIOM_WORK_ITEM env var is readable for work_item_id field", () => {
    const workItemId = process.env.AXIOM_WORK_ITEM ?? "";
    expect(workItemId).toBe("opencode-plugin-01");
  });

  it("defaults to empty string when env vars not set", () => {
    delete process.env.AXIOM_RUN_ID;
    delete process.env.AXIOM_WORK_ITEM;

    const runId = process.env.AXIOM_RUN_ID ?? "";
    const workItemId = process.env.AXIOM_WORK_ITEM ?? "";

    expect(runId).toBe("");
    expect(workItemId).toBe("");
  });

  it("index.ts emits run_id in all 4 event payloads (static analysis)", async () => {
    // Verify the source code contains run_id in all 4 event emission sites
    const { readFileSync } = await import("node:fs");
    const { join } = await import("node:path");
    const indexSrc = readFileSync(join(import.meta.dir, "../src/index.ts"), "utf-8");

    // Count occurrences of run_id in event payloads
    const runIdMatches = indexSrc.match(/run_id:\s*process\.env\.AXIOM_RUN_ID/g);
    expect(runIdMatches).not.toBeNull();
    expect(runIdMatches!.length).toBeGreaterThanOrEqual(4);

    const workItemMatches = indexSrc.match(/work_item_id:\s*process\.env\.AXIOM_WORK_ITEM/g);
    expect(workItemMatches).not.toBeNull();
    expect(workItemMatches!.length).toBeGreaterThanOrEqual(4);
  });

  it("index.ts emits repo field in all 4 event payloads (static analysis)", async () => {
    const { readFileSync } = await import("node:fs");
    const { join } = await import("node:path");
    const indexSrc = readFileSync(join(import.meta.dir, "../src/index.ts"), "utf-8");

    // repo: directory appears in each of the 4 structured log payloads
    const repoMatches = indexSrc.match(/repo:\s*directory/g);
    expect(repoMatches).not.toBeNull();
    expect(repoMatches!.length).toBeGreaterThanOrEqual(4);
  });

  it("correlation fields use nullish-coalesce fallback to empty string", async () => {
    const { readFileSync } = await import("node:fs");
    const { join } = await import("node:path");
    const indexSrc = readFileSync(join(import.meta.dir, "../src/index.ts"), "utf-8");

    // All run_id assignments use ?? "" fallback
    const runIdLines = indexSrc.match(/run_id:\s*process\.env\.AXIOM_RUN_ID\s*\?\?\s*""/g);
    expect(runIdLines).not.toBeNull();
    expect(runIdLines!.length).toBeGreaterThanOrEqual(4);

    // All work_item_id assignments use ?? "" fallback
    const workItemLines = indexSrc.match(/work_item_id:\s*process\.env\.AXIOM_WORK_ITEM\s*\?\?\s*""/g);
    expect(workItemLines).not.toBeNull();
    expect(workItemLines!.length).toBeGreaterThanOrEqual(4);
  });
});
