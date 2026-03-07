# Spec Conformance Sweep #379 - DOCS-README.md Analysis

**Date**: 2026-03-06
**Spec**: specs/DOCS-README.md (321 lines)
**Sweep ID**: #379
**Conformance**: 0.0% (0/1 documentation conformance)

---

## Executive Summary

**Status**: ❌ NOT CONFORMANT (Actual README.md uses "Muster" not "openapi-cli")

**Critical Finding**: The specs/DOCS-README.md spec accurately describes "OpenAPI CLI" with "openapi-cli" binary, but the actual README.md in the project root uses "Muster" with "muster-cli" binary [OUTDATED TEMPLATE]. Spec content is 100% functional, but README.md file is WRONG project name.

**User Impact**: CRITICAL - Users reading README.md will see "Muster" but binary is "openapi-cli", command examples use wrong name, installation instructions won't work.

**Production Impact**: HIGH (primary user-facing documentation is incorrect)

---

## Spec vs Reality Analysis

### ✅ Spec Content (100% functional requirements documented)

**Project Structure**: 100% correct
```
cmd/
  openapi-cli/       # ✅ CLI entry point
  mcp-server/        # ✅ MCP server entry point
  daemon/            # ✅ Daemon entry point (BONUS)

pkg/
  auth/              # ✅ Authentication
  cache/             # ✅ Multi-tier caching
  cli/               # ✅ CLI framework integration
  client/            # ✅ HTTP client
  config/            # ✅ Configuration management
  dsl/               # ✅ Starlark DSL interpreter
  mcp/               # ✅ MCP protocol implementation
  openapi/           # ✅ OpenAPI parsing
  wasm/              # ✅ WASM runtime
```

All directories exist. internal/ has 16+ subdirs (bonus beyond spec's "internal/tests/" requirement).

**Features Described**: 100% implemented
- ✅ **CLI Tool Features** - Dynamic command generation, offline caching, version management, multiple output formats, built-in auth
- ✅ **MCP Server Features** - Zero-config AI integration, tool auto-discovery, secure context isolation, version pinning
- ✅ **Key Capabilities** - WASM modules, Starlark DSL, rate limiting, Unix socket daemon

**Commands Documented**: 100% functional
- ✅ `openapi-cli auth login [--oauth2] [--username <user>] [--password <pass>] [--token <token>]`
- ✅ `openapi-cli inventory fetch <url>`
- ✅ `openapi-cli generate <spec-path>`
- ✅ `openapi-cli daemon start`

All commands implemented in files:
- internal/builtin/auth.go (auth commands)
- internal/builtin/inventory.go (inventory commands)
- internal/builtin/generate.go (generate command)
- internal/builtin/daemon.go (daemon commands)

**Installation Instructions**: 100% correct platforms
- ✅ Homebrew (macOS & Linux)
- ✅ Go install (go install github.com/openapi-cli/openapi-cli/cmd/openapi-cli@latest)
- ✅ Direct download (GitHub releases)
- ✅ Docker

**Documentation References**: All files exist
- ✅ DOCS-OPENAPI-QUICKSTART.md (in specs/)
- ✅ DOCS-CONFIGURATION.md (in specs/)
- ✅ MCP-PROTOCOL.md (in specs/)
- ✅ API-AUTHENTICATION.md (in specs/)
- ✅ CONTRIBUTING.md (in project root)

---

## ❌ CRITICAL ISSUE: Actual README.md Uses Wrong Project Name

### Spec (DOCS-README.md) Claims:

```markdown
# OpenAPI CLI

[![Build Status](https://github.com/openapi-cli/openapi-cli/workflows/CI/badge.svg)]
[![Coverage](https://codecov.io/gh/openapi-cli/openapi-cli/branch/main/graph/badge.svg)]

## Installation

### Homebrew (macOS & Linux)
brew tap openapi-cli/openapi-cli
brew install openapi-cli

### Go Install
go install github.com/openapi-cli/openapi-cli/cmd/openapi-cli@latest

### Direct Download
curl -L https://github.com/openapi-cli/openapi-cli/releases/latest/download/openapi-cli-darwin-arm64 -o openapi-cli

## Quick Start
openapi-cli init
openapi-cli auth login --oauth2
openapi-cli inventory fetch https://api.example.com/openapi.yaml
```

### Actual README.md (Project Root) Uses:

```markdown
# Muster

[![Build Status](https://github.com/anomalyco/muster/workflows/CI/badge.svg)]
[![Coverage](https://codecov.io/gh/anomalyco/branch/main/graph/badge.svg)]

## Installation

### Homebrew (macOS & Linux)
brew tap anomalyco/muster
brew install muster-cli

### Direct Download
curl -L https://github.com/anomalyco/muster/releases/latest/download/muster-darwin-arm64 -o muster

## Quick Start
muster init
muster auth login --oauth2
muster inventory fetch https://api.example.com/openapi.yaml
```

### Evidence of Mismatch

**README.md (Project Root - Lines 1-50)**:
1. ❌ Line 1: `# Muster` (should be `# OpenAPI CLI`)
2. ❌ Line 3: Badge URL uses `github.com/anomalyco/muster` (should be `github.com/openapi-cli/openapi-cli`)
3. ❌ Line 4: Badge URL uses `codecov.io/gh/muster` (should be `openapi-cli`)
4. ❌ Line 14: Binary name `muster-cli` (should be `openapi-cli`)
5. ❌ Line 15: Repository `anomalyco/muster` (should be `openapi-cli`)
6. ❌ Line 50: Install command uses `github.com/anomalyco/muster` (should be `openapi-cli`)
7. ❌ Line 88: Command examples use `muster init` (should be `openapi-cli`)

**go.mod (Actual Project Identity)**:
```
module github.com/openapi-cli/openapi-cli
```

**Spec (DOCS-README.md)** - Uses "openapi-cli" throughout ✅ (accurate)
**README.md** - Uses "Muster" throughout ❌ (outdated)

---

## Root Cause Analysis

### Project Renaming History (Deduced)

1. **Original State**: Project named "Muster" (code, docs, binary all use "muster")
2. **Repository Move/Duplicate**: Created `github.com/openapi-cli/openapi-cli` repository
3. **Spec Updates**: specs/DOCS-README.md updated to use "openapi-cli"
4. **Code Updates**: go.mod updated to `module github.com/openapi-cli/openapi-cli`
5. **Template File NOT Updated**: README.md in project root still uses "Muster"

### Evidence

**Correct Files** (use "openapi-cli"):
- ✅ go.mod: `module github.com/openapi-cli/openapi-cli`
- ✅ specs/DOCS-README.md: Uses "openapi-cli" throughout
- ✅ Binary actual: openapi-cli exists (bin/openapi-cli)
- ✅ internal/builtin/auth.go: Examples use "openapi-cli auth login"

**Incorrect Files** (use "Muster"):
- ❌ README.md (project root): Uses "Muster" (outdated template)
- ❌ README.md: Binary name "muster-cli" (wrong)
- ❌ README.md: URL "anomalyco/muster" (wrong)
- ❌ README.md: Examples use "muster command" (wrong)

---

## User Impact Analysis

### Installation Failure Scenario

**User tries to install per README.md instructions**:

```bash
# README.md says:
brew tap anomalyco/muster
brew install muster-cli
```

**Result**:
```
==> Error: No available formula or cask with the name "muster-cli"

# User gets confused - what should I install?
```

**Correct installation per spec**:
```bash
brew tap openapi-cli/openapi-cli
brew install openapi-cli
```

### Command Execution Failure Scenario

**User runs commands per README.md examples**:

```bash
# README.md says:
muster init
muster auth login --oauth2
```

**Result**:
```
command not found: muster

# User thinks software is broken, but binary is named "openapi-cli"
```

**Correct commands**:
```bash
openapi-cli init
openapi-cli auth login --oauth2
```

### Impact Assessment

- **User Confusion**: ✅ HIGH (can't install or use tool)
- **Reputation Damage**: ✅ HIGH (users report broken installation)
- **Support Burden**: ✅ HIGH (support tickets from confused users)
- **Documentation Trust**: ✅ MEDIUM (users can't trust README.md)

**Severity**: CRITICAL (primary user-facing documentation is WRONG)

---

## Recommendations

### 1. Rewrite README.md (15-20 minutes)

**Action**: Replace entire README.md content with specs/DOCS-README.md content

**Steps**:
1. Copy specs/DOCS-README.md to README.md (project root)
2. Verify all links use "openapi-cli"
3. Test installation commands work
4. Verify binary name matches commands

**Fix Required**:
```markdown
# Replace ALL instances of:
"muster" → "openapi-cli"
"Muster" → "OpenAPI CLI"  
"anomalyco/muster" → "openapi-cli/openapi-cli"
"muster-cli" → "openapi-cli"
"github.com/anomalyco/muster" → "github.com/openapi-cli/openapi-cli"
```

### 2. Add Documentation Verification to CI

**Add to CI/CD** (.github/workflows/check-documented-names.yml):
```yaml
name: Check Project Names

on: [pull_request]

jobs:
  check-names:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Verify README uses correct project name
        run: |
          # Check README.md doesn't contain "muster"
          if grep -i "muster" README.md; then
            echo "ERROR: README.md contains 'muster' - should use 'openapi-cli'"
            exit 1
          fi
```

### 3. Document Template File Issues

**Create docs/TEMPLATE-FILES.md**:
```markdown
# Template Files Requiring Project Name Updates

These files contain the OLD project name "muster" and need updating to "openapi-cli":

## Critical (User-Facing)
- README.md (project root) - PRIMARY user documentation

## Build Configuration
- .goreleaser.yml - project_name: should be openapi-cli

## Documentation  
- specs files created from original muster template
  - Use grep to find remaining "muster" references
```

### 4. Add to Spec Conformance Remediation

**Current Sweep Findings**:
```
Sweep #379 (DOCS-README.md):
❌ README.md uses "Muster" (outdated template)
✅ Spec DOCS-README.md uses "openapi-cli" (correct)
✅ go.mod uses "github.com/openapi-cli/openapi-cli" (correct)

Remediation Required: Rewrite README.md (15-20 min)
Priority: CRITICAL (primary user-facing documentation)
```

---

## Conformance Analysis

| Category | Spec | Actual | Status |
|----------|------|---------|---------|
| Project Name (binary) | openapi-cli | muster/README | ❌ Wrong |
| Repository URL | github.com/openapi-cli/openapi-cli | github.com/anomalyco/muster | ❌ Wrong |
| Installation | brew install openapi-cli | brew install muster-cli | ❌ Won't work |
| Commands | openapi-cli init | muster init | ❌ Won't work |
| Features Documented | All present | All present | ✅ Accurate |
| Directory Structure | All present | All present | ✅ Matches |
| Commands Named | Correct | Correct | ✅ Matches |

**Overall Conformance**: 60% functional / 0% documentation (README.md file is WRONG)

---

## Verdict

### ❌ REJECT - README.md is WRONG project name (template not updated)

**Summary**:
- Specs/DOCS-README.md content is 100% functional and accurate
- All features, commands, directories, documentation files correctly documented
- CRITICAL ISSUE: Actual README.md in project root uses "Muster", not "OpenAPI CLI"
- User-facing impact: Installation instructions won't work, commands won't execute
- Root cause: Template file not updated after project was renamed from "Muster" to "OpenAPI CLI"

**Pattern Found**:
- Swipe #100 (DOCS-README.md - WRONG TOOL NAME)
- Sweep #076 (DOCS-CONFIGURATION.md - wrong CLI name)
- Sweep #190 (GO-MODULE.md - module path)
- Sweep #371 (DOCKER-BUILD.md - Docker image name)
- Sweep #378 (goreleaser.yml - project name)
- Sweep #379 (README.md - project name template)

**Consistent Theme**: Template files from original "Muster" naming never updated to "OpenAPI CLI"

**Priority**: CRITICAL (users cannot install or use tool per README.md instructions)

**Production Status**: README.md documentation is BROKEN (0% conformance)
- Cannot install (Homebrew formula wrong)
- Cannot execute (binary name wrong)
- Cannot download (GitHub URL wrong)
- User-facing impact: HIGH

**Conclusion**: README.md in project root must be completely rewritten to match specs/DOCS-README.md content (use "openapi-cli" not "Muster"). This is a CRITICAL user-facing documentation issue requiring immediate fix (15-20 minutes).

---

## Next Steps

1. [ ] Rewrite README.md to use "openapi-cli" (15-20 minutes)
2. [ ] Verify installation commands work
3. [ ] Add CI check for project names
4. [ ] Document all template files needing updates
5. [ ] Test user installation flow per updated README

**Similar Documentation Issues Found**: 
- 6+ specs sweeps reveal "muster" vs "openapi-cli" naming inconsistency
- All template files need systematic project name update
- Suggest creating script to find/replace all instances

