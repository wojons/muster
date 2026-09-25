package test

// GAP-045 test-suite load hygiene: one session-scoped build per binary.
//
// Before this fixture every real-binary integration suite in this package ran
// its own `go build` inside the test body. The CLI suite additionally redirects
// HOME (DF-013, so the exec'd CLI cannot clobber the invoking user's command
// store) *before* that build, which silently relocated Go's build cache to an
// empty directory inside the temp HOME: every run then recompiled the entire
// dependency graph from scratch, saturating all cores (measured: 121 CPU-s /
// 600% CPU for this package alone, ~62% of the whole `go test -short ./...`
// suite's CPU).
//
// Two bounds are applied here, both local to the test suite:
//
//   - one build per binary per session (TestMain), performed BEFORE any test
//     changes HOME, and with the real toolchain caches preserved;
//   - the number of parallel compile/link actions is clamped by
//     MUSTER_TEST_WAVE (2..16, default 4) via `go build -p`, so a cold build
//     cannot eat the whole box.
//
// Tests keep their existing contract: each one gets its own copy of the binary
// inside its t.TempDir(), so per-test cleanup and isolation are unchanged.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// MusterTestWaveEnv is the environment knob bounding how many compile/link
// actions a test-suite build may run in parallel.
const MusterTestWaveEnv = "MUSTER_TEST_WAVE"

const (
	musterTestWaveMin     = 2
	musterTestWaveMax     = 16
	musterTestWaveDefault = 4
)

// MusterTestWave returns the bounded parallel-compile wave used for the
// session builds: MUSTER_TEST_WAVE clamped to [2,16]. Unset, unparsable or
// out-of-range values fall back to / clamp at the nearest bound so the suite
// can never accidentally run unbounded waves.
func MusterTestWave() int {
	raw := strings.TrimSpace(os.Getenv(MusterTestWaveEnv))
	if raw == "" {
		return musterTestWaveDefault
	}

	n, err := strconv.Atoi(raw)
	if err != nil {
		return musterTestWaveDefault
	}

	return clampMusterTestWave(n)
}

func clampMusterTestWave(n int) int {
	if n < musterTestWaveMin {
		return musterTestWaveMin
	}
	if n > musterTestWaveMax {
		return musterTestWaveMax
	}

	return n
}

// sessionBuild describes one binary built once for the whole package run.
type sessionBuild struct {
	name string // logical name tests ask for, e.g. "openapi-cli"
	pkg  string // package to build, relative to the repository root
	path string // absolute path of the shared build
}

// sessionBinaries holds the per-session builds, keyed by logical name, and is
// populated by TestMain before any test runs.
var sessionBinaries = map[string]*sessionBuild{}

// packageBinaryNames lists every binary this package needs, so the session
// fixture builds exactly what the suites execute.
var packageBinaryNames = []string{"openapi-cli", "mcp-server", "daemon"}

func TestMain(m *testing.M) {
	musterTestMain(m)
}

// musterTestMain builds the package's real binaries once, then runs the suite.
// Cleanup happens before os.Exit (which skips deferred calls), so the shared
// build directory never outlives the run.
func musterTestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "muster-test-binaries-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "GAP-045: cannot create session build dir: %v\n", err)
		os.Exit(1)
	}

	code := 1
	if err := buildSessionBinaries(dir); err != nil {
		fmt.Fprintf(os.Stderr, "GAP-045: session binary build failed: %v\n", err)
	} else {
		code = m.Run()
	}
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintf(os.Stderr, "GAP-045: cannot clean session build dir: %v\n", err)
	}

	os.Exit(code)
}

// musterBuildArgs is the argv for one session build. Kept separate so the
// bounded-wave wiring is unit-testable without spawning a real compile.
func musterBuildArgs(wave int, outPath, pkgPath string) []string {
	return []string{"build", "-p", strconv.Itoa(clampMusterTestWave(wave)), "-o", outPath, pkgPath}
}

// buildSessionBinaries performs the one build per binary for this run. The
// builds run sequentially, each bounded to MUSTER_TEST_WAVE parallel compile
// actions, and use the invoking user's real toolchain caches (this runs before
// any test redirects HOME).
func buildSessionBinaries(dir string) error {
	repoRoot, err := getRepoRoot()
	if err != nil {
		return fmt.Errorf("cannot resolve repository root: %w", err)
	}

	wave := MusterTestWave()
	for _, name := range packageBinaryNames {
		pkg := filepath.Join(repoRoot, "cmd", name)
		out := filepath.Join(dir, name+exeSuffix())
		args := musterBuildArgs(wave, out, pkg)

		// Observed live: shows the clamped wave that actually reached the
		// build, so `MUSTER_TEST_WAVE=2 go test ./demo/test-suites/` can be
		// checked without instrumenting the toolchain.
		fmt.Fprintf(os.Stderr, "GAP-045: session build (wave %d): go %s\n", wave, strings.Join(args, " "))

		cmd := exec.Command("go", args...)
		cmd.Dir = repoRoot
		out2, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("go build -p %d %s: %w: %s", wave, pkg, err, strings.TrimSpace(string(out2)))
		}

		sessionBinaries[name] = &sessionBuild{name: name, pkg: pkg, path: out}
	}

	return nil
}

// sharedBinary returns a per-test copy of the session-built binary inside the
// test's own temp dir, preserving the pre-GAP-045 contract that each test owns
// (and cleans up) the path it execs.
func sharedBinary(t *testing.T, name string) string {
	t.Helper()

	build, ok := sessionBinaries[name]
	if !ok || build.path == "" {
		t.Fatalf("GAP-045: no session build registered for %q (known: %v)", name, knownBinaryNames())
	}

	src, err := os.Open(build.path)
	if err != nil {
		t.Fatalf("GAP-045: session build for %q unreadable at %s: %v", name, build.path, err)
	}
	defer func() { _ = src.Close() }()

	dstPath := filepath.Join(t.TempDir(), name+exeSuffix())
	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatalf("GAP-045: cannot create per-test copy of %q: %v", name, err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		t.Fatalf("GAP-045: cannot copy session build of %q: %v", name, err)
	}
	if err := dst.Close(); err != nil {
		t.Fatalf("GAP-045: cannot finalise per-test copy of %q: %v", name, err)
	}

	return dstPath
}

func knownBinaryNames() []string {
	names := make([]string, 0, len(sessionBinaries))
	for name := range sessionBinaries {
		names = append(names, name)
	}

	return names
}

// preserveToolchainCaches pins Go's module and build caches to the invoking
// user's real ones before a test redirects HOME. Without this a test that
// builds under a redirected HOME recompiles the whole dependency graph, since
// Go derives both caches from HOME when they are not explicitly set.
func preserveToolchainCaches(t *testing.T) {
	t.Helper()

	for _, key := range []string{"GOCACHE", "GOMODCACHE"} {
		if os.Getenv(key) != "" {
			continue
		}
		if value := goEnv(t, key); value != "" {
			t.Setenv(key, value)
		}
	}
}

func goEnv(t *testing.T, key string) string {
	t.Helper()

	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		t.Logf("GAP-045: `go env %s` failed (%v); leaving %s to Go's defaults", key, err, key)
		return ""
	}

	return strings.TrimSpace(string(out))
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}

	return ""
}

// TestMusterTestWave_Clamped proves the knob honours its documented contract:
// unset or unparsable falls back to the default, out-of-range clamps to the
// nearest bound, and the result is always inside [2,16].
func TestMusterTestWave_Clamped(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"", musterTestWaveDefault},
		{"   ", musterTestWaveDefault},
		{"abc", musterTestWaveDefault},
		{"1.5", musterTestWaveDefault},
		{"4", 4},
		{" 6 ", 6},
		{"2", 2},
		{"16", 16},
		{"1", musterTestWaveMin},
		{"0", musterTestWaveMin},
		{"-5", musterTestWaveMin},
		{"17", musterTestWaveMax},
		{"999", musterTestWaveMax},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			t.Setenv(MusterTestWaveEnv, tc.raw)

			got := MusterTestWave()
			if got != tc.want {
				t.Fatalf("%s=%q: wave = %d, want %d", MusterTestWaveEnv, tc.raw, got, tc.want)
			}
			if got < musterTestWaveMin || got > musterTestWaveMax {
				t.Fatalf("%s=%q: wave %d outside [%d,%d]", MusterTestWaveEnv, tc.raw, got, musterTestWaveMin, musterTestWaveMax)
			}
		})
	}
}

// TestMusterBuildArgs_BoundsWave proves the knob actually reaches the build
// argv (a knob that is read but never passed would still pass the clamp test).
func TestMusterBuildArgs_BoundsWave(t *testing.T) {
	cases := []struct {
		wave int
		want string
	}{
		{0, "2"},
		{1, "2"},
		{2, "2"},
		{4, "4"},
		{16, "16"},
		{17, "16"},
		{99, "16"},
	}

	for _, tc := range cases {
		args := musterBuildArgs(tc.wave, "/tmp/out", "/repo/cmd/daemon")

		if len(args) == 0 || args[0] != "build" {
			t.Fatalf("wave %d: argv = %v, want it to start with `build`", tc.wave, args)
		}

		got := ""
		for i, arg := range args {
			if arg == "-p" {
				if i+1 >= len(args) {
					t.Fatalf("wave %d: argv %v has -p without a value", tc.wave, args)
				}
				got = args[i+1]
			}
		}
		if got != tc.want {
			t.Fatalf("wave %d: argv %v carries -p %q, want %q", tc.wave, args, got, tc.want)
		}
	}
}

// TestSharedBinary_PerTestCopyIsolated pins the fixture contract the suites
// rely on: every call hands back its own executable copy of the session build,
// with identical content, inside the calling test's temp dir.
func TestSharedBinary_PerTestCopyIsolated(t *testing.T) {
	for _, name := range packageBinaryNames {
		first := sharedBinary(t, name)
		second := sharedBinary(t, name)

		if first == second {
			t.Fatalf("%s: two sharedBinary calls returned the same path %q; per-test isolation is broken", name, first)
		}
		if first == sessionBinaries[name].path {
			t.Fatalf("%s: sharedBinary returned the session build path %q; the exec'd binary would be shared state", name, first)
		}

		for _, path := range []string{first, second} {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("%s: per-test copy %q unreadable: %v", name, path, err)
			}
			if info.Mode().Perm()&0o111 == 0 {
				t.Fatalf("%s: per-test copy %q is not executable (mode %v)", name, path, info.Mode())
			}
		}

		want, err := os.ReadFile(sessionBinaries[name].path)
		if err != nil {
			t.Fatalf("%s: cannot read session build: %v", name, err)
		}
		got, err := os.ReadFile(first)
		if err != nil {
			t.Fatalf("%s: cannot read per-test copy: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s: per-test copy differs from the session build (%d vs %d bytes)", name, len(got), len(want))
		}
	}
}

// TestPackageBinaryNames_CoverSharedBinaryRequests keeps the fixture honest: it
// scans this package's test sources for every binary the suites ask for and
// fails if one is not session-built (the suites would fail at exec time) or if
// a session build is never requested (dead build). A scan that finds no call
// sites fails too, so the guard cannot silently become vacuous.
func TestPackageBinaryNames_CoverSharedBinaryRequests(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the fixture source file")
	}

	entries, err := os.ReadDir(filepath.Dir(thisFile))
	if err != nil {
		t.Fatalf("cannot list the package directory: %v", err)
	}

	callSite := regexp.MustCompile(`sharedBinary\(t, "([^"]+)"\)`)
	requested := map[string]int{}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), entry.Name()))
		if err != nil {
			t.Fatalf("cannot read %s: %v", entry.Name(), err)
		}
		for _, match := range callSite.FindAllStringSubmatch(string(src), -1) {
			requested[match[1]]++
		}
	}

	if len(requested) == 0 {
		t.Fatal("scan found no sharedBinary call sites; the coverage guard is vacuous")
	}

	for name := range requested {
		if _, ok := sessionBinaries[name]; !ok {
			t.Errorf("suites request %q but TestMain never builds it (session builds: %v)", name, knownBinaryNames())
		}
	}
	for _, name := range packageBinaryNames {
		if requested[name] == 0 {
			t.Errorf("%q is session-built but no test asks for it; drop it or wire it up", name)
		}
	}
}
