package grpc

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"

	"github.com/wojons/muster/pkg/errors"
)

// loadProtoSources parses .proto files with protoreflect's pure-Go parser
// (no protoc invocation anywhere — acceptance criterion).
//
// Supported URI shapes:
//
//	file.proto                      — single file, imports resolved from the file's dir
//	a.proto,b.proto                 — multiple files
//	dir1:dir2=a.proto               — relative file resolved against dir1 (cwd fallback),
//	                                  with dir1:dir2 as import paths
//	dir1:dir2=a.proto,b.proto       — dir1/import paths apply to all listed files
func loadProtoSources(uri string) ([]*desc.FileDescriptor, error) {
	importPaths := []string{}
	filesPart := uri
	if idx := strings.Index(uri, "="); idx >= 0 {
		// Only treat a prefix as import paths when every path segment before
		// '=' is an existing directory (a stray '=' inside a filename or a
		// Windows drive letter must not be misread as an import-path spec).
		candidate := strings.Split(uri[:idx], ":")
		allDirs := true
		for _, d := range candidate {
			if d == "" {
				allDirs = false
				break
			}
			if info, err := os.Stat(d); err != nil || !info.IsDir() {
				allDirs = false
				break
			}
		}
		if allDirs {
			importPaths = candidate
			filesPart = uri[idx+1:]
		}
	}

	files := splitFileList(filesPart)
	if len(files) == 0 {
		return nil, errors.Errorf(errors.CodeValidation, "no .proto files in discovery URI %q", uri)
	}

	parser := &protoparse.Parser{
		Accessor:         diskAccessor(importPaths, files),
		InferImportPaths: true,
	}
	fds, err := parser.ParseFiles(files...)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeValidation,
			fmt.Sprintf("parse proto files %v (import paths %v)", files, importPaths))
	}
	return fds, nil
}

// splitFileList splits a comma-joined file list, trimming whitespace.
func splitFileList(list string) []string {
	var out []string
	for _, f := range strings.Split(list, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// diskAccessor builds a FileAccessor that reads files from disk without
// protoparse's import-path joining: absolute paths are read verbatim;
// relative names resolve against the first declared dir then the current
// directory (files named in the URI) or against the import paths in order
// (import statements, or the current directory when none are given). This
// keeps absolute temp-dir paths working (protoparse's built-in ImportPaths
// resolution would double-prefix them: <import-path>/<absolute-path>).
func diskAccessor(importPaths, files []string) protoparse.FileAccessor {
	return func(name string) (io.ReadCloser, error) {
		if filepath.IsAbs(name) {
			return openProtoFile(name)
		}
		// A requested file by literal name: the URI's dirs are its resolution
		// base (DF-026), not only their includes' import paths.
		for _, f := range files {
			if f == name {
				return openRequestedFile(importPaths, name)
			}
		}
		// An import statement: resolve against import paths in order.
		searchPaths := importPaths
		if len(searchPaths) == 0 {
			searchPaths = []string{"."}
		}
		var lastErr error
		for _, dir := range searchPaths {
			f, err := os.Open(filepath.Join(dir, name))
			if err == nil {
				return f, nil
			}
			lastErr = err
		}
		return nil, fmt.Errorf("import %q not found (searched %v): %w", name, searchPaths, lastErr)
	}
}

// openRequestedFile opens a file named verbatim in the discovery URI (as
// opposed to one pulled in by an import statement). The "dir1:dir2=file.proto"
// form declares where the file lives as well as its import paths, so a
// relative name resolves against the FIRST declared dir and falls back to the
// current directory — which keeps bare "file.proto" (no dirs declared) and
// cwd-relative usage exactly as they were.
func openRequestedFile(importPaths []string, name string) (io.ReadCloser, error) {
	if len(importPaths) > 0 {
		if f, err := os.Open(filepath.Join(importPaths[0], name)); err == nil {
			if info, statErr := f.Stat(); statErr == nil && !info.IsDir() {
				return f, nil
			}
			_ = f.Close()
		}
	}
	return openProtoFile(name)
}

// openProtoFile opens a proto file with a mapped error (CodeNotFound on
// read failure per §4.2's target-not-found row).
func openProtoFile(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeNotFound, "read proto file "+path)
	}
	return f, nil
}
