package version

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The README states the Go toolchain floor in three surfaces: the
// prerequisites table, the distro-toolchain note under it, and the demo
// section's requirements line. Nothing else in the repository ties those
// claims to go.mod, so a `go` directive bump silently left the README stale —
// it advertised 1.26.10 while go.mod declared 1.26.11 (RELEASE-022).
//
// These tests are the pairing guard: every Go version the README states must
// be exactly the version go.mod's `go` directive declares, so a bump without
// a README sync fails here. The comparison itself is also driven with
// synthetic in-memory documents, so the guard's ability to fail is proven
// inside the suite rather than assumed.

// docSyncRepoRoot resolves the repository root from this package's directory
// (the working directory of a Go test binary is its package directory), the
// same resolution pkg/client/quic_confinement_test.go uses, so the guard
// works in a linked worktree and in CI alike.
func docSyncRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(root, "go.mod"))
	require.NoError(t, err, "the test must run from its package directory inside the repository")
	_, err = os.Stat(filepath.Join(root, "README.md"))
	require.NoError(t, err, "the test must run inside a checkout that has the README")
	return root
}

// goVersionClaim is one Go version the README states, with the line that
// states it (for the failure message).
type goVersionClaim struct {
	version string
	where   string
}

var (
	// directiveVersion matches a bare go directive version (X.Y or X.Y.Z),
	// which is what distinguishes `go 1.26.11` from the `godebug` directive
	// (which also starts with "go") and the `toolchain` directive.
	directiveVersion = regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)

	// goToolchainContext matches the Go-toolchain language a README version
	// claim is written against: the go.mod reference, the `go version` /
	// `go` directive mentions, or a "Go <version>" requirement statement.
	goToolchainContext = regexp.MustCompile(`(?i)go\.mod|go[ \t]+version|go[ \t]+directive|go[ \t]+[0-9]`)

	// versionClaim matches a three-part version token (X.Y.Z), the form both
	// the go directive and the README's claims use. Its boundaries are what
	// keep the extraction precise: a v-prefixed module version (v0.63.0) has
	// no word boundary after the "v", so it never matches, and two-part
	// numbers that are not Go versions (Ubuntu 23.04) are outside the claim
	// form entirely. A future two-part `go` directive would therefore fail
	// the guard's non-vacuity assertion loudly instead of passing silently.
	versionClaim = regexp.MustCompile(`\b\d+\.\d+\.\d+\b`)
)

// goDirectiveVersion returns the version declared by go.mod's `go` directive
// ("1.26.11" for `go 1.26.11`).
func goDirectiveVersion(t *testing.T, gomod string) string {
	t.Helper()
	for _, line := range strings.Split(gomod, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "go ") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 && directiveVersion.MatchString(fields[1]) {
			return fields[1]
		}
	}
	t.Fatal("go.mod declares no parseable `go <version>` directive")
	return ""
}

// paragraphBlocks splits a markdown document into blocks of consecutive
// non-blank lines. A version claim and the toolchain language it is written
// against do not always share a line — the "currently `1.26.11`" clause of
// the distro-toolchain note wraps onto its own line — but they do share a
// paragraph, so the claim scan runs per block.
func paragraphBlocks(doc string) []string {
	var blocks []string
	var current []string
	for _, line := range strings.Split(doc, "\n") {
		if strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				blocks = append(blocks, strings.Join(current, "\n"))
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		blocks = append(blocks, strings.Join(current, "\n"))
	}
	return blocks
}

// goVersionClaims extracts every Go version the README states: a three-part
// version token inside a block that talks about the Go toolchain. The limit
// of this method is named rather than hidden: a claim whose toolchain context
// wrapped into a separate paragraph would be missed, and a Go version written
// in a two-part form would not match the claim token at all — the guard's
// non-vacuity assertion turns the total loss of claims into a failure, not a
// pass.
func goVersionClaims(readme string) []goVersionClaim {
	var claims []goVersionClaim
	for _, block := range paragraphBlocks(readme) {
		if !goToolchainContext.MatchString(block) {
			continue
		}
		for _, line := range strings.Split(block, "\n") {
			for _, version := range versionClaim.FindAllString(line, -1) {
				claims = append(claims, goVersionClaim{
					version: version,
					where:   strings.TrimSpace(line),
				})
			}
		}
	}
	return claims
}

// docSyncProblems returns one problem per README Go-version claim that
// disagrees with the go.mod directive; an empty result means the README's
// toolchain surfaces are in sync with go.mod.
func docSyncProblems(goDirective string, readme string) []string {
	var problems []string
	for _, claim := range goVersionClaims(readme) {
		if claim.version != goDirective {
			problems = append(problems, fmt.Sprintf(
				"README states Go version %s (%q) but go.mod declares %s",
				claim.version, claim.where, goDirective))
		}
	}
	return problems
}

// TestReadmeGoVersionsMatchGoMod is the RELEASE-022 pairing guard: every Go
// version the README states must be exactly the version go.mod's `go`
// directive declares, so bumping the directive without syncing the README's
// toolchain surfaces fails here. The README currently makes the claim in
// three surfaces (the prerequisites table, the distro-toolchain note, and the
// demo section), several tokens each.
func TestReadmeGoVersionsMatchGoMod(t *testing.T) {
	root := docSyncRepoRoot(t)

	gomodData, err := os.ReadFile(filepath.Join(root, "go.mod"))
	require.NoError(t, err)
	readmeData, err := os.ReadFile(filepath.Join(root, "README.md"))
	require.NoError(t, err)

	directive := goDirectiveVersion(t, string(gomodData))

	// Non-vacuity first: a README that states no Go version at all is itself
	// a regression of the prerequisites documentation, not a pass.
	claims := goVersionClaims(string(readmeData))
	require.NotEmpty(t, claims,
		"no Go version claims found in README.md: either the prerequisites "+
			"documentation lost its toolchain requirement or this test's "+
			"claim extraction rotted")

	assert.Empty(t, docSyncProblems(directive, string(readmeData)),
		"README Go-version claims disagree with go.mod's `go %s` directive; "+
			"the toolchain surfaces must move together with the directive "+
			"(RELEASE-022)", directive)
}

// TestReadmeGoVersionGuardDetectsDrift proves the guard's comparison is live
// with documents edited in memory — the repository's own files are never
// modified to prove a failure. A README lagging the directive fails and names
// the stale token, a README in sync passes, and the drift fixture is the
// exact claim shape the README shipped stale in (RELEASE-022 found 1.26.10
// prose against a 1.26.11 directive).
func TestReadmeGoVersionGuardDetectsDrift(t *testing.T) {
	const directive = "1.26.11"

	drifted := "| **Go 1.26.10+** (repo developed on **1.26.10**) | the `go` directive in [`go.mod`](go.mod) is `1.26.10`; |\n" +
		"(currently `1.26.10`; the repo is developed and tested on that release) before"
	inSync := strings.ReplaceAll(drifted, "1.26.10", directive)

	problems := docSyncProblems(directive, drifted)
	require.NotEmpty(t, problems,
		"a README claiming any Go version other than the go directive must fail the guard")
	assert.Contains(t, problems[0], "1.26.10",
		"the failure must name the stale token")
	assert.Contains(t, problems[0], directive,
		"the failure must name the expected directive")

	assert.Empty(t, docSyncProblems(directive, inSync),
		"a README whose claims match the go directive passes the guard")

	// A toolchain context with no version token extracts no claims: the live
	// guard's non-vacuity assertion is what turns that shape into a failure,
	// and this arm pins that the extractor does not invent a claim for it.
	assert.Empty(t, goVersionClaims("See the `go` directive in [go.mod](go.mod) before building."),
		"a toolchain context with no version token must not produce a claim")
}
