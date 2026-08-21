# Mono‑Repo Layout (Expanded with SDK Generation)

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Complete Project Structure for CLI + SDKs + MCP  
**Referenced Files**: `PROJECT-STRUCTURE.md`, `SDK-GENERATION-ARCHITECTURE.md`

## Overview

Complete directory structure for the mono‑repo containing:
- **OpenAPI CLI** (Go binary)
- **MCP Server** (Go binary)  
- **Daemon** (Go binary)
- **SDK Generator** (Go tool)
- **Generated SDKs** (12 languages)
- **Specifications** (135 exhaustive specs)
- **Build system**, **CI/CD**, **Documentation**

## Complete Directory Tree

```
muster/                              # Mono‑repo root
├── .github/                         # GitHub workflows
│   └── workflows/
│       ├── ci.yaml                  # CI for all languages
│       ├── release.yaml             # Release automation
│       ├── sdk-publish.yaml        # SDK publishing (12 package managers)
│       └── docker.yaml              # Docker builds
│
├── .agents/                         # AI agent skills & configurations
│   ├── skills/                      # 25+ specialized skills
│   └── prompts/                     # Agent prompt templates
│
├── api/                             # Public API definitions
│   ├── openapi/                     # OpenAPI specifications
│   ├── proto/                       # Protocol buffers
│   └── inventory/                   # Inventory schema definitions
│
├── bin/                             # Build artifacts (gitignored)
│   ├── openapi-cli                  # CLI binary
│   ├── openapi-cli-mcp             # MCP server binary
│   ├── openapi-cli-daemon          # Daemon binary
│   ├── sdk-generator               # SDK generator tool
│   └── wasm/                        # Compiled WASM modules
│
├── build/                           # Build system
│   ├── scripts/                     # Build scripts
│   ├── wasm/                        # WASM build configurations
│   ├── docker/                      # Docker configurations
│   └── packaging/                   # Package manager configurations
│
├── cmd/                             # Go entry points
│   ├── cli/                         # CLI main package
│   ├── mcp/                         # MCP server main package
│   ├── daemon/                      # Daemon main package
│   └── sdk-generator/              # SDK generator main package
│
├── configs/                         # Configuration templates
│   ├── openapi-cli.yaml            # CLI configuration
│   ├── profiles.yaml               # Profile templates
│   ├── systemd/                    # Systemd services
│   └── launchd/                    # macOS launch agents
│
├── deployments/                     # Deployment configurations
│   ├── docker-compose.yaml          # Local development
│   ├── kubernetes/                 # K8s manifests
│   ├── helm/                       # Helm charts
│   └── cloud/                      # Cloud deployment (AWS, GCP, Azure)
│
├── dist/                            # Distribution artifacts (gitignored)
│   ├── cli/                        # CLI binaries per platform
│   ├── sdk/                        # Generated SDK packages
│   └── checksums/                  # Checksum files
│
├── docs/                            # Documentation
│   ├── README.md
│   ├── CONTRIBUTING.md
│   ├── CHANGELOG.md
│   ├── SECURITY.md
│   ├── api/                        # API documentation
│   ├── architecture/               # Architecture diagrams
│   └── sdk/                        # SDK documentation
│
├── examples/                        # Usage examples
│   ├── workflows/                  # Starlark workflow examples
│   ├── configs/                    # Configuration examples
│   ├── integrations/               # Third‑party integrations
│   └── sdk/                        # SDK usage examples (12 languages)
│
├── internal/                        # Private Go code
│   ├── cli/                        # CLI implementation
│   ├── core/                       # Core domain logic
│   ├── mcp/                        # MCP server implementation
│   ├── daemon/                     # Daemon implementation
│   ├── sdk/                        # SDK generator implementation
│   └── platform/                   # Platform‑specific implementations
│
├── migrations/                      # Database migrations
│   ├── postgres/                   # PostgreSQL migrations
│   ├── sqlite/                     # SQLite migrations
│   └── mysql/                     # MySQL migrations
│
├── pkg/                             # Public Go packages
│   ├── inventory/                  # Inventory protocol types
│   ├── openapi/                    # OpenAPI utilities
│   ├── mcp/                        # MCP protocol helpers
│   ├── wasm/                       # WASM runtime interfaces
│   └── sdk/                        # SDK generation interfaces
│
├── sdk/                             # SDK generation & distribution
│   ├── generator/                  # SDK generator core
│   │   ├── internal/
│   │   │   ├── ir/                # Intermediate representation
│   │   │   ├── templates/         # Language templates (12 languages)
│   │   │   ├── transform/         # IR transformations
│   │   │   └── validation/        # SDK validation
│   │   └── cmd/generator/         # Generator CLI
│   │
│   ├── templates/                  # Language‑specific templates
│   │   ├── python/                # Python SDK templates
│   │   ├── javascript/            # JavaScript SDK templates
│   │   ├── typescript/            # TypeScript SDK templates
│   │   ├── java/                 # Java SDK templates
│   │   ├── kotlin/                # Kotlin SDK templates
│   │   ├── go/                    # Go SDK templates
│   │   ├── rust/                  # Rust SDK templates
│   │   ├── csharp/                # C# SDK templates
│   │   ├── swift/                 # Swift SDK templates
│   │   ├── ruby/                  # Ruby SDK templates
│   │   ├── php/                   # PHP SDK templates
│   │   └── dart/                  # Dart SDK templates
│   │
│   ├── distributions/              # Generated SDKs (gitignored)
│   │   ├── python/                # Generated Python SDKs
│   │   ├── javascript/            # Generated JavaScript SDKs
│   │   ├── typescript/            # Generated TypeScript SDKs
│   │   ├── java/                  # Generated Java SDKs
│   │   ├── kotlin/                # Generated Kotlin SDKs
│   │   ├── go/                    # Generated Go SDKs
│   │   ├── rust/                  # Generated Rust SDKs
│   │   ├── csharp/                # Generated C# SDKs
│   │   ├── swift/                 # Generated Swift SDKs
│   │   ├── ruby/                  # Generated Ruby SDKs
│   │   ├── php/                   # Generated PHP SDKs
│   │   └── dart/                  # Generated Dart SDKs
│   │
│   ├── tests/                      # Cross‑language tests
│   │   ├── vectors/               # Test vectors
│   │   ├── python/
│   │   ├── javascript/
│   │   ├── typescript/
│   │   ├── java/
│   │   ├── kotlin/
│   │   ├── go/
│   │   ├── rust/
│   │   ├── csharp/
│   │   ├── swift/
│   │   ├── ruby/
│   │   ├── php/
│   │   └── dart/
│   │
│   └── packaging/                  # Package manager configurations
│       ├── pip/                    # Python pip packaging
│       ├── npm/                    # JavaScript npm packaging
│       ├── maven/                  # Java Maven packaging
│       ├── gradle/                 # Kotlin Gradle packaging
│       ├── cargo/                  # Rust Cargo packaging
│       ├── nuget/                  # C# NuGet packaging
│       ├── swiftpm/                # Swift Package Manager
│       ├── rubygems/              # RubyGems packaging
│       ├── composer/               # PHP Composer packaging
│       └── pub/                    # Dart pub packaging
│
├── scripts/                         # Utility scripts
│   ├── bootstrap.sh                # Bootstrap development environment
│   ├── build-all.sh              # Build all components
│   ├── test-all.sh                # Test all languages
│   ├── publish-all.sh             # Publish all SDKs
│   └── cleanup.sh                 # Clean build artifacts
│
├── specs/                          # 135 exhaustive specifications
│   ├── master-specification.md    # Master specification
│   ├── components/                # Component specifications (6 files)
│   ├── implementation/            # Implementation specs (22 files)
│   ├── platform/                  # Platform specs (3 files)
│   ├── deployment/                # Deployment specs (31 files)
│   ├── security/                  # Security specs (23 files)
│   ├── testing/                   # Testing specs (9 files)
│   ├── sdk/                       # SDK generation specs (8 files)
│   └── validation/                # Validation scripts
│
├── test/                           # Test suites
│   ├── unit/                      # Unit tests
│   ├── integration/               # Integration tests
│   ├── e2e/                       # End‑to‑end tests
│   ├── security/                  # Security tests
│   ├── performance/               # Performance tests
│   └── determinism/               # Determinism tests
│
└── vendor/                         # Go dependencies (gitignored)
```

## Key Directories Explained

### `sdk/generator/`
Core SDK generator tool that:
1. Reads OpenAPI specs + inventory
2. Generates Intermediate Representation (IR)
3. Applies language‑specific templates (12 languages)
4. Bundles WASM modules
5. Validates generated SDKs

### `sdk/templates/`
Language‑specific templates for SDK generation:
- **Python**: `client.py.tmpl`, `command.py.tmpl`, `wasm_runtime.py.tmpl`
- **TypeScript**: Type definitions, async/await patterns
- **Rust**: `lib.rs.tmpl`, trait‑based interfaces
- **Swift**: Protocol‑oriented design, async/await
- **Kotlin**: Coroutine support, extension functions
- **C#**: Async/await, LINQ‑style APIs

### `sdk/distributions/`
Generated SDKs ready for distribution:
- Each language gets its own directory
- Includes source code, tests, documentation
- Ready for package manager publishing
- Contains bundled WASM modules

### `sdk/packaging/`
Package manager configurations for 10 package managers:
- **pip**: `setup.py`, `pyproject.toml`, `requirements.txt`
- **npm**: `package.json`, TypeScript configs
- **Maven**: `pom.xml`, parent POMs
- **Cargo**: `Cargo.toml`, feature flags
- **NuGet**: `.csproj`, `.nuspec`
- **SwiftPM**: `Package.swift`
- **RubyGems**: `gemspec`, `Gemfile`
- **Composer**: `composer.json`
- **pub**: `pubspec.yaml`

## Build System Integration

### Multi‑Language CI/CD
GitHub Actions matrix builds all 12 SDKs in parallel:
```yaml
jobs:
  build-sdks:
    strategy:
      matrix:
        language: [python, javascript, typescript, java, kotlin, go, rust, csharp, swift, ruby, php, dart]
    runs-on: ubuntu-latest
    steps:
      - run: ./scripts/build-sdk.sh ${{ matrix.language }}
```

### Deterministic Builds
All SDKs built from **same IR** → **identical behavior**:
```bash
# Generate IR from OpenAPI
sdk-generator ir --spec openapi.json --inventory inventory.json

# Generate SDK for all languages
for lang in python javascript typescript java kotlin go rust csharp swift ruby php dart; do
  sdk-generator generate --language $lang --output sdk/distributions/$lang
done
```

## Development Workflow

1. **Update OpenAPI spec** → `api/openapi/`
2. **Update inventory** → `api/inventory/`
3. **Run SDK generator** → `sdk/generator/`
4. **Test all SDKs** → `sdk/tests/`
5. **Publish packages** → `sdk/packaging/`

## Mono‑Repo Advantages

1. **Single source of truth**: One OpenAPI spec → CLI + 12 SDKs + MCP
2. **Cross‑language testing**: Test vectors run across all implementations
3. **Determinism validation**: Verify identical behavior across languages
4. **Unified versioning**: All components versioned together
5. **Simplified dependency management**: Shared WASM modules, crypto libraries

## Reference Implementation

The **Go CLI** (`cmd/openapi-cli/`) serves as **reference implementation**. All SDKs must:

1. **Pass same test suite** as Go CLI
2. **Match error codes** and messages exactly
3. **Produce identical outputs** for identical inputs
4. **Maintain API compatibility** with Go CLI

**Validation**: `sdk/tests/cross-language/` runs 1,000+ test vectors across all SDKs.

---
**Total Components**: CLI + MCP + Daemon + 12 SDKs  
**Lines of Code**: ~500,000 (estimated generated SDK code)  
**Package Managers**: 10 supported  
**Bible Standard Compliance**: ✅ Deterministic across all outputs