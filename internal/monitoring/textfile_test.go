package monitoring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPTextfileWriter_AtomicPublish(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "muster_mcp.prom")
	metrics := NewMusterMetrics("test", "v", "commit")
	metrics.RecordMCPTool("listPets", "success", "none", 0)
	writer := NewMCPTextfileWriter(metrics.MCPRegistry(), path)

	if err := writer.Write(); err != nil {
		t.Fatalf("first write: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read first output: %v", err)
	}
	text := string(before)
	for _, want := range []string{
		"# HELP muster_mcp_tool_executions_total",
		"# TYPE muster_mcp_tool_executions_total counter",
		"# TYPE muster_mcp_tool_duration_seconds histogram",
		"# TYPE muster_mcp_active_sessions gauge",
		`muster_mcp_tool_executions_total{error_class="none",product="muster",status="success",tool="listPets"} 1`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("first output lacks %q:\n%s", want, text)
		}
	}
	// The textfile carries ONLY the MCP families: a daemon_up=0 line here would
	// read as "the daemon is down" on a dashboard that never asked this binary.
	if strings.Contains(text, "muster_daemon_up") {
		t.Fatalf("textfile leaked daemon families:\n%s", text)
	}

	metrics.RecordMCPTool("listPets", "error", "validation", 0)
	if err := writer.Write(); err != nil {
		t.Fatalf("second write: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read second output: %v", err)
	}
	if string(after) == text {
		t.Fatal("destination was not replaced with updated metrics")
	}
	if !strings.Contains(string(after), `error_class="validation",product="muster",status="error",tool="listPets"} 1`) {
		t.Fatalf("second output lacks the error series:\n%s", after)
	}
	leftover, err := filepath.Glob(filepath.Join(dir, ".muster_mcp-*.prom.tmp"))
	if err != nil {
		t.Fatalf("glob temporary files: %v", err)
	}
	if len(leftover) != 0 {
		t.Fatalf("temporary files remain after publish: %v", leftover)
	}
}

func TestNormalizeMCPErrorClass_ClosedEnum(t *testing.T) {
	for _, class := range []string{"auth", "upstream", "validation", "internal"} {
		if got := NormalizeMCPErrorClass(class); got != class {
			t.Errorf("NormalizeMCPErrorClass(%q) = %q", class, got)
		}
	}
	if got := NormalizeMCPErrorClass(`raw upstream: secret=do-not-label`); got != "internal" {
		t.Fatalf("unknown error class = %q, want internal", got)
	}
}

func TestMCPTextfilePath_EnvPrecedence(t *testing.T) {
	t.Setenv(textfileDirEnv, "/tmp/collector")
	t.Setenv(mcpTextfilePathEnv, "")
	if got := MCPTextfilePath(); got != filepath.Join("/tmp/collector", "muster_mcp.prom") {
		t.Fatalf("dir-derived path = %q", got)
	}
	t.Setenv(mcpTextfilePathEnv, "/tmp/exact.prom")
	if got := MCPTextfilePath(); got != "/tmp/exact.prom" {
		t.Fatalf("exact override = %q", got)
	}
}

func TestCLITextfilePath_EnvPrecedence(t *testing.T) {
	t.Setenv(textfileDirEnv, "/tmp/collector")
	t.Setenv(cliTextfilePathEnv, "")
	if got := CLITextfilePath(); got != filepath.Join("/tmp/collector", "muster_cli.prom") {
		t.Fatalf("dir-derived path = %q", got)
	}
	t.Setenv(cliTextfilePathEnv, "/tmp/exact-cli.prom")
	if got := CLITextfilePath(); got != "/tmp/exact-cli.prom" {
		t.Fatalf("exact override = %q", got)
	}
}

func TestMCPTextfileWriter_NilRegistry(t *testing.T) {
	writer := NewMCPTextfileWriter(nil, filepath.Join(t.TempDir(), "metrics.prom"))
	if err := writer.Write(); err == nil {
		t.Fatal("nil registry write unexpectedly succeeded")
	}
}

func TestRecordMCPTool_SuccessCarriesNoErrorClass(t *testing.T) {
	metrics := NewMusterMetrics("test", "v", "commit")
	// A caller that passes a class on success must not create an error series:
	// "success" + error_class="auth" would answer "which tool is failing" wrong.
	metrics.RecordMCPTool("getPet", "success", "auth", 0)
	metrics.RecordMCPTool("getPet", "weird-status", "not-in-enum", 0)

	dir := t.TempDir()
	path := filepath.Join(dir, "muster_mcp.prom")
	if err := NewMCPTextfileWriter(metrics.MCPRegistry(), path).Write(); err != nil {
		t.Fatalf("write: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `error_class="none",product="muster",status="success",tool="getPet"} 1`) {
		t.Fatalf("success series not normalized to error_class=none:\n%s", text)
	}
	if !strings.Contains(text, `error_class="internal",product="muster",status="error",tool="getPet"} 1`) {
		t.Fatalf("unknown status/class not folded into the closed enum:\n%s", text)
	}
}
