# CLI Reference

Complete reference for all Muster CLI commands.

## Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--config` | Config file path | `~/.config/muster/config.yaml` |
| `--profile` | Configuration profile | `default` |
| `--output, -o` | Output format (json, yaml, table) | `table` |
| `--verbose, -v` | Enable verbose logging | `false` |
| `--version` | Show version | - |

## Commands

### `muster auth`

Manage authentication.

#### `muster auth login`

Authenticate interactively with OAuth2.

```bash
muster auth login [--profile <name>]
```

Flags:
- `--profile` - Save to specific profile

#### `muster auth configure`

Configure authentication manually.

```bash
muster auth configure --api-key <key> [--profile <name>]
muster auth configure --mtls-cert <path> --mtls-key <path>
```

Flags:
- `--api-key` - API key for authentication
- `--mtls-cert` - mTLS certificate path
- `--mtls-key` - mTLS key path
- `--mtls-ca` - CA certificate path
- `--profile` - Save to specific profile

#### `muster auth status`

Check authentication status.

```bash
muster auth status
```

#### `muster auth logout`

Clear authentication.

```bash
muster auth logout [--profile <name>]
```

### `muster list`

List resources.

```bash
muster list [--type <type>] [--namespace <ns>] [--limit <n>] [--sort-by <field>]
```

Flags:
- `--type, -t` - Filter by resource type
- `--namespace, -n` - Filter by namespace
- `--limit, -l` - Limit results (default: 50)
- `--sort-by` - Sort field (created, updated, name)
- `--all, -a` - Show all resources

Example:
```bash
muster list --type deployment --namespace prod --limit 20
```

### `muster get`

Get resource details.

```bash
muster get <resource> [--output <format>]
```

Arguments:
- `<resource>` - Resource identifier (type/name)

Flags:
- `--output, -o` - Output format: json, yaml, table

Example:
```bash
muster get deployment/web-app -o yaml
```

### `muster create`

Create resources from files.

```bash
muster create -f <file> [--namespace <ns>]
```

Flags:
- `--file, -f` - File path or stdin (-)
- `--namespace, -n` - Target namespace
- `--dry-run` - Validate without creating

Example:
```bash
muster create -f service.yaml -n production
```

### `muster update`

Update existing resources.

```bash
muster update <resource> -f <file>
```

Flags:
- `--file, -f` - File with updates
- `--dry-run` - Validate without updating

### `muster delete`

Delete resources.

```bash
muster delete <resource> [--force]
```

Flags:
- `--force, -f` - Skip confirmation prompt

### `muster search`

Search across resources.

```bash
muster search --query <query> [--limit <n>]
```

Flags:
- `--query, -q` - Search query
- `--limit, -l` - Limit results

### `muster config`

Manage configuration.

#### `muster config get`

```bash
muster config get <key>
```

#### `muster config set`

```bash
muster config set <key> <value> [--profile <name>]
```

#### `muster config profiles`

```bash
muster config profiles
```

### `muster completion`

Generate shell completion scripts.

```bash
muster completion bash
muster completion zsh
muster completion fish
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid command/usage |
| 3 | Authentication error |
| 4 | Resource not found |
| 5 | Permission denied |
| 6 | Network error |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `MUSTER_CONFIG` | Config file path |
| `MUSTER_PROFILE` | Default profile |
| `MUSTER_API_KEY` | API key for auth |
| `MUSTER_API_URL` | API endpoint URL |
| `MUSTER_DEBUG` | Enable debug logging |
