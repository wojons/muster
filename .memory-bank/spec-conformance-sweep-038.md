# Spec Conformance Sweep #038: QUICK-START.md

**Sweep Date**: 2026-03-05
**Spec Analyzed**: specs/QUICK-START.md (273 lines)
**Actual Implementation**: Go-based CLI/MCP system (cmd/, pkg/, internal/ directories)
**Conformance Rating**: ❌ 0% (COMPLETELY OUT OF DATE - Wrong Technology Stack)

---

## Executive Summary

The spec QUICK-START.md describes a **TypeScript/JavaScript** based project with npm build system. The actual muster implementation is a **Go-based** project with Makefile build system. This is a complete technology stack mismatch requiring full spec rewrite.

**CRITICAL FINDING**:
- Spec describes TypeScript/JavaScript architecture
- Actual implementation is Go (Golang) architecture
- NO functionality matches spec requirements

**Impact**:
- 🔴 **Documentation Impact**: HIGH - Quick start guide is completely unusable for Go project
- 🟡 **Functional Impact**: N/A - Spec is purely documentation
- 🟢 **Code Impact**: N/A - No code changes, just documentation update required

**Recommendation**: ⚠️ Rewrite QUICK-START.md to reflect Go-based architecture

---

## Tech Stack Mismatch

### Spec Requirements (TypeScript/JavaScript)
**Project Structure** (lines 6-22):
```
src/
├── cli/              # CLI entry point and commands
│   ├── index.ts      # TypeScript entry
│   └── commands/     # Dynamic command handlers
├── mcp/              # MCP server implementation
│   ├── index.ts      # MCP entry point
│   └── tools.ts      # Tool generation from OpenAPI
├── core/             # Shared core (both CLI and MCP use this)
│   ├── inventory/    # Inventory fetching and caching
│   ├── config/       # Configuration management
│   ├── auth/         # Authentication handlers
│   ├── crypto/       # Cryptographic operations
│   ├── wasm/         # WASM runtime
│   ├── dsl/          # DSL interpreter
│   └── http/         # HTTP client with middleware
└── types/            # TypeScript type definitions
```

**Build System** (lines 27-39):
- `npm install` - Install dependencies
- `npm run build` - Build
- `./dist/cli.js` - Compiled JavaScript output
- `./dist/mcp.js` - Compiled JavaScript output

**Environment Variables** (lines 44-52):
- `OPENAPICLI_HOST`
- `OPENAPICLI_INVENTORY_URL`
- `OPENAPICLI_CERT_FILE`
- `OPENAPICLI_KEY_FILE`
- `OPENAPICLI_CA_FILE`
- `OPENAPICLI_CONFIG_FILE`

**Testing** (lines 58-66):
- `npm test` - Unit tests
- `npm run test:integration` - Integration tests
- `npm run test:mock` - Mock inventory tests

**Installation** (lines 73-81):
- `npm install -g @openapi-cli/core` - Via npm
- `npx @openapi-cli/core` - Via npx
- Binary download from releases

**MCP Setup** (lines 129-147):
- `"command": "npx"`
- `"@openapi-cli/core"`
- Config uses npm package naming

**Package Name**: `@openapi-cli/core` (lines 74, 137)

### Actual Implementation (Go/Golang)
**Project Structure** (verified from muster directory):
```
muster/
├── cmd/                   # Entry points
│   ├── openapi-cli/      # CLI application
│   ├── mcp-server/       # MCP server
│   └── daemon/           # Background daemon
├── pkg/                   # Public APIs (26+ packages)
│   ├── auth/            # Authentication
│   ├── cache/           # Caching
│   ├── cli/             # CLI framework
│   ├── client/          # HTTP client
│   ├── cmdstore/        # Command storage
│   ├── config/          # Configuration
│   ├── daemon/          # Daemon client/controller
│   ├── dsl/             # Starlark DSL
│   ├── generator/       # Command generation
│   ├── inventory/       # Inventory fetching & caching
│   ├── log/             # Structured logging
│   ├── mcp/             # MCP protocol
│   ├── openapi/         # OpenAPI parsing
│   ├── prompt/          # Interactive prompts
│   ├── rate/            # Rate limiting
│   ├── repository/      # Database layer (SQLite)
│   ├── request/         # HTTP request building
│   ├── response/        # HTTP response handling
│   └── wasm/            # WASM runtime
├── internal/             # Internal utilities
│   ├── builtin/         # Built-in commands
│   ├── commands/        # Command implementations
│   ├── compliance/      # Compliance verification
│   ├── database/        # Database connections (empty)
│   ├── middleware/      # HTTP middleware
│   ├── monitoring/      # Metrics & monitoring
│   ├── perf/            # Performance utilities
│   ├── platform/        # Platform abstractions
│   ├── rendering/       # Output rendering
│   ├── security/        # Security utilities
│   ├── storage/         # Storage abstractions
│   ├── telemetry/       # Telemetry & tracing
│   └── testvectors/     # Test vectors
├── specs/                # 276 specification files
├── demo/                 # Demo application and tests
├── .agents/skills/       # 63 specialized agent skills
└── .memory-bank/         # Project context and decisions
```

**Build System** (verified from Makefile):
- `make build` - Build main binary
- `make build-cli` - Build CLI only
- `make build-mcp` - Build MCP server only
- `go build ./cmd/...` - Go build commands
- Output: `bin/openapi-cli`, `bin/mcp-server`

**Environment Variables** (Go project conventions):
- No `OPENAPICLI_*` prefix used
- Uses standard environment variable handling (Viper library)
- Config file: `~/.config/muster/config.yaml` or similar

**Testing** (Go project):
- `go test -v ./...` - Run all unit tests
- `make test-unit` - Unit tests
- `make test-integration` - Integration tests
- `make test-e2e` - End-to-end tests
- `make demo` - Run demo application

**Installation**:
- `go install github.com/anomalyco/muster/cmd/openapi-cli@latest`
- Binary download from GitHub releases
- Homebrew (if formula exists)

**MCP Setup** (Go binary):
```json
{
  "mcpServers": {
    "muster": {
      "command": "/path/to/mcp-server",
      "args": ["--host", "https://api.example.com"]
    }
  }
}
```

**Package Name**: `github.com/anomalyco/muster`

---

## Command Mismatch

### Spec Commands (TypeScript/JavaScript)
```bash
# Core commands from spec (lines 87-196)
openapi-cli --host https://api.example.com --help
openapi-cli users list
openapi-cli users get --id 123
openapi-cli --host https://api.example.com use prod
openapi-cli profiles list
openapi-cli versions --cached
openapi-cli alias create prod https://api.example.com
# ... many more
```

### Actual Commands (Go)
```bash
# CLI structure (from cmd/openapi-cli)
openapi-cli --help
openapi-cli [command] [flags]
# Binary is at bin/openapi-cli

# Demo application
make demo

# MCP server
mcp-server --help
# Binary at bin/mcp-server

# Note: Actual command syntax differs from spec
```

---

## Complete Mismatch Summary

| Aspect | Spec (TypeScript) | Actual (Go) |
|--------|------------------|-------------|
| Project Structure | `src/cli/index.ts` | `cmd/openapi-cli/main.go` |
| Build System | npm/node | go build / make |
| Output Files | `./dist/cli.js` | `bin/openapi-cli` |
| Package Manager | npm/v | modules/go get |
| Dependencies | package.json | go.mod |
| Testing Framework | npm test | go test |
| Environment Variables | OPENAPICLI_* | Standard Go env vars |
| Installation | npm install -g | go install / Homebrew |
| Package Name | @openapi-cli/core | github.com/anomalyco/muster |
| MCP Server Command | `"@openapi-cli/core"` | `/path/to/bin/mcp-server` |
| Command Syntax | `openapi-cli [args]` | Likely `openapi-cli [subcommand] [args]` |

---

## Required Actions

### Priority 0 (CRITICAL - Documentation Update)

**Rewrite QUICK-START.md for Go-based implementation**

Include sections:
1. **For Developers**
   - Go project structure (cmd/, pkg/, internal/)
   - Makefile build targets
   - Go toolchain setup
   - Testing commands (go test)

2. **For Users**
   - Go-based installation (go install / Homebrew / binary download)
   - Configuration (`~/.config/muster/config.yaml`)
   - Command syntax differences

3. **Environment Variables**
   - Actual environment variable naming
   - Actual config file location

4. **MCP Setup**
   - Go binary configuration (not npm/npx)
   - Correct command paths

5. **Testing**
   - Go test commands
   - Makefile test targets

---

## Impact Assessment

**Code Impact**: None - Only documentation needs updating

**Functional Impact**: None - Actual Go implementation works correctly

**User Impact**: **HIGH** - New users cannot follow quick start guide

**Developer Impact**: **HIGH** - New developers cannot follow development guide

**Documentation Impact**: CRITICAL - Quick start guide is 100% unusable

---

## Recommendation

**Option A**: Rewrite QUICK-START.md (Recommended)
- ✅ Provides accurate quick start for Go implementation
- ✅ Low effort (documentation update only)
- ✅ High value for users/developers
- ⏱️ Estimated: 2-4 hours

**Option B**: Deprecate QUICK-START.md (Not Recommended)
- ❌ Removes helpful quick start guide
- ❌ Increases barrier to entry
- ⚠️ Users have to find other documentation

**Option C**: Keep Spec But Document Mismatch (Not Recommended)
- ❌ Confuses new users
- ❌ Technical debt
- ❌ Creates maintenance burden

---

## Decision

**Recommended**: **Option A - Rewrite QUICK-START.md**

**Rationale**:
1. Spec is completely out of date (TypeScript vs Go)
2. No code changes required (documentation only)
2. High user/developer value
2. Low effort (2-4 hours estimated)
3. Critical for onboarding new users/developers

---

## Files Referenced

**Spec**:
- specs/QUICK-START.md (273 lines) - TypeScript/JavaScript quick start (OUTDATED)

**Actual Implementation** (Go-based):
- AGENTS.md - Project context and agent guidelines
- Makefile - Build system (Go commands)
- go.mod - Go module definition
- cmd/openapi-cli/ - CLI entry point
- cmd/mcp-server/ - MCP server entry point
- pkg/repository/ - Repository implementation (SQLite)
- pkg/wasm/ - WASM runtime
- pkg/mcp/ - MCP protocol
- pkg/auth/ - Authentication

---

## Next Steps

1. **Rewrite QUICK-START.md** (2-4 hours)
   - Update project structure section for Go
   - Update build commands for Makefile/go build
   - Update testing commands for go test
   - Update installation for go install/Homebrew
   - Update MCP setup for Go binary
   - Update environment variables for Go conventions
   - Update package name to github.com/anomalyco/muster

2. **Verify Other Quick Start Documentation**
   - Check docs/guides/quickstart.md if exists
   - Check docs/guides/installation.md if exists
   - Ensure all quick start docs are Go-based

3. **Review Other Spec Documentation**
   - Check if other specs have similar tech stack issues
   - Identify other documentation requiring updates

4. **Update Project Context**
   - Record decision (Go-based vs TypeScript) in Memory Bank
   - Document architectural decision rationale
   - Update AGENTS.md or tech-decisions.md if needed

---

## Conformance Summary

| Aspect | Spec (TypeScript) | Actual (Go) | Conformance |
|--------|------------------|-------------|-------------|
| Project Structure | src/ | cmd/, pkg/, internal/ | ❌ 0% |
| Build System | npm | go build/make | ❌ 0% |
| Output Files | ./*.js | bin/* | ❌ 0% |
| Package Name | @openapi-cli/core | github.com/anomalyco/muster | ❌ 0% |
| Installation | npm install | go install | ❌ 0% |
| Environment Variables | OPENAPICLI_* | (not specified) | ❌ 0% |
| Testing | npm test | go test | ❌ 0% |
| MCP Command | npx @openapi-cli/core | /path/to/mcp-server | ❌ 0% |

**Overall Conformance**: 0% (0/8 aspects match)

**Documentation Status**: 🔴 CRITICAL - Quick start completely unusable
**Blocker**: Documentation only (no code changes needed)
**Timeline**: 2-4 hours for rewrite

---

## Decision Point

**Architecture Decision Required**: Record decision on Go-based vs TypeScript-based architecture

**Actual Implementation**: Go (Golang) with Makefile build system
**Spec Description**: TypeScript/JavaScript with npm build system

**Historical Context**: Spec may have been written before Go implementation was finalized, or spec was copied from a different project template

**Action Required**: ✅ Update spec to match actual Go implementation (documentation update only)
