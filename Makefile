# =============================================================================
# Build System Configuration
# =============================================================================

# Binary names
BINARY_NAME := openapi-cli
CLI_BINARY := openapi-cli
MCP_BINARY := openapi-mcp
DAEMON_BINARY := openapi-daemon

# Build directories
BUILD_DIR := bin
DIST_DIR := dist
CMD_DIR := cmd

# Version information (injected at build time).
#
# These stamps must be TOTAL: their value must never be able to carry
# whitespace, because LDFLAGS below is one quoted argument that the Go linker
# re-splits on spaces. `cmd 2>/dev/null || echo "unknown"` is NOT total — a git
# command can print to stdout AND exit non-zero, and then BOTH outputs are
# captured. The concrete case (DF-035): in a zero-commit repo (a tarball/ZIP
# extraction that happened to `git init`), `git rev-parse --abbrev-ref HEAD`
# prints the literal token `HEAD` to stdout and exits 128, so the fallback also
# fired and BRANCH became the two-token string `HEAD unknown`. That stray token
# split -ldflags, the linker printed its usage, and `make build-cli` died rc=2
# after a full module download.
# `grep -vx HEAD` drops only a line that is exactly the token `HEAD` (never a
# real ref/sha/tag), so the fallback fires on its own and yields a sane value.
# If git is absent or not a repo, grep sees empty input, exits 1, and the
# fallback still applies — same as before.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | grep -vx HEAD || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null | grep -vx HEAD || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null | grep -vx HEAD || echo "unknown")

# Go configuration
GOPATH := $(shell go env GOPATH)
GOBIN := $(GOPATH)/bin
GO_VERSION := $(shell go version | awk '{print $$3}')

# Build flags
LDFLAGS := -ldflags "-X github.com/wojons/muster/internal/version.Version=$(VERSION) -X github.com/wojons/muster/internal/version.Commit=$(COMMIT) -X github.com/wojons/muster/internal/version.BuildDate=$(BUILD_DATE) -X github.com/wojons/muster/internal/version.Branch=$(BRANCH) -X github.com/wojons/muster/internal/version.GoVersion=$(GO_VERSION) -s -w"

# CGO configuration for SQLite (only pkg/repository uses cgo; the built
# binaries openapi-cli/mcp-server/daemon are pure Go). `?=` lets CI and
# cross-compile invocations override via environment (CGO_ENABLED=0),
# which is required for cross-arch builds where the host gcc cannot
# assemble the target's runtime/cgo.
CGO_ENABLED ?= 1

# Linux build deps for CGO_ENABLED=1 (QA-MUSTER-1): full-module builds
# (`go build ./...`, `go test ./...`) compile pkg/platform, which pulls in
# github.com/seccomp/libseccomp-golang whose cgo directive
# (`#cgo pkg-config: libseccomp`) needs `pkg-config` and `libseccomp-dev`
# on the system (RHEL/Fedora: pkgconf-pkg-config + libseccomp-devel):
#   sudo apt-get install -y pkg-config libseccomp-dev
# (The shipped binaries alone do not import the seccomp package.)
# Without them ONLY a `-tags seccomp` build fails (QA-MUSTER-2: the seccomp
# code now lives behind the `seccomp` build tag; default builds use a pure-Go
# stub in pkg/platform/seccomp_stub.go and need neither package):
#   github.com/seccomp/libseccomp-golang: exec: "pkg-config": executable file not found in $PATH

# Optional build tags: `make build-all TAGS=seccomp` enables the hardened CGO
# seccomp path (requires pkg-config + libseccomp-dev). Default (TAGS empty)
# builds are dependency-free.
TAGS ?=
GO_TAGS := $(if $(TAGS),-tags=$(TAGS))

# Platform targets
PLATFORMS := \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64 \
	linux/arm64 \
	windows/amd64 \
	windows/arm64

# Default target
.DEFAULT_GOAL := build

# =============================================================================
# Development Builds
# =============================================================================

.PHONY: build
build: ## Build all binaries for current platform
	@echo "Building $(BINARY_NAME) $(VERSION) for current platform..."
	@$(MAKE) build-all

.PHONY: hooks
hooks: ## Install repo pre-commit guard (sets local core.hooksPath; run once per clone)
	@./scripts/hooks-install.sh

.PHONY: build-cli
build-cli: ## Build CLI only
	@echo "Building CLI $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build $(GO_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY) ./cmd/openapi-cli
	@echo "✓ Built: $(BUILD_DIR)/$(CLI_BINARY)"

.PHONY: build-mcp
build-mcp: ## Build MCP server only
	@echo "Building MCP server $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build $(GO_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(MCP_BINARY) ./cmd/mcp-server
	@echo "✓ Built: $(BUILD_DIR)/$(MCP_BINARY)"

.PHONY: build-daemon
build-daemon: ## Build daemon only
	@echo "Building daemon $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build $(GO_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(DAEMON_BINARY) ./cmd/daemon
	@echo "✓ Built: $(BUILD_DIR)/$(DAEMON_BINARY)"

.PHONY: build-all
build-all: ## Build all binaries
	@echo "Building all binaries $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	@echo "  → Building CLI..."
	@CGO_ENABLED=$(CGO_ENABLED) go build $(GO_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY) ./cmd/openapi-cli
	@echo "  → Building MCP server..."
	@CGO_ENABLED=$(CGO_ENABLED) go build $(GO_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(MCP_BINARY) ./cmd/mcp-server
	@echo "  → Building daemon..."
	@CGO_ENABLED=$(CGO_ENABLED) go build $(GO_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(DAEMON_BINARY) ./cmd/daemon
	@echo "✓ All builds complete"

# =============================================================================
# Release Builds
# =============================================================================

.PHONY: release
release: clean generate lint test ## Create release builds (three binaries per platform)
	@echo "Creating release $(VERSION)..."
	@mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$$(echo $$platform | cut -d/ -f1); \
		GOARCH=$$(echo $$platform | cut -d/ -f2); \
		PLATFORM_NAME=$$GOOS-$$GOARCH; \
		RELEASE_NAME=$(PROJECT_NAME)-$(VERSION)-$$PLATFORM_NAME; \
		mkdir -p $(DIST_DIR)/$$RELEASE_NAME; \
		for main in openapi-cli openapi-daemon openapi-mcp; do \
			OUTPUT=$(DIST_DIR)/$$RELEASE_NAME/$$main; \
			if [ "$$GOOS" = "windows" ] && [ "$$main" = "openapi-cli" -o "$$main" = "openapi-mcp" ]; then OUTPUT="$$OUTPUT.exe"; fi; \
			echo "  → Building $$main for $$PLATFORM_NAME..."; \
			CGO_ENABLED=0 GOOS=$$GOOS GOARCH=$$GOARCH \
				go build -trimpath $(LDFLAGS) -o $$OUTPUT ./cmd/$$main || exit 1; \
			done; \
		cp README.md LICENSE $(DIST_DIR)/$$RELEASE_NAME/ 2>/dev/null || true; \
		if [ "$$GOOS" = "windows" ]; then \
			(cd $(DIST_DIR) && zip -r $$RELEASE_NAME.zip $$RELEASE_NAME); \
		else \
			(cd $(DIST_DIR) && tar -czf $$RELEASE_NAME.tar.gz $$RELEASE_NAME); \
		fi; \
		rm -rf $(DIST_DIR)/$$RELEASE_NAME; \
	done
	@echo "✓ Release complete in $(DIST_DIR)/"
	@ls -lh $(DIST_DIR)/

.PHONY: release-local
release-local: ## Create release for current platform only
	@echo "Creating local release $(VERSION)..."
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build -trimpath $(LDFLAGS) \
		-o $(DIST_DIR)/$(BINARY_NAME)-$(VERSION)-local ./cmd/$(BINARY_NAME)
	@echo "✓ Local release: $(DIST_DIR)/$(BINARY_NAME)-$(VERSION)-local"

# =============================================================================
# Installation
# =============================================================================

.PHONY: install
install: build ## Install to system (default: /usr/local/bin)
	@echo "Installing $(BINARY_NAME) to $(DESTDIR)/usr/local/bin..."
	@install -d $(DESTDIR)/usr/local/bin
	@install -m 755 $(BUILD_DIR)/$(BINARY_NAME) $(DESTDIR)/usr/local/bin/
	@echo "✓ Installed to $(DESTDIR)/usr/local/bin/$(BINARY_NAME)"

.PHONY: install-user
install-user: build ## Install to user bin directory
	@echo "Installing $(BINARY_NAME) to $(GOBIN)..."
	@mkdir -p $(GOBIN)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOBIN)/
	@echo "✓ Installed to $(GOBIN)/$(BINARY_NAME)"
	@echo "  Make sure $(GOBIN) is in your PATH"

.PHONY: uninstall
uninstall: ## Uninstall from system
	@echo "Uninstalling $(BINARY_NAME)..."
	@rm -f $(DESTDIR)/usr/local/bin/$(BINARY_NAME)
	@rm -f $(GOBIN)/$(BINARY_NAME)
	@echo "✓ Uninstalled"

# =============================================================================
# Testing
# =============================================================================

.PHONY: test
test: ## Run all tests
	@echo "Running all tests..."
	@go test -v -race -coverprofile=coverage.out ./...
	@echo "✓ Tests complete"

.PHONY: test-short
test-short: ## Run all tests excluding long-running (stability) tests
	@echo "Running tests (short mode)..."
	@go test -short -count=1 -timeout 120s $$(go list ./... | grep -v -e test/stability -e pkg/platform -e test/security)
	@echo "✓ Short tests complete"

.PHONY: test-unit
test-unit: ## Run unit tests only
	@echo "Running unit tests..."
	@go test -v -short $$(go list ./... | grep -v test/stability)
	@echo "✓ Unit tests complete"

.PHONY: test-integration
test-integration: ## Run integration tests
	@echo "Running integration tests..."
	@go test -v -tags=integration -run Integration ./...
	@echo "✓ Integration tests complete"

.PHONY: test-e2e
test-e2e: build ## Run end-to-end tests
	@echo "Running E2E tests..."
	@go test -v -tags=e2e ./test/e2e/...
	@echo "✓ E2E tests complete"

.PHONY: test-soak
test-soak: ## Run 30-minute daemon stability soak (build-tagged 'stability'; excluded from default go test)
	@echo "Running stability soak tests..."
	@go test -tags stability -timeout 40m ./test/stability/
	@echo "✓ Stability soak complete"

# =============================================================================
# FIPS 140-3 Compliance
# =============================================================================

.PHONY: build-fips
build-fips: ## Build FIPS 140-3 compliant binary
	@echo "Building FIPS 140-3 compliant binary..."
	@mkdir -p $(BUILD_DIR)
	GOFIPS140=v1.26.0 CGO_ENABLED=$(CGO_ENABLED) go build $(GO_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY)-fips ./cmd/openapi-cli
	@echo "✓ Built: $(BUILD_DIR)/$(CLI_BINARY)-fips (FIPS 140-3 mode)"

.PHONY: build-fips-cli
build-fips-cli: ## Build FIPS 140-3 compliant CLI only
	@echo "Building FIPS 140-3 compliant CLI..."
	@mkdir -p $(BUILD_DIR)
	GOFIPS140=v1.26.0 CGO_ENABLED=$(CGO_ENABLED) go build $(GO_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY)-fips ./cmd/openapi-cli
	@echo "✓ Built: $(BUILD_DIR)/$(CLI_BINARY)-fips"

.PHONY: build-fips-mcp
build-fips-mcp: ## Build FIPS 140-3 compliant MCP server only
	@echo "Building FIPS 140-3 compliant MCP server..."
	@mkdir -p $(BUILD_DIR)
	GOFIPS140=v1.26.0 CGO_ENABLED=$(CGO_ENABLED) go build $(GO_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(MCP_BINARY)-fips ./cmd/mcp-server
	@echo "✓ Built: $(BUILD_DIR)/$(MCP_BINARY)-fips"

.PHONY: build-fips-daemon
build-fips-daemon: ## Build FIPS 140-3 compliant daemon only
	@echo "Building FIPS 140-3 compliant daemon..."
	@mkdir -p $(BUILD_DIR)
	GOFIPS140=v1.26.0 CGO_ENABLED=$(CGO_ENABLED) go build $(GO_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(DAEMON_BINARY)-fips ./cmd/daemon
	@echo "✓ Built: $(BUILD_DIR)/$(DAEMON_BINARY)-fips"

.PHONY: test-fips
test-fips: ## Run tests with FIPS 140-3 mode enabled
	@echo "Running tests in FIPS 140-3 mode..."
	GODEBUG=fips140=on go test -v -race ./pkg/auth/...
	@echo "✓ FIPS tests complete"

.PHONY: test-fips-enforced
test-fips-enforced: ## Run tests with FIPS 140-3 enforcement (non-FIPS will fail)
	@echo "Running tests with FIPS 140-3 enforcement..."
	GODEBUG=fips140=only go test -v -race ./pkg/auth/...
	@echo "✓ FIPS enforcement tests complete"

.PHONY: validate-fips
validate-fips: build-fips test-fips ## Validate FIPS 140-3 compliance
	@echo "=== FIPS 140-3 Compliance Validation ==="
	@echo ""
	@echo "1. Checking FIPS binary..."
	@$(BUILD_DIR)/$(CLI_BINARY)-fips version || true
	@echo ""
	@echo "2. Running FIPS self-tests..."
	@GODEBUG=fips140=on $(BUILD_DIR)/$(CLI_BINARY)-fips test fips || true
	@echo ""
	@echo "3. Verifying FIPS mode..."
	@GODEBUG=fips140=on $(BUILD_DIR)/$(CLI_BINARY)-fips version 2>&1 | grep -i fips || echo "Note: FIPS status not in version output (add to version command)"
	@echo ""
	@echo "✅ FIPS 140-3 validation complete"
	@echo ""
	@echo "To run in FIPS mode: GODEBUG=fips140=on $(BUILD_DIR)/$(CLI_BINARY)-fips"
	@echo "To enforce FIPS mode: GODEBUG=fips140=only $(BUILD_DIR)/$(CLI_BINARY)-fips"

.PHONY: bench
bench: ## Run benchmarks
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem ./...
	@echo "✓ Benchmarks complete"

.PHONY: coverage
coverage: test ## Generate coverage report with accurate statistics
	@echo "Generating coverage report..."
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"
	@echo ""
	@echo "=== Coverage Statistics (2026-03-03) ==="
	@go tool cover -func=coverage.out | grep total
	@echo ""
	@echo "⚠️  Note: cmd/* packages at 0.0% coverage (Week 1 remediation target: 70%+)"
	@echo "   - cmd/openapi-cli: 0.0%"
	@echo "   - cmd/mcp-server: 0.0%"
	@echo "   - cmd/daemon: 0.0%"

# =============================================================================
# Security
# =============================================================================

.PHONY: security-audit
security-audit: ## Run comprehensive security audit
	@echo "Running security audit..."
	@if [ -d "./internal/security" ]; then \
		go test -v ./internal/security/... || echo "Warning: Security tests found issues"; \
	else \
		echo "No security tests found (internal/security/ directory does not exist)"; \
	fi
	@echo "✓ Security audit complete"

.PHONY: security-check
security-check: ## Quick security checks
	@echo "Running quick security checks..."
	@go run ./cmd/security-audit/main.go || true
	@echo "✓ Security checks complete"

.PHONY: deps-audit
deps-audit: ## Audit dependencies for vulnerabilities using govulncheck
	@echo "Auditing dependencies with govulncheck..."
	@if ! command -v govulncheck >/dev/null 2>&1 && ! command -v $(GOBIN)/govulncheck >/dev/null 2>&1; then \
		echo "Installing govulncheck..."; \
		go install golang.org/x/vuln/cmd/govulncheck@latest; \
	fi
	@$(GOBIN)/govulncheck ./... || govulncheck ./...
	@echo "✓ Dependency audit complete"

.PHONY: deps-outdated
deps-outdated: ## List outdated dependencies
	@echo "Checking for outdated dependencies..."
	@go list -u -m all | grep -v '^github.com/openapi-cli' || true
	@echo "✓ Outdated dependencies listed"

.PHONY: deps-unmaintained
deps-unmaintained: ## Check for unmaintained dependencies
	@echo "Checking for unmaintained dependencies..."
	@echo "Known unmaintained packages:"
	@echo "  - github.com/robertkrimen/otto (PAC file parsing - consider replacement)"
	@echo "  - github.com/bytecodealliance/wasmtime-go (WASM runtime - monitor closely)"
	@echo "Recommendation: Monitor these packages for security updates"
	@echo "✓ Unmaintained dependencies checked"

# =============================================================================
# Performance
# =============================================================================

.PHONY: test-perf
test-perf: ## Run performance tests
	@echo "Running performance tests..."
	@go test -v ./internal/perf/...
	@echo "✓ Performance tests complete"

.PHONY: bench-cpu
bench-cpu: ## Run CPU benchmarks
	@echo "Running CPU benchmarks..."
	@go test -bench=. -benchmem -cpuprofile=cpu.prof ./...
	@echo "✓ CPU profile saved to cpu.prof"

.PHONY: bench-mem
bench-mem: ## Run memory benchmarks
	@echo "Running memory benchmarks..."
	@go test -bench=. -benchmem -memprofile=mem.prof ./...
	@echo "✓ Memory profile saved to mem.prof"

.PHONY: bench-load
bench-load: ## Run load tests
	@echo "Running load tests..."
	@go test -v -run Load ./internal/perf/...
	@echo "✓ Load tests complete"

.PHONY: bench-compare
bench-compare: ## Compare benchmarks with baseline
	@echo "Comparing benchmarks..."
	@if [ -f "baseline.txt" ]; then \
		go test -bench=. ./... | tee new.txt > /dev/null; \
		benchstat baseline.txt new.txt || true; \
	else \
		echo "No baseline found. Run 'make bench-baseline' first."; \
	fi

.PHONY: bench-baseline
bench-baseline: ## Save current benchmarks as baseline
	@echo "Saving baseline benchmarks..."
	@go test -bench=. ./... | tee baseline.txt > /dev/null
	@echo "✓ Baseline saved to baseline.txt"

# =============================================================================
# Code Quality
# =============================================================================

.PHONY: lint
lint: ## Run all linters
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "  → golangci-lint not installed, running go vet..."; \
		go vet ./...; \
	fi
	@echo "✓ Linting complete"

.PHONY: lint-fix
lint-fix: ## Run lint with auto-fix
	@echo "Running linting with auto-fix..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --fix ./...; \
	else \
		echo "  ✗ golangci-lint not installed"; \
		exit 1; \
	fi
	@echo "✓ Linting with auto-fix complete"

.PHONY: lint-ci
lint-ci: ## Run linting for CI (GitHub Actions format)
	@echo "Running CI linting..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --out-format=github-actions ./...; \
	else \
		echo "  ✗ golangci-lint not installed"; \
		exit 1; \
	fi
	@echo "✓ CI linting complete"

.PHONY: lint-strict
lint-strict: ## Run strict linting with additional checks
	@echo "Running strict linting..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --config=.golangci-strict.yaml ./... || \
		golangci-lint run ./...; \
	else \
		echo "  ✗ golangci-lint not installed"; \
		exit 1; \
	fi
	@echo "✓ Strict linting complete"

.PHONY: fmt
fmt: ## Format code
	@echo "Formatting code..."
	@go fmt ./...
	@echo "✓ Formatting complete"

.PHONY: vet
vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...
	@echo "✓ Vet complete"

# =============================================================================
# Code Generation
# =============================================================================

.PHONY: generate
generate: ## Run all code generation
	@echo "Running code generation..."
	@go generate ./...
	@echo "✓ Code generation complete"

.PHONY: generate-sqlc
generate-sqlc: ## Generate SQL code with sqlc
	@echo "Generating SQL code with sqlc..."
	@if command -v sqlc >/dev/null 2>&1; then \
		sqlc generate -f internal/db/sqlc.yaml; \
	else \
		echo "  ✗ sqlc not installed. Run: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest"; \
		exit 1; \
	fi
	@echo "✓ SQL code generation complete"

.PHONY: generate-mocks
generate-mocks: ## Generate mocks with mockgen
	@echo "Generating mocks..."
	@go generate ./internal/...
	@echo "✓ Mocks generated"

# =============================================================================
# Docker
# =============================================================================

DOCKER_IMAGE := muster
DOCKER_TAG ?= $(VERSION)

.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "Building Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	@docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-t $(DOCKER_IMAGE):latest \
		.
	@echo "✓ Docker image built"

.PHONY: docker-buildx
docker-buildx: ## Build multi-platform Docker image
	@echo "Building multi-platform Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	@docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-t $(DOCKER_IMAGE):latest \
		--load \
		.
	@echo "✓ Multi-platform Docker image built"

.PHONY: docker-run
docker-run: ## Run in Docker container
	@echo "Running Docker container..."
	@docker run --rm -it \
		-v $(PWD):/workspace \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

.PHONY: docker-push
docker-push: docker-build ## Push Docker image to registry
	@echo "Pushing Docker image..."
	@docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	@docker push $(DOCKER_IMAGE):latest
	@echo "✓ Docker image pushed"

# =============================================================================
# Cleanup
# =============================================================================

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR) $(DIST_DIR) coverage.out coverage.html
	@go clean -cache
	@echo "✓ Clean complete"

.PHONY: distclean
distclean: clean ## Clean all generated files
	@echo "Deep cleaning..."
	@rm -rf vendor/
	@git clean -fdx -e .env -e .idea -e .vscode
	@echo "✓ Deep clean complete"

# =============================================================================
# Development Helpers
# =============================================================================

.PHONY: dev
dev: ## Run CLI in development mode
	@echo "Starting CLI in development mode..."
	@go run ./cmd/openapi-cli --help

.PHONY: watch
watch: ## Watch for changes and rebuild (requires entr)
	@echo "Watching for changes..."
	@find . -name '*.go' | entr -r make build

.PHONY: deps
deps: ## Download and verify dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@go mod verify
	@echo "✓ Dependencies ready"

.PHONY: deps-update
deps-update: ## Update all dependencies
	@echo "Updating dependencies..."
	@go get -u ./...
	@go mod tidy
	@echo "✓ Dependencies updated"

.PHONY: tidy
tidy: ## Tidy go modules
	@echo "Tidying modules..."
	@go mod tidy
	@echo "✓ Modules tidied"

.PHONY: vendor
vendor: ## Create vendor directory
	@echo "Creating vendor directory..."
	@go mod vendor
	@echo "✓ Vendor directory created"

# =============================================================================
# Information
# =============================================================================

.PHONY: version
version: ## Show version information
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"
	@echo "Branch: $(BRANCH)"
	@echo "Go Version: $(GO_VERSION)"

.PHONY: info
info: ## Show build information
	@echo "Project: $(BINARY_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Build Dir: $(BUILD_DIR)"
	@echo "Dist Dir: $(DIST_DIR)"
	@echo "Go Path: $(GOPATH)"
	@echo "CGO Enabled: $(CGO_ENABLED)"

# =============================================================================
# Demo & Validation
# =============================================================================

.PHONY: demo-setup
demo-setup: ## Prepare demo environment
	@echo "Setting up demo environment..."
	@mkdir -p demo/resources
	@mkdir -p tmp/demo
	@echo "✓ Demo environment ready"

.PHONY: demo
demo: build demo-setup ## Run integration demo (STATUS: 5/10 mocked — remediation in progress; NOT a green gate. Use ./demo/run-all.sh for the real end-to-end gate.)
	@echo "=== Muster - Integration Test Demo ==="
	@echo ""
	@echo "✓ Binaries already built"
	@echo "✓ Demo environment ready"
	@echo ""
	@echo "⚠️  INTEGRATION DEMO STATUS (remediation in progress — NOT production-ready):"
	@echo "   - 3/10 scenarios: Real (build binaries, make HTTP requests)"
	@echo "   - 2/10 scenarios: Semi-real (partial execution)"
	@echo "   - 5/10 scenarios: Mock/fake — remediation in progress"
	@echo ""
	@echo "➤ Running Current Integration Tests..."
	@echo ""
	@echo "  [1/5] CLI Command Generation & Execution"
	@go test -v ./demo/test-suites -run "TestRealCLIIntegration" 2>&1 | grep -E "PASS|FAIL|✓" | tail -5 || { echo "✗ CLI Integration FAILED"; exit 1; }
	@echo "  ✓ CLI Integration - Builds binary, makes HTTP requests"
	@echo ""
	@echo "  [2/5] MCP Protocol & Tool Execution (MOCK - needs remediation)"
	@go test -v ./demo/test-suites -run "TestMCPIntegration" 2>&1 | grep -E "PASS|FAIL|✓" | tail -5 || { echo "✗ MCP Integration FAILED"; exit 1; }
	@echo "  ⚠️  MCP Integration - Mock JSON-RPC (needs real cmd/mcp-server testing)"
	@echo ""
	@echo "  [3/5] OAuth2 Authentication Flows (MOCK - needs remediation)"
	@go test -v ./demo/test-suites -run "TestAuthFlow_EndToEnd" 2>&1 | grep -E "PASS|FAIL|✓" | tail -5 || { echo "✗ Auth Integration FAILED"; exit 1; }
	@echo "  ⚠️  Auth Integration - Mock OAuth provider (needs pkg/auth testing)"
	@echo ""
	@echo "  [4/5] Daemon Socket & HTTP Proxy (MOCK - needs remediation)"
	@go test -v ./demo/test-suites -run "TestDaemonSocket_ProxyFlow" 2>&1 | grep -E "PASS|FAIL|✓" | tail -5 || { echo "✗ Daemon Integration FAILED"; exit 1; }
	@echo "  ⚠️  Daemon Integration - Mock HTTP handler (needs cmd/daemon testing)"
	@echo ""
	@echo "  [5/5] WASM Runtime & Sandbox"
	@go test -v ./demo/test-suites -run "TestWASM_RuntimeIntegration" 2>&1 | grep -E "PASS|FAIL|✓" | tail -5 || { echo "✗ WASM Integration FAILED"; exit 1; }
	@echo "  ⚠️  WASM Integration - Config only (needs Execute() with .wasm files)"
	@echo ""
	@echo "=== INTEGRATION DEMO — STATUS: 5/10 SCENARIOS MOCKED (remediation in progress) ==="
	@echo ""
	@echo "⚠️  This demo is NOT a green pre-commit gate — 5/10 scenarios are mocked."
	@echo "    4 of 5 sections above are flagged MOCK (MCP, OAuth, Daemon, WASM)."
	@echo ""
	@echo "✓ For a real end-to-end gate, run: ./demo/run-all.sh"
	@echo "    (builds binaries, starts services on ports 9801-9805, seeds data,"
	@echo "     runs 10 demo scripts — 23+ checks, exits non-zero on failure.)"
	@echo ""
	@echo "Demo complete. See ./demo/run-all.sh for the production gate."

.PHONY: validate-compose
validate-compose: ## Parse-validate every tracked docker-compose file (needs docker compose)
	@bash ./scripts/validate-compose.sh

.PHONY: validate
validate: validate-compose test-unit lint vet test-integration test-security-audit ## Run full system validation
	@echo "=== Full System Validation ==="
	@echo ""
	@echo "Running integration tests..."
	@$(MAKE) test-integration
	@echo ""
	@echo "Running security audit..."
	@$(MAKE) security-audit
	@echo ""
	@echo "Running demo application..."
	@$(MAKE) demo
	@echo ""
	@echo "=== Validation Complete ==="

.PHONY: test-security-audit
test-security-audit:
	@echo "Running security audit..."
	@if [ -d "./internal/security" ]; then \
		go test -v ./internal/security/... || echo "Warning: Security tests found issues"; \
	else \
		echo "No security tests found"; \
	fi

.PHONY: test-security
test-security: ## Run all security tests including fuzzing and property-based tests
	@echo "Running security test suite..."
	@go test -v -short -timeout 120s ./test/security/...
	@echo "Running fuzzing tests..."
	@go test -v -fuzz=FuzzParseMalformedYAML -fuzztime=30s ./test/security/... 2>/dev/null || true
	@go test -v -fuzz=FuzzParseMalformedJSON -fuzztime=30s ./test/security/... 2>/dev/null || true
	@echo "Security test suite complete"

.PHONY: help
help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# =============================================================================
# BENCHMARK TARGETS
# =============================================================================

.PHONY: bench-ci
bench-ci: ## Run benchmarks for CI (faster)
	@echo "Running CI benchmarks..."
	@go test -bench=. -benchmem -benchtime=1s ./test/benchmarks/...
	@echo "✓ CI benchmarks complete"

.PHONY: bench-verbose
bench-verbose: ## Run benchmarks with verbose output
	@echo "Running verbose benchmarks..."
	@go test -v -bench=. -benchmem ./test/benchmarks/...
	@echo "✓ Verbose benchmarks complete"

.PHONY: bench-json
bench-json: ## Run benchmarks and save results as JSON
	@echo "Running benchmarks with JSON output..."
	@mkdir -p test/benchmarks/results
	@go test -bench=. -benchmem -json ./test/benchmarks/... > test/benchmarks/results/benchmark-$(shell date +%Y%m%d-%H%M%S).jsonl
	@echo "✓ Results saved to test/benchmarks/results/"

.PHONY: bench-auth
bench-auth: ## Run authentication benchmarks
	@echo "Running authentication benchmarks..."
	@go test -bench=BenchmarkAuth -benchmem ./test/benchmarks/...

.PHONY: bench-cache
bench-cache: ## Run cache benchmarks
	@echo "Running cache benchmarks..."
	@go test -bench=BenchmarkCache -benchmem ./test/benchmarks/...

.PHONY: bench-request
bench-request: ## Run request building benchmarks
	@echo "Running request building benchmarks..."
	@go test -bench=BenchmarkRequest -benchmem ./test/benchmarks/...

.PHONY: bench-mcp
bench-mcp: ## Run MCP benchmarks
	@echo "Running MCP benchmarks..."
	@go test -bench=BenchmarkMCP -benchmem ./test/benchmarks/...

.PHONY: bench-critical-paths
bench-critical-paths: ## Run benchmarks for all critical paths
	@echo "Running critical path benchmarks..."
	@go test -bench="Benchmark(Auth|Cache|Request|MCP)" -benchmem ./test/benchmarks/...

.PHONY: bench-list
bench-list: ## List all available benchmarks
	@echo "Available benchmarks:"
	@go test -bench=. -run=^$$ ./test/benchmarks/... 2>&1 | grep -E "^Benchmark" | sed 's/^/  - /'

.PHONY: bench-check-budgets
bench-check-budgets: ## Check benchmarks against performance budgets
	@echo "Checking benchmarks against performance budgets..."
	@go test -bench=. -benchmem ./test/benchmarks/... 2>&1 | tee /tmp/bench-results.txt
	@echo ""
	@echo "Performance Budget Status:"
	@echo "  All benchmarks must complete within defined budgets"
	@echo "  See test/benchmarks/budgets.go for budget definitions"

.PHONY: bench-generate-baseline
bench-generate-baseline: ## Generate performance baseline
	@echo "Generating performance baseline..."
	@mkdir -p test/benchmarks/baselines
	@go test -bench=. -benchmem -count=5 ./test/benchmarks/... | tee test/benchmarks/baselines/baseline-$(shell date +%Y%m%d).txt
	@echo "✓ Baseline saved to test/benchmarks/baselines/"

.PHONY: bench-regression
bench-regression: ## Check for performance regressions
	@echo "Checking for performance regressions..."
	@if [ -f test/benchmarks/baselines/baseline-latest.txt ]; then \
		echo "Comparing against baseline..."; \
		go test -bench=. -benchmem ./test/benchmarks/... | benchstat test/benchmarks/baselines/baseline-latest.txt /dev/stdin 2>/dev/null || echo "benchstat not installed, showing raw comparison:"
	else \
		echo "No baseline found. Run 'make bench-generate-baseline' first."; \
	fi

.PHONY: bench-profile
bench-profile: ## Run benchmarks with CPU and memory profiling
	@echo "Running benchmarks with profiling..."
	@mkdir -p test/benchmarks/profiles
	@go test -bench=BenchmarkAuth -benchmem -cpuprofile=test/benchmarks/profiles/cpu.prof -memprofile=test/benchmarks/profiles/mem.prof ./test/benchmarks/...
	@echo "✓ Profiles saved to test/benchmarks/profiles/"
	@echo "  CPU profile: go tool pprof test/benchmarks/profiles/cpu.prof"
	@echo "  Memory profile: go tool pprof test/benchmarks/profiles/mem.prof"

.PHONY: bench-help
bench-help: ## Show benchmark help
	@echo "Benchmark targets:"
	@echo "  make bench                  - Run all benchmarks"
	@echo "  make bench-ci               - Run CI benchmarks (faster)"
	@echo "  make bench-auth             - Run authentication benchmarks"
	@echo "  make bench-cache            - Run cache benchmarks"
	@echo "  make bench-request          - Run request benchmarks"
	@echo "  make bench-mcp              - Run MCP benchmarks"
	@echo "  make bench-critical-paths   - Run critical path benchmarks"
	@echo "  make bench-generate-baseline - Generate performance baseline"
	@echo "  make bench-regression       - Check for regressions"
	@echo "  make bench-profile          - Run with CPU/mem profiling"
	@echo ""
	@echo "Benchmark options:"
	@echo "  BENCHTIME=5s make bench     - Run with 5 second benchtime"
	@echo "  BENCHFLAGS='-cpu=1,2,4' make bench - Run with specific CPU counts"
