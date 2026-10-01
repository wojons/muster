package grpc

import (
	"bytes"
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
		// '=' is an existing directory (a stray '=' inside a filename must not
		// be misread as an import-path spec).
		candidate := splitImportPaths(uri[:idx])
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

// splitImportPaths splits the "dir1:dir2" import-path list of the
// "dir1:dir2=file.proto" form on ':', the separator documented for every
// platform (the legacy form "dir1;dir2=file.proto" is not accepted, so
// filepath.SplitList — ';' on Windows — is not the right splitter).
//
// A Windows absolute path carries a volume colon ("C:\dirs\calc.proto"), so a
// plain strings.Split turned the single directory "C:\dirs" into the two
// bogus segments "C" and "\dirs", the all-dirs check failed, and the '=' was
// never recognized: the whole URI was then read as one file path and every
// declared-dir form failed on Windows. A ':' is therefore a separator only
// when it is not the volume designator that opens a path segment, i.e. not the
// second character of a segment whose first character is an ASCII letter and
// which is followed by a volume root ('\' or '/'). Unix paths never match that
// shape, so this is identical to strings.Split there.
func splitImportPaths(list string) []string {
	parts := make([]string, 0, 2)
	start := 0
	for i := 0; i < len(list); i++ {
		if list[i] != ':' {
			continue
		}
		if isVolumeDesignator(list, start, i) {
			continue
		}
		parts = append(parts, list[start:i])
		start = i + 1
	}
	return append(parts, list[start:])
}

// isVolumeDesignator reports whether the ':' at index i opens a Windows
// volume ("C:\", "c:/") inside the segment that starts at index start.
func isVolumeDesignator(list string, start, i int) bool {
	if i != start+1 || i+1 >= len(list) {
		return false
	}
	drive := list[start]
	if !(drive >= 'a' && drive <= 'z' || drive >= 'A' && drive <= 'Z') {
		return false
	}
	return list[i+1] == '\\' || list[i+1] == '/'
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
			f, err := protoReader(filepath.Join(dir, name))
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
		if path := filepath.Join(importPaths[0], name); isRegularFile(path) {
			return protoReader(path)
		}
	}
	return openProtoFile(name)
}

// protoReader reads a proto source into memory and returns a reader over the
// bytes.
//
// Every reader this package hands the parser must be memory-backed: with
// InferImportPaths set, protoparse's recursive pre-pass (parseToAST in
// jhump/protoreflect v1.18.1) reads the accessor's ReadCloser through
// parser.Parse and never closes it, so an *os.File returned here leaks one
// handle per parsed file for the life of the process. On Windows the leaked
// handle also makes the file undeletable, which turned every test that parses
// a proto from a temp dir into "TempDir RemoveAll cleanup: unlinkat ...
// The process cannot access the file because it is being used by another
// process". The parse itself only reads the source, so buffering it is safe.
// The raw error is returned unwrapped so callers keep their own message.
func protoReader(path string) (io.ReadCloser, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// isRegularFile reports whether path exists and is a regular file (not a
// directory), used to probe a candidate without holding a handle open.
func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// openProtoFile opens a proto file with a mapped error (CodeNotFound on
// read failure per §4.2's target-not-found row).
func openProtoFile(path string) (io.ReadCloser, error) {
	f, err := protoReader(path)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeNotFound, "read proto file "+path)
	}
	return f, nil
}
