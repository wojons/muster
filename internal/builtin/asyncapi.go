package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wojons/muster/pkg/auth"
	"github.com/wojons/muster/pkg/protocol"
	asyncapiadapter "github.com/wojons/muster/pkg/protocol/asyncapi"
)

// ── protocol asyncapi (PROTO-021) ────────────────────────────────────────
// AsyncAPI 3.x adapter commands: one transport end-to-end (Kafka via
// github.com/segmentio/kafka-go), per the PROTO-007 spec gate. The
// operation list comes from `protocol asyncapi list`; send operations are
// one bounded publish (`publish`); receive operations are a bounded
// collect (`consume`) per specs/PROTOCOLS.md §4.5(4) — subscribe, collect
// up to the bounds, close, return. There is no unbounded mode.

// exitCoder carries the consume ending's distinct exit code to
// cmd/openapi-cli/main.go: drained (complete) = 0, timed out (max-duration
// bound) = 3, max-messages cap = 4; errors and cancellations keep the
// framework's exit 1.
type exitCoder struct {
	code int
	err  error
}

func (e *exitCoder) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *exitCoder) ExitCode() int { return e.code }

func (e *exitCoder) Unwrap() error { return e.err }

// consume exit codes (documented on the command and in README).
const (
	exitCodeDrained     = 0 // status complete: the topic window was drained
	exitCodeTimedOut    = 3 // truncated by the max-duration bound
	exitCodeMaxItemsCap = 4 // truncated by the max-messages bound
	exitCodeError       = 1 // broker error, cancellation, usage (cobra default)
)

// Flags of the asyncapi command group.
const (
	FlagAsyncAPIDoc          = "doc"
	FlagAsyncAPIOp           = "operation"
	FlagAsyncAPIArgs         = "args"
	FlagAsyncAPIBrokers      = "brokers"
	FlagAsyncAPIServer       = "server"
	FlagAsyncAPISMechanism   = "sasl-mechanism"
	FlagAsyncAPISASLUser     = "sasl-username"
	FlagAsyncAPISASLPassword = "sasl-password"
	FlagAsyncAPICred         = "auth"
	FlagAsyncAPITLS          = "tls"
	FlagAsyncAPITimeout      = "timeout"
	FlagAsyncAPIMaxMessages  = "max-messages"
)

// newProtocolAsyncAPICommand builds the `protocol asyncapi` command group.
func newProtocolAsyncAPICommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "asyncapi",
		Short: "AsyncAPI 3.x adapter: ingest documents, publish and bounded-collect over Kafka",
		Long: `AsyncAPI 3.x adapter (PROTO-021, implementing the PROTO-007 spec gate).

Operations are discovered from an AsyncAPI 3.x document (JSON or YAML, a
local file path or an http(s) URL). Operation action "send" becomes a one
bounded publish ("send" → unary); operation action "receive" becomes a
bounded collect ("receive" → event-source): subscribe, collect up to the
bounds, close the subscription, return the transcript — per
specs/PROTOCOLS.md §4.5(4) there is no long-lived consume mode.

Transports: operation action "send" produces ONE message to the channel
address (the Kafka topic, with {parameter} expressions resolved from the
channel arguments); the reserved "_key" argument carries the Kafka message
key. Receiver operations run offline-draining collects: every message
retained when the collect began is delivered, then the collect returns
complete; the max-messages and max-duration bounds truncate cleanly.

Auth (mapped into the Kafka dialer per PROTO-007 §5, never serialized into
operations, logs or transcripts):
  --sasl-mechanism plain|scram-sha256|scram-sha512 with --sasl-username and
  --sasl-password, or --auth <name> to take the password from the shared
  keychain credential store (pkg/auth). --tls enables TLS.

Consumer groups, offset commits and durable subscriptions are deliberately
NOT executed: a receive operation whose Kafka binding declares groupId is
refused with a validation error naming the unsupported field.`,
	}

	cmd.AddCommand(newProtocolAsyncAPIListCommand())
	cmd.AddCommand(newProtocolAsyncAPIPublishCommand())
	cmd.AddCommand(newProtocolAsyncAPIConsumeCommand())
	// A capped or timed-out consume is a legitimate ending, not a usage
	// mistake: never print help for a RunE error under this group.
	cmd.SilenceUsage = true
	for _, sub := range cmd.Commands() {
		sub.SilenceUsage = true
	}
	return cmd
}

// asyncapiInvocation gathers the shared discovery/auth flags.
type asyncapiInvocation struct {
	doc         string
	operation   string
	server      string
	brokers     []string
	argsJSON    string
	saslMech    string
	saslUser    string
	saslPass    string
	credName    string
	useTLS      bool
	timeout     time.Duration
	maxMessages int
}

// addDiscoveryFlags registers the flags shared by every subcommand. The
// --args flag is deliberately NOT here: publish and consume register it
// (list does not need it), and cobra panics on double registration.
func addDiscoveryFlags(cmd *cobra.Command, inv *asyncapiInvocation) {
	cmd.Flags().StringVar(&inv.doc, FlagAsyncAPIDoc, "", "AsyncAPI 3.x document (file path or http(s) URL; required)")
	cmd.Flags().StringVar(&inv.operation, FlagAsyncAPIOp, "", "Operation to invoke; omit to list discovered operations")
	cmd.Flags().StringVar(&inv.server, FlagAsyncAPIServer, "", "AsyncAPI server to select when the document declares several")
	cmd.Flags().StringSliceVar(&inv.brokers, FlagAsyncAPIBrokers, nil, "Kafka broker addresses (default: the selected server's host)")
	cmd.Flags().StringVar(&inv.saslMech, FlagAsyncAPISMechanism, "", "SASL mechanism: plain, scram-sha256, scram-sha512")
	cmd.Flags().StringVar(&inv.saslUser, FlagAsyncAPISASLUser, "", "SASL username")
	cmd.Flags().StringVar(&inv.saslPass, FlagAsyncAPISASLPassword, "", "SASL password (prefer --auth to read it from the keychain)")
	cmd.Flags().StringVar(&inv.credName, FlagAsyncAPICred, "", "Keychain credential name whose secret is used as the SASL password (shared pkg/auth store)")
	cmd.Flags().BoolVar(&inv.useTLS, FlagAsyncAPITLS, false, "Use TLS for the broker connections")
}

// buildAdapter constructs the adapter client for one CLI invocation.
func buildAdapter(inv *asyncapiInvocation, limits asyncapiadapter.EventLimits, extra ...asyncapiadapter.Option) (*asyncapiadapter.Client, error) {
	mech := strings.ToLower(strings.TrimSpace(inv.saslMech))
	switch mech {
	case "", asyncapiadapter.SASLMechanismPlain,
		asyncapiadapter.SASLMechanismScramSHA256, asyncapiadapter.SASLMechanismScramSHA512:
	default:
		return nil, fmt.Errorf("invalid --sasl-mechanism %q: supported mechanisms are plain, scram-sha256, scram-sha512", inv.saslMech)
	}

	password := inv.saslPass
	if inv.credName != "" {
		// Shared credential material comes from the keychain (pkg/auth);
		// the flag path stays for non-interactive environments.
		kc, err := auth.NewKeychain()
		if err != nil {
			return nil, fmt.Errorf("open keychain: %w", err)
		}
		secret, err := kc.Get(authServiceName, inv.credName)
		if err != nil || strings.TrimSpace(secret) == "" {
			return nil, fmt.Errorf("no stored credential %q (auth add --name %s --value <secret>)", inv.credName, inv.credName)
		}
		password = secret
	}

	opts := []asyncapiadapter.Option{
		asyncapiadapter.WithBrokers(inv.brokers),
		asyncapiadapter.WithServer(inv.server),
		asyncapiadapter.WithStreamLimits(limits),
	}
	if mech != "" {
		opts = append(opts, asyncapiadapter.WithSASL(mech, inv.saslUser, password))
	}
	if inv.useTLS {
		cfg, err := asyncapiadapter.DefaultTLSConfig()
		if err != nil {
			return nil, fmt.Errorf("build TLS config: %w", err)
		}
		opts = append(opts, asyncapiadapter.WithTLS(cfg))
	}
	if inv.timeout > 0 && limits.MaxDuration == 0 {
		// Publish/list calls: the timeout bounds calls without a caller
		// deadline. Consume already carries it as the collect bound.
		opts = append(opts, asyncapiadapter.WithTimeout(inv.timeout))
	}
	opts = append(opts, extra...)
	return asyncapiadapter.NewClient(opts...), nil
}

// parseAsyncAPIArgs decodes the --args JSON object.
func parseAsyncAPIArgs(argsJSON string) (map[string]interface{}, error) {
	if strings.TrimSpace(argsJSON) == "" || argsJSON == "{}" {
		return nil, nil
	}
	args := map[string]interface{}{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("invalid --args JSON object: %w", err)
	}
	return args, nil
}

// discoverAsyncAPIOps runs discovery and returns the operations.
func discoverAsyncAPIOps(ctx context.Context, adapter *asyncapiadapter.Client, doc string) ([]*protocol.Operation, error) {
	if strings.TrimSpace(doc) == "" {
		return nil, fmt.Errorf("--doc is required: pass an AsyncAPI 3.x document (local file or http(s) URL)")
	}
	return adapter.Discover(ctx, doc)
}

// requireAsyncAPIOp resolves one operation by ID.
func requireAsyncAPIOp(ops []*protocol.Operation, id string) (*protocol.Operation, error) {
	for _, op := range ops {
		if op.ID == id {
			return op, nil
		}
	}
	return nil, fmt.Errorf("operation %q not found in the document (discovered: %s)",
		id, strings.Join(operationIDs(ops), ", "))
}

// newProtocolAsyncAPIListCommand lists the document's operations.
func newProtocolAsyncAPIListCommand() *cobra.Command {
	var inv asyncapiInvocation
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the operations an AsyncAPI 3.x document exposes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			adapter, err := buildAdapter(&inv, asyncapiadapter.EventLimits{})
			if err != nil {
				return err
			}
			ops, err := discoverAsyncAPIOps(cmd.Context(), adapter, inv.doc)
			if err != nil {
				return err
			}
			return printAsyncAPIOperations(cmd.OutOrStdout(), ops)
		},
	}
	addDiscoveryFlags(cmd, &inv)
	return cmd
}

// newProtocolAsyncAPIPublishCommand runs one send operation as a single
// bounded Kafka publish.
func newProtocolAsyncAPIPublishCommand() *cobra.Command {
	var inv asyncapiInvocation
	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish one message for a send operation over Kafka",
		Long: `Publish ONE message for a "send" operation (PROTO-007 decision D2):

  openapi-cli protocol asyncapi publish --doc service.yaml \
    --operation sendUserSignedUp_v1 --args '{"userId":"u1","email":"u@x","_key":"u1"}'

The topic is the operation's channel address with {parameter} expressions
resolved from the --args channel parameters. The result is the broker
acknowledgement as JSON (topic, partition, offset).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			adapter, err := buildAdapter(&inv, asyncapiadapter.EventLimits{})
			if err != nil {
				return err
			}
			ops, err := discoverAsyncAPIOps(cmd.Context(), adapter, inv.doc)
			if err != nil {
				return err
			}
			op, err := requireAsyncAPIOp(ops, strings.TrimSpace(inv.operation))
			if err != nil {
				return err
			}
			args, err := parseAsyncAPIArgs(inv.argsJSON)
			if err != nil {
				return err
			}
			result, err := adapter.Execute(asyncapiContext(cmd), op.ID, args)
			if err != nil {
				return err
			}
			return printAsyncAPIResult(cmd.OutOrStdout(), result)
		},
	}
	addDiscoveryFlags(cmd, &inv)
	cmd.Flags().StringVar(&inv.argsJSON, FlagAsyncAPIArgs, "{}", "Operation arguments as a JSON object (channel parameters, message headers, payload fields; \"_key\" selects the Kafka message key)")
	return cmd
}

func asyncapiContext(cmd *cobra.Command) context.Context {
	if cmd.Context() != nil {
		return cmd.Context()
	}
	return context.Background()
}

// newProtocolAsyncAPIConsumeCommand runs one receive operation as a
// bounded collect and prints NDJSON (one line per message, then the final
// transcript envelope line). The ending's exit code is distinct: 0
// drained, 3 timed out (max-duration bound), 4 max-messages cap.
func newProtocolAsyncAPIConsumeCommand() *cobra.Command {
	var inv asyncapiInvocation
	cmd := &cobra.Command{
		Use:   "consume",
		Short: "Bounded collect for a receive operation (NDJSON output)",
		Long: `Collect messages for a "receive" operation with bounded-collect
semantics (specs/PROTOCOLS.md §4.5(4)): subscribe, collect up to
--max-messages within --timeout, close every subscription, return.

Output is NDJSON: one JSON object per received message (topic, partition,
offset, key, headers, payload), then a final line carrying the full
transcript envelope (kind, status, items_truncated, trailer).

Exit codes: 0 = drained (the collect reached the messages retained when it
began); 3 = timed out (the max-duration bound fired); 4 = max-messages cap
fired; 1 = broker error, cancellation or usage failure. The transcript is
printed on every ending.

Bounds default to 1000 messages / 30 seconds. There is deliberately no
unbounded mode.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			adapter, err := buildAdapter(&inv, asyncapiadapter.EventLimits{
				MaxMessages: inv.maxMessages,
				MaxDuration: inv.timeout,
			})
			if err != nil {
				return err
			}
			ops, err := discoverAsyncAPIOps(cmd.Context(), adapter, inv.doc)
			if err != nil {
				return err
			}
			op, err := requireAsyncAPIOp(ops, strings.TrimSpace(inv.operation))
			if err != nil {
				return err
			}
			args, err := parseAsyncAPIArgs(inv.argsJSON)
			if err != nil {
				return err
			}

			result, err := adapter.Execute(asyncapiContext(cmd), op.ID, args)
			tr, _ := result.(*asyncapiadapter.EventTranscript)
			if tr != nil {
				// The transcript is printed on EVERY ending — a truncated
				// or cancelled collect still delivers the partial items.
				_ = printAsyncAPITranscriptNDJSON(cmd.OutOrStdout(), tr)
			}
			if err != nil {
				return err // broker error / cancellation → exit 1
			}
			if tr == nil {
				return fmt.Errorf("consume returned no transcript envelope")
			}
			switch tr.Status {
			case asyncapiadapter.StatusComplete:
				return nil // exit 0: drained
			case asyncapiadapter.StatusTruncated:
				code := codeForLimit(tr)
				return &exitCoder{
					code: code,
					err: fmt.Errorf("consume ended truncated (%s): drained the %s bound — the transcript above carries the partial items",
						tr.Trailer["limit_reason"], limitName(code)),
				}
			default:
				return &exitCoder{code: exitCodeError, err: fmt.Errorf("consume ended with status %q", tr.Status)}
			}
		},
	}
	addDiscoveryFlags(cmd, &inv)
	cmd.Flags().StringVar(&inv.argsJSON, FlagAsyncAPIArgs, "{}", "Operation arguments as a JSON object (channel parameters that resolve the topic)")
	cmd.Flags().DurationVar(&inv.timeout, FlagAsyncAPITimeout, asyncapiadapter.DefaultMaxDuration, "Wall-clock bound of the collect (default 30s)")
	cmd.Flags().IntVar(&inv.maxMessages, FlagAsyncAPIMaxMessages, asyncapiadapter.DefaultMaxMessages, "Maximum messages to collect (default 1000)")
	return cmd
}

// limitName names the bound for the exit-code explanation.
func limitName(code int) string {
	if code == exitCodeMaxItemsCap {
		return asyncapiadapter.LimitReasonMessages
	}
	return asyncapiadapter.LimitReasonDuration
}

// codeForLimit maps the transcript's limit_reason onto the consume exit
// code (timed out vs capped).
func codeForLimit(tr *asyncapiadapter.EventTranscript) int {
	if tr.Trailer["limit_reason"] == asyncapiadapter.LimitReasonMessages {
		return exitCodeMaxItemsCap
	}
	return exitCodeTimedOut
}

// printAsyncAPIOperations writes the discovered operations as JSON.
func printAsyncAPIOperations(w io.Writer, ops []*protocol.Operation) error {
	type opView struct {
		ID      string            `json:"id"`
		Kind    string            `json:"kind"`
		Summary string            `json:"summary,omitempty"`
		Topic   string            `json:"topic,omitempty"`
		Source  map[string]string `json:"source,omitempty"`
	}
	views := make([]opView, 0, len(ops))
	for _, op := range ops {
		view := opView{
			ID:      op.ID,
			Summary: op.Summary,
			Topic:   op.Source[asyncapiadapter.SourceChannelAddress],
			Source:  op.Source,
		}
		switch op.Kind {
		case protocol.KindUnary:
			view.Kind = "unary"
		case protocol.KindEventSource:
			view.Kind = "event-source"
		}
		views = append(views, view)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(views)
}

// printAsyncAPIResult renders a publish ack as JSON.
func printAsyncAPIResult(w io.Writer, result interface{}) error {
	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("render result: %w", err)
	}
	_, _ = fmt.Fprintln(w, string(out))
	return nil
}

// printAsyncAPITranscriptNDJSON renders the bounded-collect transcript as
// NDJSON: one line per received item, then the final envelope line.
func printAsyncAPITranscriptNDJSON(w io.Writer, tr *asyncapiadapter.EventTranscript) error {
	enc := json.NewEncoder(w)
	for _, item := range tr.Items {
		if err := enc.Encode(item); err != nil {
			return err
		}
	}
	// The final line is the complete envelope (status authority for the
	// caller); items repeat here so the last line is self-contained.
	return enc.Encode(tr)
}
