# Authentication Guide

Configure authentication for Muster CLI.

## Authentication Methods

Muster supports three authentication methods:

1. **OAuth2** - Interactive browser-based login
2. **API Key** - Static key authentication
3. **mTLS** - Mutual TLS certificate authentication

## OAuth2 Setup

### Interactive Login

The simplest way to authenticate:

```bash
muster auth login
```

This opens a browser window for authentication. After successful login, credentials are stored securely.

### Profile-Specific Login

Authenticate to a specific profile:

```bash
muster auth login --profile production
```

### OAuth2 Configuration

Advanced OAuth2 settings can be configured in `~/.config/muster/config.yaml`:

```yaml
auth:
  oauth:
    client_id: your-client-id
    authorize_url: https://auth.example.com/oauth/authorize
    token_url: https://auth.example.com/oauth/token
    scopes:
      - read
      - write
```

## API Key Setup

### Configure API Key

For automation and CI/CD:

```bash
muster auth configure --api-key YOUR_API_KEY --profile ci
```

Or set via environment variable:

```bash
export MUSTER_API_KEY=your-api-key-here
```

### API Key Best Practices

1. **Store securely**: Use environment variables or secret management
2. **Rotate regularly**: Change keys every 90 days
3. **Scope appropriately**: Use read-only keys where possible
4. **Never commit**: Add to `.gitignore` and use `.env` files

## mTLS Setup

### Certificate Requirements

You need three files:
- Client certificate (PEM format)
- Client private key (PEM format)
- CA certificate (PEM format, optional)

### Configure mTLS

```bash
muster auth configure \
  --mtls-cert /path/to/client.crt \
  --mtls-key /path/to/client.key \
  --mtls-ca /path/to/ca.crt
```

### mTLS with Environment Variables

```bash
export MUSTER_MTLS_CERT=/path/to/client.crt
export MUSTER_MTLS_KEY=/path/to/client.key
export MUSTER_MTLS_CA=/path/to/ca.crt
```

## Authentication Profiles

### Create Profile

```bash
muster auth login --profile production
muster auth configure --api-key KEY --profile staging
```

### Use Profile

```bash
muster list --profile production
muster get deployment/app --profile staging
```

### List Profiles

```bash
muster config profiles
```

## Check Authentication Status

```bash
muster auth status
```

Output:
```
Profile: default
Method: oauth2
Expires: 2024-12-31 23:59:59
Valid: true
```

## Logout

Clear authentication for current profile:

```bash
muster auth logout
```

Clear authentication for specific profile:

```bash
muster auth logout --profile production
```

## Troubleshooting

### "Authentication Required" Error

**Cause**: No valid authentication found

**Solution**:
```bash
muster auth login
# or
muster auth configure --api-key YOUR_KEY
```

### "Token Expired" Error

**Cause**: OAuth2 token has expired

**Solution**:
```bash
muster auth login  # Re-authenticate
```

### "Invalid API Key" Error

**Cause**: API key is incorrect or revoked

**Solution**:
1. Verify key is correct
2. Generate new key from dashboard
3. Re-configure: `muster auth configure --api-key NEW_KEY`

### Permission Denied

**Cause**: Authenticated but lacking permissions

**Solution**:
1. Check your role/permissions
2. Contact administrator for access
3. Verify you're using correct profile: `muster auth status`
