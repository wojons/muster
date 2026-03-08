# Tech Context

## Core Technologies
- **Language**: Go 1.25+
- **Frameworks**: Cobra (CLI), Wasmtime (WASM)
- **Databases**: SQLite (repository layer)
- **Protocols**: OpenAPI 3.0, MCP, HTTP/1.2+ TLS

## Build Requirements
- CGO_ENABLED=1 required for SQLite
- Version injection via ldflags
- Cross-platform compilation support

## Dependencies
- getkin/kin-openapi (OpenAPI parsing)
- stretchr/testify (testing)
- viper/config (configuration)

## Development Tools
- golangci-lint (linting)
- go fmt (formatting)
- go vet (static analysis)

## Security Tools (Verified 2026-03-08)
- AES-256-GCM encryption (credentials) - pkg/auth/keychain.go:361-409
- System-specific key derivation (OS+arch+machine-id) - lines 343-359
- Path traversal rejection (sanitizeFilename) - lines 411-437
- DNS resolution + IP blocking (network validation) - pkg/dsl/builtins.go:627-718
- DoS protection (10MB limits, 30s timeout) - lines 26-27, 67

## Performance Features
- MaxBytesReader (response limiting)
- Token bucket rate limiting
- Multi-tier caching (memory + Redis)
