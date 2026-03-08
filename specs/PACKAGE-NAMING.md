# Package Naming Conventions

**Version:** 1.0.0  
**Last Updated:** 2026-02-28

---

## Import Path Conventions

```go
// Root module
import "github.com/openapi-cli/openapi-cli"

// Internal packages (implementation detail)
import "github.com/openapi-cli/openapi-cli/internal/cache"

// Public packages (reusable libraries)
import "github.com/openapi-cli/openapi-cli/pkg/config"

// Command entry points
import "github.com/openapi-cli/openapi-cli/cmd/cli"
```

---

## Package Naming Rules

| Location | Purpose | Example |
|----------|---------|---------|
| `cmd/` | Main packages (binaries) | `cmd/cli`, `cmd/mcp` |
| `pkg/` | Public API/libraries | `pkg/config`, `pkg/client` |
| `internal/` | Private implementation | `internal/cache`, `internal/db` |
| `api/` | API definitions/proto | `api/v1/service.proto` |
| `web/` | Web UI assets | `web/static/`, `web/templates/` |

---

## Naming Patterns

### Interfaces
```go
// Reader is implemented by types that can read data
type Reader interface {
    Read(p []byte) (n int, err error)
}

// HTTPClient defines HTTP client behavior
type HTTPClient interface {
    Do(req *http.Request) (*http.Response, error)
}
```

### Structs
```go
// FileCache implements caching with filesystem backend
type FileCache struct {
    dir string
    mu  sync.RWMutex
}

// DefaultConfig returns production-ready defaults
func DefaultConfig() Config {
    return Config{Timeout: 30 * time.Second}
}
```

### Functions
```go
// NewClient creates a new API client
func NewClient(baseURL string) (*Client, error)

// MustParse panics on invalid input
func MustParse(input string) Result

// IsValid checks if configuration is valid
func (c Config) IsValid() error
```

---

## Import Organization

```go
import (
    // Standard library
    "context"
    "fmt"
    "os"

    // Third-party packages
    "github.com/spf13/cobra"
    "github.com/spf13/viper"

    // Internal packages
    "github.com/openapi-cli/openapi-cli/internal/cache"
    "github.com/openapi-cli/openapi-cli/pkg/config"
)
```
