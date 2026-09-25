package auth

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memKeychain is an in-memory Keychain for tests.
type memKeychain struct {
	store map[string]string
}

func newMemKeychain() *memKeychain { return &memKeychain{store: map[string]string{}} }

func (m *memKeychain) Set(service, account, password string) error {
	m.store[service+":"+account] = password
	return nil
}

func (m *memKeychain) Get(service, account string) (string, error) {
	if v, ok := m.store[service+":"+account]; ok {
		return v, nil
	}
	return "", ErrNotFound
}

func (m *memKeychain) Delete(service, account string) error {
	delete(m.store, service+":"+account)
	return nil
}

func (m *memKeychain) List(service string) ([]Account, error) { return nil, nil }

func TestResolveCredentialHeader_Bearer(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeCredMeta(dir, map[string]string{"authsvc": "bearer"}))
	kc := newMemKeychain()
	require.NoError(t, kc.Set("muster-cli", "authsvc", "tok-123"))

	cred, err := ResolveCredentialHeaderFrom(kc, dir, "authsvc")
	require.NoError(t, err)
	assert.Equal(t, "Authorization", cred.Name)
	assert.Equal(t, "Bearer tok-123", cred.Value)
}

func TestResolveCredentialHeader_APIKey(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeCredMeta(dir, map[string]string{"k": "api-key"}))
	kc := newMemKeychain()
	require.NoError(t, kc.Set("muster-cli", "k", "sk-abc"))

	cred, err := ResolveCredentialHeaderFrom(kc, dir, "k")
	require.NoError(t, err)
	assert.Equal(t, "X-API-Key", cred.Name)
	assert.Equal(t, "sk-abc", cred.Value)
}

func TestResolveCredentialHeader_NotFound(t *testing.T) {
	_, err := ResolveCredentialHeaderFrom(newMemKeychain(), t.TempDir(), "ghost")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// The resolver must never leak the secret into error messages or the
// masked rendering.
func TestResolveCredentialHeader_Masking(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeCredMeta(dir, map[string]string{"authsvc": "bearer"}))
	kc := newMemKeychain()
	require.NoError(t, kc.Set("muster-cli", "authsvc", "super-secret-token"))

	cred, err := ResolveCredentialHeaderFrom(kc, dir, "authsvc")
	require.NoError(t, err)
	assert.Equal(t, "Bearer ***", cred.Shielded)
	assert.NotContains(t, cred.Shielded, "super-secret-token")

	_, err = ResolveCredentialHeaderFrom(newMemKeychain(), dir, "nope")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "super-secret-token")
}

// writeCredMeta writes a credentials.json metadata file into dir.
func writeCredMeta(dir string, meta map[string]string) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, CredentialMetaFileName), data, 0600)
}

func TestLoadCredentialType_FromMeta(t *testing.T) {
	dir := t.TempDir()
	meta := map[string]string{"authsvc": "bearer", "k": "api-key"}
	data, err := json.Marshal(meta)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, CredentialMetaFileName), data, 0600))

	assert.Equal(t, "bearer", LoadCredentialType(dir, "authsvc"))
	assert.Equal(t, "api-key", LoadCredentialType(dir, "k"))
	assert.Equal(t, "", LoadCredentialType(dir, "missing"))
	assert.Equal(t, "", LoadCredentialType(t.TempDir(), "anything"))
	assert.Equal(t, "", LoadCredentialType("", "x"))
}

// Integration: credential metadata + resolver produce the right header
// against a real keychain-ish flow, and AttachCredential sets it on a
// live http.Request.
func TestCredentialResolution_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	meta := map[string]string{"authsvc": "bearer", "k": "api-key"}
	data, _ := json.Marshal(meta)
	require.NoError(t, os.WriteFile(filepath.Join(dir, CredentialMetaFileName), data, 0600))

	kc := newMemKeychain()
	require.NoError(t, kc.Set("muster-cli", "authsvc", "tok-e2e"))
	require.NoError(t, kc.Set("muster-cli", "k", "sk-e2e"))

	req, err := http.NewRequest("GET", "http://localhost/x", nil)
	require.NoError(t, err)

	bearer, err := ResolveCredentialHeaderFrom(kc, dir, "authsvc")
	require.NoError(t, err)
	AttachCredential(req, bearer)
	assert.Equal(t, "Bearer tok-e2e", req.Header.Get("Authorization"))
	assert.Equal(t, "", req.Header.Get("X-API-Key"))

	// A later explicit header overrides the stored credential (user's
	// -H wins over the resolved value).
	req.Header.Set("Authorization", "Bearer manual-override")
	assert.Equal(t, "Bearer manual-override", req.Header.Get("Authorization"))

	apiKey, err := ResolveCredentialHeader(kc, "k")
	require.NoError(t, err)
	assert.Equal(t, "X-API-Key", apiKey.Name)
	assert.Equal(t, "sk-e2e", apiKey.Value)
	assert.Equal(t, "***", apiKey.Shielded)
}
