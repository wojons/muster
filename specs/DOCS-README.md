# OpenAPI CLI

> OpenAPI CLI + MCP Server - Dynamic command generation from OpenAPI specifications

[![Build Status](https://github.com/openapi-cli/openapi-cli/workflows/CI/badge.svg)](https://github.com/openapi-cli/openapi-cli/actions)
[![Coverage](https://codecov.io/gh/openapi-cli/openapi-cli/branch/main/graph/badge.svg)](https://codecov.io/gh/openapi-cli/openapi-cli)

OpenAPI CLI is a dynamic CLI tool that generates commands from OpenAPI specifications and serves them via MCP protocol. Instead of installing SDKs or manually writing API clients, OpenAPI CLI automatically generates CLI commands from any OpenAPI specification. Works offline, integrates with Claude Desktop via MCP, and provides secure authentication with OS keychain integration.

---

## Features

### CLI Tool Features
- **Dynamic Command Generation** - Commands auto-generated from your API's OpenAPI spec
- **Offline Caching** - Inventory and specs cached locally with HTTP cache header support
- **Version Management** - Pin versions, use latest, or fallback to "last known good"
- **Multiple Output Formats** - JSON, YAML, table, and custom formatters
- **Built-in Auth** - OAuth2, mTLS, API keys, with OS keychain integration

### MCP Server Features
- **Zero-Config AI Integration** - AI assistants get instant API access with no manual setup
- **Tool Auto-Discovery** - MCP tools generated dynamically from OpenAPI operations
- **Secure Context Isolation** - Separate auth contexts for inventory vs API calls
- **Version Pinning** - Ensure AI assistants use stable API versions

### Key Capabilities
- **WebAssembly Modules** - Sandboxed execution for custom data transformations
- **Starlark DSL** - Multi-step operations with loops and conditionals
- **Rate Limiting** - Client-side controls with token bucket algorithm
- **Unix Socket Daemon** - Background daemon with socket proxy support

---

## Installation

### Homebrew (macOS & Linux)

```bash
# Add tap
brew tap openapi-cli/openapi-cli

# Install CLI
brew install openapi-cli
```

### Go Install

```bash
go install github.com/openapi-cli/openapi-cli/cmd/openapi-cli@latest
```

### Direct Download

Download pre-built binaries from [GitHub Releases](https://github.com/openapi-cli/openapi-cli/releases):

```bash
# macOS (Apple Silicon)
curl -L https://github.com/openapi-cli/openapi-cli/releases/latest/download/openapi-cli-darwin-arm64 -o openapi-cli
chmod +x openapi-cli
sudo mv openapi-cli /usr/local/bin/

# Linux (x86_64)
curl -L https://github.com/openapi-cli/openapi-cli/releases/latest/download/openapi-cli-linux-amd64 -o openapi-cli
chmod +x openapi-cli
sudo mv openapi-cli /usr/local/bin/
```

### Docker

```bash
# Run CLI
docker run --rm -it \
  -v ~/.config/openapi-cli:/root/.config/openapi-cli \
  ghcr.io/openapi-cli/openapi-cli:latest \
  --spec https://api.example.com/openapi.yaml
```

---

## Quick Start

### Basic Usage

```bash
# Initialize OpenAPI CLI
openapi-cli init

# Authenticate with OAuth2
openapi-cli auth login --oauth2

# Fetch an OpenAPI specification
openapi-cli inventory fetch https://api.example.com/openapi.yaml

# Generate commands
openapi-cli generate https://api.example.com/openapi.yaml

# Use generated commands
openapi-cli pets list
openapi-cli pets get --id 42
openapi-cli pets create --name "Fluffy" --type "cat"
```

### Direct Request Mode

```bash
# GET request
openapi-cli request https://api.example.com/pets

# POST request
openapi-cli request https://api.example.com/pets POST '{"name":"Fluffy","type":"cat"}'

# PUT request
openapi-cli request https://api.example.com/pets/42 PUT '{"name":"Fluffy Jr"}'

# DELETE request
openapi-cli request https://api.example.com/pets/42 DELETE
```

### MCP Integration

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "openapi": {
      "command": "/usr/local/bin/openapi-cli",
      "args": ["mcp-server"],
      "env": {
        "OPENAPI_CONFIG": "/path/to/config.yaml"
      }
    }
  }
}
```

---

## Documentation

- **[Quick Start Guide](DOCS-OPENAPI-QUICKSTART.md)** - Getting started with OpenAPI CLI
- **[Configuration](DOCS-CONFIGURATION.md)** - Detailed configuration options
- **[MCP Protocol](MCP-PROTOCOL.md)** - MCP server integration
- **[Authentication](API-AUTHENTICATION.md)** - OAuth2, mTLS, and API keys

---

## Configuration

### Config File Location

Configuration is stored in YAML format:

- **macOS**: `~/Library/Application Support/openapi-cli/config.yaml`
- **Linux**: `~/.config/openapi-cli/config.yaml`
- **Windows**: `%AppData%\openapi-cli\config.yaml`

### Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `OPENAPI_CONFIG` | Config file path | `/path/to/config.yaml` |
| `OPENAPI_PROFILE` | Default profile | `default` |
| `OPENAPI_DEBUG` | Enable debug logging | `true` |

### Default Directory Structure

OpenAPI CLI uses standard XDG Base Directory paths:

| Directory | Location (Linux) | Location (macOS) | Location (Windows) |
|-----------|------------------|------------------|---------------------|
| Config | `~/.config/openapi-cli/` | `~/Library/Application Support/openapi-cli/` | `%APPDATA%\openapi-cli\` |
| Cache | `~/.cache/openapi-cli/` | `~/Library/Caches/openapi-cli/` | `%LOCALAPPDATA%\openapi-cli\` |
| Data | `~/.local/share/openapi-cli/` | `~/Library/Application Support/openapi-cli/` | `%LOCALAPPDATA%\openapi-cli\` |

---

## Development

### Build from Source

```bash
# Clone repository
git clone https://github.com/openapi-cli/openapi-cli.git
cd openapi-cli

# Install dependencies
go mod download

# Build all binaries
make build

# Build specific binary
make build-cli
make build-mcp

# Run tests
make test

# Build for all platforms
make build-all
```

### Prerequisites

- Go 1.25 or later
- Make
- Git

### Project Structure

```
.
├── cmd/
│   ├── openapi-cli/       # CLI entry point
│   ├── mcp-server/        # MCP server entry point
│   └── daemon/            # Daemon entry point
├── pkg/
│   ├── auth/              # Authentication (OAuth2, mTLS, keychain)
│   ├── cache/             # Multi-tier caching
│   ├── cli/               # CLI framework integration
│   ├── client/            # HTTP client
│   ├── config/            # Configuration management
│   ├── dsl/               # Starlark DSL interpreter
│   ├── mcp/               # MCP protocol implementation
│   ├── openapi/           # OpenAPI parsing
│   └── wasm/              # WASM runtime (Wasmtime)
├── internal/
│   ├── builtin/           # Built-in commands
│   ├── commands/          # Command implementations
│   └── ...
├── demo/                  # Demo application
├── specs/                 # 276 specification files
└── .memory-bank/          # Project documentation
```

### Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing`)
3. Make your changes
4. Run tests (`make test`)
5. Commit your changes (`git commit -am 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing`)
7. Open a Pull Request

---

## Commands

### Built-in Commands

```bash
# Authentication
openapi-cli auth login [--oauth2] [--username <user>] [--password <pass>] [--token <token>]
openapi-cli auth logout
openapi-cli auth status

# Configuration
openapi-cli config show

# Inventory management
openapi-cli inventory fetch <url>
openapi-cli inventory list
openapi-cli inventory validate <spec-path>

# Command generation
openapi-cli generate <spec-path>

# Daemon control
openapi-cli daemon start
openapi-cli daemon stop
openapi-cli daemon status

# Shell completion
openapi-cli completion [bash|zsh|fish|powershell]

# Help and version
openapi-cli help [command]
openapi-cli version
```

### Generated Commands

After loading an OpenAPI specification, commands are dynamically generated:

```bash
# Example generated commands
openapi-cli pets list
openapi-cli pets get --id 42
openapi-cli pets create --name "Fluffy" --type "cat"
openapi-cli pets update --id 42 --name "Fluffy Jr"
openapi-cli pets delete --id 42
```

---

## License

This project is distributed under the MIT License.

---

## Support

- **Issues**: [GitHub Issues](https://github.com/openapi-cli/openapi-cli/issues)
- **Discussions**: [GitHub Discussions](https://github.com/openapi-cli/openapi-cli/discussions)

---

## Acknowledgments

Built with:
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Viper](https://github.com/spf13/viper) - Configuration management
- [Kin-OpenAPI](https://github.com/getkin/kin-openapi) - OpenAPI parsing
- [Wasmtime](https://github.com/bytecodealliance/wasmtime-go) - WASM runtime
- [Starlark](https://go.starlark.net) - DSL interpreter
