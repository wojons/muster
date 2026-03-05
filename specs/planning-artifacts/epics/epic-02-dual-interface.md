# Epic 2: Dual Interface (CLI + MCP)

## Overview
Implement both CLI and MCP server interfaces that share the same core functionality. The CLI provides a traditional command-line experience, while the MCP server enables AI assistant integration.

## Business Value
- CLI for scripts and human users
- MCP for AI assistants (Claude, Cursor, etc.)
- Same configuration and behavior in both modes
- Maximum flexibility for different use cases

## Acceptance Criteria
### CLI Interface
1. Standalone binary with subcommand structure
2. Argument parsing with validation
3. Progress indicators and spinners
4. Multiple output formats (JSON, table, YAML, raw)
5. Shell completion generation (bash, zsh, fish)
6. Exit codes for scripting
7. Interactive prompts for missing required values

### MCP Interface
1. Implements Model Context Protocol
2. Exposes OpenAPI operations as MCP tools
3. Supports stdio transport
4. Supports HTTP transport (future)
5. Proper MCP lifecycle handling
6. Tool descriptions from OpenAPI docs
7. Can be run via npx without installation

### Shared Behavior
1. Both use same inventory system
2. Both use same configuration hierarchy
3. Both use same execution engine
4. Both support same auth methods
5. Consistent error messages

## Technical Requirements
- Commander.js for CLI framework
- @modelcontextprotocol/sdk for MCP
- Shared core modules (inventory, config, execution)
- Single entry point that branches to CLI or MCP

## Dependencies
- Epic 1: Inventory System

## Stories
### CLI
- Story 2.1: CLI entry point setup
- Story 2.2: Argument parsing and validation
- Story 2.3: Command structure generation
- Story 2.4: Output formatting (JSON, table, YAML)
- Story 2.5: Progress indicators
- Story 2.6: Shell completion scripts
- Story 2.7: Interactive prompts

### MCP
- Story 2.8: MCP server setup
- Story 2.9: Tool registration from OpenAPI
- Story 2.10: MCP request handling
- Story 2.11: stdio transport
- Story 2.12: npx compatibility

## Estimation
- **Story Points**: 21
- **Duration**: 3-4 weeks

## Risks
- MCP protocol complexity
- Keeping both interfaces in sync
- Testing both interfaces thoroughly

## Notes
- CLI binary: `openapi-cli`
- MCP entry: `openapi-cli --mcp-server` or separate binary
- Both should share 90%+ code
