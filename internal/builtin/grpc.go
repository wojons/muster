package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wojons/muster/pkg/protocol"
	grpcadapter "github.com/wojons/muster/pkg/protocol/grpc"
)

// Protocol command namespace (specs/PROTOCOLS.md §5.1): OpenAPI operations
// stay at the root; new protocols mount under their own group. Today: the
// `grpc` group with two verbs:
//
//	openapi-cli grpc discover <uri>          list operations from a discovery source
//	openapi-cli grpc call <service-method>   invoke one unary operation
//
// The wiring stays thin (PROTO-002): the adapter owns transport, auth
// metadata, deadline handling, and error mapping; this file only maps flags
// to the neutral model and renders the result (§4.4).
const (
	// FlagGrpcSource is the discovery source for `grpc call`: .proto file(s),
	// a compiled descriptor set, or "grpc://host:port" for reflection.
	FlagGrpcSource = "source"

	// FlagGrpcTarget is the invocation target host:port (required unless
	// the discovery source already names one, e.g. grpc://host:port).
	FlagGrpcTarget = "target"

	// FlagGrpcArg is a repeatable --arg key=value flag carrying the single
	// request message's fields (mirrors grpcurl ergonomics).
	FlagGrpcArg = "arg"

	// FlagGrpcToken attaches the standard "authorization: Bearer" metadata
	// header (§4.1). pkg/auth remains the token store for OAuth flows; this
	// flag carries an operator-provided token directly.
	FlagGrpcToken = "token"

	// FlagGrpcHeader is a repeatable raw "key: value" metadata flag.
	FlagGrpcHeader = "header"

	// FlagGrpcMaxMessages bounds a streaming invocation's collected items
	// (§4.5). There is no unbounded setting.
	FlagGrpcMaxMessages = "max-messages"

	// FlagGrpcTimeout bounds the wall clock of one streaming invocation and
	// is the request deadline for unary calls.
	FlagGrpcTimeout = "timeout"

	// FlagGrpcMessages carries the request message STREAM of a
	// client-streaming or bidirectional call as a JSON array.
	FlagGrpcMessages = "messages"
)

// newGrpcCommand builds the `grpc` protocol group.
func newGrpcCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grpc",
		Short: "gRPC operations (discover, schema, call) — unary and streaming",
		Long: `gRPC protocol adapter (specs/PROTOCOLS.md, matrix rows 2 and 3).

Discovery sources: .proto files ("dir1:dir2=file.proto" — a relative file
resolves against dir1, then cwd; dir1:dir2 are the import paths for its
includes), compiled descriptor sets (any non-.proto file), or server
reflection ("grpc://host:port").

'grpc discover' lists OPERATIONS (RPCs), so a file with no service lists none;
'grpc schema' lists the TYPES a source declares, so the same file is still
fully described. A descriptor set built without 'protoc --include_imports' is
closed from the protobuf runtime's well-known types when possible, and
otherwise fails with a message that names the missing file and the remedy.

Invocation covers all four RPC kinds: unary, server-streaming,
client-streaming, and bidirectional streaming. Every streaming call returns a
BOUNDED transcript envelope, never a live stream — see 'grpc call --help' for
the contract, the bound flags, and the truncation markers.`,
	}

	cmd.AddCommand(newGrpcDiscoverCommand())
	cmd.AddCommand(newGrpcSchemaCommand())
	cmd.AddCommand(newGrpcCallCommand())
	return cmd
}

// newGrpcDiscoverCommand builds `grpc discover <uri>`: ingest a source and
// list the resulting operations (kind, ID, full method).
func newGrpcDiscoverCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "discover <uri>",
		Short: "List gRPC operations from a .proto file, descriptor set, or reflection target",
		Long: `Ingest a gRPC discovery source and print the operations it exposes.

<uri>:
  file.proto               .proto source, resolved against the current directory
  dir1:dir2=file.proto     .proto file resolved against dir1 (then cwd), with
                           dir1:dir2 as import paths for its includes
  file.pb (any non-.proto) compiled FileDescriptorSet
  grpc://host:port         live server reflection`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			adapter := grpcadapter.NewAdapter()
			defer func() { _ = adapter.Close() }()

			ops, err := adapter.Discover(ctx, args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(ops) == 0 {
				fmt.Fprintln(out, "No operations found.")
				return nil
			}
			sort.Slice(ops, func(i, j int) bool { return ops[i].ID < ops[j].ID })
			for _, op := range ops {
				fmt.Fprintf(out, "%-16s %s\n", grpcKindLabel(op.Kind), op.ID)
				fmt.Fprintf(out, "         %s\n", op.Source[grpcadapter.SourceFullMethod])
			}
			return nil
		},
	}
}

// newGrpcSchemaCommand builds `grpc schema <uri>`: ingest a source and list the
// protobuf type graph it declares — messages, enums and their depth-1 field
// facts — from any of the three discovery inputs. It is the second stage-1
// producer over the same loaders (specs/PROTOCOLS.md §3.2, PROTO-013 §2.6 S3):
// the schema surface answers "what types does this source declare?", which is a
// different question from "which RPCs does it expose?", so a service-less
// .proto has no operations and still has a schema.
func newGrpcSchemaCommand() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "schema <uri>",
		Short: "List the protobuf type graph a discovery source declares",
		Long: `Ingest a discovery source and print the types it declares.

<uri> accepts the same three forms as 'grpc discover': a .proto file
("dir1:dir2=file.proto"), a compiled FileDescriptorSet, or grpc://host:port
for live server reflection.

The listing is one line per type (kind, fully-qualified name, field or value
count) plus a total. Unlike 'grpc discover', a source with no service is still
fully described: message and enum declarations are the schema surface, while
operations are RPCs.

Output
  -o text   the type listing (default)
  -o json   the whole schema set as JSON: types keyed by fully-qualified name,
            each with its fields (type, format, ref, repeated/map details,
            enum values, oneof group) and the source stamps

Examples:
  openapi-cli grpc schema schema.proto
  openapi-cli grpc schema msgs_only.proto -o json
  openapi-cli grpc schema grpc://localhost:9000`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			adapter := grpcadapter.NewAdapter()
			defer func() { _ = adapter.Close() }()

			set, err := adapter.Schema(ctx, args[0])
			if err != nil {
				return err
			}
			return printGrpcSchema(cmd, set, format)
		},
	}
	cmd.Flags().StringVarP(&format, "output", "o", "text", "Output format (text, json)")
	return cmd
}

// printGrpcSchema renders a schema set: a stable text listing, or the set
// itself as JSON (§4.4 — the output layer formats, the adapter never prints).
func printGrpcSchema(cmd *cobra.Command, set *protocol.SchemaSet, format string) error {
	out := cmd.OutOrStdout()
	switch format {
	case "json":
		pretty, err := json.MarshalIndent(set, "", "  ")
		if err != nil {
			return fmt.Errorf("format schema: %w", err)
		}
		_, _ = fmt.Fprintln(out, string(pretty))
		return nil
	case "text":
		names := set.Names()
		if len(names) == 0 {
			fmt.Fprintln(out, "No types found.")
			return nil
		}
		for _, name := range names {
			typ, _ := set.Type(name)
			detail := fmt.Sprintf("%d %s", len(typ.Fields), plural(len(typ.Fields), "field", "fields"))
			if typ.Kind == protocol.SchemaTypeEnum {
				detail = fmt.Sprintf("%d %s", len(typ.EnumValues), plural(len(typ.EnumValues), "value", "values"))
			}
			fmt.Fprintf(out, "%-8s %-40s %s\n", typ.Kind, name, detail)
		}
		fmt.Fprintf(out, "%d %s\n", len(names), plural(len(names), "type", "types"))
		return nil
	default:
		return fmt.Errorf("unsupported output format %q: supported formats are text, json", format)
	}
}

// plural picks the singular or plural noun for a count.
func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

// grpcKindLabel renders an OperationKind for the discover listing.
func grpcKindLabel(kind protocol.OperationKind) string {
	switch kind {
	case protocol.KindUnary:
		return "unary"
	case protocol.KindServerStream:
		return "server-streaming"
	case protocol.KindClientStream:
		return "client-streaming"
	case protocol.KindBidiStream:
		return "bidi"
	default:
		return "event-source"
	}
}

// newGrpcCallCommand builds `grpc call`: one invocation end-to-end (all four
// RPC kinds) whose output is bounded per specs/PROTOCOLS.md §4.5.
func newGrpcCallCommand() *cobra.Command {
	var (
		source      string
		target      string
		argPairs    []string
		token       string
		headers     []string
		format      string
		maxMessages int
		callTimeout time.Duration
		messagesRaw string
	)
	cmd := &cobra.Command{
		Use:   "call <service-method>",
		Short: "Invoke one gRPC method (unary or streaming, bounded output)",
		Long: `Invoke one gRPC method end-to-end: unary, server-streaming,
client-streaming, or bidirectional.

The operation ID uses the service-method form printed by
'openapi-cli grpc discover' (e.g. kv.KeyValue-Get).

Request messages
  unary / server-streaming   one request: --arg key=value (repeatable)
  client-streaming / bidi    a request STREAM: --messages '[{"k":1},{"k":2}]'
                             --messages also accepts @file.json, or @- for stdin
  Passing both --arg and --messages is rejected as ambiguous; a method that
  takes one request message rejects --messages.

Bounded output contract (specs/PROTOCOLS.md §4.5)
  Every streaming call returns a BOUNDED transcript envelope, never a live
  stream — an unbounded stream would hang the caller:

    {
      "kind": "server-stream" | "client-stream" | "bidi-stream",
      "status": "complete" | "truncated" | "cancelled" | "error",
      "items": [ ... up to --max-messages response messages ... ],
      "items_truncated": false,
      "trailer": { "received": 3, "messages_sent": 3, "max_messages": 1000,
                   "duration_ms": 12, "limit_reason": "max-messages",
                   "metadata": { ... gRPC trailer ... } }
    }

  --max-messages N   stop after N response items (default 1000; 0 selects the
                     default). There is no unbounded setting.
  --timeout D        wall-clock bound for one streaming call, sending included
                     (default 30s; 0 selects the default), and the request
                     deadline for a unary call.

  Hitting either bound is a CLEAN stop: the stream is cancelled locally, the
  exit code stays 0, and the envelope says so — status "truncated",
  items_truncated true, and trailer.limit_reason naming the bound. For
  client-streaming the cap truncates the REQUEST stream; the server's answer to
  that capped stream is still collected and reported.

Cancellation and backpressure
  A caller cancellation or deadline on a streaming call ends it immediately:
  status "cancelled" with a non-zero exit and a timeout error, plus whatever
  items already arrived. Requests are sent one at a time under the transport's
  flow control, so a slow server blocks the sender instead of buffering the
  stream in memory; bidi calls send and collect concurrently so a server that
  only answers while being read cannot deadlock. Trailer metadata is reported
  when the server ended the stream; a locally truncated call may not carry it.
  A mid-stream server error prints the partial transcript and then fails with
  the mapped error code.

Examples:
  openapi-cli grpc call kv.KeyValue-Get --source kv.proto --target localhost:9000 --arg key=demo
  openapi-cli grpc call kv.KeyValue-Get --source grpc://localhost:9000 --arg key=demo
  openapi-cli grpc call kv.KeyValue-Watch --source kv.proto --target localhost:9000 --arg prefix=k --max-messages 5
  openapi-cli grpc call kv.KeyValue-Upload --source kv.proto --target localhost:9000 --messages '[{"key":"a"},{"key":"b"}]'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			// Resolve the invocation target: explicit --target wins; else the
			// reflection source doubles as the target; else error.
			resolvedTarget := target
			if resolvedTarget == "" && strings.HasPrefix(source, "grpc://") {
				resolvedTarget = strings.TrimPrefix(source, "grpc://")
			}
			if resolvedTarget == "" {
				return fmt.Errorf("no invocation target: pass --target host:port or use --source grpc://host:port")
			}

			callArgs, err := parseKeyValueFlags(argPairs)
			if err != nil {
				return err
			}
			streamMessages, messagesSet, err := parseStreamMessages(messagesRaw, cmd.InOrStdin())
			if err != nil {
				return err
			}
			if messagesSet && len(argPairs) > 0 {
				return fmt.Errorf("--arg and --messages are mutually exclusive: use --arg for one request message, or --messages for a request stream")
			}

			adapter := grpcadapter.NewAdapter()
			defer func() { _ = adapter.Close() }()
			adapter.WithStreamLimits(grpcadapter.StreamLimits{
				MaxMessages: maxMessages,
				MaxDuration: callTimeout,
			})

			// Discovery (file or reflection) populates the adapter's
			// descriptor cache; reflection sources also dial the target.
			ops, err := adapter.Discover(ctx, source)
			if err != nil {
				return err
			}

			if err := adapter.Configure(ctx, resolvedTarget); err != nil {
				return err
			}
			adapter.WithMetadata(buildMetadataCarrier(token, headers))

			// The discovered kind decides how the request is fed and which
			// bound guards the call. An undiscovered ID falls through to
			// Execute, which reports it as CodeNotFound.
			kind, discovered := grpcOperationKind(ops, args[0])
			switch {
			case !discovered:
			case messagesSet && kind != protocol.KindClientStream && kind != protocol.KindBidiStream:
				return fmt.Errorf("--messages applies to client-streaming and bidirectional methods; %s is %s (use --arg for its single request message)",
					args[0], grpcKindLabel(kind))
			case !messagesSet && (kind == protocol.KindClientStream || kind == protocol.KindBidiStream):
				// Legal either way: --arg seeds a one-message request
				// stream, and no messages at all is a valid empty stream.
				// Say which one is being sent rather than leaving it
				// implicit.
				if len(callArgs) > 0 {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "note: %s is %s; --arg seeds a ONE-message request stream (use --messages for more)\n",
						args[0], grpcKindLabel(kind))
				} else {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "note: %s is %s and no --messages were given; sending an empty request stream\n",
						args[0], grpcKindLabel(kind))
				}
			}

			// Streaming calls are bounded by the adapter's stream limits (which
			// report a truncation marker instead of failing); unary calls get an
			// ordinary context deadline so an unresponsive server cannot hang.
			if !discovered || kind == protocol.KindUnary {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, callTimeout)
				defer cancel()
			}

			var result interface{}
			if messagesSet {
				result, err = adapter.ExecuteMessages(ctx, args[0], streamMessages)
			} else {
				result, err = adapter.Execute(ctx, args[0], callArgs)
			}

			// A partial transcript (mid-stream error, caller cancellation) is
			// still worth printing: the caller sees what arrived, then the
			// failure that ended it.
			if result != nil {
				if printErr := printGrpcResult(cmd, result, format); printErr != nil {
					return printErr
				}
			}
			return err
		},
	}
	cmd.Flags().StringVar(&source, FlagGrpcSource, "", "Discovery source: file.proto, descriptor-set file, or grpc://host:port (required)")
	cmd.Flags().StringVar(&target, FlagGrpcTarget, "", "Invocation target host:port (optional when --source grpc://host:port)")
	cmd.Flags().StringArrayVar(&argPairs, FlagGrpcArg, nil, "Request message field, repeatable: --arg key=value")
	cmd.Flags().StringVar(&messagesRaw, FlagGrpcMessages, "", "Request message stream for client-streaming/bidi methods: JSON array of objects, @file.json, or @- for stdin")
	cmd.Flags().IntVar(&maxMessages, FlagGrpcMaxMessages, grpcadapter.DefaultMaxMessages, "Streaming bound: stop after this many response items (0 selects the default)")
	cmd.Flags().DurationVar(&callTimeout, FlagGrpcTimeout, grpcadapter.DefaultMaxDuration, "Streaming bound: wall-clock limit for the whole call (0 selects the default); also the request deadline for unary calls")
	cmd.Flags().StringVar(&token, FlagGrpcToken, "", "Bearer token for the authorization metadata header")
	cmd.Flags().StringArrayVar(&headers, FlagGrpcHeader, nil, "Raw metadata header, repeatable: --header key:value")
	cmd.Flags().StringVarP(&format, "output", "o", "json", "Output format (json, raw)")
	_ = cmd.MarkFlagRequired(FlagGrpcSource)

	return cmd
}

// grpcOperationKind finds the discovered operation the ID names.
func grpcOperationKind(ops []*protocol.Operation, id string) (protocol.OperationKind, bool) {
	for _, op := range ops {
		if op.ID == id {
			return op.Kind, true
		}
	}
	return protocol.KindUnary, false
}

// parseStreamMessages reads the --messages flag: an inline JSON array of
// request message objects, "@file.json" for that array in a file, or "@-" for
// stdin. It reports whether the flag was set at all, so an explicitly empty
// array (a legal empty request stream) is distinguishable from "not given".
func parseStreamMessages(flag string, stdin io.Reader) ([]map[string]interface{}, bool, error) {
	if strings.TrimSpace(flag) == "" {
		return nil, false, nil
	}
	raw := []byte(flag)
	if strings.HasPrefix(flag, "@") {
		path := strings.TrimPrefix(flag, "@")
		var err error
		if path == "-" {
			if stdin == nil {
				stdin = os.Stdin
			}
			raw, err = io.ReadAll(stdin)
		} else {
			raw, err = os.ReadFile(path)
		}
		if err != nil {
			return nil, true, fmt.Errorf("read --messages source %s: %w", flag, err)
		}
	}
	var messages []map[string]interface{}
	if err := json.Unmarshal(raw, &messages); err != nil {
		return nil, true, fmt.Errorf("--messages must be a JSON array of message objects (or @file.json): %w", err)
	}
	return messages, true, nil
}

// parseKeyValueFlags parses repeated "key=value" flags into a map.
func parseKeyValueFlags(pairs []string) (map[string]interface{}, error) {
	args := make(map[string]interface{}, len(pairs))
	for _, pair := range pairs {
		idx := strings.Index(pair, "=")
		if idx <= 0 {
			return nil, fmt.Errorf("invalid --arg %q: expected key=value", pair)
		}
		args[pair[:idx]] = parseGrpcScalar(pair[idx+1:])
	}
	return args, nil
}

// parseGrpcScalar converts a CLI string to the scalar the proto JSON mapping
// expects (bools are parsed; everything else stays a string — proto JSON
// accepts numeric strings for numeric fields).
func parseGrpcScalar(v string) interface{} {
	switch strings.ToLower(v) {
	case "true":
		return true
	case "false":
		return false
	}
	return v
}

// buildMetadataCarrier assembles the auth carrier from --token/--header
// flags (§4.1: static mapping only; the adapter never invents a token store).
func buildMetadataCarrier(token string, headers []string) *grpcadapter.MetadataCarrier {
	var pairs []string
	if token != "" {
		pairs = append(pairs, "authorization", "Bearer "+token)
	}
	for _, h := range headers {
		idx := strings.Index(h, ":")
		if idx <= 0 {
			continue
		}
		pairs = append(pairs, strings.TrimSpace(h[:idx]), strings.TrimSpace(h[idx+1:]))
	}
	if len(pairs) == 0 {
		return nil
	}
	return grpcadapter.NewMetadataCarrier(pairs...)
}

// printGrpcResult renders the executor's JSON-serializable payload. `json`
// pretty-prints; `raw` prints compact JSON. §4.4: rendering lives in the
// output layer, the adapter never writes stdout itself.
func printGrpcResult(cmd *cobra.Command, result interface{}, format string) error {
	out := cmd.OutOrStdout()
	switch format {
	case "raw":
		compact, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("format result: %w", err)
		}
		_, _ = fmt.Fprintln(out, string(compact))
	case "json":
		pretty, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("format result: %w", err)
		}
		_, _ = fmt.Fprintln(out, string(pretty))
	default:
		return fmt.Errorf("unsupported output format %q: supported formats are json, raw", format)
	}
	return nil
}
