package auth

import "testing"

// MUSTER-001: every surface that attaches an operator credential (MCP server,
// daemon proxy) resolves the header through HeaderFor, so an api-key API gets
// X-API-Key and a bearer API gets Authorization: Bearer — one convention.
func TestHeaderFor(t *testing.T) {
	cases := []struct {
		name       string
		credType   string
		secret     string
		wantHeader string
		wantValue  string
	}{
		{"bearer", "bearer", "tok", "Authorization", "Bearer tok"},
		{"bearer uppercase", "BEARER", "tok", "Authorization", "Bearer tok"},
		{"api-key", "api-key", "tok", "X-API-Key", "tok"},
		{"api-key with spaces", " api-key ", "tok", "X-API-Key", "tok"},
		{"basic", "basic", "dXNlcjpwYXNz", "Authorization", "Basic dXNlcjpwYXNz"},
		{"unknown defaults to api-key header", "custom", "tok", "X-API-Key", "tok"},
		{"empty type defaults to api-key header", "", "tok", "X-API-Key", "tok"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			header, value := HeaderFor(tc.credType, tc.secret)
			if header != tc.wantHeader || value != tc.wantValue {
				t.Fatalf("HeaderFor(%q, %q) = (%q, %q), want (%q, %q)",
					tc.credType, tc.secret, header, value, tc.wantHeader, tc.wantValue)
			}
		})
	}
}
