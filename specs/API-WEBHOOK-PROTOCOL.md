# API WebSocket Webhook Protocol

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  

## Overview

The WebSocket Webhook Protocol provides persistent, bidirectional communication channels for real-time event delivery. This protocol ensures reliable message delivery through automatic reconnection, message acknowledgment, and heartbeat mechanisms. WebSocket webhooks are ideal for scenarios requiring low-latency event notifications, such as resource state changes, job progress updates, and real-time monitoring.

### Key Features

- **Persistent Connection**: WebSocket maintains a long-lived connection
- **Bidirectional Communication**: Both client and server can send messages
- **Automatic Reconnection**: Exponential backoff with configurable retry logic
- **Message Acknowledgment**: Ensures reliable event delivery
- **Heartbeat Mechanism**: Ping/pong for connection health monitoring
- **Event Filtering**: Subscribe to specific event types and resources

## Connection Establishment

### Endpoint

```
wss://host/_webhooks/stream
```

### Handshake

**Client Request:**

```http
GET /_webhooks/stream HTTP/1.1
Host: api.example.com
Connection: Upgrade
Upgrade: websocket
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
Sec-WebSocket-Version: 13
Authorization: Bearer <token>
X-Client-ID: <uuid>
X-Subscription-Id: <subscription-id>
```

**Server Response:**

```http
HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=
X-Connection-ID: conn-abc123
```

### Connection Headers

| Header | Required | Description |
|--------|----------|-------------|
| `Authorization` | Yes | Bearer token for authentication |
| `X-Client-ID` | Yes | Unique client identifier (UUID) |
| `X-Subscription-Id` | Yes | Subscription identifier for event filtering |
| `Sec-WebSocket-Version` | Yes | WebSocket protocol version (13) |

## Message Format

### Frame Structure

All messages use the following JSON structure:

```json
{
  "type": "event",
  "id": "uuid-v4",
  "timestamp": "2024-01-15T10:30:00.000Z",
  "payload": { }
}
```

### Message Types

| Type | Direction | Description |
|------|-----------|-------------|
| `event` | Server → Client | Event notification |
| `ack` | Client → Server | Message acknowledgment |
| `ping` | Server → Client | Connection health check |
| `pong` | Client → Server | Ping response |
| `error` | Bidirectional | Error notification |
| `subscribe` | Client → Server | Update subscription filters |

### Event Message

```json
{
  "type": "event",
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": "2024-01-15T10:30:00.123Z",
  "payload": {
    "subscription_id": "sub-123",
    "event_type": "resource.created",
    "resource": {
      "id": "res-456",
      "type": "pod",
      "namespace": "production"
    },
    "data": { }
  }
}
```

### Acknowledgment Message

```json
{
  "type": "ack",
  "id": "ack-550e8400",
  "timestamp": "2024-01-15T10:30:00.200Z",
  "payload": {
    "acknowledged_ids": ["550e8400-e29b-41d4-a716-446655440000"],
    "received_count": 1
  }
}
```

### Subscribe Message

```json
{
  "type": "subscribe",
  "id": "sub-550e8400",
  "timestamp": "2024-01-15T10:30:00.000Z",
  "payload": {
    "event_types": ["resource.created", "resource.updated"],
    "resource_types": ["pod", "service"],
    "namespaces": ["production"]
  }
}
```

## Authentication

### Token-Based Authentication

```go
headers := http.Header{
    "Authorization": []string{"Bearer " + token},
    "X-Client-ID":   []string{clientID},
    "X-Subscription-Id": []string{subscriptionID},
}
conn, _, err := websocket.DefaultDialer.Dial(url, headers)
```

### Token Refresh

On receiving a 401 Unauthorized response during handshake:

```go
func (c *Client) handleAuthFailure(resp *http.Response) error {
    if resp.StatusCode == 401 {
        token, err := c.refreshToken(c.ctx)
        if err != nil {
            return fmt.Errorf("token refresh failed: %w", err)
        }
        c.token = token
        // Retry connection with new token
        return c.connectWithRetry(c.ctx)
    }
    return nil
}
```

## Reconnection Logic

### Exponential Backoff Configuration

```go
package webhook

import (
    "context"
    "math"
    "math/rand"
    "time"
)

type ReconnectConfig struct {
    MaxRetries    int           // Maximum number of retry attempts
    BaseDelay     time.Duration // Initial delay between retries
    MaxDelay      time.Duration // Maximum delay cap
    JitterFactor  float64       // Random jitter (0.0 - 1.0)
}

func DefaultReconnectConfig() *ReconnectConfig {
    return &ReconnectConfig{
        MaxRetries:   10,
        BaseDelay:    1 * time.Second,
        MaxDelay:     5 * time.Minute,
        JitterFactor: 0.2,
    }
}

func (c *ReconnectConfig) NextDelay(attempt int) time.Duration {
    if attempt >= c.MaxRetries {
        return 0
    }
    
    // Exponential backoff: base * 2^attempt
    delay := c.BaseDelay * time.Duration(math.Pow(2, float64(attempt)))
    if delay > c.MaxDelay {
        delay = c.MaxDelay
    }
    
    // Add jitter to prevent thundering herd
    jitter := time.Duration(float64(delay) * c.JitterFactor * (rand.Float64() - 0.5))
    return delay + jitter
}
```

### Reconnection Implementation

```go
func (c *Client) reconnect(ctx context.Context) error {
    for attempt := 0; attempt < c.config.MaxRetries; attempt++ {
        delay := c.config.NextDelay(attempt)
        
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(delay):
        }
        
        c.logger.Printf("Reconnection attempt %d/%d", attempt+1, c.config.MaxRetries)
        
        conn, err := c.connect(ctx)
        if err == nil {
            c.conn = conn
            c.connected = true
            return c.resumeSubscription()
        }
        
        c.logger.Printf("Reconnect attempt %d failed: %v", attempt+1, err)
    }
    
    return fmt.Errorf("max reconnection attempts (%d) exceeded", c.config.MaxRetries)
}
```

### Resume After Reconnect

After reconnection, the client MUST send a resume message to request missed events:

```go
func (c *Client) resumeSubscription() error {
    resume := ResumeMessage{
        Type:        "resume",
        LastEventID: c.lastEventID,
        LastAckTime: c.lastAckTime,
    }
    
    return c.conn.WriteJSON(resume)
}
```

**Server Response on Resume:**

```json
{
  "type": "resume",
  "id": "resume-uuid",
  "timestamp": "2024-01-15T10:30:00.000Z",
  "payload": {
    "missed_events": 5,
    "events": [ ]
  }
}
```

## Event Types

### Standard Events

| Event Type | Description | Payload Structure |
|------------|-------------|-------------------|
| `resource.created` | New resource created | Full resource object |
| `resource.updated` | Resource modified | Delta/changed fields |
| `resource.deleted` | Resource removed | Resource ID and metadata |
| `batch.completed` | Batch operation finished | Summary statistics |
| `job.started` | Async job initiated | Job ID and metadata |
| `job.progress` | Job progress update | Percentage and status |
| `job.completed` | Job finished | Result and duration |
| `job.failed` | Job failed | Error details |

### Event Filtering

Clients can filter events using subscription configuration:

```go
subscription := SubscriptionConfig{
    EventTypes:    []string{"resource.created", "resource.updated"},
    ResourceTypes: []string{"pod", "service"},
    Namespaces:    []string{"production", "staging"},
    Labels: map[string]string{
        "app": "web",
        "env": "prod",
    },
}
```

## Go Implementation

### Complete Client Implementation

```go
package webhook

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
    "time"
    
    "github.com/gorilla/websocket"
    "github.com/google/uuid"
)

// Client manages WebSocket webhook connection
type Client struct {
    url            string
    token          string
    clientID       string
    subscriptionID string
    
    conn           *websocket.Conn
    config         *ReconnectConfig
    
    lastEventID    string
    lastAckTime    time.Time
    pendingAcks    map[string]time.Time
    ackMu          sync.RWMutex
    
    eventHandler   func(Event) error
    errorHandler   func(error)
    
    ctx            context.Context
    cancel         context.CancelFunc
    wg             sync.WaitGroup
    connected      bool
    mu             sync.RWMutex
}

// Event represents a webhook event
type Event struct {
    ID          string          `json:"id"`
    Type        string          `json:"type"`
    Timestamp   time.Time       `json:"timestamp"`
    Payload     json.RawMessage `json:"payload"`
}

// AckMessage represents client acknowledgment
type AckMessage struct {
    Type    string   `json:"type"`
    ID      string   `json:"id"`
    Payload struct {
        AcknowledgedIDs []string `json:"acknowledged_ids"`
    } `json:"payload"`
}

// ResumeMessage requests missed events after reconnection
type ResumeMessage struct {
    Type        string    `json:"type"`
    LastEventID string    `json:"last_event_id"`
    LastAckTime time.Time `json:"last_ack_time"`
}

// NewClient creates a new webhook client
func NewClient(url, token, subscriptionID string) *Client {
    return &Client{
        url:            url,
        token:          token,
        clientID:       uuid.New().String(),
        subscriptionID: subscriptionID,
        config:         DefaultReconnectConfig(),
        pendingAcks:    make(map[string]time.Time),
        eventHandler:   func(e Event) error { return nil },
        errorHandler:   func(e error) {},
    }
}

// SetEventHandler sets the event handler callback
func (c *Client) SetEventHandler(handler func(Event) error) {
    c.eventHandler = handler
}

// SetErrorHandler sets the error handler callback
func (c *Client) SetErrorHandler(handler func(error)) {
    c.errorHandler = handler
}

// Connect establishes the WebSocket connection
func (c *Client) Connect(ctx context.Context) error {
    c.ctx, c.cancel = context.WithCancel(ctx)
    
    conn, err := c.connectWithRetry(c.ctx)
    if err != nil {
        return fmt.Errorf("failed to connect: %w", err)
    }
    
    c.conn = conn
    c.setConnected(true)
    
    c.wg.Add(3)
    go c.readLoop()
    go c.ackLoop()
    go c.pingLoop()
    
    return nil
}

func (c *Client) connectWithRetry(ctx context.Context) (*websocket.Conn, error) {
    headers := http.Header{
        "Authorization":     []string{"Bearer " + c.token},
        "X-Client-ID":       []string{c.clientID},
        "X-Subscription-Id": []string{c.subscriptionID},
    }
    
    dialer := websocket.DefaultDialer
    dialer.HandshakeTimeout = 10 * time.Second
    
    for attempt := 0; attempt < c.config.MaxRetries; attempt++ {
        conn, resp, err := dialer.Dial(c.url, headers)
        if err == nil {
            return conn, nil
        }
        
        if resp != nil {
            if resp.StatusCode == 401 {
                return nil, fmt.Errorf("authentication failed")
            }
            if resp.StatusCode == 403 {
                return nil, fmt.Errorf("forbidden")
            }
        }
        
        if attempt < c.config.MaxRetries-1 {
            delay := c.config.NextDelay(attempt)
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            case <-time.After(delay):
            }
        }
    }
    
    return nil, fmt.Errorf("failed to connect after %d attempts", c.config.MaxRetries)
}

func (c *Client) readLoop() {
    defer c.wg.Done()
    
    for {
        select {
        case <-c.ctx.Done():
            return
        default:
        }
        
        var msg Event
        if err := c.conn.ReadJSON(&msg); err != nil {
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
                c.errorHandler(fmt.Errorf("connection closed: %w", err))
                c.setConnected(false)
                
                // Attempt reconnection
                if err := c.reconnect(c.ctx); err != nil {
                    c.errorHandler(fmt.Errorf("reconnection failed: %w", err))
                    return
                }
                continue
            }
            return
        }
        
        switch msg.Type {
        case "event":
            c.ackMu.Lock()
            c.pendingAcks[msg.ID] = time.Now()
            c.ackMu.Unlock()
            
            if err := c.eventHandler(msg); err != nil {
                c.errorHandler(fmt.Errorf("event handler error: %w", err))
            }
            
            c.lastEventID = msg.ID
            
        case "error":
            c.errorHandler(fmt.Errorf("server error: %s", msg.Payload))
            
        case "ping":
            // Ping is handled automatically by gorilla/websocket
            c.sendPong()
        }
    }
}

func (c *Client) ackLoop() {
    defer c.wg.Done()
    
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-c.ctx.Done():
            return
        case <-ticker.C:
            c.sendPendingAcks()
        }
    }
}

func (c *Client) sendPendingAcks() {
    c.ackMu.Lock()
    if len(c.pendingAcks) == 0 {
        c.ackMu.Unlock()
        return
    }
    
    ids := make([]string, 0, len(c.pendingAcks))
    for id := range c.pendingAcks {
        ids = append(ids, id)
    }
    c.pendingAcks = make(map[string]time.Time)
    c.ackMu.Unlock()
    
    ack := AckMessage{
        Type: "ack",
        ID:   uuid.New().String(),
    }
    ack.Payload.AcknowledgedIDs = ids
    
    if err := c.conn.WriteJSON(ack); err != nil {
        c.errorHandler(fmt.Errorf("ack send failed: %w", err))
    } else {
        c.lastAckTime = time.Now()
    }
}

func (c *Client) pingLoop() {
    defer c.wg.Done()
    
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-c.ctx.Done():
            return
        case <-ticker.C:
            deadline := time.Now().Add(10 * time.Second)
            if err := c.conn.WriteControl(websocket.PingMessage, []byte{}, deadline); err != nil {
                c.errorHandler(fmt.Errorf("ping failed: %w", err))
            }
        }
    }
}

func (c *Client) sendPong() {
    deadline := time.Now().Add(10 * time.Second)
    if err := c.conn.WriteControl(websocket.PongMessage, []byte{}, deadline); err != nil {
        c.errorHandler(fmt.Errorf("pong send failed: %w", err))
    }
}

func (c *Client) setConnected(connected bool) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.connected = connected
}

// IsConnected returns the current connection status
func (c *Client) IsConnected() bool {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.connected
}

// Close gracefully closes the connection
func (c *Client) Close() error {
    c.cancel()
    c.wg.Wait()
    
    if c.conn != nil {
        c.conn.WriteMessage(
            websocket.CloseMessage,
            websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
        )
        return c.conn.Close()
    }
    return nil
}
```

## Heartbeat Mechanism

### Server-Side Ping

The server sends ping messages every 30 seconds:

```go
func (s *Server) pingLoop(conn *websocket.Conn) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            deadline := time.Now().Add(10 * time.Second)
            if err := conn.WriteControl(websocket.PingMessage, []byte{}, deadline); err != nil {
                s.logger.Printf("Ping failed: %v", err)
                return
            }
        }
    }
}
```

### Client Pong Response

The client MUST respond to ping messages with pong within 10 seconds:

```go
conn.SetPongHandler(func(string) error {
    c.lastPong = time.Now()
    return nil
})
```

## Security Considerations

- **TLS Required**: Always use `wss://` (WebSocket Secure)
- **Token Validation**: Validate bearer token during handshake
- **Rate Limiting**: Implement per-connection rate limits
- **Message Size Limits**: Limit maximum message size (default: 10MB)
- **Connection Limits**: Limit connections per client/subscription

### Rate Limiting Headers

```http
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Reset: 1705317060
```

## Error Handling

### Connection Errors

| Error | Action |
|-------|--------|
| `401 Unauthorized` | Refresh token and retry |
| `403 Forbidden` | Terminate connection, check permissions |
| `404 Not Found` | Subscription not found, check configuration |
| `429 Too Many Requests` | Backoff and retry |
| `1006 Abnormal Closure` | Reconnect with exponential backoff |

### Protocol Errors

```json
{
  "type": "error",
  "id": "err-550e8400",
  "timestamp": "2024-01-15T10:30:00.000Z",
  "payload": {
    "error_code": "INVALID_MESSAGE",
    "message": "Message format validation failed",
    "details": {
      "field": "event_type",
      "reason": "unknown event type"
    }
  }
}
```

## Performance Guidelines

- **Batch Acknowledgments**: Acknowledge multiple events in single message
- **Connection Pooling**: Reuse connections across multiple subscriptions
- **Compression**: Enable per-message compression for large payloads
- **Event Deduplication**: Client MUST handle duplicate events gracefully

---

**Bible Standard Compliance**: This specification adheres to the Muster Bible Standard for real-time communication, security, and reliability. All implementations must support automatic reconnection and message acknowledgment.
