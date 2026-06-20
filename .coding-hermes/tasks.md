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

## [x] Fix D-Bus Secret Service stubs
- **Completed:** 2026-06-20 — Full D-Bus Secret Service protocol implementation (godbus/dbus/v5, ~290 lines).

## [x] Wire functional workflow tests
- **Completed:** 2026-06-20 — Replaced TODO stubs with real spec parsing + command generation.

## [x] Add unit tests for DSL package
- **Completed:** 2026-06-19 — 20 tests.

## [x] Implement auth.Token struct (AC-260)
- **Completed:** 2026-06-19 — Token struct, TokenManager, AuthorizationCodeFlow.

## [x] Implement wasm network policy API (AC-270)
- **Completed:** 2026-06-19 — NewCapabilitySet(), Grant(), Revoke(), SetNetworkPolicy().

## [x] Add version --output json flag (AC-280)
- **Completed:** 2026-06-19 — version --output flag added.

## [ ] Add unit tests for pkg/retry (AC-320)
- **Priority:** medium
- **Model:** deepseek-v4-pro
- **Files:** pkg/retry/retry_test.go (NEW)
- **Tests needed:** NewRetrier (valid config, zero attempts fallback, empty backoff), DefaultRetryConfig (sane defaults), Execute (success first try, retry on transient, exhausted error, context cancel, jitter variance), calculateDelay (constant/linear/exponential), RetryExhaustedError.Error()
- **AC:** `go test ./pkg/retry/... -count=1` passes. 12+ test functions. Cover backoff + jitter + context cancellation.
- **Spec:** pkg/retry/retry.go (137 lines) — Retrier, RetryConfig, RetryExhaustedError

## [ ] Add unit tests for pkg/response (AC-321)
- **Priority:** low
- **Model:** deepseek-v4-flash
- **Files:** pkg/response/handler_test.go (NEW)
- **Tests needed:** NewHandler, HandleResponse (nil response, 200 JSON, 200 non-JSON, 404 error, 500 error), ProcessResponse, HTTPError.Error(), Config defaults
- **AC:** `go test ./pkg/response/... -count=1` passes. 8+ test functions.
- **Spec:** pkg/response/handler.go (88 lines)

## [x] Add unit tests for pkg/openapi (AC-322)
- **Priority:** high
- **Completed:** 2026-06-20 — 28 test functions + 10 subtests, 38 total. All pass. Covers NewParser, Parse (valid/invalid/nil/missing info), ExtractOperations (petstore/empty/nil), all parameter types, body properties, SchemaToJSON, typeToJSONSchemaType (10 cases), resolveServerURL, extractPropertiesFromSchemaRef, jsonMarshalNoEscape.
- **Model:** deepseek-v4-pro
- **Files:** pkg/openapi/parser_test.go (NEW)
- **Tests needed:** NewParser, DefaultParseOptions, Parse (valid OAS 3.0, invalid JSON, nil data), ExtractOperations (petstore, empty spec), extractParameters (path/query/header), extractRequestBodyProperties (JSON body, multipart), SchemaToJSON (valid props, empty props), typeToJSONSchemaType
- **AC:** `go test ./pkg/openapi/... -count=1` passes. 15+ test functions. Cover all exported symbols.
- **Spec:** pkg/openapi/parser.go (446 lines, 3 exported functions + 2 exported methods + internal helpers)

## [ ] Add unit tests for pkg/request (AC-323)
- **Priority:** high
- **Model:** deepseek-v4-pro
- **Files:** pkg/request/builder_test.go (NEW)
- **Tests needed:** NewBuilder (with/without args), Build (method/path/headers/URL), SetHeader, WithPathParam, WithQueryParam, WithBody, JSONBody, WithFileUpload, WithMultipartField, BuildRequest, BuildGet, BuildPost, BuildPut, BuildDelete, ExecuteWithResult (needs test HTTP server), WithContentType, buildMultipartBody
- **AC:** `go test ./pkg/request/... -count=1` passes. 18+ test functions. Use httptest.NewServer for Execute tests.
- **Spec:** pkg/request/builder.go (328 lines, 22 methods on Builder)

## [ ] Add unit tests for pkg/repository (AC-324)
- **Priority:** medium
- **Model:** deepseek-v4-pro
- **Files:** pkg/repository/sqlite_test.go (NEW)
- **Tests needed:** NewSQLiteManager, CRUD operations (insert/query/delete), transaction rollback, concurrent access, error handling (missing DB, bad SQL)
- **AC:** `go test ./pkg/repository/... -count=1` passes. 10+ test functions. Use :memory: or tempfile SQLite.
- **Spec:** pkg/repository/sqlite.go (145 lines)
