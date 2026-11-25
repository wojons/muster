package builtin

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docs/guides/starlark-dsl.md, relative to the repository root.
const dslGuideRepoPath = "docs/guides/starlark-dsl.md"

// Byte copy of the pre-fix guide (docs/guides/starlark-dsl.md at commit
// db2f4ac3, before 98a083e9 corrected the builtin signatures and added the
// json() encoder section). The drift guard must fail on this text and pass on
// the live guide; the fixture keeps that failing-then-passing proof runnable.
const dslGuidePrefixFixture = "testdata/starlark-dsl-prefix.md"

// dslBuiltinSig is one DSL builtin exactly as the `dsl run --help` text prints
// it. name is the leading identifier; signature is the whitespace-normalized
// signature line ("http_get(url, headers=None) -> (status, body)").
type dslBuiltinSig struct {
	name      string
	signature string
}

// dslSigLineRe matches a builtin signature line in the rendered dsl help text,
// e.g. "  http_post(url, body=None, headers=None) -> (status, body)". Other
// lines in the help (body-type table rows, call examples like
// "  body = json({...})") do not start with identifier "(" nor carry an
// "-> result" tail, so they are not matched.
var dslSigLineRe = regexp.MustCompile(`^\s{2,}([a-zA-Z_][a-zA-Z0-9_]*)\(.*\)\s*->\s*\S`)

// dslPhantomNamesRe finds HTTP/json builtin-shaped identifiers the guide may
// name, e.g. http_patch or json_encode. Attribute-style mentions such as
// "json.loads" are deliberately excluded (the guide documents them as NOT
// provided); the pattern requires an underscore directly after "http"/"json".
var dslPhantomNamesRe = regexp.MustCompile(`\b(?:http|json)_[a-z][a-z0-9_]*\b`)

// whitespaceNormalized collapses every whitespace run to one space so the
// signature compare is insensitive to indentation or re-wrapping.
func whitespaceNormalized(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// dslRunHelpText renders the dsl help text through the command itself, the
// same way TestDslCommand_HelpDocumentsBodyTypes does.
func dslRunHelpText(t *testing.T) string {
	t.Helper()
	cmd := newDslCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Help(); err != nil {
		t.Fatalf("rendering dsl help: %v", err)
	}
	return buf.String()
}

// extractDslSignatureLines pulls the shipped builtin signatures out of the dsl
// help text. The first occurrence of each name wins (names must be unique).
func extractDslSignatureLines(t *testing.T, help string) []dslBuiltinSig {
	t.Helper()
	var out []dslBuiltinSig
	seen := map[string]bool{}
	for _, line := range strings.Split(help, "\n") {
		m := dslSigLineRe.FindStringSubmatch(line)
		if m == nil || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, dslBuiltinSig{
			name:      m[1],
			signature: whitespaceNormalized(line),
		})
	}
	if len(out) == 0 {
		t.Fatalf("no builtin signature lines found in the dsl help text; the extraction pattern needs updating. Help text:\n%s", help)
	}
	return out
}

// dslGuideDriftErrors compares the guide text against the shipped builtins and
// returns one error message per drift. An empty result means the guide matches
// the binary.
func dslGuideDriftErrors(guide string, shipped []dslBuiltinSig) []string {
	normGuide := whitespaceNormalized(guide)
	known := make(map[string]bool, len(shipped))
	var errs []string
	for _, b := range shipped {
		known[b.name] = true
		if !strings.Contains(normGuide, b.signature) {
			errs = append(errs, fmt.Sprintf("guide is missing the shipped builtin signature %q (as printed by `dsl run --help`)", b.signature))
		}
	}
	shippedNames := make([]string, 0, len(shipped))
	for _, b := range shipped {
		shippedNames = append(shippedNames, b.name)
	}
	for _, name := range dslPhantomNamesRe.FindAllString(guide, -1) {
		if !known[name] {
			errs = append(errs, fmt.Sprintf("guide names the builtin %q but the binary does not ship it (shipped: %s)", name, strings.Join(shippedNames, ", ")))
			break
		}
	}
	return errs
}

// repoRootDir walks up from the test working directory to the directory that
// holds go.mod.
func repoRootDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// TestDslGuideBuiltinDrift pins docs/guides/starlark-dsl.md to the builtins
// the CLI actually ships (DF-062 AC5):
//
//  1. every builtin name AND its signature in the `dsl run --help` text
//     produced by the command itself must appear in the guide (whitespace
//     normalized so indentation and re-wrapping do not count as drift), and
//  2. the guide must not name any HTTP/json builtin the binary lacks.
//
// The guard also proves it can detect the drift it was written for: run
// against the pre-fix guide copy in testdata/starlark-dsl-prefix.md it must
// fail with a missing-signature finding.
func TestDslGuideBuiltinDrift(t *testing.T) {
	shipped := extractDslSignatureLines(t, dslRunHelpText(t))

	// Parse-rot tripwire: if cobra help no longer mentions the builtins the
	// package ships, the extraction above is wrong and cannot guard anything.
	knownNames := map[string]bool{"http_get": true, "http_post": true, "http_put": true, "http_delete": true, "json": true}
	for _, b := range shipped {
		delete(knownNames, b.name)
	}
	if len(knownNames) != 0 {
		t.Fatalf("dsl help extraction missed shipped builtins %v; update the extraction to match the rendered help text", sortedKeys(knownNames))
	}

	guideBytes, err := os.ReadFile(filepath.Join(repoRootDir(t), dslGuideRepoPath))
	if err != nil {
		t.Fatalf("reading guide %s: %v", dslGuideRepoPath, err)
	}
	if errs := dslGuideDriftErrors(string(guideBytes), shipped); len(errs) > 0 {
		t.Fatalf("docs/guides/starlark-dsl.md drifted from the shipped DSL builtins:\n\t%s", strings.Join(errs, "\n\t"))
	}

	// The guard must bite on the doc drift it was written for.
	prefixGuide, err := os.ReadFile(dslGuidePrefixFixture)
	if err != nil {
		t.Fatalf("reading pre-fix guide fixture %s: %v", dslGuidePrefixFixture, err)
	}
	prefixErrs := dslGuideDriftErrors(string(prefixGuide), shipped)
	if len(prefixErrs) == 0 {
		t.Fatalf("drift guard did NOT fail on the pre-fix guide fixture %s; it no longer guards against the drifted doc it was written for", dslGuidePrefixFixture)
	}
	bitesOnSignature := false
	for _, e := range prefixErrs {
		if strings.Contains(e, "missing the shipped builtin signature") {
			bitesOnSignature = true
			break
		}
	}
	if !bitesOnSignature {
		t.Fatalf("drift guard failed on the pre-fix guide fixture, but not for a missing shipped signature: %v", prefixErrs)
	}
}

// sortedKeys returns the keys of the map in sorted order (test helper).
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// insertion order is irrelevant for the fatal message; sort for stability.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
