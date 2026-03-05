package platform

import (
	"os"
	"path/filepath"
)

func linuxDirs() *PlatformDirs {
	home := os.Getenv("HOME")
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	xdgData := os.Getenv("XDG_DATA_HOME")
	xdgCache := os.Getenv("XDG_CACHE_HOME")

	if xdgConfig == "" {
		xdgConfig = filepath.Join(home, ".config")
	}
	if xdgData == "" {
		xdgData = filepath.Join(home, ".local", "share")
	}
	if xdgCache == "" {
		xdgCache = filepath.Join(home, ".cache")
	}

	return &PlatformDirs{
		Config: xdgConfig,
		Data:   xdgData,
		Cache:  xdgCache,
	}
}
