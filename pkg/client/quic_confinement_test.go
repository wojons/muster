// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─────────────────────────────────────────────────────────────────────────────
// The supply-chain half of the transport rows (specs/PROTOCOLS.md §2 rows 15/16
// and §7 "Supply-chain cost, re-measured"): HTTP/2 costs NO new module — it is
// configuration plus observability over the standard library — and the one new
// module, quic-go, is confined to the HTTP/3 transport, so a caller that never
// opts into HTTP/3 never links it.
//
// Both are properties of the tree rather than of a runtime probe, so they are
// asserted by reading the tree: an import that appeared in a shared file would
// silently put QUIC on the HTTP/2 path, and nothing else in the suite would
// notice.
//
// The limit of this method is named rather than hidden: the scan is over
// *source imports*, so it catches a direct import of QUIC anywhere in the
// repository but not a transitive dependency of some other module that pulled
// it in. Only one module in go.mod is quic-go's, which is what makes the
// source-level claim meaningful (asserted below).

const (
	// quicModulePrefix is the module family the HTTP/3 transport adds.
	quicModulePrefix = "github.com/quic-go/"

	// http3TransportFile is the only shipped file allowed to import it: the
	// whole HTTP/3 transport, QUIC configuration included, is behind this file
	// (pkg/client/http3.go's package comment is the supply-chain record).
	http3TransportFile = "pkg/client/http3.go"

	// http3FixtureFile is the demo's loopback HTTP/3 service — a program, not a
	// library, and only the h3 demo leg starts it (demo/demos/20-http3-transport.sh).
	http3FixtureFile = "demo/services/transport-fixture/main.go"

	// quicRequirement is the release specs/PROTOCOLS.md decides for row 16 and
	// §7 re-measures. A bump is supposed to break this test so the pin, the
	// measured cost and the spec move together.
	quicRequirement = "github.com/quic-go/quic-go v0.63.0"
)

// skippedDirs never hold repository Go source: VCS state, virtualenvs, vendored
// trees and build output.
var skippedDirs = map[string]bool{
	"vendor":       true,
	"node_modules": true,
	"bin":          true,
	"dist":         true,
	"venv":         true,
	"__pycache__":  true,
}

// repoRoot resolves the repository root from this package's directory (the
// working directory of a Go test binary is its package directory).
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(root, "go.mod"))
	require.NoError(t, err, "the test must run from its package directory inside the repository")
	return root
}

// modulePath reads the module path declared by the repository's go.mod, so the
// "standard library or this module" rule below cannot go stale.
func modulePath(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	require.NoError(t, err)
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	t.Fatal("go.mod declares no module path")
	return ""
}

// sourceImports walks root and returns the non-test Go files of the repository
// mapped to the import paths each declares. Relative paths use slashes, so they
// compare to the constants above on every platform.
func sourceImports(t *testing.T, root string) map[string][]string {
	t.Helper()
	fset := token.NewFileSet()
	imports := make(map[string][]string)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (strings.HasPrefix(d.Name(), ".") || skippedDirs[d.Name()]) {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		key := filepath.ToSlash(rel)
		for _, spec := range file.Imports {
			importPath, uerr := strconv.Unquote(spec.Path.Value)
			if uerr != nil {
				return uerr
			}
			imports[key] = append(imports[key], importPath)
		}
		return nil
	})
	require.NoError(t, err, "scanning %s", root)

	return imports
}

// importersOf returns the repository-relative files importing an import path
// with the given prefix, sorted.
func importersOf(imports map[string][]string, prefix string) []string {
	var files []string
	for file, paths := range imports {
		for _, p := range paths {
			if strings.HasPrefix(p, prefix) {
				files = append(files, file)
				break
			}
		}
	}
	sort.Strings(files)
	return files
}

// TestQUICGoIsConfinedToTheHTTP3Files is coverage item 4 of the row: quic-go
// stays confined to the HTTP/3 transport, so the HTTP/1.1 and HTTP/2 paths
// remain plan-free. A new importer anywhere in the repository fails here.
func TestQUICGoIsConfinedToTheHTTP3Files(t *testing.T) {
	root := repoRoot(t)
	importers := importersOf(sourceImports(t, root), quicModulePrefix)

	// Non-vacuity first: the claim is about a boundary that must still have a
	// QUIC side, so the transport file has to be in the set.
	assert.Contains(t, importers, http3TransportFile,
		"the HTTP/3 transport must import the QUIC module, or this test proves nothing")

	assert.ElementsMatch(t, []string{http3TransportFile, http3FixtureFile}, importers,
		"the QUIC module may only be imported by the HTTP/3 transport and its loopback demo fixture; "+
			"a new importer puts QUIC on a path that opted out of it (specs/PROTOCOLS.md §2 row 16, §7)")
}

// TestHTTP2PathImportsTheStandardLibraryOnly is the other half of item 4: the
// files that realise the HTTP/2 path depend on nothing outside the standard
// library and this module, which is what "ADOPT — no new dependency" means for
// matrix row 15.
func TestHTTP2PathImportsTheStandardLibraryOnly(t *testing.T) {
	root := repoRoot(t)
	module := modulePath(t, root)
	imports := sourceImports(t, root)

	h2Path := []string{
		"pkg/client/transport.go",
		"pkg/client/protocol.go",
	}
	for _, file := range h2Path {
		require.Contains(t, imports, file, "%s must be part of this repository", file)
		for _, importPath := range imports[file] {
			assert.False(t, strings.HasPrefix(importPath, quicModulePrefix),
				"%s must not import QUIC: the HTTP/2 path is the plan-free one", file)
			assert.True(t, isStandardLibraryOrModule(importPath, module),
				"%s imports %q, which is neither the standard library nor this module", file, importPath)
		}
	}
}

// TestGoModPinsTheQUICRelease pins the supply-chain decision itself: the single
// new module is the release specs/PROTOCOLS.md row 16 decides, and the
// transport's own helper (qpack) stays an indirect requirement rather than
// something a shipped file imports.
func TestGoModPinsTheQUICRelease(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	require.NoError(t, err)
	gomod := string(data)

	assert.Contains(t, gomod, quicRequirement,
		"go.mod must pin the QUIC release the spec decides; a bump moves the spec's measured cost with it")

	var quicModules []string
	for _, line := range strings.Split(gomod, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.HasPrefix(fields[0], quicModulePrefix) {
			quicModules = append(quicModules, fields[0])
		}
	}
	assert.ElementsMatch(t, []string{
		"github.com/quic-go/quic-go",
		"github.com/quic-go/qpack",
	}, quicModules,
		"exactly the two quic-go modules PROTO-012 measured may be required (the second is the transport's own helper)")
}

// isStandardLibraryOrModule reports whether an import path is the standard
// library (its first element carries no dot, the property goimports groups on)
// or a package of this module.
func isStandardLibraryOrModule(importPath, module string) bool {
	if strings.HasPrefix(importPath, module+"/") || importPath == module {
		return true
	}
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}
