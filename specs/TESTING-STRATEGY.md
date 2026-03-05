# Testing Strategy

**Version:** 1.0.0  
**Last Updated:** 2026-02-28

---

## Testing Pyramid

```
       /\
      /  \        E2E Tests (5%)
     /----\       ~100 tests
    /      \
   /--------\     Integration Tests (15%)
  /          \    ~300 tests
 /------------\   
/              \  Unit Tests (80%)
----------------  ~2000 tests
```

---

## Test Organization

```
internal/
├── cache/
│   ├── cache.go
│   ├── cache_test.go           # Unit tests
│   └── cache_integration_test.go  # Integration tests
├── client/
│   ├── client.go
│   ├── client_test.go
│   └── client_integration_test.go
cmd/
├── cli/
│   ├── main.go
│   └── main_e2e_test.go        # E2E tests
└── e2e/
    └── scenarios/              # E2E test scenarios
        ├── auth_test.go
        └── api_flows_test.go
```

---

## Test Naming Conventions

### Unit Tests
```go
// Format: Test<FunctionName>_<Scenario>
func TestCache_Get_Hit(t *testing.T)
func TestCache_Get_Miss(t *testing.T)
func TestCache_Set_Overwrite(t *testing.T)
func TestParseURL_InvalidInput(t *testing.T)
```

### Integration Tests
```go
// Format: TestIntegration_<Component>_<Scenario>
func TestIntegration_Cache_Postgres(t *testing.T)
func TestIntegration_Client_HTTPTimeout(t *testing.T)
```

### E2E Tests
```go
// Format: TestE2E_<Feature>_<Scenario>
func TestE2E_Auth_LoginFlow(t *testing.T)
func TestE2E_CLI_VersionCommand(t *testing.T)
```

### Benchmarks
```go
// Format: Benchmark<FunctionName>
func BenchmarkCache_Get(b *testing.B)
func BenchmarkParseJSON(b *testing.B)
```

---

## Build Tags

```go
// Unit tests (default)
// go test ./...

// Integration tests
//go:build integration
func TestIntegration_Database(t *testing.T) { }

// E2E tests  
//go:build e2e
func TestE2E_FullFlow(t *testing.T) { }

// Run specific test types
go test -tags=integration ./...
go test -tags=e2e ./cmd/e2e/...
```

---

## Coverage Requirements

| Component | Unit | Integration | E2E |
|-----------|------|-------------|-----|
| `pkg/` libraries | 90% | 70% | - |
| `internal/` packages | 85% | 65% | - |
| `cmd/` binaries | 80% | 60% | 40% |
| Overall | 85% | 65% | 35% |

---

## Makefile Targets

```makefile
.PHONY: test test-unit test-integration test-e2e coverage

test: test-unit test-integration

test-unit:
	go test -v -race -coverprofile=coverage.unit.out ./...

test-integration:
	go test -v -tags=integration -coverprofile=coverage.integration.out ./...

test-e2e:
	go test -v -tags=e2e ./cmd/e2e/...

coverage:
	go tool cover -html=coverage.unit.out -o coverage.html
	@echo "Coverage report: coverage.html"

# CI target
ci-test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | grep total | awk '{print "Total coverage: " $$3}'
```
