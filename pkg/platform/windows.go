package platform

import (
	"os"
	"path/filepath"
)

func windowsDirs() *PlatformDirs {
	appData := os.Getenv("APPDATA")
	localAppData := os.Getenv("LOCALAPPDATA")

	if appData == "" {
		appData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
	}
	if localAppData == "" {
		localAppData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}

	return &PlatformDirs{
		Config: filepath.Join(appData, "openapi-cli"),
		Data:   filepath.Join(appData, "openapi-cli"),
		Cache:  filepath.Join(localAppData, "openapi-cli", "Cache"),
	}
}

func windowsFilename(filename string) string {
	if filename == "" {
		return ""
	}
	return filepath.Base(filepath.FromSlash(filename))
}
