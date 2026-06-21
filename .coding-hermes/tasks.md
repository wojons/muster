# Coding Tasks — Muster

## [ ] Add unit tests for pkg/generator (AC-331)
- **Priority:** high
- **Model:** deepseek-v4-pro
- **Files:** pkg/generator/generator_test.go (NEW)
- **Scope:** generator.go (1004 lines, 41 functions/types). Core engine — parses OpenAPI specs and generates Cobra commands. Currently 48.6% coverage.
- **Tests needed:** NewGenerator (valid/invalid config), Config.Validate (valid/missing AppName/missing BaseURL), GenerateCommands (petstore spec/has operations/nil spec), buildResourceCommand, buildOperationCommand, buildUsage, buildLongDescription, buildExample, inferResourceName, hasQueryParams, inferShorthand, NamingStrategy constants, OperationStyle constants
- **AC:** `go test ./pkg/generator/... -count=1` passes. 15+ test functions. Use embedded petstore JSON as test spec.
- **Spec:** pkg/generator/generator.go — well-documented package with example usage in godoc.

## [ ] Add unit tests for pkg/config/loader (AC-332)
- **Priority:** medium
- **Model:** deepseek-v4-pro
- **Files:** pkg/config/loader_test.go (NEW)
- **Scope:** loader.go (336 lines, 25 functions/types). Config file loading + env overrides. Currently 33.4% coverage.
- **Tests needed:** NewLoader, Load (from YAML file/missing file/invalid YAML), Load (package-level function), Loader.SetConfigFile, setDefaultPaths, applyEnvOverrides, BindPFlags, BindEnvVar, GetViper, ApplyCLIFlags, ConfigManager (New/Config/GetDefaultProfile), ProfileManager (New)
- **AC:** `go test ./pkg/config/... -count=1 -run "TestLoader|TestConfigManager|TestProfile"` passes. 15+ test functions. Use temp YAML files.
- **Spec:** pkg/config/loader.go

## [ ] Add unit tests for pkg/mcp/openapi_integration (AC-333)
- **Priority:** medium
- **Model:** deepseek-v4-pro
- **Files:** pkg/mcp/openapi_integration_test.go (NEW)
- **Scope:** openapi_integration.go (463 lines, 23 functions/types). Bridges OpenAPI specs to MCP tool registry. Currently 17.7% coverage.
- **Tests needed:** NewIntegration, RegisterSpec (valid/missing file/invalid spec), ListTools, GetTool, CallTool (valid/invalid tool/without base URL), toolNameToMethod, buildInputSchema, buildToolDescription
- **AC:** `go test ./pkg/mcp/... -count=1 -run "TestIntegration|TestOpenAPI"` passes. 12+ test functions. Use petstore spec.
- **Spec:** pkg/mcp/openapi_integration.go

## [ ] Add unit tests for pkg/wasm/middleware (AC-334)
- **Priority:** low
- **Model:** deepseek-v4-pro
- **Files:** pkg/wasm/middleware_test.go (NEW)
- **Scope:** middleware.go (211 lines, 15 functions/types). WASM request/response interceptor with middleware wrapping. Currently 16.4% coverage.
- **Tests needed:** NewInterceptor, NewInterceptorWithLimits, RegisteredPlugins (empty/after register), Close, Interceptor.Middleware (wraps handler/passes through/plugin not found), captureResponseWriter (Write/WriteHeader/Flush), isJSONContentType
- **AC:** `go test ./pkg/wasm/... -count=1 -run "TestInterceptor|TestCapture"` passes. 12+ test functions. Use httptest.
- **Spec:** pkg/wasm/middleware.go

## [ ] Add unit tests for pkg/errors (AC-335)
- **Priority:** low
- **Model:** deepseek-v4-flash
- **Files:** pkg/errors/errors_test.go (existing, extend)
- **Scope:** errors.go. Currently 71.4% coverage — just needs edge cases to reach 80%+.
- **Tests needed:** Errorf (format string/with args/empty format), Code (on nil error), Message (on nil error), Severity (on nil error), Details (on nil error), Unwrap (unwrapped error/nil cause), WithDetail (chain multiple/on nil), WithSeverity (all levels/on nil)
- **AC:** `go test ./pkg/errors/... -count=1` passes. errors.go coverage > 80%. 8+ new test assertions.
- **Spec:** pkg/errors/errors.go (existing test file: pkg/errors/errors_test.go)

## [x] Add unit tests for pkg/auth (AC-330)
- **Completed:** 2026-06-21 — 35 test functions, all pass. token_test.go: 7 tests (IsExpired past/future/grace/boundary/zero/nil/empty). oauth2_ratelimit_test.go: 28 tests (NewOAuthRateLimiter defaults+rates, AllowTokenRequest under/over/locked/not-locked-under/exact-threshold/refill, RecordFailedAttempt single/multi/new, RecordSuccessfulAttempt clears/nonexistent, AllowDeviceCodePoll under/over, AllowCallbackRequest under/over, GetFailedAttempts not-tracked+after-multi, ResetFailedAttempts clears/nonexistent, Configure updates/zero-max/zero-lockout, Concurrent failed-attempts/record-reset/allow-record). No spawn — direct write.

## [x] Add unit tests for internal/telemetry (AC-329)
- **Completed:** 2026-06-20 — 21 test functions, all pass. Covers NewTracer, Start (basic/with SpanKind+Attributes/NoOptions), End (valid+nil), SetAttribute (string/int/int64/float64/bool/unknown+nil), SetAttributeInt (valid+nil), SetAttributeBool (valid+nil), RecordError (valid+nil error+nil span), SetOK (valid+nil), WithSpanKind, WithAttributes, ContextPropagation. Uses tracetest.SpanRecorder for in-memory verification.
- **Priority:** medium

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

## [x] Add unit tests for pkg/retry (AC-320)
- **Completed:** 2026-06-20 — 22 tests, all pass. Covers NewRetrier (valid/zero/negative/empty backoff/zero delays), DefaultRetryConfig, Execute (success/transient/exhausted/context cancel during+pre+deadline), calculateDelay (constant/linear/exponential/capped/jitter bounds/no jitter/default), RetryExhaustedError (Error/single attempt).
- **Priority:** medium
- **Model:** deepseek-v4-pro

## [x] Add unit tests for pkg/response (AC-321)
- **Completed:** 2026-06-20 — 20 tests, all pass. Covers NewHandler (defaults/treatNon200), HandleResponse (nil/200 JSON/200 non-JSON/empty/404/500/400/TreatNon200Disabled/401/read error/large JSON), ProcessResponse (success/error/raw), HTTPError.Error (body/empty body), Config, ProcessingOptions.
- **Priority:** low
- **Model:** deepseek-v4-flash

## [x] Add unit tests for pkg/openapi (AC-322)
- **Priority:** high
- **Completed:** 2026-06-20 — 28 test functions + 10 subtests, 38 total. All pass. Covers NewParser, Parse (valid/invalid/nil/missing info), ExtractOperations (petstore/empty/nil), all parameter types, body properties, SchemaToJSON, typeToJSONSchemaType (10 cases), resolveServerURL, extractPropertiesFromSchemaRef, jsonMarshalNoEscape.
- **Model:** deepseek-v4-pro
- **Files:** pkg/openapi/parser_test.go (NEW)
- **Tests needed:** NewParser, DefaultParseOptions, Parse (valid OAS 3.0, invalid JSON, nil data), ExtractOperations (petstore, empty spec), extractParameters (path/query/header), extractRequestBodyProperties (JSON body, multipart), SchemaToJSON (valid props, empty props), typeToJSONSchemaType
- **AC:** `go test ./pkg/openapi/... -count=1` passes. 15+ test functions. Cover all exported symbols.
- **Spec:** pkg/openapi/parser.go (446 lines, 3 exported functions + 2 exported methods + internal helpers)

## [x] Add unit tests for pkg/request (AC-323)
- **Completed:** 2026-06-20 — 57 tests, all pass. 788 lines. Covers all exported methods + edge cases.

## [x] Add unit tests for pkg/repository (AC-324)
- **Completed:** 2026-06-20 — 22 tests, all pass. Covers NewSQLiteStore (memory/tempfile/invalid), Close (normal/double), DB, Exec (create/insert+delete/bad SQL/cancelled context/deadline/multiple statements), Query (populated/empty/bad SQL), QueryRow (found/no rows), concurrent access, ErrNotFound.Error (ID/zero), Store interface satisfaction, WAL mode.
- **Priority:** medium
- **Model:** deepseek-v4-pro

## [x] Add unit tests for pkg/cmdstore (AC-325)
- **Completed:** 2026-06-20 — 28 tests, all pass. Covers NewCommandStore (3 paths), Commands (5 scenarios), HasData (5 scenarios), Load (4 scenarios), Save (6 scenarios), LoadCommands (2 scenarios), ReconstructCommand, CommandMetadata, FlagMetadata, FlagKind constants, StoredData, concurrent access, save overwrite, commands-cached.
- **Priority:** medium
- **Model:** deepseek-v4-pro
- **Files:** pkg/cmdstore/store_test.go (NEW)
- **Scope:** store.go only (204 lines). Loader.go requires cobra mocking — separate task.
- **Tests needed:** NewCommandStore (no path/with path), Commands (empty disk/loaded/load error), HasData (empty/loaded/disk file), Load (from meta/from disk/from commands/empty error), Save (with data/nil data/no disk path), LoadCommands, ReconstructCommand (returns nil,nil)
- **AC:** `go test ./pkg/cmdstore/... -count=1` passes. 10+ test functions. Use temp files for disk persistence.

## [x] Add unit tests for pkg/auth/ratelimit (AC-326)
- **Completed:** 2026-06-20 — 38 tests, all pass. Covers TokenBucket (New/defaults/zero cap/zero fill, Allow burst+refill, AllowN valid/zero/negative/exceeds/non-pos, Tokens initial/after/refill, SetRate valid/zero/negative, ConcurrentAccess/AllowN/TokensAndAllow/SetRate, ExhaustionAndRefill) + TokenBucketLimiter (New valid/zero rate/zero burst, Allow under/nil-limiter/nil-bucket/refill, Wait under/nil-limiter/nil-bucket/eventually, Rate valid/nil-limiter/nil-bucket/zero).
- **Priority:** medium
- **Model:** deepseek-v4-pro (direct write, no spawn)

## [x] Add unit tests for internal/monitoring (AC-328)
- **Completed:** 2026-06-20 — 18 test functions, all pass. Covers NewMetricRecorder (validates tenantID/registry/logger), RecordCommandExecution, RecordCommandError, RecordMCPRequest, RecordToolExecution, RecordProxyRequest, RecordCacheHit, RecordCacheMiss, IncDecMCPConnections, IncDecDaemonSocketConnections, ConcurrentAccess (20 goroutines), DifferentTenantIDs, NewMetricsServer, MetricsServer StartAndShutdown (health+metrics endpoints), HealthEndpoints (all 4: health/liveness/readiness/startup), ShutdownNilServer, NamespaceConstants, GlobalMetricsRegistered.
## [x] Add unit tests for pkg/repository/sql (AC-327)
- **Completed:** 2026-06-20 — 44 tests, all pass. Covers all 5 repository types with in-memory SQLite. Base (NewRepository, NewSQLRepository, GetQuerySet valid+unsupported, DB). Inventory (New, Create+GetByID, NotFound, Update, Delete, List, Pagination, Count, Search). RateLimit (New, Create+GetByID, GetByKey found+notfound, Update, Delete, DeleteByKey, Cleanup, Increment). Audit (New, Create+GetByID, GetByEntity, List, Count, Cleanup). Fingerprint (New, Create+GetByID, GetBySpec, GetByHash, Update, Delete, DeleteBySpec, Exists true+false). Alias (New, Create+GetByID, GetByName, GetByInventory, GetByModule, Update, Delete, DeleteByInventory).
- **Priority:** medium
- **Model:** deepseek-v4-pro (direct write, no spawn)
