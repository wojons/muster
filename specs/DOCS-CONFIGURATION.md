# Configuration Guide

Learn how to configure OpenAPI CLI for your environment.

## Config File Location

OpenAPI CLI looks for configuration in the following order:

1. `--config` flag value
2. `OPENAPI_CONFIG` environment variable
3. `~/.config/openapi-cli/config.yaml` (default)
4. `~/.openapi-cli/config.yaml` (legacy)

## Config File Format

OpenAPI CLI uses YAML format:

```yaml
# ~/.config/openapi-cli/config.yaml
api:
  url: https://api.example.com
  timeout: 30

auth:
  method: oauth2  # oauth2, apikey, mtls

defaults:
  output: table
  namespace: default

profiles:
  production:
    api:
      url: https://api.prod.example.com
    auth:
      method: apikey
      api_key: ${OPENAPI_PROD_API_KEY}

  staging:
    api:
      url: https://api.staging.example.com
```

## Environment Variables

All config options can be set via environment variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `OPENAPI_CONFIG` | Config file path | `/path/to/config.yaml` |
| `OPENAPI_PROFILE` | Default profile | `production` |
| `OPENAPI_API_URL` | API endpoint URL | `https://api.example.com` |
| `OPENAPI_API_TIMEOUT` | Request timeout (seconds) | `30` |
| `OPENAPI_OUTPUT` | Default output format | `json` |
| `OPENAPI_API_KEY` | API key for auth | `key-12345` |
| `OPENAPI_MTLS_CERT` | mTLS certificate path | `/path/to/cert.pem` |
| `OPENAPI_MTLS_KEY` | mTLS key path | `/path/to/key.pem` |
| `OPENAPI_DEBUG` | Enable debug logging | `true` |

## Profiles

Profiles allow different configurations for different environments.

### Creating Profiles

Via config file:
```yaml
profiles:
  dev:
    api:
      url: http://localhost:8080
  prod:
    api:
      url: https://api.production.com
```

Via CLI:
```bash
openapi-cli config set api.url https://api.prod.com --profile production
```

### Using Profiles

```bash
# One-time use
openapi-cli list --profile production

# Set default
export OPENAPI_PROFILE=production
openapi-cli list
```

## Configuration Commands

### View Configuration

```bash
# View all config
openapi-cli config get

# View specific key
openapi-cli config get api.url

# View profile config
openapi-cli config get --profile production
```

### Set Configuration

```bash
# Set value
openapi-cli config set api.timeout 60

# Set for specific profile
openapi-cli config set defaults.output json --profile production
```

## Advanced Configuration

### HTTP Proxy

```yaml
proxy:
  url: http://proxy.company.com:8080
  username: user
  password: ${PROXY_PASSWORD}
```

### Custom Headers

```yaml
api:
  headers:
    X-Custom-Header: value
    X-Request-ID: ${REQUEST_ID}
```

### TLS Settings

```yaml
tls:
  insecure_skip_verify: false  # Only for testing
  ca_file: /path/to/custom-ca.pem
```
