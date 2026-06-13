package builtin

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wojons/muster/pkg/dsl"
	"go.starlark.net/starlark"
)

func newDslCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "dsl run <file.star>",
		Short: "Run a Starlark DSL script with HTTP builtins",
		Long: `Execute a .star Starlark script with built-in HTTP functions:
  http_get(url) -> response body string
  http_post(url, body) -> response body string
  http_put(url, body) -> response body string
  http_delete(url) -> response body string`,
		Args: cobra.ExactArgs(2),
		RunE: runDsl,
	}
}

func runDsl(cmd *cobra.Command, args []string) error {
	filePath := args[1]

	src, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading script: %w", err)
	}

	builtins := map[string]interface{}{
		"http_get":    httpGetBuiltin,
		"http_post":   httpPostBuiltin,
		"http_put":    httpPutBuiltin,
		"http_delete": httpDeleteBuiltin,
	}

	interp := dsl.NewInterpreter(builtins)

	ast, err := interp.Parse(string(src), filePath)
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	prog, err := interp.Compile(ast)
	if err != nil {
		return fmt.Errorf("compile error: %w", err)
	}

	globals, err := interp.Exec(prog)
	if err != nil {
		return fmt.Errorf("execution error: %w", err)
	}

	for name, val := range globals {
		fmt.Printf("%s = %s\n", name, val.String())
	}

	return nil
}

// --- HTTP builtins ---

func httpGetBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url string
	if err := starlark.UnpackArgs("http_get", args, kwargs, "url", &url); err != nil {
		return starlark.None, err
	}
	resp, err := http.Get(url)
	if err != nil {
		return starlark.None, fmt.Errorf("http_get %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return starlark.Tuple{starlark.MakeInt(int(resp.StatusCode)), starlark.String(strings.TrimSpace(string(body)))}, nil
}

func httpPostBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url, body string
	if err := starlark.UnpackArgs("http_post", args, kwargs, "url", &url, "body?", &body); err != nil {
		return starlark.None, err
	}
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		return starlark.None, fmt.Errorf("http_post %s: %w", url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return starlark.Tuple{starlark.MakeInt(int(resp.StatusCode)), starlark.String(strings.TrimSpace(string(respBody)))}, nil
}

func httpPutBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url, body string
	if err := starlark.UnpackArgs("http_put", args, kwargs, "url", &url, "body?", &body); err != nil {
		return starlark.None, err
	}
	req, err := http.NewRequest("PUT", url, strings.NewReader(body))
	if err != nil {
		return starlark.None, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return starlark.None, fmt.Errorf("http_put %s: %w", url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return starlark.Tuple{starlark.MakeInt(int(resp.StatusCode)), starlark.String(strings.TrimSpace(string(respBody)))}, nil
}

func httpDeleteBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url string
	if err := starlark.UnpackArgs("http_delete", args, kwargs, "url", &url); err != nil {
		return starlark.None, err
	}
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return starlark.None, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return starlark.None, fmt.Errorf("http_delete %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return starlark.Tuple{starlark.MakeInt(int(resp.StatusCode)), starlark.String(strings.TrimSpace(string(body)))}, nil
}
