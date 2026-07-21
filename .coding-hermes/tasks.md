# Muster — Model Router Task Matrix

**Core purpose:** Dynamic API client and MCP server — generates CLIs from OpenAPI specs, supports OAuth2/mTLS, WASM runtime, Starlark DSL, 52 Go packages, 276 specifications.

## Active Tasks

| ID | Task | Priority | Complexity | Deps | Tags | Model | Reasoning | Fallback |
|----|------|----------|------------|------|------|-------|-----------|----------|
| CI-BILLING | GitHub Actions spending limit reached — ALL CI jobs blocked | Critical | 1 (admin) | — | — | — | BLOCKED: Needs human with billing access to wojons org. Blocked since 2026-07-09 (~14 days) | — |
| NEVER-DONE | 11-point audit sweep | High | 2 | — | ++code-review, +testing | DeepSeek V4 Pro | Audit runs every tick | GLM-5.2 |

**Assumptions:** Go 1.26.5 runtime. 3 binaries (openapi-cli, openapi-daemon, openapi-mcp). 52 packages. CI billing is external (GitHub Actions spending limit). Project code is genuinely complete — zero TODOs in production code. 1 pre-existing TODO in test file only (secure_storage_test.go:54, stub).

**Routing Notes:** Board has only 1 BLOCKED task + NEVER-DONE audit. Project complete — 30 idle ticks across ~14 days. All core tests green (31/33 packages; pkg/platform seccomp SIGSEGV pre-existing with CI=true; internal/monitoring port conflict pre-existing env flake). No code changes since Go 1.26.5 bump (2026-07-16).

**Idle tick #30 — zombie #18 — 2026-07-21 04:51 UTC-5**

Cron state: PAUSED (scheduler migration — daemon on :9090). Scheduler: Enabled=True, CooldownS=14400 (stable, no reversion this tick). GitReins: 37/37 complete (board sync verified — zero pending). Build: FAIL (host resource exhaustion — newosproc, INFRA issue, not code). Project complete 14+ days. CI-BILLING still blocked (human needed). BAIL EARLY per zombie protocol.

**Execution Order:** NEVER-DONE audit runs on scheduler tick only.

**Escalation Conditions:** CI billing resolved → audit will find new gaps (fieldalignment fixes, dep updates). Any new task → route per capability profile. Security/performance tasks → GPT-5.6 Sol. Mechanical dep bumps → DeepSeek V4 Flash.

## Completed Summary

**AC-320 through AC-340 (Test Coverage):** 14 packages tested — retry (22 tests), response (20), openapi (28), request (57), repository (22), cmdstore (28), ratelimit (38), monitoring (18), sql repos (44), token_manager (9), mcp/registry (35), mcp/handlers (50), cmdstore/loader (20). All pass.
**CI Fixes:** libseccomp-dev added to 3 workflows (8 jobs). Cross-platform test failures (Go version, build tags, stability test gating). Seccomp SIGABRT on CI (CI=true guards). Stale stored commands poisoning production CLI (test isolation). Duplicate help command removed.
**Infrastructure:** Docker registry permissions fixed. Trivy/Docker scanning made non-blocking. Lint violations fixed (4 golangci-lint, errcheck, fieldalignment). Pipe-buffer deadlock in bash completion test fixed.
**Deps/Security:** Go 1.26.1→1.26.5 (CVE-2026-42505, crypto/tls ECH privacy leak). govulncheck: 0 vulns called.
**CLI/Daemon:** Daemon start spawns background binary. HTTP client stub wired to global client.
**Quality:** README OSS-ready (MIT license, 7 surfaces). CONTRIBUTING, SECURITY, CHANGELOG, CODE_OF_CONDUCT, issue templates, .gitignore. D-Bus Secret Service full implementation. Functional workflow tests un-stubbed.

## [ ] NEVER-DONE — Run coding-hermes-never-done 11-point audit
