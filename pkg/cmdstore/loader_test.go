package cmdstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLoader(t *testing.T) {
	store := NewCommandStore()
	err := store.Save(&StoredData{
		Commands: []CommandMetadata{
			{Name: "test", Use: "test", Short: "test"},
		},
	})
	require.NoError(t, err)

	loader := NewLoader(store, "http://example.com")
	require.NotNil(t, loader)

	baseURL, err := loader.GetBaseURL()
	require.NoError(t, err)
	assert.Equal(t, "http://example.com", baseURL)
}

func TestGetBaseURL(t *testing.T) {
	store := NewCommandStore()
	err := store.Save(&StoredData{
		Commands: []CommandMetadata{
			{Name: "test", Use: "test", Short: "test"},
		},
	})
	require.NoError(t, err)

	loader := NewLoader(store, "http://example.com")
	baseURL, err := loader.GetBaseURL()
	require.NoError(t, err)
	assert.Equal(t, "http://example.com", baseURL)
}

func TestSetHeaders(t *testing.T) {
	store := NewCommandStore()
	loader := NewLoader(store, "http://example.com")

	headers := []string{"X-Api-Key: abc123", "Accept: application/json"}
	loader.SetHeaders(headers)
	assert.Equal(t, headers, loader.headers)
}

func TestLoaderReconstructCommand(t *testing.T) {
	store := NewCommandStore()
	loader := NewLoader(store, "http://example.com")

	meta := CommandMetadata{
		Name:  "test-cmd",
		Use:   "test-cmd",
		Short: "A test command",
	}

	cmd, err := loader.reconstructCommand(meta)
	require.NoError(t, err)
	require.NotNil(t, cmd)
	assert.Equal(t, "test-cmd", cmd.Use)
	assert.Equal(t, "A test command", cmd.Short)
	assert.NotNil(t, cmd.RunE)
}
