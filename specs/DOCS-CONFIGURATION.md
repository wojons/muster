# Configuration Guide

Learn how to configure Muster CLI for your environment.

## Config File Location

Muster looks for configuration in the following order:

1. `--config` flag value
2. `MUSTER_CONFIG` environment variable
3. `~/.config/muster/config.yaml` (default)
4. `~/.muster/config.yaml` (legacy)

## Config File Format

Muster uses YAML format:

```yaml
# ~/.config/muster/config.yaml
api:
  url: https://api.muster.example.com
  timeout: 30

auth:
  method: oauth2  # oauth2, apikey, mtls
  
defaults:
  output: table
  namespace: default
  
profiles:
  production:
    api:
      url: https://api.prod.muster.example.com
    auth:
      method: apikey
      api_key: ${MUSTER_PROD_API_KEY}
      
  staging:
    api:
      url: https://api.staging.muster.example.com
```

## Environment Variables

All config options can be set via environment variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `MUSTER_CONFIG` | Config file path | `/path/to/config.yaml` |
| `MUSTER_PROFILE` | Default profile | `production` |
| `MUSTER_API_URL` | API endpoint URL | `https://api.example.com` |
| `MUSTER_API_TIMEOUT` | Request timeout (seconds) | `30` |
| `MUSTER_OUTPUT` | Default output format | `json` |
| `MUSTER_API_KEY` | API key for auth | `key-12345` |
| `MUSTER_MTLS_CERT` | mTLS certificate path | `/path/to/cert.pem` |
| `MUSTER_MTLS_KEY` | mTLS key path | `/path/to/key.pem` |
| `MUSTER_DEBUG` | Enable debug logging | `true` |

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
muster config set api.url https://api.prod.com --profile production
```

### Using Profiles

```bash
# One-time use
muster list --profile production

# Set default
export MUSTER_PROFILE=production
muster list
```

## Configuration Commands

### View Configuration

```bash
# View all config
muster config get

# View specific key
muster config get api.url

# View profile config
muster config get --profile production
```

### Set Configuration

```bash
# Set value
muster config set api.timeout 60

# Set for specific profile
muster config set defaults.output json --profile production
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
