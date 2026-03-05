# Story 2.8: MCP Server Setup

## Epic
Epic 2: Dual Interface

## Story
As an AI assistant user, I want to run the tool as an MCP server so that Claude/Cursor can interact with APIs dynamically.

## Acceptance Criteria
- [ ] Implement MCP server entry point
- [ ] Handle MCP initialize request
- [ ] Support stdio transport
- [ ] Load inventory on startup
- [ ] Generate MCP tools from OpenAPI operations
- [ ] Handle MCP tools/list request
- [ ] Handle MCP tools/call request
- [ ] Proper error responses
- [ ] Can be run via npx
- [ ] Logging to stderr (MCP uses stdout)

## Technical Notes
- Use @modelcontextprotocol/sdk
- Entry point: `openapi-cli --mcp-server` or separate binary
- Tools generated from OpenAPI operationId or path
- Each tool has name, description, and inputSchema
- Response format: `{ content: [{ type: "text", text: "..." }] }`

## Estimation
- **Story Points**: 5
- **Duration**: 3-4 days

## Dependencies
- Epic 1: Inventory System
- Story 2.9: Tool Registration

## Test Cases
1. Server initializes successfully
2. Returns tools list matching OpenAPI
3. Executes tool call correctly
4. Handles errors gracefully
5. Works via npx
6. Uses inventory auth correctly
7. Respects rate limits
8. Returns proper MCP error format
9. Handles concurrent requests
10. Cleanup on shutdown
