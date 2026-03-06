# Spec Conformance Sweep: CLI-MARKET-TEMPLATES.md

**Date**: 2026-03-06
**Sweep #**: 113
**Spec**: specs/CLI-MARKET-TEMPLATES.md (482 lines)
**Spec Type**: ❌ WRONG SPEC - Describes different system (template/scaffolding)
**Status**: ⚠️ 0% CONFORMANCE - Not in IMPLEMENTATION-MAPPING.md
**Implementation**: NOT FOUND (spec describes system this project is NOT)

---

## Executive Summary

**Overall Conformance**: 0% (0/6 major components implemented)
**Production Ready**: ✅ YES (spec is for different system, NOT required)
**Blocker**: NO
**Priority**: N/A (outdated/wrong spec)

**Critical Finding**: This spec describes a **template/scaffolding system** for project generation. The actual system ("Muster") is a **Dynamic API Client & MCP Server** that generates commands from OpenAPI specs. These are completely different systems.

---

## Scope Analysis

**What Spec Describes**:
- Template system with internal/template/ directory
- Template engine for project scaffolding
- Built-in templates for project/module/config generation
- Commands: `template list`, `template create`

**What System Actually Is**:
- Dynamic API client that configures from remote _inventory.json
- OpenAPI spec parser and command generator
- MCP server for Claude Desktop integration
- WASM runtime for plugin execution

---

## System Identity Verification

**README.md** confirms actual system:
Lines 3-4:
```
Muster is a Dynamic API Client & MCP Server - Auto-configuring, versioned, always up-to-date
```

**CLI-MARKET-TEMPLATES.md** describes different system:
Lines 3-6:
```
The template system provides project scaffolding and code generation capabilities through customizable templates.
```

**Conclusion**: System type mismatch - spec describes template system, actual is OpenAPI CLI + MCP

---

## Conformance Verification

### Required Directory Structure (Lines 9-18)

**Spec Requires**:
```
internal/template/
├── engine.go        # Template execution engine
├── scaffold.go      # Project scaffolding
├── variables.go     # Variable resolution
└── builtin/         # Built-in templates
    ├── project/
    ├── module/
    └── config/
```

**Implementation**: ❌ NOT FOUND

**Evidence**:
```bash
test -d internal/template && echo "EXISTS" || echo "NOT_FOUND"
Result: NOT_FOUND
```

- internal/template/ directory does NOT exist
- No template engine implementation found
- No scaffold.go file found
- No builtin/ template directory found

**Conformance**: 0/4 directories (0%)

---

### Template Engine (Lines 21-108)

**Spec Requires** (lines 36-73):
```go
type Engine struct {
    funcs template.FuncMap
}

func NewEngine() *Engine { }
func (e *Engine) Execute(tmplStr string, vars map[string]interface{}) (string, error) { }
func (e *Engine) ExecuteFile(templatePath string, vars map[string]interface{}) (string, error) { }
```

**Implementation**: ❌ NOT FOUND

**Evidence**:
- No Engine struct found in codebase
- No Execute() method found
- No template execution logic found

**Conformance**: 0% (no template engine)

---

### Project Scaffolding (Lines 110-244)

**Spec Requires**:
```go
type Scaffolder struct {
    engine      *Engine
    templateDir string
}

func NewScaffolder(templateDir string) *Scaffolder { }
func (s *Scaffolder) Scaffold(ctx context.Context, templateName, targetDir string, vars map[string]interface{}) error { }
```

**Implementation**: ❌ NOT FOUND

**Evidence**:
- No Scaffolder struct found
- No Scaffold() method found
- No scaffolding logic found

**Conformance**: 0% (no scaffolding)

---

### Template Commands (Lines 246-334)

**Spec Requires**:
```bash
muster template list
muster template create <template> <directory>
```

**Implementation**: ❌ NOT FOUND

**Evidence**:
- No template command in cmd/openapi-cli/
- No template command in cmd/mcp-server/
- NewTemplateCommand() function NOT found

**Conformance**: 0% (no template commands)

---

### Variable Resolution (Lines 336-455)

**Spec Requires**:
```go
type Resolver struct {
    values  map[string]interface{}
    prompts map[string]Prompt
}

func (r *Resolver) Resolve(interactive bool) error { }
```

**Implementation**: ❌ NOT FOUND

**Evidence**:
- No Resolver struct found
- No template variable resolution logic found

**Conformance**: 0% (no variable resolution)

---

### Built-in Templates (Lines 457-482)

**Spec Requires**:
```
templates/
├── project/
├── module/
└── config/
```

**Implementation**: ❌ NOT FOUND

**Evidence**:
- No templates/ directory found
- No template files found

**Conformance**: 0% (no templates)

---

## Conformance Summary

| Spec Requirement | Spec Lines | Implementation | Status | Conformance |
|------------------|------------|----------------|--------|-------------|
| Directory structure | 9-18 | NONE | ❌ NOT FOUND | 0% |
| Template engine | 21-108 | NONE | ❌ NOT FOUND | 0% |
| Project scaffolding | 110-244 | NONE | ❌ NOT FOUND | 0% |
| Template commands | 246-334 | NONE | ❌ NOT FOUND | 0% |
| Variable resolution | 336-455 | NONE | ❌ NOT FOUND | 0% |
| Built-in templates | 457-482 | NONE | ❌ NOT FOUND | 0% |
| **OVERALL** | **1-482** | **NONE** | **❌ 0%** | **0%** |

---

## Identity Mismatch Analysis

**Spec Describes**: Template/Scaffolding System
Purpose: Project scaffolding and code generation
Features: Template engine, scaffolder, variable resolution
Commands: `template list`, `template create`

**System Actually Is**: Dynamic API Client + MCP Server
Purpose: Generate commands from OpenAPI specs
Features: OpenAPI parser, command generator, MCP server
Commands: `inventory fetch`, `generate commands`, `auth oauth2`, `daemon start`

**Conclusion**: ❌ COMPLETE SYSTEM MISMATCH

Similar to sweep #077 (CLI-PLUGIN-SYSTEM) and sweep #011 (QUOTA-MANAGEMENT), spec describes system this project is NOT.

---

## Production Readiness

✅ **YES - Production Ready**

**Rationale**:
1. This spec is NOT for the current system
2. System achieves its purpose WITHOUT template system
3. Spec appears to be outdated/abandoned feature
4. Template functionality NOT required for core purpose

---

## Root Cause Analysis

**Spec Status**: OUTDATED/ABANDONED

**Evidence**:
1. `IMPLEMENTATION-MAPPING.md` does NOT list template system
2. No internal/template/ directory exists
3. No template-related commands found
4. Project purpose is OpenAPI CLI + MCP (NOT scaffolding)

**Possible Origins**:
1. Initial project plan included template system (later abandoned)
2. Spec written for different project ( mistakenly included)
3. Copy-paste error from other codebase's specs

**Similar Findings**:
- Sweep #077: CLI-PLUGIN-SYSTEM (0% conformance)
- Sweep #011: QUOTA-MANAGEMENT (0% conformance - enterprise feature)

---

## Recommended Actions

**Priority 1 - IMMEDIATE (Documentation cleanup)**:
- [ ] Move to specs/ARCHIVE/ directory
- [ ] Label as "OUTDATED - WRONG SYSTEM"
- [ ] Create README explaining purpose

**Alternative Approach**:
- [ ] Update spec to match actual system capabilities
- [ ] Remove scaffolding/template requirements
- [ ] Rebrand as "Command Generation Templates" (if any template-like features exist)

**Best Practice Recommendation**: Archive spec (15 minutes)
- Move to specs/ARCHIVE/CLI-MARKET-TEMPLATES-DEPRECATED-WRONG-SYSTEM.md
- Add comment: "This spec describes a template/scaffolding system. The actual system is a Dynamic API Client + MCP Server."
- No implementation needed

---

## Timeline Estimate

**Documentation Fix**: 10-15 minutes (archive spec)

**Implementation Effort**: NOT REQUIRED (system NOT intended to have template scaffolding)

---

## Conclusion

**Overall Conformance**: 0% (0/6 requirements met)
**Production Ready**: ✅ YES (spec is for different system)
**Blocker**: NO

**Key Finding**: This spec describes a template/scaffolding system for project generation. The actual "Muster" system is a Dynamic API Client + MCP Server that generates commands from OpenAPI specs. These are completely different systems.

**Recommendation**: Archive this spec as "OUTDATED - WRONG SYSTEM" (15 minutes, no code changes needed)

---

**Similar Specs to Review** (if time permits):
- specs/CLI-PLUGIN-SYSTEM.md (sweep #077: 0% conformance)
- specs/QUOTA-MANAGEMENT.md (sweep #011: 0% conformance - enterprise feature)

---

**Sweep Method**: Line-by-line code inspection with exact evidence
**Confidence**: HIGH (actual code verification + scope analysis)
