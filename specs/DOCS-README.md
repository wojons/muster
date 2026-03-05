# Muster

> Dynamic API Client & MCP Server - Auto-configuring, versioned, always up-to-date

[![Build Status](https://github.com/anomalyco/muster/workflows/CI/badge.svg)](https://github.com/anomalyco/muster/actions)
[![Coverage](https://codecov.io/gh/anomalyco/muster/branch/main/graph/badge.svg)](https://codecov.io/gh/anomalyco/muster)
[![Version](https://img.shields.io/github/v/release/anomalyco/muster)](https://github.com/anomalyco/muster/releases)
[![License](https://img.shields.io/github/license/anomalyco/muster)](LICENSE)

Muster is a dynamic API client that configures itself from remote inventory. Instead of installing stale SDKs or manually updating MCP packages, Muster downloads configuration from your API's `_inventory.json`, caches it locally, and generates commands automatically from your OpenAPI spec. Works offline, respects versioning, and integrates seamlessly with AI assistants via the Model Context Protocol (MCP).

---

## Features

### CLI Tool Features
- **Dynamic Command Generation** - Commands auto-generated from your API's OpenAPI spec
- **Offline Caching** - Inventory and specs cached locally with HTTP cache header support
- **Version Management** - Pin versions, use latest, or fallback to "last known good"
- **Alias System** - Create shortcuts for frequently used hosts and versions
- **Multiple Output Formats** - JSON, YAML, table, and custom formatters
- **Built-in Auth** - mTLS, OAuth2, Bearer tokens, with OS keychain integration

### MCP Server Features
- **Zero-Config AI Integration** - AI assistants get instant API access with no manual setup
- **Tool Auto-Discovery** - MCP tools generated dynamically from OpenAPI operations
- **Secure Context Isolation** - Separate auth contexts for inventory vs API calls
- **Version Pinning** - Ensure AI assistants use stable API versions

### Key Capabilities
- **WebAssembly Modules** - Sandboxed execution for custom data transformations
- **DSL Workflows** - Multi-step operations with loops and conditionals
- **Rate Limiting** - Client-side controls with exponential backoff
- **SOCKS5 Proxy** - Built-in proxy support for enterprise environments
- **Client Fingerprinting** - Optional device identification (backend-controlled)

---

## Installation

### Homebrew (macOS & Linux)

```bash
# Add tap
brew tap anomalyco/muster

# Install CLI
brew install muster-cli

# Install MCP server
brew install muster-mcp

# Or install both
brew install muster-cli muster-mcp
```

### Direct Download

Download pre-built binaries from [GitHub Releases](https://github.com/anomalyco/muster/releases):

```bash
# macOS (Apple Silicon)
curl -L https://github.com/anomalyco/muster/releases/latest/download/muster-cli-darwin-arm64 -o muster
chmod +x muster
sudo mv muster /usr/local/bin/

# Linux (x86_64)
curl -L https://github.com/anomalyco/muster/releases/latest/download/muster-cli-linux-amd64 -o muster
chmod +x muster
sudo mv muster /usr/local/bin/

# Windows (PowerShell)
# Download muster-cli-windows-amd64.exe from releases
```

### Go Install

```bash
go install github.com/anomalyco/muster/cmd/muster-cli@latest
go install github.com/anomalyco/muster/cmd/muster-mcp@latest
```

### Docker

```bash
# Run CLI
docker run --rm -it \
  -v ~/.config/muster:/root/.config/muster \
  ghcr.io/anomalyco/muster-cli:latest \
  --host https://api.example.com

# Run MCP server
docker run --rm -it \
  -v ~/.config/muster:/root/.config/muster \
  ghcr.io/anomalyco/muster-mcp:latest \
  --host https://api.example.com
```

---

## Quick Start

### Basic Usage

```bash
# Configure for an API
muster --host https://api.example.com

# List available commands (auto-generated from OpenAPI spec)
muster --help

# Call an API endpoint
muster users list --limit 10
muster users get --id 123
muster posts create --title "Hello" --body "World"
```

### First Command

```bash
# The tool downloads configuration from https://api.example.com/_inventory.json
# and generates commands based on the OpenAPI spec

# View generated commands
muster --host https://api.example.com commands list

# Get help for a specific command
muster users create --help
```

### Authentication

```bash
# Using client certificates (mTLS)
muster --host https://api.example.com \
  --cert ./client.crt \
  --key ./client.key \
  users list

# Using Bearer token from environment
export MUSTER_API_TOKEN="your-token"
muster --host https://api.example.com users list

# Store credentials in OS keychain
muster config set-auth --host https://api.example.com --token
```

### MCP Integration

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "my-api": {
      "command": "muster-mcp",
      "args": ["--host", "https://api.example.com"],
      "env": {
        "MUSTER_CERT_FILE": "./client.crt",
        "MUSTER_KEY_FILE": "./client.key"
      }
    }
  }
}
```

---

## Documentation

- **[Full Documentation](https://docs.muster.dev)** - Complete guides and API reference
- **[Installation Guide](https://docs.muster.dev/install)** - Detailed installation instructions
- **[Configuration Reference](https://docs.muster.dev/config)** - All configuration options
- **[MCP Integration](https://docs.muster.dev/mcp)** - AI assistant setup
- **[Examples](https://docs.muster.dev/examples)** - Common use cases and patterns

### Quick Examples

```bash
# Version management
muster --host https://api.example.com --version 1.2.3 users list
muster --host https://api.example.com --use-known-good users list
muster alias create prod https://api.example.com --version 1.2.3
muster use prod

# Working offline
muster --offline users list

# Custom output format
muster users list --output json | jq '.[].name'
muster users list --format "{{.id}}: {{.name}}"

# Multi-step workflow
muster workflow run deployment.yaml
```

---

## Configuration

### Config File Location

Configuration is stored in YAML format:

- **macOS**: `~/Library/Application Support/muster/config.yaml`
- **Linux**: `~/.config/muster/config.yaml`
- **Windows**: `%AppData%\muster\config.yaml`

Override with `--config /path/to/config.yaml`

### Environment Variables

All settings can be configured via environment variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `MUSTER_HOST` | Default API host | `https://api.example.com` |
| `MUSTER_CERT_FILE` | Client certificate path | `/path/to/cert.pem` |
| `MUSTER_KEY_FILE` | Client key path | `/path/to/key.pem` |
| `MUSTER_API_TOKEN` | API bearer token | `secret-token` |
| `MUSTER_INVENTORY_TOKEN` | Inventory auth token | `inv-token` |
| `MUSTER_OFFLINE` | Force offline mode | `true` |
| `MUSTER_PROXY` | SOCKS5 proxy URL | `socks5://localhost:1080` |
| `MUSTER_CONFIG_DIR` | Config directory path | `~/.muster` |
| `MUSTER_CACHE_DIR` | Cache directory path | `~/.cache/muster` |

Priority order:
1. CLI flags
2. Environment variables (`MUSTER_*`)
3. Config file
4. Inventory-provided defaults
5. Built-in defaults

---

## Development

### Build from Source

```bash
# Clone repository
git clone https://github.com/anomalyco/muster.git
cd muster

# Install dependencies
make deps

# Build all binaries
make build

# Build specific binary
make build-cli
make build-mcp
make build-daemon

# Run tests
make test

# Run with race detector
make test-race
```

### Prerequisites

- Go 1.23 or later
- Make
- Git

### Project Structure

```
.
├── cmd/
│   ├── muster-cli/      # CLI entry point
│   ├── muster-mcp/      # MCP server entry point
│   └── muster-daemon/   # Daemon entry point
├── internal/
│   ├── cli/             # CLI commands and formatters
│   ├── mcp/             # MCP server implementation
│   ├── daemon/          # Background service
│   ├── inventory/       # Inventory fetching & caching
│   ├── openapi/         # OpenAPI spec parsing
│   ├── wasm/            # WebAssembly runtime
│   ├── config/          # Configuration management
│   └── version/         # Version info
├── pkg/
│   └── ...              # Public APIs
└── specs/               # Design specifications
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

### Development Commands

```bash
# Format code
make fmt

# Run linters
make lint

# Generate mocks
make generate

# Build for all platforms
make build-all

# Create release
make release VERSION=v1.0.0

# Clean build artifacts
make clean
```

---

## License

This project is dual-licensed under:

- **[MIT License](LICENSE-MIT)** - See [LICENSE-MIT](LICENSE-MIT) for details
- **[Apache License 2.0](LICENSE-APACHE)** - See [LICENSE-APACHE](LICENSE-APACHE) for details

You may choose either license at your option.

---

## Support

- **Issues**: [GitHub Issues](https://github.com/anomalyco/muster/issues)
- **Discussions**: [GitHub Discussions](https://github.com/anomalyco/muster/discussions)
- **Security**: Please report security vulnerabilities to security@muster.dev

---

## Acknowledgments

Built with:
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Viper](https://github.com/spf13/viper) - Configuration management
- [Kin-OpenAPI](https://github.com/getkin/kin-openapi) - OpenAPI validation
- [Wasmtime](https://github.com/bytecodealliance/wasmtime-go) - WebAssembly runtime
