/**
 * Tests for disk flush module — atomic write-to-temp-then-rename.
 *
 * REQ-PLG-121: Disk flush cadence with atomic write pattern.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-121
 * axiom:trace work_item=plugin-metrics-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-121 plan=phase-107-3 jira_ref=DEX-392
 */

import { describe, it, expect, beforeEach, afterEach } from "bun:test"
import { mkdirSync, writeFileSync, existsSync, readFileSync, readdirSync, rmSync } from "fs"
import { join } from "path"
import { tmpdir } from "os"
import { createDiskFlusher } from "../src/metrics/disk-flush"

describe("DiskFlusher (REQ-PLG-121)", () => {
  let testDir: string

  beforeEach(() => {
    testDir = join(tmpdir(), `disk-flush-test-${Date.now()}`)
    mkdirSync(testDir, { recursive: true })
  })

  afterEach(() => {
    try { rmSync(testDir, { recursive: true, force: true }) } catch {}
  })

  it("writes data to final path (no .tmp left behind)", () => {
    const sessionsDir = join(testDir, "sessions")
    const flusher = createDiskFlusher({ sessionsDir, sessionId: "test-001" })

    flusher.flush({ hello: "world" })

    const finalPath = join(sessionsDir, "test-001.json")
    const tmpPath = join(sessionsDir, "test-001.json.tmp")

    expect(existsSync(finalPath)).toBe(true)
    expect(existsSync(tmpPath)).toBe(false) // .tmp should be renamed away

    const content = JSON.parse(readFileSync(finalPath, "utf-8"))
    expect(content.hello).toBe("world")
  })

  it("creates sessions directory if it does not exist", () => {
    const sessionsDir = join(testDir, "nonexistent", "sessions")
    const flusher = createDiskFlusher({ sessionsDir, sessionId: "test-002" })

    flusher.flush({ data: 42 })

    expect(existsSync(join(sessionsDir, "test-002.json"))).toBe(true)
  })

  it("overwrites existing file on subsequent flushes", () => {
    const sessionsDir = join(testDir, "sessions")
    const flusher = createDiskFlusher({ sessionsDir, sessionId: "test-003" })

    flusher.flush({ version: 1 })
    flusher.flush({ version: 2 })

    const content = JSON.parse(readFileSync(join(sessionsDir, "test-003.json"), "utf-8"))
    expect(content.version).toBe(2)
  })

  it("cleanup removes stale .tmp files", () => {
    const sessionsDir = join(testDir, "sessions")
    mkdirSync(sessionsDir, { recursive: true })

    // Create stale .tmp files (simulating crash)
    writeFileSync(join(sessionsDir, "old-session.json.tmp"), "{}")
    writeFileSync(join(sessionsDir, "another.json.tmp"), "{}")
    // Create a real session file that should NOT be removed
    writeFileSync(join(sessionsDir, "good-session.json"), "{}")

    const flusher = createDiskFlusher({ sessionsDir, sessionId: "new-session" })
    flusher.cleanup()

    expect(existsSync(join(sessionsDir, "old-session.json.tmp"))).toBe(false)
    expect(existsSync(join(sessionsDir, "another.json.tmp"))).toBe(false)
    expect(existsSync(join(sessionsDir, "good-session.json"))).toBe(true) // preserved
  })

  it("cleanup handles missing sessions directory gracefully", () => {
    const sessionsDir = join(testDir, "nonexistent-dir")
    const flusher = createDiskFlusher({ sessionsDir, sessionId: "test" })

    // Should not throw
    flusher.cleanup()
  })

  it("writes valid JSON with pretty formatting", () => {
    const sessionsDir = join(testDir, "sessions")
    const flusher = createDiskFlusher({ sessionsDir, sessionId: "test-004" })

    const data = { session_id: "test-004", tokens: { input: 100, output: 50 } }
    flusher.flush(data)

    const raw = readFileSync(join(sessionsDir, "test-004.json"), "utf-8")
    // Pretty-printed JSON should have newlines
    expect(raw).toContain("\n")
    expect(JSON.parse(raw)).toEqual(data)
  })
})
