package request

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// DF-015: ValidateRequestBaseURL must reject the base URLs that cannot
// participate in an HTTP request with the actionable --base-url /
// MUSTER_BASE_URL hint, and pass every usable base through unchanged.
func TestValidateRequestBaseURL(t *testing.T) {
	t.Run("empty base URL is rejected with the actionable hint", func(t *testing.T) {
		err := ValidateRequestBaseURL("")
		require.Error(t, err)
		require.Contains(t, err.Error(), "--base-url")
		require.Contains(t, err.Error(), "MUSTER_BASE_URL")
	})

	t.Run("relative base URL is rejected with the actionable hint", func(t *testing.T) {
		err := ValidateRequestBaseURL("/api/v3")
		require.Error(t, err)
		require.Contains(t, err.Error(), `"/api/v3" is relative`)
		require.Contains(t, err.Error(), "spec declares a relative server URL")
	})

	t.Run("absolute http and https pass", func(t *testing.T) {
		require.NoError(t, ValidateRequestBaseURL("http://localhost:9801"))
		require.NoError(t, ValidateRequestBaseURL("https://api.example.com/v3"))
	})

	t.Run("non-http absolute scheme passes (not this guard's call)", func(t *testing.T) {
		// The guard's contract is "has a scheme" — non-http schemes die at
		// http.NewRequest, not here, and user-supplied overrides are never
		// second-guessed.
		require.NoError(t, ValidateRequestBaseURL("ftp://files.example.com"))
	})
}
