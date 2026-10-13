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

// readBuildInfo reads the build metadata the Go toolchain embeds into the
// binary. It is a variable so tests can drive Get()'s fallback with a
// synthetic BuildInfo: the ambient one available under `go test` carries no
// vcs settings (Main.Version is "(devel)"), so it cannot exercise the
// git-checkout path.
var readBuildInfo = debug.ReadBuildInfo

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

	// devVersionPrefix marks a version a bare build derives from its vcs
	// revision (REVIEW2-MUSTER-3). Such a binary is not a release, so it
	// must not claim a release number, but "dev-<revision>" keeps it
	// traceable to the exact source commit. It intentionally reuses the
	// DefaultVersion word so every consumer's "is this a dev build" check
	// keeps working.
	devVersionPrefix = DefaultVersion + "-"

	// dirtySuffix marks a build produced from a checkout with uncommitted
	// changes, matching `git describe --dirty`.
	dirtySuffix = "-dirty"

	// vcsRevisionLength is the abbreviation applied to the full vcs
	// revision recorded by the toolchain, matching git's default abbrev
	// and the Makefile's `git rev-parse --short HEAD` stamp.
	vcsRevisionLength = 12
)

// Get returns the current version info.
//
// Build stamps that arrive empty or whitespace-only (for example a release
// pipeline whose -ldflags -X values were left unset) are normalized back to
// the bare-build defaults so the version surface can never report an empty
// Version or Commit. Legitimate stamped values pass through untouched.
//
// When the binary was built WITHOUT -ldflags (go install, go run, or a bare
// go build), the package-level variables keep their "dev"/"unknown"
// defaults. Get() then enriches the info from the build information embedded
// by the Go toolchain: the module version for a module install
// (go install module@version) and the vcs.revision/vcs.time build settings
// recorded for a build inside a git checkout — a bare `go build` of this
// repository therefore reports "dev-<short revision>", not "dev (unknown)".
// Values the toolchain cannot supply keep their defaults, so the output
// stays a non-empty, truthful token.
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
// information, if any.
func applyBuildInfoFallback(info *Info) {
	bi, ok := readBuildInfo()
	if !ok || bi == nil {
		return
	}
	applyBuildInfo(info, bi)
}

// applyBuildInfo fills unstamped fields from bi. A field already carrying a
// real (non-default) value is never overwritten: an -ldflags stamp is
// authoritative and always wins over the toolchain-embedded metadata.
//
// Version resolution order for a field left at its default:
//  1. the main module version, for `go install module@version` (and any
//     build whose main module version is a real version);
//  2. "dev-<short revision>", for a bare `go build` inside a git checkout —
//     the toolchain records the vcs revision as a build setting;
//  3. the "dev" default, when the toolchain recorded no metadata at all
//     (no git checkout, -buildvcs=false), which reports "dev (unknown)" as
//     documented.
func applyBuildInfo(info *Info, bi *debug.BuildInfo) {
	if bi == nil {
		return
	}
	if info.GoVersion == DefaultStamp && bi.GoVersion != "" {
		info.GoVersion = bi.GoVersion
	}

	vcs := vcsInfo(bi)

	if info.Version == DefaultVersion {
		if mv := moduleVersion(bi); mv != "" {
			info.Version = mv
		} else if vcs.short != "" {
			info.Version = devVersionPrefix + vcs.short
		}
	}
	if info.Commit == DefaultStamp && vcs.revision != "" {
		info.Commit = vcs.revision
	}
	if info.BuildDate == DefaultStamp && vcs.time != "" {
		info.BuildDate = vcs.time
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

// vcsMetadata is the vcs provenance the Go toolchain records for a build
// inside a git checkout.
type vcsMetadata struct {
	// revision is the abbreviated vcs revision, with dirtySuffix appended
	// when the checkout had uncommitted changes.
	revision string
	// short is the bare abbreviated revision, without dirtySuffix.
	short string
	// time is the vcs commit time (RFC3339).
	time string
}

// vcsInfo returns the vcs provenance recorded by the toolchain for a build
// inside a git checkout: the abbreviated revision (marked dirty when the
// tree had uncommitted changes) and the vcs commit time. Both are empty for
// a build without vcs metadata.
func vcsInfo(bi *debug.BuildInfo) vcsMetadata {
	var full string
	var meta vcsMetadata
	var dirty bool
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			full = s.Value
		case "vcs.time":
			meta.time = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if full == "" {
		return vcsMetadata{time: meta.time}
	}
	meta.short = full
	if len(meta.short) > vcsRevisionLength {
		meta.short = meta.short[:vcsRevisionLength]
	}
	meta.revision = meta.short
	if dirty {
		meta.revision += dirtySuffix
	}
	return meta
}

// normalizeStamp returns fallback when stamp is empty or whitespace-only;
// otherwise it returns stamp trimmed of surrounding whitespace.
func normalizeStamp(stamp, fallback string) string {
	if strings.TrimSpace(stamp) == "" {
		return fallback
	}
	return stamp
}
