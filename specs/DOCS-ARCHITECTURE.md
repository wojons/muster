# ⚠️ DEPRECATED - This spec is OBSOLETE

**WARNING**: This specification describes an outdated architecture that does NOT match the current implementation.  

**Status**: OBSOLETE - Do not use for reference  
**Last Valid Update**: Unknown  
**Superseded By**: [master-specification.md](./master-specification.md)  
**Verification Date**: 2026-03-06 (Idle-Time Spec Conformance Sweep SWEEP-001)

**Critical Issues**:
- ❌ Command Layer structure completely wrong (cmd/muster/ doesn't exist - actual: cmd/openapi-cli/, cmd/mcp-server/, cmd/daemon/)
- ❌ Service Layer doesn't exist (pkg/service/ - actual: distributed across pkg/request/, pkg/response/, etc.)
- ❌ Output Layer doesn't exist (pkg/output/ - actual: pkg/ui/)
- ❌ Testing architecture directories are wrong (not tests/integration/, tests/e2e/)

**Recommendation**: Refer to [master-specification.md](./master-specification.md) for authoritative architecture documentation.

---

# Architecture Overview

This document describes the architecture of Muster CLI.

## System Overview

```
┌─────────────────────────────────────────────────────────┐
│                      User Interface                      │
│                      (CLI Commands)                      │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│                   Command Layer                        │
│              (cmd/muster/)                             │
│   • Argument parsing (cobra)                           │
│   • Flag validation                                    │
│   • Help text generation                               │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│                   Service Layer                        │
│              (pkg/service/)                            │
│   • Business logic                                     │
│   • Resource management                                │
│   • Authentication handling                            │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│                   Client Layer                         │
│              (pkg/client/)                             │
│   • HTTP client                                        │
│   • Request/response handling                          │
│   • Retry logic                                        │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│                    API Layer                           │
│              (pkg/api/)                                │
│   • Request builders                                   │
│   • Response parsers                                   │
│   • Error handling                                     │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│                   Config Layer                         │
│              (pkg/config/)                             │
│   • Config file management                             │
│   • Profile handling                                   │
│   • Environment variables                              │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│                   Auth Layer                           │
│              (pkg/auth/)                               │
│   • OAuth2 flow                                        │
│   • API key management                                 │
│   • mTLS handling                                      │
│   • Token storage                                      │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│                   Output Layer                         │
│              (pkg/output/)                             │
│   • Table formatting                                   │
│   • JSON/YAML marshaling                               │
│   • Color support                                      │
└─────────────────────────────────────────────────────────┘
```

## Component Descriptions

### Command Layer

Located in `cmd/muster/`, this layer:
- Defines CLI commands using Cobra
- Parses command-line arguments and flags
- Validates input parameters
- Coordinates service layer calls
- Handles command output

Key files:
- `root.go` - Root command definition
- `list.go` - List command
- `get.go` - Get command
- `auth.go` - Authentication commands

### Service Layer

Located in `pkg/service/`, contains business logic:
- `resource_service.go` - Resource CRUD operations
- `auth_service.go` - Authentication logic
- `config_service.go` - Configuration management

### Client Layer

Located in `pkg/client/`, handles HTTP communication:
- `http_client.go` - HTTP client with retries
- `transport.go` - Custom transport with auth
- `middleware.go` - Logging, metrics middleware

### API Layer

Located in `pkg/api/`, manages API contracts:
- `types.go` - API request/response types
- `resource.go` - Resource API endpoints
- `error.go` - Error response handling

### Config Layer

Located in `pkg/config/`, manages configuration:
- `config.go` - Config struct and loading
- `profile.go` - Profile management
- `env.go` - Environment variable parsing

### Auth Layer

Located in `pkg/auth/`, handles authentication:
- `oauth.go` - OAuth2 flow implementation
- `apikey.go` - API key management
- `mtls.go` - mTLS certificate handling
- `storage.go` - Secure credential storage

### Output Layer

Located in `pkg/output/`, formats output:
- `table.go` - Table formatting
- `json.go` - JSON marshaling
- `yaml.go` - YAML marshaling
- `color.go` - Color support

## Data Flow

### Command Execution Flow

```
User Input → CLI Parser → Command Handler → Service Layer
                                                  │
                    Response ← Output Formatter ←┘
```

### API Request Flow

```
Service Layer → API Builder → HTTP Client → Server
                                                  │
           Response ← Parser ← Client ←───────────┘
```

### Authentication Flow

```
Auth Request → Auth Method (OAuth2/API Key/mTLS) → Token/Credentials
                                                                       │
                    Valid Request ← Auth Middleware ←─────────────────┘
```

## Configuration Flow

1. Load defaults
2. Load config file
3. Apply environment variables
4. Apply command-line flags
5. Load profile-specific settings
6. Override with auth credentials

## MCP Server Architecture

The MCP server exposes CLI functionality via the Model Context Protocol:

```
Claude Desktop ←→ MCP Server ←→ Service Layer
                    │
            ├──────┴──────┐
            ▼             ▼
      Tool Handlers  Resource Handlers
```

## Testing Architecture

- **Unit Tests**: `*_test.go` alongside source files
- **Integration Tests**: `tests/integration/`
- **E2E Tests**: `tests/e2e/`
- **Mocks**: `pkg/mocks/`

## Dependencies

### Production
- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Configuration
- `gopkg.in/yaml.v3` - YAML parsing
- `github.com/fatih/color` - Terminal colors
- `github.com/olekukonko/tablewriter` - Table output

### Development
- `github.com/stretchr/testify` - Testing utilities
- `github.com/golang/mock` - Mock generation
- `github.com/golangci/golangci-lint` - Linting
