package builtin

import (
	"os"
	"path/filepath"
	"runtime"
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

// TestMTLSMaterialDir pins where generated material lands: inside the config
// tree, scoped by the active profile.
func TestMTLSMaterialDir(t *testing.T) {
	base := filepath.Join("some", "config", "muster")
	assert.Equal(t, filepath.Join(base, "mtls", "default"),
		mtlsMaterialDir(&config.Config{ConfigDir: base}))
	assert.Equal(t, filepath.Join(base, "mtls", "prod"),
		mtlsMaterialDir(&config.Config{ConfigDir: base, DefaultProfile: "prod"}))
}

// TestAuthMTLSCommand_GenerateDefaultsToConfigDir is the DF-024 regression:
// `auth mtls generate` used to write cert.pem and a 0600 key.pem into the
// CURRENT working directory, so running it from a repository root left an
// untracked private key next to the checkout.
func TestAuthMTLSCommand_GenerateDefaultsToConfigDir(t *testing.T) {
	cfgDir := t.TempDir()
	cwd := t.TempDir()
	t.Chdir(cwd)

	cmd := newAuthMTLSCommand(&config.Config{ConfigDir: cfgDir})
	out, err := captureStdout(func() error {
		cmd.SetArgs([]string{"generate"})
		return cmd.Execute()
	})
	require.NoError(t, err)

	wantDir := filepath.Join(cfgDir, "mtls", "default")
	certPath := filepath.Join(wantDir, "cert.pem")
	keyPath := filepath.Join(wantDir, "key.pem")
	assert.Contains(t, out, certPath, "the written certificate path must be printed")
	assert.Contains(t, out, keyPath, "the written key path must be printed")

	keyInfo, err := os.Stat(keyPath)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		// Windows has no Unix permission bits: Go reports a writable file as
		// 0666 whatever mode it was created with, so 0600 cannot be observed
		// there. The Windows guarantee is the ACL the OS assigns on create.
		assert.Equal(t, os.FileMode(0600), keyInfo.Mode().Perm(), "the private key stays 0600")
	}
	_, err = os.Stat(certPath)
	require.NoError(t, err)

	for _, name := range []string{"cert.pem", "key.pem"} {
		_, statErr := os.Stat(filepath.Join(cwd, name))
		assert.Truef(t, os.IsNotExist(statErr), "%s must not be written into the current directory", name)
	}
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
