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

## [ ] Implement auth.Token struct (AC-260)
- **Priority:** medium
- **Model:** deepseek-v4-pro --provider ollama-cloud
- **Files:** pkg/auth/token.go (new), restore test/e2e/auth_keychain_integration_test.go, test/e2e/auth_real_integration_test.go
- **AC:** Token struct with AccessToken, RefreshToken, Expiry, TokenType fields. Build-tag-gated e2e tests un-gated and pass.

## [ ] Implement wasm network policy API (AC-270)
- **Priority:** medium  
- **Model:** glm-5.2 --provider zai-glm
- **Files:** pkg/wasm/network.go (new), restore test/security/wasm_security_test.go
- **AC:** NewCapabilitySet(), Runtime.CheckNetworkAccess(host, port), Runtime.SetNetworkPolicy(policy). Build-tag-gated security test un-gated and passes.

## [ ] Add version --output json flag (AC-280)
- **Priority:** low
- **Model:** deepseek-v4-pro --provider ollama-cloud
- **Files:** internal/builtin/version.go
- **AC:** version --output json returns {"version":"dev","commit":"unknown"} JSON
