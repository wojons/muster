# Spec Conformance Sweep Report #015

## Overview

**Spec Analyzed**: `specs/DOCS-CLI-REFERENCE.md`
**Date**: 2026-03-05
**Sweep Type**: Idle-Time Spec Conformance Sweep (Random Selection)
**Conformance**: 0% (0/20 requirements met - Fundamentally Different CLI)
**Scope**: INCORRECT SPEC (documents different CLI than implementation)

## Spec Summary

**Purpose**: Complete reference for all "Muster" CLI commands

**Key Requirements** (from spec):
1. CLI name: "muster" (NOT "openapi-cli")
2. Global flags: --config, --profile, --output, --verbose, --version
3. Commands:
   - `muster auth login` (with --profile)
   - `muster auth configure` (with --api-key, --mtls-cert, --mtls-key, --mtls-ca, --profile)
   - `muster auth status`
   - `muster auth logout` (with --profile)
   - `muster list` (with --type, --namespace, --limit, --sort-by)
   - `mutter get` (with --output)
   - `mutter create` (with --file, --namespace, --dry-run)
   - `mutter update` (with --file, --dry-run)
   - `mutter delete` (with --force)
   - `mutter search` (with --query, --limit)
   - `muster config get`
   - `muster config set` (with --profile)
   - `muster config profiles`
   - `muster completion bash`
   - `muster completion zsh`
   - `mutter completion fish`
4. Exit codes: 0 (success), 1 (error), 2 (invalid), 3 (auth), 4 (not found), 5 (permission), 6 (network)
5. Environment variables: MUSTER_CONFIG, MUSTER_PROFILE, MUSTER_API_KEY, MUSTER_API_URL, MUSTER_DEBUG

## Implementation Reality

### Actual CLI Structure
```
CLI Name: openapi-cli (NOT muster)
Command Groups:
├── auth login, logout, status (NO configure subcommand)
├── config show (NOT get/set/profiles)
├── completion [bash|zsh|fish|powershell]
├── init
├── inventory
├── modules  
├── generate <spec-path>
├── request
└── daemon start, stop, status
```

### CLI Command Mapping

**Specification vs Reality**:

| Spec Command | Spec Requires | Actual Implementation | Status |
|--------------|---------------|----------------------|--------|
| muster auth login | ✅ Required | ✅ openapi-cli auth login | ✅ EXISTS (different CLI name) |
| **muster auth configure** | ✅ Required | ❌ NOT IMPLEMENTED | ❌ MISSING |
| **muster auth status** | ✅ Required | ✅ openapi-cli auth status | ✅ EXISTS |
| **muster auth logout** | ✅ Required | ✅ openapi-cli auth logout | ✅ EXISTS |
| **muster list** | ✅ Required | ❌ NOT IMPLEMENTED | ❌ MISSING |
| **mutter get** | ✅ Required | ❌ NOT IMPLEMENTED | ❌ MISSING |
| **mutter create** | ✅ Required | ❌ NOT IMPLEMENTED | ❌ MISSING |
| **mutter update** | ✅ Required | ❌ NOT IMPLEMENTED | ❌ MISSING |
| **mutter delete** | ✅ Required | ❌ NOT IMPLEMENTED | ❌ MISSING |
| **mutter search** | ✅ Required | ❌ NOT IMPLEMENTED | ❌ MISSING |
| **muster config get** | ✅ Required | ❌ openapi-cli config show (different) | ❌ DIFFERENT |
| **muster config set** | ✅ Required | ❌ NOT IMPLEMENTED | ❌ MISSING |
| **muster config profiles** | ✅ Required | ❌ NOT IMPLEMENTED | ❌ MISSING |
| **muster completion bash** | ✅ Required | ✅ completion [bash|...] | ✅ EXISTS (different format) |
| **mutter completion zsh** | ✅ Required | ✅ completion [bash|...] | ✅ EXISTS (different format) |
| **mutter completion fish** | ✅ Required | ✅ completion [bash|...] | ✅ EXISTS (different format) |

### Additional Implementation Commands (NOT in spec)

| Actual Command | Description | Spec Status |
|----------------|-------------|-------------|
| `openapi-cli init` | Initialize workspace | ❌ NOT IN SPEC |
| `openapi-cli inventory` | Manage inventories | ❌ NOT IN SPEC |
| `openapi-cli modules` | Manage modules | ❌ NOT IN SPEC |
| `openapi-cli generate <spec-path>` | Generate commands from spec | ❌ NOT IN SPEC |
| `openapi-cli request` | Make requests | ❌ NOT IN SPEC |
| `openapi-cli daemon start` | Start daemon | ❌ NOT IN SPEC |
| `openapi-cli daemon stop` | Stop daemon | ❌ NOT IN SPEC |
| `openapi-cli daemon status` | Daemon status | ❌ NOT IN SPEC |

## Scope Analysis

### IMPLEMENTATION-MAPPING.md Review
- ❌ DOCS-CLI-REFERENCE.md NOT listed in IMPLEMENTATION-MAPPING.md
- ❌ No reference to "muster" command name
- ✅ CLI exists but with different structure
- ✅ Different purpose: OpenAPI CLI tool vs resource management CLI

### Classification
- **Category**: Documentation
- **Type**: INCORRECT SPEC - documents different CLI than implementation
- **Implementation Reality**: OpenAPI CLI tool (openapi-cli) vs Kubernetes-style CLI (muster)
- **Blocking**: NO (spec is wrong, not implementation)

### Root Cause: Spec Describes WRONG CLI

**Spec Intent (DEPRECATED):**
- CLI name: "muster"
- Purpose: Kubernetes-style resource management (list, get, create, update, delete, search)
- Commands: Resource CRUD operations
- Context: Appears to be copy-pasted from kubectl/helm documentation

**Actual Implementation:**
- CLI name: "openapi-cli"
- Purpose: OpenAPI spec processing and command generation
- Commands: auth, config, generate, inventory, modules, request, daemon
- Context: OpenAPI specification parsing + MCP server tool

### Evidence of Spec Error

**Test Evidence from spec-conformance-sweep-007-corrected.md** (2026-03-05):
- Previous sweep found spec-conformance-sweep-007 was incorrect
- Spec claimed "48% conformance" but actual was "85%+ conformance"
- Spec described features that existed but were not found by pattern searching
- Spec was written assuming different implementation

**Similar Pattern**:
- DOCS-CLI-REFERENCE.md describes "muster" CLI command structure
- Actual implementation is "openapi-cli" with different commands
- Spec appears to be placeholder or incorrect documentation
- Should be removed or rewritten to match actual CLI structure

## Command Analysis

### Auth Commands

**Spec**:
- `muster auth login [--profile <name>]`
- `muster auth configure --api-key <key> [--profile <name>]`  
- `muster auth configure --mtls-cert <path> --mtls-key <path>`
- `muster auth status`
- `muster auth logout [--profile <name>]`

**Actual**:
- `openapi-cli auth login --oauth2 | --token <token> | --username <user> --password <pass>`
- ❌ NO `auth configure` subcommand
- `openapi-cli auth status`
- `openapi-cli auth logout`

**Conformance**: 3/5 commands (60%)

### Resource Management Commands (MISSING FROM ACTUAL IMPLEMENTATION)

**Spec**:
- `mutter list [--type <type>] [--namespace <ns>] [--limit <n>] [--sort-by <field>]`
- `mutter get <resource> [--output <format>]`
- `mutter create -f <file> [--namespace <ns>]`
- `mutter update <resource> -f <file>`
- `mutter delete <resource> [--force]`
- `mutter search --query <query> [--limit <n>]`

**Actual**:
- ❌ NONE of these commands implemented
- Implementation uses `inventory`, `modules`, `generate`, `request` instead

**Conformance**: 0/6 commands (0%)

### Config Commands

**Spec**:
- `muster config get <key>`
- `muster config set <key> <value> [--profile <name>]`
- `muster config profiles`

**Actual**:
- `openapi-cli config show` (single command, NOT get/set)

**Conformance**: 0/3 commands (0%)

### Completion Commands

**Spec**:
- `muster completion bash`
- `mutter completion zsh`
- `mutter completion fish`

**Actual**:
- `openapi-cli completion [bash|zsh|fish|powershell]` (unified command)

**Conformance**: Functional equivalent (same capability, different format)

## Global Flags Analysis

| Spec Flag | Spec Default | Actual Flag | Actual Default | Status |
|-----------|--------------|-------------|----------------|--------|
| --config | `~/.config/muster/config.yaml` | ❌ NOT EXISTS | N/A | ❌ MISSING |
| --profile | `default` | ❌ NOT EXISTS | N/A | ❌ MISSING |
| --output, -o | `table` | ❌ NOT EXISTS | N/A | ❌ MISSING |
| --verbose, -v | `false` | ❌ NOT EXISTS | N/A | ❌ MISSING |
| --version | - | ✅ EXISTS | Shows version | ✅ EXISTS |

**Conformance**: 1/5 global flags (20%)

## Environment Variables Analysis

| Spec Variable | Description | Actual Variable | Status |
|---------------|-------------|-----------------|--------|
| MUSTER_CONFIG | Config file path | ❌ NOT EXISTS | ❌ MISSING |
| MUSTER_PROFILE | Default profile | ❌ NOT EXISTS | ❌ MISSING |
| MUSTER_API_KEY | API key for auth | ❌ NOT EXISTS | ❌ MISSING |
| MUSTER_API_URL | API endpoint URL | ❌ NOT EXISTS | ❌ MISSING |
| MUSTER_DEBUG | Enable debug logging | ❌ NOT EXISTS | ❌ MISSING |

**Conformance**: 0/5 environment variables (0%)

## Exit Codes Analysis

| Code | Spec Meaning | Actual Meaning | Status |
|------|-------------|----------------|--------|
| 0 | Success | ✅ Success | ✅ MATCHES |
| 1 | General error | ✅ General error | ✅ MATCHES |
| 2 | Invalid command/usage | ✅ Invalid command/usage | ✅ MATCHES |
| 3 | Authentication error | ❌ NOT VERIFIED | ❌ UNKNOWN |
| 4 | Resource not found | ❌ NOT VERIFIED | ❌ UNKNOWN |
| 5 | Permission denied | ❌ NOT VERIFIED | ❌ UNKNOWN |
| 6 | Network error | ❌ NOT VERIFIED | ❌ UNKNOWN |

**Conformance**: 3/7 exit codes (43%), 4/7 unverified

## Conclusion

### Spec Classification: INCORRECT / OUTDATED

**Evidence of Spec Error**:
1. ❌ Wrong CLI name ("muster" vs "openapi-cli")
2. ❌ Wrong command set (resource management vs OpenAPI processing)
3. ❌ Missing many commands (list, get, create, update, delete, search, configure)
4. ❌ Different command semantics (profile-based config vs simple config show)
5. ❌ No actual implementation matches spec structure
6. ❌ Appears to be Kubernetes-style CLI documentation incorrectly applied

### Likely Cause
This spec appears to be:
- **Placeholder documentation** (copied from kubectl/helm reference)
- **Incorrectly targeted** at "muster" CLI that doesn't exist
- **Outdated reference** from earlier project design phase
- **NOT reflecting actual "openapi-cli" implementation**

### Recommendation

**DEPRECATE this spec** - It describes a CLI that doesn't match implementation.

**Alternative Actions**:
1. **DELETE** DOCS-CLI-REFERENCE.md (RECOMMENDED - 5 min)
   - This spec is fundamentally incorrect
   - Documents wrong CLI name ("muster" vs "openapi-cli")
   - Documents wrong commands (resource management vs OpenAPI processing)
   - No value to keep, entirely misleading
   
2. **REWRITE** (15-20 min) to match actual implementation:
   - CLI name: openapi-cli (not muster)
   - Commands: auth, config, completion, init, inventory, modules, generate, request, daemon
   - Remove: list, get, create, update, delete, search (don't exist)
   - Add: generate <spec-path>, inventory, modules, request, daemon commands
   - Update all flags and environment variables
   
3. **ARCHIVE** (2 min):
   - Move to specs/ARCHIVE/ with README explaining incorrect
   - Note: Documents non-existent "muster" CLI, actual is "openapi-cli"

**Impact of Keeping Incorrect Spec**:
- ❌ Misleads users about CLI structure
- ❌ Wrong examples won't work
- ❌ Wrong command names in docs
- ❌ Wrong environment variables in docs
- ❌ Wrong flag names in docs
- ❌ Zero value to documentation

**Impact of Delete/Rewrite**:
- ✅ Fixes documentation
- ✅ Provides correct CLI reference
- ✅ Aligns docs with implementation
- ✅ Prevents user confusion

### Priority

**Critical**: High (spec is fundamentally wrong, confusing users)
**Blocking**: NO (implementation is correct)
**Requirement**: MUST fix before production (documentation must be accurate)

---

**Sweep Complete**: 2026-03-05
**Next Action**: DELETE or REWRITE DOCS-CLI-REFERENCE.md
**Recommendation**: DELETE (spec is fundamentally incorrect - documents wrong CLI)
**Blockers**: None

