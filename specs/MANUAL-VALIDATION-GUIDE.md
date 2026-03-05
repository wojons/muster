# Manual Validation Guide Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Validation

## Overview

Complete specification of **manual validation procedures** for critical system behaviors that require human verification.

## 1. Validation Scenarios

### 1.1 Cross-Language WASM Determinism

**Purpose**: Verify identical WASM execution across all 12 language SDKs

**Procedure**:
```bash
# 1. Build test WASM module
cd tests/wasm-determinism
make build-test-module

# 2. Run in each language
for lang in go python javascript java rust csharp cpp swift ruby kotlin dart php; do
    ./validate-$lang.sh
done

# 3. Compare outputs
./compare-outputs.sh
```

**Expected Result**: All 12 implementations produce identical output hashes

### 1.2 Authentication Flow Verification

**Purpose**: Verify OAuth2 and mTLS authentication work correctly

**Procedure**:
```bash
# OAuth2 flow
openapi-cli auth oauth2 --provider github --interactive
# Follow browser flow, verify token stored in keychain

# mTLS flow
openapi-cli auth mtls --cert client.crt --key client.key --ca ca.crt
# Verify successful handshake
```

**Validation Points**:
- [ ] Browser opens automatically
- [ ] Token received and stored securely
- [ ] Subsequent requests use token automatically
- [ ] Token refresh works after expiry

### 1.3 MCP Server Integration

**Purpose**: Verify MCP server integrates with Claude Desktop

**Procedure**:
1. Configure Claude Desktop:
```json
{
  "mcpServers": {
    "openapi": {
      "command": "openapi-cli",
      "args": ["mcp", "serve"]
    }
  }
}
```

2. Restart Claude Desktop
3. Verify tools appear in tool list
4. Test tool execution

**Expected Results**:
- [ ] Server starts without errors
- [ ] Tools listed in Claude
- [ ] Tool execution returns results
- [ ] Error handling works correctly

## 2. Validation Checklist

### Pre-Release Checklist

- [ ] All 12 language SDKs pass WASM determinism tests
- [ ] OAuth2 authentication works with major providers (GitHub, Google, Azure)
- [ ] mTLS authentication works with test certificates
- [ ] Keychain integration works on all platforms
- [ ] MCP server integrates with Claude Desktop
- [ ] Daemon mode works on Linux/macOS
- [ ] CLI generates correct commands from OpenAPI specs
- [ ] Rate limiting works correctly
- [ ] Token revocation works
- [ ] WASM sandbox prevents escapes

### Security Checklist

- [ ] No secrets logged in plain text
- [ ] Keychain fallback not triggered unless requested
- [ ] WASM modules cannot access filesystem outside sandbox
- [ ] Network requests respect allowed hosts list
- [ ] Rate limiting prevents abuse
- [ ] Authentication tokens are encrypted at rest
- [ ] TLS 1.2+ required for all connections

## 3. Platform-Specific Validation

### macOS
- [ ] Keychain Access prompts user appropriately
- [ ] Notarization passes
- [ ] Apple Silicon and Intel builds work

### Linux
- [ ] Secret Service integration works
- [ ] Systemd service starts correctly
- [ ] Unix socket permissions are correct

### Windows
- [ ] Credential Manager integration works
- [ ] Windows Defender false positives checked
- [ ] PowerShell completion works

## 4. Automation Opportunities

The following manual validations should be automated:

1. **WASM determinism** → Automated cross-language test suite
2. **OAuth2 flows** → Use test OAuth2 server in CI
3. **MCP integration** → Automated MCP client test
4. **Keychain integration** → Platform-specific CI jobs

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
