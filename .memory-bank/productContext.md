# Product Context

## Why This Project Exists

### Problems Solved

1. **CLI Market Fragmentation**
   - Developers use dozens of specialized CLI tools (curl, httpie, postman)
   - Each API requires learning a new tool or writing custom scripts
   - No standardized way to interact with APIs

2. **MCP Tool Proliferation**
   - Each service requires custom MCP server implementation
   - High maintenance burden for AI tool creators
   - Inconsistent behavior across different MCP servers

3. **API Documentation Gap**
   - OpenAPI specs are documentation-only, not executable
   - Gap between API definition and usable client
   - Manual work required to generate clients

4. **Security Complexity**
   - Multiple authentication methods across services
   - Credential management scattered across tools
   - No unified secure storage solution

5. **Platform Inconsistency**
   - Different tools for Linux, macOS, Windows
   - Shell completion varies by platform
   - Configuration management differs

## Perfect Implementation Vision

A **single binary** that:
- Automatically generates CLI commands from any OpenAPI spec
- Serves those commands via MCP protocol to Claude Desktop
- Runs as a daemon with Unix socket proxy
- Supports WASM plugins for custom logic
- Uses Starlark DSL for simple automation
- Works identically on Linux, macOS, Windows
- Self-updates from inventory servers
- Provides enterprise-grade security (mTLS, OAuth2, revocation)

## User Workflows

### Developer Workflow
1. Install: `brew install openapi-cli`
2. Authenticate: `openapi-cli auth oauth2 --provider github`
3. Fetch inventory: `openapi-cli inventory fetch https://api.example.com`
4. Generate commands: Automatic from OpenAPI spec
5. Execute: `openapi-cli api users list`

### AI Assistant Workflow
1. Configure Claude Desktop with MCP server
2. AI discovers available tools automatically
3. AI uses structured JSON-RPC calls
4. Real-time command execution
5. Consistent, reliable responses

### DevOps Workflow
1. Deploy as daemon: `openapi-cli daemon start`
2. Unix socket proxy for internal services
3. Service discovery via inventory
4. Centralized credential management
5. Audit logging for compliance

## Value Proposition

**For Developers**:
- One tool for all APIs
- Automatic CLI generation
- Type-safe API interactions
- Built-in authentication

**For AI Assistants**:
- Structured tool calling
- Consistent interface
- No custom code needed
- Real-time execution

**For Organizations**:
- Centralized API management
- Security compliance
- Audit trails
- Reduced maintenance

## Differentiation

Unlike existing tools:
- **Not just a generator** - Runtime execution
- **Not just a client** - MCP server mode
- **Not just a CLI** - Daemon mode available
- **Not platform-specific** - Cross-platform by design
- **Not static** - Dynamic updates via inventory

## Blind-Person Visualization

Imagine a universal remote control for APIs:
- **CLI Interface**: Buttons organized by API endpoint (GET, POST, PUT, DELETE)
- **MCP Interface**: Voice commands that Claude understands
- **Daemon**: Background service always ready to respond
- **User Workflow**: Point → Select → Execute → Results appear
- **Error States**: Clear messages, retry suggestions, circuit breaker protection

## Evolution

**Phase 1**: Core CLI with basic OpenAPI support  
**Phase 2**: MCP server mode for AI assistants  
**Phase 3**: WASM plugin system  
**Phase 4**: Daemon mode with socket proxy  
**Phase 5**: Enterprise features (mTLS, audit, multi-tenant)

## Success Metrics

- **Adoption**: 1000+ stars on GitHub in first month
- **Usage**: 10,000+ downloads via package managers
- **Retention**: 80% of users continue using after 30 days
- **Satisfaction**: <10 critical bugs reported
- **Performance**: <100ms average response time

## Current State

✅ Complete specifications (98 files)  
✅ Ralph Wiggum Loop ready  
🔄 Awaiting autonomous implementation
