package grpc

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jhump/protoreflect/desc"

	apperrors "github.com/wojons/muster/pkg/errors"
)

// writeProtoDir creates a directory holding protoName (content = source) and
// returns its path.
func writeProtoDir(t *testing.T, protoName, source string) string {
	t.Helper()
	dir := t.TempDir()
	writeProtoFile(t, filepath.Join(dir, protoName), source)
	return dir
}

// writeProtoFile writes source to path, creating parent directories.
func writeProtoFile(t *testing.T, path, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// calcProto names its service after pkg so a resolution test can tell WHICH
// copy of an identically named file was actually read.
func calcProto(pkg string) string {
	return `syntax = "proto3";
package ` + pkg + `;
service Calculator {
  rpc Add(AddRequest) returns (AddResponse);
}
message AddRequest { int32 a = 1; int32 b = 2; }
message AddResponse { int32 sum = 1; }
`
}

// serviceNames returns the fully-qualified service names of the descriptors.
func serviceNames(fds []*desc.FileDescriptor) []string {
	var out []string
	for _, fd := range fds {
		for _, svc := range fd.GetServices() {
			out = append(out, svc.GetFullyQualifiedName())
		}
	}
	return out
}

// TestLoadProtoSources_RelativeFileResolvesAgainstFirstDeclaredDir is the
// DF-026 regression: "dir=file.proto" must resolve a RELATIVE file against the
// declared dir, not the current working directory.
func TestLoadProtoSources_RelativeFileResolvesAgainstFirstDeclaredDir(t *testing.T) {
	dir := writeProtoDir(t, "calc.proto", calcProto("calc"))
	t.Chdir(t.TempDir()) // cwd deliberately elsewhere

	fds, err := loadProtoSources(dir + "=calc.proto")
	if err != nil {
		t.Fatalf("loadProtoSources(%q) = %v, want nil", dir+"=calc.proto", err)
	}
	if got := serviceNames(fds); len(got) != 1 || got[0] != "calc.Calculator" {
		t.Fatalf("services = %v, want [calc.Calculator]", got)
	}
	// The file keeps the name it was requested by (relative), so descriptor
	// names and derived artifacts do not change shape.
	if got := fds[0].GetName(); got != "calc.proto" {
		t.Fatalf("file descriptor name = %q, want %q", got, "calc.proto")
	}
}

// TestLoadProtoSources_ResolutionOrder_FirstDirThenCwd pins the fallback
// order: the first declared dir wins, and a file absent there still resolves
// against the current directory.
func TestLoadProtoSources_ResolutionOrder_FirstDirThenCwd(t *testing.T) {
	declared := writeProtoDir(t, "calc.proto", calcProto("declared"))
	cwd := writeProtoDir(t, "calc.proto", calcProto("fromcwd"))
	t.Chdir(cwd)

	// 1. First declared dir wins over cwd when both hold the file.
	fds, err := loadProtoSources(declared + "=calc.proto")
	if err != nil {
		t.Fatalf("first-dir resolution: %v", err)
	}
	if got := serviceNames(fds); len(got) != 1 || got[0] != "declared.Calculator" {
		t.Fatalf("services = %v, want [declared.Calculator] (first declared dir must win)", got)
	}

	// 2. Declared dir without the file -> cwd fallback.
	empty := t.TempDir()
	fds, err = loadProtoSources(empty + "=calc.proto")
	if err != nil {
		t.Fatalf("cwd fallback: %v", err)
	}
	if got := serviceNames(fds); len(got) != 1 || got[0] != "fromcwd.Calculator" {
		t.Fatalf("services = %v, want [fromcwd.Calculator] (cwd fallback)", got)
	}

	// 3. Bare filename (no declared dirs) still resolves against cwd.
	fds, err = loadProtoSources("calc.proto")
	if err != nil {
		t.Fatalf("bare filename: %v", err)
	}
	if got := serviceNames(fds); len(got) != 1 || got[0] != "fromcwd.Calculator" {
		t.Fatalf("services = %v, want [fromcwd.Calculator] (bare filename uses cwd)", got)
	}
}

// TestLoadProtoSources_AbsoluteFilePathUnchanged keeps the documented
// absolute-file form working from any cwd.
func TestLoadProtoSources_AbsoluteFilePathUnchanged(t *testing.T) {
	dir := writeProtoDir(t, "calc.proto", calcProto("calc"))
	t.Chdir(t.TempDir())

	fds, err := loadProtoSources(dir + "=" + filepath.Join(dir, "calc.proto"))
	if err != nil {
		t.Fatalf("absolute file with declared dir: %v", err)
	}
	if got := serviceNames(fds); len(got) != 1 || got[0] != "calc.Calculator" {
		t.Fatalf("services = %v, want [calc.Calculator]", got)
	}
}

// TestLoadProtoSources_NoDeclaredDirDoesNotSearchOtherDirs pins the other side
// of the fallback: without a dir= half there is no base to resolve against, so
// a file living only in another directory stays NOT_FOUND.
func TestLoadProtoSources_NoDeclaredDirDoesNotSearchOtherDirs(t *testing.T) {
	writeProtoDir(t, "calc.proto", calcProto("calc"))
	t.Chdir(t.TempDir())

	_, err := loadProtoSources("calc.proto")
	if err == nil {
		t.Fatal("loadProtoSources(calc.proto) = nil error, want NOT_FOUND")
	}
	if !apperrors.IsError(err, apperrors.CodeNotFound) {
		t.Fatalf("error = %v, want AppError CodeNotFound", err)
	}
}

// TestLoadProtoSources_ImportPathsAcrossDeclaredDirs proves the multi-dir form
// still serves imports: the requested file resolves against the first dir while
// its include is found in the second.
func TestLoadProtoSources_ImportPathsAcrossDeclaredDirs(t *testing.T) {
	const commonProto = `syntax = "proto3";
package shared;
message Money { int64 cents = 1; }
`
	const consumerProto = `syntax = "proto3";
package calc;
import "shared/common.proto";
service Calculator {
  rpc Add(AddRequest) returns (AddResponse);
}
message AddRequest { int32 a = 1; int32 b = 2; shared.Money amount = 3; }
message AddResponse { int32 sum = 1; }
`
	srcDir := writeProtoDir(t, "calc.proto", consumerProto)
	incDir := writeProtoDir(t, "shared/common.proto", commonProto)
	t.Chdir(t.TempDir())

	fds, err := loadProtoSources(srcDir + ":" + incDir + "=calc.proto")
	if err != nil {
		t.Fatalf("loadProtoSources(dir:incdir=calc.proto) = %v, want nil", err)
	}
	if got := serviceNames(fds); len(got) != 1 || got[0] != "calc.Calculator" {
		t.Fatalf("services = %v, want [calc.Calculator]", got)
	}
	// The include was linked, so the imported message resolved.
	field := fds[0].GetServices()[0].GetMethods()[0].GetInputType()
	if !strings.Contains(field.GetFullyQualifiedName(), "AddRequest") {
		t.Fatalf("input type = %q, want the AddRequest message", field.GetFullyQualifiedName())
	}
}

// TestDiscover_ProtoSourceWithDeclaredDir covers the same fix through the
// adapter seam (the surface `openapi-cli grpc discover` drives).
func TestDiscover_ProtoSourceWithDeclaredDir(t *testing.T) {
	dir := writeProtoDir(t, "calc.proto", calcProto("calc"))
	t.Chdir(t.TempDir())

	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	ops, err := adapter.Discover(context.Background(), dir+"=calc.proto")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("operations = %d, want 1", len(ops))
	}
	if ops[0].ID != "calc.Calculator-Add" {
		t.Fatalf("operation ID = %q, want calc.Calculator-Add", ops[0].ID)
	}
	if got := ops[0].Source[SourceFullMethod]; got != "/calc.Calculator/Add" {
		t.Fatalf("full method = %q, want /calc.Calculator/Add", got)
	}
}

// TestSplitImportPaths_WindowsVolumeIsNotASeparator pins the declared-dir
// splitter against Windows absolute paths. Before this, loadProtoSources
// split "C:\dirs\001=calc.proto" on every ':' — turning the single declared
// directory into "C" + "\dirs\001" — the all-dirs check failed, the '=' was
// never recognized and the whole URI was read as one file path, so every
// declared-dir form (dir=file.proto) failed on windows-latest.
func TestSplitImportPaths_WindowsVolumeIsNotASeparator(t *testing.T) {
	cases := []struct {
		name string
		list string
		want []string
	}{
		{"single unix dir", "/tmp/abc/001", []string{"/tmp/abc/001"}},
		{"two unix dirs", "/tmp/a:/tmp/b", []string{"/tmp/a", "/tmp/b"}},
		{"single windows dir", `C:\Users\runneradmin\AppData\Local\Temp\abc\001`,
			[]string{`C:\Users\runneradmin\AppData\Local\Temp\abc\001`}},
		{"forward-slash volume", "C:/Users/abc/001", []string{"C:/Users/abc/001"}},
		{"two windows dirs", `C:\a\001:C:\b\002`, []string{`C:\a\001`, `C:\b\002`}},
		{"mixed separator kinds", `dir1:C:\b\002`, []string{"dir1", `C:\b\002`}},
		{"lowercase drive", `c:\a\001:c:\b\002`, []string{`c:\a\001`, `c:\b\002`}},
		{"empty segment preserved", "a::b", []string{"a", "", "b"}},
		{"drive-rooted tail is exempt", `dir1:X:\b`, []string{"dir1", `X:\b`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitImportPaths(tc.list)
			if len(got) != len(tc.want) {
				t.Fatalf("splitImportPaths(%q) = %q, want %q", tc.list, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("splitImportPaths(%q) = %q, want %q", tc.list, got, tc.want)
				}
			}
		})
	}
}

// TestDiskAccessor_ReadersAreInMemory pins the handle-leak contract every
// reader loadProtoSources hands the parser must honour.
//
// protoparse's import-path pre-pass (parseToAST in jhump/protoreflect v1.18.1,
// reached because the parser sets InferImportPaths) reads the accessor's
// ReadCloser through parser.Parse and never closes it. Returning an *os.File
// therefore leaks one descriptor per parsed file for the life of the process,
// and on Windows the leaked handle also keeps the file undeletable: every
// grpc test whose fixture proto lives in a t.TempDir() failed with
// "TempDir RemoveAll cleanup: unlinkat ... wire.proto: The process cannot
// access the file because it is being used by another process". Memory-backed
// readers make the missing Close harmless.
func TestDiskAccessor_ReadersAreInMemory(t *testing.T) {
	dir := writeProtoDir(t, "calc.proto", calcProto("calc"))
	accessor := diskAccessor([]string{dir}, []string{"calc.proto"})

	// Both the verbatim declared-dir form and the absolute-path form.
	for _, name := range []string{"calc.proto", filepath.Join(dir, "calc.proto")} {
		rc, err := accessor(name)
		if err != nil {
			t.Fatalf("accessor(%q) = %v, want a reader", name, err)
		}
		if _, isFile := rc.(*os.File); isFile {
			t.Fatalf("accessor(%q) returned an *os.File: the parser never closes it, so every parse leaks a handle", name)
		}
		if _, err := io.ReadAll(rc); err != nil {
			t.Fatalf("read %q: %v", name, err)
		}
		_ = rc.Close()
	}
}
