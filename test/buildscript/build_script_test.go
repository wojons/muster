// Package main holds the QA-MUSTER-20 build-script verification tests.
//
// scripts/build.sh is the cross-platform wrapper the build-system spec
// documents (specs/BUILD-SYSTEM.md §3.2 body, §7.2 invocation). Its
// pre-QA-MUSTER-20 form was dead code: it ran `go build ./cmd` although cmd/
// holds only package-less entry-point directories (cmd/openapi-cli,
// cmd/mcp-server, cmd/daemon), so `bash scripts/build.sh linux amd64` exited 1
// with "no Go files in <repo>/cmd"; its -ldflags named the retired
// github.com/anomalyco/muster module, so even a successful build stamped
// nothing the binary could report; and the spec embedded a byte copy of the
// broken script plus a `./scripts/build.sh` invocation that only works when the
// file is executable.
//
// These tests pin the three properties a reader of the spec depends on:
//
//  1. the script parses and builds the three REAL entry points (never the bare
//     cmd directory), with the module path from go.mod in every -X stamp;
//  2. the spec's §3.2 fence is byte-identical to the shipped script and §7.2
//     invokes it in a form that works on a fresh checkout;
//  3. a real `bash scripts/build.sh linux amd64` run with VERSION overridden
//     exits 0, leaves an executable bin/muster-linux-amd64, and that binary
//     reports the injected version instead of the "dev" fallback.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	buildScript = "../../scripts/build.sh"
	specPath    = "../../specs/BUILD-SYSTEM.md"
	goModPath   = "../../go.mod"

	// entryPoints are the real main packages under cmd/.
	openapiCLIEntry = "./cmd/openapi-cli"
	mcpServerEntry  = "./cmd/mcp-server"
	daemonEntry     = "./cmd/daemon"

	// linuxAMD64Output is the AC-1 artifact path.
	linuxAMD64Output = "../../bin/muster-linux-amd64"
)

// readRepoFile reads a repo-relative path (relative to this package).
func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(rel)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// modulePath returns the module directive from go.mod.
func modulePath(t *testing.T) string {
	t.Helper()
	for _, line := range strings.Split(readRepoFile(t, goModPath), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "module "))
			if path == "" {
				t.Fatal("go.mod module directive is empty")
			}
			return path
		}
	}
	t.Fatal("go.mod has no module directive")
	return ""
}

// specFence extracts the body of the first ```bash fence of a level-3 section.
func specFence(t *testing.T, heading string) string {
	t.Helper()
	spec := readRepoFile(t, specPath)
	idx := strings.Index(spec, heading)
	if idx < 0 {
		t.Fatalf("spec section %q missing from %s", heading, specPath)
	}
	rest := spec[idx:]
	open := -1
	var marker string
	for _, m := range []string{"```bash\r\n", "```bash\n"} {
		if o := strings.Index(rest, m); o >= 0 {
			open, marker = o, m
			break
		}
	}
	if open < 0 {
		t.Fatalf("section %q has no bash fence", heading)
	}
	tbody := rest[open+len(marker):]
	closeIdx := strings.Index(tbody, "\n```")
	if closeIdx < 0 {
		t.Fatalf("section %q fence is unterminated", heading)
	}
	return tbody[:closeIdx] + "\n"
}

// TestBuildScript_SyntaxAndRealEntryPoints pins AC-2: the script parses, builds
// all three real entry points, and no longer names the package-less cmd/
// directory as a build target.
func TestBuildScript_SyntaxAndRealEntryPoints(t *testing.T) {
	script := readRepoFile(t, buildScript)

	syntax := exec.Command("bash", "-n", buildScript)
	if out, err := syntax.CombinedOutput(); err != nil {
		t.Fatalf("bash -n %s failed: %v\n%s", buildScript, err, out)
	}

	// Every entry point the script builds must be a real main package.
	for name, entry := range map[string]string{
		"openapi-cli": openapiCLIEntry,
		"mcp-server":  mcpServerEntry,
		"daemon":      daemonEntry,
	} {
		if !strings.Contains(script, `"`+entry+`"`) {
			t.Errorf("script does not build %s by its real path %q", name, entry)
		}
		mainGo := filepath.Join("..", "..", strings.TrimPrefix(entry, "./"), "main.go")
		if _, err := os.Stat(mainGo); err != nil {
			t.Errorf("%s is not a main package (%s): %v", entry, mainGo, err)
		}
	}

	// The defect: `go build ... ./cmd`, which has no Go files.
	if regexp.MustCompile(`(?m)^\s*go build[^\n]*\./\$(CMD_DIR|\{?CMD_DIR\}?)\s*$`).MatchString(script) {
		t.Errorf("script still targets the bare cmd/ directory:\n%s", script)
	}
	if strings.Contains(script, `CMD_DIR="cmd"`) {
		t.Error(`script still defines CMD_DIR="cmd" as a build target`)
	}
	if regexp.MustCompile(`(?m)^\s*go build[^\n]*\s\./cmd\s*$`).MatchString(script) {
		t.Error("script still runs `go build ./cmd`")
	}
}

// TestBuildScript_VersionStampModulePath pins AC-3: every -X stamp names the
// module from go.mod, and the retired anomalyco path is gone.
func TestBuildScript_VersionStampModulePath(t *testing.T) {
	script := readRepoFile(t, buildScript)
	mod := modulePath(t)

	if strings.Contains(script, "anomalyco") {
		t.Errorf("script still references the retired anomalyco module path")
	}
	if got := strings.Count(script, "anomalyco"); got != 0 {
		t.Errorf("grep -c anomalyco scripts/build.sh = %d, want 0", got)
	}

	stamp := regexp.MustCompile(`-X\s+(\S+)/internal/version\.(\w+)=`)
	matches := stamp.FindAllStringSubmatch(script, -1)
	if len(matches) == 0 {
		t.Fatal("script has no -X internal/version stamps")
	}
	seen := map[string]bool{}
	for _, m := range matches {
		if m[1] != mod {
			t.Errorf("-X stamp uses module %q, want %q (from go.mod)", m[1], mod)
		}
		seen[m[2]] = true
	}
	for _, field := range []string{"Version", "Commit", "BuildDate"} {
		if !seen[field] {
			t.Errorf("missing -X stamp for internal/version.%s", field)
		}
	}
}

// TestBuildScript_SpecMatchesShippedScript pins AC-5: the §3.2 embedded body is
// byte-identical to the shipped script, and §7.2 invokes it in a working form.
func TestBuildScript_SpecMatchesShippedScript(t *testing.T) {
	script := readRepoFile(t, buildScript)

	if documented := specFence(t, "### 3.2 Cross-Platform Build Script"); documented != script {
		t.Errorf("specs/BUILD-SYSTEM.md §3.2 body differs from scripts/build.sh\n--- documented ---\n%s\n--- shipped ---\n%s", documented, script)
	}

	usage := specFence(t, "### 7.2 Cross-Platform Builds")
	if !strings.Contains(usage, "bash scripts/build.sh linux amd64") {
		t.Errorf("§7.2 does not document `bash scripts/build.sh linux amd64`:\n%s", usage)
	}
	if strings.Contains(usage, "./scripts/build.sh linux amd64") {
		t.Error("§7.2 still documents the exec-bit-dependent `./scripts/build.sh` invocation")
	}
}

// TestBuildScript_LinuxAMD64BuildInjectsVersion pins AC-1 and AC-4 end to end:
// the documented command exits 0, leaves an executable artifact at the
// documented path, and the binary reports the injected VERSION rather than the
// build-info/"dev" fallback.
//
// The build leg needs a host Go toolchain matching the script's cgo default and
// a linux/amd64 host; it is skipped (with the reason) elsewhere rather than
// silently passing.
func TestBuildScript_LinuxAMD64BuildInjectsVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("builds three binaries; skipped in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("no go toolchain on PATH: %v", err)
	}

	const injected = "v9.9.9"
	cmd := exec.Command("bash", buildScript, "linux", "amd64")
	cmd.Env = append(os.Environ(), "VERSION="+injected)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash scripts/build.sh linux amd64 exited non-zero: %v\n%s", err, out)
	}

	info, err := os.Stat(linuxAMD64Output)
	if err != nil {
		t.Fatalf("no artifact at bin/muster-linux-amd64: %v\n%s", err, out)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("bin/muster-linux-amd64 mode = %v, want executable", info.Mode())
	}

	versionOut, err := exec.Command(linuxAMD64Output, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("%s version failed: %v\n%s", linuxAMD64Output, err, versionOut)
	}
	if !strings.Contains(string(versionOut), injected) {
		t.Errorf("binary reports %q, want it to contain the injected %q (buildinfo/dev fallback leaked)", strings.TrimSpace(string(versionOut)), injected)
	}
	if strings.TrimSpace(string(versionOut)) == "dev (unknown)" {
		t.Error("binary fell back to the bare-build defaults; ldflags stamp did not land")
	}
}
