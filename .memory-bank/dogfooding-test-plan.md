# Muster Dogfooding Test Plan

**Created**: 2026-03-22
**Purpose**: Validate Muster system is actually complete and production-ready using @dogfooding and @completeness-validator skills
**Target Level**: Production (本番完了 / productio)

---

## Executive Summary

This plan uses the Muster system to test itself ("eat our own dog food") to validate:
1. All binaries build and run correctly
2. All tests pass (unit, integration, e2e)
3. CLI commands work as documented
4. MCP server functions properly
5. Daemon operates correctly
6. WASM modules load and execute
7. Authentication flows work
8. Configuration management is functional

---

## Phase 1: Build Validation (Tier 1 - AST/Static)

### 1.1 Binary Build Verification
```bash
# Clean build from scratch
make clean
make build

# Verify binaries exist
ls -lh bin/
# Expected: openapi-cli (15M), openapi-mcp (13M), openapi-daemon (8.4M)
```

### 1.2 Static Analysis
```bash
# Run linters
make lint

# Run go vet
make vet

# Check for compilation errors
go build ./...
```

### 1.3 Dependency Verification
```bash
# Verify go.mod is tidy
go mod tidy
git diff go.mod go.sum

# Check for security vulnerabilities in dependencies
go list -m -json all | grep -E '"Path"|"Version"'
```

---

## Phase 2: Runtime Verification (Tier 2 - Integration)

### 2.1 Unit Tests
```bash
# Run all unit tests with coverage
go test -v -cover ./...

# Target: >70% coverage for cmd/* packages
go test -cover ./cmd/openapi-cli   # Target: 55.6%+
go test -cover ./cmd/mcp-server    # Target: 72.0%+
go test -cover ./cmd/daemon        # Target: 56.9%+
```

### 2.2 Integration Tests
```bash
# Run integration test suite
make test-integration

# Run demo test suite
make demo

# Docker-based integration tests (Docker is locally installed)
docker-compose -f docker-compose.yml up --abort-on-container-exit --build
```

### 2.3 End-to-End Tests
```bash
# Run E2E tests
make test-e2e

# Full stack validation
./demo/test-suites/06-full-stack.sh
```

---

## Phase 3: Production Readiness Review (Tier 3 - PRR)

### 3.1 CLI Functionality Test
```bash
# Test CLI binary
./bin/openapi-cli --help
./bin/openapi-cli version
./bin/openapi-cli config show
./bin/openapi-cli inventory list

# Test OpenAPI spec parsing
./bin/openapi-cli generate https://petstore3.swagger.io/api/v3/openapi.json
```

### 3.2 MCP Server Test
```bash
# Start MCP server in stdio mode
./bin/openapi-mcp --help

# Verify MCP protocol support
# (Requires MCP client for full testing)
```

### 3.3 Daemon Test
```bash
# Start daemon
./bin/openapi-daemon start

# Check daemon status
./bin/openapi-daemon status

# Test daemon proxy
curl --unix-socket /tmp/muster-daemon.sock http://localhost/health

# Stop daemon
./bin/openapi-daemon stop
```

### 3.4 Authentication Test
```bash
# Test auth flows
./bin/openapi-cli auth login --help
./bin/openapi-cli auth status
./bin/openapi-cli auth logout
```

### 3.5 Configuration Test
```bash
# Test config management
./bin/openapi-cli config profile list
./bin/openapi-cli config show
```

---

## Phase 4: Dogfooding Specific Tests

### 4.1 Use Muster to Parse Its Own Specs
```bash
# Generate CLI commands from Muster's own specs
./bin/openapi-cli generate ./specs/master-specification.md

# List available operations
./bin/openapi-cli inventory fetch --spec ./specs/master-specification.md
```

### 4.2 WASM Module Execution
```bash
# Test WASM loading (if modules exist)
ls demo/wasm-modules/

# Execute WASM via CLI
./bin/openapi-cli module execute <module.wasm>
```

### 4.3 DSL Script Execution
```bash
# Run DSL scripts from demo
ls demo/scripts/
./bin/openapi-cli script run demo/scripts/example.star
```

---

## Phase 5: Completeness Validation Checklist

### Level 1: Superficial (表面完了)
- [ ] Code exists in repository
- [ ] Compiles without errors (`make build`)
- [ ] No obvious syntax errors

### Level 2: Functional (機能完了)
- [ ] Runs in isolation
- [ ] Basic tests pass (`go test ./...`)
- [ ] Mock data works (demo examples)

### Level 3: Integrated (統合完了)
- [ ] Connected to real systems (SQLite database)
- [ ] Uses actual data sources
- [ ] Inter-component communication works (daemon ↔ CLI)

### Level 4: Production (本番完了)
- [ ] Deployable (Docker images build)
- [ ] Health checks respond (daemon /health)
- [ ] Monitoring in place (Prometheus metrics)
- [ ] Edge cases handled (error handling)

### Level 5: Complete (完全完了 / perfectus)
- [ ] All specs implemented (265/227 analyzed)
- [ ] All tests passing (unit, integration, e2e)
- [ ] Documentation complete (README, AGENTS.md)
- [ ] No technical debt (no TODO/FIXME in production code)
- [ ] Handoff-ready (memory-bank complete)

---

## Phase 6: Anti-Pattern Detection

### 6.1 Mock Data Check
```bash
# Search for mock data patterns
grep -r "mock\|Mock\|MOCK" pkg/ --include="*.go" | grep -v "_test.go" | grep -v "vendor/"
grep -r "TODO\|FIXME\|HACK" pkg/ --include="*.go" | grep -v "_test.go"
```

### 6.2 Stub Implementation Check
```bash
# Search for unimplemented functions
grep -r "not implemented\|NotImplemented\|panic(" pkg/ --include="*.go" | grep -v "_test.go"
grep -r "return nil, nil" pkg/ --include="*.go" | grep -v "_test.go"
```

### 6.3 Integration Gap Check
```bash
# Verify all imports resolve
go mod verify

# Check for missing connections
grep -r "// TODO: connect\|// wire up" . --include="*.go"
```

---

## Phase 7: Docker Testing (Locally Installed)

### 7.1 Container Build Test
```bash
# Build Docker images
docker build -t muster-cli:latest .
docker build -t muster-mcp:latest -f Dockerfile.mcp .
docker build -t muster-daemon:latest -f Dockerfile.daemon .
```

### 7.2 Container Runtime Test
```bash
# Run full stack with docker-compose
docker-compose up -d

# Test CLI in container
docker-compose exec cli ./openapi-cli version

# Test daemon connectivity
docker-compose exec cli curl http://daemon:8080/health

# Cleanup
docker-compose down
```

---

## Completion Criteria

### PASS Criteria (本番完了)
- [ ] All 3 binaries build successfully
- [ ] All unit tests pass (>70% coverage)
- [ ] Integration tests pass
- [ ] E2E tests pass
- [ ] CLI commands work as documented
- [ ] Daemon starts/stops correctly
- [ ] Docker containers build and run
- [ ] No critical gaps in completeness validation

### FAIL Criteria (未完成)
- [ ] Any binary fails to build
- [ ] Tests fail with real errors
- [ ] CLI commands crash
- [ ] Daemon fails to start
- [ ] Mock data detected in production code
- [ ] Integration gaps found
- [ ] Docker build fails

---

## Report Template

```
MUSTER DOGFOODING VALIDATION REPORT
====================================
Date: 2026-03-22
Validator: @completeness-validator + @sherlock
Target Level: Production (本番完了)

TIER 1: STATIC ANALYSIS
-----------------------
[ ] Binary Build: PASS/FAIL
[ ] Lint Check: PASS/FAIL
[ ] Type Check: PASS/FAIL

TIER 2: RUNTIME VERIFICATION
----------------------------
[ ] Unit Tests: X/Y passed (Z%)
[ ] Integration Tests: X/Y passed
[ ] E2E Tests: X/Y passed

TIER 3: PRODUCTION READINESS
----------------------------
[ ] CLI Functional: PASS/FAIL
[ ] MCP Server: PASS/FAIL
[ ] Daemon: PASS/FAIL
[ ] Docker: PASS/FAIL

COMPLETENESS LEVEL: [superficialis/functionalis/integratus/productio/perfectus]

CRITICAL GAPS:
1. [gap description]
2. [gap description]

RECOMMENDATION: [READY/NOT READY] for production
```

---

## Next Steps After Validation

1. **If PASS**: Document success in `.memory-bank/progress.md`
2. **If FAIL**: Create issues for critical gaps, update TODO.md
3. **If PARTIAL**: Prioritize gaps by severity, schedule fixes
4. **Continuous**: Run dogfooding validation weekly

---

## Related Skills Used

- `@dogfooding` - Test system using itself
- `@completeness-validator` - Validate true completion
- `@testing-e2e` - End-to-end workflows
- `@testing-integration` - Component connections
- `@trust-but-verify` - Independent validation
- `@gap-analysis` - Identify missing requirements

---

## References

- `.agents/skills/dogfooding/SKILL.md`
- `.agents/skills/completeness-validator/SKILL.md`
- `AGENTS.md` - Project guidelines
- `specs/master-specification.md` - Primary spec
