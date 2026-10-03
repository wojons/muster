// Package version provides build-time version information.
// These variables are set via ldflags at build time.
package version

import (
	"fmt"
	"runtime/debug"
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
//
// When the binary was built WITHOUT -ldflags (go install, go run, or a bare
// go build) the package-level variables keep their "dev"/"unknown" defaults.
// In that case Get() enriches the info from the build information embedded by
// the Go toolchain: the module version for a module install (go install
// module@version) and the vcs.revision/vcs.time build settings recorded for
// a build inside a git checkout. Values the toolchain cannot supply keep
// their defaults, so the output stays a non-empty, truthful token.
func Get() Info {
	info := Info{
		Version:   normalizeStamp(Version, DefaultVersion),
		Commit:    normalizeStamp(Commit, DefaultStamp),
		BuildDate: normalizeStamp(BuildDate, DefaultStamp),
		Branch:    normalizeStamp(Branch, DefaultStamp),
		GoVersion: normalizeStamp(GoVersion, DefaultStamp),
	}
	applyBuildInfoFallback(&info)
	return info
}

// applyBuildInfoFallback fills unstamped fields from the embedded build
// information, if any. A field already carrying a real (non-default) value
// is never overwritten: ldflags stamps remain authoritative.
func applyBuildInfoFallback(info *Info) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if info.GoVersion == DefaultStamp && bi.GoVersion != "" {
		info.GoVersion = bi.GoVersion
	}
	if info.Version == DefaultVersion {
		if v := moduleVersion(bi); v != "" {
			info.Version = v
		}
	}
	if info.Commit == DefaultStamp || info.BuildDate == DefaultStamp {
		revision, stamp := vcsInfo(bi)
		if info.Commit == DefaultStamp && revision != "" {
			info.Commit = revision
		}
		if info.BuildDate == DefaultStamp && stamp != "" {
			info.BuildDate = stamp
		}
	}
}

// moduleVersion returns the embedded main-module version when it is a real
// module version. "(devel)" (go run / bare go build) and empty values are
// rejected: those builds carry their provenance in the vcs build settings
// instead, which vcsInfo reads.
func moduleVersion(bi *debug.BuildInfo) string {
	v := strings.TrimSpace(bi.Main.Version)
	if v == "" || v == "(devel)" {
		return ""
	}
	return v
}

// vcsInfo returns the vcs revision (shortened, like git's default abbrev)
// and the vcs commit time recorded by the toolchain for a build inside a
// git checkout.
func vcsInfo(bi *debug.BuildInfo) (revision, stamp string) {
	var fullRevision string
	var dirty bool
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			fullRevision = s.Value
		case "vcs.time":
			stamp = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if fullRevision == "" {
		return "", stamp
	}
	revision = fullRevision
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if dirty {
		revision += "-dirty"
	}
	return revision, stamp
}

// normalizeStamp returns fallback when stamp is empty or whitespace-only;
// otherwise it returns stamp trimmed of surrounding whitespace.
func normalizeStamp(stamp, fallback string) string {
	if strings.TrimSpace(stamp) == "" {
		return fallback
	}
	return stamp
}
