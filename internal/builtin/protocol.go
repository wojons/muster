package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wojons/muster/pkg/client"
	"github.com/wojons/muster/pkg/protocol"
	"github.com/wojons/muster/pkg/protocol/jsonrpc"
)

// ── protocol ─────────────────────────────────────────────────────────────
// The protocol subcommand namespace (specs/PROTOCOLS.md §5.1: new protocols
// mount under a protocol group; root OpenAPI verbs stay frozen). JSON-RPC
// 2.0 / OpenRPC is the first member (PROTO-004); later protocol adapters
// (grpc, graphql, ...) register sibling subcommands here.

// jsonrpcDefaultTimeout bounds each Execute call when the caller's context
// carries no deadline (the caller's own deadline always wins per
// specs/PROTOCOLS.md §4.3). CLI invocations have no ambient deadline, so a
// bounded default keeps interactive commands from hanging forever.
const jsonrpcDefaultTimeout = 30 * time.Second

// newProtocolCommand builds the `protocol` command group.
func newProtocolCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "protocol",
		Short: "Discover and call operations from non-OpenAPI protocols",
		Long: `Discover and call operations from protocol families other than OpenAPI.

Each protocol family mounts a subcommand (e.g. protocol jsonrpc). Discovery
ingests the protocol's discovery document and prints the neutral operation
model; calls execute one operation and print the JSON result.

Spec: specs/PROTOCOLS.md (protocol adapter seam).`,
	}

	cmd.AddCommand(newProtocolJSONRPCCommand())
	return cmd
}

// newProtocolJSONRPCCommand builds `protocol jsonrpc` (PROTO-004).
func newProtocolJSONRPCCommand() *cobra.Command {
	var (
		docURI      string
		endpoint    string
		method      string
		argsJSON    string
		callTimeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "jsonrpc",
		Short: "JSON-RPC 2.0 / OpenRPC adapter",
		Long: `Discover JSON-RPC 2.0 methods from an OpenRPC discovery document and
execute them over HTTP POST.

Discovery (no --method): fetches the OpenRPC document from --doc (a local
file path or an http(s) URL) and prints each method as a neutral operation.

Call (--method): executes one method against --endpoint with the given JSON
arguments and prints the decoded result. JSON-RPC batches are not
implemented (optional in JSON-RPC 2.0); every call is a single request.`,
	}

	cmd.Flags().StringVar(&docURI, "doc", "", "OpenRPC discovery document (file path or http(s) URL)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "JSON-RPC endpoint URL (default: the document's servers[0].url)")
	cmd.Flags().StringVar(&method, "method", "", "method to execute (omit to only list discovered operations)")
	cmd.Flags().StringVar(&argsJSON, "args", "{}", "method arguments as a JSON object")
	cmd.Flags().DurationVar(&callTimeout, "timeout", jsonrpcDefaultTimeout, "per-call timeout (applies only when the call has no other deadline)")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(docURI) == "" {
			return fmt.Errorf("--doc is required (OpenRPC discovery document)")
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		// The discoverer client's endpoint is irrelevant (the document names
		// the RPC endpoint); the execution client binds to the resolved
		// endpoint. Both share one HTTP client per §3.1.
		httpClient := client.NewClient()
		discoverer := jsonrpc.NewClient("http://127.0.0.1",
			jsonrpc.WithHTTPClient(httpClient),
		)

		ops, err := discoverer.Discover(ctx, docURI)
		if err != nil {
			return err
		}

		if strings.TrimSpace(method) == "" {
			return printJSONRPCOperations(cmd.OutOrStdout(), ops)
		}

		return runJSONRPCCall(cmd, runJSONRPCCallParams{
			ops:        ops,
			endpoint:   endpoint,
			method:     method,
			argsJSON:   argsJSON,
			timeout:    callTimeout,
			httpClient: httpClient,
		})
	}

	return cmd
}

// runJSONRPCCallParams carries the validated call inputs.
type runJSONRPCCallParams struct {
	ops        []*protocol.Operation
	endpoint   string
	method     string
	argsJSON   string
	timeout    time.Duration
	httpClient *client.Client
}

// runJSONRPCCall resolves the endpoint (flag wins, else the operation's
// Source stamp) and executes one method, printing the JSON result.
func runJSONRPCCall(cmd *cobra.Command, p runJSONRPCCallParams) error {
	var target *protocol.Operation
	for _, op := range p.ops {
		if op.ID == p.method {
			target = op
			break
		}
	}
	if target == nil {
		return fmt.Errorf("method %q not found in the discovery document (discovered: %s)",
			p.method, strings.Join(operationIDs(p.ops), ", "))
	}

	endpoint := p.endpoint
	if endpoint == "" {
		// Executor ground truth lives on Operation.Source (§3.2); the
		// stage-3 wiring reads it only here, in the adapter's own path.
		endpoint = target.Source["server"]
	}
	if endpoint == "" {
		return fmt.Errorf("no endpoint: pass --endpoint or add servers[] to the OpenRPC document")
	}

	var callArgs map[string]interface{}
	if strings.TrimSpace(p.argsJSON) != "" {
		if err := json.Unmarshal([]byte(p.argsJSON), &callArgs); err != nil {
			return fmt.Errorf("invalid --args JSON object: %w", err)
		}
	}

	executor := jsonrpc.NewClient(endpoint,
		jsonrpc.WithHTTPClient(p.httpClient),
		jsonrpc.WithTimeout(p.timeout),
	)

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := executor.Execute(ctx, target.ID, callArgs)
	if err != nil {
		return err
	}

	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("render result: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(out))
	return nil
}

// printJSONRPCOperations writes the discovered operations as JSON.
func printJSONRPCOperations(w io.Writer, ops []*protocol.Operation) error {
	if len(ops) == 0 {
		fmt.Fprintln(w, "[]")
		return nil
	}

	type opView struct {
		ID          string            `json:"id"`
		Kind        string            `json:"kind"`
		Summary     string            `json:"summary,omitempty"`
		Description string            `json:"description,omitempty"`
		Parameters  []map[string]any  `json:"parameters,omitempty"`
		Source      map[string]string `json:"source,omitempty"`
	}

	views := make([]opView, 0, len(ops))
	for _, op := range ops {
		view := opView{
			ID:          op.ID,
			Summary:     op.Summary,
			Description: op.Description,
			Source:      op.Source,
		}
		switch op.Kind {
		case protocol.KindUnary:
			view.Kind = "unary"
		case protocol.KindServerStream:
			view.Kind = "server-stream"
		case protocol.KindClientStream:
			view.Kind = "client-stream"
		case protocol.KindBidiStream:
			view.Kind = "bidi-stream"
		case protocol.KindEventSource:
			view.Kind = "event-source"
		}
		for _, param := range op.Parameters {
			view.Parameters = append(view.Parameters, map[string]any{
				"name":        param.Name,
				"in":          param.In,
				"type":        param.Type,
				"required":    param.Required,
				"description": param.Description,
			})
		}
		views = append(views, view)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(views)
}

// operationIDs returns the operations' IDs in document order.
func operationIDs(ops []*protocol.Operation) []string {
	ids := make([]string, 0, len(ops))
	for _, op := range ops {
		ids = append(ids, op.ID)
	}
	return ids
}
