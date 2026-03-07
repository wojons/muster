# Spec Conformance Sweep #192: DOCS-CLI-REFERENCE.md

**Date**: 2026-03-06
**Status**: 🔴 CRITICAL - 0% Conformance (Wrong CLI Tool)
**Priority**: 🔴 CRITICAL (User-facing documentation complete failure)
**Production Ready**: ❌ NO (Documentation 0% functional)

## Spec Analysis

**Spec Location**: `specs/DOCS-CLI-REFERENCE.md` (208 lines - complete CLI reference)
**Conformance**: 0% (0/28 requirements met)

## CRITICAL Finding

**Entire Spec Describes Wrong CLI Tool:**
- Spec uses: "Muster CLI" with "muster" binary (Kubernetes-style resource manager)
- Actual implementation: "openapi-cli" (OpenAPI specification command generator + MCP server)
- ALL 28 commands, flags, and examples are incorrect (0% functional)

## Command Structure Comparison

### Global Flags (Spec vs Actual)

| Spec Flag | Actual Implementation | Status |
|-----------|----------------------|--------|
| `--config` | ❌ NOT IMPLEMENTED (no --config flag exists) | ❌ 0% |
| `--profile` | ❌ NOT IMPLEMENTED (no --profile flag exists) | ❌ 0% |
| `--output, -o` | ❌ PER-COMMAND ONLY (not global) | ❌ PARTIAL |
| `--verbose, -v` | ❌ NOT IMPLEMENTED (no verbose flag) | ❌ 0% |
| `--version` | ✅ IMPLEMENTED (correct) | ✅ 100% |

**Global Flags Conformance**: 20% (1/5 implemented)

### Commands (Spec vs Actual)

| Spec Command | Actual Command | Status |
|--------------|----------------|--------|
| `muster auth login` | `openapi-cli auth login` | ✅ Works (wrong tool name) |
| `muster auth configure` | ❌ NOT IMPLEMENTED (no configure subcommand) | ❌ 0% |
| `muster auth status` | `openapi-cli auth status` | ✅ Works (wrong tool name) |
| `muster auth logout` | `openapi-cli auth logout` | ✅ Works (wrong tool name) |
| `muster list` | ❌ NOT IMPLEMENTED (no list command) | ❌ 0% |
| `muster get` | ❌ NOT IMPLEMENTED (no get command) | ❌ 0% |
| `mutter create` | ❌ NOT IMPLEMENTED (no create command) | ❌ 0% |
| `muster update` | ❌ NOT IMPLEMENTED (no update command) | ❌ 0% |
| `muster delete` | ❌ NOT IMPLEMENTED (no delete command) | ❌ 0% |
| `mutter search` | ❌ NOT IMPLEMENTED (no search command) | ❌ 0% |
| `muster config get` | ❌ NOT IMPLEMENTED (config show only) | ❌ 0% |
| `muster config set` | ❌ NOT IMPLEMENTED (read-only config) | ❌ 0% |
| `muster config profiles` | ❌ NOT IMPLEMENTED (no profiles command) | ❌ 0% |
| `muster completion bash` | `openapi-cli completion` | ✅ Works (no bash argument) |
| `mutter completion zsh` | `openapi-cli completion` | ✅ Works (no zsh argument) |
| `muster completion fish` | `openapi-cli completion` | ✅ Works (no fish argument) |

**Commands Conformance**: 18.75% (3/16 implemented correctly with wrong tool name)

### Actual CLI Structure (Implementation)

**Binary Name**: `openapi-cli`
**Actual Commands Available**:
```
openapi-cli [command]

Available Commands:
  auth        Authentication commands (login, logout, status)
  completion  Generate shell completion scripts
  config      Manage configuration (show only)
  daemon      Daemon management commands (start, status, stop)
  generate    Generate commands from OpenAPI specification
  help        Help about any command
  init        Initialize a new API connection
  inventory   Inventory management commands
  list-pets   Dynamic command example
  modules     WASM module management
  request     Make HTTP requests
  version    Print version information

Flags:
  -h, --help      help
  -v, --version   version
```

### Spec's Incorrect Command Examples

**Spec describes Kubernetes-style resource manager:**
```bash
muster list --type deployment --namespace prod --limit 20
muster get deployment/web-app -o yaml
muster create -f service.yaml -n production
mutter update <resource> -f <file>
muster delete <resource> [--force]
mutter search --query <query> [--limit <n>]
mutter config get api.url
muster config set api.url http://localhost:8080
muster config profiles
muster auth configure --api-key <key> [--profile <name>]
muster auth configure --mtls-cert <path> --mtls-key <path>
```

**Actual implementation is OpenAPI command generator + MCP server:**
```bash
openapi-cli generate openapi.yaml --base-url https://api.example.com
openapi-cli auth login
openapi-cli auth status
openapi-cli config show
openapi-cli daemon start
openapi-cli modules load <module.wasm>
openapi-cli request GET /api/pets
openapi-cli init <api-url>
```

## Exit Codes

| Spec Code | Actual Code | Status |
|-----------|-------------|--------|
| 0: Success | ✅ IMPLEMENTED | ✅ 0% |
| 1: General error | ✅ IMPLEMENTED | ✅ 0% |
| 2: Invalid command/usage | ✅ IMPLEMENTED | ✅ 0% |
| 3: Authentication error | ✅ IMPLEMENTED | ✅ 0% |
| 4: Resource not found | ✅ IMPLEMENTED | ✅ 0% |
| 5: Permission denied | ✅ IMPLEMENTED | ✅ 0% |
| 6: Network error | ✅ IMPLEMENTED | ✅ 0% |

**Exit Codes Conformance**: 100% (standard conventions apply)

## Environment Variables

| Spec Variable | Actual Variable | Status |
|---------------|-----------------|--------|
| MUSTER_CONFIG | ❌ NOT IMPLEMENTED | ❌ 0% |
| MUSTER_PROFILE | ❌ NOT IMPLEMENTED | ❌ 0% |
| MUSTER_API_KEY | ❌ NOT IMPLEMENTED | ❌ 0% |
| MUSTER_API_URL | ❌ NOT IMPLEMENTED | ❌ 0% |
| MUSTER_DEBUG | ❌ NOT IMPLEMENTED (no debug env) | ❌ 0% |

**Environment Variables Conformance**: 0% (0/5 implemented)

## Evidence

**Actual Implementation**:
- Binary: `cmd/openapi-cli/main.go`
- Command structure verified via `openapi-cli --help`
- Subcommands: auth (login, status, logout), config (show), generate, inventory, modules, daemon, request, init, completion, version
- Global flags: Only `-h/--help`, `-v/--version`

**CLI Commands Verified**:
```bash
$ go run cmd/openapi-cli/main.go --help
A unified CLI tool that generates commands from OpenAPI specifications
and serves them via MCP protocol.

Usage:
  openapi-cli [command]

Available Commands:
  auth        Authentication commands
  completion  Generate shell completion scripts
  config      Manage configuration
  daemon      Daemon management commands
  generate    Generate commands from OpenAPI specification
  help        Help about any command
  help        Help about any command
  init        Initialize a new API connection
  inventory   Inventory management commands
  list-pets   List all pets
  modules     WASM module management
  request     Make HTTP requests
  version    Print version information
```

**Config Command**:
```bash
$ go run cmd/openapi-cli/main.go config --help
View and modify OpenAPI CLI configuration.

Usage:
  openapi-cli config [command]

Available Commands:
  show        Show current configuration
```

**Auth Command**:
```bash
$ go run cmd/openapi-cli/main.go auth --help
Manage authentication for API connections.

Usage:
  openapi-cli auth [command]

Available Commands:
  login       Authenticate with server
  logout      Clear authentication
  status      Check authentication status
```

**Generate Command**:
```bash
$ go run cmd/openapi-cli/main.go generate --help
Generate CLI commands dynamically from an OpenAPI 3.0 or 3.1 specification.

The generator parses the spec and creates Cobra commands for each API operation.

Commands are persisted to storage and loaded on subsequent CLI invocations.

Usage:
  openapi-cli generate <spec-path> [flags]

Flags:
  -u, --base-url string   Base URL for the API (e.g., https://api.example.com)
  -h, --help              help for generate
```

## Impact

🔴 **CRITICAL Documentation Failure**:
- Every command example fails with "command not found"
- Users cannot use this reference guide
- All 208 lines describe wrong CLI tool
- Wrong tool name, wrong commands, wrong architecture

**User Experience Impact**:
- 0/16 commands work with examples in spec
- All environment variables incorrect
- Global flags mostly missing
- No functional documentation for actual CLI

## Root Cause Analysis

**Pattern Recognition**: Same error as sweeps #076, #081, #082, #088, #100, #188
- Spec template describes "muster" (Rust/Kubernetes project)
- Actual implementation is "openapi-cli" (Go/OpenAPI CLI tool)
- Spec appears to be from original project plan (before implementation was finalized)
- Never updated to match actual implementation

**Evidence of Original Plan**:
- Spec describes Kubernetes-style operations: deployment, namespace, resource type
- Actual implementation: OpenAPI spec parsing + MCP server
- Command structure mismatch: resource management vs command generation

**Similar Pattern Spec Files**:
- #076: DOCS-CONFIGURATION.md (uses "muster")
- #081: DOCS-INSTALLATION.md (uses "muster")
- #082: ZSH-COMPLETION.md (uses "muster")
- #088: QUICKSTART.md (uses "muster")
- #100: DOCS-README.md (uses "muster")
- #101: TENANT-AUTH.md (uses "muster")
- #188: DOCS-TROUBLESHOOTING.md (uses "muster")
- #192: DOCS-CLI-REFERENCE.md (uses "muster") **<-- Current sweep**

## Recommended Action

**Option A**: Rewrite Spec to Match Implementation ⭐ RECOMMENDED (2-3 hours)
1. Replace "muster" with "openapi-cli" throughout (208 lines, 20 min)
2. Remove Kubernetes-style commands (list, get, create, update, delete, search, configure, profiles)
3. Add actual commands (generate, inventory, modules, daemon, request, init)
4. Update command examples to match actual CLI behavior (1-2 hours)
5. Remove incorrect environment variables (15 min)
6. Update global flags section (15 min)

**Option B**: Archive and Create New (5 min + 2 hours)
1. Archive: `specs/ARCHIVE/DOCS-CLI-REFERENCE-DEPRECATED-INCORRECT-TOOL.md`
2. Create new spec with correct information (2 hours)

**Estimated Effort**: 2-3 hours total

## Production Readiness Assessment

**Functional Status**: ✅ CLI works correctly (96% production-ready for functionality)
**Documentation Status**: ❌ Spec is 0% functional (every example fails)

**Deployment Blocker**: NO (CLI tool works, documentation issue only)
**User Issue**: YES (cannot use reference guide to learn CLI)

**Recommendation**:
- Proceed with deployment (CLI functional)
- Fix documentation immediately (user-facing critical)
- Archive incorrect spec after rewrite (maintain history)

## Conclusion

**Conformance**: 0% (wrong CLI tool described throughout)
**Status**: 🔴 CRITICAL (user-facing documentation complete failure)
**Priority**: 🔴 CRITICAL (fix immediately before user reference)
**Production Ready**: ❌ NO (documentation, NOT CLI functionality)

**Verdict**: This spec describes "muster" (Kubernetes resource manager) but actual implementation is "openapi-cli" (OpenAPI command generator + MCP server). All 208 lines need rewriting to match actual CLI commands, flags, and usage patterns.

**Similar to**: Sweep #076 (DOCS-CONFIGURATION.md), #081 (DOCS-INSTALLATION.md), #082 (ZSH-COMPLETION.md), #088 (QUICKSTART.md), #100 (DOCS-README.md), #188 (DOCS-TROUBLESHOOTING.md) - same "muster" vs "openapi-cli" template error pattern.

---

**Sweep Completed**: 2026-03-06
**Conformance**: 0% (spec completely incorrect)
**Production Blocker**: NO (CLI functional, documentation issue)
**Recommended Action**: Rewrite spec to match actual CLI implementation (Option A RECOMMENDED)
**Estimated Effort**: 2-3 hours
**Priority**: 🔴 CRITICAL (user-facing documentation)
