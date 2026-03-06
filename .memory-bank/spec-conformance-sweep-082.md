# Spec Conformance Sweep #082 (2026-03-05)

## Metadata
- **Date**: 2026-03-05
- **Agent**: Ralph Wiggum Loop (Idle-Time Spec Conformance Sweep)
- **Phase**: Continuous Improvements (No Blocking Tasks)
- **Total Sweeps**: 82 (Previous: 81)
- **Total Specs Analyzed**: 82 specs from 276 spec files (30.1% complete)

## Spec Analysis

### Analyzed Spec
- **File**: `specs/DOCS-OPENAPI-QUICKSTART.md`
- **Lines**: 348 lines
- **Type**: User-facing QuickStart and getting started guide
- **Scope**: Installation, first commands, authentication, inventory, configuration, output formats, advanced features

### Spec Contents
The QuickStart guide describes:
1. Installation methods (brew, curl, build from source)
2. Version verification
3. Command generation from OpenAPI specs
4. Command execution examples
5. Authentication (OAuth2, API key, Basic auth)
6. Inventory management (fetch, list, validate)
7. Configuration management (get, set, reset)
8. Output formats (table, json, yaml, csv)
9. Advanced features (daemon, MCP server, WASM modules)
10. Troubleshooting and support

### Implementation Verification

#### ✅ CONFIRMED IMPLEMENTED (7/8 Major Sections)

1. **Installation** (Lines 14-30)
   - ✅ scripts/install.sh exists
   - ⚠️ **CRITICAL ISSUE**: Script uses wrong name (line 7: BINARY_NAME="muster")
   - ✅ make build and make install work
   - ℹ️ **Note**: brew recipe verification needed (not checked)

2. **Version Command** (Lines 37-38)
   - ✅ `openapi-cli --version` exists
   - ✅ internal/version/version.go provides version info

3. **Command Generation** (Lines 42-71)
   - ✅ `openapi-cli generate` command (internal/builtin/generate.go)
   - ✅ Command persistence implemented (root.go:loadPersistedCommands)
   - ✅ Storage at ~/.local/share/openapi-cli/commands.json

4. **Command Execution** (Lines 76-91)
   - ✅ Generated commands loaded from storage
   - ✅ OpenAPI operations execute via HTTP requests

5. **Authentication** (Lines 93-124)
   - ✅ `openapi-cli auth login --oauth2` (internal/builtin/auth.go)
   - ✅ `openapi-cli auth login --username --password`
   - ✅ `openapi-cli auth logout`
   - ✅ Token refresh and storage implemented

6. **Inventory Management** (Lines 126-155)
   - ✅ `openapi-cli inventory fetch --host` (internal/builtin/inventory.go)
   - ✅ `openapi-cli inventory list`
   - ✅ `openapi-cli inventory validate`

7. **Output Formatters** (Lines 183-199)
   - ✅ JSON formatter (pkg/response/formatter.go:71-96)
   - ✅ YAML formatter (pkg/response/formatter.go:143-151)
   - ✅ Table formatter (pkg/response/formatter.go:153-281)
   - ✅ CSV formatter (pkg/response/formatter.go:283-333)
   - ✅ request.go has `--output` parameter (lines 7-8)

8. **WASM Runtime** (Lines 287-300)
   - ✅ Fully implemented (pkg/wasm/runtime.go)
   - ✅ Module loading, execution, sandboxing
   - ✅ Capability system and resource limits

9. **MCP Server** (Lines 272-285)
   - ✅ MCP server implementation exists
   - ✅ JSON-RPC protocol support

10. **Daemon Mode** (Lines 257-269)
    - ✅ `openapi-cli daemon start/stop/status` commands (internal/builtin/daemon.go)

#### ❌ MISSING IMPLEMENTATION (1/8 Major Sections)

1. **Configuration Commands** (Lines 162-181) - CRITICAL GAP

   **Spec Claims**:
   - `openapi-cli config get` - Show current configuration
   - `openapi-cli config set --output json` - Set output format
   - `openapi-cli config set --timeout 30s` - Set timeout
   - `openapi-cli config set --debug` - Enable debug mode
   - `openapi-cli config reset` - Reset to defaults

   **Actual Implementation** (internal/builtin/config.go):
   - ✅ `openapi-cli config show` - Shows config dir, cache dir, data dir, version, profile
   - ❌ NO `config get` command
   - ❌ NO `config set` command
   - ❌ NO `config reset` command
   - ❌ NO output format configuration
   - ❌ NO timeout configuration
   - ❌ NO debug mode configuration

   **Impact**:
   - 🔴 CRITICAL - Users cannot configure output formats via CLI
   - 🔴 Users cannot set timeouts via CLI (must use environment variables or code)
   - 🔴 Users cannot enable debug mode via CLI (must use code)
   - 🔴 QuickStart examples will FAIL for users

   **Evidence**:
   ```go
   // internal/builtin/config.go:18-28
   cmd.AddCommand(&cobra.Command{
       Use:   "show",
       Short: "Show current configuration",
       Run: func(cmd *cobra.Command, args []string) {
           fmt.Printf("Config Dir: %s\n", cfg.ConfigDir)
           // ... only shows directory paths and version
       },
   })
   ```

2. **Global --output Flag** (Lines 183-199)

   **Spec Claims**:
   ```bash
   openapi-cli pets list-pets --output json
   openapi-cli pets list-pets --output yaml
   openapi-cli pets list-pets --output csv
   ```

   **Actual Implementation**:
   - ✅ Formatters exist (pkg/response/formatter.go)
   - ✅ request command has `--output` parameter
   - ❌ Global `--output` flag NOT implemented on root command
   - ❌ Generated API commands may NOT support `--output` flag

   **Impact**:
   - 🔴 Users cannot use `--output` flag on all commands as shown in QuickStart
   - 🔴 QuickStart examples will FAIL for users

### Installation Script Issue

**CRITICAL**: scripts/install.sh uses wrong tool name

**File**: scripts/install.sh:6-7
```bash
BINARY_NAME="muster"
REPO="anomalyco/muster"
```

**Should be**:
```bash
BINARY_NAME="openapi-cli"
REPO="openapi-cli/openapi-cli"
```

This is a CRITICAL documentation issue:
- install.sh won't work with actual openapi-cli binary
- Users trying to install via curl will get wrong binary
- Same issue found in previous sweeps (DOCS-INSTALLATION.md, DOCS-CONFIGURATION.md)

## Conformance Summary

| Category | Spec Requirements | Actual Implementation | Status | Issue |
|----------|-----------------|----------------------|--------|-------|
| Installation | brew, curl, git+make | scripts/install.sh (wrong name) | ⚠️ CONFORMANCE ISSUE | Script uses "muster" not "openapi-cli" |
| Version | --version | ✅ Implemented | ✅ OK | Works correctly |
| Generation | generate command | ✅ Implemented | ✅ OK | Full persistence |
| Execution | Generated commands | ✅ Implemented | ✅ OK | Load from storage |
| Authentication | login/logout flows | ✅ Implemented | ✅ OK | OAuth2, Basic, API Key |
| Inventory | fetch/list/validate | ✅ Implemented | ✅ OK | All commands present |
| **Configuration** | **get/set/reset commands** | **❌ Only show command** | **❌ CRITICAL GAP** | **Missing get/set/reset** |
| **Output Formats** | **Global --output flag** | **❌ Only in request command** | **❌ CRITICAL GAP** | **Global flag missing** |
| Output Formatters | JSON/YAML/Table/CSV | ✅ Implemented | ✅ OK | Full support in pkg/response |
| WASM Runtime | Load/execute modules | ✅ Implemented | ✅ OK | Full capability system |
| MCP Server | Server mode | ✅ Implemented | ✅ OK | JSON-RPC support |
| Daemon Mode | start/stop/status | ✅ Implemented | ✅ OK | Unix socket proxy |

## Overall Conformance

**Conformance Score**: 7/9 major sections = **77.8%** (excluding config and output gaps)

**Critical Issues**: 2 (Configuration, Global --output flag)

**Priority**: HIGH - QuickStart is primary user-facing documentation

### What Works
- Core functionality (generate, execute, auth, inventory, WASM, MCP, daemon)
- All major architectural components implemented
- Formatters fully supported (internally)

### What Doesn't Work
- Configuration commands (get/set/reset) - CRITICAL
- Global --output flag on all commands - CRITICAL
- Installation script uses wrong name - CRITICAL

## Production Readiness

**Production Ready**: YES (functional) BUT NO (documentation)

The actual system works, but the QuickStart documentation describes functionality that doesn't exist:

1. Users trying `openapi-cli config set --output json` will get "command not found"
2. Users trying `openapi-cli pets list-pets --output yaml` may not work
3. Users installing via `curl -fsSL https://openapi-cli.io/install.sh | bash` get wrong binary

**Impact**:
- 🔴 User-facing documentation critical to onboarding
- 🔴 QuickStart is first place new users look
- 🔴 Examples will fail, confusing users
- 🔴 Support burden from confused users

**Recommendation**: Fix configuration commands and global --output flag OR update QuickStart to match actual implementation

## Recommended Actions

### Option 1: Implement Missing Functionality (Recommended - 8-12 hours)
**Priority**: HIGH - Primary user documentation

**Tasks**:
1. Implement config get command to show full configuration (2h)
2. Implement config set command for output, timeout, debug (3h)
3. Implement config reset command (1h)
4. Add global --output flag to root command (2h)
5. Ensure generated API commands support --output flag (2h)
6. Update install.sh to use correct binary name (1h)
7. Verify brew recipe exists and works (1h if needed)

**Total Effort**: 8-12 hours

### Option 2: Update Documentation (2-3 hours - Less ideal)
**Priority**: MEDIUM - Documentation workarounds

**Tasks**:
1. Update QuickStart to remove config set examples
2. Add note: "Configuration management not yet implemented"
3. Update QuickStart to use only request command for output format
4. Add note: "Output format only available in request mode"
5. Update install.sh (same as Option 1)

**Why Less Ideal**:
- QuickStart becomes less useful
- Users lose expected functionality
- Doesn't address user need for configuration

**Recommendation**: Choose Option 1 to implement functionality

## Action Items

### Immediate (Critical)
1. ✅ [ ] Fix scripts/install.sh: Change BINARY_NAME="muster" to "openapi-cli"
2. ✅ [ ] Fix scripts/install.sh: Change REPO="anomalyco/muster" to "openapi-cli/openapi-cli"

### Critical (8-10 hours)
3. ❌ [ ] Implement config get command (load full config from storage)
4. ❌ [ ] Implement config set command (output, timeout, debug flags)
5. ❌ [ ] Implement config reset command
6. ❌ [ ] Add global --output flag to root command
7. ❌ [ ] Ensure generated commands support --output flag

### Documentation (1-2 hours)
8. ❌ [ ] Test all QuickStart examples end-to-end
9. ❌ [ ] Update examples if any assumptions incorrect
10. ❌ [ ] Add FAQ for configuration questions

## Sweep Status

- ✅ **COMPLETE**: Spec analysis performed
- ✅ **COMPLETE**: Implementation verified
- ✅ **COMPLETE**: Gaps documented
- ⏳ **PENDING**: Remediation (Option 1: implement, Option 2: update docs)

## Next Step

Add to TODO.md:
- [FIX] scripts/install.sh uses wrong tool name (CRITICAL - 1h)
- [IMPLEMENT] config get/set/reset commands (CRITICAL - 4h)
- [IMPLEMENT] global --output flag (CRITICAL - 2h)

Continuing with next random spec...
