package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

func SetSecurePermissions(path string) error {
	return os.Chmod(path, 0600)
}

func SetSecureDirPermissions(path string) error {
	return os.Chmod(path, 0700)
}

func EnsureSecureFile(path string) error {
	return os.WriteFile(path, []byte{}, 0600)
}

func IsSecurePath(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().Perm() == 0600
}

func CreateSystemdService() error {
	dirs := Directories()
	servicePath := filepath.Join(dirs.Config, "systemd", "user", "openapi-cli.service")

	if _, err := os.Stat(filepath.Dir(servicePath)); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(servicePath), 0755); err != nil {
			return fmt.Errorf("failed to create systemd directory: %w", err)
		}
	}

	serviceContent := `[Unit]
Description=OpenAPI CLI Daemon
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/openapi-cli daemon start
Restart=always

[Install]
WantedBy=default.target
`

	return os.WriteFile(servicePath, []byte(serviceContent), 0644)
}

func GetSystemdServicePath() string {
	homedir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homedir, ".config", "systemd", "user", "openapi-cli.service")
}
