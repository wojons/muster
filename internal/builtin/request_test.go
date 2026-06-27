package builtin

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// validateURLScheme
// =============================================================================

func TestValidateURLScheme(t *testing.T) {
	t.Run("http scheme is valid", func(t *testing.T) {
		err := validateURLScheme("http")
		require.NoError(t, err)
	})

	t.Run("https scheme is valid", func(t *testing.T) {
		err := validateURLScheme("https")
		require.NoError(t, err)
	})

	t.Run("ftp scheme is invalid", func(t *testing.T) {
		err := validateURLScheme("ftp")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported scheme")
		assert.Contains(t, err.Error(), `"ftp"`)
	})

	t.Run("empty scheme is invalid", func(t *testing.T) {
		err := validateURLScheme("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported scheme")
	})

	t.Run("HTTP uppercase is case-sensitive and invalid", func(t *testing.T) {
		// The function does exact string comparison, no case-insensitive check
		err := validateURLScheme("HTTP")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"HTTP"`)
	})

	t.Run("unknown scheme gives descriptive error", func(t *testing.T) {
		err := validateURLScheme("gopher")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported scheme")
		assert.Contains(t, err.Error(), `"gopher"`)
	})
}

// =============================================================================
// handleRequest — error paths
// =============================================================================

func TestHandleRequest_ParseURLError(t *testing.T) {
	// A URL with an invalid control character will fail URL parsing
	err := handleRequest(nil, "GET", "http://example.com/\x00path", nil, nil, "", false, 5*time.Second, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse URL")
}

func TestHandleRequest_InvalidScheme(t *testing.T) {
	err := handleRequest(nil, "GET", "ftp://example.com/path", nil, nil, "", false, 5*time.Second, "", "")
	require.Error(t, err)
	// The error wraps: "invalid URL scheme: unsupported scheme ..."
	assert.True(t, strings.Contains(err.Error(), "invalid URL scheme"), "expected 'invalid URL scheme' in error: %v", err)
	assert.True(t, strings.Contains(err.Error(), `"ftp"`), "expected 'ftp' mentioned in error: %v", err)
}
