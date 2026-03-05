# Linting Configuration

**Version:** 1.0.0  
**Last Updated:** 2026-02-28

---

## golangci-lint Configuration

```yaml
# .golangci.yaml
run:
  timeout: 5m
  tests: true
  skip-dirs:
    - vendor
    - third_party
    - testdata
  skip-files:
    - ".*\\.pb\\.go$"
    - ".*_mock\\.go$"

output:
  formats:
    - format: colored-line-number
  print-issued-lines: true
  print-linter-name: true

linters-settings:
  errcheck:
    check-type-assertions: true
    check-blank: true
  
  govet:
    enable-all: true
    disable:
      - shadow
  
  gocyclo:
    min-complexity: 15
  
  maligned:
    suggest-new: true
  
  dupl:
    threshold: 100
  
  goconst:
    min-len: 3
    min-occurrences: 3
  
  misspell:
    locale: US
  
  lll:
    line-length: 120
  
  goimports:
    local-prefixes: github.com/muster/muster
  
  gocritic:
    enabled-tags:
      - performance
      - style
      - experimental
    disabled-checks:
      - wrapperFunc
  
  funlen:
    lines: 80
    statements: 50

linters:
  enable:
    - bodyclose
    - dogsled
    - dupl
    - errcheck
    - gochecknoinits
    - goconst
    - gocritic
    - gocyclo
    - gofmt
    - goimports
    - golint
    - gomnd
    - goprintffuncname
    - gosec
    - gosimple
    - govet
    - ineffassign
    - interfacer
    - lll
    - misspell
    - nakedret
    - rowserrcheck
    - scopelint
    - staticcheck
    - structcheck
    - stylecheck
    - typecheck
    - unconvert
    - unparam
    - unused
    - varcheck
    - whitespace
    - funlen
  
  disable:
    - depguard      # Too restrictive for our use case
    - gochecknoglobals  # We need some globals (e.g., version)
    - prealloc      # Often causes false positives
    - testpackage   # We test internal packages

issues:
  exclude-rules:
    # Exclude some linters from running on tests
    - path: _test\.go
      linters:
        - gocyclo
        - errcheck
        - dupl
        - gosec
        - lll
        - funlen
    
    # Exclude shadow checking in test files
    - path: _test\.go
      text: "shadow"
      linters:
        - govet
    
    # Exclude known linter issues
    - text: "weak cryptographic primitive"
      linters:
        - gosec
    
    # Exclude line length check for long URLs
    - source: "https://"
      linters:
        - lll
```

---

## Enabled Linters

| Linter | Purpose |
|--------|---------|
| `errcheck` | Unchecked errors |
| `gosimple` | Simplify code |
| `govet` | Go vet checks |
| `ineffassign` | Ineffective assignments |
| `staticcheck` | Static analysis |
| `unused` | Unused code |
| `gocritic` | Code style issues |
| `gosec` | Security issues |
| `goimports` | Import formatting |
| `misspell` | Spelling mistakes |
| `funlen` | Function length |
| `gocyclo` | Cyclomatic complexity |

---

## Disabled Linters and Why

| Linter | Reason |
|--------|--------|
| `depguard` | Too restrictive; blocks valid imports |
| `gochecknoglobals` | False positives on version/config vars |
| `prealloc` | Many false positives in loops |
| `testpackage` | We need to test internal packages |
| `exhaustivestruct` | Too verbose for config structs |
| `forcetypeassert` | Unnecessary with proper error handling |
| `gomnd` | Magic number detection too noisy |

---

## Custom Rules

```yaml
# Additional exclusions in .golangci.yaml
issues:
  exclude:
    # Exclude shadow checking for err variables
    - "declaration of \"err\" shadows"
    
    # Exclude defer in loop warnings (we handle carefully)
    - "deferInLoop"
    
  exclude-use-default: false

  # Maximum issues per linter
  max-issues-per-linter: 50
  
  # Maximum total issues
  max-same-issues: 3
```

---

## Makefile Integration

```makefile
.PHONY: lint lint-fix lint-ci

lint:
	golangci-lint run ./...

lint-fix:
	golangci-lint run --fix ./...

lint-ci:
	golangci-lint run --out-format=github-actions ./...

# Run with custom config
lint-strict:
	golangci-lint run --config=.golangci-strict.yaml ./...
```

---

## CI/CD Integration

```yaml
# .github/workflows/lint.yaml
name: Lint

on: [push, pull_request]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v4
        with:
          version: latest
          args: --timeout=5m
```

---

## IDE Integration

### VS Code
```json
{
  "go.lintTool": "golangci-lint",
  "go.lintFlags": ["--fast"],
  "go.toolsManagement.autoUpdate": false
}
```

### GoLand
Settings → Go → Linters → Enable golangci-lint
