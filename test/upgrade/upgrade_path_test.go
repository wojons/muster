// Package main holds the QA-MUSTER-4 upgrade-path verification tests.
//
// The QA battery's tar-sync strips .git, so its agent-side upgrade cell sees
// an empty tag list and prints `upgrade N/A "no release tags in repo"` even
// though origin carries v0.1.0 and v0.1.1. scripts/verify-upgrade-path.sh is
// the local, .git-present verification path; these tests keep the script
// honest: bash syntax, the documented exit-code contract (2 = nothing to
// upgrade, never a pass), and the per-tag module-path stamping rule that
// historical releases (v0.1.0's Makefile still targets the pre-rename
// module path) would otherwise silently violate.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const upgradeScript = "../../scripts/verify-upgrade-path.sh"

// runUpgradeScript runs verify-upgrade-path.sh with explicit tag arguments
// (cwd = the test's own package dir, test/upgrade) and returns the exit code
// plus combined output.
func runUpgradeScript(prevTag, headRef string, env ...string) (int, string) {
	args := []string{"bash", upgradeScript}
	if prevTag != "" || headRef != "" {
		args = append(args, prevTag, headRef)
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), env...)
	out, _ := cmd.CombinedOutput() // exit code IS the result; nonzero is data
	return exitCodeOf(cmd), string(out)
}

func exitCodeOf(cmd *exec.Cmd) int {
	if cmd.ProcessState == nil {
		return -1
	}
	return cmd.ProcessState.ExitCode()
}

// TestUpgradeScriptSyntax pins the script's bash syntax so a broken edit
// cannot reach the real verification run.
func TestUpgradeScriptSyntax(t *testing.T) {
	cmd := exec.Command("bash", "-n", upgradeScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify-upgrade-path.sh fails bash -n: %v: %s", err, out)
	}
}

// TestUpgradeScriptDocumentsSkipContract pins the documented skip verdicts:
// a repo with fewer than two tags exits 2 with the honest "not counted as a
// pass" wording — the same anti-silent-N/A rule the QA-MUSTER-4 row asks for.
func TestUpgradeScriptDocumentsSkipContract(t *testing.T) {
	data, err := os.ReadFile(upgradeScript)
	if err != nil {
		t.Fatalf("read %s: %v", upgradeScript, err)
	}
	s := string(data)
	for _, want := range []string{
		"Not counted as a pass",
		"exit 2",
		`"no release tags`, // the tar-sync N/A wording this change exists to kill
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script must carry the skip contract %q", want)
		}
	}
}

// TestUpgradeScriptStampsEachTagsOwnModulePath guards the v0.1.0 lesson: the
// script must read `module` from the PREVIOUS tag's go.mod (its Makefile
// still stamped the pre-rename github.com/openapi-cli/openapi-cli path) and
// target internal/version, or the stamp silently misses and reports dev.
func TestUpgradeScriptStampsEachTagsOwnModulePath(t *testing.T) {
	data, err := os.ReadFile(upgradeScript)
	if err != nil {
		t.Fatalf("read %s: %v", upgradeScript, err)
	}
	s := string(data)
	if !strings.Contains(s, "prev/go.mod") || !strings.Contains(s, "\"module\"") {
		t.Error("script must derive the module path from the checked-out tag's go.mod, not a hardcoded constant")
	}
	if !strings.Contains(s, "internal/version") {
		t.Error("script must stamp internal/version (the package the version command reads)")
	}
}

// TestUpgradeScriptRealUpgrade exercises the genuine tagged-upgrade path:
// v0.1.0 → v0.1.1 must exit 0 and report the upgraded version string. This
// is the QA-MUSTER-4 acceptance evidence — it runs the script end to end.
func TestUpgradeScriptRealUpgrade(t *testing.T) {
	if testing.Short() {
		t.Skip("real upgrade builds two binaries; skipped in -short suites — run explicitly")
	}
	rc, out := runUpgradeScript("v0.1.0", "v0.1.1")
	if rc != 0 {
		t.Fatalf("v0.1.0 → v0.1.1 upgrade verification failed (rc=%d):\n%s", rc, out)
	}
	if !strings.Contains(out, "UPGRADE OK: v0.1.0") {
		t.Errorf("expected the OK verdict for v0.1.0 → v0.1.1, got:\n%s", out)
	}
}

// TestUpgradeScriptRejectsSameTagUpgrade pins the skip branch when HEAD ref
// IS the previous tag: there is nothing to upgrade, exit 2, never exit 0.
func TestUpgradeScriptRejectsSameTagUpgrade(t *testing.T) {
	if testing.Short() {
		t.Skip("shares the build path with TestUpgradeScriptRealUpgrade")
	}
	rc, out := runUpgradeScript("v0.1.1", "v0.1.1")
	if rc != 2 {
		t.Fatalf("upgrading v0.1.1 to v0.1.1 must be reported as nothing-to-upgrade (exit 2), got rc=%d:\n%s", rc, out)
	}
	if !strings.Contains(out, "UPGRADE N/A") || !strings.Contains(out, "Not counted as a pass") {
		t.Errorf("skip must be honest N/A wording, got:\n%s", out)
	}
}

// TestUpgradeGuideExists keeps the doc paired with the script.
func TestUpgradeGuideExists(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "..", "docs", "guides", "upgrade-verification.md")); err != nil {
		t.Fatalf("docs/guides/upgrade-verification.md missing: %v", err)
	}
}
