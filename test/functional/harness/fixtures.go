package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestServer represents a test HTTP server
type TestServer struct {
	server *httptest.Server
	t      *testing.T
}

// NewTestServer creates a new test HTTP server
func NewTestServer(t *testing.T, handler http.HandlerFunc) *TestServer {
	return &TestServer{
		server: httptest.NewServer(handler),
		t:      t,
	}
}

// URL returns the server URL
func (ts *TestServer) URL() string {
	return ts.server.URL
}

// Close closes the server
func (ts *TestServer) Close() {
	ts.server.Close()
}

// TestEnvironment represents a test environment
type TestEnvironment struct {
	t       *testing.T
	system  *System
	server  *TestServer
	tempDir string
	configs map[string]string
}

// NewTestEnvironment creates a new test environment
func NewTestEnvironment(t *testing.T) *TestEnvironment {
	tempDir, err := os.MkdirTemp("", "muster-env-*")
	require.NoError(t, err)

	return &TestEnvironment{
		t:       t,
		tempDir: tempDir,
		configs: make(map[string]string),
	}
}

// Setup sets up the test environment
func (te *TestEnvironment) Setup() *System {
	system := NewSystem(te.t)
	system.Build()
	system.Start()
	te.system = system

	te.t.Cleanup(func() {
		te.Cleanup()
	})

	return system
}

// SetConfig sets a configuration value
func (te *TestEnvironment) SetConfig(key, value string) {
	te.configs[key] = value
}

// GetConfig gets a configuration value
func (te *TestEnvironment) GetConfig(key string) string {
	return te.configs[key]
}

// GetTempDir returns the temp directory
func (te *TestEnvironment) GetTempDir() string {
	return te.tempDir
}

// Cleanup cleans up the test environment
func (te *TestEnvironment) Cleanup() {
	if te.server != nil {
		te.server.Close()
	}
	if te.system != nil {
		te.system.Stop()
	}
	if te.tempDir != "" {
		os.RemoveAll(te.tempDir)
	}
}

// WriteTempFile writes content to a temp file
func (te *TestEnvironment) WriteTempFile(name, content string) string {
	path := filepath.Join(te.tempDir, name)
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(te.t, err)
	return path
}

// CreateMockServer creates a mock HTTP server
func (te *TestEnvironment) CreateMockServer(handler http.HandlerFunc) *TestServer {
	server := NewTestServer(te.t, handler)
	te.server = server
	return server
}

// TestFixture represents a reusable test fixture
type TestFixture struct {
	name        string
	description string
	setup       func(*testing.T) interface{}
	teardown    func(*testing.T, interface{})
}

// NewTestFixture creates a new test fixture
func NewTestFixture(name, description string) *TestFixture {
	return &TestFixture{
		name:        name,
		description: description,
	}
}

// Setup setups the fixture
func (tf *TestFixture) Setup(fn func(*testing.T) interface{}) *TestFixture {
	tf.setup = fn
	return tf
}

// Teardown tears down the fixture
func (tf *TestFixture) TearDown(fn func(*testing.T, interface{})) *TestFixture {
	tf.teardown = fn
	return tf
}

// Apply applies the fixture to a test
func (tf *TestFixture) Apply(t *testing.T, fn func(interface{})) {
	t.Run(tf.name, func(t *testing.T) {
		var fixture interface{}

		if tf.setup != nil {
			fixture = tf.setup(t)
		}

		defer func() {
			if tf.teardown != nil {
				tf.teardown(t, fixture)
			}
		}()

		fn(fixture)
	})
}

// JSONFixture provides JSON test fixtures
type JSONFixture struct {
	data map[string]interface{}
}

// NewJSONFixture creates a new JSON fixture
func NewJSONFixture(jsonStr string) *JSONFixture {
	var data map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &data)
	if err != nil {
		data = make(map[string]interface{})
	}
	return &JSONFixture{data: data}
}

// Get gets a value
func (jf *JSONFixture) Get(key string) interface{} {
	return jf.data[key]
}

// Set sets a value
func (jf *JSONFixture) Set(key string, value interface{}) {
	jf.data[key] = value
}

// String returns JSON string
func (jf *JSONFixture) String() string {
	data, _ := json.Marshal(jf.data)
	return string(data)
}

// Map returns the underlying map
func (jf *JSONFixture) Map() map[string]interface{} {
	return jf.data
}

// FileFixture provides file-based test fixtures
type FileFixture struct {
	baseDir string
}

// NewFileFixture creates a new file fixture
func NewFileFixture(baseDir string) *FileFixture {
	return &FileFixture{baseDir: baseDir}
}

// Read reads a file
func (ff *FileFixture) Read(name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(ff.baseDir, name))
	return string(data), err
}

// Write writes a file
func (ff *FileFixture) Write(name, content string) error {
	return os.WriteFile(filepath.Join(ff.baseDir, name), []byte(content), 0644)
}

// Path returns the full path
func (ff *FileFixture) Path(name string) string {
	return filepath.Join(ff.baseDir, name)
}

// HTTPFixture provides HTTP test fixtures
type HTTPFixture struct {
	responses map[string]*http.Response
	bodies    map[string][]byte
}

// NewHTTPFixture creates a new HTTP fixture
func NewHTTPFixture() *HTTPFixture {
	return &HTTPFixture{
		responses: make(map[string]*http.Response),
		bodies:    make(map[string][]byte),
	}
}

// SetResponse sets a response for a path
func (hf *HTTPFixture) SetResponse(path string, statusCode int, body string) {
	_ = httptest.NewRequest("GET", path, nil)
	resp := httptest.NewRecorder()
	resp.WriteHeader(statusCode)
	resp.WriteString(body)
	hf.responses[path] = &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	hf.bodies[path] = []byte(body)
}

// GetResponse gets a response for a path
func (hf *HTTPFixture) GetResponse(path string) (*http.Response, bool) {
	resp, ok := hf.responses[path]
	return resp, ok
}

// MockClient returns an HTTP client with mocking
func (hf *HTTPFixture) MockClient() *http.Client {
	return &http.Client{
		Transport: &roundTripper{
			fixture: hf,
		},
	}
}

type roundTripper struct {
	fixture *HTTPFixture
}

func (rt *roundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, ok := rt.fixture.GetResponse(r.URL.Path)
	if ok {
		return resp, nil
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("Not found")),
	}, nil
}

// ServerFixture provides server test fixtures
type ServerFixture struct {
	servers map[string]*httptest.Server
}

// NewServerFixture creates a new server fixture
func NewServerFixture() *ServerFixture {
	return &ServerFixture{
		servers: make(map[string]*httptest.Server),
	}
}

// AddServer adds a server
func (sf *ServerFixture) AddServer(name string, handler http.HandlerFunc) string {
	server := httptest.NewServer(handler)
	sf.servers[name] = server
	return server.URL
}

// GetServer gets a server by name
func (sf *ServerFixture) GetServer(name string) (*httptest.Server, bool) {
	server, ok := sf.servers[name]
	return server, ok
}

// RemoveServer removes a server
func (sf *ServerFixture) RemoveServer(name string) {
	if server, ok := sf.servers[name]; ok {
		server.Close()
		delete(sf.servers, name)
	}
}

// CloseAll closes all servers
func (sf *ServerFixture) CloseAll() {
	for _, server := range sf.servers {
		server.Close()
	}
	sf.servers = make(map[string]*httptest.Server)
}

// ConfigFixture provides configuration test fixtures
type ConfigFixture struct {
	configs map[string]string
}

// NewConfigFixture creates a new config fixture
func NewConfigFixture() *ConfigFixture {
	return &ConfigFixture{
		configs: make(map[string]string),
	}
}

// Set sets a config value
func (cf *ConfigFixture) Set(key, value string) {
	cf.configs[key] = value
}

// Get gets a config value
func (cf *ConfigFixture) Get(key string) (string, bool) {
	val, ok := cf.configs[key]
	return val, ok
}

// All returns all configs
func (cf *ConfigFixture) All() map[string]string {
	return cf.configs
}

// ToYAML converts to YAML
func (cf *ConfigFixture) ToYAML() string {
	var builder strings.Builder
	for key, value := range cf.configs {
		builder.WriteString(fmt.Sprintf("%s: %s\n", key, value))
	}
	return builder.String()
}

// ToJSON converts to JSON
func (cf *ConfigFixture) ToJSON() string {
	data, _ := json.Marshal(cf.configs)
	return string(data)
}

// StateFixture provides state test fixtures
type StateFixture struct {
	states []State
}

// NewStateFixture creates a new state fixture
func NewStateFixture() *StateFixture {
	return &StateFixture{
		states: []State{StateStopped},
	}
}

// AddState adds a state
func (sf *StateFixture) AddState(state State) {
	sf.states = append(sf.states, state)
}

// GetStates gets all states
func (sf *StateFixture) GetStates() []State {
	return make([]State, len(sf.states))
}

// HasState checks if state exists
func (sf *StateFixture) HasState(state State) bool {
	for _, s := range sf.states {
		if s == state {
			return true
		}
	}
	return false
}

// Reset resets the fixture
func (sf *StateFixture) Reset() {
	sf.states = []State{StateStopped}
}

// DatabaseFixture provides database test fixtures
type DatabaseFixture struct {
	queries []string
}

// NewDatabaseFixture creates a new database fixture
func NewDatabaseFixture() *DatabaseFixture {
	return &DatabaseFixture{
		queries: make([]string, 0),
	}
}

// AddQuery adds a query
func (df *DatabaseFixture) AddQuery(query string) {
	df.queries = append(df.queries, query)
}

// GetQueries gets all queries
func (df *DatabaseFixture) GetQueries() []string {
	return make([]string, len(df.queries))
}

// Reset resets the fixture
func (df *DatabaseFixture) Reset() {
	df.queries = make([]string, 0)
}

// ContextFixture provides context test fixtures
type ContextFixture struct {
	ctx      context.Context
	cancel   context.CancelFunc
	timeouts map[string]time.Duration
}

// NewContextFixture creates a new context fixture
func NewContextFixture() *ContextFixture {
	ctx, cancel := context.WithCancel(context.Background())
	return &ContextFixture{
		ctx:      ctx,
		cancel:   cancel,
		timeouts: make(map[string]time.Duration),
	}
}

// Context returns the context
func (cf *ContextFixture) Context() context.Context {
	return cf.ctx
}

// Cancel cancels the context
func (cf *ContextFixture) Cancel() {
	cf.cancel()
}

// WithTimeout creates context with timeout
func (cf *ContextFixture) WithTimeout(t *testing.T, timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	cf.ctx = ctx
	cf.cancel = cancel
	return ctx
}

// WithDeadline creates context with deadline
func (cf *ContextFixture) WithDeadline(deadline time.Time) context.Context {
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	cf.ctx = ctx
	cf.cancel = cancel
	return ctx
}

// Cleanup cleans up the fixture
func (cf *ContextFixture) Cleanup() {
	if cf.cancel != nil {
		cf.cancel()
	}
}
