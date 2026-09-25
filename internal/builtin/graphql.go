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
	graphqladapter "github.com/wojons/muster/pkg/protocol/graphql"
)

// Protocol command namespace (specs/PROTOCOLS.md §5.1): OpenAPI operations stay
// at the root; new protocols mount under their own group. The GraphQL group
// (PROTO-005) follows the JSON-RPC group's shape: one command whose flags select
// discovery or one invocation.
//
//	openapi-cli protocol graphql --schema schema.graphql
//	openapi-cli protocol graphql --schema graphql://host:port/graphql --operation user --args '{"id":"u1"}'
//
// The wiring stays thin: the adapter owns schema ingestion, the query document,
// the single-endpoint POST, error mapping and the request builder; this file
// maps flags to the neutral model and renders results (§4.4).
const (
	// FlagGraphQLSchema is the discovery source: an SDL document, a recorded
	// introspection response, or graphql://host[:port]/path for a live
	// introspection query.
	FlagGraphQLSchema = "schema"

	// FlagGraphQLEndpoint is the invocation endpoint. It overrides the
	// endpoint the schema source named (a live introspection target).
	FlagGraphQLEndpoint = "endpoint"

	// FlagGraphQLOperation is the operation (field) to invoke; omitted means
	// list the discovered operations.
	FlagGraphQLOperation = "operation"

	// FlagGraphQLArgs is the operation's arguments as a JSON object. Arguments
	// are bound as GraphQL variables — never interpolated into the document.
	FlagGraphQLArgs = "args"

	// FlagGraphQLFields restricts the derived selection's top level to these
	// field names (comma-separated), in order.
	FlagGraphQLFields = "fields"

	// FlagGraphQLDocument prints the query document the operation builds
	// instead of calling the endpoint.
	FlagGraphQLDocument = "document"

	// FlagGraphQLMaxDepth / FlagGraphQLMaxFields bound the derived selection.
	FlagGraphQLMaxDepth  = "max-depth"
	FlagGraphQLMaxFields = "max-fields"

	// FlagGraphQLTimeout bounds a call whose caller's context carries no
	// deadline.
	FlagGraphQLTimeout = "timeout"

	// FlagGraphQLOutput selects json (pretty) or raw (compact) rendering.
	FlagGraphQLOutput = "output"
)

// newProtocolGraphQLCommand builds `protocol graphql` (PROTO-005).
func newProtocolGraphQLCommand() *cobra.Command {
	var (
		schemaURI string
		endpoint  string
		operation string
		argsJSON  string
		fieldsCSV string
		document  bool
		maxDepth  int
		maxFields int
		timeout   time.Duration
		output    string
	)

	cmd := &cobra.Command{
		Use:   "graphql",
		Short: "GraphQL adapter: ingest a schema, invoke one query or mutation over a single POST endpoint",
		Long: `Ingest a GraphQL schema and call it as typed commands.

Schema ingestion (--schema):
  schema.graphql | introspection.json   a local SDL or recorded introspection document
  http(s)://host/schema.graphql        a document served over HTTP
  file:///path/schema.graphql          an explicit file URL
  graphql://host[:port]/path           run the standard introspection QUERY against
                                       that endpoint and build commands from the result

Listing (--schema without --operation): prints every ingested operation as JSON,
including the source stamp naming where each operation came from.

Calling (--schema with --operation): builds the operation's query document,
binds --args as GraphQL variables, and sends ONE POST to --endpoint (default: the
endpoint the schema source named, i.e. the introspection target). The REST-style
path/method request builder is deliberately not used: GraphQL is one endpoint,
one POST, a document plus variables.

Other flags:
  --document           print the query document instead of calling the endpoint
  --fields a,b         restrict the derived selection's top level, in order
  --max-depth N        selection recursion depth (default 2)
  --max-fields N       max selected fields per level (default 25)

Known limits (bounded first row — subscriptions are the follow-up row):
  * Subscriptions are not ingested; the Subscription root is recognized and skipped.
  * Selection sets are DERIVED from the schema (never hand-written): leaf fields,
    recursion to --max-depth, union fields selected as __typename, and fields whose
    arguments include a required argument are skipped. The document therefore
    contains only variable DECLARATIONS — argument values travel as variables.
  * SDL ingestion covers the type system: schema/type/extend/interface/union/enum/
    input/scalar plus descriptions and directive applications (directives are
    parsed, then ignored). Inline fragments are not generated.
  * Custom scalars have no JSON type of their own: they map to the neutral
    "string" type and are sent through unchanged.
  * Introspection disabled by the server is an actionable error naming the SDL
    fallback, never an empty command set.

Spec: specs/PROTOCOLS.md (protocol adapter seam).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			if strings.TrimSpace(schemaURI) == "" {
				return fmt.Errorf("--schema is required: pass an SDL document (schema.graphql), a recorded introspection response (introspection.json), or graphql://host:port/path for live introspection")
			}

			callArgs, err := parseGraphQLArgs(argsJSON)
			if err != nil {
				return err
			}

			// One HTTP client per invocation (§3.1): discovery and execution
			// share it.
			httpClient := client.NewClient()
			adapter := graphqladapter.NewClient(endpoint,
				graphqladapter.WithHTTPClient(httpClient),
				graphqladapter.WithTimeout(timeout),
				graphqladapter.WithSelection(graphqladapter.SelectionOptions{
					MaxDepth:  maxDepth,
					MaxFields: maxFields,
					Fields:    splitCSV(fieldsCSV),
				}),
			)

			ops, err := adapter.Discover(ctx, schemaURI)
			if err != nil {
				return err
			}

			if strings.TrimSpace(operation) == "" {
				return printGraphQLOperations(cmd.OutOrStdout(), ops)
			}

			target := graphqladapter.OperationByID(ops, operation)
			if target == nil {
				return fmt.Errorf("operation %q not found in %s (discovered: %s)",
					operation, schemaURI, strings.Join(graphqladapter.OperationIDs(ops), ", "))
			}

			if document {
				req, err := adapter.Builder().Build(target, callArgs, endpoint)
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), req.Query)
				return nil
			}

			result, err := adapter.Execute(ctx, operation, callArgs)
			if err != nil {
				return err
			}
			return printGraphQLResult(cmd, result, output)
		},
	}

	cmd.Flags().StringVar(&schemaURI, FlagGraphQLSchema, "", "Schema source: SDL file, recorded introspection JSON, http(s) document URL, or graphql://host:port/path for live introspection (required)")
	cmd.Flags().StringVar(&endpoint, FlagGraphQLEndpoint, "", "Invocation endpoint (default: the endpoint the schema source named)")
	cmd.Flags().StringVar(&operation, FlagGraphQLOperation, "", "Operation (field) to invoke; omit to list discovered operations")
	cmd.Flags().StringVar(&argsJSON, FlagGraphQLArgs, "{}", "Operation arguments as a JSON object, bound as GraphQL variables")
	cmd.Flags().StringVar(&fieldsCSV, FlagGraphQLFields, "", "Comma-separated top-level selection fields (default: every selectable field)")
	cmd.Flags().BoolVar(&document, FlagGraphQLDocument, false, "Print the query document the operation builds instead of calling the endpoint")
	cmd.Flags().IntVar(&maxDepth, FlagGraphQLMaxDepth, graphqladapter.DefaultSelectionDepth, "Selection recursion depth (object levels)")
	cmd.Flags().IntVar(&maxFields, FlagGraphQLMaxFields, graphqladapter.DefaultSelectionFields, "Maximum selected fields per level")
	cmd.Flags().DurationVar(&timeout, FlagGraphQLTimeout, graphqladapter.DefaultTimeout, "Per-call timeout (applies only when the call has no other deadline)")
	cmd.Flags().StringVarP(&output, FlagGraphQLOutput, "o", "json", "Output format (json, raw)")

	return cmd
}

// parseGraphQLArgs decodes the --args JSON object (empty means no arguments).
func parseGraphQLArgs(argsJSON string) (map[string]interface{}, error) {
	if strings.TrimSpace(argsJSON) == "" {
		return nil, nil
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("invalid --args JSON object: %w", err)
	}
	return args, nil
}

// splitCSV splits a comma-separated flag value, dropping empty entries.
func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// graphqlParamView is one argument as the discovery output renders it: the
// neutral type the command/MCP surface uses, plus the GraphQL type it came from.
type graphqlParamView struct {
	Name        string        `json:"name"`
	In          string        `json:"in"`
	Type        string        `json:"type"`
	GraphQLType string        `json:"graphql_type,omitempty"`
	Required    bool          `json:"required"`
	Enum        []interface{} `json:"enum,omitempty"`
	Default     interface{}   `json:"default,omitempty"`
	Description string        `json:"description,omitempty"`
}

// graphqlOpView is one ingested operation as the discovery output renders it.
type graphqlOpView struct {
	ID            string             `json:"id"`
	Kind          string             `json:"kind"`
	OperationType string             `json:"operation_type"`
	Field         string             `json:"field"`
	FieldType     string             `json:"field_type"`
	Summary       string             `json:"summary,omitempty"`
	Description   string             `json:"description,omitempty"`
	Parameters    []graphqlParamView `json:"parameters,omitempty"`
	Source        map[string]string  `json:"source,omitempty"`
}

// printGraphQLOperations writes the discovered operations as JSON. The source
// stamp is included so the provenance of every operation (which field, which
// root type, which schema document, which endpoint) is visible.
func printGraphQLOperations(w io.Writer, ops []*protocol.Operation) error {
	views := make([]graphqlOpView, 0, len(ops))
	for _, op := range ops {
		view := graphqlOpView{
			ID:            op.ID,
			Kind:          graphqlKindLabel(op.Kind),
			OperationType: op.Source[graphqladapter.SourceOperationType],
			Field:         graphqladapter.FieldName(op),
			FieldType:     op.Source[graphqladapter.SourceFieldType],
			Summary:       op.Summary,
			Description:   op.Description,
			Source:        op.Source,
		}
		graphqlTypes := parseArgTypes(op.Source[graphqladapter.SourceArgTypes])
		for _, param := range op.Parameters {
			view.Parameters = append(view.Parameters, graphqlParamView{
				Name:        param.Name,
				In:          param.In,
				Type:        param.Type,
				GraphQLType: graphqlTypes[param.Name],
				Required:    param.Required,
				Enum:        param.Enum,
				Default:     param.Default,
				Description: param.Description,
			})
		}
		views = append(views, view)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(views)
}

// parseArgTypes decodes the operation's arg_types source stamp
// ("id:ID!,filter:UserFilter") into a name → GraphQL type map.
func parseArgTypes(stamp string) map[string]string {
	if strings.TrimSpace(stamp) == "" {
		return nil
	}
	out := make(map[string]string)
	for _, pair := range strings.Split(stamp, ",") {
		name, typeName, found := strings.Cut(pair, ":")
		if !found {
			continue
		}
		out[strings.TrimSpace(name)] = strings.TrimSpace(typeName)
	}
	return out
}

// graphqlKindLabel renders an OperationKind for the discovery listing.
func graphqlKindLabel(kind protocol.OperationKind) string {
	switch kind {
	case protocol.KindUnary:
		return "unary"
	case protocol.KindServerStream:
		return "server-stream"
	case protocol.KindClientStream:
		return "client-stream"
	case protocol.KindBidiStream:
		return "bidi-stream"
	case protocol.KindEventSource:
		return "event-source"
	default:
		return "unknown"
	}
}

// printGraphQLResult renders the executor's JSON-serializable payload. `json`
// pretty-prints; `raw` prints compact JSON (§4.4: rendering lives in the output
// layer; the adapter never writes to stdout itself).
func printGraphQLResult(cmd *cobra.Command, result interface{}, format string) error {
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
