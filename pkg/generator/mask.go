package generator

import (
	"regexp"
	"strings"
)

// DF-039: formatted output of generated typed commands (table/json/yaml)
// must not echo credential-shaped values — real harness APIs return
// session-scoped API keys (cs_XX_…) that scripts then capture into logs.
// Masking happens on the parsed response data BEFORE any formatted
// rendering; the --raw flag remains the explicit escape hatch that dumps
// the body untouched. This mirrors the DF-023 masking of credential HEADERS
// in internal/builtin/request.go, applied to response VALUES.

// bearerValueRe matches a "Bearer <token>" value so it renders as
// "Bearer ***" (same shape as maskAuthHeaderValue in
// internal/builtin/request.go).
var bearerValueRe = regexp.MustCompile(`(?i)bearer\s+\S+`)

// sessionKeyRe matches a session API key token: the cs_[a-z]{2}_ prefix
// followed by the rest of the token (no whitespace). Only the prefix is
// kept in the masked rendering so users can still tell which key class
// it was: cs_sk_9f2a… renders as cs_sk_***.
var sessionKeyRe = regexp.MustCompile(`cs_[a-z]{2}_\S*`)

// maskCredentialString masks credential-shaped values within a display
// string: session API keys (cs_[a-z]{2}_…) render as their 6-char prefix
// plus "***", and "Bearer <token>" renders as "Bearer ***". Non-credential
// strings are returned unchanged.
func maskCredentialString(s string) string {
	s = bearerValueRe.ReplaceAllStringFunc(s, func(m string) string {
		i := strings.IndexAny(m, " \t")
		if i < 0 {
			return m
		}
		return m[:i+1] + "***"
	})
	return sessionKeyRe.ReplaceAllStringFunc(s, func(m string) string {
		return m[:6] + "***"
	})
}

// maskCredentialValues walks parsed JSON data (maps, slices, strings) and
// masks credential-shaped string values in place. Non-string scalars and
// other types pass through unchanged.
func maskCredentialValues(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, val := range t {
			t[k] = maskCredentialValues(val)
		}
		return t
	case []interface{}:
		for i, val := range t {
			t[i] = maskCredentialValues(val)
		}
		return t
	case string:
		return maskCredentialString(t)
	default:
		return v
	}
}
