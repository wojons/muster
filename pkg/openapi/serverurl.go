package openapi

import (
	"net/url"

	"github.com/getkin/kin-openapi/openapi3"
)

// isAbsoluteHTTPURL reports whether s parses as an absolute http(s) URL.
// The predicate mirrors pkg/mcp's DF-MUSTER-1 check (url.Parse + Scheme),
// so every surface grades "usable as a request base" the same way.
func isAbsoluteHTTPURL(s string) bool {
	if s == "" {
		return false
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// ResolveRelativeServerURL resolves a single server URL that is relative
// (e.g. "/api/v3") against the URL the spec itself was fetched from, per
// RFC 3986 and the OpenAPI specification (Server Object: "relative URLs…
// are resolved against the hosting server"). A relative spec fetched from
// https://host/api/v3/openapi.json with servers[0].url=/api therefore
// targets https://host/api. An already-absolute server URL, an empty
// source URL (local file — no base to resolve against), a non-http(s)
// source, or a resolution failure all return the input unchanged, so
// local-file specs keep their current behavior.
func ResolveRelativeServerURL(serverURL, specSourceURL string) string {
	if serverURL == "" || isAbsoluteHTTPURL(serverURL) {
		return serverURL
	}
	if !isAbsoluteHTTPURL(specSourceURL) {
		return serverURL
	}
	// url.Parse cannot fail here: both sides passed isAbsoluteHTTPURL /
	// are empty-checked above.
	base, _ := url.Parse(specSourceURL)
	ref, err := url.Parse(serverURL)
	if err != nil {
		return serverURL
	}
	resolved := base.ResolveReference(ref)
	resolved.Fragment = ""
	s := resolved.String()
	if s == "" {
		return serverURL
	}
	return s
}

// ApplySourceURLToDocument resolves every RELATIVE server URL declared in
// the document — root, path-item, and operation level — against the URL
// the spec was fetched from (DF-015). Absolute server URLs and local-file
// sources (empty specSourceURL) are left untouched. Mutates doc in place;
// call before operation extraction so every downstream consumer (generator
// per-operation ServerURL, persisted metadata BaseURL, MCP tool BaseURL)
// sees the resolved value.
func ApplySourceURLToDocument(doc *openapi3.T, specSourceURL string) {
	if doc == nil || specSourceURL == "" || !isAbsoluteHTTPURL(specSourceURL) {
		return
	}
	for _, server := range doc.Servers {
		if server != nil {
			server.URL = ResolveRelativeServerURL(server.URL, specSourceURL)
		}
	}
	if doc.Paths != nil {
		for _, pathItem := range doc.Paths.Map() {
			if pathItem == nil {
				continue
			}
			for _, server := range pathItem.Servers {
				if server != nil {
					server.URL = ResolveRelativeServerURL(server.URL, specSourceURL)
				}
			}
			for _, op := range pathItem.Operations() {
				if op == nil || op.Servers == nil {
					continue
				}
				for _, server := range *op.Servers {
					if server != nil {
						server.URL = ResolveRelativeServerURL(server.URL, specSourceURL)
					}
				}
			}
		}
	}
}

// HasRelativeServerURL reports whether the document declares at least one
// RELATIVE (non-absolute-http) server URL at root, path-item, or operation
// level. The pkg/generator and pkg/cmdstore run handlers use it to decide
// whether a relative base URL reached execution because the spec itself
// declared one (actionable hint) — as opposed to a user-supplied override,
// which is passed through as-is.
func HasRelativeServerURL(doc *openapi3.T) bool {
	if doc == nil {
		return false
	}
	check := func(u string) bool {
		return u != "" && !isAbsoluteHTTPURL(u)
	}
	for _, server := range doc.Servers {
		if server != nil && check(server.URL) {
			return true
		}
	}
	if doc.Paths != nil {
		for _, pathItem := range doc.Paths.Map() {
			if pathItem == nil {
				continue
			}
			for _, server := range pathItem.Servers {
				if server != nil && check(server.URL) {
					return true
				}
			}
			for _, op := range pathItem.Operations() {
				if op == nil || op.Servers == nil {
					continue
				}
				for _, server := range *op.Servers {
					if server != nil && check(server.URL) {
						return true
					}
				}
			}
		}
	}
	return false
}

// ActionableRelativeBaseURLError is the hint attached to run-handler errors
// when a command still carries an empty or relative base URL (DF-015). The
// escape hatches it names are the two documented overrides.
const ActionableRelativeBaseURLError = "spec declares a relative server URL; pass --base-url (or set MUSTER_BASE_URL)"
