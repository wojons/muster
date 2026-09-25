package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apperrors "github.com/wojons/muster/pkg/errors"
)

// ── gRPC streaming CLI wiring (PROTO-003) ────────────────────────────────
//
// The fixture server is a dynamic gRPC service on a 127.0.0.1 listener the
// test starts itself: loopback only, no external host, no network dependency
// (specs/PROTOCOLS.md §4.6).

// wireProto is the CLI test service: one unary method, a server-streaming
// method that ends, one that never ends, a unary method that never answers, a
// client-streaming method, and a bidi echo.
const wireProto = `syntax = "proto3";
package wire;

service Wire {
  // Ping echoes: the unary path.
  rpc Ping(PingRequest) returns (PingResponse);
  // SlowPing never answers on its own: the unary deadline fixture.
  rpc SlowPing(PingRequest) returns (PingResponse);
  // Stream sends three events, then ends normally.
  rpc Stream(PingRequest) returns (stream PingResponse);
  // Hang sends one event and then never ends: the streaming bound fixture.
  rpc Hang(PingRequest) returns (stream PingResponse);
  // Upload joins the request messages: the client-streaming path.
  rpc Upload(stream PingRequest) returns (PingResponse);
  // Chat echoes every request message: the bidi path.
  rpc Chat(stream PingRequest) returns (stream PingResponse);
}

message PingRequest {
  string text = 1;
}

message PingResponse {
  string text = 1;
}
`

// wireTranscript mirrors the §4.5 envelope for assertions.
type wireTranscript struct {
	Kind           string                   `json:"kind"`
	Status         string                   `json:"status"`
	Items          []map[string]interface{} `json:"items"`
	ItemsTruncated bool                     `json:"items_truncated"`
	Trailer        map[string]interface{}   `json:"trailer"`
}

// wireMethods holds the parsed method descriptors the fixture dispatches on.
type wireMethods struct {
	ping     *desc.MethodDescriptor
	slowPing *desc.MethodDescriptor
	stream   *desc.MethodDescriptor
	hang     *desc.MethodDescriptor
	upload   *desc.MethodDescriptor
	chat     *desc.MethodDescriptor
}

// parseWireProto parses wireProto into linked file descriptors.
func parseWireProto(t *testing.T) []*desc.FileDescriptor {
	t.Helper()
	accessor := protoparse.FileContentsFromMap(map[string]string{"wire.proto": wireProto})
	parser := &protoparse.Parser{Accessor: accessor}
	fds, err := parser.ParseFiles("wire.proto")
	require.NoError(t, err, "parse the fixture proto")
	return fds
}

// wireServiceMethods resolves the wire.Wire methods.
func wireServiceMethods(t *testing.T) *wireMethods {
	t.Helper()
	for _, fd := range parseWireProto(t) {
		for _, svc := range fd.GetServices() {
			if svc.GetFullyQualifiedName() != "wire.Wire" {
				continue
			}
			return &wireMethods{
				ping:     svc.FindMethodByName("Ping"),
				slowPing: svc.FindMethodByName("SlowPing"),
				stream:   svc.FindMethodByName("Stream"),
				hang:     svc.FindMethodByName("Hang"),
				upload:   svc.FindMethodByName("Upload"),
				chat:     svc.FindMethodByName("Chat"),
			}
		}
	}
	t.Fatal("wire.Wire service not found in the fixture proto")
	return nil
}

// startWireServer serves the fixture on a loopback listener and returns the
// target host:port the CLI should dial.
func startWireServer(t *testing.T) string {
	t.Helper()
	def := wireServiceMethods(t)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "listen on loopback")
	srv := grpc.NewServer()
	srv.RegisterService(wireServiceDesc(def), nil)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	return lis.Addr().String()
}

// wireServiceDesc builds the ServiceDesc with dynamic handlers.
func wireServiceDesc(def *wireMethods) *grpc.ServiceDesc {
	unary := func(method *desc.MethodDescriptor, handle func(ctx context.Context, req *dynamic.Message) (interface{}, error)) grpc.MethodDesc {
		return grpc.MethodDesc{
			MethodName: method.GetName(),
			Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
				req := dynamic.NewMessage(method.GetInputType())
				if err := dec(req); err != nil {
					return nil, err
				}
				return handle(ctx, req)
			},
		}
	}
	echo := func(ctx context.Context, req *dynamic.Message) (interface{}, error) {
		text, _ := req.GetFieldByName("text").(string)
		resp := dynamic.NewMessage(def.ping.GetOutputType())
		resp.SetFieldByName("text", text)
		return resp, nil
	}

	return &grpc.ServiceDesc{
		ServiceName: "wire.Wire",
		Methods: []grpc.MethodDesc{
			unary(def.ping, echo),
			unary(def.slowPing, func(ctx context.Context, req *dynamic.Message) (interface{}, error) {
				<-ctx.Done()
				return nil, status.Error(codes.DeadlineExceeded, "SlowPing: the client's deadline ended the call")
			}),
		},
		Streams: []grpc.StreamDesc{
			{
				StreamName:    "Stream",
				ServerStreams: true,
				Handler: func(srv interface{}, stream grpc.ServerStream) error {
					req := dynamic.NewMessage(def.stream.GetInputType())
					if err := stream.RecvMsg(req); err != nil {
						return err
					}
					text, _ := req.GetFieldByName("text").(string)
					for i := 0; i < 3; i++ {
						resp := dynamic.NewMessage(def.stream.GetOutputType())
						resp.SetFieldByName("text", text+"-"+string(rune('0'+i)))
						if err := stream.SendMsg(resp); err != nil {
							return err
						}
					}
					return nil
				},
			},
			{
				StreamName:    "Hang",
				ServerStreams: true,
				Handler: func(srv interface{}, stream grpc.ServerStream) error {
					req := dynamic.NewMessage(def.hang.GetInputType())
					if err := stream.RecvMsg(req); err != nil {
						return err
					}
					resp := dynamic.NewMessage(def.hang.GetOutputType())
					resp.SetFieldByName("text", "hanging")
					if err := stream.SendMsg(resp); err != nil {
						return err
					}
					<-stream.Context().Done()
					return stream.Context().Err()
				},
			},
			{
				StreamName:    "Upload",
				ClientStreams: true,
				Handler: func(srv interface{}, stream grpc.ServerStream) error {
					var texts []string
					for {
						req := dynamic.NewMessage(def.upload.GetInputType())
						err := stream.RecvMsg(req)
						if err == io.EOF {
							break
						}
						if err != nil {
							return err
						}
						text, _ := req.GetFieldByName("text").(string)
						texts = append(texts, text)
					}
					resp := dynamic.NewMessage(def.upload.GetOutputType())
					resp.SetFieldByName("text", strings.Join(texts, ","))
					return stream.SendMsg(resp)
				},
			},
			{
				StreamName:    "Chat",
				ServerStreams: true,
				ClientStreams: true,
				Handler: func(srv interface{}, stream grpc.ServerStream) error {
					for {
						req := dynamic.NewMessage(def.chat.GetInputType())
						err := stream.RecvMsg(req)
						if err == io.EOF {
							return nil
						}
						if err != nil {
							return err
						}
						text, _ := req.GetFieldByName("text").(string)
						resp := dynamic.NewMessage(def.chat.GetOutputType())
						resp.SetFieldByName("text", "echo:"+text)
						if err := stream.SendMsg(resp); err != nil {
							return err
						}
					}
				},
			},
		},
	}
}

// writeWireProto writes the fixture proto to a temp file and returns its path.
func writeWireProto(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wire.proto")
	require.NoError(t, os.WriteFile(path, []byte(wireProto), 0o600))
	return path
}

// runGrpcCall runs `openapi-cli grpc call ...` and returns stdout and the
// command error (negative tests assert the error).
func runGrpcCall(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out := &bytes.Buffer{}
	cmd := newGrpcCallCommand()
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// requireGrpcCall runs a call that must succeed and returns stdout.
func requireGrpcCall(t *testing.T, args ...string) string {
	t.Helper()
	out, err := runGrpcCall(t, args...)
	require.NoError(t, err, "grpc call %v", args)
	return out
}

// decodeWireTranscript decodes the printed envelope.
func decodeWireTranscript(t *testing.T, out string) wireTranscript {
	t.Helper()
	var tr wireTranscript
	require.NoError(t, json.Unmarshal([]byte(out), &tr), "stdout must be a transcript envelope: %q", out)
	return tr
}

// ── unary (PROTO-002 path, kept honest by the streaming work) ─────────────

func TestGrpcCall_Unary_PrintsResponse(t *testing.T) {
	target := startWireServer(t)

	out := requireGrpcCall(t,
		"wire.Wire-Ping",
		"--source", writeWireProto(t),
		"--target", target,
		"--arg", "text=hello",
	)

	assert.Contains(t, out, `"text": "hello"`, "the unary response must be printed")
	assert.NotContains(t, out, `"kind"`, "a unary call returns the plain response, not a transcript")
}

func TestGrpcCall_Unary_DeadlineBoundedByTimeout(t *testing.T) {
	target := startWireServer(t)

	start := time.Now()
	out, err := runGrpcCall(t,
		"wire.Wire-SlowPing",
		"--source", writeWireProto(t),
		"--target", target,
		"--timeout", "200ms",
	)
	elapsed := time.Since(start)

	require.Error(t, err, "a server that never answers must not hang the CLI")
	require.Less(t, elapsed, 3*time.Second, "the --timeout deadline must end the call")
	assert.True(t, apperrors.IsError(err, apperrors.CodeTimeout),
		"deadline error = %v, want AppError CodeTimeout", err)
	assert.NotContains(t, out, `"status"`, "a timed-out call prints no transcript envelope")
}

// ── server-streaming ────────────────────────────────────────────────────

func TestGrpcCall_ServerStream_Complete(t *testing.T) {
	target := startWireServer(t)

	out := requireGrpcCall(t,
		"wire.Wire-Stream",
		"--source", writeWireProto(t),
		"--target", target,
		"--arg", "text=ev",
	)
	tr := decodeWireTranscript(t, out)

	assert.Equal(t, "server-stream", tr.Kind)
	assert.Equal(t, "complete", tr.Status)
	require.Len(t, tr.Items, 3)
	assert.Equal(t, "ev-0", tr.Items[0]["text"])
	assert.False(t, tr.ItemsTruncated)
	assert.Equal(t, float64(1000), tr.Trailer["max_messages"])
}

func TestGrpcCall_ServerStream_MaxMessagesTruncatesCleanly(t *testing.T) {
	target := startWireServer(t)

	out := requireGrpcCall(t,
		"wire.Wire-Stream",
		"--source", writeWireProto(t),
		"--target", target,
		"--arg", "text=ev",
		"--max-messages", "2",
	)
	tr := decodeWireTranscript(t, out)

	assert.Equal(t, "truncated", tr.Status, "the cap must be reported, not hidden")
	assert.True(t, tr.ItemsTruncated)
	require.Len(t, tr.Items, 2)
	assert.Equal(t, "max-messages", tr.Trailer["limit_reason"])
	assert.Equal(t, float64(2), tr.Trailer["received"])
}

func TestGrpcCall_ServerStream_TimeoutBoundStopsEndlessStream(t *testing.T) {
	target := startWireServer(t)

	start := time.Now()
	out := requireGrpcCall(t,
		"wire.Wire-Hang",
		"--source", writeWireProto(t),
		"--target", target,
		"--timeout", "250ms",
	)
	elapsed := time.Since(start)

	require.Less(t, elapsed, 3*time.Second, "an endless stream must be stopped by --timeout")
	tr := decodeWireTranscript(t, out)
	assert.Equal(t, "truncated", tr.Status)
	assert.True(t, tr.ItemsTruncated)
	assert.Equal(t, "max-duration", tr.Trailer["limit_reason"])
	require.Len(t, tr.Items, 1, "the event received before the bound must be kept")
}

func TestGrpcCall_ServerStream_MaxMessagesStopsEndlessStream(t *testing.T) {
	target := startWireServer(t)

	start := time.Now()
	out := requireGrpcCall(t,
		"wire.Wire-Hang",
		"--source", writeWireProto(t),
		"--target", target,
		"--max-messages", "1",
	)
	elapsed := time.Since(start)

	require.Less(t, elapsed, 3*time.Second, "--max-messages must stop an endless stream")
	tr := decodeWireTranscript(t, out)
	assert.Equal(t, "truncated", tr.Status)
	assert.Equal(t, "max-messages", tr.Trailer["limit_reason"])
	require.Len(t, tr.Items, 1)
}

// ── client-streaming ────────────────────────────────────────────────────

func TestGrpcCall_ClientStream_InlineMessages(t *testing.T) {
	target := startWireServer(t)

	out := requireGrpcCall(t,
		"wire.Wire-Upload",
		"--source", writeWireProto(t),
		"--target", target,
		"--messages", `[{"text":"a"},{"text":"b"},{"text":"c"}]`,
	)
	tr := decodeWireTranscript(t, out)

	assert.Equal(t, "client-stream", tr.Kind)
	assert.Equal(t, "complete", tr.Status)
	require.Len(t, tr.Items, 1, "a client-streaming method answers with one message")
	assert.Equal(t, "a,b,c", tr.Items[0]["text"])
	assert.Equal(t, float64(3), tr.Trailer["messages_sent"])
}

func TestGrpcCall_ClientStream_MessagesFromFile(t *testing.T) {
	target := startWireServer(t)

	path := filepath.Join(t.TempDir(), "messages.json")
	require.NoError(t, os.WriteFile(path, []byte(`[{"text":"x"},{"text":"y"}]`), 0o600))

	out := requireGrpcCall(t,
		"wire.Wire-Upload",
		"--source", writeWireProto(t),
		"--target", target,
		"--messages", "@"+path,
	)
	tr := decodeWireTranscript(t, out)

	assert.Equal(t, "x,y", tr.Items[0]["text"])
	assert.Equal(t, float64(2), tr.Trailer["messages_sent"])
}

func TestGrpcCall_ClientStream_CappedRequestStreamIsMarked(t *testing.T) {
	target := startWireServer(t)

	out := requireGrpcCall(t,
		"wire.Wire-Upload",
		"--source", writeWireProto(t),
		"--target", target,
		"--messages", `[{"text":"a"},{"text":"b"},{"text":"c"}]`,
		"--max-messages", "2",
	)
	tr := decodeWireTranscript(t, out)

	assert.Equal(t, "truncated", tr.Status)
	assert.True(t, tr.ItemsTruncated)
	assert.Equal(t, float64(2), tr.Trailer["messages_sent"])
	assert.Equal(t, "a,b", tr.Items[0]["text"], "the answer to the capped stream is still reported")
}

// TestGrpcCall_ClientStream_ArgSeedsOneMessage: on a client-streaming method,
// --arg is a one-message request stream (the documented seed rule), and no
// flags at all sends a legal empty stream.
func TestGrpcCall_ClientStream_ArgSeedsOneMessage(t *testing.T) {
	target := startWireServer(t)

	out := requireGrpcCall(t,
		"wire.Wire-Upload",
		"--source", writeWireProto(t),
		"--target", target,
		"--arg", "text=solo",
	)
	tr := decodeWireTranscript(t, out)

	assert.Equal(t, "complete", tr.Status)
	assert.Equal(t, "solo", tr.Items[0]["text"])
	assert.Equal(t, float64(1), tr.Trailer["messages_sent"])
}

// TestGrpcCall_ClientStream_NoFlagsSendsEmptyStream: an empty request stream
// is legal and is reported as such instead of being treated as an error.
func TestGrpcCall_ClientStream_NoFlagsSendsEmptyStream(t *testing.T) {
	target := startWireServer(t)

	out := requireGrpcCall(t,
		"wire.Wire-Upload",
		"--source", writeWireProto(t),
		"--target", target,
	)
	tr := decodeWireTranscript(t, out)

	assert.Equal(t, "complete", tr.Status)
	assert.Equal(t, float64(0), tr.Trailer["messages_sent"])
	require.Len(t, tr.Items, 1)
}

// ── bidirectional ───────────────────────────────────────────────────────

func TestGrpcCall_Bidi_RoundTripsMessages(t *testing.T) {
	target := startWireServer(t)

	out := requireGrpcCall(t,
		"wire.Wire-Chat",
		"--source", writeWireProto(t),
		"--target", target,
		"--messages", `[{"text":"one"},{"text":"two"}]`,
	)
	tr := decodeWireTranscript(t, out)

	assert.Equal(t, "bidi-stream", tr.Kind)
	assert.Equal(t, "complete", tr.Status)
	require.Len(t, tr.Items, 2)
	assert.Equal(t, "echo:one", tr.Items[0]["text"])
	assert.Equal(t, "echo:two", tr.Items[1]["text"])
	assert.Equal(t, float64(2), tr.Trailer["messages_sent"])
}

// ── flag contract ───────────────────────────────────────────────────────

func TestGrpcCall_MessagesOnUnaryMethodIsRejected(t *testing.T) {
	target := startWireServer(t)

	_, err := runGrpcCall(t,
		"wire.Wire-Ping",
		"--source", writeWireProto(t),
		"--target", target,
		"--messages", `[{"text":"a"}]`,
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "client-streaming", "the error must name the methods --messages applies to")
	assert.Contains(t, err.Error(), "unary", "the error must name the method's own kind")
}

func TestGrpcCall_ArgAndMessagesAreMutuallyExclusive(t *testing.T) {
	target := startWireServer(t)

	_, err := runGrpcCall(t,
		"wire.Wire-Upload",
		"--source", writeWireProto(t),
		"--target", target,
		"--arg", "text=a",
		"--messages", `[{"text":"a"}]`,
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestGrpcCall_InvalidMessagesJSONIsRejected(t *testing.T) {
	target := startWireServer(t)

	_, err := runGrpcCall(t,
		"wire.Wire-Upload",
		"--source", writeWireProto(t),
		"--target", target,
		"--messages", `{"text":"not-an-array"}`,
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "JSON array")
}

func TestGrpcCall_UnknownOperationIsReported(t *testing.T) {
	target := startWireServer(t)

	_, err := runGrpcCall(t,
		"wire.Wire-Nope",
		"--source", writeWireProto(t),
		"--target", target,
	)

	require.Error(t, err)
	assert.True(t, apperrors.IsError(err, apperrors.CodeNotFound),
		"unknown operation error = %v, want AppError CodeNotFound", err)
}

func TestGrpcCall_BoundFlagDefaults(t *testing.T) {
	cmd := newGrpcCallCommand()

	maxMessages, err := cmd.Flags().GetInt(FlagGrpcMaxMessages)
	require.NoError(t, err)
	assert.Equal(t, 1000, maxMessages, "--max-messages default must be the documented bound")

	timeout, err := cmd.Flags().GetDuration(FlagGrpcTimeout)
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, timeout, "--timeout default must be the documented bound")

	for _, flag := range []string{FlagGrpcMessages, FlagGrpcTimeout, FlagGrpcMaxMessages} {
		assert.NotNil(t, cmd.Flags().Lookup(flag), "flag --%s must be registered", flag)
	}
}

// TestGrpcCall_HelpDocumentsTheBoundedContract pins the documentation the
// deliverable requires: the bound flags, the status vocabulary, and the
// truncation marker must all be described in the command help.
func TestGrpcCall_HelpDocumentsTheBoundedContract(t *testing.T) {
	cmd := newGrpcCallCommand()

	for _, want := range []string{
		"--max-messages",
		"--timeout",
		"--messages",
		"truncated",
		"cancelled",
		"items_truncated",
		"limit_reason",
		"stream",
	} {
		assert.Contains(t, cmd.Long, want, "grpc call --help must document %q", want)
	}

	group := newGrpcCommand()
	assert.Contains(t, group.Long, "server-streaming", "the group help must name the streaming kinds")
	assert.Contains(t, group.Long, "BOUNDED", "the group help must state the bounded-output contract")
}

// TestGrpcCommand_MountedOnRoot: the grpc group stays mounted on the frozen
// root command set (specs/PROTOCOLS.md §5.2).
func TestGrpcCommand_MountedOnRoot(t *testing.T) {
	root := NewRootCommand(nil)
	found := false
	for _, c := range root.Commands() {
		if c.Name() == "grpc" {
			found = true
		}
	}
	assert.True(t, found, "the grpc group must be mounted on the root command")
}
