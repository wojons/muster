package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadConfig_AuthTokenFromEnvironment(t *testing.T) {
	oldAuthTokenFile := *authTokenFileFlag
	defer func() { *authTokenFileFlag = oldAuthTokenFile }()

	*authTokenFileFlag = ""
	t.Setenv("MUSTER_AUTH_TOKEN", "environment-regression-token")

	config := loadConfig()
	require.Equal(t, "environment-regression-token", config.AuthToken)
	require.Empty(t, config.AuthTokenFile)
}

func TestResolveAuthToken_FileOverridesEnvironment(t *testing.T) {
	const fileToken = "file-regression-token"

	path := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(path, []byte(fileToken+"\n"), 0o600))
	config := &MCPServerConfig{
		AuthToken:     "environment-regression-token",
		AuthTokenFile: path,
	}

	require.NoError(t, resolveAuthToken(config))
	require.Equal(t, fileToken, config.AuthToken)
}

func TestResolveAuthToken_EmptyFileFailsWithoutLeakingCredential(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(path, []byte("\n"), 0o600))
	config := &MCPServerConfig{
		AuthToken:     "environment-regression-token",
		AuthTokenFile: path,
	}

	err := resolveAuthToken(config)
	require.Error(t, err)
	require.Contains(t, err.Error(), "is empty")
	require.NotContains(t, err.Error(), config.AuthToken)
}

func TestNewMCPHTTPClient_BearerAuthOnEveryRequest(t *testing.T) {
	const token = "jsonrpc-regression-token"

	var requests int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("Authorization = %q, want configured bearer credential", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(api.Close)

	client := newMCPHTTPClient(token)
	for _, path := range []string{"/discovery", "/execute"} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, api.URL+path, nil)
		require.NoError(t, err)
		resp, err := client.Do(context.Background(), req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	require.Equal(t, 2, requests)
}

func TestNewMCPHTTPClient_WithoutTokenSendsNoAuthorizationHeader(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want no header", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(api.Close)

	client := newMCPHTTPClient("")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, api.URL, nil)
	require.NoError(t, err)
	resp, err := client.Do(context.Background(), req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func TestAuthConfigurationErrorsDoNotContainToken(t *testing.T) {
	const token = "must-not-appear-in-errors"
	config := &MCPServerConfig{
		AuthToken:     token,
		AuthTokenFile: filepath.Join(t.TempDir(), "missing"),
	}

	err := resolveAuthToken(config)
	require.Error(t, err)
	require.False(t, strings.Contains(err.Error(), token))
}
