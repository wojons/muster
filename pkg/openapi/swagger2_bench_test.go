package openapi

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// PERF-001 benchmarks.
//
// Both benchmarks run against one deterministic, production-size OpenAPI 3.x
// JSON document built in memory by productionSpec (about 14 MB, the size class
// of the GitHub v3 REST API description that motivated the row). The document
// is generated rather than committed so the benchmark stays hermetic and the
// repository does not carry a multi-megabyte fixture; its shape (root "openapi"
// first, ~1.2k operations, a components/schemas block referenced from every
// operation) mirrors the real spec closely enough for the costs under
// measurement — a JSON root-field probe and the kin-openapi document load.
//
// BenchmarkSwagger2RootFields measures exactly the version-detection path that
// used to decode the whole document into a map just to read two root scalars.
// BenchmarkDiscoverParse measures the in-process discover path (Parse +
// ExtractOperations), which is what `openapi-cli discover <spec>` spends its
// time on.

// productionSpecPathCount and productionSpecSchemaCount size the synthetic
// document. Each path carries one GET and one POST; each operation contributes
// roughly 10 KB of description text, so the totals below land the document at
// about 14 MB (asserted by TestProductionSpecSize).
const (
	productionSpecPathCount   = 700
	productionSpecSchemaCount = 160
	productionSpecPropsPerSch = 24
)

// productionSpecOnce builds the document at most once per benchmark process:
// generation is deterministic and its cost must never land inside a timed loop.
var productionSpecOnce = sync.OnceValue(buildProductionSpec)

// productionSpec returns the shared production-size 3.x JSON document.
func productionSpec() []byte {
	return productionSpecOnce()
}

// buildProductionSpec renders the synthetic production-size OpenAPI 3.x JSON
// document. It writes bytes directly (rather than marshaling maps) so the exact
// shape — including the root key ORDER with "openapi" first, as in the real
// GitHub spec — is pinned by the code that produces it.
func buildProductionSpec() []byte {
	var buf bytes.Buffer
	buf.Grow(15 << 20) // 15 MB: one growth-free pass for the whole document

	buf.WriteString(`{"openapi":"3.0.3","info":{"title":"Synthetic GitHub v3 REST API","version":"1.1.4","description":"`)
	buf.WriteString(strings.Repeat("Production-size OpenAPI description used to measure parse cost. ", 40))
	buf.WriteString(`"},"servers":[{"url":"https://api.github.com"}],"paths":{`)

	for p := 0; p < productionSpecPathCount; p++ {
		if p > 0 {
			buf.WriteByte(',')
		}
		fmt.Fprintf(&buf, `"/repos/{owner}/{repo}/synthetic-%d/{id}":{`, p)
		writeBenchOperation(&buf, p, "get", "Get synthetic resource %d")
		buf.WriteByte(',')
		writeBenchOperation(&buf, p, "post", "Create synthetic resource %d")
		buf.WriteByte('}')
	}

	buf.WriteString(`},"components":{"schemas":{`)
	for s := 0; s < productionSpecSchemaCount; s++ {
		if s > 0 {
			buf.WriteByte(',')
		}
		fmt.Fprintf(&buf, `"Model%d":{"type":"object","description":"`, s)
		buf.WriteString(strings.Repeat("Schema description text for the synthetic model. ", 12))
		buf.WriteString(`","properties":{`)
		for prop := 0; prop < productionSpecPropsPerSch; prop++ {
			if prop > 0 {
				buf.WriteByte(',')
			}
			fmt.Fprintf(&buf, `"field_%d":{"type":"string","description":"`, prop)
			buf.WriteString(strings.Repeat("Field description text. ", 8))
			buf.WriteString(`"}`)
		}
		buf.WriteString(`},"required":["field_0"]}`)
	}
	buf.WriteString(`}}}`)

	return buf.Bytes()
}

// writeBenchOperation writes one path item method. descFmt receives the path
// index so every operation gets a distinct operationId and summary.
func writeBenchOperation(buf *bytes.Buffer, path int, method, descFmt string) {
	fmt.Fprintf(buf, `"%s":{"operationId":"op%d%s","tags":["synthetic-%d"],"summary":"`, method, path, method, path%20)
	fmt.Fprintf(buf, descFmt, path)
	buf.WriteString(`","description":"`)
	buf.WriteString(strings.Repeat("Long operation description text used to reach production-size documents. ", 140))
	buf.WriteString(`","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}},`)
	buf.WriteString(`{"name":"per_page","in":"query","schema":{"type":"integer","default":30}}]`)
	if method == "post" {
		fmt.Fprintf(buf, `,"requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Model%d"}}}}`, path%productionSpecSchemaCount)
	}
	buf.WriteString(`,"responses":{"200":{"description":"OK","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Model`)
	fmt.Fprintf(buf, `%d`, path%productionSpecSchemaCount)
	buf.WriteString(`"}}}},"404":{"description":"Not Found"}}}`)
}

// TestProductionSpecSize pins the synthetic document at the production size
// class the PERF-001 numbers are quoted against: a one-line change to the
// generator must not quietly shrink (or balloon) the measured input.
func TestProductionSpecSize(t *testing.T) {
	const (
		minBytes = 12 << 20 // 12 MB
		maxBytes = 16 << 20 // 16 MB
	)
	doc := productionSpec()
	t.Logf("synthetic production spec: %d bytes", len(doc))
	if len(doc) < minBytes || len(doc) > maxBytes {
		t.Fatalf("synthetic production spec is %d bytes, want between %d and %d", len(doc), minBytes, maxBytes)
	}
	// Generation must be stable: a second call returns the same bytes.
	if !bytes.Equal(doc, productionSpec()) {
		t.Fatal("productionSpec is not deterministic")
	}
	// The document must be a 3.x document whose root key order puts "openapi"
	// first, matching the real GitHub spec the numbers came from.
	if !bytes.HasPrefix(doc, []byte(`{"openapi":"3.0.3"`)) {
		t.Fatalf("synthetic spec does not lead with the openapi root field: %.40q", doc)
	}
}

// BenchmarkSwagger2RootFields measures the Swagger 2.0 version probe
// (isSwagger2Document) on a production-size 3.x JSON document. This is the
// path PERF-001 rewrote: before the streaming scan it decoded the entire
// document into map[string]interface{} (about 46 MB and 116-128 ms per call on
// the 14 MB GitHub spec) to read two root scalars.
func BenchmarkSwagger2RootFields(b *testing.B) {
	doc := productionSpec()
	b.SetBytes(int64(len(doc)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if isSwagger2Document(doc) {
			b.Fatal("production-size 3.x document misdetected as Swagger 2.0")
		}
	}
}

// BenchmarkDiscoverParse measures the in-process discover path: Parse followed
// by ExtractOperations, on the same production-size document. This is the cost
// `openapi-cli discover <spec>` pays after reading the file, and it is where
// the duplicated whole-document decode used to appear.
func BenchmarkDiscoverParse(b *testing.B) {
	doc := productionSpec()
	b.SetBytes(int64(len(doc)))
	b.ReportAllocs()

	parser := NewParser()
	ctx := context.Background()
	opts := DefaultParseOptions()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := parser.Parse(ctx, doc, opts)
		if err != nil {
			b.Fatalf("parse: %v", err)
		}
		if _, err := parser.ExtractOperations(result.Document); err != nil {
			b.Fatalf("extract operations: %v", err)
		}
	}
}
