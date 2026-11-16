package builtin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/config"
)

// TestTenantCLIHelperProcess is NOT a test: it is the child half of the
// separate-process tenant tests below. The parent re-executes this test binary
// with MUSTER_TENANT_CLI_HELPER=1 and the CLI argv after "--", so each
// invocation runs the package's real CLI entry point (config.Load + the root
// command the binary builds) in its own process, with its own view of disk.
func TestTenantCLIHelperProcess(t *testing.T) {
	if os.Getenv(tenantHelperEnvVar) != "1" {
		t.Skip("helper process only; driven by the separate-process tenant tests")
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "helper: load config: %v\n", err)
		os.Exit(2)
	}
	root := NewRootCommandWithAudit(cfg, nil)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.SetArgs(cliArgsAfterSeparator(os.Args))
	if err := root.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

const tenantHelperEnvVar = "MUSTER_TENANT_CLI_HELPER"

// cliArgsAfterSeparator returns the argv tail after the "--" terminator the Go
// test flag parser leaves in place.
func cliArgsAfterSeparator(argv []string) []string {
	for i, arg := range argv {
		if arg == "--" && i+1 < len(argv) {
			return argv[i+1:]
		}
	}
	return nil
}

// runTenantCLIProcess executes one CLI invocation in a separate process whose
// HOME (and every MUSTER_* variable) is pinned to home, then returns its
// stdout, stderr and error.
func runTenantCLIProcess(t *testing.T, home string, args ...string) (string, string, error) {
	t.Helper()

	testArgs := append([]string{"-test.run=^TestTenantCLIHelperProcess$", "--"}, args...)
	cmd := exec.Command(os.Args[0], testArgs...)
	cmd.Env = tenantHelperEnv(home)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// tenantHelperEnv builds the child environment: the parent's env minus every
// MUSTER_* variable and every platform config-dir variable, plus the isolated
// HOME and the helper marker. Stripping MUSTER_* keeps an ambient
// MUSTER_CONFIG / MUSTER_PROFILE from redirecting the child out of home; the
// Go runtime keeps the rest of the environment it was started with.
func tenantHelperEnv(home string) []string {
	drop := map[string]bool{
		"HOME":            true,
		"USERPROFILE":     true,
		"XDG_CONFIG_HOME": true,
		"APPDATA":         true,
		"LOCALAPPDATA":    true,
	}
	env := make([]string, 0, len(os.Environ())+4)
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if drop[name] || strings.HasPrefix(name, "MUSTER_") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env,
		"HOME="+home,
		"USERPROFILE="+home,
		"XDG_CONFIG_HOME=",
		tenantHelperEnvVar+"=1",
	)
	return env
}

// tenantIDFromCurrent reads the "ID:" field out of `tenant current` output.
func tenantIDFromCurrent(output string) string {
	for _, line := range strings.Split(output, "\n") {
		rest, ok := strings.CutPrefix(line, "ID:")
		if ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// TestTenantCommand_SeparateProcessesAgree is the REVIEW-MUSTER-22 regression:
// `tenant set`, `tenant show` and `tenant current` must agree about the
// selection when each runs in its own process.
//
// Pre-fix, `show` read cfg.DefaultProfile (config file, never written by
// `tenant set`) while `current` and `list` read the active marker in
// tenants.json, so a second process answered "No tenant selected (default)."
// from one reader and the tenant from the other.
func TestTenantCommand_SeparateProcessesAgree(t *testing.T) {
	home := t.TempDir()

	stdout, stderr, err := runTenantCLIProcess(t, home, "tenant", "list")
	require.NoError(t, err, "tenant list (fresh) stderr=%s", stderr)
	require.Contains(t, stdout, "No tenants configured.")

	stdout, stderr, err = runTenantCLIProcess(t, home, "tenant", "set", "acme")
	require.NoError(t, err, "tenant set acme stderr=%s", stderr)
	require.Contains(t, stdout, "Set active tenant to acme")

	showOut, stderr, err := runTenantCLIProcess(t, home, "tenant", "show")
	require.NoError(t, err, "tenant show stderr=%s", stderr)

	currentOut, stderr, err := runTenantCLIProcess(t, home, "tenant", "current")
	require.NoError(t, err, "tenant current stderr=%s", stderr)

	currentID := tenantIDFromCurrent(currentOut)
	require.Equal(t, "acme", currentID, "tenant current must name the tenant that was set; output:\n%s", currentOut)
	require.Equal(t, currentID, strings.TrimSpace(showOut),
		"tenant show and tenant current must resolve the same tenant (REVIEW-MUSTER-22); show=%q current:\n%s",
		showOut, currentOut)

	// A second tenant can be registered without a config profile (REVIEW-MUSTER-22
	// item 4) and does not steal the selection.
	stdout, stderr, err = runTenantCLIProcess(t, home, "tenant", "add", "globex", "--tier", "pro")
	require.NoError(t, err, "tenant add globex stderr=%s", stderr)
	require.Contains(t, stdout, "Added tenant globex [pro]")

	showOut, stderr, err = runTenantCLIProcess(t, home, "tenant", "show")
	require.NoError(t, err, "tenant show after add stderr=%s", stderr)
	require.Equal(t, "acme", strings.TrimSpace(showOut), "tenant add must not change the active tenant")

	// Activating the second tenant is visible to every reader in the next process.
	stdout, stderr, err = runTenantCLIProcess(t, home, "tenant", "set", "globex")
	require.NoError(t, err, "tenant set globex stderr=%s", stderr)
	require.Contains(t, stdout, "Set active tenant to globex")

	showOut, stderr, err = runTenantCLIProcess(t, home, "tenant", "show")
	require.NoError(t, err, "tenant show (globex) stderr=%s", stderr)
	currentOut, stderr, err = runTenantCLIProcess(t, home, "tenant", "current")
	require.NoError(t, err, "tenant current (globex) stderr=%s", stderr)
	require.Equal(t, "globex", strings.TrimSpace(showOut))
	require.Equal(t, "globex", tenantIDFromCurrent(currentOut))
	require.Contains(t, currentOut, "[pro]")

	stdout, stderr, err = runTenantCLIProcess(t, home, "tenant", "list")
	require.NoError(t, err, "tenant list stderr=%s", stderr)
	require.Contains(t, stdout, "* globex [pro]")
	require.Contains(t, stdout, "acme [free]")

	// Adding a tenant that already exists is an error, not a silent duplicate.
	_, stderr, err = runTenantCLIProcess(t, home, "tenant", "add", "globex")
	require.Error(t, err, "adding an existing tenant must fail")
	require.Contains(t, stderr, "already exists")
}

// TestTenantHelpDoesNotPromiseUnimplementedGating pins help-text honesty
// (REVIEW-MUSTER-22 item 3): the tenant command used to advertise "isolated
// settings, quotas, and audit records", none of which exist. The help must say
// what the tenant actually is, and must name the supported way to add one.
func TestTenantHelpDoesNotPromiseUnimplementedGating(t *testing.T) {
	cfg := &config.Config{}

	var out bytes.Buffer
	root := NewRootCommandWithAudit(cfg, nil)
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"tenant", "--help"})
	require.NoError(t, root.ExecuteContext(context.Background()))

	help := out.String()
	// The pre-fix sentence was: "Manage the multi-tenant context for this CLI.
	// Each tenant maps to a named config profile with isolated settings,
	// quotas, and audit records." None of that is true of the code.
	for _, claim := range []string{
		"isolated settings",
		"quotas, and audit records",
		"Each tenant maps to a named config profile",
	} {
		require.NotContains(t, help, claim,
			"tenant help must not repeat the pre-fix promise; help was:\n%s", help)
	}
	require.Contains(t, help, "does not gate or partition anything today")
	// The supported way to add a tenant beyond the first must be discoverable.
	require.Regexp(t, `(?m)^\s+add\s+Register a tenant without activating it$`, help,
		"the supported way to add a tenant must be listed; help was:\n%s", help)

	var addOut bytes.Buffer
	addRoot := NewRootCommandWithAudit(cfg, nil)
	addRoot.SetOut(&addOut)
	addRoot.SetErr(&addOut)
	addRoot.SetArgs([]string{"tenant", "add", "--help"})
	require.NoError(t, addRoot.ExecuteContext(context.Background()))

	addHelp := addOut.String()
	require.Contains(t, addHelp, "tenant add <tenant-id>")
	require.Contains(t, addHelp, "--name")
	require.Contains(t, addHelp, "--tier")
}

// TestTenantCommand_SeparateProcessesAgreeOnDefaultProfileFallback pins the one
// documented tie-break: when the tenant store records no active tenant, every
// reader resolves the config file's persisted default_profile — consistently,
// in separate processes.
func TestTenantCommand_SeparateProcessesAgreeOnDefaultProfileFallback(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "muster")
	require.NoError(t, os.MkdirAll(configDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"),
		[]byte("default_profile: beta\n"), 0600))

	showOut, stderr, err := runTenantCLIProcess(t, home, "tenant", "show")
	require.NoError(t, err, "tenant show stderr=%s", stderr)

	currentOut, stderr, err := runTenantCLIProcess(t, home, "tenant", "current")
	require.NoError(t, err, "tenant current stderr=%s", stderr)

	require.Equal(t, "beta", tenantIDFromCurrent(currentOut), "tenant current output:\n%s", currentOut)
	require.Equal(t, "beta", strings.TrimSpace(showOut),
		"show and current must agree on the persisted-default fallback; current:\n%s", currentOut)
}
