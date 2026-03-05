# Specification Index

## 🚀 Quick Navigation

### For First-Time Readers
1. **[EXECUTIVE-SUMMARY.md](EXECUTIVE-SUMMARY.md)** - Start here! One-page overview
2. **[QUICK-START.md](QUICK-START.md)** - Developer setup and usage guide
3. **[README.md](README.md)** - Project overview and navigation

### For Implementers
1. **[TECHNICAL-DECISIONS.md](TECHNICAL-DECISIONS.md)** - Why we chose Go, Wasmtime, Starlark
2. **[planning-artifacts/architecture-v2.md](planning-artifacts/architecture-v2.md)** - Complete technical architecture
3. **[planning-artifacts/PRD.md](planning-artifacts/PRD.md)** - All product requirements
4. **[IMPLEMENTATION-ROADMAP.md](IMPLEMENTATION-ROADMAP.md)** - Phased implementation plan

### For Understanding Flows
- **[ARCHITECTURE-VISUAL.md](ARCHITECTURE-VISUAL.md)** - Visual diagrams and data flows

## 📚 Complete Document Listing

### Root Level Documents (5 files)

| Document | Lines | Purpose |
|----------|-------|---------|
| **README.md** | 220 | Main entry point, navigation, overview |
| **EXECUTIVE-SUMMARY.md** | 300 | One-page product overview with examples |
| **QUICK-START.md** | 250 | Developer setup, user guide, common tasks |
| **ARCHITECTURE-VISUAL.md** | 450 | Visual diagrams, data flows, cache structure |
| **TECHNICAL-DECISIONS.md** | 850 | Detailed technical decisions (Go, Wasmtime, Starlark) |
| **IMPLEMENTATION-ROADMAP.md** | 400 | Phased implementation plan |
| **VERIFICATION.md** | 350 | Coverage verification checklist |

### Planning Artifacts (7 files)

| Document | Lines | Purpose |
|----------|-------|---------|
| **PRD.md** | 300 | Product Requirements Document - features, user stories |
| **architecture.md** | 550 | Original architecture document |
| **architecture-v2.md** | 600 | Updated architecture with technical decisions |
| **SECURITY-REVOCATION.md** | 350 | Inventory and module revocation mechanism |
| **SECURITY-ROTATION.md** | 350 | Certificate and key rotation procedures |
| **OPERATIONS-OBSERVABILITY.md** | 450 | Metrics, logging, tracing, health checks |

### Epics (7 files)

| Epic | Focus | Story Points |
|------|-------|--------------|
| **epic-01-inventory-system.md** | Inventory fetching, caching, versioning | 21 |
| **epic-02-dual-interface.md** | CLI + MCP implementation | 21 |
| **epic-03-request-execution.md** | HTTP client, middleware | 13 |
| **epic-04-module-registry.md** | WASM module system | 21 |
| **epic-05-configuration.md** | Config management | 8 |
| **epic-06-workflows.md** | Multi-step workflows | 13 |
| **epic-07-security-layer.md** | Auth, secrets, crypto | 21 |

**Total Epic Story Points**: 118 (~17 weeks)

### Stories (8 files - sample implementation)

| Story | Epic | Description |
|-------|------|-------------|
| **story-01-01-https-fetching.md** | Inventory | HTTPS with TLS/mTLS |
| **story-01-04-version-management.md** | Inventory | Version pinning, aliases |
| **story-01-05-schema-validation.md** | Inventory | OpenAPI validation |
| **story-02-01-path-mapping.md** | Dual Interface | OpenAPI → CLI commands |
| **story-02-08-mcp-setup.md** | Dual Interface | MCP server implementation |
| **story-03-01-http-client.md** | Request Execution | HTTP client setup |
| **story-04-01-registry-client.md** | Module Registry | WASM module downloading |
| **story-07-01-npm-package.md** | Distribution | npm packaging |

## 🎯 Implementation Roadmap

### Phase 1: Foundation (Weeks 1-3)
**Goal**: Basic inventory fetch and CLI generation

**Documents to read**:
- EXECUTIVE-SUMMARY.md (understand the product)
- TECHNICAL-DECISIONS.md (understand the stack)
- epic-01-inventory-system.md
- story-01-01-https-fetching.md

**Deliverables**:
- [ ] Bun project setup
- [ ] Inventory fetch from HTTPS
- [ ] Local caching
- [ ] Basic CLI command generation

### Phase 2: Core (Weeks 4-6)
**Goal**: Working CLI and MCP

**Documents to read**:
- epic-02-dual-interface.md
- story-02-01-path-mapping.md
- story-02-08-mcp-setup.md
- epic-03-request-execution.md

**Deliverables**:
- [ ] Dynamic CLI commands
- [ ] MCP stdio server
- [ ] HTTP request execution
- [ ] Basic auth (Bearer, API key)

### Phase 3: Scripting (Weeks 7-9)
**Goal**: DSL for workflows

**Documents to read**:
- epic-06-workflows.md
- TECHNICAL-DECISIONS.md (Starlark section)

**Deliverables**:
- [ ] Starlark integration
- [ ] HTTP primitives for DSL
- [ ] Simple workflow execution

### Phase 4: Security (Weeks 10-12)
**Goal**: Production-ready auth

**Documents to read**:
- epic-07-security-layer.md
- TECHNICAL-DECISIONS.md (permissions section)

**Deliverables**:
- [ ] OS keychain integration
- [ ] mTLS support
- [ ] Inventory signing
- [ ] Client fingerprinting

### Phase 5: Advanced (Weeks 13-15)
**Goal**: WASM and proxy mode

**Documents to read**:
- epic-04-module-registry.md
- TECHNICAL-DECISIONS.md (WASM section)

**Deliverables**:
- [ ] Wasmtime integration
- [ ] WASM module loading
- [ ] Proxy daemon mode
- [ ] Webhook support

### Phase 6: Polish (Weeks 16-17)
**Goal**: Production release

**Deliverables**:
- [ ] Bundled server (SQLite → MySQL/PG)
- [ ] S3/CloudFront support
- [ ] Documentation
- [ ] Distribution setup

## 🔧 Technology Stack Summary

| Component | Technology | Reason |
|-----------|-----------|--------|
| **Runtime** | Bun | Single binary, TypeScript native, fast |
| **WASM** | Wasmtime | Production-ready, WASI, secure |
| **DSL** | Starlark | Sandboxed, Python-like, extensible |
| **HTTP Client** | Axios | Battle-tested, interceptors |
| **Secrets** | keytar | OS keychain integration |
| **CLI** | Commander.js | Industry standard |
| **MCP** | @modelcontextprotocol/sdk | Official SDK |

## 🏗️ Architecture Patterns

### Three Deployment Modes
1. **Standalone** (default) - stdio MCP, self-contained
2. **HTTP Server** - Multi-client, shared state
3. **Proxy Daemon** - Enterprise, single TLS connection per user

### Permission Model
- **Server-required**: Minimum capabilities from inventory
- **User-granted**: CLI flags reduce permissions
- **Interactive**: Prompt for sensitive permissions
- **Rule**: Most restrictive wins (intersection)

### Configuration Hierarchy
1. CLI flags (highest priority)
2. Environment variables
3. Config files
4. Inventory defaults
5. Built-in defaults (lowest priority)

## 📖 Reading Guide by Role

### Product Manager
- EXECUTIVE-SUMMARY.md
- PRD.md

### Architect
- TECHNICAL-DECISIONS.md
- architecture-v2.md
- ARCHITECTURE-VISUAL.md

### Backend Developer
- epic-01-inventory-system.md
- epic-02-dual-interface.md
- epic-03-request-execution.md
- epic-07-security-layer.md

### Security Engineer
- epic-07-security-layer.md
- TECHNICAL-DECISIONS.md (permissions, signing)

### DevOps/Platform
- epic-05-configuration.md
- TECHNICAL-DECISIONS.md (bundled server)

### AI/ML Engineer (MCP focus)
- epic-02-dual-interface.md
- story-02-08-mcp-setup.md
- ARCHITECTURE-VISUAL.md

## 🎓 Key Concepts

### Inventory System
Remote HTTPS endpoint (`/_inventory.json`) that provides:
- OpenAPI specification location
- WASM module references
- Authentication configuration
- Client-side controls (rate limiting, retry)
- Version requirements

### Why This Architecture?
**Problem**: SDKs and MCP packages go stale, require updates
**Solution**: Tool configures itself from remote inventory
**Benefits**:
- Always up-to-date
- Works offline (cached)
- No version conflicts
- Dynamic per-user configuration

### Security Model
- **WASM Sandboxing**: Untrusted code runs safely
- **OS Keychain**: Secrets never in env vars
- **mTLS**: Mutual authentication
- **Client Fingerprinting**: Fraud detection
- **Multiple Signing Options**: TLS, GPG, SSH

## ✅ Specification Completeness

| Phase | Status | Documents |
|-------|--------|-----------|
| **Product Definition** | ✅ Complete | PRD.md, EXECUTIVE-SUMMARY.md |
| **Architecture** | ✅ Complete | architecture-v2.md, TECHNICAL-DECISIONS.md |
| **Visual Design** | ✅ Complete | ARCHITECTURE-VISUAL.md |
| **Epic Breakdown** | ✅ Complete | 7 epics defined |
| **Story Examples** | ✅ Complete | 8 sample stories |
| **Quick Start** | ✅ Complete | QUICK-START.md |

**Status**: 🟢 **READY FOR IMPLEMENTATION**

## 🚦 Next Steps

1. **Review**: Read EXECUTIVE-SUMMARY.md for overview
2. **Setup**: Follow QUICK-START.md for development environment
3. **Decisions**: Review TECHNICAL-DECISIONS.md for technical choices
4. **Implement**: Start with Phase 1 (epic-01-inventory-system.md)
5. **Iterate**: Test both CLI and MCP modes after each phase

## 📞 Questions?

If you're an AI agent implementing this:
1. Start with the epics in order (01 → 07)
2. Reference architecture-v2.md for technical details
3. Use the stories as implementation guides
4. Test both CLI and MCP after each epic

---

**Total Specifications**: 23 files, ~6,000 lines  
**BMAD Method Phase**: Solutioning ✅ → Implementation ⏳  
**Last Updated**: 2026-02-28
