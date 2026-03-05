# Troubleshooting Guide

Solutions for common issues with Muster CLI.

## Common Issues

### Connection Errors

**Symptom**: `Error: connection refused` or timeout errors

**Solutions**:
1. Check network connectivity
2. Verify API URL: `muster config get api.url`
3. Check if proxy is required:
   ```yaml
   proxy:
     url: http://proxy.company.com:8080
   ```
4. Test with curl:
   ```bash
   curl -I $(muster config get api.url)
   ```

### Authentication Failures

**Symptom**: `401 Unauthorized` or `403 Forbidden`

**Solutions**:
1. Check auth status: `muster auth status`
2. Re-authenticate: `muster auth login`
3. Verify API key: `muster auth configure --api-key KEY`
4. Check token expiration
5. Verify correct profile: `muster config profiles`

### Resource Not Found

**Symptom**: `404 Not Found`

**Solutions**:
1. Verify resource name: `muster list --type <type>`
2. Check namespace: `muster list --namespace <ns>`
3. Ensure correct profile: `--profile <name>`

### Permission Denied

**Symptom**: `403 Forbidden`

**Solutions**:
1. Check your role/permissions in dashboard
2. Verify you're using correct authentication
3. Contact administrator for access

## Debug Mode

Enable debug logging for detailed information:

```bash
# Via flag
muster --verbose list

# Via environment
export MUSTER_DEBUG=true
muster list
```

Debug output includes:
- HTTP requests/responses
- Authentication flow
- Configuration loading
- API call timing

## Log Files

Muster logs to stderr by default. To save logs:

```bash
muster list 2> muster.log
```

## Reset Configuration

Reset to default configuration:

```bash
rm ~/.config/muster/config.yaml
muster config init
```

## Known Issues

### Issue: Slow Performance on Large Lists

**Workaround**: Use pagination and filtering:
```bash
muster list --limit 50 --sort-by updated
```

### Issue: Colors Not Showing

**Cause**: Terminal doesn't support ANSI colors

**Solution**: Force plain output:
```bash
muster list --no-color
# or
export NO_COLOR=1
```

### Issue: Config File Not Found

**Symptom**: `config file not found` warning

**Solution**: Create default config:
```bash
muster config init
```

## Getting Help

### Community Support

- **GitHub Issues**: [github.com/your-org/muster/issues](https://github.com/your-org/muster/issues)
- **Discussions**: [github.com/your-org/muster/discussions](https://github.com/your-org/muster/discussions)

### Reporting Bugs

When reporting bugs, include:

1. Muster version: `muster --version`
2. Operating system and version
3. Full command with flags
4. Error message (run with `--verbose`)
5. Steps to reproduce

### Feature Requests

Submit feature requests via GitHub Issues with:
- Clear description
- Use case explanation
- Proposed interface/example usage

## Diagnostic Commands

```bash
# Check version and build info
muster --version

# Verify configuration
muster config get

# Test authentication
muster auth status

# List available profiles
muster config profiles

# Check API connectivity
muster list --limit 1 --verbose
```
