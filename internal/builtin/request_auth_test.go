package builtin

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/auth"
)

var errUnknownCredential = errors.New("credential not found")

// stubRequestCredential points resolveRequestCredential at a canned
// credential for the duration of the test, so no test touches the platform
// keychain.
func stubRequestCredential(t *testing.T, name, header, value string, resolveErr error) {
	t.Helper()
	orig := resolveRequestCredential
	resolveRequestCredential = func(n string) (auth.CredentialHeader, error) {
		require.Equal(t, name, n)
		if resolveErr != nil {
			return auth.CredentialHeader{}, resolveErr
		}
		return auth.CredentialHeader{Name: header, Value: value, Shielded: "Bearer MASKED"}, nil
	}
	t.Cleanup(func() { resolveRequestCredential = orig })
}

// With a stored bearer credential, --auth NAME sends the Authorization
// header to a live local httptest server (DF-023 acceptance criterion).
func TestHandleRequest_AuthSendsBearerHeader(t *testing.T) {
	var gotAuth, gotAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("X-API-Key")
		w.WriteHeader(200)
	}))
	defer server.Close()

	stubRequestCredential(t, "authsvc", "Authorization", "Bearer live-bearer-1", nil)

	err := handleRequest(nil, "GET", server.URL, nil, nil, "", false, 5*time.Second, "", "", "authsvc")
	require.NoError(t, err)
	require.Equal(t, "Bearer live-bearer-1", gotAuth)
	require.Equal(t, "", gotAPIKey)
}

// api-key credentials attach as X-API-Key (DF-023 acceptance criterion).
func TestHandleRequest_AuthSendsAPIKeyHeader(t *testing.T) {
	var gotAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("X-API-Key")
		w.WriteHeader(200)
	}))
	defer server.Close()

	stubRequestCredential(t, "k", "X-API-Key", "sk-live-1", nil)

	err := handleRequest(nil, "GET", server.URL, nil, nil, "", false, 5*time.Second, "", "", "k")
	require.NoError(t, err)
	require.Equal(t, "sk-live-1", gotAPIKey)
}

// An explicit -H with the same name overrides the stored credential (the
// documented manual remedy keeps working).
func TestHandleRequest_ExplicitHeaderWinsOverAuth(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	defer server.Close()

	stubRequestCredential(t, "authsvc", "Authorization", "Bearer stored", nil)

	headers := []string{"Authorization: Bearer manual"}
	err := handleRequest(nil, "GET", server.URL, headers, nil, "", false, 5*time.Second, "", "", "authsvc")
	require.NoError(t, err)
	require.Equal(t, "Bearer manual", gotAuth)
}

// An unknown credential fails before any request is sent.
func TestHandleRequest_AuthUnknownCredentialFails(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))
	defer server.Close()

	stubRequestCredential(t, "ghost", "", "", errUnknownCredential)

	err := handleRequest(nil, "GET", server.URL, nil, nil, "", false, 5*time.Second, "", "", "ghost")
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolve --auth")
	require.False(t, called)
}

// Verbose output masks credential headers: the secret is never echoed.
// The verbose dump writes to os.Stderr; we swap it for a pipe and assert
// the masked rendering appears and the raw secret does not.
func TestHandleRequest_VerboseMasksCredentialHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	stubRequestCredential(t, "authsvc", "Authorization", "Bearer super-secret-token", nil)

	// Swap os.Stderr for a pipe to capture the verbose dump.
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	reqErr := handleRequest(nil, "GET", server.URL, nil, nil, "", true, 5*time.Second, "", "", "authsvc")

	w.Close()
	os.Stderr = origStderr
	dump, _ := io.ReadAll(r)
	require.NoError(t, reqErr)

	require.Contains(t, string(dump), "Bearer ***",
		"verbose output must render the credential masked")
	require.NotContains(t, string(dump), "super-secret-token",
		"verbose output must never echo the stored secret")
}
