# Archive Directory

This directory contains deprecated or incorrect specification files that have been replaced.

## DOCS-QUICKSTART-DEPRECATED-INCORRECT-TOOL.md

**Status**: DEPRECATED - CRITICAL ERROR

**Issue**: This specification described "mutter" (a Kubernetes-style resource manager), but the actual tool is "openapi-cli" (OpenAPI CLI generator + MCP server).

**Impact**: Users following this guide would fail at the first command.

**Replaced By**: [DOCS-OpenAPI-QuickStart.md](../DOCS-OPENAPI-QUICKSTART.md)

**History**:
- Date: 2026-03-05
- Issue: Spec Conformance Sweep #052 identified critical mismatch
- Original spec: 0/13 requirements met (completely wrong tool)
- Correction: Rewrite with accurate openapi-cli commands and workflows

**Root Cause**: Specification document created for different tool (mutter), applied to openapi-cli without verification.

**Verification Needed**: Check all spec files for tool name mismatches:
- "mutter" binary name vs actual "openapi-cli"
- Resource manager commands vs actual OpenAPI/dynamic tool
- Kubernetes terminology vs actual CLI/MCP tool architecture
