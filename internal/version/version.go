// Package version provides build-time version information.
// These variables are set via ldflags at build time.
package version

import "fmt"

// Build-time variables, injected by the build system.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
	Branch    = "unknown"
	GoVersion = "unknown"
)

// Info holds version information.
type Info struct {
	Version   string
	Commit    string
	BuildDate string
	Branch    string
	GoVersion string
}

// Short returns a short version string.
func (i Info) Short() string {
	return fmt.Sprintf("%s (%s)", i.Version, i.Commit)
}

// Get returns the current version info.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		Branch:    Branch,
		GoVersion: GoVersion,
	}
}
