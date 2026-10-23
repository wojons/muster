package request

import (
	"strings"
	"testing"
)

// MUSTER-001: query values must be percent-encoded. The previous raw
// concatenation produced an invalid request target for any value with a space
// ("?q=round trip"), which servers reject with 400 Bad Request — every filtered
// call from a generated CLI failed.
func TestBuildURL_EncodesQueryParameterValues(t *testing.T) {
	b := NewBuilder("http://127.0.0.1:3099")
	b.WithQueryParam("namespace", "muster-001")
	b.WithQueryParam("q", "round trip")
	b.WithQueryParam("contains", "a&b=c")

	got := b.buildURLFor("/api/memories")

	if strings.Contains(got, " ") {
		t.Fatalf("URL contains a raw space: %q", got)
	}
	if !strings.Contains(got, "q=round+trip") && !strings.Contains(got, "q=round%20trip") {
		t.Fatalf("q value not encoded: %q", got)
	}
	if !strings.Contains(got, "contains=a%26b%3Dc") {
		t.Fatalf("reserved characters not encoded: %q", got)
	}
	if !strings.Contains(got, "namespace=muster-001") {
		t.Fatalf("plain value mangled: %q", got)
	}
}

// Repeated keys stay repeated after encoding (DF-017).
func TestBuildURL_EncodesRepeatedQueryKeys(t *testing.T) {
	b := NewBuilder("http://example.test")
	b.WithQueryParamSlice("tags", []string{"a b", "c"})

	got := b.buildURLFor("/items")
	if !strings.Contains(got, "tags=a") || !strings.Contains(got, "tags=c") {
		t.Fatalf("repeated keys missing: %q", got)
	}
	if strings.Contains(got, " ") {
		t.Fatalf("URL contains a raw space: %q", got)
	}
}

// No query parameters → no trailing "?".
func TestBuildURL_NoQueryParams(t *testing.T) {
	b := NewBuilder("http://example.test")
	if got := b.buildURLFor("/items"); strings.Contains(got, "?") {
		t.Fatalf("unexpected query separator: %q", got)
	}
}
