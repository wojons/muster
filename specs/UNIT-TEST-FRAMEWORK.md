# Unit Test Framework

**Version:** 1.0.0  
**Last Updated:** 2026-02-28

---

## testify/assert Usage

```go
import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestParseConfig(t *testing.T) {
    cfg, err := ParseConfig(`{"timeout": 30}`)
    
    // Assertions don't stop test
    assert.NoError(t, err)
    assert.Equal(t, 30, cfg.Timeout)
    assert.NotNil(t, cfg)
    
    // Require stops test on failure
    require.NoError(t, err)  // Fatal if error
    require.NotNil(t, cfg)   // Stop if nil
}
```

### Common Assertions

```go
// Equality
assert.Equal(t, expected, actual)
assert.NotEqual(t, unexpected, actual)

// Nil/Empty
assert.Nil(t, obj)
assert.NotNil(t, obj)
assert.Empty(t, slice)
assert.NotEmpty(t, slice)

// Errors
assert.NoError(t, err)
assert.Error(t, err)
assert.ErrorIs(t, err, ErrNotFound)
assert.ErrorContains(t, err, "not found")

// Collections
assert.Contains(t, slice, item)
assert.Len(t, slice, 3)
assert.ElementsMatch(t, expected, actual)

// Strings
assert.Contains(t, str, substring)
assert.HasPrefix(t, str, prefix)
assert.Regexp(t, "^test.*", str)
```

---

## Mock Generation with mockgen

### Installation

```bash
go install github.com/golang/mock/mockgen@latest
```

### Interface Definition

```go
// internal/client/http.go
type HTTPClient interface {
    Do(req *http.Request) (*http.Response, error)
    Get(url string) (*http.Response, error)
}
```

### Generate Mock

```bash
#go:generate mockgen -source=http.go -destination=http_mock.go -package=client
mockgen -source=internal/client/http.go \
        -destination=internal/client/http_mock.go \
        -package=client
```

### Using Mocks

```go
import (
    "testing"
    "github.com/golang/mock/gomock"
)

func TestAPIClient_Get(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockHTTP := NewMockHTTPClient(ctrl)
    
    // Set expectations
    mockHTTP.EXPECT().
        Get("https://api.example.com/users").
        Return(&http.Response{StatusCode: 200}, nil).
        Times(1)
    
    client := NewAPIClient(mockHTTP)
    resp, err := client.GetUsers()
    
    assert.NoError(t, err)
    assert.Equal(t, 200, resp.StatusCode)
}
```

---

## Table-Driven Tests

```go
func TestValidateEmail(t *testing.T) {
    tests := []struct {
        name    string
        email   string
        wantErr bool
        errMsg  string
    }{
        {
            name:    "valid email",
            email:   "user@example.com",
            wantErr: false,
        },
        {
            name:    "missing @",
            email:   "userexample.com",
            wantErr: true,
            errMsg:  "missing @",
        },
        {
            name:    "empty string",
            email:   "",
            wantErr: true,
            errMsg:  "email is required",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateEmail(tt.email)
            if tt.wantErr {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.errMsg)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

---

## Test Fixtures

```go
// testdata/users.json
[
  {"id": 1, "name": "Alice", "email": "alice@example.com"},
  {"id": 2, "name": "Bob", "email": "bob@example.com"}
]

// testdata_test.go
func loadTestUsers(t *testing.T) []User {
    data, err := os.ReadFile("testdata/users.json")
    require.NoError(t, err)
    
    var users []User
    err = json.Unmarshal(data, &users)
    require.NoError(t, err)
    
    return users
}

// usage
func TestUserService_Load(t *testing.T) {
    users := loadTestUsers(t)
    assert.Len(t, users, 2)
}
```

---

## Mock HTTP Server

```go
import "net/http/httptest"

func TestAPIClient_WithMockServer(t *testing.T) {
    // Create mock server
    server := httptest.NewServer(http.HandlerFunc(
        func(w http.ResponseWriter, r *http.Request) {
            assert.Equal(t, "/api/users", r.URL.Path)
            assert.Equal(t, "GET", r.Method)
            
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusOK)
            w.Write([]byte(`{"id": 1, "name": "Alice"}`))
        }),
    )
    defer server.Close()

    // Use server URL
    client := NewAPIClient(server.URL)
    user, err := client.GetUser(1)
    
    assert.NoError(t, err)
    assert.Equal(t, "Alice", user.Name)
}
```

---

## Test Helper Functions

```go
// testutil/helpers.go
package testutil

func MustTempDir(t *testing.T) string {
    t.Helper()
    dir, err := os.MkdirTemp("", "muster-test-*")
    require.NoError(t, err)
    
    t.Cleanup(func() {
        os.RemoveAll(dir)
    })
    
    return dir
}

func AssertJSONEqual(t *testing.T, expected, actual string) {
    t.Helper()
    
    var exp, act map[string]interface{}
    json.Unmarshal([]byte(expected), &exp)
    json.Unmarshal([]byte(actual), &act)
    
    assert.Equal(t, exp, act)
}
```
