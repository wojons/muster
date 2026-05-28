// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-309 plan=phase-70-7/task-70-7-1/step-70-7-1-1 jira_ref=DEX-64
import { describe, it, expect } from "bun:test";

describe("REQ-PLG-309: chat.message hook for L4 conflict detection", () => {
  it("chat.message hook is registered in plugin return object", async () => {
    const { readFileSync } = await import("node:fs");
    const { join } = await import("node:path");
    const src = readFileSync(join(import.meta.dir, "../src/index.ts"), "utf-8");
    expect(src).toContain('"chat.message"');
    expect(src).toContain("REQ-PLG-309");
  });

  it("chat.message hook resets idle detector (same as event hook)", async () => {
    const { readFileSync } = await import("node:fs");
    const { join } = await import("node:path");
    const src = readFileSync(join(import.meta.dir, "../src/index.ts"), "utf-8");
    // The chat.message hook must reset the idle detector
    const chatMsgIdx = src.indexOf('"chat.message"');
    const idleResetIdx = src.indexOf("idleDetector.reset()", chatMsgIdx);
    expect(chatMsgIdx).toBeGreaterThan(0);
    expect(idleResetIdx).toBeGreaterThan(chatMsgIdx);
  });

  it("chat.message hook respects quiet mode (REQ-PLG-308)", async () => {
    const { readFileSync } = await import("node:fs");
    const { join } = await import("node:path");
    const src = readFileSync(join(import.meta.dir, "../src/index.ts"), "utf-8");
    // The chat.message hook must check quiet mode before conflict detection
    const chatMsgIdx = src.indexOf('"chat.message"');
    const quietModeIdx = src.indexOf("quietModeTimer", chatMsgIdx);
    expect(quietModeIdx).toBeGreaterThan(chatMsgIdx);
  });

  it("detectConflicts is available from network layer", async () => {
    const { detectConflicts } = await import("../src/layers/network");
    expect(typeof detectConflicts).toBe("function");
    // detectConflicts(workItem, userId, members) returns array
    const result = detectConflicts("WORK-123", "user-1", []);
    expect(Array.isArray(result)).toBe(true);
    expect(result).toHaveLength(0); // no conflicts with empty members
  });
});
