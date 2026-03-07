package platform

import (
	"runtime"
)

type PlatformDirs struct {
	Config string
	Data   string
	Cache  string
	State  string
}

func Directories() *PlatformDirs {
	switch runtime.GOOS {
	case "darwin":
		return macOSDirs()
	case "linux":
		return linuxDirs()
	case "windows":
		return windowsDirs()
	default:
		return &PlatformDirs{
			Config: ".",
			Data:   ".",
			Cache:  ".",
		}
	}
}
