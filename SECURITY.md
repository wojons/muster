# Security Policy

## Reporting a vulnerability

Email wojonstech@gmail.com. Do NOT open a public issue.

Response within 48 hours. Disclosure coordinated with you.

## Supported versions

| Version | Supported |
|---------|-----------|
| main    | ✅ |

Only the current `main` branch receives security fixes.

## Scope

- CLI argument injection
- WASM sandbox escapes
- MCP protocol attacks
- Credential leakage (keychain, env vars, config files)
- Network request forgery

## Out of scope

- Vulnerabilities in demo services (they're mock APIs for testing)
- Denial of service via resource exhaustion (CLI tool, single-user)
