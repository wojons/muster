// Package wstest provides a TEST-ONLY in-process GraphQL server speaking the
// graphql-transport-ws subprotocol over a loopback WebSocket — the §4.6
// network-free fixture for the subscriptions adapter (PROTO-022), mirroring
// pkg/protocol/asyncapi/kafkatest. It is also the fixture the subscriptions
// demo leg (demo/demos/22-graphql-subscriptions.sh) compiles via `go run`.
//
// The server does not run a GraphQL engine: a subscription to field F
// receives `Events` next frames of the shape
//
//	{"data": {"F": {"seq": N, "msg": "event N"}}}
//
// then a complete frame. Enough of the protocol is enforced to make the
// fixture a real counterpart, not a mirror: a client that skips
// connection_init, offers no subprotocol, or subscribes before the ack is
// refused with a connection error; a client that sends its own complete
// before the server finishes is recorded as an unsubscribe (Unsubscribes()).
package wstest

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// DefaultEvents is how many next frames one subscription delivers when the
// server's Events field is unset.
const DefaultEvents = 5

// Server is the in-process graphql-transport-ws fixture.
type Server struct {
	// Events is the number of next frames each subscription delivers
	// before the server completes it (<=0 selects DefaultEvents).
	Events int

	// Delay separates consecutive next frames, so a caller's
	// max-duration bound can fire mid-stream (0 = as fast as possible).
	Delay time.Duration

	// FailAt, when set, makes the server send an `error` frame instead of
	// the next frame with that sequence number. A pointer so the default
	// (unset) never collides with a legitimate position 0.
	FailAt *int

	// ErrorPayload is the error frame's payload (JSON array of GraphQL
	// errors); empty selects one standard error object.
	ErrorPayload string

	mu           sync.Mutex
	listener     net.Listener
	httpSrv      *http.Server
	url          string
	unsubscribes int
	earlyStop    map[*subscriptionRun]bool
}

// subscriptionRun is one accepted subscription the server is feeding.
type subscriptionRun struct {
	stop chan struct{}
	done chan struct{}
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"graphql-transport-ws"},
	CheckOrigin:  func(*http.Request) bool { return true },
}

// Start listens on a loopback port chosen by the OS and serves until Stop.
// The returned server's URL() is the ws:// endpoint clients dial; it also
// accepts plain HTTP POST (an echo GraphQL handler) so a caller can use one
// endpoint for discovery-style probes and subscriptions alike.
func Start() (*Server, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{
		listener:  l,
		url:       "ws://" + l.Addr().String() + "/graphql",
		earlyStop: map[*subscriptionRun]bool{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", s.handle)
	s.httpSrv = &http.Server{Handler: mux}
	go func() { _ = s.httpSrv.Serve(l) }() // closed via Stop
	return s, nil
}

// URL is the endpoint clients dial.
func (s *Server) URL() string { return s.url }

// Unsubscribes reports how many client complete frames arrived before the
// server finished its own stream — the clean-unsubscribe evidence.
func (s *Server) Unsubscribes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unsubscribes
}

// Stop closes every live subscription and shuts the listener down.
func (s *Server) Stop() {
	s.mu.Lock()
	runs := make([]*subscriptionRun, 0, len(s.earlyStop))
	for run := range s.earlyStop {
		runs = append(runs, run)
	}
	s.mu.Unlock()
	for _, run := range runs {
		close(run.stop)
	}
	_ = s.httpSrv.Close()
}

// handle is the upgrade endpoint plus a plain-POST echo for readiness.
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Upgrade") == "" {
		// Readiness probe / plain POST echo.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"ready":true}}`))
		return
	}
	if !containsSubprotocol(r.Header.Get("Sec-WebSocket-Protocol")) {
		http.Error(w, "graphql-transport-ws subprotocol required", http.StatusBadRequest)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	go s.serveConn(conn)
}

func containsSubprotocol(header string) bool {
	for _, p := range regexp.MustCompile(`\s*,\s*`).Split(header, -1) {
		if p == "graphql-transport-ws" {
			return true
		}
	}
	return false
}

var subscribeFieldRe = regexp.MustCompile(`(?s)\{\s*(\w+)`)

// serveConn speaks the protocol with one client until it disconnects.
func (s *Server) serveConn(conn *websocket.Conn) {
	defer func() { _ = conn.Close() }()

	inited := false
	for {
		var frame struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Payload struct {
				Query string `json:"query"`
			} `json:"payload"`
		}
		if err := conn.ReadJSON(&frame); err != nil {
			return
		}
		switch frame.Type {
		case "connection_init":
			if inited {
				_ = conn.WriteJSON(map[string]interface{}{"type": "connection_error"})
				return
			}
			inited = true
			_ = conn.WriteJSON(map[string]interface{}{"type": "connection_ack"})
		case "subscribe":
			if !inited {
				_ = conn.WriteJSON(map[string]any{"id": frame.ID, "type": "connection_error"})
				return
			}
			s.runSubscription(conn, frame.ID, frame.Payload.Query)
		case "complete":
			s.mu.Lock()
			s.unsubscribes++
			for run := range s.earlyStop {
				select {
				case <-run.stop:
				default:
					close(run.stop)
				}
			}
			s.mu.Unlock()
		}
	}
}

// runSubscription feeds one subscription: Events next frames then complete.
func (s *Server) runSubscription(conn *websocket.Conn, id, query string) {
	s.mu.Lock()
	events := s.Events
	delay := s.Delay
	errPayload := s.ErrorPayload
	s.mu.Unlock()
	if events <= 0 {
		events = DefaultEvents
	}
	if errPayload == "" {
		errPayload = `[{"message":"boom: the fixture failed the stream"}]`
	}

	field := subscribeFieldRe.FindStringSubmatch(query)
	name := "event"
	if len(field) >= 2 {
		name = field[1]
	}

	run := &subscriptionRun{stop: make(chan struct{}), done: make(chan struct{})}
	s.mu.Lock()
	s.earlyStop[run] = true
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.earlyStop, run)
			s.mu.Unlock()
			close(run.done)
		}()
		for i := 0; i < events; i++ {
			select {
			case <-run.stop:
				return
			default:
			}
			if delay > 0 {
				select {
				case <-run.stop:
					return
				case <-time.After(delay):
				}
			}
			if s.FailAt != nil && i == *s.FailAt {
				_ = conn.WriteJSON(map[string]any{"id": id, "type": "error", "payload": json.RawMessage(errPayload)})
				return
			}
			next := map[string]any{
				"id":      id,
				"type":    "next",
				"payload": map[string]any{"data": map[string]any{name: map[string]any{"seq": i, "msg": fmt.Sprintf("event %d", i)}}},
			}
			if err := conn.WriteJSON(next); err != nil {
				return
			}
		}
		_ = conn.WriteJSON(map[string]any{"id": id, "type": "complete"})
	}()
}
