package version

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubBuildInfo replaces the build-info reader for the duration of the test
// so Get()'s fallback can be driven with synthetic metadata. The ambient
// BuildInfo under `go test` carries no vcs settings (Main.Version is
// "(devel)"), so any test that asserts bare-build defaults must pin it.
func stubBuildInfo(t *testing.T, bi *debug.BuildInfo, ok bool) {
	t.Helper()
	orig := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) { return bi, ok }
	t.Cleanup(func() { readBuildInfo = orig })
}

// resetStamps restores the package-level build stamps to their unstamped
// defaults for the duration of the test, so Get() observes a bare build.
func resetStamps(t *testing.T) {
	t.Helper()
	origVersion, origCommit, origBuildDate := Version, Commit, BuildDate
	origBranch, origGoVersion := Branch, GoVersion
	Version, Commit, BuildDate = DefaultVersion, DefaultStamp, DefaultStamp
	Branch, GoVersion = DefaultStamp, DefaultStamp
	t.Cleanup(func() {
		Version, Commit, BuildDate = origVersion, origCommit, origBuildDate
		Branch, GoVersion = origBranch, origGoVersion
	})
}

// defaultInfo mirrors what Get() normalizes an unstamped build to before the
// build-info fallback runs.
func defaultInfo() Info {
	return Info{
		Version:   DefaultVersion,
		Commit:    DefaultStamp,
		BuildDate: DefaultStamp,
		Branch:    DefaultStamp,
		GoVersion: DefaultStamp,
	}
}

func TestInfo_Short(t *testing.T) {
	tests := []struct {
		name string
		info Info
		want string
	}{
		{
			name: "standard version",
			info: Info{Version: "1.0.0", Commit: "abc123"},
			want: "1.0.0 (abc123)",
		},
		{
			name: "dev version",
			info: Info{Version: "dev", Commit: "unknown"},
			want: "dev (unknown)",
		},
		{
			name: "empty version",
			info: Info{Version: "", Commit: ""},
			want: " ()",
		},
		{
			name: "long commit hash",
			info: Info{Version: "2.3.1", Commit: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0"},
			want: "2.3.1 (a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.Short()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGet(t *testing.T) {
	info := Get()

	require.NotNil(t, info)

	// Verify all fields are set (package-level vars have defaults; the
	// build-info fallback may enrich unstamped fields from the embedded
	// build information, but every field must stay a non-empty token).
	assert.NotEmpty(t, info.Version)
	assert.NotEmpty(t, info.Commit)
	assert.NotEmpty(t, info.BuildDate)
	assert.NotEmpty(t, info.Branch)
	assert.NotEmpty(t, info.GoVersion)
}

func TestGet_NormalizesEmptyBuildStamps(t *testing.T) {
	// A release pipeline that injects -ldflags with an EMPTY value (for
	// example a container build invoked without its VERSION build arg) must
	// never yield a binary whose `version` output is empty. Get() normalizes
	// empty/whitespace stamps back to the documented defaults so the version
	// surface is always a non-empty token: a stamped value, or "dev"/
	// "unknown" for a bare build.
	//
	// The build-info reader is stubbed out here so the assertions describe
	// the normalization alone; the bare-checkout derivation is covered by
	// TestGet_BareBuildReportsVcsRevision.
	stubBuildInfo(t, &debug.BuildInfo{}, false)

	origVersion, origCommit, origBuildDate := Version, Commit, BuildDate
	t.Cleanup(func() {
		Version, Commit, BuildDate = origVersion, origCommit, origBuildDate
	})

	t.Run("empty Version falls back to dev, other stamps preserved", func(t *testing.T) {
		Version = ""
		Commit = "436cad0b"
		BuildDate = "2026-09-28T20:36:31Z"

		info := Get()
		assert.Equal(t, "dev", info.Version)
		assert.Equal(t, "436cad0b", info.Commit)
		assert.Equal(t, "2026-09-28T20:36:31Z", info.BuildDate)
	})

	t.Run("empty Commit and BuildDate fall back to unknown-or-buildinfo", func(t *testing.T) {
		Version = "v1.2.3"
		Commit = ""
		BuildDate = ""

		info := Get()
		assert.Equal(t, "v1.2.3", info.Version)
		// Unstamped fields may be enriched from the embedded build
		// information; they must always stay non-empty, never blank.
		assert.NotEmpty(t, info.Commit)
		assert.NotEmpty(t, info.BuildDate)
	})

	t.Run("whitespace-only stamps fall back too", func(t *testing.T) {
		Version = "   "
		Commit = " "

		info := Get()
		assert.Equal(t, "dev", info.Version)
		assert.NotEmpty(t, info.Commit)
	})

	t.Run("legitimate stamps pass through untouched", func(t *testing.T) {
		Version = "v0.1.1-272-g436cad0b"
		Commit = "436cad0b"
		BuildDate = "2026-09-28T20:36:31Z"

		info := Get()
		assert.Equal(t, "v0.1.1-272-g436cad0b", info.Version)
		assert.Equal(t, "436cad0b", info.Commit)
		assert.Equal(t, "2026-09-28T20:36:31Z", info.BuildDate)
	})
}

func TestInfo_Short_AllFieldsPopulated(t *testing.T) {
	info := Info{
		Version:   "3.0.0",
		Commit:    "deadbeef",
		BuildDate: "2024-01-15",
		Branch:    "main",
		GoVersion: "go1.23.0",
	}

	got := info.Short()
	assert.Equal(t, "3.0.0 (deadbeef)", got)

	// Verify other fields are preserved but not in Short()
	assert.Equal(t, "2024-01-15", info.BuildDate)
	assert.Equal(t, "main", info.Branch)
	assert.Equal(t, "go1.23.0", info.GoVersion)
}

func TestGet_ReturnsPointerLikeStruct(t *testing.T) {
	info1 := Get()
	info2 := Get()

	// Both calls should return identical data (same package-level vars)
	assert.Equal(t, info1, info2)
}

func TestInfo_Short_DoesNotMutate(t *testing.T) {
	info := Info{Version: "1.0.0", Commit: "abc123"}
	before := info

	info.Short()

	// Short() should not modify the struct
	assert.Equal(t, before, info)
}

func TestGet_BuildInfoFallback(t *testing.T) {
	// A build WITHOUT -ldflags (go install module@version, go run, bare go
	// build) leaves the package vars at their defaults. Get() must enrich
	// them from the toolchain-embedded build information instead of
	// reporting "dev (unknown)" — the REVIEW-MUSTER-2 / REVIEW2-MUSTER-3
	// defect.
	origVersion, origCommit, origBuildDate := Version, Commit, BuildDate
	t.Cleanup(func() {
		Version, Commit, BuildDate = origVersion, origCommit, origBuildDate
	})

	t.Run("stamped version is never overwritten by build info", func(t *testing.T) {
		Version = "v9.9.9-stamped"
		Commit = "stamped-commit"
		BuildDate = "stamped-date"
		stubBuildInfo(t, &debug.BuildInfo{
			GoVersion: "go1.26.6",
			Main:      debug.Module{Version: "v0.1.2"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "819ab9000142c0ffee5a1b2c3d4e5f60718293a4"},
				{Key: "vcs.time", Value: "2026-10-13T12:00:00Z"},
			},
		}, true)

		info := Get()
		assert.Equal(t, "v9.9.9-stamped", info.Version)
		assert.Equal(t, "stamped-commit", info.Commit)
		assert.Equal(t, "stamped-date", info.BuildDate)
	})
}

// bareBuildInfo is the build metadata the Go toolchain embeds for a bare
// `go build` inside a git checkout: an abbreviated-revision vcs setting, the
// commit time and a "(devel)" main-module version.
func bareBuildInfo(dirty bool) *debug.BuildInfo {
	modified := "false"
	if dirty {
		modified = "true"
	}
	return &debug.BuildInfo{
		GoVersion: "go1.26.6",
		Main:      debug.Module{Path: "github.com/wojons/muster", Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "-buildmode", Value: "exe"},
			{Key: "vcs", Value: "git"},
			{Key: "vcs.revision", Value: "819ab9000142c0ffee5a1b2c3d4e5f60718293a4"},
			{Key: "vcs.time", Value: "2026-10-13T12:00:00Z"},
			{Key: "vcs.modified", Value: modified},
		},
	}
}

func TestApplyBuildInfo_BareCheckoutReportsDevPlusVcsRevision(t *testing.T) {
	info := defaultInfo()
	applyBuildInfo(&info, bareBuildInfo(true))

	assert.Equal(t, "dev-819ab9000142", info.Version,
		"a bare build must report dev-<short revision>, never plain dev")
	assert.Equal(t, "819ab9000142-dirty", info.Commit)
	assert.Equal(t, "2026-10-13T12:00:00Z", info.BuildDate)
	assert.Equal(t, "go1.26.6", info.GoVersion)
	assert.Equal(t, DefaultStamp, info.Branch,
		"the toolchain records no branch setting, so it keeps its default")
	assert.NotEqual(t, DefaultVersion, info.Version)
}

func TestApplyBuildInfo_CleanCheckoutHasNoDirtySuffix(t *testing.T) {
	info := defaultInfo()
	applyBuildInfo(&info, bareBuildInfo(false))

	assert.Equal(t, "dev-819ab9000142", info.Version)
	assert.Equal(t, "819ab9000142", info.Commit)
	assert.NotContains(t, info.Commit, dirtySuffix)
}

func TestApplyBuildInfo_ModuleInstallVersionBeatsDevFallback(t *testing.T) {
	// `go install github.com/wojons/muster/cmd/mcp-server@v0.1.2` embeds the
	// real module version and no vcs settings.
	info := defaultInfo()
	applyBuildInfo(&info, &debug.BuildInfo{
		GoVersion: "go1.26.6",
		Main:      debug.Module{Path: "github.com/wojons/muster/cmd/mcp-server", Version: "v0.1.2"},
	})

	assert.Equal(t, "v0.1.2", info.Version)
	assert.Equal(t, DefaultStamp, info.Commit)
	assert.Equal(t, "go1.26.6", info.GoVersion)
}

func TestApplyBuildInfo_NoMetadataKeepsDocumentedDefaults(t *testing.T) {
	// The only build that still reports "dev (unknown)": one whose binary
	// carries no toolchain metadata at all (built outside a git checkout, or
	// with -buildvcs=false).
	tests := []struct {
		name string
		bi   *debug.BuildInfo
	}{
		{name: "nil build info", bi: nil},
		{name: "empty build info", bi: &debug.BuildInfo{}},
		{name: "devel version without vcs settings", bi: &debug.BuildInfo{
			Main: debug.Module{Path: "github.com/wojons/muster", Version: "(devel)"},
		}},
		{name: "vcs settings without a revision", bi: &debug.BuildInfo{
			Main:     debug.Module{Version: "(devel)"},
			Settings: []debug.BuildSetting{{Key: "vcs.time", Value: "2026-10-13T12:00:00Z"}},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := defaultInfo()
			applyBuildInfo(&info, tt.bi)

			assert.Equal(t, DefaultVersion, info.Version)
			assert.Equal(t, DefaultStamp, info.Commit)
			// vcs.time alone may still fill the build date; nothing else.
			assert.Equal(t, DefaultStamp, info.Branch)
		})
	}
}

func TestApplyBuildInfo_StampedVersionStillGetsCommitFromBuildInfo(t *testing.T) {
	// AC-3 of REVIEW2-MUSTER-3: `-ldflags -X ...Version=v9.9.9-test` on a
	// git checkout must print the stamped version while still acquiring the
	// revision for the commit half.
	info := defaultInfo()
	info.Version = "v9.9.9-test"

	applyBuildInfo(&info, bareBuildInfo(false))

	assert.Equal(t, "v9.9.9-test", info.Version)
	assert.Equal(t, "819ab9000142", info.Commit)
	assert.Equal(t, "v9.9.9-test (819ab9000142)", info.Short())
}

func TestApplyBuildInfo_StampedCommitIsNotOverwritten(t *testing.T) {
	info := defaultInfo()
	info.Commit = "stamped-commit"

	applyBuildInfo(&info, bareBuildInfo(false))

	assert.Equal(t, "stamped-commit", info.Commit)
	assert.Equal(t, "dev-819ab9000142", info.Version)
}

func TestApplyBuildInfo_LongRevisionIsAbbreviated(t *testing.T) {
	info := defaultInfo()
	applyBuildInfo(&info, &debug.BuildInfo{
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
		},
	})

	assert.Equal(t, "dev-0123456789ab", info.Version)
	assert.Equal(t, "0123456789ab", info.Commit)
	assert.Len(t, info.Commit, vcsRevisionLength)
}

func TestApplyBuildInfo_ShortRevisionIsKeptWhole(t *testing.T) {
	info := defaultInfo()
	applyBuildInfo(&info, &debug.BuildInfo{
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}},
	})

	assert.Equal(t, "dev-abc123", info.Version)
	assert.Equal(t, "abc123", info.Commit)
}

func TestGet_BareBuildReportsVcsRevision(t *testing.T) {
	// End-to-end through Get(): an unstamped build (the package defaults)
	// with a git checkout's metadata must not report "dev (unknown)".
	resetStamps(t)
	stubBuildInfo(t, bareBuildInfo(true), true)

	info := Get()

	assert.Equal(t, "dev-819ab9000142", info.Version)
	assert.Equal(t, "819ab9000142-dirty", info.Commit)
	assert.Equal(t, "dev-819ab9000142 (819ab9000142-dirty)", info.Short())
	assert.NotEqual(t, "dev (unknown)", info.Short())
	assert.Contains(t, info.Short(), "819ab9000142")
}

func TestGet_NoBuildInfoKeepsDevUnknown(t *testing.T) {
	// Documents the one remaining "dev (unknown)" case: a binary carrying no
	// toolchain metadata at all.
	resetStamps(t)
	stubBuildInfo(t, nil, false)

	info := Get()

	assert.Equal(t, DefaultVersion, info.Version)
	assert.Equal(t, DefaultStamp, info.Commit)
	assert.Equal(t, "dev (unknown)", info.Short())
}

func TestGet_StampedVersionWinsOverBuildInfo(t *testing.T) {
	// The ldflags path: an explicit -X stamp must survive the fallback.
	resetStamps(t)
	Version = "v9.9.9-test"
	stubBuildInfo(t, bareBuildInfo(true), true)

	info := Get()

	assert.Equal(t, "v9.9.9-test", info.Version)
	assert.Equal(t, "v9.9.9-test (819ab9000142-dirty)", info.Short())
}

func TestVcsInfo_ReadsRevisionTimeAndModified(t *testing.T) {
	tests := []struct {
		name         string
		settings     []debug.BuildSetting
		wantRevision string
		wantShort    string
		wantTime     string
	}{
		{
			name: "clean checkout",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "0123456789abcdef"},
				{Key: "vcs.time", Value: "2026-10-13T12:00:00Z"},
				{Key: "vcs.modified", Value: "false"},
			},
			wantRevision: "0123456789ab",
			wantShort:    "0123456789ab",
			wantTime:     "2026-10-13T12:00:00Z",
		},
		{
			name: "modified checkout",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "0123456789ab"},
				{Key: "vcs.modified", Value: "true"},
			},
			wantRevision: "0123456789ab" + dirtySuffix,
			wantShort:    "0123456789ab",
		},
		{
			name: "no vcs settings",
			settings: []debug.BuildSetting{
				{Key: "-buildmode", Value: "exe"},
			},
		},
		{
			name: "time without revision",
			settings: []debug.BuildSetting{
				{Key: "vcs.time", Value: "2026-10-13T12:00:00Z"},
			},
			wantTime: "2026-10-13T12:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vcsInfo(&debug.BuildInfo{Settings: tt.settings})

			assert.Equal(t, tt.wantRevision, got.revision)
			assert.Equal(t, tt.wantShort, got.short)
			assert.Equal(t, tt.wantTime, got.time)
		})
	}
}
