package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInfo_Short(t *testing.T) {
	tests := []struct {
		name    string
		info    Info
		want    string
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

	// Verify all fields are set (package-level vars have defaults)
	assert.Equal(t, "dev", info.Version)
	assert.Equal(t, "unknown", info.Commit)
	assert.Equal(t, "unknown", info.BuildDate)
	assert.Equal(t, "unknown", info.Branch)
	assert.Equal(t, "unknown", info.GoVersion)
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
