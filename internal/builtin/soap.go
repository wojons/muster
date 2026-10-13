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
	soapadapter "github.com/wojons/muster/pkg/protocol/soap"
)

// Protocol command namespace (specs/PROTOCOLS.md §5.1): the SOAP group
// (PROTO-008) follows the GraphQL group's shape: one command whose flags
// select discovery or one invocation.
//
//	openapi-cli protocol soap --wsdl service.wsdl
//	openapi-cli protocol soap --wsdl service.wsdl --operation EchoService-Echo \
//	  --args '{"message":"hi","tags":["a","b"]}'
//
// OUT OF SCOPE (see the implementation spec §6): WS-Security and MTOM/attachments.
const (
	// FlagSoapWSDL is the discovery source: a WSDL 1.1 or 2.0 document (local
	// file or http(s) URL).
	FlagSoapWSDL = "wsdl"

	// FlagSoapEndpoint overrides the endpoint (default: the wsdl:service
	// address the document names).
	FlagSoapEndpoint = "endpoint"

	// FlagSoapOperation is the operation to invoke; omitted means list.
	FlagSoapOperation = "operation"

	// FlagSoapArgs is the operation's arguments as a JSON object. Repeated
	// fields take JSON arrays; "_output":"xml" inside the object selects raw
	// XML response rendering.
	FlagSoapArgs = "args"

	// FlagSoapVersion forces the SOAP envelope version (default: from the WSDL).
	FlagSoapVersion = "soap-version"

	// FlagSoapAddressing enables the WS-Addressing 1.0 header block.
	FlagSoapAddressing = "addressing"

	// FlagSoapDocument prints the request envelope instead of calling.
	FlagSoapDocument = "document"

	// FlagSoapTimeout bounds a call whose caller's context carries no deadline.
	FlagSoapTimeout = "timeout"

	// FlagSoapOutput selects json (pretty) or raw (compact) rendering.
	FlagSoapOutput = "output"
)

// newProtocolSOAPCommand builds `protocol soap` (PROTO-008).
func newProtocolSOAPCommand() *cobra.Command {
	var (
		wsdl       string
		endpoint   string
		operation  string
		argsJSON   string
		version    string
		addressing bool
		document   bool
		timeout    time.Duration
		output     string
	)

	cmd := &cobra.Command{
		Use:   "soap",
		Short: "SOAP 1.1/1.2 adapter: ingest a WSDL, invoke one operation over an HTTP POST envelope",
		Long: `Ingest a WSDL and call its operations as typed commands.

Schema ingestion (--wsdl):
  service.wsdl                     a WSDL 1.1 <definitions> document (local file)
  http(s)://host/service.wsdl      a WSDL served over HTTP
  WSDL 2.0 <description> documents are also ingested (SOAP 1.2 binding).

Listing (--wsdl without --operation): prints every ingested operation as JSON,
including the typed message parts (required / nillable / repeated) and the
source stamp (endpoint, SOAPAction, SOAP version).

Calling (--wsdl with --operation): builds the version-appropriate SOAP
envelope — SOAP 1.1 posts text/xml with a SOAPAction header; SOAP 1.2 posts
application/soap+xml with an action content-type parameter — and sends ONE
POST to --endpoint (default: the wsdl:service address).

Response rendering is JSON by default (the Body's payload element); pass
"_output":"xml" inside --args for the raw response XML.

Header shape:
  --addressing       adds the WS-Addressing 1.0 header block (wsa:Action,
                     wsa:To, wsa:MessageID)
  --soap-version     force 1.1 or 1.2 (default: the WSDL's binding version)

SOAP faults map onto error codes with the fault code and detail preserved:
Client/Sender -> BAD_REQUEST, Server/Receiver -> INTERNAL_ERROR.

OUT OF SCOPE: WS-Security (UsernameToken, X.509 signing, WS-SecurityPolicy)
and MTOM/XOP attachments are NOT implemented — a WSDL that requires them will
ingest, but the call will be refused by the service; the limitation is
documented, never silently ignored.

Spec: specs/implementation/PROTO-008-soap-wsdl.md`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			if strings.TrimSpace(wsdl) == "" {
				return fmt.Errorf("--wsdl is required: pass a WSDL 1.1 or 2.0 document (local file or http(s) URL)")
			}

			callArgs, err := parseGraphQLArgs(argsJSON) // same JSON-object decode
			if err != nil {
				return err
			}

			// One HTTP client per invocation (§3.1): discovery and execution
			// share it.
			httpClient := client.NewClient()
			opts := []soapadapter.Option{
				soapadapter.WithHTTPClient(httpClient),
				soapadapter.WithTimeout(timeout),
			}
			if strings.TrimSpace(version) != "" {
				v := soapadapter.SOAPVersion(version)
				if v != soapadapter.SOAP11 && v != soapadapter.SOAP12 {
					return fmt.Errorf("invalid --soap-version %q: supported versions are 1.1, 1.2", version)
				}
				opts = append(opts, soapadapter.WithVersion(v))
			}
			if addressing {
				opts = append(opts, soapadapter.WithAddressing(true))
			}
			adapter := soapadapter.NewClient(endpoint, opts...)

			ops, err := adapter.Discover(ctx, wsdl)
			if err != nil {
				return err
			}

			if strings.TrimSpace(operation) == "" {
				return printSOAPOperations(cmd.OutOrStdout(), ops)
			}

			var target *protocol.Operation
			for _, op := range ops {
				if op.ID == operation {
					target = op
					break
				}
			}
			if target == nil {
				return fmt.Errorf("operation %q not found in %s (discovered: %s)",
					operation, wsdl, strings.Join(operationIDs(ops), ", "))
			}

			if document {
				env, err := adapter.BuildEnvelope(target, callArgs)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Content-Type: %s\n", env.ContentType)
				if env.SOAPAction != "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "SOAPAction: %s\n", env.SOAPAction)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(env.XML))
				return nil
			}

			// Endpoint resolution is the adapter's: Execute prefers the
			// endpoint this client was constructed with (--endpoint) and
			// falls back to the wsdl:service address (pkg/protocol/soap
			// client.go). Resolving it here again would be dead code.
			result, err := adapter.Execute(ctx, operation, callArgs)
			if err != nil {
				return err
			}
			return printGraphQLResult(cmd, result, output) // shared JSON/raw renderer
		},
	}

	cmd.Flags().StringVar(&wsdl, FlagSoapWSDL, "", "WSDL 1.1 or 2.0 document (file path or http(s) URL; required)")
	cmd.Flags().StringVar(&endpoint, FlagSoapEndpoint, "", "Invocation endpoint (default: the wsdl:service address)")
	cmd.Flags().StringVar(&operation, FlagSoapOperation, "", "Operation (Service-Operation form) to invoke; omit to list discovered operations")
	cmd.Flags().StringVar(&argsJSON, FlagSoapArgs, "{}", "Operation arguments as a JSON object (repeated fields take arrays; \"_output\":\"xml\" selects raw XML)")
	cmd.Flags().StringVar(&version, FlagSoapVersion, "", "Force the SOAP envelope version: 1.1 or 1.2 (default: the WSDL binding's)")
	cmd.Flags().BoolVar(&addressing, FlagSoapAddressing, false, "Add the WS-Addressing 1.0 header block (wsa:Action, wsa:To, wsa:MessageID)")
	cmd.Flags().BoolVar(&document, FlagSoapDocument, false, "Print the request envelope (and its headers) instead of calling the endpoint")
	cmd.Flags().DurationVar(&timeout, FlagSoapTimeout, soapadapter.DefaultTimeout, "Per-call timeout (applies only when the call has no other deadline)")
	cmd.Flags().StringVarP(&output, FlagSoapOutput, "o", "json", "Output format (json, raw)")

	return cmd
}

// printSOAPOperations writes the discovered operations as JSON, including the
// typed part facts and the source stamp.
func printSOAPOperations(w io.Writer, ops []*protocol.Operation) error {
	type partView struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Required bool   `json:"required"`
		Repeated bool   `json:"repeated"`
		Nillable bool   `json:"nillable"`
	}
	type opView struct {
		ID          string            `json:"id"`
		Kind        string            `json:"kind"`
		SOAPVersion string            `json:"soap_version"`
		SOAPAction  string            `json:"soap_action,omitempty"`
		Style       string            `json:"style"`
		Endpoint    string            `json:"endpoint,omitempty"`
		Summary     string            `json:"summary,omitempty"`
		Parts       []partView        `json:"parts,omitempty"`
		Source      map[string]string `json:"source,omitempty"`
	}

	views := make([]opView, 0, len(ops))
	for _, op := range ops {
		view := opView{
			ID:          op.ID,
			Kind:        "unary",
			SOAPVersion: op.Source[soapadapter.SourceSOAPVersion],
			SOAPAction:  op.Source[soapadapter.SourceSOAPAction],
			Style:       op.Source[soapadapter.SourceStyle],
			Endpoint:    op.Source[soapadapter.SourceEndpoint],
			Summary:     op.Summary,
			Source:      op.Source,
		}
		for _, p := range op.RequestBody {
			view.Parts = append(view.Parts, partView{
				Name:     p.Name,
				Type:     p.Type,
				Required: p.Required,
				Repeated: p.Repeated,
				Nillable: strings.Contains(p.Description, "nillable"),
			})
		}
		views = append(views, view)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(views)
}
