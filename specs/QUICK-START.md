# Quick Start Guide

## For Developers

### Project Structure
```
src/
├── cli/              # CLI entry point and commands
│   ├── index.ts      # CLI entry
│   └── commands/     # Dynamic command handlers
├── mcp/              # MCP server implementation
│   ├── index.ts      # MCP entry point
│   └── tools.ts      # Tool generation from OpenAPI
├── core/             # Shared core (both CLI and MCP use this)
│   ├── inventory/    # Inventory fetching and caching
│   ├── config/       # Configuration management
│   ├── auth/         # Authentication handlers
│   ├── crypto/       # Cryptographic operations
│   ├── wasm/         # WASM runtime
│   ├── dsl/          # DSL interpreter
│   └── http/         # HTTP client with middleware
└── types/            # TypeScript type definitions
```

### Running Locally

```bash
# Install dependencies
npm install

# Build
npm run build

# Run CLI
./dist/cli.js --host https://api.example.com --help

# Run MCP server
./dist/mcp.js --host https://api.example.com
```

### Environment Variables

```bash
# Required
export OPENAPICLI_HOST=https://api.example.com

# Optional
export OPENAPICLI_INVENTORY_URL=https://api.example.com/_inventory.json
export OPENAPICLI_CERT_FILE=./client.crt
export OPENAPICLI_KEY_FILE=./client.key
export OPENAPICLI_CA_FILE=./ca.crt
export OPENAPICLI_CONFIG_FILE=~/.config/openapi-cli/config.yaml
```

### Testing

```bash
# Unit tests
npm test

# Integration tests
npm run test:integration

# Test with mock inventory
npm run test:mock
```

## For Users

### Installation

```bash
# Via npm
npm install -g @openapi-cli/core

# Via npx (no install)
npx @openapi-cli/core --host https://api.example.com

# Via binary (download from releases)
./openapi-cli --host https://api.example.com
```

### Basic Usage

```bash
# First run - downloads inventory
openapi-cli --host https://api.example.com --help

# Use generated commands
openapi-cli users list
openapi-cli users get --id 123

# With auth
openapi-cli --host https://api.example.com \
  --header "Authorization: Bearer token" \
  users list
```

### Configuration File

Create `~/.config/openapi-cli/config.yaml`:

```yaml
profiles:
  prod:
    host: https://api.example.com
    cert_file: ./prod.crt
    key_file: ./prod.key
    
  staging:
    host: https://staging-api.example.com
    header:
      Authorization: Bearer staging-token

# Default settings
default_profile: prod
rate_limiting:
  requests_per_minute: 100
```

Then use:
```bash
openapi-cli use prod
openapi-cli users list  # Uses prod profile
```

### MCP Setup (Claude, Cursor, etc.)

Add to your MCP config:

```json
{
  "mcpServers": {
    "my-api": {
      "command": "npx",
      "args": [
        "@openapi-cli/core",
        "--mcp-server",
        "--host",
        "https://api.example.com"
      ],
      "env": {
        "OPENAPICLI_CERT_FILE": "/path/to/cert.pem"
      }
    }
  }
}
```

## Common Tasks

### Working Offline

```bash
# Inventory is cached, works offline
openapi-cli --host https://api.example.com users list

# See cached versions
openapi-cli versions --cached

# Use specific cached version
openapi-cli --version 1.2.3 users list
```

### Managing Versions

```bash
# Use latest (default)
openapi-cli --host https://api.example.com --use-latest

# Use last known good
openapi-cli --use-known-good

# Mark current as known good
openapi-cli mark-good

# Pin to version
openapi-cli --version 1.2.3 --pin
```

### Aliases

```bash
# Create alias
openapi-cli alias create prod https://api.example.com --version 1.2.3

# List aliases
openapi-cli alias list

# Use alias
openapi-cli use prod

# Update alias
openapi-cli alias update prod --version 1.2.4
```

### mTLS (Client Certificates)

```bash
# Via CLI flags
openapi-cli \
  --host https://api.example.com \
  --cert ./client.crt \
  --key ./client.key \
  --ca ./ca.crt \
  users list

# Via environment
export OPENAPICLI_CERT_FILE=./client.crt
export OPENAPICLI_KEY_FILE=./client.key
openapi-cli --host https://api.example.com users list

# Via config file
# config.yaml:
# host: https://api.example.com
# cert_file: ./client.crt
# key_file: ./client.key
```

### Secrets

```bash
# Store in OS keychain
openapi-cli secret set api-token "my-secret-token"

# Use in commands
openapi-cli --host https://api.example.com \
  --secret-header "Authorization: Bearer api-token" \
  users list
```

## Troubleshooting

### Debug Mode

```bash
# Enable debug logging
openapi-cli --debug --host https://api.example.com users list
```

### Check Inventory

```bash
# Show current inventory info
openapi-cli info

# Force refresh
openapi-cli refresh

# Clear cache
openapi-cli cache clear
```

### Common Issues

**Issue**: "Cannot fetch inventory"  
**Solution**: Check HTTPS URL and TLS certificates

**Issue**: "WASM module failed to load"  
**Solution**: Check module signature and integrity

**Issue**: "Auth failed"  
**Solution**: Verify credentials and auth type in inventory

**Issue**: "Rate limited"  
**Solution**: Adjust client-side rate limiting or contact API provider

## Next Steps

- Read [EXECUTIVE-SUMMARY.md](EXECUTIVE-SUMMARY.md) for full overview
- Read [planning-artifacts/PRD.md](planning-artifacts/PRD.md) for detailed requirements
- Read [planning-artifacts/architecture.md](planning-artifacts/architecture.md) for technical design
- Pick an epic from [planning-artifacts/epics/](planning-artifacts/epics/) to implement
