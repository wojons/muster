package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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

	// FlagGrpcArg is a repeatable --arg key=value flag carrying operation
	// arguments (mirrors grpcurl ergonomics).
	FlagGrpcArg = "arg"

	// FlagGrpcToken attaches "authorization: Bearer <token>" metadata (§4.1).
	// pkg/auth remains the token store for OAuth flows; this flag carries an
	// operator-provided token directly.
	FlagGrpcToken = "token"

	// FlagGrpcHeader is a repeatable raw "key: value" metadata flag.
	FlagGrpcHeader = "header"
)

// newGrpcCommand builds the `grpc` protocol group.
func newGrpcCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grpc",
		Short: "gRPC operations (discover, call) — unary",
		Long: `gRPC protocol adapter (specs/PROTOCOLS.md, matrix row 2).

Discovery sources: .proto files ("dir1:dir2=file.proto" — a relative file
resolves against dir1, then cwd; dir1:dir2 are the import paths for its
includes), compiled descriptor sets (any non-.proto file), or server
reflection ("grpc://host:port").

Invocation is unary-only: streaming methods are rejected with a clear
error (PROTO-003 boundary).`,
	}

	cmd.AddCommand(newGrpcDiscoverCommand())
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

// newGrpcCallCommand builds `grpc call`: one unary invocation end-to-end.
func newGrpcCallCommand() *cobra.Command {
	var (
		source   string
		target   string
		argPairs []string
		token    string
		headers  []string
		format   string
	)
	cmd := &cobra.Command{
		Use:   "call <service-method>",
		Short: "Invoke one unary gRPC method",
		Long: `Invoke one unary gRPC method end-to-end.

The operation ID uses the service-method form printed by
'openapi-cli grpc discover' (e.g. kv.KeyValue-Get).

Examples:
  openapi-cli grpc call kv.KeyValue-Get --source kv.proto --target localhost:9000 --arg key=demo
  openapi-cli grpc call kv.KeyValue-Get --source grpc://localhost:9000 --arg key=demo`,
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

			adapter := grpcadapter.NewAdapter()
			defer func() { _ = adapter.Close() }()

			// Discovery (file or reflection) populates the adapter's
			// descriptor cache; reflection sources also dial the target.
			if _, err := adapter.Discover(ctx, source); err != nil {
				return err
			}

			if err := adapter.Configure(ctx, resolvedTarget); err != nil {
				return err
			}
			adapter.WithMetadata(buildMetadataCarrier(token, headers))

			result, err := adapter.Execute(ctx, args[0], callArgs)
			if err != nil {
				return err
			}

			return printGrpcResult(cmd, result, format)
		},
	}
	cmd.Flags().StringVar(&source, FlagGrpcSource, "", "Discovery source: file.proto, descriptor-set file, or grpc://host:port (required)")
	cmd.Flags().StringVar(&target, FlagGrpcTarget, "", "Invocation target host:port (optional when --source grpc://host:port)")
	cmd.Flags().StringArrayVar(&argPairs, FlagGrpcArg, nil, "Operation argument, repeatable: --arg key=value")
	cmd.Flags().StringVar(&token, FlagGrpcToken, "", "Bearer token for the authorization metadata key")
	cmd.Flags().StringArrayVar(&headers, FlagGrpcHeader, nil, "Raw metadata header, repeatable: --header key:value")
	cmd.Flags().StringVarP(&format, "output", "o", "json", "Output format (json, raw)")
	_ = cmd.MarkFlagRequired(FlagGrpcSource)

	return cmd
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
