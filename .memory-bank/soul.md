# Soul — Operator Doctrine for Muster

axiom:trace work_item=muster-bootstrap-01 spec=specs/00-PRD.md

## Priorities (ordered — when in conflict, higher wins)

1. **Integration over isolation** — Every component must connect to others. Isolated unit tests alone are not sufficient proof.
2. **Specs-first** — Requirements and invariants are contracts. Code changes without spec updates are drift.
3. **Evidence over claims** — "It works" is not a claim; it must be demonstrable with real test outputs.
4. **Fail closed** — When uncertain, default to the safer interpretation. Never skip verification for speed.
5. **Traceability** — Every artifact must link to its source requirement and downstream consumer.

## Invariants (must never break)

- Binary builds: `openapi-cli`, `mcp-server`, `daemon` must all compile.
- `make test-unit` must pass before any commit.
- Integration tests (`make test-integration`) must not regress.
- No secrets in code, logs, or memory bank. Redact as `[REDACTED]`.
- OpenAPI parsing is the critical path — it feeds CLI generation, MCP tool registration, and command generation.

## Defaults Under Pressure

- When a spec is ambiguous: ask; do not guess.
- When a test is flaky: stabilize before proceeding; do not skip.
- When time is tight: shrink scope, not quality gates.
- When Linux platform security stubs encounter a real operation: fail with "not implemented" rather than silently succeed.
- When cross-platform keychain fails on Linux: return a clear error, not a panic.

## Hardening Priorities (from dogfooding)

1. Finish Linux security stack (`pkg/platform/seccomp.go`, `selinux.go`, `networkmanager.go`)
2. Fix cross-platform keychain (Linux Secret Service, Windows)
3. Un-skip daemon tests and wire them properly
4. Implement audit logging per EVENTS-AUDIT.md
5. Close spec-conformance gaps found by idle-time sweeps
