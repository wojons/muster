package request

import (
	"fmt"
	"net/url"

	"github.com/wojons/muster/pkg/openapi"
)

// ValidateRequestBaseURL rejects base URLs that cannot participate in an
// HTTP request: empty, or relative (e.g. "/api/v3" from a spec declaring a
// relative servers[].url). Wire this into every generated/persisted command
// run handler so a bare `unsupported protocol scheme ""` http error never
// reaches the user; the returned error names the two escape hatches
// (--base-url flag, MUSTER_BASE_URL env var) documented in the README
// quickstart (DF-015).
//
// A non-empty, non-relative value passes unchanged — user-supplied override
// bases (https://staging.example.com) and already-resolved spec bases are
// never second-guessed here.
func ValidateRequestBaseURL(baseURL string) error {
	if baseURL == "" {
		return fmt.Errorf("no base URL available: %s", openapi.ActionableRelativeBaseURLError)
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" {
		return fmt.Errorf("base URL %q is relative: %s", baseURL, openapi.ActionableRelativeBaseURLError)
	}
	return nil
}
