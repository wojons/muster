# Agent Guidelines for Muster

Guidelines for AI agents working on the Muster codebase (dynamic API client & MCP server).

## CRITICAL: Integration Over Isolation

**This is NOT a collection of packages - it's an integrated system.**

Every component must:
1. ✅ Work in isolation (unit tests)
2. ✅ **Connect to other components** (integration tests)
3. ✅ **Prove end-to-end functionality** (demo application)

## Quick Commands

```bash
# Build
make build                    # Build main binary
make build-cli                # Build CLI only
make build-mcp                # Build MCP server only
make build-daemon             # Build daemon (implement!)

# Testing (CRITICAL - run ALL before committing)
go test -v ./...              # Run all unit tests
make test-unit                # Unit tests only
make test-integration         # ⭐ INTEGRATION TESTS
make test-e2e                 # ⭐ END-TO-END TESTS
make test-race                # Race condition tests

# Validation
make validate                 # ⭐ FULL SYSTEM VALIDATION
make demo                     # ⭐ RUN DEMO APPLICATION
check                         # Run all checks (test + lint + validate)

# Lint/Format
make lint                     # Run golangci-lint
make fmt                      # Format with go fmt
make vet                      # Run go vet
golangci-lint run ./...       # Full linting

# Development
make dev                      # Run in dev mode
make watch                    # Watch and rebuild (requires entr)
make deps                     # Download dependencies
make tidy                     # Tidy go modules

# Skills (USE THESE!)
make skill-gap-analysis       # Find implementation gaps
make skill-test-gap-analysis  # Find missing tests
make skill-security-scan      # Security validation
make skill-trust-verify       # Verify system claims
```

## Project Structure

```
cmd/                        # Entry points
  openapi-cli/              # CLI application
  mcp-server/               # MCP server  
  daemon/                   # Background daemon

pkg/                        # Public APIs (26+ packages)
  auth/                     # Authentication (OAuth2, mTLS, keychain)
  cache/                    # Multi-tier caching (memory, Redis)
  cli/                      # CLI framework integration
  client/                   # HTTP client
  cmdstore/                 # Command storage/management
  completion/               # Shell completion generators
  config/                   # Configuration management
  daemon/                   # Daemon client/controller
  dsl/                      # Starlark DSL interpreter
  errors/                   # Custom error types
  generator/                # Command generation from OpenAPI
  inventory/                # Inventory fetching & caching
  log/                      # Structured logging
  mcp/                      # MCP protocol implementation
  openapi/                  # OpenAPI parsing
  platform/                 # Platform-specific code (Linux/macOS/Windows)
  prompt/                   # Interactive prompts
  rate/                     # Rate limiting
  repository/               # Database layer (SQLite)
  request/                  # HTTP request building
  response/                 # HTTP response handling
  wasm/                     # WASM runtime (Wasmtime)

internal/                   # Internal utilities
  builtin/                  # Built-in commands
  commands/                 # Command implementations
  compliance/               # Compliance verification
  database/                 # Database connections
  middleware/               # HTTP middleware
  monitoring/               # Metrics & monitoring
  perf/                     # Performance utilities
  platform/                 # Platform abstractions
  rendering/                # Output rendering
  security/                 # Security utilities
  storage/                  # Storage abstractions
  telemetry/                # Telemetry & tracing
  testvectors/              # Test vectors
  version/                  # Version information

specs/                      # 276 specification files
  master-specification.md   # Primary north star spec
  implementation/           # Implementation specs
  *.md                      # Feature specifications

.memory-bank/               # Project context & decisions
  projectbrief.md           # Project overview
  productContext.md         # Why this exists
  systemPatterns.md         # Architecture patterns
  techContext.md            # Technical decisions
  activeContext.md          # Current work focus
  progress.md               # What's done/remaining

.agents/skills/             # 63 available agent skills
  */SKILL.md                # Individual skill definitions

demo/                       # Demo application
test/                       # Test utilities & fixtures
bin/                        # Compiled binaries (gitignored)
docs/                       # Documentation
migrations/                 # Database migrations
scripts/                    # Build & utility scripts
web/                        # Web assets
api/                        # API definitions
chart/                      # Helm charts
k8s/                        # Kubernetes manifests
debian/                     # Debian packaging
build/                      # Build scripts
deployments/                # Deployment configs
configs/                    # Configuration examples
tmp/                        # Temporary files (gitignored)
```

## Code Style

### Imports (grouped)
```go
import (
    "context"
    "fmt"
    "time"
    
    "github.com/getkin/kin-openapi/openapi3"
    "github.com/stretchr/testify/require"
    
    "github.com/anomalyco/muster/pkg/errors"
)
```

### Error Handling
Use custom `AppError` from `pkg/errors`:
```go
// Creating errors
return nil, errors.New(errors.CodeNotFound, "resource not found")

// Wrapping errors
return nil, errors.Wrap(err, errors.CodeDatabase, "failed to query")

// With details
return errors.New(errors.CodeValidation, "invalid input").
    WithDetail("field", "email").
    WithSeverity(errors.SeverityHigh)

// Checking errors
if errors.IsError(err, errors.CodeNotFound) { ... }
```

### Naming Conventions
- **Types**: PascalCase (`SQLiteManager`, `ParseOptions`)
- **Interfaces**: Noun describing behavior (`Parser`, `Transaction`)
- **Methods**: MixedCase (`ExtractOperations`, `WithDetail`)
- **Private**: camelCase (`extractParameters`, `buildPath`)
- **Constants**: Use typed constants with descriptive names
- **Error codes**: `Code` prefix (`CodeNotFound`, `CodeValidation`)
- **Test functions**: `Test<Type>_<Method>_<Scenario>`

### Types
```go
// Use explicit types, avoid `interface{}` when possible
type Code string
type Severity string

// Struct tags for JSON
type Config struct {
    Path string `json:"path" yaml:"path"`
}

// Options pattern
type ParseOptions struct {
    SkipValidation bool
    MaxDepth       int
}
```

### Testing
**REQUIREMENT: Write THREE types of tests**
```go
// 1. Unit test
func TestParser_Parse_ValidSpec(t *testing.T) {
    ctx := context.Background()
    parser := NewParser()
    
    result, err := parser.Parse(ctx, spec, opts)
    require.NoError(t, err)
    require.NotNil(t, result)
    require.Equal(t, "1.0.0", result.Version)
}

// 2. Integration test - prove connections work
func TestParser_Integration_WithGenerator(t *testing.T) {
    // Parse spec
    doc, err := parser.Parse(spec)
    require.NoError(t, err)
    
    // Feed to generator
    commands, err := generator.Generate(doc)
    require.NoError(t, err)
    
    // Verify connection works
    require.NotEmpty(t, commands)
}

// 3. End-to-end test - prove system works
func TestE2E_FullPipeline(t *testing.T) {
    // Load spec → Parse → Generate → Execute → Validate
    result := RunFullPipeline("petstore.yaml")
    require.NoError(t, result.Error)
    require.NotNil(t, result.Response)
}
```

### Comments
- All exported functions must have comments
- Comments should describe what the function does, not how
- Use complete sentences with proper punctuation

```go
// ExtractOperations extracts all operations from an OpenAPI document.
// Returns a slice of operations or an error if parsing fails.
func (p *Parser) ExtractOperations(doc *openapi3.T) ([]*Operation, error) {
```

### Build Configuration
- Requires CGO_ENABLED=1 for SQLite support
- Version info injected via ldflags at build time
- Supports cross-compilation for multiple platforms

## Integration Testing Guidelines

### Every Component Needs Integration Tests
```go
// After implementing any component:
// 1. Unit tests (component in isolation)
// 2. Integration tests (component + dependencies)
// 3. Add to demo application
```

### Integration Test Pattern
```go
func TestComponent_Integration_WithUpstream(t *testing.T) {
    // Given: upstream component produces output
    upstreamOutput := upstreamComponent.Process(input)
    
    // When: this component consumes it
    result, err := thisComponent.Process(upstreamOutput)
    
    // Then: connection works
    require.NoError(t, err)
    require.NotNil(t, result)
}

func TestComponent_Integration_WithDownstream(t *testing.T) {
    // Given: this component produces output
    thisOutput := thisComponent.Process(input)
    
    // When: downstream consumes it
    result, err := downstreamComponent.Process(thisOutput)
    
    // Then: connection works
    require.NoError(t, err)
}
```

### Demo Application Updates
**After EVERY feature:**
1. Add to `demo/openapi-specs/` if it's an API feature
2. Add to `demo/wasm-modules/` if it's a WASM feature
3. Add to `demo/scripts/` if it's a DSL feature
4. Add to `demo/test-suites/` integration test
5. Document in `demo/README.md`

## Before Committing (UPDATED CHECKLIST)

### 1. Unit Tests
```bash
go test -v ./...
```
- [ ] All tests passing
- [ ] Coverage > 80%

### 2. Integration Tests
```bash
make test-integration
```
- [ ] Integration tests passing
- [ ] Components connect properly

### 3. End-to-End Tests
```bash
make test-e2e
```
- [ ] E2E scenarios passing
- [ ] System works as whole

### 4. Demo Validation
```bash
make demo
```
- [ ] Demo runs successfully
- [ ] Feature is demonstrable

### 5. Full Validation
```bash
make validate
```
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Linting passes
- [ ] Security scan passes

### 6. Skill Validation (USE SKILLS!)
```bash
# Run critical skills
make skill-gap-analysis
make skill-test-gap-analysis
make skill-security-scan
```
- [ ] No critical gaps found
- [ ] No security issues
- [ ] System claims validated

### 7. Code Quality
```bash
make lint
make fmt
make vet
```
- [ ] No linting errors
- [ ] Code formatted
- [ ] No vet issues

## Available Skills (63 Total - USE THEM!)

**Critical Skills for Integration:**
- `@testing-integration` - Build integration tests
- `@testing-e2e` - End-to-end validation
- `@test-gap-analysis` - Find missing tests
- `@gap-analysis` - Find spec-implementation gaps

**Critical Skills for Quality:**
- `@security-scan` - Security validation
- `@trust-but-verify` - Validate claims
- `@assumption-testing` - Challenge assumptions
- `@redteam` - Adversarial testing

**Critical Skills for Completeness:**
- `@spec-gap-analysis` - Analyze spec coverage
- `@test-orchestrator` - Coordinate tests
- `@reality-validation` - Real-world validation
- `@devils-advocate` - Challenge decisions

### How to Use Skills
```bash
# Makefile targets (add to Makefile)
skill-gap-analysis:
	@opencode run --agent gap-analysis "Analyze implementation gaps"

skill-test-gap-analysis:
	@opencode run --agent test-gap-analysis "Find missing test coverage"

skill-security-scan:
	@opencode run --agent security-scan "Scan for vulnerabilities"

skill-trust-verify:
	@opencode run --agent trust-but-verify "Validate system claims"
```

### Skill Schedule
- **Daily**: @security-scan, @trust-but-verify
- **Weekly**: @gap-analysis, @test-gap-analysis
- **Per Phase**: @testing-integration, @testing-e2e
- **Monthly**: @redteam, @devils-advocate

## Integration Test Requirements

### Must Have Integration Tests For:
1. **OpenAPI Parser → Command Generator**
   - Parse spec → Generate commands → Validate structure
   
2. **CLI → MCP Server**
   - CLI command → MCP tool registration → Execution
   
3. **MCP Server → WASM Runtime**
   - Tool call → Load WASM → Execute → Return result
   
4. **Authentication → Protected Endpoints**
   - Auth flow → Token storage → Protected request → Valid response
   
5. **Daemon → Socket → Proxy**
   - Start daemon → Connect socket → Route request → Response
   
6. **Caching Layer**
   - Request → Cache check → Miss → Fetch → Store → Hit
   
7. **Full Stack**
   - OpenAPI → CLI → MCP → WASM → Response

### Integration Test Location
```
demo/test-suites/
├── 01-cli-generation.sh
├── 02-mcp-flow.sh
├── 03-wasm-execution.sh
├── 04-auth-flows.sh
├── 05-daemon-socket.sh
└── 06-full-stack.sh
```

## Demo Application Requirements

### The Demo Must:
1. **Exercise ALL major features**
2. **Run with `make demo`**
3. **Complete in < 5 minutes**
4. **Show clear success/failure**
5. **Be documented in README**

### Demo Structure:
```
demo/
├── README.md              # Quick start guide
├── docker-compose.yml     # Full stack
├── openapi-specs/         # Test APIs
├── wasm-modules/          # Test plugins
├── scripts/               # DSL examples
└── test-suites/           # Integration tests
```

## Resources

- **Memory Bank**: `.memory-bank/` (project context and decisions)
- **Contributing**: `CONTRIBUTING.md`
- **Makefile**: `make help` for all targets
- **Go version**: 1.25.0 (see go.mod)
- **Specs**: `specs/` (276 specification files)
- **Demo**: `demo/` (prove system works)
- **Skills**: `.agents/skills/` (63 available skills)

## Key Principles

1. **Integration is NOT optional** - Every component must connect
2. **Demo proves functionality** - If you can't demo it, it's not done
3. **Skills improve quality** - Use the 63 available skills regularly
4. **Adversarial thinking** - Challenge assumptions, test edge cases
5. **Trust but verify** - Validate all claims independently

## Questions?

If unclear:
1. Check `specs/` for detailed specifications
2. Check `.agents/skills/` for relevant skills
3. Check `demo/` for working examples
4. Run `make validate` to check current state

**Remember: The goal is a WORKING SYSTEM, not just code that compiles.**

## GitReins Quality Harness (MANDATORY)

This repo uses GitReins as its quality gate. Every commit runs static guards.
If guards fail, the commit is BLOCKED. You cannot skip this.

### Quick check before committing:

```bash
PATH="$HOME/go/bin:$HOME/gitreins-poc/.venv/bin:$PATH" gitreins guard
```

### What's checked:
- **secrets** — API keys, tokens, passwords (BLOCKS on fail — no exceptions)
- **build** — compiles the project (BLOCKS on fail)
- **lint** — go vet / golangci-lint (WARNS on fail)
- **tests** — runs tests for changed packages only (BLOCKS on fail)

### Test mode: full
Only packages with staged changes are tested. Pre-existing failures in
untouched code will NOT block your commit. If you change go.mod,
Makefile, .gitreins/config.yaml, or a config file, the full suite runs
as a safety net.

### Tasks and evaluation:

```bash
# Create a task with criteria
gitreins task create fix-auth "Fix authentication" \
  "Login accepts email+password and returns JWT" \
  "Invalid credentials return 401" \
  "Rate limiting works after 5 failed attempts"

# Do the work, then evaluate:
gitreins task start fix-auth
# ... implement ...
gitreins task complete fix-auth    # triggers LLM evaluation

# Or evaluate standalone:
gitreins judge fix-auth
```

### If guards fail:
1. READ the output — the guard tells you exactly what failed and where
2. Fix the issues. Do NOT commit with --no-verify unless it's a docs-only
   change or a GitReins self-upgrade.
3. Re-run `gitreins guard` until it passes
4. Then commit

### Never:
- Commit API keys or tokens — secrets guard catches these, and it's correct
- Skip guards with --no-verify for code changes
- Push if guards failed (let CI catch it if you must, but fix locally)
- Commit `.gitreins/tasks.yaml` — it's local task state
