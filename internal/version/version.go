// Package version provides build-time version information.
// These variables are set via ldflags at build time.
package version

import (
	"fmt"
	"strings"
)

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

// Stamp defaults for builds that do not inject version metadata via
// -ldflags (a bare `go build`). These are the documented values a
// development build reports.
const (
	DefaultVersion = "dev"
	DefaultStamp   = "unknown"
)

// Get returns the current version info.
//
// Build stamps that arrive empty or whitespace-only (for example a release
// pipeline whose -ldflags -X values were left unset) are normalized back to
// the bare-build defaults so the version surface can never report an empty
// Version or Commit. Legitimate stamped values pass through untouched.
func Get() Info {
	return Info{
		Version:   normalizeStamp(Version, DefaultVersion),
		Commit:    normalizeStamp(Commit, DefaultStamp),
		BuildDate: normalizeStamp(BuildDate, DefaultStamp),
		Branch:    normalizeStamp(Branch, DefaultStamp),
		GoVersion: normalizeStamp(GoVersion, DefaultStamp),
	}
}

// normalizeStamp returns fallback when stamp is empty or whitespace-only;
// otherwise it returns stamp trimmed of surrounding whitespace.
func normalizeStamp(stamp, fallback string) string {
	if strings.TrimSpace(stamp) == "" {
		return fallback
	}
	return stamp
}
