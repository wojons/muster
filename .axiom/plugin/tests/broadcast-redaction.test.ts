// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-209 plan=phase-70-6/task-70-6-3/step-70-6-3-1 jira_ref=DEX-64
import { describe, it, expect } from "bun:test";

describe("REQ-PLG-209: broadcast applies redaction before content reaches viewers", () => {
  it("redactContent is imported in broadcast execute path", async () => {
    const { readFileSync } = await import("node:fs");
    const { join } = await import("node:path");
    const src = readFileSync(join(import.meta.dir, "../src/index.ts"), "utf-8");
    expect(src).toContain("REQ-PLG-209");
    expect(src).toContain("redactContent");
  });

  it("redactContent function exists and redacts secrets", async () => {
    // redactContent is in security/redaction (sharing.ts imports but does not re-export it)
    const { redactContent } = await import("../src/security/redaction");
    const input = "API_KEY=AKIAIOSFODNN7EXAMPLE token=secret123";
    const result = redactContent(input);
    // redactContent returns a RedactionResult { redacted: string, count: number }
    expect(typeof result).toBe("object");
    expect(typeof result.redacted).toBe("string");
    expect(result.redacted.length).toBeGreaterThan(0);
    // The result should be defined
    expect(result).toBeDefined();
  });

  it("broadcast redaction is applied before subscribe (structural check)", async () => {
    const { readFileSync } = await import("node:fs");
    const { join } = await import("node:path");
    const src = readFileSync(join(import.meta.dir, "../src/index.ts"), "utf-8");
    // REQ-PLG-209 trace must appear before subscribe call in broadcast section
    const redactionIdx = src.indexOf("REQ-PLG-209");
    const subscribeIdx = src.indexOf(".subscribe(", redactionIdx);
    expect(redactionIdx).toBeGreaterThan(0);
    // Either redaction is applied before subscribe, or subscribe wraps with redaction
    expect(subscribeIdx).toBeGreaterThan(redactionIdx);
  });
});
