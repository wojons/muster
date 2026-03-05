package platform

import (
	"os"
	"path/filepath"
)

func macOSDirs() *PlatformDirs {
	home := os.Getenv("HOME")

	return &PlatformDirs{
		Config: filepath.Join(home, "Library", "Application Support", "openapi-cli"),
		Data:   filepath.Join(home, "Library", "Application Support", "openapi-cli"),
		Cache:  filepath.Join(home, "Library", "Caches", "openapi-cli"),
	}
}

func unixFilename(filename string) string {
	if filename == "" {
		return ""
	}
	return filepath.Base(filepath.FromSlash(filename))
}

func ensureDir(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0700)
	}
	return nil
}
