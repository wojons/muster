# MCP Setup Guide

Configure Muster as an MCP (Model Context Protocol) server for Claude Desktop.

## What is MCP?

Model Context Protocol (MCP) allows Claude Desktop to interact with external tools and data sources. Muster provides an MCP server that exposes CLI functionality to Claude.

## Claude Desktop Configuration

### Step 1: Locate Config File

Find your Claude Desktop configuration:

- **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows**: `%APPDATA%\Claude\claude_desktop_config.json`
- **Linux**: `~/.config/Claude/claude_desktop_config.json`

### Step 2: Add Muster MCP Server

Edit the configuration file and add the Muster MCP server:

```json
{
  "mcpServers": {
    "muster": {
      "command": "muster",
      "args": ["mcp", "serve"],
      "env": {
        "MUSTER_PROFILE": "default",
        "MUSTER_API_KEY": "your-api-key-here"
      }
    }
  }
}
```

### Step 3: Restart Claude Desktop

Quit and reopen Claude Desktop to load the new MCP server.

### Step 4: Verify Connection

In Claude Desktop, you should see the Muster tools available. Try asking:

> "What resources are available in my Muster environment?"

## Configuration Options

### Using Environment Variables

```json
{
  "mcpServers": {
    "muster": {
      "command": "muster",
      "args": ["mcp", "serve"],
      "env": {
        "MUSTER_PROFILE": "production",
        "MUSTER_CONFIG": "/path/to/config.yaml",
        "MUSTER_DEBUG": "true"
      }
    }
  }
}
```

### Using Profiles

Configure different profiles for different environments:

```json
{
  "mcpServers": {
    "muster-prod": {
      "command": "muster",
      "args": ["mcp", "serve", "--profile", "production"]
    },
    "muster-dev": {
      "command": "muster",
      "args": ["mcp", "serve", "--profile", "development"]
    }
  }
}
```

## Troubleshooting

### MCP Server Not Connecting

**Symptom**: Claude doesn't show Muster tools

**Solutions**:
1. Verify muster is in your PATH: `which muster`
2. Check config file syntax with `jq` or JSON validator
3. Check Claude Desktop logs for errors

### Authentication Errors

**Symptom**: Tools return authentication errors

**Solutions**:
1. Verify API key is set in environment variables
2. Run `muster auth status` to check CLI authentication
3. Re-authenticate with `muster auth login`

### Permission Denied

**Symptom**: Cannot execute muster command

**Solutions**:
1. Check file permissions on muster binary
2. Ensure muster is in a directory accessible to Claude Desktop
3. Try using the full path to the muster binary

## Available MCP Tools

Once configured, Claude can use these Muster tools:

- `muster_list` - List resources
- `muster_get` - Get resource details
- `muster_search` - Search resources
- `muster_create` - Create resources
- `muster_update` - Update resources
- `muster_delete` - Delete resources
