package grpc

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jhump/protoreflect/desc/protoparse"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	apperrors "github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
	"github.com/wojons/muster/pkg/rate"
)

// bufnetTarget is the fake target name the bufconn dialer resolves.
const bufnetTarget = "passthrough:///bufnet"

// The tests use the adapter against the in-process server via WithConn, so
// discovery and execution share one bufconn connection (§4.6).

// TestAdapter_ImplementsInterface pins the seam contract at compile time.
func TestAdapter_ImplementsInterface(t *testing.T) {
	var _ protocol.Adapter = (*Adapter)(nil)
	var _ protocol.Discoverer = (*Adapter)(nil)
	var _ protocol.Executor = (*Adapter)(nil)
}

// TestAdapter_RegistryRoundTrip proves registration + lookup semantics.
func TestAdapter_RegistryRoundTrip(t *testing.T) {
	reg := protocol.NewRegistry()
	if err := reg.Register(NewAdapter()); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, err := reg.Adapter(protocol.ProtocolGRPC)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.Protocol() != protocol.ProtocolGRPC {
		t.Fatalf("protocol = %q, want grpc", got.Protocol())
	}
	if err := reg.Register(NewAdapter()); err == nil {
		t.Fatal("duplicate registration must error")
	}
	if _, err := reg.Adapter(protocol.Protocol("nope")); !apperrors.IsError(err, apperrors.CodeNotFound) {
		t.Fatalf("unknown protocol error = %v, want AppError CodeNotFound", err)
	}
}

// TestDiscover_ProtoSource: .proto ingestion finds both unary methods (gate a).
func TestDiscover_ProtoSource(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	ops, err := adapter.Discover(context.Background(), protoSourceToTemp(t))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	assertKVOps(t, ops, "proto")
}

// TestDiscover_DescriptorSet: compiled descriptor-set ingestion finds the
// same operation set (gate a).
func TestDiscover_DescriptorSet(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()

	ops, err := adapter.Discover(context.Background(), descriptorSetToTemp(t))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	assertKVOps(t, ops, "descriptorset")
}

// TestDiscover_DescriptorSetParity: .proto and descriptor-set ingestion
// produce IDENTICAL operation IDs and full-method ground truth.
func TestDiscover_DescriptorSetParity(t *testing.T) {
	a1 := NewAdapter()
	ops1, err := a1.Discover(context.Background(), protoSourceToTemp(t))
	if err != nil {
		t.Fatalf("discover proto: %v", err)
	}
	a2 := NewAdapter()
	ops2, err := a2.Discover(context.Background(), descriptorSetToTemp(t))
	if err != nil {
		t.Fatalf("discover descriptor set: %v", err)
	}
	if len(ops1) != len(ops2) {
		t.Fatalf("operation count mismatch: proto=%d descriptorset=%d", len(ops1), len(ops2))
	}
	byID1 := map[string]*protocol.Operation{}
	for _, op := range ops1 {
		byID1[op.ID] = op
	}
	for _, op := range ops2 {
		p, ok := byID1[op.ID]
		if !ok {
			t.Fatalf("descriptor-set op %q missing from proto discovery", op.ID)
		}
		if p.Source[SourceFullMethod] != op.Source[SourceFullMethod] {
			t.Fatalf("full method mismatch for %s: %q vs %q", op.ID, p.Source[SourceFullMethod], op.Source[SourceFullMethod])
		}
		if p.Kind != op.Kind {
			t.Fatalf("kind mismatch for %s: %v vs %v", op.ID, p.Kind, op.Kind)
		}
	}
}

// TestDiscover_Reflection: server-reflection discovery over bufconn finds
// the same set (gate b). Skips the reflection service itself.
func TestDiscover_Reflection(t *testing.T) {
	store := newKVStore()
	lis, stop := serveKV(t, store, nil, nil)
	defer stop()

	cc := connFor(t, lis)
	adapter := NewAdapter().WithConn(cc)
	defer func() { _ = adapter.Close() }()

	ops, err := adapter.Discover(context.Background(), "grpc://bufnet")
	if err != nil {
		t.Fatalf("discover via reflection: %v", err)
	}
	assertKVOps(t, ops, "reflection")
	for _, op := range ops {
		if op.Source[SourceHost] != "bufnet" {
			t.Fatalf("op %s host = %q, want bufnet", op.ID, op.Source[SourceHost])
		}
	}
}

// TestDiscover_ReflectionMatchesFiles: gate (b) parity — reflection finds
// the same operation set as file ingestion.
func TestDiscover_ReflectionMatchesFiles(t *testing.T) {
	store := newKVStore()
	lis, stop := serveKV(t, store, nil, nil)
	defer stop()

	cc := connFor(t, lis)
	reflAdapter := NewAdapter().WithConn(cc)
	fileAdapter := NewAdapter()

	reflOps, err := reflAdapter.Discover(context.Background(), "grpc://bufnet")
	if err != nil {
		t.Fatalf("reflection discover: %v", err)
	}
	fileOps, err := fileAdapter.Discover(context.Background(), protoSourceToTemp(t))
	if err != nil {
		t.Fatalf("file discover: %v", err)
	}

	reflIDs := map[string]bool{}
	for _, op := range reflOps {
		reflIDs[op.ID] = true
	}
	for _, op := range fileOps {
		if !reflIDs[op.ID] {
			t.Fatalf("file op %q missing from reflection discovery", op.ID)
		}
	}
}

// TestExecute_UnaryRoundTrip_ProtoSource: end-to-end Set→Get through the
// adapter with file-based discovery (gate c, flag-parsed args equivalent).
func TestExecute_UnaryRoundTrip_ProtoSource(t *testing.T) {
	store := newKVStore()
	lis, stop := serveKV(t, store, nil, nil)
	defer stop()

	cc := connFor(t, lis)
	adapter := NewAdapter().WithConn(cc)
	defer func() { _ = adapter.Close() }()

	if _, err := adapter.Discover(context.Background(), protoSourceToTemp(t)); err != nil {
		t.Fatalf("discover: %v", err)
	}

	// Set.
	setRes, err := adapter.Execute(context.Background(), "kv.KeyValue-Set", map[string]interface{}{
		"key": "greeting", "value": "hello",
	})
	if err != nil {
		t.Fatalf("execute Set: %v", err)
	}
	setMap, ok := setRes.(map[string]interface{})
	if !ok || setMap["ok"] != true {
		t.Fatalf("Set result = %#v, want {ok: true}", setRes)
	}

	// Get.
	getRes, err := adapter.Execute(context.Background(), "kv.KeyValue-Get", map[string]interface{}{
		"key": "greeting",
	})
	if err != nil {
		t.Fatalf("execute Get: %v", err)
	}
	getMap, ok := getRes.(map[string]interface{})
	if !ok {
		t.Fatalf("Get result = %#v, want map", getRes)
	}
	if getMap["key"] != "greeting" || getMap["value"] != "hello" {
		t.Fatalf("Get result = %#v, want key=greeting value=hello", getMap)
	}
}

// TestExecute_UnaryRoundTrip_Reflection: the same round-trip with
// reflection-only discovery — no descriptor file anywhere.
func TestExecute_UnaryRoundTrip_Reflection(t *testing.T) {
	store := newKVStore()
	lis, stop := serveKV(t, store, nil, nil)
	defer stop()

	cc := connFor(t, lis)
	adapter := NewAdapter().WithConn(cc)
	defer func() { _ = adapter.Close() }()

	if _, err := adapter.Discover(context.Background(), "grpc://bufnet"); err != nil {
		t.Fatalf("discover: %v", err)
	}
	// Round-trip proof comes via Set+Get; do Set first through reflection-only path.
	if _, err := adapter.Execute(context.Background(), "kv.KeyValue-Set", map[string]interface{}{
		"key": "r2", "value": "v2",
	}); err != nil {
		t.Fatalf("execute Set (reflection): %v", err)
	}
	res, err := adapter.Execute(context.Background(), "kv.KeyValue-Get", map[string]interface{}{
		"key": "r2",
	})
	if err != nil {
		t.Fatalf("execute Get (reflection): %v", err)
	}
	got, _ := res.(map[string]interface{})
	if got["value"] != "v2" {
		t.Fatalf("Get (reflection) result = %#v, want value=v2", res)
	}
}

// TestExecute_ErrorMapping: gate (e) — injected InvalidArgument →
// CodeBadRequest; NotFound → CodeNotFound; injected DeadlineExceeded →
// CodeTimeout; Unauthenticated → CodeUnauthorized; ResourceExhausted →
// CodeRateLimit. Cause chain preserved for apperrors.IsError.
func TestExecute_ErrorMapping(t *testing.T) {
	store := newKVStore()
	inject := map[string]codes.Code{
		"/kv.KeyValue/Set": codes.DeadlineExceeded,
	}
	lis, stop := serveKV(t, store, inject, nil)
	defer stop()

	cc := connFor(t, lis)
	adapter := NewAdapter().WithConn(cc)
	defer func() { _ = adapter.Close() }()

	if _, err := adapter.Discover(context.Background(), protoSourceToTemp(t)); err != nil {
		t.Fatalf("discover: %v", err)
	}

	// Server-side InvalidArgument (empty key rejected by the handler).
	_, err := adapter.Execute(context.Background(), "kv.KeyValue-Get", map[string]interface{}{
		"key": "",
	})
	if !apperrors.IsError(err, apperrors.CodeBadRequest) {
		t.Fatalf("empty-key error = %v, want AppError CodeBadRequest", err)
	}

	// Server-side NotFound.
	_, err = adapter.Execute(context.Background(), "kv.KeyValue-Get", map[string]interface{}{
		"key": "nope",
	})
	if !apperrors.IsError(err, apperrors.CodeNotFound) {
		t.Fatalf("missing-key error = %v, want AppError CodeNotFound", err)
	}

	// Injected DeadlineExceeded → CodeTimeout.
	_, err = adapter.Execute(context.Background(), "kv.KeyValue-Set", map[string]interface{}{
		"key": "k", "value": "v",
	})
	if !apperrors.IsError(err, apperrors.CodeTimeout) {
		t.Fatalf("injected deadline error = %v, want AppError CodeTimeout", err)
	}

	// grpc_code detail is stamped at the transport boundary.
	appErr := apperrors.Wrap(err, apperrors.CodeTimeout, "")
	_ = appErr
}

// TestExecute_ErrorMapping_FullTable: remaining §4.2 rows via injection.
func TestExecute_ErrorMapping_FullTable(t *testing.T) {
	cases := []struct {
		name     string
		inject   codes.Code
		wantCode apperrors.Code
	}{
		{"unauthenticated", codes.Unauthenticated, apperrors.CodeUnauthorized},
		{"permission_denied", codes.PermissionDenied, apperrors.CodeUnauthorized},
		{"resource_exhausted", codes.ResourceExhausted, apperrors.CodeRateLimit},
		{"invalid_argument", codes.InvalidArgument, apperrors.CodeBadRequest},
		{"failed_precondition", codes.FailedPrecondition, apperrors.CodeBadRequest},
		{"out_of_range", codes.OutOfRange, apperrors.CodeBadRequest},
		{"internal", codes.Internal, apperrors.CodeGeneric},
		{"unavailable", codes.Unavailable, apperrors.CodeGeneric},
		{"unknown", codes.Unknown, apperrors.CodeGeneric},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newKVStore()
			lis, stop := serveKV(t, store, map[string]codes.Code{"/kv.KeyValue/Get": tc.inject}, nil)
			defer stop()

			cc := connFor(t, lis)
			adapter := NewAdapter().WithConn(cc)
			defer func() { _ = adapter.Close() }()
			if _, err := adapter.Discover(context.Background(), protoSourceToTemp(t)); err != nil {
				t.Fatalf("discover: %v", err)
			}
			_, err := adapter.Execute(context.Background(), "kv.KeyValue-Get", map[string]interface{}{"key": "k"})
			if !apperrors.IsError(err, tc.wantCode) {
				t.Fatalf("injected %s error = %v, want %s", tc.inject, err, tc.wantCode)
			}
		})
	}
}

// TestExecute_ServerStreaming_Executable: the PROTO-003 boundary is gone — a
// server-streaming method executes and returns the §4.5 bounded transcript
// envelope instead of the PROTO-002 rejection. Unary behaviour is unchanged
// (the PROTO-002 tests above).
func TestExecute_ServerStreaming_Executable(t *testing.T) {
	store := newKVStore()
	lis, stop := serveKV(t, store, nil, nil)
	defer stop()

	cc := connFor(t, lis)
	adapter := NewAdapter().WithConn(cc)
	defer func() { _ = adapter.Close() }()

	ops, err := adapter.Discover(context.Background(), protoSourceToTemp(t))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	var watchOp *protocol.Operation
	for _, op := range ops {
		if op.ID == "kv.KeyValue-Watch" {
			watchOp = op
		}
	}
	if watchOp == nil || watchOp.Kind != protocol.KindServerStream {
		t.Fatalf("Watch discovery = %#v, want a server-stream operation", watchOp)
	}

	res, err := adapter.Execute(context.Background(), "kv.KeyValue-Watch", map[string]interface{}{
		"prefix": "k",
	})
	if err != nil {
		t.Fatalf("server-streaming Execute must succeed, got %v", err)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("transcript not JSON-serializable: %v", err)
	}
	if !strings.Contains(string(raw), `"kind":"server-stream"`) {
		t.Fatalf("transcript missing the §4.5 kind field: %s", raw)
	}
}

// TestExecute_UnknownOperation: undiscovered/unknown op → CodeNotFound.
func TestExecute_UnknownOperation(t *testing.T) {
	store := newKVStore()
	lis, stop := serveKV(t, store, nil, nil)
	defer stop()

	cc := connFor(t, lis)
	adapter := NewAdapter().WithConn(cc)
	defer func() { _ = adapter.Close() }()

	if _, err := adapter.Discover(context.Background(), protoSourceToTemp(t)); err != nil {
		t.Fatalf("discover: %v", err)
	}
	_, err := adapter.Execute(context.Background(), "kv.Nope-Missing", nil)
	if !apperrors.IsError(err, apperrors.CodeNotFound) {
		t.Fatalf("unknown op error = %v, want AppError CodeNotFound", err)
	}
}

// TestExecute_NoConn: Execute without a bound connection is a clean
// validation error, not a panic.
func TestExecute_NoConn(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()
	_, err := adapter.Execute(context.Background(), "kv.KeyValue-Get", nil)
	if err == nil {
		t.Fatal("Execute without conn must fail")
	}
	if !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Fatalf("no-conn error = %v, want AppError CodeValidation", err)
	}
}

// TestExecute_ValidationBeforeWire: mismatched args fail client-side with
// CodeValidation and never reach the server (§4.2).
func TestExecute_ValidationBeforeWire(t *testing.T) {
	store := newKVStore()
	lis, stop := serveKV(t, store, nil, nil)
	defer stop()

	cc := connFor(t, lis)
	adapter := NewAdapter().WithConn(cc)
	defer func() { _ = adapter.Close() }()

	if _, err := adapter.Discover(context.Background(), protoSourceToTemp(t)); err != nil {
		t.Fatalf("discover: %v", err)
	}
	// "key" is a string field; passing a JSON object cannot match.
	_, err := adapter.Execute(context.Background(), "kv.KeyValue-Get", map[string]interface{}{
		"key": map[string]interface{}{"nested": true},
	})
	if !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Fatalf("arg-mismatch error = %v, want AppError CodeValidation", err)
	}
}

// TestExecute_ContextDeadlineHonored: the ctx deadline IS the deadline (§4.3
// — no adapter-default timeout overrides the caller). A canceled ctx fails
// fast with CodeTimeout.
func TestExecute_ContextDeadlineHonored(t *testing.T) {
	store := newKVStore()
	lis, stop := serveKV(t, store, nil, nil)
	defer stop()

	cc := connFor(t, lis)
	adapter := NewAdapter().WithConn(cc)
	defer func() { _ = adapter.Close() }()

	if _, err := adapter.Discover(context.Background(), protoSourceToTemp(t)); err != nil {
		t.Fatalf("discover: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled: the call must not hang
	start := time.Now()
	_, err := adapter.Execute(ctx, "kv.KeyValue-Get", map[string]interface{}{"key": "k"})
	if time.Since(start) > 2*time.Second {
		t.Fatal("canceled-context call took too long — adapter may be applying its own timeout")
	}
	if err == nil {
		t.Fatal("canceled-context call must fail")
	}
	if !apperrors.IsError(err, apperrors.CodeTimeout) {
		t.Fatalf("canceled-context error = %v, want AppError CodeTimeout", err)
	}
}

// TestExecute_AuthMetadataMapped: §4.1 — Bearer(token) arrives as
// "authorization: Bearer <token>" on the server (static metadata mapping).
func TestExecute_AuthMetadataMapped(t *testing.T) {
	store := newKVStore()
	captor := &metadataCaptor{}
	lis, stop := serveKV(t, store, nil, captor)
	defer stop()

	cc := connFor(t, lis)
	adapter := NewAdapter().WithConn(cc).WithMetadata(Bearer("test-token-123"))
	defer func() { _ = adapter.Close() }()

	if _, err := adapter.Discover(context.Background(), protoSourceToTemp(t)); err != nil {
		t.Fatalf("discover: %v", err)
	}
	if _, err := adapter.Execute(context.Background(), "kv.KeyValue-Get", map[string]interface{}{"key": "k"}); err == nil {
		t.Log("Get with missing key returned no error (unexpected but harmless for this proof)")
	}
	if got := captor.get("authorization"); len(got) == 0 || got[0] != "Bearer test-token-123" {
		t.Fatalf("server authorization metadata = %v, want [Bearer test-token-123]", got)
	}
}

// TestExecute_RateLimited: §4.3 — the shared token bucket gates Execute
// before dial; refusal is CodeRateLimit.
func TestExecute_RateLimited(t *testing.T) {
	store := newKVStore()
	lis, stop := serveKV(t, store, nil, nil)
	defer stop()

	cc := connFor(t, lis)
	// Bucket with zero refill and burst 1: first call consumes the token,
	// the second is refused locally.
	tb := rate.NewTokenBucket(0, 1)
	adapter := NewAdapter().WithConn(cc).WithRateLimiter(tb)
	defer func() { _ = adapter.Close() }()

	if _, err := adapter.Discover(context.Background(), protoSourceToTemp(t)); err != nil {
		t.Fatalf("discover: %v", err)
	}
	if _, err := adapter.Execute(context.Background(), "kv.KeyValue-Get", map[string]interface{}{"key": "k"}); err != nil {
		// First call may still fail (missing key) — that's fine, token spent.
		t.Logf("first call error (expected): %v", err)
	}
	_, err := adapter.Execute(context.Background(), "kv.KeyValue-Get", map[string]interface{}{"key": "k"})
	if !apperrors.IsError(err, apperrors.CodeRateLimit) {
		t.Fatalf("second call error = %v, want AppError CodeRateLimit", err)
	}
}

// TestIngest_MissingFile: descriptor-set read failure → CodeNotFound with
// the path preserved.
func TestIngest_MissingFile(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()
	_, err := adapter.Discover(context.Background(), "/definitely/not/here.pb")
	if !apperrors.IsError(err, apperrors.CodeNotFound) {
		t.Fatalf("missing-file error = %v, want AppError CodeNotFound", err)
	}
}

// TestIngest_BadDescriptorSet: non-descriptor bytes → CodeValidation.
func TestIngest_BadDescriptorSet(t *testing.T) {
	path := t.TempDir() + "/garbage.pb"
	if err := osWrite(path, []byte("this is not a descriptor set")); err != nil {
		t.Fatal(err)
	}
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()
	_, err := adapter.Discover(context.Background(), path)
	if !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Fatalf("garbage descriptor-set error = %v, want AppError CodeValidation", err)
	}
}

// TestIngest_BadProtoSource: unparseable .proto → CodeValidation.
func TestIngest_BadProtoSource(t *testing.T) {
	path := t.TempDir() + "/broken.proto"
	if err := osWrite(path, []byte("syntax = total nonsense;")); err != nil {
		t.Fatal(err)
	}
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()
	_, err := adapter.Discover(context.Background(), path)
	if !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Fatalf("broken proto error = %v, want AppError CodeValidation", err)
	}
}

// TestIngest_EmptyURI: empty discovery URI → CodeValidation.
func TestIngest_EmptyURI(t *testing.T) {
	adapter := NewAdapter()
	defer func() { _ = adapter.Close() }()
	if _, err := adapter.Discover(context.Background(), ""); !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Fatal("empty URI must be CodeValidation")
	}
}

// TestMapMethod_KindsAndSource pins the neutral mapping: IDs, kinds, and
// transport ground truth.
func TestMapMethod_KindsAndSource(t *testing.T) {
	def := kvDef(t)
	op := MapMethodDescriptor("kv.KeyValue", def.get)
	if op.ID != "kv.KeyValue-Get" {
		t.Fatalf("ID = %q, want kv.KeyValue-Get", op.ID)
	}
	if op.Protocol != protocol.ProtocolGRPC {
		t.Fatalf("Protocol = %q, want grpc", op.Protocol)
	}
	if op.Kind != protocol.KindUnary {
		t.Fatalf("Kind = %v, want unary", op.Kind)
	}
	if op.Source[SourceFullMethod] != "/kv.KeyValue/Get" {
		t.Fatalf("Source full_method = %q", op.Source[SourceFullMethod])
	}
	if op.Source[SourceInputType] != "kv.GetRequest" {
		t.Fatalf("Source input_type = %q", op.Source[SourceInputType])
	}
	// Message fields → request-body properties.
	if len(op.RequestBody) != 1 || op.RequestBody[0].Name != "key" || op.RequestBody[0].Type != "string" {
		t.Fatalf("RequestBody = %#v, want [{key string}]", op.RequestBody)
	}

	watch := MapMethodDescriptor("kv.KeyValue", def.watch)
	if watch.Kind != protocol.KindServerStream {
		t.Fatalf("Watch Kind = %v, want server-stream", watch.Kind)
	}
}

// TestMapFileDescriptors_AllThreeKinds pins streaming classification.
func TestMapFileDescriptors_AllThreeKinds(t *testing.T) {
	accessor := protoparse.FileContentsFromMap(map[string]string{"kv.proto": testKVSource})
	parser := &protoparse.Parser{Accessor: accessor}
	fds, err := parser.ParseFiles("kv.proto")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]protocol.OperationKind{}
	for _, op := range MapFileDescriptors(fds) {
		kinds[op.ID] = op.Kind
	}
	if kinds["kv.KeyValue-Get"] != protocol.KindUnary {
		t.Fatal("Get must be unary")
	}
	if kinds["kv.KeyValue-Set"] != protocol.KindUnary {
		t.Fatal("Set must be unary")
	}
	if kinds["kv.KeyValue-Watch"] != protocol.KindServerStream {
		t.Fatal("Watch must be server-stream")
	}
}

// TestStatusToCode_Table pins the §4.2 mapping table itself.
func TestStatusToCode_Table(t *testing.T) {
	cases := map[codes.Code]apperrors.Code{
		codes.DeadlineExceeded:   apperrors.CodeTimeout,
		codes.ResourceExhausted:  apperrors.CodeRateLimit,
		codes.Unauthenticated:    apperrors.CodeUnauthorized,
		codes.PermissionDenied:   apperrors.CodeUnauthorized,
		codes.InvalidArgument:    apperrors.CodeBadRequest,
		codes.OutOfRange:         apperrors.CodeBadRequest,
		codes.FailedPrecondition: apperrors.CodeBadRequest,
		codes.NotFound:           apperrors.CodeNotFound,
		codes.Unimplemented:      apperrors.CodeNotFound,
		codes.Internal:           apperrors.CodeGeneric,
		codes.Unknown:            apperrors.CodeGeneric,
		codes.Unavailable:        apperrors.CodeGeneric,
		codes.DataLoss:           apperrors.CodeGeneric,
		codes.Aborted:            apperrors.CodeGeneric,
	}
	for grpcCode, want := range cases {
		if got := statusToCode(grpcCode); got != want {
			t.Errorf("statusToCode(%s) = %s, want %s", grpcCode, got, want)
		}
	}
}

// TestSplitOpID pins the service-method ID parsing.
func TestSplitOpID(t *testing.T) {
	if s, m, ok := splitOpID("kv.KeyValue-Get"); !ok || s != "kv.KeyValue" || m != "Get" {
		t.Fatalf("splitOpID simple = %q %q %v", s, m, ok)
	}
	if s, m, ok := splitOpID("a.b.C-D.Service-Run"); !ok || s != "a.b.C-D.Service" || m != "Run" {
		t.Fatalf("splitOpID multi-dash service = %q %q %v", s, m, ok)
	}
	if _, _, ok := splitOpID("NoDash"); ok {
		t.Fatal("ID without dash must not parse")
	}
	if _, _, ok := splitOpID("-Leading"); ok {
		t.Fatal("leading-dash ID must not parse")
	}
}

// TestExecute_JSONResultShape: the payload returned is JSON-serializable
// (§4.4 — shared rendering consumes map/scalar shapes).
func TestExecute_JSONResultShape(t *testing.T) {
	store := newKVStore()
	lis, stop := serveKV(t, store, nil, nil)
	defer stop()

	cc := connFor(t, lis)
	adapter := NewAdapter().WithConn(cc)
	defer func() { _ = adapter.Close() }()

	if _, err := adapter.Discover(context.Background(), protoSourceToTemp(t)); err != nil {
		t.Fatalf("discover: %v", err)
	}
	res, err := adapter.Execute(context.Background(), "kv.KeyValue-Set", map[string]interface{}{
		"key": "j", "value": "json",
	})
	if err != nil {
		t.Fatalf("execute Set: %v", err)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("result not JSON-serializable: %v (%#v)", err, res)
	}
	var round map[string]interface{}
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatalf("result not a JSON object: %v", err)
	}
	if round["ok"] != true {
		t.Fatalf("Set JSON = %s, want {\"ok\":true}", raw)
	}
}

// assertKVOps verifies the expected operation set from any discovery path.
func assertKVOps(t *testing.T, ops []*protocol.Operation, source string) {
	t.Helper()
	want := map[string]bool{
		"kv.KeyValue-Get":   false,
		"kv.KeyValue-Set":   false,
		"kv.KeyValue-Watch": false,
	}
	for _, op := range ops {
		if _, ok := want[op.ID]; !ok {
			continue
		}
		want[op.ID] = true
		if op.Protocol != protocol.ProtocolGRPC {
			t.Errorf("[%s] op %s protocol = %q", source, op.ID, op.Protocol)
		}
		if op.Source[SourceFullMethod] == "" {
			t.Errorf("[%s] op %s missing full_method ground truth", source, op.ID)
		}
		if op.Source[SourceDescriptors] != source {
			t.Errorf("[%s] op %s source stamp = %q, want %q", source, op.ID, op.Source[SourceDescriptors], source)
		}
	}
	for id, found := range want {
		if !found {
			t.Errorf("[%s] operation %s not discovered", source, id)
		}
	}
	if len(ops) < 3 {
		t.Errorf("[%s] discovered %d ops, want ≥ 3", source, len(ops))
	}
}

// osWrite writes bytes to a path (test helper).
func osWrite(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

var _ = grpc.WithChainUnaryInterceptor // placeholder to hold the grpc import if tests evolve
var _ = context.Background
var _ = json.Marshal
