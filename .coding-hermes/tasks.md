# Coding Tasks — Muster

## [ ] Fix repo README.md for open source
- **Priority:** high
- **Model:** deepseek-v4-pro
- **Files:** README.md
- **AC:** 
  - All URLs point to github.com/wojons/muster (not anomalyco)
  - Binary names match actual binaries: openapi-cli, openapi-mcp, openapi-daemon
  - Remove broken CI/coverage/version badges or replace with working ones
  - Quick Start section shows actual commands that work: `git clone`, `go build ./...`, `./demo/run-all.sh`
  - Remove Homebrew/Docker/Go install sections that reference non-existent releases
  - Development section shows `go build -o bin/openapi-cli ./cmd/openapi-cli` etc.
  - Project structure matches actual layout
  - Keep the feature descriptions — they're accurate

## [ ] Fix demo/README.md 
- **Priority:** high
- **Model:** deepseek-v4-pro
- **Files:** demo/README.md
- **AC:**
  - Shows the real demo: `cd demo && ./run-all.sh` (one command)
  - Lists the 10 actual demo scripts with what each demonstrates
  - Shows the 5 services with ports
  - Prerequisites: Go 1.23+, Python 3, Flask
  - No references to non-existent test-suites/, make targets, or mock servers

## [ ] Add LICENSE (MIT)
- **Priority:** high
- **Model:** deepseek-v4-flash
- **Files:** LICENSE (new)
- **AC:** Standard MIT license, copyright holder: wojons

## [ ] Add LICENSE-APACHE (Apache 2.0)
- **Priority:** high
- **Model:** deepseek-v4-flash
- **Files:** LICENSE-APACHE (new)
- **AC:** Standard Apache License 2.0 boilerplate

## [ ] Add CONTRIBUTING.md
- **Priority:** medium
- **Model:** deepseek-v4-pro
- **Files:** CONTRIBUTING.md (new)
- **AC:** Fork → branch → build → test → PR flow. Reference `./demo/run-all.sh` as the test suite. Go fmt + vet requirements.

## [ ] Add CODE_OF_CONDUCT.md
- **Priority:** medium
- **Model:** deepseek-v4-flash
- **Files:** CODE_OF_CONDUCT.md (new)
- **AC:** Contributor Covenant 2.1 standard

## [ ] Add SECURITY.md
- **Priority:** medium
- **Model:** deepseek-v4-flash
- **Files:** SECURITY.md (new)
- **AC:** How to report vulnerabilities, supported versions, disclosure policy

## [ ] Add CHANGELOG.md
- **Priority:** medium
- **Model:** deepseek-v4-pro
- **Files:** CHANGELOG.md (new)
- **AC:** Document what's in the current release — all 3 consumers (CLI/MCP/Daemon), WASM, DSL, 5 demo services, 10 demo scripts

## [ ] Add GitHub issue templates
- **Priority:** low
- **Model:** deepseek-v4-flash
- **Files:** .github/ISSUE_TEMPLATE/bug_report.md, .github/ISSUE_TEMPLATE/feature_request.md (new)
- **AC:** Standard bug report and feature request templates

## [ ] Add .gitignore entries
- **Priority:** low
- **Model:** deepseek-v4-flash
- **Files:** .gitignore (edit)
- **AC:** Add __pycache__/, _storage/, .DS_Store, *.pyc patterns

## [ ] Fix D-Bus Secret Service stubs
- **Priority:** low
- **Model:** deepseek-v4-pro
- **Files:** internal/platform/linux/keyring_secret_service.go, internal/platform/linux/credential.go
- **AC:** Linux credential store uses actual D-Bus Secret Service protocol

## [ ] Wire functional workflow tests
- **Priority:** low
- **Model:** deepseek-v4-pro
- **Files:** test/functional/workflows/cli_workflow_test.go
- **AC:** Tests load spec and generate commands instead of TODO stubs

## [ ] Add unit tests for DSL package
- **Priority:** low
- **Model:** deepseek-v4-pro
- **Files:** pkg/dsl/interpreter_test.go (new)
- **AC:** Parse, Compile, Exec, Call, SetGlobal, GetGlobal all covered
