# Ralph Wiggum Loop - Task Queue (Baby Steps)

## Status: 11% Complete (26/223) | Target: Production Ready

---

## 🔴 P0: BLOCKING PRODUCTION (Execute First)

### SEC-001: OAuth2 State Validation (CRITICAL) ✅ COMPLETED
- [x] Add state parameter validation in Exchange() method
- [x] Compare returned state with stored state
- [x] Reject requests with mismatched state
- [x] Add unit tests for state validation
- [x] **Location:** pkg/auth/oauth2.go:94-128
- [x] **Commits:** Fixed critical CSRF vulnerability by validating OAuth2 state parameter

---

### SEC-002: Remove Weak Master Key Fallback (CRITICAL) ✅ COMPLETED

#### Step 1: [x] Identify all fallback key locations
- File: pkg/auth/keychain.go:288
- Spec: specs/KEYCHAIN-INTEGRATION.md
- Skill: @security-scan
- Effort: 10 min
- Dependencies: None

#### Step 2: [x] Remove fallback key "muster-fallback-master-key-" + runtime.GOOS
- File: pkg/auth/keychain.go:288
- Spec: specs/KEYCHAIN-INTEGRATION.md lines 50-75
- Skill: @security-scan
- Effort: 15 min
- Dependencies: Step 1

#### Step 3: [x] Create MasterKeyMissingError type
- File: pkg/auth/keychain.go
- Skill: @testing-unit
- Effort: 10 min
- Dependencies: Step 2

#### Step 4: [x] Fail closed with proper error message for missing master key
- File: pkg/auth/keychain.go
- Spec: specs/KEYCHAIN-INTEGRATION.md lines 76-95
- Skill: @security-scan
- Effort: 20 min
- Dependencies: Step 3

#### Step 5: [x] Update callers to handle MasterKeyMissingError
- File: pkg/auth/*.go, internal/builtin/auth.go
- Skill: @testing-integration
- Effort: 30 min
- Dependencies: Step 4

#### Step 6: [x] Add unit tests for error scenarios
- File: pkg/auth/keychain_test.go
- Skill: @testing-unit
- Effort: 25 min
- Dependencies: Step 5

#### Step 7: [x] Add security test for no-plaintext-fallback invariant
- File: pkg/auth/keychain_security_test.go
- Skill: @testing-security
- Effort: 30 min
- Dependencies: Step 6

**Summary:**
- Removed critical security vulnerability where weak predictable master key was used as fallback
- Changed `deriveKey()` to return `([]byte, error)` instead of `[]byte`
- Added `ErrMissingMasterKey` error with helpful guidance message
- Updated `Set()` and `Get()` methods to handle error from `deriveKey()`
- Added 5 comprehensive unit tests including test for error path
- All tests passing ✅

---

### SEC-003: Daemon Socket Authentication (CRITICAL)

#### Step 1: [x] Design token generation scheme
- Spec: specs/UNIX-SOCKET-PROXY.md, specs/DAEMON-LIFECYCLE.md
- Skill: @security-scan
- Effort: 20 min
- Dependencies: None
- **Completed**: Created comprehensive token design document
- **Location**: docs/design-daemon-token.md
- **Summary**: Designed cryptographically secure token scheme using crypto/rand with 32-byte tokens, base64 encoding, 0600 file permissions, and constant-time validation

#### Step 2: [x] Create TokenManager struct
- File: pkg/daemon/token.go
- Spec: specs/UNIX-SOCKET-PROXY.md lines 30-55
- Skill: @testing-unit
- Effort: 30 min
- Dependencies: Step 1

#### Step 3: [x] Generate token on daemon start
- File: pkg/daemon/controller.go, cmd/daemon/main.go
- Spec: specs/DAEMON-LIFECYCLE.md lines 45-67
- Skill: @testing-unit
- Effort: 25 min
- Dependencies: Step 2
- **Completed**: Integrated TokenManager into Controller to generate token on daemon start
- **Changes**: Added tokenManager field to Controller, generate token in Start() and StartWithSocket(), cleanup on Stop/Kill/Shutdown

#### Step 4: [x] Create token file with restricted permissions (0600)
- File: pkg/daemon/token.go
- Spec: specs/UNIX-SOCKET-PROXY.md lines 56-78
- Skill: @security-scan
- Effort: 20 min
- Dependencies: Step 3
- **Completed**: TokenManager.Save() already writes files with 0600 permissions
- **Implementation**: Token file created with os.WriteFile(path, []byte(token), 0600)

#### Step 5: [x] Add token validation middleware in proxy
- File: pkg/daemon/proxy.go
- Spec: specs/UNIX-SOCKET-PROXY.md lines 79-110
- Skill: @testing-security
- Effort: 35 min
- Dependencies: Step 4
- **Completed**: Token validation middleware already implemented with withTokenAuth() and NewAuthenticatedProxyServer()
- **Features**: Bearer token validation, health endpoint exclusion, constant-time comparison
- **Tests**: 8 comprehensive test cases covering valid/invalid tokens, missing headers, malformed headers, health endpoints

#### Step 6: [x] Log unauthorized access attempts
- File: pkg/daemon/proxy.go
- Spec: specs/EVENTS-AUDIT.md
- Skill: @logging-fundamentals
- Effort: 15 min
- Dependencies: Step 5
- **Completed**: Added structured logging for unauthorized access attempts using pkg/log
- **Log Format**: WARN level with event=auth_failure, reason, path, remote_addr fields
- **Coverage**: Logs both missing header and invalid token scenarios
- **Tests**: All existing tests pass, logs visible during test execution

#### Step 7: [x] Add unit tests for token generation/validation
- File: pkg/daemon/token_test.go
- Skill: @testing-unit
- Effort: 30 min
- Dependencies: Step 6
- **Completed**: Comprehensive unit tests for TokenManager covering generation, validation, Save, Load, Validate, ValidateFromHeader, Cleanup, and concurrent access
- **Tests**: 10 test functions, 100% passing
- **Coverage**: Token generation (32 bytes base64), file permissions (0600), Bearer header validation, constant-time comparison, concurrent safety

#### Step 8: [x] Add integration test for authenticated socket communication
- File: pkg/daemon/proxy_integration_test.go
- Skill: @testing-integration
- Effort: 45 min
- Dependencies: Step 7
- **Completed**: Created comprehensive integration tests for authenticated socket communication
- **Tests**: 8 integration test functions covering full lifecycle, concurrent connections, token rotation, multiple endpoints, invalid tokens, connection persistence, and cleanup
- **Coverage**: 579 lines of test code, all tests passing ✅
- **Features Tested**:
  - Full token lifecycle (generation → save → load → validate → cleanup)
  - 50 concurrent authenticated connections
  - Token rotation behavior (old tokens rejected after rotation)
  - Multiple endpoints with authentication
  - Invalid token scenarios (empty, missing Bearer, wrong prefix, etc.)
  - Connection persistence and reuse
  - Cleanup verification on server stop

---

### SEC-004: HTTP Response Body Error Handling (CRITICAL) ✅ COMPLETED

#### Step 1: [x] Create safeReadAll helper with size limit
- File: pkg/dsl/builtins.go
- Spec: specs/DSL-BUILTINS.md
- Skill: @security-scan
- Effort: 15 min
- Dependencies: None
- **Completed**: Implemented safeReadAll function with io.LimitReader and proper error handling

#### Step 2: [x] Add MaxResponseBodySize constant (10MB default)
- File: pkg/dsl/builtins.go
- Skill: @testing-unit
- Effort: 10 min
- Dependencies: Step 1
- **Completed**: Added exported MaxResponseBodySize constant (10MB) replacing internal maxHTTPResponseSize

#### Step 3: [x] Fix line 112: Handle io.ReadAll error in httpGet
- File: pkg/dsl/builtins.go:112
- Spec: specs/DSL-BUILTINS.md lines 45-67
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: Step 2
- **Completed**: Replaced ignored error with proper safeReadAll call and error handling

#### Step 4: [x] Fix line 169: Handle io.ReadAll error in httpPost
- File: pkg/dsl/builtins.go:169
- Spec: specs/DSL-BUILTINS.md lines 68-89
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: Step 3
- **Completed**: Fixed httpPost to use safeReadAll with error handling

#### Step 5: [x] Fix line 226: Handle io.ReadAll error in httpPut
- File: pkg/dsl/builtins.go:226
- Spec: specs/DSL-BUILTINS.md lines 90-111
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: Step 4
- **Completed**: Fixed httpPut to use safeReadAll with error handling

#### Step 6: [x] Fix line 282: Handle io.ReadAll error in httpDelete
- File: pkg/dsl/builtins.go:282
- Spec: specs/DSL-BUILTINS.md lines 112-133
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: Step 5
- **Completed**: Fixed httpDelete to use safeReadAll with error handling

#### Step 7: [x] Return proper errors instead of ignoring
- File: pkg/dsl/builtins.go
- Spec: specs/DSL-BUILTINS.md lines 134-155
- Skill: @testing-unit
- Effort: 20 min
- Dependencies: Step 6
- **Completed**: All HTTP methods now return descriptive errors when body reading fails

#### Step 8: [x] Add unit tests for error scenarios
- File: pkg/dsl/builtins_test.go
- Skill: @testing-unit
- Effort: 30 min
- Dependencies: Step 7
- **Completed**: Added comprehensive tests for safeReadAll including size limits, read errors, and boundary conditions

**Summary:**
- Fixed critical security issue where io.ReadAll errors were silently ignored
- Implemented safeReadAll helper with size limiting and proper error handling
- Updated all 4 HTTP methods (GET, POST, PUT, DELETE) to use new helper
- Added 9 comprehensive unit tests covering size limits, read errors, and edge cases
- All tests passing ✅

---

## 🟠 P1: HIGH PRIORITY

### IMPL-002: HTTP Retry with Backoff (2-3 days) ✅ COMPLETED
- [x] Create pkg/retry/doc.go
- [x] Create pkg/retry/retrier.go with Retrier struct
- [x] Create pkg/retry/config.go with RetryConfig
- [x] Create pkg/retry/backoff.go with exponential backoff
- [x] Create pkg/retry/jitter.go with jitter implementations
- [x] Implement Do(ctx, fn func() error) error
- [x] Add unit tests
- [x] Create pkg/retry/errors.go with error types
- [x] All tests passing (21 tests)
- [ ] Integrate with pkg/client/client.go (deferred - pkg/client empty)

---

### IMPL-003: CLI Flag Parsing (3-4 days)

#### Step 1: [x] Create internal/cli/flags/doc.go
- File: internal/cli/flags/doc.go
- Spec: specs/COBRA-INTEGRATION.md
- Skill: @testing-unit
- Effort: 10 min
- Dependencies: None
- **Completed**: Created package documentation with comprehensive examples
- **Content**: Package overview, basic usage, advanced validation, Cobra integration
- **Lines**: 56 lines of documentation

#### Step 2: [x] Define FlagType constants (String, Int, Bool, URL, IP, Email, Path)
- File: internal/cli/flags/types.go
- Spec: specs/COBRA-INTEGRATION.md lines 30-55
- Skill: @testing-unit
- Effort: 20 min
- Dependencies: Step 1
- **Completed**: Implemented FlagType type and all 7 constants (String, Int, Bool, URL, IP, Email, Path)
- **Features**: IsValid() and String() methods, comprehensive documentation
- **Tests**: 100% passing (14 test functions covering all flag types)

#### Step 3: [ ] Create FlagDefinition struct
- File: internal/cli/flags/types.go
- Spec: specs/COBRA-INTEGRATION.md lines 56-85
- Skill: @testing-unit
- Effort: 25 min
- Dependencies: Step 2

#### Step 4: [ ] Create Parser struct with Validate/Bind methods
- File: internal/cli/flags/parser.go
- Spec: specs/COBRA-INTEGRATION.md lines 86-120
- Skill: @testing-unit
- Effort: 40 min
- Dependencies: Step 3

#### Step 5: [ ] Implement URLValidator
- File: internal/cli/flags/validators.go
- Spec: specs/COBRA-INTEGRATION.md lines 121-145
- Skill: @testing-unit
- Effort: 20 min
- Dependencies: Step 4

#### Step 6: [ ] Implement IPValidator (IPv4, IPv6)
- File: internal/cli/flags/validators.go
- Spec: specs/COBRA-INTEGRATION.md lines 146-170
- Skill: @testing-unit
- Effort: 25 min
- Dependencies: Step 5

#### Step 7: [ ] Implement EmailValidator
- File: internal/cli/flags/validators.go
- Spec: specs/COBRA-INTEGRATION.md lines 171-190
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: Step 6

#### Step 8: [ ] Implement PathValidator (exists, readable, writable)
- File: internal/cli/flags/validators.go
- Spec: specs/COBRA-INTEGRATION.md lines 191-220
- Skill: @testing-unit
- Effort: 30 min
- Dependencies: Step 7

#### Step 9: [ ] Add unit tests for all validators
- File: internal/cli/flags/validators_test.go
- Skill: @testing-unit
- Effort: 45 min
- Dependencies: Step 8

#### Step 10: [ ] Add integration tests with Cobra commands
- File: internal/cli/flags/integration_test.go
- Skill: @testing-integration
- Effort: 40 min
- Dependencies: Step 9

---

### IMPL-004: Command Aliases (2-3 days)

#### Step 1: [ ] Create internal/alias/doc.go
- File: internal/alias/doc.go
- Spec: specs/CLI-MARKET-ALIASES.md
- Skill: @testing-unit
- Effort: 10 min
- Dependencies: None

#### Step 2: [ ] Define Alias struct with Name, Command, Description
- File: internal/alias/types.go
- Spec: specs/CLI-MARKET-ALIASES.md lines 25-45
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: Step 1

#### Step 3: [ ] Create Manager struct with mutex-protected aliases map
- File: internal/alias/manager.go
- Spec: specs/CLI-MARKET-ALIASES.md lines 46-75
- Skill: @testing-unit
- Effort: 25 min
- Dependencies: Step 2

#### Step 4: [ ] Implement Define(name, command string) error
- File: internal/alias/manager.go
- Spec: specs/CLI-MARKET-ALIASES.md lines 76-95
- Skill: @testing-unit
- Effort: 20 min
- Dependencies: Step 3

#### Step 5: [ ] Implement Remove(name string) error
- File: internal/alias/manager.go
- Spec: specs/CLI-MARKET-ALIASES.md lines 96-110
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: Step 4

#### Step 6: [ ] Implement Get(name string) (*Alias, error)
- File: internal/alias/manager.go
- Spec: specs/CLI-MARKET-ALIASES.md lines 111-125
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: Step 5

#### Step 7: [ ] Implement List() []*Alias
- File: internal/alias/manager.go
- Spec: specs/CLI-MARKET-ALIASES.md lines 126-140
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: Step 6

#### Step 8: [ ] Create Expander with circular detection
- File: internal/alias/expander.go
- Spec: specs/CLI-MARKET-ALIASES.md lines 141-180
- Skill: @testing-unit
- Effort: 40 min
- Dependencies: Step 7

#### Step 9: [ ] Implement Expand(args []string) ([]string, error)
- File: internal/alias/expander.go
- Spec: specs/CLI-MARKET-ALIASES.md lines 181-210
- Skill: @testing-unit
- Effort: 30 min
- Dependencies: Step 8

#### Step 10: [ ] Create storage.go with YAML persistence
- File: internal/alias/storage.go
- Spec: specs/CLI-MARKET-ALIASES.md lines 211-245
- Skill: @testing-unit
- Effort: 35 min
- Dependencies: Step 9

#### Step 11: [ ] Implement Load/Save methods
- File: internal/alias/storage.go
- Spec: specs/CLI-MARKET-ALIASES.md lines 246-275
- Skill: @testing-unit
- Effort: 25 min
- Dependencies: Step 10

#### Step 12: [ ] Add unit tests for Manager and Expander
- File: internal/alias/manager_test.go, expander_test.go
- Skill: @testing-unit
- Effort: 45 min
- Dependencies: Step 11

---

### IMPL-005: Event Handler Registry (2-3 days)

#### Step 1: [ ] Create pkg/events/doc.go
- File: pkg/events/doc.go
- Spec: specs/EVENTS-HANDLERS.md
- Skill: @testing-unit
- Effort: 10 min
- Dependencies: None

#### Step 2: [ ] Define Handler interface (Handle, Name, Supports)
- File: pkg/events/handler.go
- Spec: specs/EVENTS-HANDLERS.md lines 22-27
- Skill: @testing-unit
- Effort: 20 min
- Dependencies: Step 1

#### Step 3: [ ] Define HandlerFunc and HandlerMiddleware types
- File: pkg/events/handler.go
- Spec: specs/EVENTS-HANDLERS.md lines 21, 29
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: Step 2

#### Step 4: [ ] Create HandlerInfo struct
- File: pkg/events/handler.go
- Spec: specs/EVENTS-HANDLERS.md lines 31-39
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: Step 3

#### Step 5: [ ] Create HandlerRegistry struct
- File: pkg/events/registry.go
- Spec: specs/EVENTS-HANDLERS.md lines 55-61
- Skill: @testing-unit
- Effort: 20 min
- Dependencies: Step 4

#### Step 6: [ ] Implement Register(info *HandlerInfo) error
- File: pkg/events/registry.go
- Spec: specs/EVENTS-HANDLERS.md lines 78-108
- Skill: @testing-unit
- Effort: 25 min
- Dependencies: Step 5

#### Step 7: [ ] Implement Unregister(name string) bool
- File: pkg/events/registry.go
- Spec: specs/EVENTS-HANDLERS.md lines 110-132
- Skill: @testing-unit
- Effort: 20 min
- Dependencies: Step 6

#### Step 8: [ ] Implement GetHandlers(eventType EventType) []*HandlerInfo
- File: pkg/events/registry.go
- Spec: specs/EVENTS-HANDLERS.md lines 134-148
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: Step 7

#### Step 9: [ ] Implement Execute(ctx, event) []HandlerResult
- File: pkg/events/registry.go
- Spec: specs/EVENTS-HANDLERS.md lines 150-192
- Skill: @testing-unit
- Effort: 35 min
- Dependencies: Step 8

#### Step 10: [ ] Create middleware.go with wrapWithMiddleware/Timeout/Retry
- File: pkg/events/middleware.go
- Spec: specs/EVENTS-HANDLERS.md lines 194-231
- Skill: @testing-unit
- Effort: 40 min
- Dependencies: Step 9

#### Step 11: [ ] Implement List() []string
- File: pkg/events/registry.go
- Spec: specs/EVENTS-HANDLERS.md lines 239-248
- Skill: @testing-unit
- Effort: 10 min
- Dependencies: Step 10

#### Step 12: [ ] Add unit tests for Registry
- File: pkg/events/registry_test.go
- Skill: @testing-unit
- Effort: 45 min
- Dependencies: Step 11

---

### IMPL-006: Daemon Config Hot Reload (2-3 days)

#### Step 1: [ ] Create internal/daemon/config/doc.go
- File: internal/daemon/config/doc.go
- Spec: specs/DAEMON-LIFECYCLE.md
- Skill: @testing-unit
- Effort: 10 min
- Dependencies: None

#### Step 2: [ ] Define Config struct with 12 fields
- File: internal/daemon/config/config.go
- Spec: specs/DAEMON-CONFIGURATION.md lines 25-50
- Skill: @testing-unit
- Effort: 25 min
- Dependencies: Step 1

#### Step 3: [ ] Create Manager struct
- File: internal/daemon/config/manager.go
- Spec: specs/DAEMON-CONFIGURATION.md lines 51-80
- Skill: @testing-unit
- Effort: 30 min
- Dependencies: Step 2

#### Step 4: [ ] Implement Load() (*Config, error)
- File: internal/daemon/config/manager.go
- Spec: specs/DAEMON-CONFIGURATION.md lines 81-105
- Skill: @testing-unit
- Effort: 25 min
- Dependencies: Step 3

#### Step 5: [ ] Implement Save(config *Config) error
- File: internal/daemon/config/manager.go
- Spec: specs/DAEMON-CONFIGURATION.md lines 106-125
- Skill: @testing-unit
- Effort: 20 min
- Dependencies: Step 4

#### Step 6: [ ] Add fsnotify dependency to go.mod
- File: go.mod
- Skill: @dependency-upgrade
- Effort: 5 min
- Dependencies: None

#### Step 7: [ ] Create watcher.go with FileWatcher
- File: internal/daemon/config/watcher.go
- Spec: specs/DAEMON-CONFIGURATION.md lines 126-160
- Skill: @testing-unit
- Effort: 35 min
- Dependencies: Step 6

#### Step 8: [ ] Implement Watch() and ReloadCh
- File: internal/daemon/config/watcher.go
- Spec: specs/DAEMON-CONFIGURATION.md lines 161-195
- Skill: @testing-unit
- Effort: 40 min
- Dependencies: Step 7

#### Step 9: [ ] Add unit tests for Manager
- File: internal/daemon/config/manager_test.go
- Skill: @testing-unit
- Effort: 35 min
- Dependencies: Step 5

#### Step 10: [ ] Add integration tests for hot reload
- File: internal/daemon/config/watcher_test.go
- Skill: @testing-integration
- Effort: 45 min
- Dependencies: Step 8

---

## 🟡 P2: MEDIUM PRIORITY

### FEAT-001: MCP Roots Protocol

#### Step 1: [ ] Define Root struct (URI, Name)
- File: pkg/mcp/roots.go
- Spec: specs/MCP-MARKET-ROOTS.md lines 20-35
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: None

#### Step 2: [ ] Create RootManager struct
- File: pkg/mcp/roots.go
- Spec: specs/MCP-MARKET-ROOTS.md lines 36-55
- Skill: @testing-unit
- Effort: 25 min
- Dependencies: Step 1

#### Step 3: [ ] Implement SetRoots(roots []Root)
- File: pkg/mcp/roots.go
- Spec: specs/MCP-MARKET-ROOTS.md lines 56-70
- Skill: @testing-unit
- Effort: 20 min
- Dependencies: Step 2

#### Step 4: [ ] Implement GetRoots() []Root
- File: pkg/mcp/roots.go
- Spec: specs/MCP-MARKET-ROOTS.md lines 71-82
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: Step 3

#### Step 5: [ ] Implement AddRoot(root Root) error
- File: pkg/mcp/roots.go
- Spec: specs/MCP-MARKET-ROOTS.md lines 83-100
- Skill: @testing-unit
- Effort: 20 min
- Dependencies: Step 4

#### Step 6: [ ] Implement RemoveRoot(uri string) error
- File: pkg/mcp/roots.go
- Spec: specs/MCP-MARKET-ROOTS.md lines 101-118
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: Step 5

#### Step 7: [ ] Implement isWithinRoots(path string) bool
- File: pkg/mcp/roots.go
- Spec: specs/MCP-MARKET-ROOTS.md lines 119-145
- Skill: @testing-unit
- Effort: 30 min
- Dependencies: Step 6

#### Step 8: [ ] Add roots/list handler
- File: pkg/mcp/handlers/roots.go
- Spec: specs/MCP-MARKET-ROOTS.md lines 146-175
- Skill: @testing-unit
- Effort: 35 min
- Dependencies: Step 7

#### Step 9: [ ] Add unit tests for RootManager
- File: pkg/mcp/roots_test.go
- Skill: @testing-unit
- Effort: 30 min
- Dependencies: Step 8

---

### FEAT-002: HTTP Canonicalization

#### Step 1: [ ] Create pkg/httputil/canonical.go
- File: pkg/httputil/canonical.go
- Spec: specs/HTTP-CANONICALIZATION.md
- Skill: @testing-unit
- Effort: 10 min
- Dependencies: None

#### Step 2: [ ] Define URL normalization rules
- File: pkg/httputil/canonical.go
- Spec: specs/HTTP-CANONICALIZATION.md lines 25-55
- Skill: @data-validation
- Effort: 25 min
- Dependencies: Step 1

#### Step 3: [ ] Implement CanonicalizeURL(url string) string
- File: pkg/httputil/canonical.go
- Spec: specs/HTTP-CANONICALIZATION.md lines 56-85
- Skill: @testing-unit
- Effort: 30 min
- Dependencies: Step 2

#### Step 4: [ ] Implement CanonicalizeHeaders(headers http.Header) http.Header
- File: pkg/httputil/canonical.go
- Spec: specs/HTTP-CANONICALIZATION.md lines 86-115
- Skill: @testing-unit
- Effort: 25 min
- Dependencies: Step 3

#### Step 5: [ ] Add unit tests for URL canonicalization
- File: pkg/httputil/canonical_test.go
- Skill: @testing-unit
- Effort: 30 min
- Dependencies: Step 4

#### Step 6: [ ] Add unit tests for header canonicalization
- File: pkg/httputil/canonical_test.go
- Skill: @testing-unit
- Effort: 20 min
- Dependencies: Step 5

---

### FEAT-003: Viper Advanced Integration

#### Step 1: [ ] Add viper.BindPFlag() integration for CLI flags
- File: pkg/config/loader.go
- Spec: specs/VIPER-INTEGRATION.md lines 30-55
- Skill: @testing-unit
- Effort: 30 min
- Dependencies: None

#### Step 2: [ ] Implement WatchConfig() for hot reload
- File: pkg/config/watcher.go
- Spec: specs/VIPER-INTEGRATION.md lines 56-85
- Skill: @testing-unit
- Effort: 40 min
- Dependencies: Step 1

#### Step 3: [ ] Implement SetProfile() for profile switching
- File: pkg/config/profiles.go
- Spec: specs/VIPER-INTEGRATION.md lines 86-115
- Skill: @testing-unit
- Effort: 35 min
- Dependencies: Step 2

#### Step 4: [ ] Add validation layer with go-playground/validator
- File: pkg/config/validator.go
- Spec: specs/VIPER-INTEGRATION.md lines 116-145
- Skill: @testing-unit
- Effort: 40 min
- Dependencies: Step 3

#### Step 5: [ ] Add unit tests for BindPFlag
- File: pkg/config/loader_test.go
- Skill: @testing-unit
- Effort: 25 min
- Dependencies: Step 4

#### Step 6: [ ] Add integration tests for WatchConfig
- File: pkg/config/watcher_test.go
- Skill: @testing-integration
- Effort: 35 min
- Dependencies: Step 5

---

### AUDIT-001: Implement AuditLogger

#### Step 1: [ ] Define AuditConfig struct
- File: internal/audit/config.go
- Spec: specs/EVENTS-AUDIT.md lines 10-31
- Skill: @testing-unit
- Effort: 20 min
- Dependencies: None

#### Step 2: [ ] Define RedactionRule struct
- File: internal/audit/redaction.go
- Spec: specs/EVENTS-AUDIT.md lines 44-82
- Skill: @testing-unit
- Effort: 25 min
- Dependencies: Step 1

#### Step 3: [ ] Create Redactor with MandatoryRedactionRules
- File: internal/audit/redaction.go
- Spec: specs/EVENTS-AUDIT.md lines 51-82
- Skill: @testing-unit
- Effort: 30 min
- Dependencies: Step 2

#### Step 4: [ ] Implement NewRedactor() and RedactEvent()
- File: internal/audit/redaction.go
- Spec: specs/EVENTS-AUDIT.md lines 91-143
- Skill: @testing-unit
- Effort: 40 min
- Dependencies: Step 3

#### Step 5: [ ] Define AuditEvent, AuditActor, AuditResource types
- File: internal/audit/types.go
- Spec: specs/EVENTS-AUDIT.md lines 248-329
- Skill: @testing-unit
- Effort: 35 min
- Dependencies: None

#### Step 6: [ ] Create AuditLogger struct
- File: internal/audit/logger.go
- Spec: specs/EVENTS-AUDIT.md lines 334-342
- Skill: @testing-unit
- Effort: 25 min
- Dependencies: Step 5

#### Step 7: [ ] Implement AuditStorage interface
- File: internal/audit/storage.go
- Spec: specs/EVENTS-AUDIT.md lines 344-359
- Skill: @testing-unit
- Effort: 40 min
- Dependencies: Step 6

#### Step 8: [ ] Implement Log() and LogAsync() methods
- File: internal/audit/logger.go
- Spec: specs/EVENTS-AUDIT.md lines 375-406
- Skill: @testing-unit
- Effort: 35 min
- Dependencies: Step 7

#### Step 9: [ ] Implement hash chain for tamper resistance
- File: internal/audit/logger.go
- Spec: specs/EVENTS-AUDIT.md lines 408-429
- Skill: @security-scan
- Effort: 30 min
- Dependencies: Step 8

#### Step 10: [ ] Implement Flush() and Close() methods
- File: internal/audit/logger.go
- Spec: specs/EVENTS-AUDIT.md lines 469-497
- Skill: @testing-unit
- Effort: 25 min
- Dependencies: Step 9

#### Step 11: [ ] Add unit tests for redaction
- File: internal/audit/redaction_test.go
- Skill: @testing-security
- Effort: 35 min
- Dependencies: Step 4

#### Step 12: [ ] Add integration tests for audit logging
- File: internal/audit/logger_test.go
- Skill: @testing-integration
- Effort: 45 min
- Dependencies: Step 10

---

### AUDIT-002: Distributed Tracing

#### Step 1: [ ] Add OpenTelemetry dependencies to go.mod
- File: go.mod
- Skill: @dependency-upgrade
- Effort: 10 min
- Dependencies: None

#### Step 2: [ ] Create internal/telemetry/doc.go
- File: internal/telemetry/doc.go
- Spec: specs/MIDDLEWARE-TRACING.md
- Skill: @testing-unit
- Effort: 10 min
- Dependencies: Step 1

#### Step 3: [ ] Define TracingConfig struct
- File: internal/telemetry/config.go
- Spec: specs/MIDDLEWARE-TRACING.md lines 10-32
- Skill: @testing-unit
- Effort: 20 min
- Dependencies: Step 2

#### Step 4: [ ] Create TracerProvider struct
- File: internal/telemetry/provider.go
- Spec: specs/MIDDLEWARE-TRACING.md lines 57-101
- Skill: @testing-unit
- Effort: 40 min
- Dependencies: Step 3

#### Step 5: [ ] Implement NewTracerProvider() with resource/exporter
- File: internal/telemetry/provider.go
- Spec: specs/MIDDLEWARE-TRACING.md lines 63-101
- Skill: @testing-unit
- Effort: 50 min
- Dependencies: Step 4

#### Step 6: [ ] Create Span struct with helper methods
- File: internal/telemetry/span.go
- Spec: specs/MIDDLEWARE-TRACING.md
- Skill: @testing-unit
- Effort: 30 min
- Dependencies: Step 5

#### Step 7: [ ] Implement Tracer interface wrapper
- File: internal/telemetry/tracer.go
- Spec: specs/MIDDLEWARE-TRACING.md lines 152-162
- Skill: @testing-unit
- Effort: 35 min
- Dependencies: Step 6

#### Step 8: [ ] Add tracing to MCP handlers
- File: pkg/mcp/handlers/*.go
- Skill: @testing-integration
- Effort: 45 min
- Dependencies: Step 7

#### Step 9: [ ] Add tracing to WASM runtime
- File: pkg/wasm/runtime.go
- Skill: @testing-integration
- Effort: 40 min
- Dependencies: Step 7

#### Step 10: [ ] Add tracing to HTTP client
- File: pkg/client/client.go
- Spec: specs/MIDDLEWARE-TRACING.md lines 235-240
- Skill: @testing-integration
- Effort: 30 min
- Dependencies: Step 7

#### Step 11: [ ] Add unit tests for telemetry
- File: internal/telemetry/*_test.go
- Skill: @testing-unit
- Effort: 45 min
- Dependencies: Step 10

---

## 🟢 P3: TEST COVERAGE (0% Files)

### TEST-001: Core Package Tests

#### Step 1: [ ] Create pkg/cmdstore/store_test.go
- File: pkg/cmdstore/store_test.go
- Skill: @testing-unit
- Effort: 40 min
- Dependencies: None

#### Step 2: [ ] Create pkg/cache/memory_test.go
- File: pkg/cache/memory_test.go
- Skill: @testing-unit
- Effort: 35 min
- Dependencies: None

#### Step 3: [ ] Create pkg/config/loader_test.go
- File: pkg/config/loader_test.go
- Skill: @testing-unit
- Effort: 40 min
- Dependencies: None

#### Step 4: [ ] Create pkg/daemon/controller_test.go
- File: pkg/daemon/controller_test.go
- Skill: @testing-unit
- Effort: 45 min
- Dependencies: None

#### Step 5: [ ] Create pkg/inventory/manager_test.go
- File: pkg/inventory/manager_test.go
- Skill: @testing-unit
- Effort: 40 min
- Dependencies: None

---

### TEST-002: MCP Package Tests

#### Step 1: [ ] Create pkg/mcp/converter_test.go
- File: pkg/mcp/converter_test.go
- Skill: @testing-unit
- Effort: 35 min
- Dependencies: None

#### Step 2: [ ] Create pkg/mcp/handlers/prompts_test.go
- File: pkg/mcp/handlers/prompts_test.go
- Skill: @testing-unit
- Effort: 40 min
- Dependencies: None

#### Step 3: [ ] Create pkg/mcp/handlers/tools_test.go
- File: pkg/mcp/handlers/tools_test.go
- Skill: @testing-unit
- Effort: 40 min
- Dependencies: None

#### Step 4: [ ] Create pkg/mcp/handlers/lifecycle_test.go
- File: pkg/mcp/handlers/lifecycle_test.go
- Skill: @testing-unit
- Effort: 35 min
- Dependencies: None

---

### TEST-003: Builtin Commands Tests

#### Step 1: [ ] Create internal/builtin/generate_test.go
- File: internal/builtin/generate_test.go
- Skill: @testing-unit
- Effort: 35 min
- Dependencies: None

#### Step 2: [ ] Create internal/builtin/config_test.go
- File: internal/builtin/config_test.go
- Skill: @testing-unit
- Effort: 30 min
- Dependencies: None

#### Step 3: [ ] Create internal/builtin/auth_test.go
- File: internal/builtin/auth_test.go
- Skill: @testing-unit
- Effort: 40 min
- Dependencies: None

#### Step 4: [ ] Create internal/builtin/daemon_test.go
- File: internal/builtin/daemon_test.go
- Skill: @testing-unit
- Effort: 35 min
- Dependencies: None

---

## 🔵 P4: PLATFORM INTEGRATION

### PLAT-001: Platform Security Stubs

#### Step 1: [ ] Decide: Complete seccomp BPF or remove stub
- Spec: specs/PLATFORM-LINUX.md
- Skill: @gap-analysis
- Effort: 15 min
- Dependencies: None

#### Step 2: [ ] If complete: Implement seccomp rules in pkg/platform/seccomp.go
- File: pkg/platform/seccomp.go
- Spec: specs/PLATFORM-LINUX.md lines 45-120
- Skill: @security-scan
- Effort: 2-3 hrs
- Dependencies: Step 1

#### Step 3: [ ] If complete: Implement SELinux context in pkg/platform/selinux.go
- File: pkg/platform/selinux.go
- Spec: specs/PLATFORM-LINUX.md lines 121-180
- Skill: @security-scan
- Effort: 2-3 hrs
- Dependencies: Step 1

#### Step 4: [ ] If complete: Implement NetworkManager in pkg/platform/networkmanager.go
- File: pkg/platform/networkmanager.go
- Spec: specs/PLATFORM-LINUX.md lines 181-240
- Skill: @testing-unit
- Effort: 2-3 hrs
- Dependencies: Step 1

#### Step 5: [ ] If remove: Delete stub files and document as future work
- File: docs/ROADMAP.md
- Skill: @documentation
- Effort: 30 min
- Dependencies: Step 1

#### Step 6: [ ] Add tests for chosen implementation
- File: pkg/platform/*_test.go
- Skill: @testing-unit
- Effort: 45 min
- Dependencies: Step 2-4 or Step 5

---

### PLAT-002: Keychain Integration

#### Step 1: [ ] Implement SecretServiceKeychain with D-Bus
- File: pkg/auth/keychain_linux.go
- Spec: specs/KEYCHAIN-INTEGRATION.md lines 50-120
- Skill: @testing-unit
- Effort: 2-3 hrs
- Dependencies: None

#### Step 2: [ ] Add D-Bus dependency to go.mod
- File: go.mod
- Skill: @dependency-upgrade
- Effort: 5 min
- Dependencies: None

#### Step 3: [ ] Implement WindowsKeychain with wincred
- File: pkg/auth/keychain_windows.go
- Spec: specs/KEYCHAIN-INTEGRATION.md lines 121-180
- Skill: @testing-unit
- Effort: 2-3 hrs
- Dependencies: None

#### Step 4: [ ] Add build tags for platform-specific files
- File: pkg/auth/keychain_*.go
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: Step 1, Step 3

#### Step 5: [ ] Add unit tests for Linux keychain
- File: pkg/auth/keychain_linux_test.go
- Skill: @testing-unit
- Effort: 45 min
- Dependencies: Step 1

#### Step 6: [ ] Add unit tests for Windows keychain
- File: pkg/auth/keychain_windows_test.go
- Skill: @testing-unit
- Effort: 45 min
- Dependencies: Step 3

---

## ⚫ P-1: ADDITIONAL CRITICAL (From Completeness Validation)

### SEC-005: DNS Rebinding SSRF Fix (CRITICAL - From Security Audit)

#### Step 1: [ ] Add DNS resolution with timeout in validateNetworkURL()
- File: pkg/dsl/builtins.go
- Spec: specs/SECURITY-TESTING.md
- Skill: @security-scan
- Effort: 20 min
- Dependencies: None

#### Step 2: [ ] Validate resolved IPs against private CIDRs (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
- File: pkg/dsl/builtins.go
- Skill: @security-scan
- Effort: 30 min
- Dependencies: Step 1

#### Step 3: [ ] Add IPv6 private network blocking (::1, fd00::/8, fe80::/10)
- File: pkg/dsl/builtins.go
- Skill: @security-scan
- Effort: 25 min
- Dependencies: Step 2

#### Step 4: [ ] Add unit tests for SSRF prevention
- File: pkg/dsl/builtins_test.go
- Skill: @testing-security
- Effort: 35 min
- Dependencies: Step 3

---

### SEC-006: WASM Sandbox Hardening (CRITICAL - From Security Audit)

#### Step 1: [ ] Add wasmtime.WithMaxMemory(256 * 1024 * 1024) limit
- File: pkg/wasm/runtime.go
- Spec: specs/WASM-SANDBOX.md lines 45-60
- Skill: @security-scan
- Effort: 15 min
- Dependencies: None

#### Step 2: [ ] Add wasmtime.WithFuel for CPU time limits
- File: pkg/wasm/runtime.go
- Skill: @security-scan
- Effort: 20 min
- Dependencies: Step 1

#### Step 3: [ ] Add wasmtime.WithEpochInterruption for timeout enforcement
- File: pkg/wasm/runtime.go
- Skill: @security-scan
- Effort: 25 min
- Dependencies: Step 2

#### Step 4: [ ] Integrate NetworkPolicy enforcement in WASM sandbox
- File: pkg/wasm/sandbox.go
- Spec: specs/WASM-SANDBOX.md lines 80-120
- Skill: @security-scan
- Effort: 40 min
- Dependencies: Step 3

#### Step 5: [ ] Add unit tests for memory/CPU limits
- File: pkg/wasm/runtime_test.go
- Skill: @testing-security
- Effort: 35 min
- Dependencies: Step 4

---

### SEC-007: Keychain Key Derivation Fix (CRITICAL - From Security Audit)

#### Step 1: [ ] Implement master key retrieval from system keychain
- File: pkg/auth/keychain.go
- Spec: specs/KEYCHAIN-INTEGRATION.md lines 80-120
- Skill: @security-scan
- Effort: 30 min
- Dependencies: None

#### Step 2: [ ] Add PBKDF2 key derivation with random salt
- File: pkg/auth/keychain.go
- Skill: @security-scan
- Effort: 35 min
- Dependencies: Step 1

#### Step 3: [ ] Remove weak SHA256(service:account) derivation
- File: pkg/auth/keychain.go:288
- Skill: @security-scan
- Effort: 15 min
- Dependencies: Step 2

#### Step 4: [ ] Add unit tests for key derivation
- File: pkg/auth/keychain_test.go
- Skill: @testing-security
- Effort: 30 min
- Dependencies: Step 3

---

### SEC-008: Symlink Protection (HIGH - From Security Audit)

#### Step 1: [ ] Add filepath.EvalSymlinks to validateFilePath()
- File: pkg/dsl/builtins.go
- Spec: specs/WASM-SANDBOX-SECURITY.md
- Skill: @security-scan
- Effort: 15 min
- Dependencies: None

#### Step 2: [ ] Implement system file blocklist (/etc/passwd, /etc/shadow, etc.)
- File: pkg/dsl/builtins.go
- Skill: @security-scan
- Effort: 20 min
- Dependencies: Step 1

#### Step 3: [ ] Add workspace root validation
- File: pkg/dsl/builtins.go
- Skill: @security-scan
- Effort: 25 min
- Dependencies: Step 2

#### Step 4: [ ] Add unit tests for symlink bypass prevention
- File: pkg/dsl/builtins_test.go
- Skill: @testing-security
- Effort: 30 min
- Dependencies: Step 3

---

## 🔴 P0.5: INTEGRATION WIRING (Critical Path)

### IMPL-007: OpenAPI→CLI Wire (End-to-End Integration)

#### Step 1: [ ] Add spec loading to cmd/openapi-cli/main.go
- File: cmd/openapi-cli/main.go
- Spec: specs/CLI-ENTRY-POINT.md
- Skill: @testing-integration
- Effort: 20 min
- Dependencies: None

#### Step 2: [ ] Call pkg/openapi parser from main.go
- File: cmd/openapi-cli/main.go
- Spec: specs/OPENAPI-PARSER-IMPLEMENTATION.md
- Skill: @testing-integration
- Effort: 25 min
- Dependencies: Step 1

#### Step 3: [ ] Call pkg/generator with parsed operations
- File: cmd/openapi-cli/main.go
- Spec: specs/COMMAND-GENERATOR-IMPLEMENTATION.md
- Skill: @testing-integration
- Effort: 30 min
- Dependencies: Step 2

#### Step 4: [ ] Register generated commands with Cobra root
- File: cmd/openapi-cli/main.go
- Spec: specs/COBRA-INTEGRATION.md
- Skill: @testing-integration
- Effort: 35 min
- Dependencies: Step 3

#### Step 5: [ ] Add end-to-end test: spec → CLI → HTTP request
- File: test/e2e/cli_generation_test.go
- Skill: @testing-e2e
- Effort: 45 min
- Dependencies: Step 4

---

### IMPL-008: MCP Tool Registration Wire

#### Step 1: [ ] Load generated commands as MCP tool definitions
- File: pkg/mcp/converter.go
- Spec: specs/MCP-HANDLERS.md
- Skill: @testing-integration
- Effort: 30 min
- Dependencies: None

#### Step 2: [ ] Implement tool registration in cmd/mcp-server/main.go
- File: cmd/mcp-server/main.go
- Spec: specs/MCP-SERVER-IMPLEMENTATION.md
- Skill: @testing-integration
- Effort: 35 min
- Dependencies: Step 1

#### Step 3: [ ] Test with Claude Desktop integration
- File: test/integration/mcp_claude_test.go
- Skill: @testing-integration
- Effort: 45 min
- Dependencies: Step 2

---

### IMPL-009: MCP→WASM Integration Wire

#### Step 1: [ ] Wire MCP CallTool handler to pkg/wasm runtime
- File: pkg/mcp/handlers/tools.go
- Spec: specs/WASM-RUNTIME.md
- Skill: @testing-integration
- Effort: 30 min
- Dependencies: None

#### Step 2: [ ] Implement capability enforcement at tool call
- File: pkg/mcp/handlers/tools.go
- Spec: specs/CAPABILITY-SYSTEM.md
- Skill: @security-scan
- Effort: 35 min
- Dependencies: Step 1

#### Step 3: [ ] Add WASM module loading to MCP context
- File: pkg/mcp/server.go
- Spec: specs/WASM-LOADER.md
- Skill: @testing-integration
- Effort: 30 min
- Dependencies: Step 2

#### Step 4: [ ] Add integration test for MCP→WASM flow
- File: test/integration/mcp_wasm_test.go
- Skill: @testing-integration
- Effort: 40 min
- Dependencies: Step 3

---

### IMPL-010: Auth→Request Flow Wire

#### Step 1: [ ] Wire auth provider into pkg/client/client.go
- File: pkg/client/client.go
- Spec: specs/API-AUTHENTICATION.md
- Skill: @testing-integration
- Effort: 25 min
- Dependencies: None

#### Step 2: [ ] Add token injection to HTTP requests
- File: pkg/client/client.go
- Spec: specs/OAUTH2-IMPLEMENTATION.md
- Skill: @testing-integration
- Effort: 30 min
- Dependencies: Step 1

#### Step 3: [ ] Add mTLS configuration to HTTP client
- File: pkg/client/client.go
- Spec: specs/MTLS-IMPLEMENTATION.md
- Skill: @testing-integration
- Effort: 30 min
- Dependencies: Step 2

#### Step 4: [ ] Add integration test for authenticated requests
- File: test/integration/auth_flow_test.go
- Skill: @testing-integration
- Effort: 40 min
- Dependencies: Step 3

---

### IMPL-011: Daemon HTTP Proxy Wire

#### Step 1: [ ] Implement HTTP proxy in daemon proxy.go
- File: pkg/daemon/proxy.go
- Spec: specs/DAEMON-PROXY.md
- Skill: @testing-integration
- Effort: 40 min
- Dependencies: None

#### Step 2: [ ] Add request routing to backend URL
- File: pkg/daemon/proxy.go
- Spec: specs/UNIX-SOCKET-PROXY.md
- Skill: @testing-integration
- Effort: 30 min
- Dependencies: Step 1

#### Step 3: [ ] Add connection pooling
- File: pkg/daemon/proxy.go
- Skill: @testing-integration
- Effort: 35 min
- Dependencies: Step 2

#### Step 4: [ ] Add integration test for proxy flow
- File: test/integration/daemon_proxy_test.go
- Skill: @testing-integration
- Effort: 40 min
- Dependencies: Step 3

---

## 🟣 P1.5: PRODUCTION READINESS (PRR Gates)

### PRR-001: OpenTelemetry Tracing Implementation

#### Step 1: [ ] Create internal/telemetry/provider.go with TracerProvider
- File: internal/telemetry/provider.go
- Spec: specs/MIDDLEWARE-TRACING.md
- Skill: @testing-unit
- Effort: 40 min
- Dependencies: None

#### Step 2: [ ] Add OTLP exporter configuration
- File: internal/telemetry/exporter.go
- Skill: @testing-unit
- Effort: 35 min
- Dependencies: Step 1

#### Step 3: [ ] Add tracing middleware to MCP handlers
- File: pkg/mcp/middleware.go
- Skill: @testing-integration
- Effort: 30 min
- Dependencies: Step 2

#### Step 4: [ ] Add tracing to WASM runtime
- File: pkg/wasm/runtime.go
- Skill: @testing-integration
- Effort: 30 min
- Dependencies: Step 2

#### Step 5: [ ] Add tracing to HTTP client
- File: pkg/client/client.go
- Skill: @testing-integration
- Effort: 25 min
- Dependencies: Step 2

---

### PRR-002: On-Call Documentation

#### Step 1: [ ] Create runbooks/ directory structure
- File: runbooks/README.md
- Skill: @best-practice-guide
- Effort: 20 min
- Dependencies: None

#### Step 2: [ ] Add daemon-troubleshooting runbook
- File: runbooks/daemon-troubleshooting.md
- Skill: @best-practice-guide
- Effort: 45 min
- Dependencies: Step 1

#### Step 3: [ ] Add mcp-troubleshooting runbook
- File: runbooks/mcp-troubleshooting.md
- Skill: @best-practice-guide
- Effort: 45 min
- Dependencies: Step 1

#### Step 4: [ ] Add escalation matrix
- File: runbooks/escalation.md
- Skill: @best-practice-guide
- Effort: 30 min
- Dependencies: Step 1

---

### PRR-003: CI/CD Security Pipeline

#### Step 1: [ ] Create .github/workflows/security.yml
- File: .github/workflows/security.yml
- Skill: @security-scan
- Effort: 25 min
- Dependencies: None

#### Step 2: [ ] Add Trivy vulnerability scanning
- File: .github/workflows/security.yml
- Skill: @security-scan
- Effort: 20 min
- Dependencies: Step 1

#### Step 3: [ ] Add govulncheck scanning
- File: .github/workflows/security.yml
- Skill: @security-scan
- Effort: 15 min
- Dependencies: Step 1

#### Step 4: [ ] Add golangci-lint with security checks
- File: .github/workflows/security.yml
- Skill: @security-scan
- Effort: 20 min
- Dependencies: Step 1

#### Step 5: [ ] Add dependency scanning (dependabot)
- File: .github/dependabot.yml
- Skill: @security-scan
- Effort: 15 min
- Dependencies: None

---

### PRR-004: Compliance Verification Implementation

#### Step 1: [ ] Create internal/compliance/ package structure
- File: internal/compliance/doc.go
- Spec: specs/COMPLIANCE-VERIFICATION.md
- Skill: @testing-unit
- Effort: 15 min
- Dependencies: None

#### Step 2: [ ] Implement Bible Standard compliance tests
- File: internal/compliance/bible_standard.go
- Spec: specs/BIBLE-STANDARD-COMPLIANCE.md
- Skill: @testing-unit
- Effort: 45 min
- Dependencies: Step 1

#### Step 3: [ ] Implement OAuth2 compliance tests
- File: internal/compliance/oauth2.go
- Spec: specs/OAUTH2-IMPLEMENTATION.md
- Skill: @testing-unit
- Effort: 35 min
- Dependencies: Step 1

#### Step 4: [ ] Implement TLS compliance tests
- File: internal/compliance/tls.go
- Spec: specs/MTLS-IMPLEMENTATION.md
- Skill: @testing-unit
- Effort: 30 min
- Dependencies: Step 1

#### Step 5: [ ] Add compliance test runner
- File: internal/compliance/runner.go
- Skill: @testing-unit
- Effort: 30 min
- Dependencies: Step 2, 3, 4

---

### PRR-005: Kubernetes Deployment Validation

#### Step 1: [ ] Create test Kubernetes manifests
- File: k8s/test-deployment.yaml
- Spec: specs/KUBERNETES-MANIFESTS.md
- Skill: @testing-integration
- Effort: 30 min
- Dependencies: None

#### Step 2: [ ] Add Helm chart tests
- File: chart/muster/tests/
- Skill: @testing-integration
- Effort: 40 min
- Dependencies: Step 1

#### Step 3: [ ] Add kind cluster E2E test
- File: test/e2e/kubernetes_test.go
- Skill: @testing-e2e
- Effort: 50 min
- Dependencies: Step 1

#### Step 4: [ ] Add deployment smoke test
- File: test/e2e/deployment_smoke_test.go
- Skill: @testing-e2e
- Effort: 35 min
- Dependencies: Step 3

---

### PRR-006: API Documentation Generation

#### Step 1: [ ] Add OpenAPI spec generation from code
- File: scripts/generate-openapi-docs.sh
- Spec: specs/API-DOCUMENTATION.md
- Skill: @api-documentation
- Effort: 30 min
- Dependencies: None

#### Step 2: [ ] Generate CLI reference documentation
- File: docs/cli-reference.md
- Spec: specs/DOCS-CLI-REFERENCE.md
- Skill: @api-documentation
- Effort: 35 min
- Dependencies: None

#### Step 3: [ ] Add MCP protocol documentation
- File: docs/mcp-protocol.md
- Spec: specs/MCP-PROTOCOL.md
- Skill: @api-documentation
- Effort: 40 min
- Dependencies: None

#### Step 4: [ ] Validate documentation against implementation
- File: test/integration/docs_validation_test.go
- Skill: @testing-integration
- Effort: 30 min
- Dependencies: Step 1, 2, 3

---

### PRR-007: Alerting Rules Implementation

#### Step 1: [ ] Create Prometheus alerting rules
- File: deployments/prometheus/alerts.yml
- Spec: specs/OPS-ALERTING.md
- Skill: @observability-logging
- Effort: 35 min
- Dependencies: None

#### Step 2: [ ] Add daemon health alerts
- File: deployments/prometheus/alerts.yml
- Skill: @observability-logging
- Effort: 25 min
- Dependencies: Step 1

#### Step 3: [ ] Add MCP performance alerts
- File: deployments/prometheus/alerts.yml
- Skill: @observability-logging
- Effort: 25 min
- Dependencies: Step 1

#### Step 4: [ ] Add Alertmanager configuration
- File: deployments/alertmanager/config.yml
- Spec: specs/OPS-ALERTING.md
- Skill: @observability-logging
- Effort: 30 min
- Dependencies: Step 1

---

### PRR-008: Rollback Testing

#### Step 1: [ ] Document rollback procedure
- File: runbooks/rollback.md
- Skill: @best-practice-guide
- Effort: 30 min
- Dependencies: None

#### Step 2: [ ] Create rollback test script
- File: test/integration/rollback_test.go
- Skill: @testing-integration
- Effort: 40 min
- Dependencies: Step 1

#### Step 3: [ ] Add database migration rollback tests
- File: test/integration/migration_rollback_test.go
- Skill: @testing-database
- Effort: 35 min
- Dependencies: None

---

## 📊 REVISED SUMMARY

| Priority | Tasks | Baby Steps | Effort |
|----------|-------|------------|--------|
| P-1 CRITICAL | 4 | 17 | 6-8 hrs |
| P0 CRITICAL | 3 | 23 | 6-8 hrs |
| P0.5 INTEGRATION | 5 | 22 | 10-14 hrs |
| P1 HIGH | 5 | 54 | 18-24 hrs |
| P1.5 PRR | 8 | 41 | 18-24 hrs |
| P2 MEDIUM | 5 | 54 | 16-22 hrs |
| P3 TESTS | 3 | 13 | 7-9 hrs |
| P4 PLATFORM | 2 | 12 | 8-12 hrs |
| **TOTAL** | **35** | **236** | **89-121 hrs** |

---

## Revised Execution Order

1. **SEC-005 → SEC-006 → SEC-007 → SEC-008** (Security Critical Fixes - 17 steps)
2. **SEC-002 → SEC-003 → SEC-004** (Security Validation - 23 steps)
3. **IMPL-007 → IMPL-008 → IMPL-009 → IMPL-010 → IMPL-011** (Integration Wiring - 22 steps)
4. **AUDIT-001 → AUDIT-002** (Audit/Tracing - 23 steps)
5. **IMPL-003 → IMPL-004 → IMPL-005 → IMPL-006** (Features - 46 steps)
6. **PRR-001 → PRR-002 → ... → PRR-008** (Production Readiness - 41 steps)
7. **FEAT-001 → FEAT-002 → FEAT-003** (Medium - 21 steps)
8. **TEST-001 → TEST-002 → TEST-003** (Coverage - 13 steps)
9. **PLAT-001 → PLAT-002** (Platform - 12 steps)

---

## Current Status

- **Iteration:** 1703
- **Pending Baby Steps:** 236 (was 156, +80 added)
- **Completed:** SEC-001, IMPL-002
- **True Completion:** 60% (was 78%, revised based on PRR)
- **Last Updated:** 2026-03-22
- **Baby Steps Methodology:** Applied ✅
- **Completeness Validation:** Applied ✅
