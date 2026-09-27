# Muster Documentation

Welcome to the comprehensive documentation for **Muster** - a modern CLI tool for OpenAPI specification interaction, MCP serving, and WASM execution.

## Quick Links

- [Installation Guide](guides/installation.md) - Get started in 5 minutes
- [Quickstart Tutorial](guides/quickstart.md) - Your first project
- [API Documentation](api/README.md) - Complete API documentation
- [Command Reference](guides/commands.md) - All CLI commands

## Documentation Structure

### For Users
- [Installation](guides/installation.md) - Platform-specific setup
- [Quickstart](guides/quickstart.md) - 5-minute tutorial
- [Configuration](guides/configuration.md) - Customize your setup
- [Commands](guides/commands.md) - Complete command reference
- [Starlark DSL](guides/starlark-dsl.md) - Scripting and automation
- [Troubleshooting](guides/troubleshooting.md) - Common issues and solutions

### For Developers
- [Architecture Overview](architecture/overview.md) - System design
- [API Documentation](api/README.md) - REST API reference
- [REST API Reference](api/rest-api.md) - Complete REST API documentation
- [MCP Protocol](api/mcp-protocol.md) - Model Context Protocol
- [Development Setup](contributing/setup.md) - Development environment

### Code Examples
- [Repository Examples](examples/repository-examples.md) - Database operations and persistence
- [MCP Handlers Examples](examples/mcp-handlers-examples.md) - MCP protocol handlers
- [Cache Examples](examples/cache-examples.md) - Multi-tier caching patterns
- [DSL Examples](examples/dsl-examples.md) - Starlark scripting examples
- [Daemon Examples](examples/daemon-examples.md) - Background service management

### Technical Documentation
- [Technical Specifications](../specs/) - Formal requirements

## Getting Started

### Install

Muster has no published Homebrew taps, package repositories, or pre-built
release binaries — the only supported installation is **build from source**.
The repository is currently private, so you need read access to
`github.com/wojons/muster` first.

Quick path (see the [Installation Guide](guides/installation.md) for
prerequisites, all platforms, and troubleshooting):

```bash
git clone https://github.com/wojons/muster.git
cd muster
make build-cli                 # requires Go 1.26.6+ and a C compiler (CGO)
make build-mcp                 # optional: MCP server -> bin/openapi-mcp
export PATH="$PWD/bin:$PATH"   # binaries land in bin/
```

### First Steps

```bash
# Verify installation
openapi-cli --version

# Explore available commands
openapi-cli --help

# Run the MCP server (separate binary; build it with `make build-mcp`)
openapi-mcp --spec openapi.yaml
```

## Key Features

- **OpenAPI Integration**: Parse and interact with any OpenAPI 3.0/3.1 specification
- **Dynamic CLI Generation**: Automatically generate CLI commands from API definitions
- **MCP Server Support**: Model Context Protocol server for AI assistant integration
- **WASM Runtime**: Secure execution of WebAssembly modules
- **Starlark DSL**: Scriptable automation with embedded programming language
- **Multi-Platform Auth**: OAuth2, mTLS, API key support across platforms
- **Daemon Mode**: Background service with Unix socket proxy

## Documentation Versions

Documentation is always available for the latest stable release:

- **Latest** - Current stable release ([docs/](https://github.com/wojons/muster/tree/main/docs))
- **In Development** - Work-in-progress features ([repository](https://github.com/wojons/muster))

## Getting Help

- **Issues**: [GitHub Issues](https://github.com/wojons/muster/issues)
- **Discussions**: [GitHub Discussions](https://github.com/wojons/muster/discussions)
- **CLI Help**: Run `openapi-cli --help` or `openapi-cli <command> --help`

## Contributing

We welcome contributions! See [Contributing Guide](contributing/setup.md) for details.

---

**Version**: 1.0.0  
**Last Updated**: 2026-03-02
