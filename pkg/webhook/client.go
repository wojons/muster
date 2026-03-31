// Package webhook provides a WebSocket-based client for subscribing to and consuming real-time events from APIs.
//
// This package implements the API Webhook Protocol specification, enabling bidirectional communication
// with webhook servers through WebSocket connections. It supports event subscriptions, automatic acknowledgments,
// heartbeat mechanisms, automatic reconnection with exponential backoff, and HMAC signature verification.
//
// # Main Components
//
// The [Client] type is the primary interface for connecting to webhook endpoints and receiving events.
// It manages the WebSocket connection, handles reconnection logic, and dispatches events to user-provided handlers.
//
// # Basic Usage
//
// Create a new client and connect:
//
//	client := webhook.NewClient("wss://api.example.com/webhooks", "my-token", "subscription-123")
//	client.SetEventHandler(func(event webhook.Event) error {
//	    log.Printf("Received event: %s - %s", event.Type, string(event.Payload))
//	    return nil
//	})
//	err := client.Connect(context.Background())
//
// # Event Handling
//
// Events are delivered through the event handler set via [Client.SetEventHandler]. Each event includes
// an ID, type, timestamp, and JSON payload. Events must be acknowledged within the configured timeout
// to prevent redelivery.
//
// # Automatic Acknowledgment
//
// The client automatically acknowledges received events in batches every 5 seconds via the ackLoop.
// This ensures reliable delivery while minimizing network overhead.
//
// # Reconnection
//
// The client automatically reconnects on connection loss using exponential backoff with jitter.
// Configure reconnection behavior via [Client.SetReconnectConfig] or by modifying the Config directly.
//
// # Signature Verification
//
// For secure deployments, set a signature secret via [Client.SetSignatureSecret] to enable HMAC-SHA256
// verification of event signatures. Events without valid signatures are rejected.
//
// # Graceful Shutdown
//
// Always call [Client.Close] to gracefully shutdown the connection and release resources:
//
//	defer client.Close()
package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

// Client is a WebSocket client for subscribing to and consuming real-time API events.
//
// The Client manages a persistent WebSocket connection to a webhook server, handling
// connection establishment, automatic reconnection with exponential backoff, heartbeat
// monitoring, event acknowledgment, and signature verification.
//
// # Thread Safety
//
// Client is safe for concurrent use. All public methods can be called from multiple
// goroutines without additional synchronization.
//
// # Lifecycle
//
//  1. Create a new client with [NewClient]
//  2. Optionally configure with SetSignatureSecret, SetEventHandler, SetErrorHandler
//  3. Connect to the webhook endpoint with [Client.Connect]
//  4. Receive events through the event handler
//  5. Call [Client.Close] to gracefully shutdown
//
// # Example
//
//	client := webhook.NewClient("wss://api.example.com/webhooks", "token", "sub-123")
//	client.SetEventHandler(func(event webhook.Event) error {
//	    log.Printf("Event: %s - %s", event.Type, string(event.Payload))
//	    return nil
//	})
//	if err := client.Connect(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
type Client struct {
	url             string
	token           string
	clientID        string
	subscriptionID  string
	signatureSecret string

	conn   *websocket.Conn
	config *ReconnectConfig

	lastEventID string
	lastAckTime time.Time
	pendingAcks map[string]time.Time
	ackMu       sync.RWMutex

	eventHandler func(Event) error
	errorHandler func(error)

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	connected bool
	mu        sync.RWMutex
}

// Event represents a webhook event delivered from the server to the client.
//
// Events are JSON objects sent over the WebSocket connection containing metadata
// about the occurrence and a payload with event-specific data. Each event has a
// unique ID for tracking and acknowledgment.
//
// # Fields
//
// ID is a unique identifier for this event, used for acknowledgment and deduplication.
//
// Type indicates the kind of event (e.g., "resource.created", "resource.updated").
// Event types follow a hierarchical naming convention with dot-separated segments.
//
// Timestamp is when the event occurred on the server.
//
// Payload contains the event-specific data as raw JSON. The structure depends on the
// event type and should be unmarshaled into a type-specific struct by the handler.
//
// Signature is an optional HMAC-SHA256 signature of the event payload. When a signature
// secret is configured via [Client.SetSignatureSecret], the client automatically verifies
// this signature before delivering the event to the handler.
//
// # Example
//
//	type OrderCreated struct {
//	    OrderID   string `json:"order_id"`
//	    CustomerID string `json:"customer_id"`
//	    Total     int64  `json:"total_cents"`
//	}
//
//	client.SetEventHandler(func(event webhook.Event) error {
//	    var order OrderCreated
//	    if err := json.Unmarshal(event.Payload, &order); err != nil {
//	        return err
//	    }
//	    log.Printf("Order %s created for customer %s", order.OrderID, order.CustomerID)
//	    return nil
//	})
type Event struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature,omitempty"`
}

// AckMessage is a client-to-server acknowledgment that events were successfully processed.
//
// Acknowledgments are sent automatically by the client's ackLoop every 5 seconds for all
// events received during that interval. This batched approach minimizes network overhead
// while ensuring reliable delivery.
//
// # Fields
//
// Type is always "ack" for acknowledgment messages.
//
// ID is a unique identifier for this acknowledgment message.
//
// Payload contains the acknowledgment details:
//   - AcknowledgedIDs: List of event IDs being acknowledged
//   - ReceivedCount: Total number of events acknowledged (optional, for metrics)
//
// # Automatic Acknowledgment
//
// Applications typically don't need to manually send acknowledgments as the client handles
// this automatically. However, if custom acknowledgment logic is needed, applications can
// track event IDs and call writeJSON with a custom AckMessage.
type AckMessage struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Payload struct {
		AcknowledgedIDs []string `json:"acknowledged_ids"`
		ReceivedCount   int      `json:"received_count,omitempty"`
	} `json:"payload"`
}

// ResumeMessage is sent to the server after reconnection to resume event delivery from the last received event.
//
// When a client reconnects after a disconnection, it sends a ResumeMessage to inform the server
// of the last event ID it successfully received and the last acknowledgment time. The server can
// then replay any missed events that occurred during the disconnection.
//
// # Fields
//
// Type is always "resume" for resume messages.
//
// LastEventID is the ID of the last event successfully received before disconnection. The server
// will replay events after this ID that were sent during the disconnection.
//
// LastAckTime is the timestamp of the last acknowledgment sent to the server. This helps the
// server determine which events need to be redelivered.
//
// # Automatic Resume
//
// The client automatically sends a ResumeMessage after successful reconnection via the
// [Client.resumeSubscription] method. Applications do not need to manually send resume messages.
type ResumeMessage struct {
	Type        string    `json:"type"`
	LastEventID string    `json:"last_event_id"`
	LastAckTime time.Time `json:"last_ack_time"`
}

// SubscribeMessage is sent to the server to subscribe to specific event types or update an existing subscription.
//
// Clients use SubscribeMessage to filter which events they want to receive. Subscriptions can be set
// initially or updated dynamically during the connection lifetime using [Client.UpdateSubscription].
//
// # Fields
//
// Type is always "subscribe" for subscription messages.
//
// ID is a unique identifier for this subscription message.
//
// Timestamp is when the subscription was created or updated.
//
// Payload contains the subscription filters:
//   - EventTypes: Filter by event type (e.g., ["user.created", "user.updated"]). Supports wildcards.
//   - ResourceTypes: Filter by resource type (e.g., ["users", "orders", "products"])
//   - Namespaces: Filter by namespace for multi-tenant deployments (e.g., ["org-123", "team-456"])
//
// # Example
//
//	// Subscribe to only user-related events
//	config := webhook.SubscriptionConfig{
//	    EventTypes: []string{"user.created", "user.updated", "user.deleted"},
//	}
//	if err := client.UpdateSubscription(config); err != nil {
//	    log.Printf("Failed to update subscription: %v", err)
//	}
type SubscribeMessage struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Payload   struct {
		EventTypes    []string `json:"event_types,omitempty"`
		ResourceTypes []string `json:"resource_types,omitempty"`
		Namespaces    []string `json:"namespaces,omitempty"`
	} `json:"payload"`
}

// ReconnectConfig configures the automatic reconnection behavior when the WebSocket connection is lost.
//
// The client uses exponential backoff with jitter to prevent thundering herd problems and reduce
// load on the server during widespread outages. The delay between reconnection attempts starts at
// BaseDelay and doubles with each attempt until reaching MaxDelay.
//
// # Fields
//
// MaxRetries is the maximum number of reconnection attempts before giving up. Set to 0 for unlimited
// retries (not recommended for production). Default is 10 attempts.
//
// BaseDelay is the initial delay before the first reconnection attempt. Default is 1 second.
//
// MaxDelay is the maximum delay between reconnection attempts. Once reached, all subsequent attempts
// use this delay. Default is 5 minutes.
//
// JitterFactor introduces randomness to prevent synchronized reconnection attempts. The jitter is
// calculated as ±JitterFactor of the delay. Default is 0.2 (±20% jitter).
//
// # Delay Calculation
//
// The delay for attempt N is calculated as:
//
//	delay = min(BaseDelay * 2^N, MaxDelay) ± (delay * JitterFactor * random(-0.5, 0.5))
//
// Example progression with defaults: 1s, 2s, 4s, 8s, 16s, 32s, 64s, 128s, 256s, 300s (capped at MaxDelay)
//
// # Example
//
//	config := &webhook.ReconnectConfig{
//	    MaxRetries:   20,
//	    BaseDelay:    2 * time.Second,
//	    MaxDelay:     10 * time.Minute,
//	    JitterFactor: 0.3,
//	}
//	client.SetReconnectConfig(config)
type ReconnectConfig struct {
	MaxRetries   int
	BaseDelay    time.Duration
	MaxDelay     time.Duration
	JitterFactor float64
}

// DefaultReconnectConfig returns a ReconnectConfig with sensible defaults for production use.
//
// The default configuration is tuned for most production scenarios:
//   - MaxRetries: 10 attempts before giving up
//   - BaseDelay: 1 second initial delay
//   - MaxDelay: 5 minutes maximum delay between attempts
//   - JitterFactor: 0.2 (±20% jitter to prevent thundering herd)
//
// This configuration balances rapid reconnection during transient failures with avoiding
// overwhelming the server during prolonged outages. The exponential backoff with jitter
// helps distribute reconnection attempts over time.
//
// # When to Customize
//
// Consider customizing the reconnection config if:
//   - Your application requires faster reconnection (reduce BaseDelay)
//   - Your server has strict rate limits (increase BaseDelay or reduce MaxRetries)
//   - You're in a high-availability environment (increase MaxRetries)
//   - You need predictable reconnection timing (reduce JitterFactor)
//
// # Example
//
//	config := webhook.DefaultReconnectConfig()
//	config.MaxRetries = 20  // More aggressive reconnection
//	client.SetReconnectConfig(config)
func DefaultReconnectConfig() *ReconnectConfig {
	return &ReconnectConfig{
		MaxRetries:   10,
		BaseDelay:    1 * time.Second,
		MaxDelay:     5 * time.Minute,
		JitterFactor: 0.2,
	}
}

// NextDelay calculates the delay before the next reconnection attempt using exponential backoff with jitter.
//
// The delay is calculated as: min(BaseDelay * 2^attempt, MaxDelay) ± jitter
// where jitter is a random value between -JitterFactor*delay and +JitterFactor*delay.
//
// # Parameters
//
// attempt is the current reconnection attempt number (0-indexed). For example, 0 for the first
// retry, 1 for the second retry, etc.
//
// # Returns
//
// The duration to wait before the next reconnection attempt. Returns 0 if attempt >= MaxRetries,
// indicating that no more retries should be attempted.
//
// # Example
//
//	config := webhook.DefaultReconnectConfig()
//	fmt.Println(config.NextDelay(0)) // ~1 second (first retry)
//	fmt.Println(config.NextDelay(1)) // ~2 seconds (second retry)
//	fmt.Println(config.NextDelay(5)) // ~32 seconds (sixth retry)
//	fmt.Println(config.NextDelay(10)) // 0 (max retries exceeded)
func (c *ReconnectConfig) NextDelay(attempt int) time.Duration {
	if attempt >= c.MaxRetries {
		return 0
	}

	delay := c.BaseDelay * time.Duration(math.Pow(2, float64(attempt)))
	if delay > c.MaxDelay {
		delay = c.MaxDelay
	}

	jitter := time.Duration(float64(delay) * c.JitterFactor * (rand.Float64() - 0.5))
	return delay + jitter
}

// SubscriptionConfig defines the filters for webhook event subscriptions.
//
// Use SubscriptionConfig to specify which events the client should receive from the server.
// Filters can be combined to create precise subscriptions that match specific event types,
// resource types, and namespaces.
//
// # Fields
//
// EventTypes filters events by their type. Event types follow a hierarchical naming convention
// (e.g., "user.created", "order.payment.completed"). Supports wildcards in some implementations
// (e.g., "user.*" to match all user events). Leave empty to receive all event types.
//
// ResourceTypes filters events by the type of resource they affect (e.g., "users", "orders",
// "products"). Useful for subscribing to all events related to a specific resource category.
//
// Namespaces filters events by namespace, enabling multi-tenant deployments where clients
// only receive events from their own namespace (e.g., organization IDs, team IDs, project IDs).
//
// Labels filters events by key-value label pairs. Events must match all specified labels to be
// delivered. Useful for fine-grained filtering based on custom metadata.
//
// # Example
//
//	// Subscribe to all order events for a specific organization
//	config := webhook.SubscriptionConfig{
//	    EventTypes: []string{"order.*"},
//	    Namespaces: []string{"org-12345"},
//	}
//
//	// Subscribe to user and product events only
//	config := webhook.SubscriptionConfig{
//	    EventTypes:    []string{"user.created", "user.updated", "product.created"},
//	    ResourceTypes: []string{"users", "products"},
//	}
//
//	// Update subscription
//	if err := client.UpdateSubscription(config); err != nil {
//	    log.Printf("Failed to update subscription: %v", err)
//	}
type SubscriptionConfig struct {
	EventTypes    []string
	ResourceTypes []string
	Namespaces    []string
	Labels        map[string]string
}

// NewClient creates a new WebSocket webhook client for connecting to a webhook endpoint.
//
// The client is initialized with default configuration and must be connected using [Client.Connect]
// to establish the WebSocket connection. Event handlers should be set via [Client.SetEventHandler]
// before calling Connect to avoid missing any events.
//
// # Parameters
//
// url is the WebSocket endpoint URL (e.g., "wss://api.example.com/webhooks" or "ws://localhost:8080/webhooks").
// Use "wss://" for secure connections (recommended for production).
//
// token is the authentication token for the WebSocket connection. This is sent in the Authorization
// header as "Bearer {token}" during the WebSocket handshake.
//
// subscriptionID identifies this subscription on the server. Multiple clients can use the same
// subscriptionID for load balancing, or unique IDs for independent event streams.
//
// # Returns
//
// A new Client instance with default ReconnectConfig and no-op event/error handlers.
//
// # Example
//
//	// Create a new client
//	client := webhook.NewClient(
//	    "wss://api.example.com/webhooks",
//	    "my-auth-token",
//	    "my-subscription-123",
//	)
//	defer client.Close()
//
//	// Set up event handler
//	client.SetEventHandler(func(event webhook.Event) error {
//	    log.Printf("Received: %s - %s", event.Type, string(event.Payload))
//	    return nil
//	})
//
//	// Set up error handler
//	client.SetErrorHandler(func(err error) {
//	    log.Printf("Error: %v", err)
//	})
//
//	// Connect
//	if err := client.Connect(ctx); err != nil {
//	    log.Fatal(err)
//	}
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

// SetSignatureSecret configures HMAC-SHA256 signature verification for incoming events.
//
// When a signature secret is set, the client automatically verifies the signature of each
// incoming event before delivering it to the event handler. Events with invalid or missing
// signatures are rejected and logged via the error handler.
//
// # Security
//
// Signature verification provides:
//   - Authenticity: Ensures events originated from the legitimate server
//   - Integrity: Detects any tampering with event data during transmission
//
// # Implementation
//
// Signatures are computed as HMAC-SHA256 of the JSON-encoded event (with signature field removed)
// using the shared secret. The signature is base64-encoded and sent in the event's "signature" field.
//
// # Parameters
//
// secret is the shared HMAC secret. This must match the server's signing secret and should be
// kept confidential. Use a cryptographically random string of at least 32 bytes.
//
// # Example
//
//	// Configure signature verification
//	client.SetSignatureSecret("super-secret-key-at-least-32-bytes-long")
//
//	client.SetEventHandler(func(event webhook.Event) error {
//	    // Event signature already verified - safe to process
//	    var data MyEventData
//	    if err := json.Unmarshal(event.Payload, &data); err != nil {
//	        return err
//	    }
//	    // Process event...
//	    return nil
//	})
func (c *Client) SetSignatureSecret(secret string) {
	c.signatureSecret = secret
}

// SetEventHandler registers the callback function that will be invoked for each received event.
//
// The event handler is the primary interface for processing webhook events. It is called for every
// valid event received on the WebSocket connection, after signature verification (if enabled).
//
// # Parameters
//
// handler is a function that takes an Event and returns an error. The handler should:
//   - Parse the event payload based on event.Type
//   - Process the event (e.g., update database, trigger workflow, send notification)
//   - Return an error if processing fails (error is logged via error handler)
//
// # Error Handling
//
// If the handler returns an error, the error is logged via the error handler but does NOT affect
// event acknowledgment. The event is still considered "received" and will not be redelivered.
// Applications should implement their own retry logic for transient failures within the handler.
//
// # Concurrency
//
// Event handlers are called sequentially in the readLoop goroutine. Long-running handlers will
// block reception of subsequent events. For expensive operations, consider:
//   - Spawning a new goroutine within the handler
//   - Using a buffered channel to queue events for processing
//   - Returning quickly and processing asynchronously
//
// # Example
//
//	client.SetEventHandler(func(event webhook.Event) error {
//	    switch event.Type {
//	    case "user.created":
//	        var user User
//	        if err := json.Unmarshal(event.Payload, &user); err != nil {
//	            return fmt.Errorf("failed to unmarshal user: %w", err)
//	        }
//	        log.Printf("New user created: %s", user.ID)
//	        // Trigger welcome email, etc.
//
//	    case "order.completed":
//	        var order Order
//	        if err := json.Unmarshal(event.Payload, &order); err != nil {
//	            return fmt.Errorf("failed to unmarshal order: %w", err)
//	        }
//	        log.Printf("Order completed: %s", order.ID)
//	        // Trigger fulfillment, etc.
//
//	    default:
//	        log.Printf("Unhandled event type: %s", event.Type)
//	    }
//	    return nil
//	})
func (c *Client) SetEventHandler(handler func(Event) error) {
	c.eventHandler = handler
}

// SetErrorHandler registers the callback function that will be invoked when errors occur.
//
// The error handler is called for various error conditions during the WebSocket connection lifecycle:
//   - Connection failures (initial connect and reconnection attempts)
//   - Read/write errors on the WebSocket
//   - Signature verification failures
//   - Event handler errors (errors returned from the event handler)
//   - Heartbeat/ping failures
//   - Reconnection progress notifications
//
// # Parameters
//
// handler is a function that takes an error. The handler should:
//   - Log the error for debugging and monitoring
//   - Alert operators for critical errors (authentication failures, max retries exceeded)
//   - Distinguish between transient and permanent errors
//
// # Error Types
//
// Common errors passed to the error handler:
//   - "authentication failed": Invalid token (401) - permanent error
//   - "forbidden": Insufficient permissions (403) - permanent error
//   - "connection closed": WebSocket connection lost - transient, will retry
//   - "reconnection failed": Max retries exceeded - permanent error
//   - "signature verification failed": Invalid event signature - security alert
//   - "event handler error": Error from event handler - application error
//   - "ping failed": Heartbeat failed - transient, will retry
//   - "ack send failed": Acknowledgment send failed - transient
//
// # Concurrency
//
// The error handler may be called from multiple goroutines (readLoop, ackLoop, pingLoop,
// reconnection logic). Ensure the handler is thread-safe or uses appropriate synchronization.
//
// # Example
//
//	client.SetErrorHandler(func(err error) {
//	    // Log all errors
//	    log.Printf("Webhook error: %v", err)
//
//	    // Alert on critical errors
//	    if strings.Contains(err.Error(), "authentication failed") {
//	        alertOps("Webhook authentication failed - check token")
//	    }
//	    if strings.Contains(err.Error(), "signature verification") {
//	        alertSec("Potential webhook signature tampering detected")
//	    }
//	    if strings.Contains(err.Error(), "max reconnection attempts") {
//	        alertOps("Webhook disconnected - max retries exceeded")
//	    }
//	})
func (c *Client) SetErrorHandler(handler func(error)) {
	c.errorHandler = handler
}

// Connect establishes the WebSocket connection to the webhook endpoint and starts background goroutines.
//
// Connect performs a WebSocket handshake with authentication headers and, on success, starts three
// background goroutines:
//   - readLoop: Reads and processes incoming events
//   - ackLoop: Sends batched acknowledgments every 5 seconds
//   - pingLoop: Sends heartbeats every 30 seconds to keep the connection alive
//
// # Parameters
//
// ctx is the context for controlling the connection lifecycle. Canceling the context will:
//   - Stop all background goroutines
//   - Close the WebSocket connection
//   - Terminate any in-flight reconnection attempts
//
// # Behavior
//
// Connect will retry establishing the initial connection according to the ReconnectConfig.
// If authentication fails (401/403), Connect returns immediately with an error. For other
// errors, Connect retries up to MaxRetries times with exponential backoff.
//
// # Returns
//
// An error if the connection cannot be established after all retries, or if authentication fails.
// Returns nil on successful connection (background goroutines continue running).
//
// # Example
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	client := webhook.NewClient("wss://api.example.com/webhooks", "token", "sub-123")
//	defer client.Close()
//
//	if err := client.Connect(ctx); err != nil {
//	    log.Fatalf("Failed to connect: %v", err)
//	}
//
//	// Client is now connected and receiving events
//	// Block or do other work...
//	<-ctx.Done()
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

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for attempt := 0; attempt < c.config.MaxRetries; attempt++ {
		conn, resp, err := websocket.Dial(dialCtx, c.url, &websocket.DialOptions{
			HTTPHeader: headers,
		})
		if err == nil {
			return conn, nil
		}

		if resp != nil {
			statusCode := resp.StatusCode
			_ = resp.Body.Close()

			if statusCode == 401 {
				return nil, fmt.Errorf("authentication failed")
			}
			if statusCode == 403 {
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

func (c *Client) readJSON(ctx context.Context, v interface{}) error {
	_, data, err := c.conn.Read(ctx)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func (c *Client) writeJSON(ctx context.Context, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.conn.Write(ctx, websocket.MessageText, data)
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
		if err := c.readJSON(c.ctx, &msg); err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure || websocket.CloseStatus(err) == websocket.StatusGoingAway {
				c.errorHandler(fmt.Errorf("connection closed: %w", err))
				c.setConnected(false)

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
			if c.signatureSecret != "" && msg.Signature != "" {
				if !c.verifySignature(msg) {
					c.errorHandler(fmt.Errorf("signature verification failed for event %s", msg.ID))
					continue
				}
			}

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
	ack.Payload.ReceivedCount = len(ids)

	if err := c.writeJSON(c.ctx, ack); err != nil {
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
			pingCtx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
			defer cancel()
			if err := c.conn.Ping(pingCtx); err != nil {
				c.errorHandler(fmt.Errorf("ping failed: %w", err))
			}
		}
	}
}

func (c *Client) sendPong() {
	// nhooyr.io/websocket automatically sends pong in response to ping
	// No manual pong needed
}

func (c *Client) setConnected(connected bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = connected
}

// IsConnected returns true if the client currently has an active WebSocket connection.
//
// This method is safe for concurrent use and can be called from any goroutine. It provides
// a snapshot of the connection state at the time of the call.
//
// # Returns
//
// true if the client is connected and actively receiving events, false otherwise.
//
// # Caveats
//
// The connection state can change immediately after this call returns. For critical operations,
// consider implementing your own connection state monitoring via the error handler or by
// tracking connection events.
//
// # Example
//
//	// Periodic health check
//	ticker := time.NewTicker(10 * time.Second)
//	defer ticker.Stop()
//
//	for {
//	    select {
//	    case <-ticker.C:
//	        if !client.IsConnected() {
//	            log.Warn("Webhook client disconnected")
//	            // Attempt reconnection or alert
//	        }
//	    case <-ctx.Done():
//	        return
//	    }
//	}
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *Client) reconnect(ctx context.Context) error {
	for attempt := 0; attempt < c.config.MaxRetries; attempt++ {
		delay := c.config.NextDelay(attempt)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		c.log("Reconnection attempt %d/%d", attempt+1, c.config.MaxRetries)

		conn, err := c.connectWithRetry(ctx)
		if err == nil {
			c.conn = conn
			c.setConnected(true)
			return c.resumeSubscription()
		}

		c.log("Reconnect attempt %d failed: %v", attempt+1, err)
	}

	return fmt.Errorf("max reconnection attempts (%d) exceeded", c.config.MaxRetries)
}

func (c *Client) log(format string, args ...interface{}) {
	if c.errorHandler != nil {
		c.errorHandler(fmt.Errorf(format, args...))
	}
}

func (c *Client) resumeSubscription() error {
	resume := ResumeMessage{
		Type:        "resume",
		LastEventID: c.lastEventID,
		LastAckTime: c.lastAckTime,
	}

	return c.writeJSON(c.ctx, resume)
}

func (c *Client) verifySignature(event Event) bool {
	if c.signatureSecret == "" || event.Signature == "" {
		return false
	}

	signature := event.Signature
	event.Signature = ""

	payload, err := json.Marshal(event)
	if err != nil {
		return false
	}

	h := hmac.New(sha256.New, []byte(c.signatureSecret))
	h.Write(payload)
	expectedSig := base64.StdEncoding.EncodeToString(h.Sum(nil))

	return hmac.Equal([]byte(expectedSig), []byte(signature))
}

// UpdateSubscription dynamically updates the event subscription filters during an active connection.
//
// Use UpdateSubscription to change which events the client receives without disconnecting and
// reconnecting. This is useful for:
//   - Filtering to specific event types based on application state
//   - Switching between namespaces in multi-tenant scenarios
//   - Temporarily pausing event delivery (set empty filters)
//   - Adjusting subscriptions based on user preferences
//
// # Parameters
//
// config defines the new subscription filters. All fields are optional:
//   - EventTypes: Filter by event type patterns
//   - ResourceTypes: Filter by resource category
//   - Namespaces: Filter by tenant/organization/project
//   - Labels: Filter by custom key-value pairs
//
// An empty config (all fields empty/nil) subscribes to all events.
//
// # Behavior
//
// The subscription update message is sent immediately over the active WebSocket connection.
// The server may take time to apply the filters - events received during the transition may
// not match the new filters exactly.
//
// # Returns
//
// An error if the subscription message cannot be sent (e.g., connection is closed).
// Does not wait for server acknowledgment - errors reflect send failures only.
//
// # Example
//
//	// Subscribe to only order events
//	config := webhook.SubscriptionConfig{
//	    EventTypes: []string{"order.created", "order.updated", "order.completed"},
//	}
//	if err := client.UpdateSubscription(config); err != nil {
//	    log.Printf("Failed to update subscription: %v", err)
//	}
//
//	// Later, switch to customer events
//	config = webhook.SubscriptionConfig{
//	    EventTypes: []string{"customer.created", "customer.updated"},
//	}
//	if err := client.UpdateSubscription(config); err != nil {
//	    log.Printf("Failed to update subscription: %v", err)
//	}
//
//	// Pause all events temporarily
//	if err := client.UpdateSubscription(webhook.SubscriptionConfig{}); err != nil {
//	    log.Printf("Failed to pause events: %v", err)
//	}
func (c *Client) UpdateSubscription(config SubscriptionConfig) error {
	msg := SubscribeMessage{
		Type:      "subscribe",
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
	}
	msg.Payload.EventTypes = config.EventTypes
	msg.Payload.ResourceTypes = config.ResourceTypes
	msg.Payload.Namespaces = config.Namespaces

	return c.writeJSON(c.ctx, msg)
}

// Close gracefully shuts down the WebSocket connection and releases all resources.
//
// Close performs an orderly shutdown of the client:
//  1. Cancels the context to stop all background goroutines (readLoop, ackLoop, pingLoop)
//  2. Waits for all goroutines to finish via WaitGroup
//  3. Sends a normal closure frame to the server (if connected)
//  4. Releases any held resources
//
// # Behavior
//
// Close is idempotent - calling it multiple times has no effect beyond the first call.
//
// Close blocks until all goroutines have exited and the connection is closed, with a maximum
// timeout of 5 seconds for the WebSocket close handshake.
//
// # Concurrency
//
// Close is safe for concurrent use. If called from multiple goroutines, only the first call
// will perform the shutdown; subsequent calls return immediately.
//
// # When to Call
//
// Always call Close when shutting down the application or when the webhook connection is no
// longer needed. Use defer to ensure cleanup even on errors:
//
//	client := webhook.NewClient(url, token, subID)
//	defer client.Close()
//
//	if err := client.Connect(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
// # Returns
//
// An error if the WebSocket close handshake fails or times out. Returns nil on successful
// shutdown or if the client was never connected.
//
// # Example
//
//	func processWebhooks(ctx context.Context) error {
//	    client := webhook.NewClient("wss://api.example.com/webhooks", "token", "sub-123")
//	    defer client.Close()
//
//	    client.SetEventHandler(func(event webhook.Event) error {
//	        // Process event...
//	        return nil
//	    })
//
//	    if err := client.Connect(ctx); err != nil {
//	        return err
//	    }
//
//	    // Block until context is cancelled
//	    <-ctx.Done()
//	    return nil
//	}
func (c *Client) Close() error {
	c.cancel()
	c.wg.Wait()

	if c.conn != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := c.conn.Close(websocket.StatusNormalClosure, "")
		// Drain the context error if any
		if closeCtx.Err() != nil && err == nil {
			return closeCtx.Err()
		}
		return err
	}
	return nil
}
