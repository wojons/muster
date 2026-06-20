# Coding Tasks — Muster

## [x] Fix repo README.md for open source
- **Completed:** MIT license, all 7 surfaces, correct URLs/commands, real quickstart

## [x] Fix demo/README.md 
- **Completed:** One-command demo, actual directory structure, prerequisites

## [x] Add LICENSE (MIT)
- **Completed:** Standard MIT, copyright wojons

## [x] Add CONTRIBUTING.md
- **Completed:** Fork → branch → build → demo → PR flow

## [x] Add SECURITY.md
- **Completed:** Reporting, supported versions, scope

## [x] Add CHANGELOG.md
- **Completed:** All features, fork changes, removals

## [x] Add CODE_OF_CONDUCT.md
- **Completed:** Contributor Covenant 2.1

## [x] Add GitHub issue templates
- **Completed:** Bug report + feature request

## [x] Add .gitignore entries
- **Completed:** __pycache__/, *.pyc, _storage/

## [ ] Fix D-Bus Secret Service stubs
- **Priority:** low
- **Model:** deepseek-v4-pro
- **Files:** internal/platform/linux/keyring_secret_service.go, internal/platform/linux/credential.go
- **AC:** Linux credential store uses actual D-Bus Secret Service protocol

## [x] Wire functional workflow tests
- **Priority:** low
- **Completed:** 2026-06-20 — Replaced TODO stubs with real spec parsing + command generation. New TestUserWorkflow_SpecToCommands validates full pipeline. 5/5 tests pass.

## [x] Add unit tests for DSL package
- **Priority:** low
- **Completed:** 2026-06-19 — 20 tests: NewInterpreter (empty, with builtins), Parse (valid, function, invalid), Compile (valid, nil), Exec (basic, print, error), Call (positional, kwargs, builtin), SetGlobal/GetGlobal (set, missing, after-exec, multiple, overwrite), Globals, EndToEnd (factorial). All pass.

## [x] Implement auth.Token struct (AC-260)
- **Completed:** 2026-06-19 — Token struct, TokenManager (auto-refresh), AuthorizationCodeFlow (Start/Exchange). 8/8 OAuth2 e2e subtests pass. commit 53ed73c.
- **Note:** Keychain integration tests fail due to missing D-Bus Secret Service on this host (pre-existing env issue).

## [x] Implement wasm network policy API (AC-270)
- **Completed:** 2026-06-19 — NewCapabilitySet(), Grant(), Revoke(), SetNetworkPolicy(), CheckNetworkAccess(). Wildcard capability matching. 14/14 WASM security tests pass. build-tag-gate removed from test/security/wasm_security_test.go.

## [x] Add version --output json flag (AC-280)
- **Completed:** 2026-06-19 — version --output flag added. Produces valid JSON: {"Version":"dev","Commit":"unknown",...}. commit pending.
