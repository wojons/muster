package grpc

import (
	"context"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/jhump/protoreflect/dynamic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

// testKVSource is the shared test .proto: one service, two unary methods
// (gate: "≥2 unary methods") and all three streaming kinds (PROTO-003).
const testKVSource = `syntax = "proto3";
package kv;

option go_package = "github.com/wojons/muster/testdata/grpc";

// A key-value service with two unary methods and every streaming kind.
service KeyValue {
  // Get returns the value for a key.
  rpc Get(GetRequest) returns (GetResponse);
  // Set stores a value under a key.
  rpc Set(SetRequest) returns (SetResponse);
  // Watch is server-streaming: three events, then the stream ends normally.
  rpc Watch(WatchRequest) returns (stream WatchEvent);
  // Tail is server-streaming and never ends on its own: the client's bound
  // (max-messages, max-duration) or its cancellation is the only way out.
  rpc Tail(TailRequest) returns (stream WatchEvent);
  // Upload is client-streaming: N request messages in, one response out.
  rpc Upload(stream UploadRequest) returns (UploadResponse);
  // Chat is bidirectional: every request message is echoed back until the
  // client stops sending.
  rpc Chat(stream ChatMessage) returns (stream ChatMessage);
  // ChatHold is Chat, except a request whose text is "hold" leaves the stream
  // open after the echo (the cancellation proof).
  rpc ChatHold(stream ChatMessage) returns (stream ChatMessage);
}

message GetRequest {
  string key = 1;
}

message GetResponse {
  string key = 1;
  string value = 2;
}

message SetRequest {
  string key = 1;
  string value = 2;
}

message SetResponse {
  bool ok = 1;
}

message WatchRequest {
  string prefix = 1;
}

message WatchEvent {
  string key = 1;
  string value = 2;
}

message TailRequest {
  string prefix = 1;
}

message UploadRequest {
  string key = 1;
}

message UploadResponse {
  int32 count = 1;
  string joined = 2;
}

message ChatMessage {
  string text = 1;
}
`

// kvServiceDef holds the parsed method descriptors the dynamic test server
// dispatches on.
type kvServiceDef struct {
	get      *desc.MethodDescriptor
	set      *desc.MethodDescriptor
	watch    *desc.MethodDescriptor
	tail     *desc.MethodDescriptor
	upload   *desc.MethodDescriptor
	chat     *desc.MethodDescriptor
	chatHold *desc.MethodDescriptor
}

// parseTestProto parses the in-memory test source into linked descriptors.
func parseTestProto(t *testing.T) []*desc.FileDescriptor {
	t.Helper()
	accessor := protoparse.FileContentsFromMap(map[string]string{"kv.proto": testKVSource})
	parser := &protoparse.Parser{Accessor: accessor}
	fds, err := parser.ParseFiles("kv.proto")
	if err != nil {
		t.Fatalf("parse test proto: %v", err)
	}
	return fds
}

// kvDef extracts the kv.KeyValue service's method descriptors.
func kvDef(t *testing.T) *kvServiceDef {
	t.Helper()
	for _, fd := range parseTestProto(t) {
		for _, svc := range fd.GetServices() {
			if svc.GetFullyQualifiedName() != "kv.KeyValue" {
				continue
			}
			return &kvServiceDef{
				get:      svc.FindMethodByName("Get"),
				set:      svc.FindMethodByName("Set"),
				watch:    svc.FindMethodByName("Watch"),
				tail:     svc.FindMethodByName("Tail"),
				upload:   svc.FindMethodByName("Upload"),
				chat:     svc.FindMethodByName("Chat"),
				chatHold: svc.FindMethodByName("ChatHold"),
			}
		}
	}
	t.Fatal("kv.KeyValue service not found in test proto")
	return nil
}

// kvStore is the in-memory backing store for the test service.
type kvStore struct {
	mu    sync.RWMutex
	items map[string]string
}

func newKVStore() *kvStore { return &kvStore{items: map[string]string{}} }

func (s *kvStore) get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.items[key]
	return v, ok
}

func (s *kvStore) set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = value
}

// serveKV serves a dynamic KeyValue service with a REGISTERED grpc.ServiceDesc
// (dynamic handlers — what codegen would emit, minus the codegen) plus
// reflection on a bufconn listener. Registration matters: reflection can only
// enumerate registered services and resolve their descriptors from the global
// registry, so the file descriptors are registered there too (that is exactly
// what generated pb.go init() does).
//
// inject maps a full method name to a status code the handler returns
// unconditionally (error-mapping proofs); captor (optional) records the
// incoming metadata the server observes (§4.1 proof).
func serveKV(t *testing.T, store *kvStore, inject map[string]codes.Code, captor *metadataCaptor) (*bufconn.Listener, func()) {
	t.Helper()
	def := kvDef(t)

	// Simulate generated-code registration: make the file descriptors
	// resolvable by the reflection service. Register at most once per
	// process — RegisterFile panics on duplicate names (generated pb.go
	// init() runs once for the same reason).
	if _, err := protoregistry.GlobalFiles.FindFileByPath("kv.proto"); err != nil {
		for _, fd := range parseTestProto(t) {
			_ = protoregistry.GlobalFiles.RegisterFile(fd.UnwrapFile())
		}
	}

	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	srv.RegisterService(kvServiceDesc(def, store, inject, captor), nil)
	reflection.Register(srv)
	go func() { _ = srv.Serve(lis) }()
	return lis, srv.Stop
}

// kvServiceDesc builds the ServiceDesc for kv.KeyValue with dynamic handlers.
func kvServiceDesc(def *kvServiceDef, store *kvStore, inject map[string]codes.Code, captor *metadataCaptor) *grpc.ServiceDesc {
	capture := func(ctx context.Context) {
		if captor != nil {
			if md, ok := metadata.FromIncomingContext(ctx); ok {
				captor.capture(md)
			}
		}
	}
	return &grpc.ServiceDesc{
		ServiceName: "kv.KeyValue",
		Methods: []grpc.MethodDesc{
			{
				MethodName: "Get",
				Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
					capture(ctx)
					req := dynamic.NewMessage(def.get.GetInputType())
					if err := dec(req); err != nil {
						return nil, err
					}
					if code, ok := inject["/kv.KeyValue/Get"]; ok {
						return nil, status.Error(code, "injected failure for Get")
					}
					key, _ := req.GetFieldByName("key").(string)
					if key == "" {
						return nil, status.Error(codes.InvalidArgument, "key must not be empty")
					}
					value, ok := store.get(key)
					if !ok {
						return nil, status.Error(codes.NotFound, "key not found: "+key)
					}
					resp := dynamic.NewMessage(def.get.GetOutputType())
					resp.SetFieldByName("key", key)
					resp.SetFieldByName("value", value)
					return resp, nil
				},
			},
			{
				MethodName: "Set",
				Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
					capture(ctx)
					req := dynamic.NewMessage(def.set.GetInputType())
					if err := dec(req); err != nil {
						return nil, err
					}
					if code, ok := inject["/kv.KeyValue/Set"]; ok {
						return nil, status.Error(code, "injected failure for Set")
					}
					key, _ := req.GetFieldByName("key").(string)
					if key == "" {
						return nil, status.Error(codes.InvalidArgument, "key must not be empty")
					}
					value, _ := req.GetFieldByName("value").(string)
					store.set(key, value)
					resp := dynamic.NewMessage(def.set.GetOutputType())
					resp.SetFieldByName("ok", true)
					return resp, nil
				},
			},
		},
		Streams: []grpc.StreamDesc{
			{
				StreamName:    "Watch",
				ServerStreams: true,
				Handler: func(srv interface{}, stream grpc.ServerStream) error {
					req := dynamic.NewMessage(def.watch.GetInputType())
					if err := stream.RecvMsg(req); err != nil {
						return err
					}
					prefix, _ := req.GetFieldByName("prefix").(string)
					for i := 0; i < 3; i++ {
						resp := dynamic.NewMessage(def.watch.GetOutputType())
						resp.SetFieldByName("key", prefix+"-"+string(rune('0'+i)))
						resp.SetFieldByName("value", "event")
						if err := stream.SendMsg(resp); err != nil {
							return err
						}
					}
					return nil
				},
			},
			{
				// Tail streams one event and then never ends on its own: only
				// the client's bound or cancellation can stop it. This is the
				// "an unbounded stream must not hang the caller" fixture.
				StreamName:    "Tail",
				ServerStreams: true,
				Handler: func(srv interface{}, stream grpc.ServerStream) error {
					req := dynamic.NewMessage(def.tail.GetInputType())
					if err := stream.RecvMsg(req); err != nil {
						return err
					}
					prefix, _ := req.GetFieldByName("prefix").(string)
					resp := dynamic.NewMessage(def.tail.GetOutputType())
					resp.SetFieldByName("key", prefix+"-tail")
					resp.SetFieldByName("value", "event")
					if err := stream.SendMsg(resp); err != nil {
						return err
					}
					// Optional injected mid-stream failure: the client must
					// keep the item it already received and report the error.
					if code, ok := inject["/kv.KeyValue/Tail"]; ok {
						return status.Error(code, "injected failure after the first event")
					}
					<-stream.Context().Done()
					return stream.Context().Err()
				},
			},
			{
				StreamName:    "Upload",
				ClientStreams: true,
				Handler: func(srv interface{}, stream grpc.ServerStream) error {
					var keys []string
					for {
						req := dynamic.NewMessage(def.upload.GetInputType())
						err := stream.RecvMsg(req)
						if err == io.EOF {
							break
						}
						if err != nil {
							return err
						}
						key, _ := req.GetFieldByName("key").(string)
						keys = append(keys, key)
					}
					resp := dynamic.NewMessage(def.upload.GetOutputType())
					resp.SetFieldByName("count", int32(len(keys)))
					resp.SetFieldByName("joined", strings.Join(keys, ","))
					return stream.SendMsg(resp)
				},
			},
			{
				StreamName:    "Chat",
				ServerStreams: true,
				ClientStreams: true,
				Handler: func(srv interface{}, stream grpc.ServerStream) error {
					return echoChat(stream, def.chat, false)
				},
			},
			{
				StreamName:    "ChatHold",
				ServerStreams: true,
				ClientStreams: true,
				Handler: func(srv interface{}, stream grpc.ServerStream) error {
					return echoChat(stream, def.chatHold, true)
				},
			},
		},
	}
}

// echoChat drives a bidirectional echo stream. With hold set, a request whose
// text is "hold" makes the server keep the stream open after echoing it (it
// stops only when the client goes away) — the cancellation fixture.
func echoChat(stream grpc.ServerStream, method *desc.MethodDescriptor, hold bool) error {
	for {
		req := dynamic.NewMessage(method.GetInputType())
		err := stream.RecvMsg(req)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		text, _ := req.GetFieldByName("text").(string)
		resp := dynamic.NewMessage(method.GetOutputType())
		resp.SetFieldByName("text", text)
		if err := stream.SendMsg(resp); err != nil {
			return err
		}
		if hold && text == "hold" {
			<-stream.Context().Done()
			return stream.Context().Err()
		}
	}
}

// connFor dials a bufconn listener with the canonical §4.6 pattern
// (WithContextDialer over the in-memory listener; no real network).
func connFor(t *testing.T, lis *bufconn.Listener) *grpc.ClientConn {
	t.Helper()
	cc, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = cc.Close() })
	return cc
}

// protoSourceToTemp writes the test .proto to a temp dir and returns its path.
func protoSourceToTemp(t *testing.T) string {
	t.Helper()
	path := t.TempDir() + "/kv.proto"
	if err := os.WriteFile(path, []byte(testKVSource), 0o644); err != nil {
		t.Fatalf("write test proto: %v", err)
	}
	return path
}

// descriptorSetToTemp compiles the test .proto into a FileDescriptorSet
// file (as `protoc --descriptor_set_out` would produce, but built via
// protoreflect — proving the compiled-set path consumes a standard artifact).
func descriptorSetToTemp(t *testing.T) string {
	t.Helper()
	fds := parseTestProto(t)
	set := &descriptorpb.FileDescriptorSet{}
	for _, fd := range fds {
		set.File = append(set.File, fd.AsFileDescriptorProto())
	}
	raw, err := proto.Marshal(set)
	if err != nil {
		t.Fatalf("marshal descriptor set: %v", err)
	}
	path := t.TempDir() + "/kv.pb"
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write descriptor set: %v", err)
	}
	return path
}

// metadataCaptor records incoming metadata on the server side (§4.1 proof).
type metadataCaptor struct {
	mu       sync.Mutex
	received metadata.MD
}

func (c *metadataCaptor) capture(md metadata.MD) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.received = md
}

func (c *metadataCaptor) get(key string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.received.Get(key)
}
