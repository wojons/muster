package version

import (
	"fmt"
	"runtime"
)

// Build-time variables (set via ldflags)
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Info contains version information
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// Get returns current version info
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns formatted version string
func (i Info) String() string {
	return fmt.Sprintf("openapi-cli version %s (commit: %s, built: %s)\n%s/%s %s",
		i.Version, i.Commit, i.BuildDate, runtime.GOOS, runtime.GOARCH, runtime.Version())
}

// Short returns a short version string
func (i Info) Short() string {
	commit := i.Commit
	if len(commit) > 7 {
		commit = commit[:7]
	}
	return fmt.Sprintf("%s (%s)", i.Version, commit)
}
