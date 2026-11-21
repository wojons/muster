// Package request: form-encoded body encoding (QA-MUSTER-041).
//
// Operations that declare `application/x-www-form-urlencoded` request bodies
// (611 of the 612 operations in Stripe's spec3) used to be generated as
// argument-free commands: the parser only read `application/json`, no body
// flags were created, and the request went out with Content-Length: 0. The
// helpers here convert the same body-map shape the JSON path already builds
// into urlencoded values, so the CLI, the persisted-command loader and the MCP
// executor all encode form bodies identically.
package request

import (
	"fmt"
	neturl "net/url"
	"sort"
	"strings"
)

// FormValuesFromMap converts a body map into urlencoded form values.
//
// Encoding rules (chosen to match what form-encoded APIs such as Stripe,
// Rails and PHP expect):
//   - string / number / bool → one key with fmt.Sprintf("%v")
//   - slice                  → the key repeated once per element (?tag=a&tag=b)
//   - nested map             → bracket notation (metadata[foo]=bar)
//   - nil                    → key omitted (the caller did not supply a value)
func FormValuesFromMap(body map[string]interface{}) neturl.Values {
	values := neturl.Values{}
	if body == nil {
		return values
	}
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		appendFormValue(values, key, body[key])
	}
	return values
}

func appendFormValue(values neturl.Values, key string, value interface{}) {
	switch v := value.(type) {
	case nil:
		return
	case []string:
		for _, item := range v {
			values.Add(key, item)
		}
	case []interface{}:
		for _, item := range v {
			if item == nil {
				continue
			}
			values.Add(key, fmt.Sprintf("%v", item))
		}
	case map[string]interface{}:
		// Sorted for deterministic bodies: form APIs accept any order, but
		// tests (and humans reading --body) should not depend on map order.
		inner := make([]string, 0, len(v))
		for k := range v {
			inner = append(inner, k)
		}
		sort.Strings(inner)
		for _, k := range inner {
			appendFormValue(values, key+"["+k+"]", v[k])
		}
	default:
		values.Add(key, fmt.Sprintf("%v", v))
	}
}

// ParseFormBody validates and normalizes a raw urlencoded body string, as
// supplied through the -b/--body escape hatch. A body with no pairs is
// rejected: silently sending an empty body is the defect this exists to fix.
// Every segment must be a real key=value pair, so a JSON payload pasted by
// mistake fails loudly instead of being sent as one enormous form key.
func ParseFormBody(raw string) (neturl.Values, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("empty form body: expected key=value&key2=value2")
	}
	for _, segment := range strings.Split(trimmed, "&") {
		if segment == "" {
			continue
		}
		key, _, found := strings.Cut(segment, "=")
		if !found || key == "" {
			return nil, fmt.Errorf("invalid urlencoded form body: segment %q is not key=value (value: %q)", segment, raw)
		}
	}
	values, err := neturl.ParseQuery(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid urlencoded form body: %w (value: %q)", err, raw)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("invalid urlencoded form body: no key=value pairs (value: %q)", raw)
	}
	return values, nil
}
