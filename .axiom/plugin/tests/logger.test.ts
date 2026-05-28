// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-134 plan=phase-70-ts-tests/task-ts-2/step-ts-2-1
import { describe, it, expect, spyOn } from "bun:test";
import { createFallbackLogger } from "../src/logger";

describe("createFallbackLogger", () => {
  it("routes warn() to process.stderr, never stdout", () => {
    const stderrSpy = spyOn(process.stderr, "write").mockImplementation(() => true);
    const stdoutSpy = spyOn(process.stdout, "write").mockImplementation(() => true);

    const log = createFallbackLogger();
    log.warn("test warning message");

    expect(stderrSpy).toHaveBeenCalledWith(expect.stringContaining("test warning message"));
    expect(stdoutSpy).not.toHaveBeenCalled();

    stderrSpy.mockRestore();
    stdoutSpy.mockRestore();
  });

  it("routes error() to process.stderr, never stdout", () => {
    const stderrSpy = spyOn(process.stderr, "write").mockImplementation(() => true);
    const stdoutSpy = spyOn(process.stdout, "write").mockImplementation(() => true);

    const log = createFallbackLogger();
    log.error("test error message");

    expect(stderrSpy).toHaveBeenCalledWith(expect.stringContaining("test error message"));
    expect(stdoutSpy).not.toHaveBeenCalled();

    stderrSpy.mockRestore();
    stdoutSpy.mockRestore();
  });

  it("includes [axiom plugin] prefix in output", () => {
    const stderrSpy = spyOn(process.stderr, "write").mockImplementation(() => true);

    const log = createFallbackLogger();
    log.warn("prefix check");

    const output = stderrSpy.mock.calls[0][0] as string;
    expect(output).toContain("[axiom plugin]");

    stderrSpy.mockRestore();
  });

  it("includes WARN label in warn() output", () => {
    const stderrSpy = spyOn(process.stderr, "write").mockImplementation(() => true);

    const log = createFallbackLogger();
    log.warn("label check");

    const output = stderrSpy.mock.calls[0][0] as string;
    expect(output).toContain("WARN");

    stderrSpy.mockRestore();
  });

  it("includes ERROR label in error() output", () => {
    const stderrSpy = spyOn(process.stderr, "write").mockImplementation(() => true);

    const log = createFallbackLogger();
    log.error("error label check");

    const output = stderrSpy.mock.calls[0][0] as string;
    expect(output).toContain("ERROR");

    stderrSpy.mockRestore();
  });

  it("output ends with newline", () => {
    const stderrSpy = spyOn(process.stderr, "write").mockImplementation(() => true);

    const log = createFallbackLogger();
    log.warn("newline check");

    const output = stderrSpy.mock.calls[0][0] as string;
    expect(output.endsWith("\n")).toBe(true);

    stderrSpy.mockRestore();
  });
});
