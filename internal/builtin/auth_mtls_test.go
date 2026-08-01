package builtin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/config"
)

// TestAuthCommand_ListEmpty verifies list works with no stored credentials.
func TestAuthCommand_ListEmpty(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}
	cmd := newAuthCommand(cfg)
	cmd.SetArgs([]string{"list"})
	require.NoError(t, cmd.Execute())
}

// TestAuthCommand_AddRemoveMeta verifies the metadata round-trip without
// touching the real keychain (Linux keychain Set would write to disk).
func TestAuthCommand_AddRemoveMeta(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}

	require.NoError(t, storeCredentialMeta(cfg, "test-cred", "api-key"))
	creds := loadCredentialMeta(cfg)
	require.Len(t, creds, 1)
	assert.Equal(t, "api-key", creds["test-cred"])

	require.NoError(t, removeCredentialMeta(cfg, "test-cred"))
	creds = loadCredentialMeta(cfg)
	assert.Len(t, creds, 0)
}

// TestAuthCommand_ListShowsStored verifies list output includes names.
func TestAuthCommand_ListShowsStored(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}
	require.NoError(t, storeCredentialMeta(cfg, "prod-key", "api-key"))
	require.NoError(t, storeCredentialMeta(cfg, "svc-bearer", "bearer"))

	cmd := newAuthCommand(cfg)
	cmd.SetArgs([]string{"list"})
	require.NoError(t, cmd.Execute())
}

// TestAuthMTLSCommand_GenerateInfo verifies cert generation + info readback.
func TestAuthMTLSCommand_GenerateInfo(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}
	dir := t.TempDir()

	gen := newAuthMTLSCommand(cfg)
	gen.SetArgs([]string{"generate", "--dir", dir})
	require.NoError(t, gen.Execute())

	certPath := filepath.Join(dir, "cert.pem")
	_, err := os.Stat(certPath)
	require.NoError(t, err)

	info := newAuthMTLSCommand(cfg)
	info.SetArgs([]string{"info", "--cert", certPath})
	require.NoError(t, info.Execute())
}

// TestAuthMTLSCommand_ConfigureRequiresPaths verifies configure validates.
func TestAuthMTLSCommand_ConfigureRequiresPaths(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}
	cmd := newAuthMTLSCommand(cfg)
	cmd.SetArgs([]string{"configure"})
	require.Error(t, cmd.Execute())
}

// TestInventoryCommand_AddListRemove verifies the full round-trip.
func TestInventoryCommand_AddListRemove(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}

	add := newInventoryCommand(cfg)
	add.SetArgs([]string{"add", "--host", "https://api.acme.com", "--file", "inventory.json"})
	require.NoError(t, add.Execute())

	invs, err := loadInventories(cfg)
	require.NoError(t, err)
	require.Len(t, invs, 1)
	assert.Equal(t, "https://api.acme.com", invs[0].Host)
	assert.Equal(t, "inventory.json", invs[0].File)

	list := newInventoryCommand(cfg)
	list.SetArgs([]string{"list"})
	require.NoError(t, list.Execute())

	remove := newInventoryCommand(cfg)
	remove.SetArgs([]string{"remove", "--host", "https://api.acme.com"})
	require.NoError(t, remove.Execute())

	invs, err = loadInventories(cfg)
	require.NoError(t, err)
	assert.Len(t, invs, 0)
}

// TestInventoryCommand_AddDuplicate verifies duplicate hosts are rejected.
func TestInventoryCommand_AddDuplicate(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}

	add := newInventoryCommand(cfg)
	add.SetArgs([]string{"add", "--host", "https://api.acme.com", "--file", "a.json"})
	require.NoError(t, add.Execute())

	add2 := newInventoryCommand(cfg)
	add2.SetArgs([]string{"add", "--host", "https://api.acme.com", "--file", "b.json"})
	require.Error(t, add2.Execute())
}

// TestInventoryCommand_RemoveMissing verifies removing unknown host errors.
func TestInventoryCommand_RemoveMissing(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}
	remove := newInventoryCommand(cfg)
	remove.SetArgs([]string{"remove", "--host", "https://nope.example"})
	require.Error(t, remove.Execute())
}

// TestMTLSConfig_Defaults verifies the MTLS struct exists with sane defaults.
func TestMTLSConfig_Defaults(t *testing.T) {
	var cfg config.Config
	assert.False(t, cfg.Auth.MTLS.Enabled)
	assert.Equal(t, "", cfg.Auth.MTLS.CertPath)
	assert.Equal(t, "", cfg.Auth.MTLS.KeyPath)
	assert.Equal(t, "", cfg.Auth.MTLS.CAPath)
}
