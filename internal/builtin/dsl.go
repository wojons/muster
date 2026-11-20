package builtin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wojons/muster/pkg/dsl"
	"go.starlark.net/starlark"
)

// bodyEncodingHint explains how to hand an HTTP builtin a request body. It is
// appended to every unsupported-body-type error so the message tells the user
// what to do instead of only what failed.
const bodyEncodingHint = "body accepts str (sent verbatim), dict, list, tuple, int, float, bool, or None " +
	"(all non-str values are marshalled to JSON); use json(value) to encode a value as a JSON string"

func newDslCommand() *cobra.Command {
	var verbose bool
	cmd := &cobra.Command{
		Use:   "dsl run <file.star>",
		Short: "Run a Starlark DSL script with HTTP builtins",
		Long: `Execute a .star Starlark script with built-in HTTP functions:
  http_get(url, headers=None) -> (status, body)
  http_post(url, body=None, headers=None) -> (status, body)
  http_put(url, body=None, headers=None) -> (status, body)
  http_delete(url, headers=None) -> (status, body)
  json(value) -> string

Body types accepted by http_post and http_put:
  str            sent verbatim (use for a pre-encoded payload)
  dict           marshalled to a JSON object:  {"title": "x"} -> {"title":"x"}
  list, tuple    marshalled to a JSON array
  int, float, bool, None   marshalled to the matching JSON scalar
  None/omitted   no body

Dict keys must be strings. Requests default to Content-Type: application/json;
pass a headers dict to override it.

headers is an optional dict of HTTP headers, e.g.
  http_post(url, body, {"Authorization": "Bearer " + token})

json(value) returns the JSON encoding of a Starlark value as a string:
  body = json({"title": "x"})   # '{"title":"x"}'
  http_post(url, body)`,
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
		"json":        jsonBuiltin,
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

// bodyBytesFromValue converts the optional Starlark body value of an HTTP
// builtin into request body bytes.
//
// A Starlark string is sent verbatim (pre-encoded payloads keep working
// byte-for-byte); any other JSON-encodable value — dict, list, tuple, int,
// float, bool — is marshalled to JSON; nil/None means "no body".
func bodyBytesFromValue(name string, v starlark.Value) ([]byte, error) {
	if v == nil || v == starlark.None {
		return nil, nil
	}
	if s, ok := v.(starlark.String); ok {
		return []byte(string(s)), nil
	}
	encoded, err := jsonEncodeValue(name, v)
	if err != nil {
		return nil, err
	}
	return []byte(encoded), nil
}

// jsonEncodeValue marshals a Starlark value to its JSON text. Errors are
// prefixed with the calling builtin and always end with bodyEncodingHint so a
// rejected body tells the user how to encode one.
func jsonEncodeValue(name string, v starlark.Value) (string, error) {
	goVal, err := starlarkToGoValue(v)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	encoded, err := json.Marshal(goVal)
	if err != nil {
		return "", fmt.Errorf("%s: encoding value as JSON: %w", name, err)
	}
	return string(encoded), nil
}

// starlarkToGoValue converts a Starlark value into the Go shape encoding/json
// expects. Only JSON-representable values are accepted; anything else returns
// an error carrying bodyEncodingHint.
func starlarkToGoValue(v starlark.Value) (interface{}, error) {
	switch v := v.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(v), nil
	case starlark.String:
		return string(v), nil
	case starlark.Int:
		// json.Number keeps arbitrarily large integers exact.
		return json.Number(v.String()), nil
	case starlark.Float:
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("float %s has no JSON representation", v.String())
		}
		return f, nil
	case *starlark.List:
		return starlarkIterableToGo(v)
	case starlark.Tuple:
		return starlarkIterableToGo(v)
	case *starlark.Dict:
		obj := make(map[string]interface{}, v.Len())
		for _, item := range v.Items() {
			key, ok := item[0].(starlark.String)
			if !ok {
				return nil, fmt.Errorf("dict key %s: got %s, want string (JSON object keys must be strings); %s",
					item[0].String(), item[0].Type(), bodyEncodingHint)
			}
			val, err := starlarkToGoValue(item[1])
			if err != nil {
				return nil, err
			}
			obj[string(key)] = val
		}
		return obj, nil
	default:
		return nil, fmt.Errorf("unsupported body type %s: %s", v.Type(), bodyEncodingHint)
	}
}

// starlarkIterableToGo converts a Starlark list or tuple into a Go slice.
func starlarkIterableToGo(v starlark.Iterable) ([]interface{}, error) {
	out := []interface{}{}
	it := v.Iterate()
	defer it.Done()
	var elem starlark.Value
	for it.Next(&elem) {
		goVal, err := starlarkToGoValue(elem)
		if err != nil {
			return nil, err
		}
		out = append(out, goVal)
	}
	return out, nil
}

// jsonBuiltin encodes a Starlark value as a JSON string. Strings are quoted
// (this is an encoder, not a pass-through): json("x") returns "\"x\"".
func jsonBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var value starlark.Value
	if err := starlark.UnpackArgs("json", args, kwargs, "value", &value); err != nil {
		return starlark.None, err
	}
	encoded, err := jsonEncodeValue("json", value)
	if err != nil {
		return starlark.None, err
	}
	return starlark.String(encoded), nil
}

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
	var url string
	var bodyVal, headersVal starlark.Value
	if err := starlark.UnpackArgs("http_post", args, kwargs, "url", &url, "body?", &bodyVal, "headers?", &headersVal); err != nil {
		return starlark.None, err
	}
	body, err := bodyBytesFromValue("http_post", bodyVal)
	if err != nil {
		return starlark.None, err
	}
	headers, err := headersFromValue("http_post", headersVal)
	if err != nil {
		return starlark.None, err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
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
	var url string
	var bodyVal, headersVal starlark.Value
	if err := starlark.UnpackArgs("http_put", args, kwargs, "url", &url, "body?", &bodyVal, "headers?", &headersVal); err != nil {
		return starlark.None, err
	}
	body, err := bodyBytesFromValue("http_put", bodyVal)
	if err != nil {
		return starlark.None, err
	}
	headers, err := headersFromValue("http_put", headersVal)
	if err != nil {
		return starlark.None, err
	}
	req, err := http.NewRequest("PUT", url, bytes.NewReader(body))
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
