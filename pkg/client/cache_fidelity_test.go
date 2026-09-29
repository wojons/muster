package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/cache"
)

// fixtureRoundTripper serves one canned response per call. The response is
// built fresh for every call so a cell cannot pass because an earlier read
// mutated the fixture, and the call count is the proof a hit never reached it.
type fixtureRoundTripper struct {
	calls    int
	response func() *http.Response
}

func (f *fixtureRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	f.calls++
	return f.response(), nil
}

// hopByHopFixtureNames are the connection-scoped headers the fixture response
// carries. None of them may reach a cached entry (RFC 7230 §6.1, RFC 7234 §3).
var hopByHopFixtureNames = []string{"Connection", "Keep-Alive", "Transfer-Encoding", "X-Connection-Scoped"}

// TestCacheRoundTripper_CacheHitPreservesResponseFidelity is the DF-041 cell: a
// cached 2xx GET must be served the response the exchange produced — its
// protocol version, its status and its end-to-end headers — instead of being
// rebuilt as HTTP/1.1 with a lone Content-Type. The connection-scoped headers
// are the other half of the contract: they describe the connection the response
// arrived on, so they are stripped from the stored copy rather than replayed
// (RFC 7234 §3).
func TestCacheRoundTripper_CacheHitPreservesResponseFidelity(t *testing.T) {
	const (
		requestURL = "http://example.com/api/fidelity"
		body       = `{"data":"x"}`
		date       = "Mon, 29 Sep 2026 12:00:00 GMT"
	)

	for _, tc := range []struct {
		name   string
		status string
		code   int
	}{
		{"200 OK", "200 OK", http.StatusOK},
		// A non-200 2xx status: caching it is today's behaviour, so the status
		// the entry serves must be the one it was stored with, not "200 OK".
		{"non-200 2xx", "207 Multi-Status", http.StatusMultiStatus},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inner := &fixtureRoundTripper{response: func() *http.Response {
				return &http.Response{
					Status:     tc.status,
					StatusCode: tc.code,
					// The wire version of an HTTP/2 exchange. The cache must
					// report this, never a default of HTTP/1.1 (DF-041).
					Proto:      WireVersionHTTP2,
					ProtoMajor: 2,
					ProtoMinor: 0,
					Header: http.Header{
						"Content-Type":        {"application/json"},
						"X-Foo":               {"bar"},
						"ETag":                {`"tag-1"`},
						"Link":                {`</next>; rel="next"`},
						"Date":                {date},
						"Set-Cookie":          {"a=1; Path=/", "b=2; Path=/"},
						"Connection":          {"close, X-Connection-Scoped"},
						"Keep-Alive":          {"timeout=5"},
						"Transfer-Encoding":   {"chunked"},
						"X-Connection-Scoped": {"named by Connection, so it is connection-scoped too"},
						// Deliberately not the stored body's length: the entry's
						// own bookkeeping must win.
						"Content-Length": {"999"},
					},
					Body:          io.NopCloser(strings.NewReader(body)),
					ContentLength: int64(len(body)),
				}
			}}

			store := cache.NewMemoryCache(100)
			t.Cleanup(func() { _ = store.Close() })
			transport := NewCacheRoundTripper(inner, store, time.Minute)

			// The miss is served by the exchange itself: the response the inner
			// transport returned, with its headers and version intact. A miss is
			// not a reason to hand the caller less than the wire delivered, and
			// the hop-by-hop headers are only dropped from the stored copy.
			miss, err := transport.RoundTrip(httptest.NewRequest(http.MethodGet, requestURL, nil))
			require.NoError(t, err)
			missBody, err := io.ReadAll(miss.Body)
			require.NoError(t, err)
			require.NoError(t, miss.Body.Close())
			assert.Equal(t, body, string(missBody))
			assert.Equal(t, WireVersionHTTP2, miss.Proto, "a miss is the wire response")
			assert.Equal(t, "bar", miss.Header.Get("X-Foo"), "a miss is the wire response")
			assert.Equal(t, "close, X-Connection-Scoped", miss.Header.Get("Connection"),
				"hop-by-hop headers are stripped from the stored entry, not from the response the wire delivered")

			// The hit is served from the entry the miss wrote.
			hit, err := transport.RoundTrip(httptest.NewRequest(http.MethodGet, requestURL, nil))
			require.NoError(t, err)
			defer hit.Body.Close()
			require.Equal(t, 1, inner.calls, "the second GET must be served from the cache")

			assert.Equal(t, tc.status, hit.Status, "the stored status must be served, not a hardcoded 200 OK")
			assert.Equal(t, tc.code, hit.StatusCode)

			// Protocol version: from the response the exchange produced.
			assert.Equal(t, WireVersionHTTP2, hit.Proto)
			assert.Equal(t, 2, hit.ProtoMajor)
			assert.Equal(t, 0, hit.ProtoMinor)

			// Every end-to-end header the response carried.
			assert.Equal(t, "application/json", hit.Header.Get("Content-Type"))
			assert.Equal(t, "bar", hit.Header.Get("X-Foo"))
			assert.Equal(t, `"tag-1"`, hit.Header.Get("ETag"))
			assert.Equal(t, `</next>; rel="next"`, hit.Header.Get("Link"))
			assert.Equal(t, date, hit.Header.Get("Date"))
			assert.Equal(t, []string{"a=1; Path=/", "b=2; Path=/"}, hit.Header.Values("Set-Cookie"),
				"repeated headers must survive the round trip through the entry")

			// Connection-scoped headers are not part of a stored entry.
			for _, name := range hopByHopFixtureNames {
				assert.Emptyf(t, hit.Header.Get(name),
					"%s is connection-scoped and must not be replayed from the cache", name)
			}

			// Content-Length describes the bytes the entry serves.
			assert.Equal(t, strconv.Itoa(len(body)), hit.Header.Get("Content-Length"))
			assert.Equal(t, int64(len(body)), hit.ContentLength)

			hitBody, err := io.ReadAll(hit.Body)
			require.NoError(t, err)
			assert.Equal(t, body, string(hitBody))

			// The envelope is the stored shape, and what is stored is all a hit
			// can ever serve: the hop-by-hop set must not be in it. The JSON
			// field names are pinned here because the envelope is a storage
			// format (an in-memory or Redis entry written by one build is read
			// by the next).
			raw, found, err := store.Get(t.Context(), "HEADER:GET:"+requestURL+":Response-Meta")
			require.NoError(t, err)
			require.True(t, found, "the entry must carry its response envelope")
			var envelope struct {
				Proto      string              `json:"proto"`
				ProtoMajor int                 `json:"proto_major"`
				ProtoMinor int                 `json:"proto_minor"`
				Status     string              `json:"status"`
				StatusCode int                 `json:"status_code"`
				Header     map[string][]string `json:"header"`
			}
			require.NoError(t, json.Unmarshal(raw, &envelope))
			assert.Equal(t, WireVersionHTTP2, envelope.Proto)
			assert.Equal(t, 2, envelope.ProtoMajor)
			assert.Equal(t, 0, envelope.ProtoMinor)
			assert.Equal(t, tc.status, envelope.Status)
			assert.Equal(t, tc.code, envelope.StatusCode)
			assert.Contains(t, envelope.Header, "X-Foo")
			// A kept header is stored under its canonical spelling, so
			// Header.Get finds it whatever spelling the response arrived with
			// (the fixture spells it "ETag", whose canonical form is "Etag").
			assert.Contains(t, envelope.Header, "Etag")
			assert.NotContains(t, envelope.Header, "ETag")
			for _, name := range hopByHopFixtureNames {
				assert.NotContainsf(t, envelope.Header, name,
					"%s must not be stored: a stored connection-scoped header is replayed on a later connection", name)
			}
		})
	}
}

// TestCacheRoundTripper_HitPreservesWireProtocolAndHeaders asks the same
// fidelity question over a real HTTP/2 exchange instead of a synthetic
// response: the version and the headers a hit serves are the ones the peer
// sent, and the response object agrees with the record the base transport
// observed (the two surfaces the DF-041 divergence was measured between).
func TestCacheRoundTripper_HitPreservesWireProtocolAndHeaders(t *testing.T) {
	const body = `{"wire":true}`

	var hits atomic.Int64
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Foo", "bar")
		w.Header().Set("ETag", `"wire-v1"`)
		w.Header().Add("Set-Cookie", "first=1")
		w.Header().Add("Set-Cookie", "second=2")
		_, _ = io.WriteString(w, body)
	}))
	s.EnableHTTP2 = true
	s.StartTLS()
	t.Cleanup(s.Close)
	stubDefaultTransport(t, s)

	store := cache.NewMemoryCache(100)
	t.Cleanup(func() { _ = store.Close() })
	c := NewClient(WithCache(store, time.Minute))

	requestURL := s.URL + "/wire"

	first, err := c.Do(t.Context(), mustRequest(t, http.MethodGet, requestURL))
	require.NoError(t, err)
	firstBody, err := io.ReadAll(first.Body)
	require.NoError(t, err)
	require.NoError(t, first.Body.Close())
	require.Equal(t, WireVersionHTTP2, first.Proto,
		"the fixture must really be an HTTP/2 exchange, or this cell proves nothing")
	require.Equal(t, body, string(firstBody))

	second, err := c.Do(t.Context(), mustRequest(t, http.MethodGet, requestURL))
	require.NoError(t, err)
	defer func() { _ = second.Body.Close() }()

	require.Equal(t, int64(1), hits.Load(), "the second GET must be served from the cache")

	assert.Equal(t, WireVersionHTTP2, second.Proto,
		"the hit reports the version the exchange used, not a default of HTTP/1.1")
	assert.Equal(t, 2, second.ProtoMajor)
	assert.Equal(t, 0, second.ProtoMinor)
	assert.Equal(t, c.Protocol().Observed, second.Proto,
		"the response object and the base-transport record must agree once the cache stops rebuilding Proto (DF-041)")
	assert.Equal(t, SourceCache, c.Protocol().Source)

	assert.Equal(t, first.Header.Get("Date"), second.Header.Get("Date"),
		"the stored Date is served unrewritten")
	assert.Equal(t, "application/json", second.Header.Get("Content-Type"))
	assert.Equal(t, "bar", second.Header.Get("X-Foo"))
	assert.Equal(t, `"wire-v1"`, second.Header.Get("ETag"))
	assert.Equal(t, []string{"first=1", "second=2"}, second.Header.Values("Set-Cookie"))

	secondBody, err := io.ReadAll(second.Body)
	require.NoError(t, err)
	assert.Equal(t, body, string(secondBody))
	assert.Equal(t, int64(len(body)), second.ContentLength)
}

// TestCacheRoundTripper_LegacyEntryKeepsItsStoredContentTypeAndVersion covers
// the additive half: an entry written before the envelope existed — body plus
// the Content-Type and version keys only — must keep serving exactly what it
// recorded, with no other header invented for it and no version defaulted.
func TestCacheRoundTripper_LegacyEntryKeepsItsStoredContentTypeAndVersion(t *testing.T) {
	const (
		requestURL = "http://example.com/api/legacy"
		body       = "plain text"
	)

	store := cache.NewMemoryCache(10)
	t.Cleanup(func() { _ = store.Close() })

	cacheKey := "GET:" + requestURL
	require.NoError(t, store.Set(t.Context(), cacheKey, []byte(body), time.Minute))
	require.NoError(t, store.Set(t.Context(), "HEADER:"+cacheKey+":Content-Type", []byte("text/plain"), time.Minute))
	require.NoError(t, store.Set(t.Context(), versionKey(cacheKey), []byte(WireVersionHTTP11), time.Minute))

	inner := &fixtureRoundTripper{response: func() *http.Response {
		return &http.Response{
			Status:        "200 OK",
			StatusCode:    http.StatusOK,
			Proto:         WireVersionHTTP2,
			ProtoMajor:    2,
			ProtoMinor:    0,
			Header:        http.Header{"Content-Type": {"application/json"}, "X-Foo": {"bar"}},
			Body:          io.NopCloser(strings.NewReader("from the wire")),
			ContentLength: int64(len("from the wire")),
		}
	}}
	transport := NewCacheRoundTripper(inner, store, time.Minute)

	resp, err := transport.RoundTrip(httptest.NewRequest(http.MethodGet, requestURL, nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 0, inner.calls, "a seeded entry is a hit, envelope or not")
	assert.Equal(t, "text/plain", resp.Header.Get("Content-Type"), "the entry's recorded Content-Type")
	assert.Len(t, resp.Header, 1, "a legacy entry carries no other header, and none may be invented")
	assert.Equal(t, WireVersionHTTP11, resp.Proto, "the entry's recorded version, not a default")
	assert.Equal(t, 1, resp.ProtoMajor)
	assert.Equal(t, 1, resp.ProtoMinor)

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
	assert.Equal(t, int64(len(body)), resp.ContentLength)
}
