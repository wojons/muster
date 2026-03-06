# Quick Start Guide

Get up and running with Muster in 5 minutes.

## First Command

After installation, verify Muster is working:

```bash
muster --version
```

## Authentication

Muster requires authentication to access your data:

```bash
# Interactive login
muster auth login

# Or configure API key
muster auth configure --api-key YOUR_API_KEY
```

## Basic Usage

### List Resources

```bash
# List all resources
muster list

# List with filters
muster list --type deployment --namespace production
```

### Get Details

```bash
# Get resource details
muster get deployment/web-app

# Get in YAML format
muster get deployment/web-app -o yaml
```

### Create Resources

```bash
# Create from file
muster create -f resource.yaml

# Create from stdin
cat resource.yaml | muster create -f -
```

## Common Examples

### Example 1: View Recent Changes

```bash
muster list --sort-by updated --limit 10
```

### Example 2: Export Configuration

```bash
muster get deployment/web-app -o json > backup.json
```

### Example 3: Search Resources

```bash
muster search --query "status:failed"
```

## Next Steps

- Read the [CLI Reference](DOCS-CLI-REFERENCE.md) for all commands
- Learn about [Configuration](DOCS-CONFIGURATION.md)
- Set up [MCP Integration](DOCS-MCP-SETUP.md) for Claude Desktop
