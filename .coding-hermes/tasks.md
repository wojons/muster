# Coding Tasks — Muster

## [ ] Run demo suite health check
- **Priority:** high
- **Model:** deepseek-v4-flash
- **Type:** verification
- **Command:** `cd /home/kara/muster && go build ./... && ./demo/run-all.sh`
- **AC:** All 23 demo checks pass, exit code 0

## [ ] Fix D-Bus Secret Service stubs
- **Priority:** low
- **Model:** glm-5.2
- **Files:** internal/platform/linux/keyring_secret_service.go, internal/platform/linux/credential.go
- **AC:** Linux credential store uses actual D-Bus Secret Service protocol instead of stub TODOs

## [ ] Wire functional workflow tests
- **Priority:** low
- **Model:** glm-5.2
- **Files:** test/functional/workflows/cli_workflow_test.go
- **AC:** Tests load OpenAPI spec from mock server and generate commands instead of TODO stubs

## [ ] Add unit test coverage for DSL package
- **Priority:** medium
- **Model:** glm-5.2
- **Files:** pkg/dsl/interpreter_test.go (new)
- **AC:** Parse, Compile, Exec, Call, SetGlobal, GetGlobal all have unit tests

## [ ] Add unit tests for config package
- **Priority:** medium
- **Model:** glm-5.2
- **Files:** pkg/config/ (existing)
- **AC:** parseDuration, parseInt, loader helpers covered
