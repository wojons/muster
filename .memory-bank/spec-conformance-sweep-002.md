# Idle-Time Spec Conformance Sweep #2 - ZSH-COMPLETION.md

**Date**: 2026-03-05
**Spec File**: `specs/ZSH-COMPLETION.md`
**Sweep Method**: Complete spec vs. implementation conformance analysis
**Status**: ⚠️ MAJOR SPEC MISMATCH - Spec written for Python/Rust tool "muster", implementation is Go-based "openapi-cli"

---

## Overview

**Purpose**: Verify that the Zsh completion specification matches the actual implementation in the codebase.

**Approach**:
1. Read complete spec file (496 lines)
2. Identify ALL requirements in the spec
3. Map requirements to actual implementation files
4. Check for implementation vs. spec discrepancies
5. Document gaps and actionable remediation tasks

---

## Requirements Analysis

### Complete Requirements List (from spec)

1. **Zsh completion file structure**: `completions/_muster`
2. **Core completion script** with global options (-h/--help, -v/--version, -q/--quiet, -d/--debug, -c/--config)
3. **Subcommand completions**: init, build, test, deploy, config, version
4. **Command-specific arguments** for each subcommand
5. **Dynamic completion generator**: `src/completion/zsh_generator.py`
6. **Completion installation script**: `scripts/install-completions.sh`
7. **Advanced completion features**:
   - Context-aware file completions (*.rs for build, *.toml for config, *.yaml for deploy)
   - Dynamic option completion (query tool for available options)
   - Service name completion from `.muster/services.json`
8. **Completion testing**: `tests/test_zsh_completion.py`
9. **Manual completion script** with full Zsh syntax

---

## Implementation Mapping

### ✅ IMPLEMENTED (Go/Cobra approach - Different from spec's Python approach)

| Requirement | Implementation File | Status | Notes |
|-------------|-------------------|--------|-------|
| Multi-shell completion | `pkg/completion/generator.go` | ✅ IMPLEMENTED | Supports bash, zsh, fish, powershell in Go (not Python) |
| Command completion | `pkg/completion/generator.go` + Cobra | ✅ IMPLEMENTED | Uses Cobra's completion system (generates on-the-fly) |
| Option completion | `internal/builtin/completion.go` | ✅ IMPLEMENTED | Works via Cobra's GenBash/GenZsh/GenFish/GenPowerShell |
| Dynamic completion | `pkg/completion/dynamic.go` | ✅ IMPLEMENTED | Resources, namespaces, profiles, HTTP methods, output formats |
| Completion generation | `pkg/completion/generator.go:77-105` | ✅ IMPLEMENTED | Template-based script generation in Go |
| Tests for generator | `pkg/completion/generator_test.go` | ✅ PRESENT | 105 lines, tests bash, zsh, fish, powershell |
| Install path logic | `pkg/completion/generator.go:273-287` | ✅ IMPLEMENTED | Returns system-wide and user-local paths |

---

## Critical Gaps and Issues

### Critical Issues

#### 1. ❌ SPEC ERROR: Wrong Language/Tool Name

**Issue**: Spec is written for **Python/Rust CLI tool named "muster"** but actual implementation is **Go-based "openapi-cli"**.

**Evidence**:
- Spec line 71: `#compdef muster` (wrong tool name)
- Spec line 89: `**File**: src/completion/zsh_generator.py** (Python file, should be Go)
- Spec line 218: `from ..cli import main_parser` (Python import, wrong language)
- Spec line 437: `current=1; words=(muster '')` (wrong tool name in test)

**Actual Implementation**:
- Language: Go (not Python)
- Tool Name: openapi-cli (not muster)
- Files: `pkg/completion/generator.go`, `pkg/completion/dynamic.go` (not Python files)

**Impact**: HIGH - Spec is completely incorrect for this codebase
**Remediation**: SPEC UPDATE NEEDED - Rewrite spec for Go/Cobra implementation

---

#### 2. ❌ SPEC ERROR: Wrong Implementation Approach

**Issue**: Spec assumes static completion script files (`completions/_muster`) but Go/Cobra generates scripts on-the-fly.

**Evidence in Spec**:
- Lines 14-22: Spec shows static file structure
- Lines 24-85: Full Zsh completion script with manual definitions
- Lines 30-38: Manual global and subcommand definitions

**Evidence in Implementation**:
- `pkg/completion/generator.go:77-105`: Generates completion scripts dynamically via templates
- `internal/builtin/completion.go:52-60`: Uses Cobra's GenBash/GenZsh/GenFish/GenPowerShell
- Scripts are generated on-demand, not stored as static files

**Impact**: MEDIUM - Spec's approach doesn't match Go/Cobra best practices
**Remediation**: SPEC UPDATE NEEDED - Document Go/Cobra dynamic completion approach

---

#### 3. ❌ SPEC ERROR: Wrong File Structure

**Issue**: Spec references Python files that don't exist in this Go codebase.

**Spec Mismatches**:
- `src/completion/zsh_generator.py` (line 89) → Does NOT exist
- `scripts/install-completions.sh` (line 235) → Does NOT exist
- `tests/test_zsh_completion.py` (line 358) → Does NOT exist

**Actual Implementation**:
- `pkg/completion/generator.go` → Go implementation
- `pkg/completion/dynamic.go` → Dynamic completion in Go
- `pkg/completion/generator_test.go` → Go tests
- `internal/builtin/completion.go` → CLI completion command

**Impact**: MEDIUM - Spec documentation is completely wrong for this codebase
**Remediation**: SPEC FIX REQUIRED - Update all file references to Go equivalents

---

### Gap 1: Advanced Context-Aware File Completions (Not Implemented)

**Requirement** (lines 309-328 in spec):
```zsh
_muster_file_completion() {
    local cmd="${line[1]}"
    local subcmd="${line[2]}"
    
    case "$cmd" in
        build)
            _files -g "*.rs"  # Rust source files
            ;;
        config)
            _files -g "*.toml"  # TOML config files
            ;;
        deploy)
            _files -g "*.yaml" -g "*.yml"  # YAML deployment files
            ;;
        *)
            _files
            ;;
    esac
}
```

**Implementation Check**: NOT IMPLEMENTED

Cobra's completion system doesn't support file type filtering per command context out of the box.

**Impact**: LOW - Nice-to-have feature, not required for basic completion
**Remediation**: OPTIONAL - Could add custom completion logic in future

---

### Gap 2: Service Name Completion (Not Implemented)

**Requirement** (lines 341-351 in spec):
```zsh
_muster_service_names() {
    local -a services
    
    # Read from configuration or query API
    if [[ -f ".muster/services.json" ]]; then
        services=($(jq -r '.services[].name' .muster/services.json 2>/dev/null))
    fi
    
    _describe -t services 'services' services
}
```

**Implementation Check**: NOT IMPLEMENTED

No service name completion from configuration files.

**Impact**: LOW - Nice-to-have feature for deployment workflows
**Remediation**: OPTIONAL - Could add in future

---

### Gap 3: Installation Script (Not Implemented)

**Requirement** (lines 235-302 in spec):
- `scripts/install-completions.sh` with install/uninstall functions
- System-wide and user-local installation support
- Zsh completion activation

**Implementation Check**: NOT IMPLEMENTED

No installation script exists. Users must manually copy completion output or pipe to file.

**Impact**: LOW - Go/Cobra users typically pipe completion output directly
**Remediation**: OPTIONAL - Could add convenience install script

---

### Gap 4: Python Test File References (Wrong)

**Requirement** (lines 356-437 in spec):
- `tests/test_zsh_completion.py` with pytest
- Tests for completion script generation
- Tests for Zsh syntax validity
- Tests for completion integration behavior

**Implementation Check**: GO TESTS EXIST (lines 1-105 in `pkg/completion/generator_test.go`)

Actual tests are in Go, not Python. Tests cover:
- Script generation for bash, zsh, fish, powershell
- Unsupported shell handling
- Install path logic

**Impact**: NONE - Tests exist in Go instead of Python (correct for this codebase)
**Remediation**: SPEC UPDATE NEEDED - Update test documentation to show Go tests

---

## Reality Check Summary

### What This Spec Actually Says:

This spec was written for a **Python/Rust CLI tool named "muster"** that uses:
- Python completion generator (`src/completion/zsh_generator.py`)
- Static completion scripts (`completions/_muster`)
- Python tests (`tests/test_zsh_completion.py`)
- Manual Zsh script definitions

### What Actually Exists:

**Go-based "openapi-cli" implementation** that uses:
- Go completion generator (`pkg/completion/generator.go`)
- Dynamic script generation via Go templates
- Go tests (`pkg/completion/generator_test.go`)
- Cobra framework for completion

---

## Recommended Actions

### MUST FIX (Critical - Spec Inaccuracy)

1. **Spec Rewrite Required**:
   - Update all references from "muster" to "openapi-cli"
   - Replace Python code examples with Go code
   - Update file structure to match Go implementation
   - Change manual script definitions to demonstrate Go/Cobra approach

2. **Documentation Update**:
   - Document that completion is generated on-the-fly (not static files)
   - Show actual Go code examples (`pkg/completion/generator.go`)
   - Update test documentation to reflect Go tests
   - Remove Python-specific installation instructions

### SHOULD FIX (Important - Feature Gaps)

3. **Optional Enhancement**: Add context-aware file completions
   - Create custom completion functions in Go
   - Add file type filtering per command context
   - Priority: LOW (quality enhancement, not required for production)

4. **Optional Enhancement**: Add service name completion
   - Read from configuration files
   - Query API for available services
   - Priority: LOW (quality enhancement)

### NICE TO HAVE (Convenience)

5. **Optional Add**: Create installation script
   - `scripts/install-completions.sh` convenience wrapper
   - Document Go/Cobra-idiomatic installation
   - Priority: LOW

---

## Conclusion

**Overall**: ❌ CRITICAL SPEC MISMATCH

The `ZSH-COMPLETION.md` spec is **COMPLETELY INCORRECT** for this codebase. It was written for a different tool ("muster" in Python/Rust) and must be completely rewritten for the actual Go-based "openapi-cli" implementation.

**What Works**:
- ✅ Completion functionality exists and works (bash, zsh, fish, powershell)
- ✅ Dynamic completion implemented (resources, namespaces, profiles)
- ✅ Tests present in Go
- ✅ Installation paths calculated correctly

**What's Wrong**:
- ❌ Spec references wrong tool name ("muster" vs "openapi-cli")
- ❌ Spec shows Python code instead of Go
- ❌ Spec shows static files instead of dynamic generation
- ❌ Spec test examples are Python instead of Go
- ❌ Advanced features not implemented (file context, service names)

**Is This a Bug in Implementation?**: NO
- Implementation follows Go/Cobra best practices
- Completion works correctly for all supported shells
- Tests cover the actual implementation

**Is This a Bug in Specification?**: YES
- Spec was copied from another project ("muster") without adaptation
- All examples are wrong for this codebase
- File structure is completely different

**Action Required**: SPEC MUST BE REWRITTEN for Go/Cobra implementation

---

## Gap Summary Table

| Type | Count | Remediation Required |
|------|-------|---------------------|
| Critical: Wrong Tool Name | Extensive | SPEC REWRITE |
| Critical: Wrong Language | 4 files | SPEC REWRITE |
| Critical: Wrong Implementation Approach | All examples | SPEC REWRITE |
| Medium: Install Script Missing | 1 file | OPTIONAL |
| Low: Advanced Features Missing | 2 features | FUTURE ENHANCEMENT |
| Total Gaps | 8+ issues | SPEC REWRITE REQUIRED |

---

## Files Verified

**Implementation Files (Go - WORKING)**:
- `pkg/completion/generator.go` ✅ (287 lines)
- `pkg/completion/dynamic.go` ✅ (173 lines)
- `internal/builtin/completion.go` ✅ (80 lines)
- `pkg/completion/generator_test.go` ✅ (105 lines)

**Spec Files (Python/Rust - WRONG)**:
- `specs/ZSH-COMPLETION.md` ❌ (496 lines - completely incorrect for this codebase)

**Missing Files (Referenced in spec)**:
- `src/completion/zsh_generator.py` ❌ (doesn't exist)
- `tests/test_zsh_completion.py` ❌ (doesn't exist - exists in Go as generator_test.go)
- `completions/_muster` ❌ (doesn't need to exist - Cobra generates on-the-fly)
- `scripts/install-completions.sh` ❌ (doesn't exist, optional convenience)

---

## Next Steps

**Required (Spec Conformance)**:
- [ ] Rewrite `specs/ZSH-COMPLETION.md` for Go/Cobra implementation
- [ ] Update all examples from Python to Go
- [ ] Change tool name references from "muster" to "openapi-cli"
- [ ] Document dynamic completion generation approach
- [ ] Update test documentation to show Go tests

**Optional (Feature Enhancement)**:
- [ ] Add context-aware file completions (LOW priority)
- [ ] Add service name completion (LOW priority)
- [ ] Create installation convenience script (LOW priority)

---

**Sweep Status**: ⚠️ SPEC CRITICAL MISMATCH DETECTED - Spec rewrite required before validation can continue
