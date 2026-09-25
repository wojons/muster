package builtin

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/cmdstore"
	"github.com/wojons/muster/pkg/generator"
)

// The cmdstore loader mirrors generator.AuthFlagName because an import
// cycle prevents it from importing pkg/generator. This test pins the two
// values equal — a rename on one side without the other would silently
// break the persistence contract (the loader would register/re-skip the
// wrong flag name).
func TestAuthFlagName_ParityBetweenGeneratorAndCmdstore(t *testing.T) {
	require.Equal(t, generator.AuthFlagName, cmdstore.AuthFlagName)
	require.Equal(t, "auth", generator.AuthFlagName)
}

// The annotation contract between the two packages: the generator stamps
// AuthFlagAnnotation on commands where IT registered the runtime --auth
// flag; persistence (extractCommandMetadata in generate.go) keys the
// FlagKindAuth decision on exactly that annotation + name pair, and the
// loader's skip/route decision keys on the persisted Kind. This test pins
// the constant values shared across the three surfaces.
func TestAuthFlagAnnotation_Constants(t *testing.T) {
	require.Equal(t, "muster/auth-flag", generator.AuthFlagAnnotation)
	require.Equal(t, cmdstore.FlagKindAuth, cmdstore.FlagKind("auth"))
}

// The runtime flag's metadata shape round-trips: persisting a
// FlagKindAuth flag into the store and reading it back preserves the kind
// (the loader's skip decision depends on it surviving serialization).
func TestAuthFlag_PersistKindRoundTrips(t *testing.T) {
	cs := cmdstore.NewCommandStore()
	require.NoError(t, cs.Save(&cmdstore.StoredData{
		Version: "1.0",
		Commands: []cmdstore.CommandMetadata{{
			Name:   "list-tasks",
			Use:    "list-tasks",
			Short:  "List tasks",
			Method: "GET",
			Path:   "/tasks",
			Flags: []cmdstore.FlagMetadata{
				{Name: "auth", Type: "string", Kind: cmdstore.FlagKindAuth},
			},
		}},
	}))

	got, err := cs.Commands()
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].Flags, 1)
	require.Equal(t, cmdstore.FlagKindAuth, got[0].Flags[0].Kind)
}
