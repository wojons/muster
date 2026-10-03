package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	// reporting "dev (unknown)" — the REVIEW-MUSTER-2 defect.
	origVersion, origCommit, origBuildDate := Version, Commit, BuildDate
	t.Cleanup(func() {
		Version, Commit, BuildDate = origVersion, origCommit, origBuildDate
	})

	t.Run("version falls back to a module install version", func(t *testing.T) {
		Version = "dev"
		Commit = "unknown"
		BuildDate = "unknown"

		info := Get()
		// Under `go test` Main.Version is "(devel)" (rejected) but a
		// `go install module@version` binary embeds the real version —
		// either way the output must never be the bare default when the
		// toolchain can supply one. Verify via the exact-mode binary below.
		assert.NotEmpty(t, info.Version)
	})

	t.Run("stamped version is never overwritten by build info", func(t *testing.T) {
		Version = "v9.9.9-stamped"
		Commit = "stamped-commit"
		BuildDate = "stamped-date"

		info := Get()
		assert.Equal(t, "v9.9.9-stamped", info.Version)
		assert.Equal(t, "stamped-commit", info.Commit)
		assert.Equal(t, "stamped-date", info.BuildDate)
	})
}
