# Troubleshooting Guide

Common issues and solutions for OpenAPI CLI.

## Installation Issues

### Permission Denied (Linux/macOS)

**Error**:
```
bash: /usr/local/bin/openapi-cli: Permission denied
```

**Causes**:
- Binary is not executable
- Insufficient permissions

**Solutions**:

1. **Fix permissions**:
   ```bash
   chmod +x /usr/local/bin/openapi-cli
   ```

2. **Install to user directory**:
   ```bash
   mkdir -p ~/.local/bin
   curl -L https://github.com/openapi-cli/openapi-cli/releases/latest/download/openapi-cli-linux-amd64 -o ~/.local/bin/openapi-cli
   chmod +x ~/.local/bin/openapi-cli
   export PATH="$HOME/.local/bin:$PATH"
   ```

3. **Check file permissions**:
   ```bash
   ls -la /usr/local/bin/openapi-cli
   # Should show: -rwxr-xr-x
   ```

### Command Not Found

**Error**:
```
command not found: openapi-cli
```

**Causes**:
- Binary not in PATH
- Installation not complete
- Shell not restarted

**Solutions**:

1. **Check installation location**:
   ```bash
   which openapi-cli || find /usr/local -name openapi-cli
   ```

2. **Add to PATH** (add to `~/.bashrc`, `~/.zshrc`, or `~/.profile`):
   ```bash
   echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
   source ~/.bashrc
   ```

3. **Verify PATH**:
   ```bash
   echo $PATH | grep -o '[^:]*bin'
   ```

### macOS Gatekeeper Blocking

**Error**:

```
"openapi-cli" can't be opened because Apple cannot check it for malicious software.
```

**Solution**:

```bash
# Remove quarantine attribute
xattr -d com.apple.quarantine /usr/local/bin/openapi-cli

# Or allow the entire location
sudo xattr -d -r com.apple.quarantine /usr/local/bin/
```

### Windows Smartscreen Warning

**Solution**:

1. Click "More info"
2. Click "Run anyway"

Or use PowerShell:

```powershell
Unblock-File -Path C:\Path\To\openapi-cli.exe
```

## Specification Issues

### Invalid OpenAPI Specification

**Error**:
```
Error: Invalid OpenAPI specification
```

**Causes**:
- Syntax errors in YAML/JSON
- Missing required fields
- Type mismatches
- Invalid references

**Solutions**:

1. **Validate the specification**:
   ```bash
   openapi-cli generate your-spec.yaml
   # If the spec parses successfully, commands are generated.
   # If it fails, the error message will indicate the problem.
   ```

2. **Check common issues**:
   - Indentation in YAML (use 2 spaces)
   - Missing `openapi` or `swagger` version
   - Invalid `$ref` references
   - Missing required fields

3. **Use online validator**:
   - https://validator.swagger.io/
   - https://apitools.dev/swagger-parser/online/

4. **Example of valid specification**:
   ```yaml
   openapi: 3.0.3
   info:
     title: Sample API
     version: 1.0.0
   paths:
     /pets:
       get:
         summary: List pets
         responses:
           '200':
             description: Success
   ```

### Specification Not Found

**Error**:
```
Error: Specification not found: openapi.yaml
```

**Solutions**:

1. **Check file path**:
   ```bash
   ls -la openapi.yaml
   # Use absolute path
   openapi-cli --spec /full/path/to/openapi.yaml
   ```

2. **Set spec in config**:
   ```bash
   openapi-cli config set spec.path ./api/openapi.yaml
   ```

3. **Verify encoding**:
   ```bash
   file openapi.yaml
   # Should show: ASCII text or UTF-8 Unicode text
   ```

### Large Specification Slow Loading

**Cause**: Large specifications (10MB+) can be slow to parse.

**Solutions**:

1. **Increase cache**:
   ```yaml
   cache:
     enabled: true
     max_size: 10GB
   ```

2. **Split specification**:
   (Not yet implemented — use `generate` to work with the full spec.)

3. **Generate CLI commands**:
   ```bash
   openapi-cli generate openapi.yaml  # Pre-generates commands
   ```

## Configuration Issues

### Configuration Not Loading

**Error**:
```
Error: Configuration file not found
```

**Solutions**:

1. **Check default locations**:
   ```bash
   # List config files found
   openapi-cli config show --help
   ```

2. **Switch to a named profile**:
   ```bash
   openapi-cli config profile switch <profile-name>
   ```

3. **Validate configuration**:
   ```bash
   openapi-cli config show
   ```

### Environment Variables Not Working

**Solution**:

1. **Use correct format** (section and key separated by `_`):
   ```bash
   # Correct
   export MUSTER_SPEC_PATH=openapi.yaml
   export MUSTER_HTTP_TIMEOUT=30s
   
   # Incorrect (will be ignored)
   export SPEC_PATH=openapi.yaml
   ```

2. **Variable expansion in config**:
   ```yaml
   auth:
     apikey:
       value: ${API_KEY}  # Works
   ```

3. **Verify values**:
   ```bash
   openapi-cli config show --resolved
   ```

### Profile Not Found

**Error**:
```
Error: Profile 'production' not found
```

**Solution**:

1. **Check available profiles**:
   ```bash
   openapi-cli config show profiles
   ```

2. **Define profile in config**:
   ```yaml
   profiles:
     production:
       spec:
         base_url: https://api.prod.example.com
   ```

3. **Set profile from environment**:
   ```bash
   export MUSTER_PROFILE=production
   ```

## HTTP/API Issues

### Connection Timeout

**Error**:
```
Error: Request timeout after 30s
```

**Solutions**:

1. **Increase timeout**:
   ```bash
   openapi-cli config set http.timeout 60s
   ```

2. **Check network**:
   ```bash
   ping api.example.com
   traceroute api.example.com
   ```

3. **Test with curl**:
   ```bash
   curl -v https://api.example.com/v1/pets
   ```

### SSL/TLS Errors

**Error**:
```
Error: x509: certificate signed by unknown authority
```

**Solutions**:

1. **Verify SSL (most secure)**:
   ```bash
   openapi-cli config set http.verify_ssl true
   ```

2. **Disable SSL verification (development only)**:
   ```bash
   openapi-cli config set http.verify_ssl false
   ```

3. **Add custom CA**:
   ```bash
   export SSL_CERT_FILE=/path/to/ca.crt
   ```

4. **Check certificate**:
   ```bash
   openssl s_client -connect api.example.com:443
   ```

### 401 Unauthorized

**Cause**: Missing or invalid authentication.

**Solutions**:

1. **Check authentication status**:
   ```bash
   openapi-cli auth list
   ```

2. **Store credentials**:
   ```bash
   openapi-cli auth add --name myapi --type api-key --value YOUR_KEY
   ```

3. **Set API key**:
   ```bash
   export API_KEY=your-secret-key
   openapi-cli auth add --name myapi --type api-key --value $API_KEY
   ```

4. **Test authentication**:
   ```bash
   openapi-cli config show auth
   ```

### 429 Too Many Requests

**Cause**: Rate limiting by API provider.

**Solutions**:

1. **Enable rate limiting**:
   ```yaml
   rate_limit:
     enabled: true
     requests_per_second: 10
     burst: 100
   ```

2. **Wait and retry**:
   ```bash
   openapi-cli config set http.max_retries 5
   openapi-cli config set http.retry_delay 5s
   ```

3. **Use exponential backoff**:
   ```yaml
   http:
     retry_backoff: exponential
   ```

### 500 Server Error

**Solutions**:

1. **Retry request**:
   ```bash
   openapi-cli config set http.max_retries 3
   ```

2. **Check API status**:
   ```bash
   openapi-cli request GET https://api.example.com/health
   ```

3. **Check API logs** if available

## MCP Server Issues

### MCP Server Not Starting

**Error**:
```
Error: Failed to start MCP server
```

**Solutions**:

1. **Check spec file**:
   ```bash
   openapi-cli generate openapi.yaml
   ```

2. **Check port availability**:
   ```bash
   lsof -i :8080
   ```

3. **Run with debug**:
   ```bash
   ./bin/openapi-mcp --help
   ```

### MCP Server Not Responding

**Solution**:

Check in Claude Desktop or other MCP client configuration:

```json
{
  "mcpServers": {
    "openapi-cli": {
      "command": "/path/to/bin/openapi-mcp",
      "args": ["--spec", "/full/path/to/openapi.yaml"]
    }
  }
}
```

Verify with:

```bash
echo '{"jsonrpc":"2.0","method":"initialize","params":{},"id":1}' | ./bin/openapi-mcp
```

## WASM Issues

### WASM Module Not Loading

**Error**:
```
Error: Failed to load WASM module
```

**Solutions**:

1. **Check WASM format**:
   ```bash
   file module.wasm
   # Should show: WebAssembly (wasm) binary
   ```

2. **Recompile with correct targets**:
   ```bash
   # Go
   GOARCH=wasm GOOS=js go build -o module.wasm main.go
   
   # Rust
   rustc --target wasm32-unknown-unknown -O module.wasm
   ```

3. **Check WASM compatibility**:
   ```bash
   openapi-cli modules run module.wasm
   ```

### Out of Memory

**Error**:
```
Error: WASM module out of memory
```

**Solution**:

Increase memory limit:

```bash
openapi-cli modules run module.wasm
```

Or set in config:

```yaml
wasm:
  enabled: true
  max_memory: 512MB
```

### WASM Timeout

**Solution**:

Increase timeout:

```bash
openapi-cli modules run module.wasm
```

## Daemon Issues

### Daemon Not Starting

**Error**:
```
Error: Failed to start daemon
```

**Solutions**:

1. **Check PID file**:
   ```bash
   ls -la /var/run/openapi-cli.pid
   cat /var/run/openapi-cli.pid
   ```

2. **Check socket permissions**:
   ```bash
   ls -la /tmp/openapi-cli.sock
   # Should show writable by current user
   ```

3. **Check logs**:
   ```bash
   # Logs shown in stderr or log file
   openapi-cli daemon status
   ```

### Daemon Already Running

**Error**:
```
Error: Daemon already running (PID: 12345)
```

**Solution**:

1. **Stop existing daemon**:
   ```bash
   openapi-cli daemon stop
   ```

2. **Force stop**:
   ```bash
   openapi-cli daemon stop
   ```

3. **Kill manually**:
   ```bash
   kill $(cat /var/run/openapi-cli.pid)
   ```

### Socket Connection Refused

**Solution**:

1. **Check daemon status**:
   ```bash
   openapi-cli daemon status
   ```

2. **Check socket path**:
   ```bash
   ls -la /tmp/openapi-cli.sock
   ```

3. **Restart daemon**:
   ```bash
   openapi-cli daemon stop && openapi-cli daemon start
   ```

## Cache Issues

### Cache Not Working

**Solutions**:

1. **Check cache status**:
   ```bash
   ls -la ~/.cache/openapi-cli/
   ```

2. **Clear cache**:
   ```bash
   rm -rf ~/.cache/openapi-cli/
   ```

3. **Check cache directory**:
   ```bash
   ls -la ~/.cache/openapi-cli
   ```

## Logging Issues

### No Debug Output

**Solution**:

Enable debug mode:

```bash
openapi-cli request GET https://petstore.swagger.io/v2/pet/1
```

Or set log level:

```bash
export MUSTER_LOG_LEVEL=debug
```

### Logs Not Written to File

**Solution**:

Configure log file:

```yaml
log:
  level: debug
  output: /var/log/openapi-cli.log
```

Check permissions:

```bash
ls -la /var/log/
# Create log directory if needed
sudo mkdir -p /var/log/
sudo chmod 1777 /var/log/
```

## System Check/Diagnostics

Run system diagnostics:

```bash
openapi-cli version
openapi-cli config show
openapi-cli audit status
```

This will check:
- Installation and version
- Configuration
- Audit logging status
- File system permissions
- Network connectivity

## Getting More Help

### Verbose Debug Output

```bash
openapi-cli request GET https://petstore.swagger.io/v2/pet/1 --verbose
```

### Contact Support

1. Check [GitHub Issues](https://github.com/openapi-cli/openapi-cli/issues)
2. Join [GitHub Discussions](https://github.com/openapi-cli/openapi-cli/discussions)
3. Collect diagnostic info:

   ```bash
   openapi-cli version > diagnostics.txt
   openapi-cli config show > config.txt
   cat diagnostics.txt config.txt > support-request.txt
   ```

### Bug Reporting

When reporting bugs, include:

- OpenAPI CLI version (`openapi-cli --version`)
- Operating system and version
- Configuration (`openapi-cli config show`)
- Trace/Debug output with `--debug` flag
- Steps to reproduce the issue

## Common Gotchas

1. **YAML indentation**: Always use 2 spaces, never tabs
2. **Environment variables**: Use `MUSTER_` prefix
3. **File paths**: Use absolute paths or ensure current directory is correct
4. **Timeouts**: May need to increase for slow APIs
5. **Authentication**: Always test auth before using in scripts
6. **Caching**: Clear cache if spec changes don't appear

## Platform-Specific Issues

### Linux

**Library missing**:
```bash
# Error: libssl.so.1.1: cannot open shared object file
sudo apt-get install libssl1.1
```

### macOS

**Xcode Command Line Tools needed**:
```bash
xcode-select --install
```

### Windows

**PowerShell execution policy**:
```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```
