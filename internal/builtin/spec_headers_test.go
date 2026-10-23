package builtin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// MUSTER-001: authenticated spec endpoints (DuckBrain's namespace spec route is
// x-api-key gated) need request headers on the spec fetch, and a malformed
// header must fail loudly instead of being silently dropped.
func TestFetchSpec_SendsConfiguredHeaders(t *testing.T) {
	var gotKey, gotAccept string
	spec := `{"openapi":"3.1.0","info":{"title":"probe","version":"1.0.0"},"paths":{"/ping":{"get":{"operationId":"ping","responses":{"200":{"description":"ok"}}}}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		gotAccept = r.Header.Get("accept")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(spec))
	}))
	defer srv.Close()

	data, err := fetchSpec(srv.URL, []string{"x-api-key: s3cret", "accept: application/json"})
	if err != nil {
		t.Fatalf("fetchSpec: %v", err)
	}
	if gotKey != "s3cret" {
		t.Errorf("x-api-key = %q, want s3cret", gotKey)
	}
	if gotAccept != "application/json" {
		t.Errorf("accept = %q", gotAccept)
	}
	if !strings.Contains(string(data), `"openapi"`) {
		t.Errorf("spec body not returned: %s", data)
	}
}

func TestFetchSpec_RejectsMalformedHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream must not be contacted when a header is malformed")
	}))
	defer srv.Close()

	if _, err := fetchSpec(srv.URL, []string{"no-colon-here"}); err == nil {
		t.Fatal("fetchSpec accepted a header without a colon")
	}
}

// An unauthenticated fetch still fails with the upstream status — the header
// support must not swallow a 401.
func TestFetchSpec_SurfacesUpstreamStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := fetchSpec(srv.URL, nil); err == nil {
		t.Fatal("fetchSpec returned success for a 401")
	} else if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error should name the upstream status, got: %v", err)
	}
}
