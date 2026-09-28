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
// mount under a protocol group; root OpenAPI verbs stay frozen). Members:
// JSON-RPC 2.0 / OpenRPC (PROTO-004) and GraphQL (PROTO-005); later protocol
// adapters (soap, asyncapi, ...) register sibling subcommands here.

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
	cmd.AddCommand(newProtocolGraphQLCommand())
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

Discovery (--doc, no --method): fetches the OpenRPC document from --doc (a
local file path or an http(s) URL) and prints each method as a neutral
operation.

Call (--doc with --method): executes one method against --endpoint (default:
the document's servers[0].url) with the given JSON arguments and prints the
decoded result.

Call-only (--endpoint with --method, no --doc): sends the JSON-RPC 2.0
request straight to --endpoint with no discovery document fetch and no method
lookup. This is the integration path for a caller that already knows the
method name and its parameters; --method is required in this mode.

JSON-RPC batches are not implemented (optional in JSON-RPC 2.0); every call is
a single request.`,
	}

	cmd.Flags().StringVar(&docURI, "doc", "", "OpenRPC discovery document (file path or http(s) URL); enables discovery and document-backed calls")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "JSON-RPC endpoint URL (default: the document's servers[0].url); required for a call with no --doc")
	cmd.Flags().StringVar(&method, "method", "", "method to execute; with --endpoint and no --doc it calls directly (omit to only list discovered operations)")
	cmd.Flags().StringVar(&argsJSON, "args", "{}", "method arguments as a JSON object")
	cmd.Flags().DurationVar(&callTimeout, "timeout", jsonrpcDefaultTimeout, "per-call timeout (applies only when the call has no other deadline)")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		// One HTTP client per invocation (§3.1): discovery and execution share
		// it in --doc mode.
		httpClient := client.NewClient()

		// No --doc: endpoint-only (call-only) mode. The method name and
		// arguments are taken as given and no discovery document is fetched
		// (DF-025). Usage errors name the missing flag.
		if strings.TrimSpace(docURI) == "" {
			return runJSONRPCEndpointOnly(cmd, runJSONRPCEndpointOnlyParams{
				endpoint:   endpoint,
				method:     method,
				argsJSON:   argsJSON,
				timeout:    callTimeout,
				httpClient: httpClient,
			})
		}

		// --doc mode: discovery first, then the same request path. The
		// discoverer client's endpoint is irrelevant (the document names
		// the RPC endpoint); the execution client binds to the resolved
		// endpoint.
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

// runJSONRPCEndpointOnlyParams carries the validated endpoint-only inputs.
type runJSONRPCEndpointOnlyParams struct {
	endpoint   string
	method     string
	argsJSON   string
	timeout    time.Duration
	httpClient *client.Client
}

// runJSONRPCEndpointOnly executes one method against --endpoint without
// fetching an OpenRPC discovery document (DF-025). Because there is no
// document to look the method up in, both call inputs are mandatory: a
// missing one is reported as a usage error naming the flag that is absent.
func runJSONRPCEndpointOnly(cmd *cobra.Command, p runJSONRPCEndpointOnlyParams) error {
	endpoint := strings.TrimSpace(p.endpoint)
	method := strings.TrimSpace(p.method)

	switch {
	case endpoint == "" && method == "":
		return fmt.Errorf("--doc is required to discover operations; for a call without a discovery document pass --endpoint <url> and --method <name>")
	case endpoint == "":
		return fmt.Errorf("--endpoint is required when --doc is omitted (endpoint-only call of method %q)", method)
	case method == "":
		return fmt.Errorf("--method is required when --doc is omitted (endpoint-only call to %s); pass --method <name> or --doc <discovery document>", endpoint)
	}

	return runJSONRPCRequest(cmd, endpoint, method, p.argsJSON, p.timeout, p.httpClient)
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

	return runJSONRPCRequest(cmd, endpoint, target.ID, p.argsJSON, p.timeout, p.httpClient)
}

// runJSONRPCRequest executes one JSON-RPC 2.0 method against endpoint and
// prints the decoded result. Both invocation modes (document-backed and
// endpoint-only) funnel through here: the request build and the response
// decode live in pkg/protocol/jsonrpc, which reuses the pkg/mcp codec, so this
// function only decodes the caller's JSON arguments and renders the result.
func runJSONRPCRequest(cmd *cobra.Command, endpoint, method, argsJSON string, timeout time.Duration, httpClient *client.Client) error {
	var callArgs map[string]interface{}
	if strings.TrimSpace(argsJSON) != "" {
		if err := json.Unmarshal([]byte(argsJSON), &callArgs); err != nil {
			return fmt.Errorf("invalid --args JSON object: %w", err)
		}
	}

	executor := jsonrpc.NewClient(endpoint,
		jsonrpc.WithHTTPClient(httpClient),
		jsonrpc.WithTimeout(timeout),
	)

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := executor.Execute(ctx, method, callArgs)
	if err != nil {
		return err
	}

	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("render result: %w", err)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
	return nil
}

// printJSONRPCOperations writes the discovered operations as JSON.
func printJSONRPCOperations(w io.Writer, ops []*protocol.Operation) error {
	if len(ops) == 0 {
		_, _ = fmt.Fprintln(w, "[]")
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
