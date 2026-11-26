package builtin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/cmdstore"
	"github.com/wojons/muster/pkg/config"
)

// df061StoreCommands is the known store the discovery guard uses: command
// names shaped exactly like real generated ones (grouped "resource/operation"
// flat names, including the long GitHub-style names users must be able to
// discover instead of guessing).
var df061StoreCommands = []cmdstore.CommandMetadata{
	{
		Name:   "repos/get",
		Use:    "repos/get <owner> <repo>",
		Short:  "Get a repository",
		Method: "GET",
		Path:   "/repos/{owner}/{repo}",
		Args:   []string{"owner", "repo"},
	},
	{
		Name:   "agents/add-selected-repo-to-org-secret",
		Use:    "agents/add-selected-repo-to-org-secret <org> <secret_name>",
		Short:  "Add selected repository to an organization secret",
		Method: "PUT",
		Path:   "/orgs/{org}/actions/secrets/{secret_name}",
		Args:   []string{"org", "secret_name"},
	},
	{
		Name:   "issues/create-milestone",
		Use:    "issues/create-milestone <owner> <repo>",
		Short:  "Create a milestone",
		Method: "POST",
		Path:   "/repos/{owner}/{repo}/milestones",
		Args:   []string{"owner", "repo"},
	},
}

func writeDF061Store(t *testing.T) (storePath string, sd *cmdstore.StoredData) {
	t.Helper()
	sd = &cmdstore.StoredData{
		Version:     "1.0",
		SpecPath:    "testdata/github-spec.json",
		SpecHash:    "df061guard",
		BaseURL:     "http://127.0.0.1:1",
		GeneratedAt: time.Now(),
		Commands:    df061StoreCommands,
	}
	data, err := json.Marshal(sd)
	require.NoError(t, err)
	storePath = filepath.Join(t.TempDir(), "commands.json")
	require.NoError(t, os.WriteFile(storePath, data, 0o600))
	return storePath, sd
}

// AC2: `generate --list` must enumerate persisted commands, grouped by
// method/path — every persisted name must appear.
func TestGenerateList_EnumeratesPersistedCommands_DF061(t *testing.T) {
	storePath, sd := writeDF061Store(t)
	cfg := &config.Config{PersistedCommandsPath: storePath}

	listing := renderCommandListing(sd)
	require.Contains(t, listing, "Commands (3):")
	for _, c := range sd.Commands {
		assert.Contains(t, listing, c.Name, "listing must name %q", c.Name)
		assert.Contains(t, listing, c.Path, "listing must show %q path", c.Path)
	}
	// Grouped by method, in canonical order (discover.go methodOrder:
	// GET, POST, PUT, ...).
	getIdx := strings.Index(listing, "GET:")
	postIdx := strings.Index(listing, "POST:")
	putIdx := strings.Index(listing, "PUT:")
	require.GreaterOrEqual(t, getIdx, 0)
	require.Less(t, getIdx, postIdx, "GET group must precede POST group")
	require.Less(t, postIdx, putIdx, "POST group must precede PUT group")

	// The --list flag path prints the same enumeration.
	cmd := newGenerateCommand(cfg)
	require.NoError(t, cmd.Flags().Set("list", "true"))
	out, err := captureStdout(func() error { return cmd.RunE(cmd, nil) })
	require.NoError(t, err)
	for _, c := range sd.Commands {
		assert.Contains(t, out, c.Name, "generate --list output must name %q", c.Name)
	}
	assert.Contains(t, out, "Command count: 3")
}

// AC1: the generate success hint must name the real discovery path — total
// count, a preview of registered names, and a listing command that actually
// enumerates the persisted store (never the old bare --help pointer).
func TestGenerateHint_NamesDiscoveryPath_DF061(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	specPath := filepath.Join(t.TempDir(), "spec.yaml")
	specContent := []byte(`openapi: 3.0.0
info:
  title: Hint API
  version: 1.0.0
paths:
  /widgets:
    get:
      operationId: get-widgets
      responses:
        '200':
          description: OK
  /gadgets:
    post:
      operationId: create-gadget
      responses:
        '201':
          description: Created
`)
	require.NoError(t, os.WriteFile(specPath, specContent, 0o644))

	cfg := &config.Config{}
	root := NewRootCommand(cfg)
	generate, _, err := root.Find([]string{"generate"})
	require.NoError(t, err)

	out, err := captureStdout(func() error { return generate.RunE(generate, []string{specPath}) })
	require.NoError(t, err)

	assert.NotContains(t, out, "Run 'openapi-cli --help' to see available commands",
		"the dead --help hint must be gone")
	assert.Contains(t, out, "Registered 2 generated commands:")
	assert.Contains(t, out, "  - get-widgets")
	assert.Contains(t, out, "  - create-gadget")
	assert.Contains(t, out, "Run 'openapi-cli generate --list' to enumerate every persisted command")
}

// AC3 + AC5 (guard): every persisted command name must appear in the shell
// completion surface. Bash embeds names statically in the generated script;
// cobra's zsh script is runtime-dynamic and delegates to `__complete`, so
// the zsh guard asserts the delegation is present in the script AND drives
// that exact engine, asserting every persisted name in its output. If the
// store load path or visibility gating ever regresses, the names go missing
// here.
func TestCompletion_IncludesPersistedCommands_DF061(t *testing.T) {
	storePath, sd := writeDF061Store(t)
	cfg := &config.Config{PersistedCommandsPath: storePath}

	t.Run("bash script embeds every persisted name", func(t *testing.T) {
		var script strings.Builder
		root := NewRootCommand(cfg)
		require.NotNil(t, root)
		require.NoError(t, root.GenBashCompletion(&script))
		require.NotEmpty(t, script.String())
		for _, c := range sd.Commands {
			assert.Contains(t, script.String(), c.Name,
				"bash completion script must include %q", c.Name)
		}
	})

	t.Run("zsh delegates to __complete which surfaces every persisted name", func(t *testing.T) {
		var script strings.Builder
		root := NewRootCommand(cfg)
		require.NotNil(t, root)
		require.NoError(t, root.GenZshCompletion(&script))
		require.NotEmpty(t, script.String())
		assert.Contains(t, script.String(), "__complete",
			"zsh script must delegate to the __complete runtime engine")

		// Drive the exact engine the zsh script calls at runtime.
		// os.Args is read by cobra's Execute() as the fallback argv, so it
		// MUST be restored or later tests executing commands directly
		// inherit this argv (QA-MUSTER036 ordering pollution).
		oldArgs := os.Args
		t.Cleanup(func() { os.Args = oldArgs })
		os.Args = []string{"openapi-cli", "__complete", ""}
		completer := NewRootCommand(cfg)
		require.NotNil(t, completer)
		out, runErr := captureStdout(func() error {
			completer.SetArgs([]string{"__complete", ""})
			return completer.Execute()
		})
		require.NoError(t, runErr)
		for _, c := range sd.Commands {
			assert.Contains(t, out, c.Name, "__complete output must include %q", c.Name)
		}
	})
}
