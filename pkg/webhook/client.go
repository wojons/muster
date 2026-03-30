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

type Event struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature,omitempty"`
}

type AckMessage struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Payload struct {
		AcknowledgedIDs []string `json:"acknowledged_ids"`
		ReceivedCount   int      `json:"received_count,omitempty"`
	} `json:"payload"`
}

type ResumeMessage struct {
	Type        string    `json:"type"`
	LastEventID string    `json:"last_event_id"`
	LastAckTime time.Time `json:"last_ack_time"`
}

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

type ReconnectConfig struct {
	MaxRetries   int
	BaseDelay    time.Duration
	MaxDelay     time.Duration
	JitterFactor float64
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

	delay := c.BaseDelay * time.Duration(math.Pow(2, float64(attempt)))
	if delay > c.MaxDelay {
		delay = c.MaxDelay
	}

	jitter := time.Duration(float64(delay) * c.JitterFactor * (rand.Float64() - 0.5))
	return delay + jitter
}

type SubscriptionConfig struct {
	EventTypes    []string
	ResourceTypes []string
	Namespaces    []string
	Labels        map[string]string
}

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

func (c *Client) SetSignatureSecret(secret string) {
	c.signatureSecret = secret
}

func (c *Client) SetEventHandler(handler func(Event) error) {
	c.eventHandler = handler
}

func (c *Client) SetErrorHandler(handler func(error)) {
	c.errorHandler = handler
}

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
