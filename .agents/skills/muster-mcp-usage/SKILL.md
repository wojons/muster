---
name: muster-mcp-usage
description: Use the muster MCP server: build it, connect a real MCP client, initialize and call generated SOAP/REST tools over stdio. First real consumer pass of the MCP tool surface.
version: 1.0.0
category: dogfood
---

# Using the muster MCP server

## Build
`make build-mcp` → bin/openapi-mcp (v0.2.0). Bare `go build ./cmd/mcp-server` also stamps now (BuildInfo vcs fallback, e6f4538a).

## Connect
stdio JSON-RPC 2.0. Handshake:
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"probe","version":"0"}}}
then notifications/initialized, then tools/list.

## Invoke
stdio: {"jsonrpc":"2.0","id":N,"method":"tools/call","params":{"name":"...","arguments":{...}}}

## Known harness quirks (mcp-go v0.x at HEAD d95acc02)
- Readiness: constructor opens a port (net.Listen) but the stdio server does not IPC-serve; the readiness gate polls RPC /mcp, where a stdio build returns 404. Harmless local signal.
- ping (raw) returns capacity JSON with the header if absent, not `null`.
- get-schema 404s (GET /mcp/{serverID}/schema) — the harness mcp-go gate cannot embed an MCP method listener on the HTTP session path.
- External-sock path never decodes the harness's base36 session token — any picked client id fails the handshake.
