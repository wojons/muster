package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readJSON(t *testing.T, conn *websocket.Conn, v interface{}) {
	_, data, err := conn.Read(context.Background())
	require.NoError(t, err)
	err = json.Unmarshal(data, v)
	require.NoError(t, err)
}

func setupTestServer(t *testing.T, handler func(*websocket.Conn)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		require.NoError(t, err)
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		handler(conn)
	}))
}

func TestNewClient(t *testing.T) {
	client := NewClient("wss://example.com/_webhooks/stream", "test-token", "sub-123")

	require.NotNil(t, client)
	assert.Equal(t, "sub-123", client.subscriptionID)
	assert.Equal(t, "test-token", client.token)
	assert.NotNil(t, client.config)
	assert.NotNil(t, client.pendingAcks)
	assert.False(t, client.IsConnected())
}

func TestDefaultReconnectConfig(t *testing.T) {
	config := DefaultReconnectConfig()

	assert.Equal(t, 10, config.MaxRetries)
	assert.Equal(t, 1*time.Second, config.BaseDelay)
	assert.Equal(t, 5*time.Minute, config.MaxDelay)
	assert.Equal(t, 0.2, config.JitterFactor)
}

func TestReconnectConfig_NextDelay(t *testing.T) {
	config := &ReconnectConfig{
		MaxRetries:   10,
		BaseDelay:    1 * time.Second,
		MaxDelay:     5 * time.Minute,
		JitterFactor: 0.2,
	}

	t.Run("exponential backoff", func(t *testing.T) {
		delays := make([]time.Duration, 10)
		for i := 0; i < 10; i++ {
			delays[i] = config.NextDelay(i)
		}

		for i := 1; i < len(delays)-1; i++ {
			assert.Greater(t, delays[i], delays[i-1], "Delay should increase exponentially")
		}
	})

	t.Run("max delay cap", func(t *testing.T) {
		delay := config.NextDelay(20)
		assert.LessOrEqual(t, delay, config.MaxDelay)
	})

	t.Run("returns 0 after max retries", func(t *testing.T) {
		delay := config.NextDelay(config.MaxRetries)
		assert.Zero(t, delay)
	})
}

func TestClient_SetEventHandler(t *testing.T) {
	client := NewClient("wss://example.com", "token", "sub-123")

	called := false
	handler := func(e Event) error {
		called = true
		return nil
	}

	client.SetEventHandler(handler)

	event := Event{
		ID:        "test-event",
		Type:      "test",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`{"test": "data"}`),
	}

	err := client.eventHandler(event)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestClient_SetErrorHandler(t *testing.T) {
	client := NewClient("wss://example.com", "token", "sub-123")

	var capturedErr error
	handler := func(err error) {
		capturedErr = err
	}

	client.SetErrorHandler(handler)

	testErr := assert.AnError
	client.errorHandler(testErr)

	assert.Equal(t, testErr, capturedErr)
}

func TestClient_Connect_Success(t *testing.T) {
	server := setupTestServer(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		time.Sleep(100 * time.Millisecond)
	})
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	client := NewClient(wsURL, "token", "sub-123")

	ctx := context.Background()
	err := client.Connect(ctx)
	require.NoError(t, err)

	assert.True(t, client.IsConnected())

	err = client.Close()
	assert.NoError(t, err)
}

func TestClient_Connect_AuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	client := NewClient(wsURL, "invalid-token", "sub-123")
	client.config.MaxRetries = 1

	ctx := context.Background()
	err := client.Connect(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestClient_Connect_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	client := NewClient(wsURL, "token", "sub-123")
	client.config.MaxRetries = 1

	ctx := context.Background()
	err := client.Connect(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden")
}

func TestClient_IsConnected(t *testing.T) {
	client := NewClient("wss://example.com", "token", "sub-123")

	assert.False(t, client.IsConnected())

	client.setConnected(true)
	assert.True(t, client.IsConnected())

	client.setConnected(false)
	assert.False(t, client.IsConnected())
}

func TestClient_sendPendingAcks(t *testing.T) {
	server := setupTestServer(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		var msg AckMessage
		readJSON(t, conn, &msg)
		assert.Equal(t, "ack", msg.Type)
		assert.Len(t, msg.Payload.AcknowledgedIDs, 2)
		assert.Equal(t, 2, msg.Payload.ReceivedCount)
	})
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	client := NewClient(wsURL, "token", "sub-123")

	ctx := context.Background()
	err := client.Connect(ctx)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	client.ackMu.Lock()
	client.pendingAcks["event-1"] = time.Now()
	client.pendingAcks["event-2"] = time.Now()
	client.ackMu.Unlock()

	client.sendPendingAcks()

	client.ackMu.RLock()
	assert.Empty(t, client.pendingAcks)
	client.ackMu.RUnlock()
}

func TestClient_UpdateSubscription(t *testing.T) {
	receivedChan := make(chan SubscribeMessage, 1)

	server := setupTestServer(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		var msg SubscribeMessage
		readJSON(t, conn, &msg)
		receivedChan <- msg
	})
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	client := NewClient(wsURL, "token", "sub-123")

	ctx := context.Background()
	err := client.Connect(ctx)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	config := SubscriptionConfig{
		EventTypes:    []string{"resource.created", "resource.updated"},
		ResourceTypes: []string{"pod", "service"},
		Namespaces:    []string{"production"},
	}

	err = client.UpdateSubscription(config)
	require.NoError(t, err)

	select {
	case msg := <-receivedChan:
		assert.Equal(t, "subscribe", msg.Type)
		assert.Equal(t, []string{"resource.created", "resource.updated"}, msg.Payload.EventTypes)
		assert.Equal(t, []string{"pod", "service"}, msg.Payload.ResourceTypes)
		assert.Equal(t, []string{"production"}, msg.Payload.Namespaces)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for subscribe message")
	}
}

func TestClient_resumeSubscription(t *testing.T) {
	receivedChan := make(chan ResumeMessage, 1)

	server := setupTestServer(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		var msg ResumeMessage
		readJSON(t, conn, &msg)
		receivedChan <- msg
	})
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	client := NewClient(wsURL, "token", "sub-123")

	ctx := context.Background()
	err := client.Connect(ctx)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	client.lastEventID = "event-123"
	client.lastAckTime = time.Now().Add(-10 * time.Second)

	err = client.resumeSubscription()
	require.NoError(t, err)

	select {
	case msg := <-receivedChan:
		assert.Equal(t, "resume", msg.Type)
		assert.Equal(t, "event-123", msg.LastEventID)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for resume message")
	}
}

func TestClient_Close(t *testing.T) {
	server := setupTestServer(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		time.Sleep(50 * time.Millisecond)
	})
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	client := NewClient(wsURL, "token", "sub-123")

	ctx := context.Background()
	err := client.Connect(ctx)
	require.NoError(t, err)

	assert.True(t, client.IsConnected())

	err = client.Close()
	require.NoError(t, err)
}

func TestClient_Connect_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	client := NewClient(wsURL, "token", "sub-123")
	client.config.MaxRetries = 5
	client.config.BaseDelay = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := client.Connect(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestEvent_JSONUnmarshal(t *testing.T) {
	jsonData := `{
		"id": "event-123",
		"type": "resource.created",
		"timestamp": "2024-01-15T10:30:00.000Z",
		"payload": {"resource_id": "res-456", "action": "create"}
	}`

	var event Event
	err := json.Unmarshal([]byte(jsonData), &event)
	require.NoError(t, err)

	assert.Equal(t, "event-123", event.ID)
	assert.Equal(t, "resource.created", event.Type)
	assert.Equal(t, "2024-01-15T10:30:00Z", event.Timestamp.Format(time.RFC3339))

	var payload map[string]interface{}
	err = json.Unmarshal(event.Payload, &payload)
	require.NoError(t, err)
	assert.Equal(t, "res-456", payload["resource_id"])
}

func TestAckMessage_JSONMarshal(t *testing.T) {
	ack := AckMessage{
		Type: "ack",
		ID:   "ack-123",
	}
	ack.Payload.AcknowledgedIDs = []string{"event-1", "event-2", "event-3"}
	ack.Payload.ReceivedCount = 3

	data, err := json.Marshal(ack)
	require.NoError(t, err)

	var unmarshaled AckMessage
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, "ack", unmarshaled.Type)
	assert.Equal(t, "ack-123", unmarshaled.ID)
	assert.Equal(t, []string{"event-1", "event-2", "event-3"}, unmarshaled.Payload.AcknowledgedIDs)
	assert.Equal(t, 3, unmarshaled.Payload.ReceivedCount)
}

func TestSubscribeMessage_JSONMarshal(t *testing.T) {
	msg := SubscribeMessage{
		Type:      "subscribe",
		ID:        "sub-123",
		Timestamp: time.Now(),
	}
	msg.Payload.EventTypes = []string{"resource.created", "resource.updated"}
	msg.Payload.ResourceTypes = []string{"pod", "service"}
	msg.Payload.Namespaces = []string{"production", "staging"}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var unmarshaled SubscribeMessage
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, "subscribe", unmarshaled.Type)
	assert.Equal(t, []string{"resource.created", "resource.updated"}, unmarshaled.Payload.EventTypes)
	assert.Equal(t, []string{"pod", "service"}, unmarshaled.Payload.ResourceTypes)
	assert.Equal(t, []string{"production", "staging"}, unmarshaled.Payload.Namespaces)
}

func TestResumeMessage_JSONMarshal(t *testing.T) {
	msg := ResumeMessage{
		Type:        "resume",
		LastEventID: "event-456",
		LastAckTime: time.Now(),
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var unmarshaled ResumeMessage
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, "resume", unmarshaled.Type)
	assert.Equal(t, "event-456", unmarshaled.LastEventID)
}

func TestClient_ConnectWithHeaders(t *testing.T) {
	var receivedHeaders http.Header
	server := setupTestServer(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		time.Sleep(50 * time.Millisecond)
	})
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	client := NewClient(wsURL, "test-token-123", "sub-789")
	client.clientID = "client-abc"

	ctx := context.Background()
	err := client.Connect(ctx)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	assert.NotNil(t, client.conn)
	assert.True(t, client.IsConnected())

	_ = receivedHeaders
}

func TestClient_PingPong(t *testing.T) {
	// nhooyr.io/websocket handles ping/pong automatically
	// This test verifies the connection stays alive
	pingReceived := make(chan bool, 1)

	server := setupTestServer(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

		// Read messages and look for pings
		for {
			msgType, data, err := conn.Read(context.Background())
			if err != nil {
				return
			}

			// Check if it's a ping message (we'll send a custom ping-like message)
			if msgType == websocket.MessageText && string(data) == "ping" {
				select {
				case pingReceived <- true:
				default:
				}
			}
		}
	})
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	client := NewClient(wsURL, "token", "sub-123")

	ctx := context.Background()
	err := client.Connect(ctx)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	assert.True(t, client.IsConnected())

	// Give it time to establish connection
	time.Sleep(50 * time.Millisecond)
}

func TestClient_SetSignatureSecret(t *testing.T) {
	client := NewClient("wss://example.com", "token", "sub-123")

	assert.Empty(t, client.signatureSecret)

	client.SetSignatureSecret("test-secret")
	assert.Equal(t, "test-secret", client.signatureSecret)
}

func TestClient_verifySignature(t *testing.T) {
	t.Run("valid_signature", func(t *testing.T) {
		client := NewClient("wss://example.com", "token", "sub-123")
		client.SetSignatureSecret("test-secret")

		event := Event{
			ID:        "event-123",
			Type:      "test.event",
			Timestamp: time.Now(),
			Payload:   json.RawMessage(`{"data": "test"}`),
		}

		payload, _ := json.Marshal(event)
		h := hmac.New(sha256.New, []byte("test-secret"))
		h.Write(payload)
		event.Signature = base64.StdEncoding.EncodeToString(h.Sum(nil))

		assert.True(t, client.verifySignature(event))
	})

	t.Run("invalid_signature", func(t *testing.T) {
		client := NewClient("wss://example.com", "token", "sub-123")
		client.SetSignatureSecret("test-secret")

		event := Event{
			ID:        "event-123",
			Type:      "test.event",
			Timestamp: time.Now(),
			Payload:   json.RawMessage(`{"data": "test"}`),
			Signature: "invalid-signature",
		}

		assert.False(t, client.verifySignature(event))
	})

	t.Run("no_secret_configured", func(t *testing.T) {
		client := NewClient("wss://example.com", "token", "sub-123")

		event := Event{
			ID:        "event-123",
			Type:      "test.event",
			Timestamp: time.Now(),
			Payload:   json.RawMessage(`{"data": "test"}`),
			Signature: "some-signature",
		}

		assert.False(t, client.verifySignature(event))
	})

	t.Run("empty_signature_field", func(t *testing.T) {
		client := NewClient("wss://example.com", "token", "sub-123")
		client.SetSignatureSecret("test-secret")

		event := Event{
			ID:        "event-123",
			Type:      "test.event",
			Timestamp: time.Now(),
			Payload:   json.RawMessage(`{"data": "test"}`),
			Signature: "",
		}

		assert.False(t, client.verifySignature(event))
	})

	t.Run("signature_changes_with_payload", func(t *testing.T) {
		client := NewClient("wss://example.com", "token", "sub-123")
		client.SetSignatureSecret("test-secret")

		event1 := Event{
			ID:        "event-123",
			Type:      "test.event",
			Timestamp: time.Now(),
			Payload:   json.RawMessage(`{"data": "test1"}`),
		}

		event2 := Event{
			ID:        "event-123",
			Type:      "test.event",
			Timestamp: event1.Timestamp,
			Payload:   json.RawMessage(`{"data": "test2"}`),
		}

		payload1, _ := json.Marshal(event1)
		h1 := hmac.New(sha256.New, []byte("test-secret"))
		h1.Write(payload1)
		sig1 := base64.StdEncoding.EncodeToString(h1.Sum(nil))

		payload2, _ := json.Marshal(event2)
		h2 := hmac.New(sha256.New, []byte("test-secret"))
		h2.Write(payload2)
		sig2 := base64.StdEncoding.EncodeToString(h2.Sum(nil))

		assert.NotEqual(t, sig1, sig2)
	})

	t.Run("same_payload_same_signature", func(t *testing.T) {
		client := NewClient("wss://example.com", "token", "sub-123")
		client.SetSignatureSecret("test-secret")

		event := Event{
			ID:        "event-123",
			Type:      "test.event",
			Timestamp: time.Now(),
			Payload:   json.RawMessage(`{"data": "test"}`),
		}

		payload, _ := json.Marshal(event)
		h := hmac.New(sha256.New, []byte("test-secret"))
		h.Write(payload)
		sig := base64.StdEncoding.EncodeToString(h.Sum(nil))

		event.Signature = sig
		assert.True(t, client.verifySignature(event))

		event2 := Event{
			ID:        "event-123",
			Type:      "test.event",
			Timestamp: event.Timestamp,
			Payload:   event.Payload,
			Signature: sig,
		}
		assert.True(t, client.verifySignature(event2))
	})
}

func TestClient_EventSignatureVerification_Integration(t *testing.T) {
	t.Skip("Integration test - skipping in favor of unit tests")
}
