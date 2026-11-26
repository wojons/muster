package graphql_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
	graphql "github.com/wojons/muster/pkg/protocol/graphql"
	"github.com/wojons/muster/pkg/protocol/graphql/wstest"
)

// subscriptionFixture starts the in-process graphql-transport-ws server and a
// client discovered against the shared SDL fixture with its endpoint pointed
// at the fixture (the unary POST endpoint and the WS endpoint are the same
// URL: one endpoint, two framings — the §4.7 no-second-call-path rule).
func subscriptionFixture(t *testing.T, mutate func(*wstest.Server)) (*wstest.Server, *graphql.Client) {
	t.Helper()
	srv, err := wstest.Start()
	require.NoError(t, err)
	t.Cleanup(srv.Stop)
	if mutate != nil {
		mutate(srv)
	}

	client := graphql.NewClient(srv.URL(), graphql.WithTimeout(5*time.Second))
	_, err = client.Discover(context.Background(), "testdata/schema.graphql")
	require.NoError(t, err)
	return srv, client
}

// decodeTranscript renders a Subscribe result as the envelope so assertions
// can index it.
func decodeTranscript(t *testing.T, raw interface{}) *graphql.SubscriptionTranscript {
	t.Helper()
	b, err := json.Marshal(raw)
	require.NoError(t, err)
	var tr graphql.SubscriptionTranscript
	require.NoError(t, json.Unmarshal(b, &tr))
	return &tr
}

// TestSubscribe_CompleteCollect_CleanUnsubscribe is the §4.5(4) happy path:
// subscribe, collect every event the server sends, the server completes, the
// client unsubscribes, the transcript reports complete with the full envelope.
func TestSubscribe_CompleteCollect_CleanUnsubscribe(t *testing.T) {
	srv, client := subscriptionFixture(t, func(s *wstest.Server) { s.Events = 3 })
	_ = srv // the fixture's unsubscribed counter is pinned by the cap test below

	result, err := client.Subscribe(context.Background(), "userChanged", map[string]interface{}{"id": "u1"}, graphql.SubscriptionLimits{})
	require.NoError(t, err)
	tr := decodeTranscript(t, result)

	assert.Equal(t, "subscription", tr.Kind)
	assert.Equal(t, graphql.SubStatusComplete, tr.Status)
	assert.False(t, tr.ItemsTruncated)
	require.Len(t, tr.Items, 3)
	assert.Equal(t, float64(0), tr.Items[0].(map[string]interface{})["userChanged"].(map[string]interface{})["seq"],
		"an item is the decoded data member of one next frame")
	assert.Equal(t, float64(3), tr.Trailer["received"])
	assert.Equal(t, float64(graphql.DefaultSubscriptionMaxMessages), tr.Trailer["max_messages"])
	assert.True(t, tr.Trailer["unsubscribed"].(bool), "the client's complete frame went out")
	assert.NotContains(t, tr.Trailer, "limit_reason")
}

// TestSubscribe_CapTruncatesCleanly pins the §4.5(4) bounded envelope: a
// server streaming more than MaxMessages is stopped by the cap — a CLEAN
// stop, status truncated, limit_reason max-messages, the partial items
// preserved and the client unsubscribed.
func TestSubscribe_CapTruncatesCleanly(t *testing.T) {
	srv, client := subscriptionFixture(t, func(s *wstest.Server) { s.Events = 50 })

	result, err := client.Subscribe(context.Background(), "userChanged", map[string]interface{}{"id": "u1"}, graphql.SubscriptionLimits{MaxMessages: 5})
	require.NoError(t, err, "hitting the cap is a clean stop, not an error")
	tr := decodeTranscript(t, result)

	assert.Equal(t, graphql.SubStatusTruncated, tr.Status)
	assert.True(t, tr.ItemsTruncated)
	assert.Equal(t, graphql.LimitReasonMessages, tr.Trailer["limit_reason"])
	require.Len(t, tr.Items, 5)
	assert.Equal(t, float64(5), tr.Trailer["received"])
	assert.Equal(t, float64(5), tr.Trailer["max_messages"])
	assert.True(t, tr.Trailer["unsubscribed"].(bool), "the clean unsubscribe went out at the cap")
	require.Eventually(t, func() bool { return srv.Unsubscribes() == 1 }, 2*time.Second, 5*time.Millisecond,
		"the fixture recorded exactly one client unsubscribe")
}

// TestSubscribe_DurationTruncatesCleanly pins the wall-clock bound: a server
// that streams forever is stopped by MaxDuration — the other clean stop.
func TestSubscribe_DurationTruncatesCleanly(t *testing.T) {
	_, client := subscriptionFixture(t, func(s *wstest.Server) {
		s.Events = 100000
		s.Delay = 20 * time.Millisecond
	})

	started := time.Now()
	result, err := client.Subscribe(context.Background(), "userChanged", map[string]interface{}{"id": "u1"}, graphql.SubscriptionLimits{MaxDuration: 150 * time.Millisecond})
	require.NoError(t, err, "hitting the wall clock is a clean stop, not an error")
	tr := decodeTranscript(t, result)

	assert.Equal(t, graphql.SubStatusTruncated, tr.Status)
	assert.True(t, tr.ItemsTruncated)
	assert.Equal(t, graphql.LimitReasonDuration, tr.Trailer["limit_reason"])
	assert.Less(t, time.Since(started), time.Second, "the bound actually stopped the collect")
	assert.NotEmpty(t, tr.Items, "partial items are preserved")
	assert.True(t, tr.Trailer["unsubscribed"].(bool),
		"the socket stayed open long enough for the clean unsubscribe at a wall-clock stop")
}

// TestSubscribe_ServerErrorMidStream pins §4.5 rule 5: a server-sent error
// frame terminates the transcript with status error, the partial items are
// preserved, and the §4.2-mapped AppError comes back alongside it.
func TestSubscribe_ServerErrorMidStream(t *testing.T) {
	_, client := subscriptionFixture(t, func(s *wstest.Server) {
		s.Events = 10
		failAt := 2
		s.FailAt = &failAt
	})

	result, err := client.Subscribe(context.Background(), "userChanged", map[string]interface{}{"id": "u1"}, graphql.SubscriptionLimits{})
	require.Error(t, err)
	tr := decodeTranscript(t, result)

	assert.Equal(t, graphql.SubStatusError, tr.Status)
	assert.Len(t, tr.Items, 2, "items collected before the failure are preserved")
	assert.Contains(t, tr.Trailer["error"], "boom: the fixture failed the stream")
	assert.True(t, errors.IsError(err, errors.CodeGeneric), "a fixture GraphQL error maps per §4.2, got %v", err)
}

// TestSubscribe_CallerCancellation pins the cancelled vocabulary: the
// CALLER's own context ending is a real failure (CodeTimeout), never a
// truncation, and the partial items are preserved.
func TestSubscribe_CallerCancellation(t *testing.T) {
	_, client := subscriptionFixture(t, func(s *wstest.Server) {
		s.Events = 100000
		s.Delay = 50 * time.Millisecond
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := client.Subscribe(ctx, "userChanged", map[string]interface{}{"id": "u1"}, graphql.SubscriptionLimits{})
	require.Error(t, err)
	tr := decodeTranscript(t, result)

	assert.Equal(t, graphql.SubStatusCancelled, tr.Status)
	assert.True(t, errors.IsError(err, errors.CodeTimeout), "got %v", err)
	assert.NotContains(t, tr.Trailer, "limit_reason", "a cancellation is not a bound we imposed")
}

// TestSubscribe_NonSubscriptionOperationRefused pins the executor split: a
// query or mutation op cannot reach the subscription path, and a subscription
// op invoked through Execute runs the bounded collect, not a live stream.
func TestSubscribe_NonSubscriptionOperationRefused(t *testing.T) {
	srv, client := subscriptionFixture(t, func(s *wstest.Server) { s.Events = 1 })

	_, err := client.Subscribe(context.Background(), "user", map[string]interface{}{"id": "u1"}, graphql.SubscriptionLimits{})
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeValidation), "got %v", err)
	assert.Contains(t, err.Error(), "not a subscription")

	// Execute on the subscription op routes through the same bounded
	// collect (the MCP path — §4.5(4)).
	result, err := client.Execute(context.Background(), "userChanged", map[string]interface{}{"id": "u1"})
	require.NoError(t, err)
	tr := decodeTranscript(t, result)
	assert.Equal(t, "subscription", tr.Kind)
	assert.Equal(t, graphql.SubStatusComplete, tr.Status)
	assert.NotZero(t, srv)
}

// TestSubscribe_MissingEndpointRefused pins the endpoint derivation: without
// an endpoint there is no transport to select and the error says so.
func TestSubscribe_MissingEndpointRefused(t *testing.T) {
	client := graphql.NewClient("", graphql.WithTimeout(5*time.Second))
	_, err := client.Discover(context.Background(), "testdata/schema.graphql")
	require.NoError(t, err)

	_, err = client.Subscribe(context.Background(), "userChanged", map[string]interface{}{"id": "u1"}, graphql.SubscriptionLimits{})
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeValidation), "got %v", err)
	assert.Contains(t, err.Error(), "no endpoint")
}

// TestWebsocketURLDerivation pins the §4.7 transport-selection table: the WS
// URL is derived from the SAME endpoint the unary path posts to, never a
// second endpoint config.
func TestWebsocketURLDerivation(t *testing.T) {
	for _, tc := range []struct {
		endpoint, want string
	}{
		{"http://h:8080/graphql", "ws://h:8080/graphql"},
		{"https://h/graphql", "wss://h/graphql"},
		{"ws://h:8080/graphql", "ws://h:8080/graphql"},
		{"wss://h/graphql", "wss://h/graphql"},
	} {
		got, err := graphql.WebsocketURL(tc.endpoint)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got)
	}
	_, err := graphql.WebsocketURL("h:8080/graphql")
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeValidation), "got %v", err)
}

// TestMCP_SubscriptionToolIsBoundedCollectOnly pins §4.5(4) on the MCP
// surface: the subscription tool executes through the SAME executor as every
// other tool (Execute → the bounded collect), its description says so, and
// the transcript is the well-formed envelope — never a live stream and never
// a second tool surface (no subscribe/open-stream command exists).
func TestMCP_SubscriptionToolIsBoundedCollectOnly(t *testing.T) {
	srv, client := subscriptionFixture(t, func(s *wstest.Server) { s.Events = 2 })

	registry, err := graphql.NewGraphQLRegistry(client.Operations(), client.Schema(), client)
	require.NoError(t, err)

	cmd := registry.GetCommand("userChanged")
	require.NotNil(t, cmd, "the subscription is exposed as a tool (bounded collect)")
	assert.Contains(t, cmd.Description, "bounded collect",
		"the tool description names the §4.5(4) contract")
	assert.Contains(t, cmd.Description, "never a live stream")

	result, err := registry.ExecuteCommand(context.Background(), "userChanged", map[string]interface{}{"id": "u1"})
	require.NoError(t, err)
	tr := decodeTranscript(t, result)
	assert.Equal(t, "subscription", tr.Kind)
	assert.Equal(t, graphql.SubStatusComplete, tr.Status)
	require.Len(t, tr.Items, 2)

	// No persistent-subscription surface: every registry command maps to one
	// ingested operation, all of which execute through the bounded collect.
	for _, c := range registry.ListCommands() {
		assert.NotContains(t, c.Name, "subscribe", "no subscribe tool")
		assert.NotContains(t, c.Name, "stream", "no stream tool")
	}
	assert.NotZero(t, srv)
}

// compile-time pin: the client's event-source ops carry the neutral kind the
// MCP safety rules key off.
var _ = protocol.KindEventSource
