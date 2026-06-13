package builtin

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/wojons/muster/pkg/dsl"
	"github.com/spf13/cobra"
	"go.starlark.net/starlark"
)

// newDslCommand creates the 'dsl' subcommand for running Starlark DSL scripts.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-1
func newDslCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dsl",
		Short: "Starlark DSL workflow commands",
		Long:  `Run Starlark DSL scripts for workflow chaining and API orchestration.`,
	}

	cmd.AddCommand(newDslRunCommand())

	return cmd
}

// newDslRunCommand creates the 'dsl run <file.star>' subcommand.
func newDslRunCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "run <file.star>",
		Short: "Execute a Starlark DSL script",
		Long: `Execute a Starlark DSL script that can chain HTTP API calls.

Available builtins:
  http_get(url)          — HTTP GET, returns response body as string
  http_post(url, body?)  — HTTP POST with optional JSON body, returns response body as string
  http_put(url, body?)   — HTTP PUT with optional JSON body, returns response body as string
  http_delete(url)       — HTTP DELETE, returns response body as string`,
		Args: cobra.ExactArgs(1),
		RunE: runDslScript,
	}
}

// runDslScript reads, parses, compiles, and executes a Starlark DSL script.
func runDslScript(cmd *cobra.Command, args []string) error {
	filename := args[0]

	src, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read script %s: %w", filename, err)
	}

	builtins := map[string]interface{}{
		"http_get":    httpGetBuiltin,
		"http_post":   httpPostBuiltin,
		"http_put":    httpPutBuiltin,
		"http_delete": httpDeleteBuiltin,
	}

	interp := dsl.NewInterpreter(builtins)

	ast, err := interp.Parse(string(src), filename)
	if err != nil {
		return fmt.Errorf("parse %s: %w", filename, err)
	}

	program, err := interp.Compile(ast)
	if err != nil {
		return fmt.Errorf("compile %s: %w", filename, err)
	}

	globals, err := interp.Exec(program)
	if err != nil {
		return fmt.Errorf("execute %s: %w", filename, err)
	}

	// Print any top-level string results
	for name, val := range globals {
		if s, ok := starlark.AsString(val); ok {
			fmt.Printf("%s = %s\n", name, s)
		} else {
			fmt.Printf("%s = %s\n", name, val.String())
		}
	}

	return nil
}

// ── HTTP builtins ────────────────────────────────────────────────────────────

// httpGetBuiltin performs an HTTP GET and returns the response body as a Starlark string.
func httpGetBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url string
	if err := starlark.UnpackArgs("http_get", args, kwargs, "url", &url); err != nil {
		return nil, err
	}
	return doHTTPRequest(http.MethodGet, url, "")
}

// httpPostBuiltin performs an HTTP POST with an optional JSON body.
func httpPostBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url, body string
	if err := starlark.UnpackArgs("http_post", args, kwargs, "url", &url, "body?", &body); err != nil {
		return nil, err
	}
	return doHTTPRequest(http.MethodPost, url, body)
}

// httpPutBuiltin performs an HTTP PUT with an optional JSON body.
func httpPutBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url, body string
	if err := starlark.UnpackArgs("http_put", args, kwargs, "url", &url, "body?", &body); err != nil {
		return nil, err
	}
	return doHTTPRequest(http.MethodPut, url, body)
}

// httpDeleteBuiltin performs an HTTP DELETE and returns the response body as a Starlark string.
func httpDeleteBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url string
	if err := starlark.UnpackArgs("http_delete", args, kwargs, "url", &url); err != nil {
		return nil, err
	}
	return doHTTPRequest(http.MethodDelete, url, "")
}

// doHTTPRequest executes an HTTP request and returns the response body as a Starlark string.
func doHTTPRequest(method, url, body string) (starlark.Value, error) {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("http %s %s: %w", method, url, err)
	}

	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response %s %s: %w", method, url, err)
	}

	return starlark.String(string(respBytes)), nil
}
