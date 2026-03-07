# Phase 31: Real Integration Requirements

**Status**: ❌ NOT COMPLETE - Toy tests pass, real integration missing
**Last Updated**: 2026-03-03
**Adversarial Review**: FAILED - Tests check existence, not functionality

---

## The Problem

Current Phase 31 "completion" is a **facade**:
- ✅ 6 test suites "passing" 
- ❌ Tests only check if files exist / commands don't crash
- ❌ No actual end-to-end workflows proven
- ❌ Components still isolated (not integrated)

**Example of Toy Test vs Real Test:**

```bash
# TOY TEST (Current - PASSES but proves nothing)
if [ -f "$BINARY" ]; then echo "✓ Binary exists"; fi
if $BINARY generate --help; then echo "✓ Generate works"; fi

# REAL TEST (Required - Proves integration)
# 1. Parse petstore.yaml
# 2. Generate CLI commands for GET /pets
# 3. Execute: openapi-cli pets list
# 4. Verify HTTP request made to mock server
# 5. Verify response parsed and displayed correctly
```

---

## What "COMPLETE" Actually Means

### 31.1 Real CLI Integration Test
**Current**: Checks binary exists and help works
**Required**:
```bash
# test-cli-real.sh
# Given: petstore.yaml with GET /pets endpoint
# When: Run 'openapi-cli generate petstore.yaml'
# Then: Commands registered and executable
# And: 'openapi-cli pets list' makes HTTP request
# And: Response displayed in proper format
```

**Must Prove:**
- [ ] OpenAPI parser extracts operations correctly
- [ ] Command generator creates executable commands
- [ ] Generated commands make real HTTP requests
- [ ] Responses are parsed and formatted
- [ ] Error cases handled gracefully

### 31.2 Real MCP Integration Test  
**Current**: Checks MCP package exists
**Required**:
```bash
# test-mcp-real.sh
# Given: MCP server started with petstore spec
# When: Send JSON-RPC initialize request
# Then: Server responds with capabilities
# And: tools/list returns petstore operations as tools
# And: tools/call executes HTTP request
# And: Response returned via JSON-RPC
```

**Must Prove:**
- [ ] MCP server starts and accepts connections
- [ ] OpenAPI operations registered as MCP tools
- [ ] JSON-RPC protocol works end-to-end
- [ ] Tool calls trigger HTTP requests
- [ ] Responses returned in MCP format

### 31.3 Real WASM Integration Test
**Current**: Checks WASM directory exists
**Required**:
```bash
# test-wasm-real.sh
# Given: Valid WASM module (e.g., hello.wasm)
# When: Load module into WASM runtime
# Then: Module executes in sandbox
# And: Capabilities enforced (filesystem, network)
# And: Result returned to caller
```

**Must Prove:**
- [ ] WASM runtime integrated (wazero/wasmtime)
- [ ] Modules can be loaded and executed
- [ ] Sandbox restricts capabilities
- [ ] Resource limits enforced
- [ ] Errors handled safely

### 31.4 Real Auth Integration Test
**Current**: Checks auth packages exist
**Required**:
```bash
# test-auth-real.sh
# Given: OAuth2 configuration
# When: Run 'openapi-cli auth login --oauth2'
# Then: OAuth flow completes
# And: Token stored securely
# And: Token used in subsequent requests
# And: Token refreshes automatically
```

**Must Prove:**
- [ ] OAuth2 flow works end-to-end
- [ ] Tokens stored and retrieved from keychain
- [ ] Authenticated requests include tokens
- [ ] Token refresh works
- [ ] mTLS authentication works

### 31.5 Real Daemon Integration Test
**Current**: Checks daemon directory exists
**Required**:
```bash
# test-daemon-real.sh
# Given: Daemon started with 'daemon start'
# When: Connect HTTP client to Unix socket
# And: Send HTTP request through socket
# Then: Request routed to appropriate handler
# And: Response returned through socket
```

**Must Prove:**
- [ ] Daemon creates and listens on Unix socket
- [ ] HTTP requests proxied through socket
- [ ] Routing works correctly
- [ ] Concurrent connections handled
- [ ] Clean shutdown

### 31.6 Real Full Stack Test
**Current**: Checks packages exist
**Required**:
```bash
# test-fullstack-real.sh
# Full pipeline:
# 1. Start daemon
# 2. Start MCP server  
# 3. Load OpenAPI spec
# 4. Register tools with MCP
# 5. Call tool via MCP
# 6. Execute via WASM (if needed)
# 7. Make HTTP request
# 8. Return response through full chain
```

**Must Prove:**
- [ ] All components start together
- [ ] Spec loads and registers
- [ ] MCP tool calls trigger execution
- [ ] WASM executes if required
- [ ] HTTP request made
- [ ] Response flows back through chain

---

## Critical Implementation Gaps

### 1. WASM Runtime (0% Real Integration)
**Current**: Skeleton files exist
**Missing**: 
- [ ] WASM engine integration (wazero/wasmtime)
- [ ] Module execution
- [ ] Sandbox enforcement
- [ ] Host function bindings

**Required for Real Test:**
```go
// pkg/wasm/runtime.go must actually run WASM
func (r *Runtime) Execute(module []byte, function string, args ...interface{}) (interface{}, error) {
    // Real WASM execution here
}
```

### 2. Daemon Socket & Proxy (0% Real Integration)
**Current**: PID file management only
**Missing**:
- [ ] Unix socket creation
- [ ] HTTP proxy implementation
- [ ] Request routing
- [ ] Connection handling

**Required for Real Test:**
```go
// cmd/daemon/main.go must actually proxy
func startProxy(socketPath string, handler http.Handler) error {
    // Real socket + HTTP proxy here
}
```

### 3. MCP Tool Registration (0% Real Integration)
**Current**: Handler registry exists
**Missing**:
- [ ] OpenAPI→Tool conversion
- [ ] Tool registration with server
- [ ] Tool call execution
- [ ] HTTP request triggering

**Required for Real Test:**
```go
// pkg/mcp/handlers/tools.go must register real tools
func (h *ToolHandler) RegisterFromSpec(spec *openapi3.T) error {
    // Convert OpenAPI ops to MCP tools
    // Register with server
}
```

### 4. Command Persistence (0% Real Integration)
**Current**: Commands added to parent at runtime
**Missing**:
- [ ] Commands saved to config
- [ ] Commands loaded on startup
- [ ] Dynamic reloading

**Required for Real Test:**
```go
// Generated commands must be usable in new CLI instance
```

### 5. Auth Token Refresh (0% Real Integration)
**Current**: OAuth flow started
**Missing**:
- [ ] Token refresh implementation
- [ ] Automatic refresh on expiry
- [ ] Storage in secure keychain

**Required for Real Test:**
```go
// pkg/auth/oauth2.go Refresh must work
func (p *OAuth2Provider) Refresh(token *Token) (*Token, error) {
    // Real refresh logic here
}
```

---

## Acceptance Criteria for Phase 31 COMPLETE

### Minimum Viable Integration (MVP)

**CLI Integration (Must Have):**
```bash
# User can actually:
$ openapi-cli generate demo/openapi-specs/petstore.yaml
Generated 5 commands: pets list, pets create, pets get, pets update, pets delete

$ openapi-cli pets list
ID    Name      Species
1     Fido      dog
2     Whiskers  cat
```

**MCP Integration (Must Have):**
```bash
# MCP server actually works:
$ mcp-server --spec demo/openapi-specs/petstore.yaml &
$ echo '{"jsonrpc":"2.0","method":"tools/list","id":1}' | nc localhost 8080
{"jsonrpc":"2.0","result":{"tools":[{"name":"pets_list"}...]},"id":1}
```

**WASM Integration (Should Have):**
```bash
# WASM actually executes:
$ openapi-cli modules install demo/wasm-modules/transform.wasm
$ openapi-cli modules exec transform --input '{"name":"test"}'
{"name":"test","transformed":true}
```

**Auth Integration (Should Have):**
```bash
# Auth actually works:
$ openapi-cli auth login --oauth2
# ... OAuth flow ...
$ openapi-cli request GET https://api.example.com/protected
# Uses stored token automatically
```

**Daemon Integration (Could Have):**
```bash
# Daemon actually proxies:
$ openapi-cli daemon start
$ curl --unix-socket /tmp/muster.sock http://localhost/pets
# Returns response from configured API
```

---

## Updated TODO.md Structure

Replace Phase 31 section with:

```markdown
## Phase 31: Integration & Validation ⏳ IN PROGRESS (40%)

**Status**: Toy tests complete (✅), Real integration missing (❌)
**Adversarial Review**: Failed - Tests check existence not functionality

### 31.1 Real CLI Integration ⏳ (0%)
- [ ] Implement command persistence
- [ ] Connect OpenAPI parser to command execution
- [ ] Test real HTTP requests from generated commands
- [ ] **ACCEPTANCE**: User can generate and execute real commands

### 31.2 Real MCP Integration ⏳ (0%)
- [ ] Implement OpenAPI→Tool conversion
- [ ] Register tools with MCP server
- [ ] Test JSON-RPC end-to-end
- [ ] **ACCEPTANCE**: MCP server works with Claude Desktop

### 31.3 Real WASM Integration ⏳ (0%)
- [ ] Integrate WASM runtime (wazero)
- [ ] Implement module execution
- [ ] Test sandbox enforcement
- [ ] **ACCEPTANCE**: WASM modules execute with capability restrictions

### 31.4 Real Auth Integration ⏳ (20%)
- [x] OAuth2 flow skeleton
- [ ] Implement token refresh
- [ ] Test end-to-end auth
- [ ] **ACCEPTANCE**: Auth flows work with real APIs

### 31.5 Real Daemon Integration ⏳ (0%)
- [ ] Implement Unix socket
- [ ] Implement HTTP proxy
- [ ] Test request routing
- [ ] **ACCEPTANCE**: Daemon proxies HTTP requests

### 31.6 Real Integration Tests ⏳ (0%)
- [ ] Replace toy tests with real tests
- [ ] Tests must exercise actual workflows
- [ ] Tests must fail if functionality broken
- [ ] **ACCEPTANCE**: All tests prove real integration
```

---

## Files to Update

1. **.agents/skills/ralph-wiggum-loop/TODO.md**
   - Mark Phase 31 as IN PROGRESS (40%)
   - Add detailed real integration tasks
   - Remove toy test completion markers

2. **.agents/skills/ralph-wiggum-loop/PROMPT.md**
   - Update status to reflect real gaps
   - Add acceptance criteria section
   - Emphasize "real functionality over passing tests"

3. **.memory-bank/progress.md**
   - Change 85% → 40%
   - Document adversarial findings
   - Add real integration requirements

4. **.memory-bank/activeContext.md**
   - Mark Phase 31 as INCOMPLETE
   - List critical gaps
   - Define real completion criteria

5. **specs/INTEGRATION-TEST-SUITE.md**
   - Document difference between toy and real tests
   - Add real integration test patterns
   - Include acceptance criteria

---

## Success Metrics (Real)

**NOT this:**
- 6 test suites passing ✅
- Demo directory exists ✅
- All green checkmarks ✅

**BUT this:**
- User can clone repo and actually USE the system
- `make demo` runs real end-to-end workflows
- Components actually connect (not just exist)
- Tests would catch real integration bugs
- System could be deployed to production

---

## Next Actions for Ralph Loop

**Priority 1: CLI Real Integration**
1. Implement command persistence
2. Wire OpenAPI parser → command execution
3. Test with real HTTP requests

**Priority 2: MCP Real Integration**
1. Build OpenAPI→Tool converter
2. Register tools with server
3. Test with real JSON-RPC client

**Priority 3: Replace Toy Tests**
1. Delete current integration tests
2. Write tests that exercise real code paths
3. Tests fail if integration broken

**Priority 4: WASM, Auth, Daemon**
1. Integrate real WASM runtime
2. Complete auth token refresh
3. Implement socket + proxy

**DO NOT proceed to Phase 32 until Priority 1-3 complete.**
