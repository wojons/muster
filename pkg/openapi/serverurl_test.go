package openapi

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestResolveRelativeServerURL(t *testing.T) {
	const src = "https://host.example/api/v3/openapi.json"

	t.Run("resolves root-relative server against spec fetch URL", func(t *testing.T) {
		// '/api/v3' fetched from https://host/api/v3/openapi.json →
		// https://host/api/v3 (the DF-015 acceptance example).
		require.Equal(t, "https://host.example/api/v3",
			ResolveRelativeServerURL("/api/v3", src))
	})

	t.Run("root-relative reference replaces the whole path (RFC 3986)", func(t *testing.T) {
		require.Equal(t, "https://host.example/v2",
			ResolveRelativeServerURL("/v2", src))
	})

	t.Run("dir-relative reference resolves against the base dir (RFC 3986)", func(t *testing.T) {
		// A relative reference without a leading slash resolves against the
		// base path's directory: /api/v3/openapi.json + v2 → /api/v3/v2.
		require.Equal(t, "https://host.example/api/v3/v2",
			ResolveRelativeServerURL("v2", src))
	})

	t.Run("absolute server URL passes through untouched", func(t *testing.T) {
		require.Equal(t, "https://other.example/x",
			ResolveRelativeServerURL("https://other.example/x", src))
		require.Equal(t, "http://localhost:9801",
			ResolveRelativeServerURL("http://localhost:9801", src))
	})

	t.Run("empty source URL (local file) keeps relative behavior", func(t *testing.T) {
		require.Equal(t, "/api/v3", ResolveRelativeServerURL("/api/v3", ""))
	})

	t.Run("non-http source URL keeps relative behavior", func(t *testing.T) {
		require.Equal(t, "/api/v3", ResolveRelativeServerURL("/api/v3", "file:///tmp/spec.json"))
	})

	t.Run("unparseable server URL returns input unchanged", func(t *testing.T) {
		require.Equal(t, "://bad", ResolveRelativeServerURL("://bad", src))
	})

	t.Run("empty server URL passes through", func(t *testing.T) {
		require.Equal(t, "", ResolveRelativeServerURL("", src))
	})
}

func TestApplySourceURLToDocument(t *testing.T) {
	const src = "https://host.example/api/openapi.json"

	newDoc := func(t *testing.T, specJSON string) *openapi3.T {
		t.Helper()
		doc, err := openapi3.NewLoader().LoadFromData([]byte(specJSON))
		require.NoError(t, err)
		return doc
	}

	t.Run("resolves root-level servers", func(t *testing.T) {
		doc := newDoc(t, `{"openapi":"3.0.0","info":{"title":"t","version":"1"},
			"servers":[{"url":"/api"},{"url":"https://abs.example"}],
			"paths":{"/x":{"get":{"responses":{"200":{"description":"ok"}}}}}}`)
		ApplySourceURLToDocument(doc, src)
		require.Equal(t, "https://host.example/api", doc.Servers[0].URL)
		require.Equal(t, "https://abs.example", doc.Servers[1].URL)
	})

	t.Run("resolves path-item and operation-level servers", func(t *testing.T) {
		doc := newDoc(t, `{"openapi":"3.0.0","info":{"title":"t","version":"1"},
			"paths":{
				"/x":{"servers":[{"url":"/path-srv"}],
					"get":{"responses":{"200":{"description":"ok"}}}},
				"/y":{"get":{"servers":[{"url":"/op-srv"}],
					"responses":{"200":{"description":"ok"}}}}
			}}`)
		ApplySourceURLToDocument(doc, src)
		require.Equal(t, "https://host.example/path-srv", doc.Paths.Find("/x").Servers[0].URL)
		op := doc.Paths.Find("/y").GetOperation("GET")
		require.NotNil(t, op.Servers)
		require.Equal(t, "https://host.example/op-srv", (*op.Servers)[0].URL)
	})

	t.Run("nil document and empty source are no-ops", func(t *testing.T) {
		ApplySourceURLToDocument(nil, src) // must not panic
		doc := newDoc(t, `{"openapi":"3.0.0","info":{"title":"t","version":"1"},
			"servers":[{"url":"/api"}],"paths":{}}`)
		ApplySourceURLToDocument(doc, "")
		require.Equal(t, "/api", doc.Servers[0].URL)
	})

	t.Run("strips fragment inherited from the spec source URL", func(t *testing.T) {
		doc := newDoc(t, `{"openapi":"3.0.0","info":{"title":"t","version":"1"},
			"servers":[{"url":"/api"}],"paths":{}}`)
		ApplySourceURLToDocument(doc, "https://host.example/api/openapi.json#section")
		require.Equal(t, "https://host.example/api", doc.Servers[0].URL)
		require.NotContains(t, doc.Servers[0].URL, "#")
	})
}

func TestHasRelativeServerURL(t *testing.T) {
	newDoc := func(t *testing.T, specJSON string) *openapi3.T {
		t.Helper()
		doc, err := openapi3.NewLoader().LoadFromData([]byte(specJSON))
		require.NoError(t, err)
		return doc
	}

	t.Run("true for relative root server", func(t *testing.T) {
		doc := newDoc(t, `{"openapi":"3.0.0","info":{"title":"t","version":"1"},
			"servers":[{"url":"/api"}],"paths":{}}`)
		require.True(t, HasRelativeServerURL(doc))
	})

	t.Run("false for absolute servers", func(t *testing.T) {
		doc := newDoc(t, `{"openapi":"3.0.0","info":{"title":"t","version":"1"},
			"servers":[{"url":"https://abs.example"}],"paths":{}}`)
		require.False(t, HasRelativeServerURL(doc))
	})

	t.Run("true for relative operation-level server", func(t *testing.T) {
		doc := newDoc(t, `{"openapi":"3.0.0","info":{"title":"t","version":"1"},
			"paths":{"/x":{"get":{"servers":[{"url":"/op"}],
				"responses":{"200":{"description":"ok"}}}}}}`)
		require.True(t, HasRelativeServerURL(doc))
	})

	t.Run("false for nil or serverless docs", func(t *testing.T) {
		require.False(t, HasRelativeServerURL(nil))
		doc := newDoc(t, `{"openapi":"3.0.0","info":{"title":"t","version":"1"},"paths":{}}`)
		require.False(t, HasRelativeServerURL(doc))
	})
}
