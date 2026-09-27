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
	var verbose bool
	cmd := &cobra.Command{
		Use:   "dsl run <file.star>",
		Short: "Run a Starlark DSL script with HTTP builtins",
		Long: `Execute a .star Starlark script with built-in HTTP functions:
  http_get(url, headers=None) -> (status, body)
  http_post(url, body, headers=None) -> (status, body)
  http_put(url, body, headers=None) -> (status, body)
  http_delete(url, headers=None) -> (status, body)

headers is an optional dict of HTTP headers, e.g.
  http_post(url, body, {"Authorization": "Bearer " + token})`,
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDsl(cmd, args, verbose)
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "V", false, "Print all global variable bindings after script execution")
	return cmd
}

func runDsl(cmd *cobra.Command, args []string, verbose bool) error {
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

	// Route print() output to stdout (script mode) instead of the structured
	// log sink, so script output appears as plain text on stdout.
	interp := dsl.NewInterpreterWithPrint(builtins, func(_ *starlark.Thread, msg string) {
		fmt.Println(msg)
	})

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

	// Only dump global bindings when --verbose is set; the default (script
	// mode) shows only intended print() output.
	if verbose {
		for name, val := range globals {
			fmt.Printf("%s = %s\n", name, val.String())
		}
	}

	return nil
}

// --- HTTP builtins ---

// headersFromValue converts an optional Starlark dict of header name/value
// string pairs into an http.Header. Returns nil for starlark.None (or a nil
// value). Non-dict values produce an unpack error naming the builtin.
func headersFromValue(name string, v starlark.Value) (http.Header, error) {
	if v == nil || v == starlark.None {
		return nil, nil
	}
	d, ok := v.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("%s: headers: got %s, want dict", name, v.Type())
	}
	headers := make(http.Header, d.Len())
	for _, item := range d.Items() {
		k, ok := starlark.AsString(item[0])
		if !ok {
			return nil, fmt.Errorf("%s: headers: key %s: got %s, want string", name, item[0].String(), item[0].Type())
		}
		val, ok := starlark.AsString(item[1])
		if !ok {
			return nil, fmt.Errorf("%s: headers[%q]: got %s, want string", name, k, item[1].Type())
		}
		headers.Set(k, val)
	}
	return headers, nil
}

func httpGetBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url string
	var headersVal starlark.Value
	if err := starlark.UnpackArgs("http_get", args, kwargs, "url", &url, "headers?", &headersVal); err != nil {
		return starlark.None, err
	}
	headers, err := headersFromValue("http_get", headersVal)
	if err != nil {
		return starlark.None, err
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return starlark.None, fmt.Errorf("http_get %s: %w", url, err)
	}
	for k, vals := range headers {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return starlark.None, fmt.Errorf("http_get %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return starlark.Tuple{starlark.MakeInt(int(resp.StatusCode)), starlark.String(strings.TrimSpace(string(body)))}, nil
}

func httpPostBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url, body string
	var headersVal starlark.Value
	if err := starlark.UnpackArgs("http_post", args, kwargs, "url", &url, "body?", &body, "headers?", &headersVal); err != nil {
		return starlark.None, err
	}
	headers, err := headersFromValue("http_post", headersVal)
	if err != nil {
		return starlark.None, err
	}
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return starlark.None, fmt.Errorf("http_post %s: %w", url, err)
	}
	// Default first, then caller headers, so an explicit Content-Type wins.
	req.Header.Set("Content-Type", "application/json")
	for k, vals := range headers {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return starlark.None, fmt.Errorf("http_post %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	return starlark.Tuple{starlark.MakeInt(int(resp.StatusCode)), starlark.String(strings.TrimSpace(string(respBody)))}, nil
}

func httpPutBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url, body string
	var headersVal starlark.Value
	if err := starlark.UnpackArgs("http_put", args, kwargs, "url", &url, "body?", &body, "headers?", &headersVal); err != nil {
		return starlark.None, err
	}
	headers, err := headersFromValue("http_put", headersVal)
	if err != nil {
		return starlark.None, err
	}
	req, err := http.NewRequest("PUT", url, strings.NewReader(body))
	if err != nil {
		return starlark.None, fmt.Errorf("http_put %s: %w", url, err)
	}
	// Default first, then caller headers, so an explicit Content-Type wins.
	req.Header.Set("Content-Type", "application/json")
	for k, vals := range headers {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return starlark.None, fmt.Errorf("http_put %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	return starlark.Tuple{starlark.MakeInt(int(resp.StatusCode)), starlark.String(strings.TrimSpace(string(respBody)))}, nil
}

func httpDeleteBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url string
	var headersVal starlark.Value
	if err := starlark.UnpackArgs("http_delete", args, kwargs, "url", &url, "headers?", &headersVal); err != nil {
		return starlark.None, err
	}
	headers, err := headersFromValue("http_delete", headersVal)
	if err != nil {
		return starlark.None, err
	}
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return starlark.None, fmt.Errorf("http_delete %s: %w", url, err)
	}
	for k, vals := range headers {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return starlark.None, fmt.Errorf("http_delete %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return starlark.Tuple{starlark.MakeInt(int(resp.StatusCode)), starlark.String(strings.TrimSpace(string(body)))}, nil
}
