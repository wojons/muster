package builtin

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/wojons/muster/pkg/errors"
)

// This file is the CLI half of PROTO-013's two gates: `grpc schema <uri>` is the
// user-facing proof that a service-less .proto is described as TYPES (0
// operations, 4 types), and the descriptor-set closure fix is visible as a
// working `grpc discover` on a set built without --include_imports plus a
// legible failure on one whose imports are unresolvable.

// grpcFixtureDir holds the committed .proto/.pb fixtures.
func grpcFixtureDir() string { return filepath.Join("..", "..", "pkg", "protocol", "grpc", "testdata") }

// runGrpcSchema runs `openapi-cli grpc schema ...` and returns stdout + error.
func runGrpcSchema(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out := &bytes.Buffer{}
	cmd := newGrpcSchemaCommand()
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// runGrpcDiscover runs `openapi-cli grpc discover ...`.
func runGrpcDiscover(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out := &bytes.Buffer{}
	cmd := newGrpcDiscoverCommand()
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// TestGrpcSchema_ServiceLessFileListsTypesAndZeroOperations: the ambiguity the
// row exists to remove — `discover` says "No operations found." (exit 0) and
// `schema` says FOUR TYPES for the same file.
func TestGrpcSchema_ServiceLessFileListsTypesAndZeroOperations(t *testing.T) {
	uri := filepath.Join(grpcFixtureDir(), "msgs_only.proto")

	out, err := runGrpcDiscover(t, uri)
	require.NoError(t, err, "a service-less file is not a failure")
	assert.Contains(t, out, "No operations found.")

	typesOut, err := runGrpcSchema(t, uri)
	require.NoError(t, err)
	assert.Contains(t, typesOut, "4 types", "the service-less fixture declares 4 types:\n%s", typesOut)
	for _, want := range []string{"only.Thing", "only.Other", "only.Third", "only.Flavor", "enum", "message"} {
		assert.Contains(t, typesOut, want, "schema listing must name %q:\n%s", want, typesOut)
	}
}

// TestGrpcSchema_JSONOutputIsMachineReadable: the schema verb's JSON surface.
func TestGrpcSchema_JSONOutputIsMachineReadable(t *testing.T) {
	uri := filepath.Join(grpcFixtureDir(), "schema.proto")

	out, err := runGrpcSchema(t, uri, "--output", "json")
	require.NoError(t, err)

	var set struct {
		Types map[string]struct {
			Kind   string `json:"kind"`
			Fields []struct {
				Name   string   `json:"name"`
				Ref    string   `json:"ref"`
				Format string   `json:"format"`
				Enum   []string `json:"enum"`
			} `json:"fields"`
		} `json:"types"`
		Source map[string]string `json:"source"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &set), "stdout must be JSON:\n%s", out)

	outer, ok := set.Types["schema.Outer"]
	require.True(t, ok, "schema.Outer must be present: %v", set.Types)
	assert.Equal(t, "message", outer.Kind)
	assert.Equal(t, "proto", set.Source["descriptors"])

	byName := map[string]string{}
	formats := map[string]string{}
	enums := map[string][]string{}
	for _, f := range outer.Fields {
		byName[f.Name] = f.Ref
		formats[f.Name] = f.Format
		enums[f.Name] = f.Enum
	}
	assert.Equal(t, "schema.Inner", byName["inner"], "nested field must carry its FQN reference")
	assert.Equal(t, "date-time", formats["created"], "a well-known type must carry its JSON form")
	assert.Equal(t, []string{"COLOR_UNSPECIFIED", "RED"}, enums["color"])

	if _, ok := set.Types["schema.Color"]; !ok {
		t.Errorf("schema.Color must be present as a type: %v", set.Types)
	}
	assert.Contains(t, out, `"oneof"`, "oneof membership must be visible in the JSON output")
}

// TestGrpcSchema_UnknownFormatIsRejected: the verb's format flag is explicit.
func TestGrpcSchema_UnknownFormatIsRejected(t *testing.T) {
	uri := filepath.Join(grpcFixtureDir(), "msgs_only.proto")
	_, err := runGrpcSchema(t, uri, "--output", "yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "text, json")
}

// TestGrpcDiscover_DescriptorSetWithoutImportsWorks: the closure fix as the
// operator sees it — the bare `--descriptor_set_out` artifact now discovers its
// operation instead of failing with a library error.
func TestGrpcDiscover_DescriptorSetWithoutImportsWorks(t *testing.T) {
	uri := filepath.Join(grpcFixtureDir(), "schema_nodeps.pb")

	out, err := runGrpcDiscover(t, uri)
	require.NoError(t, err, "a set without --include_imports must be closed from the runtime registry")
	assert.Contains(t, out, "schema.Desc-Do")

	withImports, err := runGrpcDiscover(t, filepath.Join(grpcFixtureDir(), "schema_all.pb"))
	require.NoError(t, err)
	assert.Equal(t, withImports, out, "the two sets must discover identically")
}

// TestGrpcDiscover_UnresolvableDependencyIsLegible: the fail-early branch must
// name the missing file AND the remedy — the whole point of the fix, since the
// previous message named the file but offered no way out.
func TestGrpcDiscover_UnresolvableDependencyIsLegible(t *testing.T) {
	uri := filepath.Join(grpcFixtureDir(), "main_nodeps.pb")

	_, err := runGrpcDiscover(t, uri)
	require.Error(t, err)
	assert.True(t, apperrors.IsError(err, apperrors.CodeValidation), "error class = %v", err)
	msg := err.Error()
	assert.Contains(t, msg, "dep.proto", "the missing file must be named")
	assert.Contains(t, msg, "--include_imports", "the remedy must be named")
	assert.True(t, strings.Contains(msg, "not self-contained") || strings.Contains(msg, "imports"),
		"the message must say what is wrong: %s", msg)
}

// TestGrpcSchema_IsMountedUnderTheGrpcGroup: §5.1 — no new binary, the verb
// mounts under the existing protocol group.
func TestGrpcSchema_IsMountedUnderTheGrpcGroup(t *testing.T) {
	group := newGrpcCommand()
	names := map[string]bool{}
	for _, c := range group.Commands() {
		names[c.Name()] = true
	}
	assert.True(t, names["schema"], "grpc schema must be mounted: %v", names)
	assert.True(t, names["discover"] && names["call"], "the existing verbs stay: %v", names)
	assert.Contains(t, group.Long, "grpc schema", "the group help must describe the schema verb")
}
