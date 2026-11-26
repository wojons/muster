package cmdstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/wojons/muster/pkg/auth"
	"github.com/wojons/muster/pkg/log"
	"github.com/wojons/muster/pkg/request"
	"gopkg.in/yaml.v3"
)

// AuthFlagName is the runtime --auth credential-attachment flag on
// reloaded commands (DF-023). Mirrors generator.AuthFlagName (pkg/generator
// cannot be imported here without an import cycle): a parity test in
// internal/builtin (which imports both packages) pins the two values equal.
const AuthFlagName = "auth"

// RawBodyFlagName is the raw request-body flag on persisted commands.
// Mirror of generator.RawBodyFlagName (pkg/generator cannot be imported
// here without an import cycle); used for messaging when persisted
// metadata carries no raw-body flag (QA-MUSTER-036).
const RawBodyFlagName = "body"

// resolveStoredFn resolves a stored credential by name for the --auth
// flag. Package-level var (default auth.ResolveStored) so tests can seam
// it without touching the platform keychain; production paths always use
// the default.
var resolveStoredFn = auth.ResolveStored

// AuthRuntimeAnnotation is stamped on reloaded commands where the loader
// itself registered the runtime --auth flag (i.e. the spec had no
// parameter of that name). The run handler consumes --auth only when this
// annotation is present, so a spec's own "auth" parameter is never
// diverted into the credential resolver.
const AuthRuntimeAnnotation = "muster/auth-runtime-flag"

// Loader reconstructs commands from stored metadata
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/cmdstore/loader.go
type Loader struct {
	store   *CommandStore
	baseURL string
	headers []string // user-supplied headers from --header flag
}

// NewLoader creates a new command loader
func NewLoader(store *CommandStore, baseURL string) *Loader {
	return &Loader{
		store:   store,
		baseURL: baseURL,
	}
}

// LoadCommands loads and reconstructs all commands from storage
func (l *Loader) LoadCommands() ([]*cobra.Command, error) {
	metadata, err := l.store.Commands()
	if err != nil {
		return nil, fmt.Errorf("load metadata: %w", err)
	}

	if len(metadata) == 0 {
		return []*cobra.Command{}, nil
	}

	var commands []*cobra.Command
	for _, meta := range metadata {
		cmd, err := l.reconstructCommand(meta)
		if err != nil {
			return nil, fmt.Errorf("reconstruct command %s: %w", meta.Name, err)
		}
		commands = append(commands, cmd)
	}

	return commands, nil
}

// GetBaseURL returns the base URL from stored metadata
func (l *Loader) GetBaseURL() (string, error) {
	meta, err := l.store.Load()
	if err != nil {
		return "", err
	}
	if meta == nil {
		return "", fmt.Errorf("no stored data")
	}

	if meta.BaseURL != "" {
		return meta.BaseURL, nil
	}
	return l.baseURL, nil
}

// SetHeaders stores user-supplied headers for all loaded commands.
func (l *Loader) SetHeaders(headers []string) {
	l.headers = headers
}

// reconstructCommand creates a cobra.Command from metadata
func (l *Loader) reconstructCommand(meta CommandMetadata) (*cobra.Command, error) {
	// Determine base URL for this command
	baseURL := l.baseURL
	if meta.BaseURL != "" {
		baseURL = meta.BaseURL
	}

	// Create command
	// DF-019: SilenceUsage keeps a runtime/HTTP error (e.g. a 404 from the
	// server) from dumping the full Usage/Examples/Flags block. Flag-parse
	// errors still show usage via the custom FlagErrorFunc below, which
	// un-silences usage for that error class — same pattern as
	// internal/builtin/request.go.
	cmd := &cobra.Command{
		Use:          meta.Use,
		Short:        meta.Short,
		Long:         meta.Long,
		Example:      meta.Example,
		SilenceUsage: true,
	}

	// Flag-parse errors (unknown flag, bad value) should still show usage.
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		c.SilenceUsage = false
		return err
	})

	// Arg-count errors (wrong number of positional args) should still show
	// usage: validate against the persisted argument count and un-silence on
	// failure. The handler itself also checks (expected %d arguments, got %d)
	// but that path is a runtime error, not an arg-validation error.
	if len(meta.Args) > 0 {
		want := len(meta.Args)
		cmd.Args = func(c *cobra.Command, args []string) error {
			if len(args) != want {
				c.SilenceUsage = false
				return fmt.Errorf("accepts %d arg(s), received %d", want, len(args))
			}
			return nil
		}
	}

	// Add flags. --header is registered FIRST (below the loop used to do it
	// after) so a persisted flag of the same name cannot panic pflag with
	// "flag redefined: header" at CLI startup — a panic there hides every
	// command, not just this one (QA-MUSTER-041).
	cmd.Flags().StringSliceP("header", "H", nil, "Add HTTP header (format: 'Key: Value')")

	for _, flagMeta := range meta.Flags {
		// DF-016: --raw is a presentation flag the loader registers itself
		// (below). Older stores persisted it as regular metadata (the
		// generate-time skip list only omitted output/namespace/debug), so
		// re-registering it here would panic with "flag redefined", and
		// forwarding it as a query-kind flag is exactly the ?raw=true leak.
		if flagMeta.Name == "raw" {
			continue
		}
		// DF-023: the runtime --auth credential-attachment flag is
		// re-registered by the loader itself (below) and consumed in the
		// run handler — never re-registered from metadata, never forwarded
		// as a query parameter. A spec parameter genuinely named "auth"
		// persists WITHOUT FlagKindAuth (the generator only stamps the
		// annotation when it registered the flag) and is unaffected.
		if flagMeta.Kind == FlagKindAuth {
			continue
		}
		// Defensive: a store written before the generator reserved the CLI's
		// own flag names can still carry one (e.g. a `header` body property).
		// Skipping it loses that one value; re-registering it would panic the
		// whole CLI, which is strictly worse.
		if cmd.Flags().Lookup(flagMeta.Name) != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping persisted flag --%s: the CLI already defines it\n", flagMeta.Name)
			continue
		}
		switch flagMeta.Type {
		case "string":
			defaultVal := flagMeta.Default
			cmd.Flags().StringP(flagMeta.Name, flagMeta.Shorthand, defaultVal, flagMeta.Description)
		case "int":
			defaultVal := 0
			if flagMeta.Default != "" {
				defaultVal, _ = strconv.Atoi(flagMeta.Default)
			}
			cmd.Flags().IntP(flagMeta.Name, flagMeta.Shorthand, defaultVal, flagMeta.Description)
		case "bool":
			defaultVal := false
			if flagMeta.Default == "true" {
				defaultVal = true
			}
			cmd.Flags().BoolP(flagMeta.Name, flagMeta.Shorthand, defaultVal, flagMeta.Description)
		case "array":
			var defaultVal []string
			if flagMeta.Default != "" {
				defaultVal = append(defaultVal, flagMeta.Default)
			}
			cmd.Flags().StringSliceP(flagMeta.Name, flagMeta.Shorthand, defaultVal, flagMeta.Description)
		}

		if flagMeta.Required && flagMeta.Kind != FlagKindBody {
			_ = cmd.MarkFlagRequired(flagMeta.Name)
		}
	}

	// DF-023: runtime credential attachment on reloaded commands. Register
	// the loader's --auth flag ONLY when metadata does not already carry a
	// flag with that name (a spec parameter genuinely named "auth" was
	// registered from metadata above and must reach the server as a normal
	// parameter, never the credential resolver). The annotation tells the
	// run handler this --auth is the runtime credential flag.
	if cmd.Flags().Lookup(AuthFlagName) == nil {
		cmd.Flags().String(AuthFlagName, "", "Attach a stored credential by name (see 'auth add')")
		if cmd.Annotations == nil {
			cmd.Annotations = map[string]string{}
		}
		cmd.Annotations[AuthRuntimeAnnotation] = "true"
	}

	// Runtime base-URL override: --base-url flag or MUSTER_BASE_URL env var
	// takes precedence over the stored/generate-time URL, so a generated CLI
	// can be pointed at staging/prod without regenerating.
	cmd.Flags().StringP("base-url", "", "", "Override the API base URL at runtime (env: MUSTER_BASE_URL)")

	// Add output flag
	cmd.Flags().StringP("output", "o", "table", "Output format (json, yaml, table)")

	// DF-016: register --raw so pflag consumes it locally instead of
	// forwarding it as a `?raw=true` query parameter, matching the
	// generated path (pkg/generator).
	cmd.Flags().BoolP("raw", "r", false, "Output raw API response")

	// Set up the run handler to execute the HTTP request
	cmd.RunE = l.createRunHandler(meta, baseURL)

	return cmd, nil
}

// createRunHandler creates a handler that makes HTTP requests.
// Supports: multipart file upload, --header flag, MUSTER_AUTH_TOKEN env var,
// and pagination (page/limit query params only when explicitly set).
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/cmdstore/loader.go
func (l *Loader) createRunHandler(meta CommandMetadata, baseURL string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Runtime override: explicit --base-url flag wins, then the
		// MUSTER_BASE_URL env var, then the stored/generate-time URL.
		if v, _ := cmd.Flags().GetString("base-url"); v != "" {
			baseURL = v
		} else if env := os.Getenv("MUSTER_BASE_URL"); env != "" {
			baseURL = env
		}

		// DF-015: fail with the actionable --base-url / MUSTER_BASE_URL hint
		// when the persisted base URL is still empty or relative (e.g. an
		// older store generated from a local-file spec declaring a relative
		// servers[].url) instead of the bare `unsupported protocol scheme ""`
		// http error.
		if err := request.ValidateRequestBaseURL(baseURL); err != nil {
			return fmt.Errorf("cannot execute %s %s: %w", meta.Method, meta.Path, err)
		}

		// Build HTTP request
		builder := request.NewBuilder(baseURL, meta.Path, meta.Method)

		// Add path parameters from args
		if len(args) != len(meta.Args) {
			return fmt.Errorf("expected %d arguments, got %d", len(meta.Args), len(args))
		}
		for i, argName := range meta.Args {
			if i < len(args) {
				builder.WithPathParam(argName, args[i])
			}
		}

		// Apply global loader headers (from --header flag on generate/root command)
		for _, h := range l.headers {
			parts := strings.SplitN(h, ":", 2)
			if len(parts) == 2 {
				builder.SetHeader(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
			}
		}

		// Apply MUSTER_AUTH_TOKEN env var as Bearer auth if set
		if authToken := os.Getenv("MUSTER_AUTH_TOKEN"); authToken != "" {
			builder.SetHeader("Authorization", "Bearer "+authToken)
		}

		// DF-023: attach the stored credential named by --auth BEFORE
		// per-command --header flags so an explicit manual header with the
		// same name keeps precedence (SetHeader overwrites). Only the
		// loader-registered runtime flag (annotation) is consumed here; a
		// spec's own "auth" parameter is forwarded as a query parameter.
		if cmd.Flag(AuthFlagName) != nil && cmd.Annotations[AuthRuntimeAnnotation] == "true" {
			authName, _ := cmd.Flags().GetString(AuthFlagName)
			if authName != "" {
				cred, err := resolveStoredFn(authName)
				if err != nil {
					return fmt.Errorf("resolve --auth: %w", err)
				}
				builder.SetHeader(cred.Name, cred.Value)
			}
		}

		// Apply per-command --header flags
		if cmd.Flag("header") != nil {
			hdrValues, _ := cmd.Flags().GetStringSlice("header")
			for _, h := range hdrValues {
				parts := strings.SplitN(h, ":", 2)
				if len(parts) == 2 {
					builder.SetHeader(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
				}
			}
		}

		// Separate flags into file, body, header, and query groups
		var bodyFlags []FlagMetadata
		var queryFlags []FlagMetadata
		var rawBodyFlags []FlagMetadata
		var fileFlagPath string

		for _, flagMeta := range meta.Flags {
			// DF-016: --raw is a presentation flag consumed in
			// executeAndReadResponse; never classify it as a query parameter.
			if flagMeta.Name == "raw" {
				continue
			}
			// DF-023: --auth is the runtime credential-attachment flag
			// consumed in the run handler above; never classify it as a
			// query parameter.
			if flagMeta.Kind == FlagKindAuth {
				continue
			}
			switch flagMeta.Kind {
			case FlagKindBody:
				bodyFlags = append(bodyFlags, flagMeta)
			case FlagKindRawBody:
				// raw JSON request body (--body/-b) — handled below
				rawBodyFlags = append(rawBodyFlags, flagMeta)
			case FlagKindHeader:
				// handled below
			case FlagKindFile:
				// file-upload: read the --file flag value
				if cmd.Flag(flagMeta.Name) != nil {
					fp, _ := cmd.Flags().GetString(flagMeta.Name)
					if fp != "" {
						fileFlagPath = fp
					}
				}
			default:
				queryFlags = append(queryFlags, flagMeta)
			}
		}

		// QA-MUSTER-036: enforce required request-body properties BEFORE any
		// network I/O. A bodyless write against an operation whose schema
		// declares required properties must fail locally with the missing flag
		// names; stdout stays untouched. The raw --body payload satisfies every
		// required property at once (DF-MUSTER-5), so the check is skipped
		// when it is set.
		if len(bodyFlags) > 0 &&
			(meta.Method == http.MethodPost || meta.Method == http.MethodPut || meta.Method == http.MethodPatch) {
			rawBodySet := false
			for _, flagMeta := range rawBodyFlags {
				if f := cmd.Flag(flagMeta.Name); f != nil && f.Changed {
					rawBodySet = true
					break
				}
			}
			if !rawBodySet {
				var missing []string
				for _, flagMeta := range bodyFlags {
					if !flagMeta.Required {
						continue
					}
					if f := cmd.Flag(flagMeta.Name); f == nil || !f.Changed {
						missing = append(missing, "--"+flagMeta.Name)
					}
				}
				if len(missing) > 0 {
					rawFlag := RawBodyFlagName
					if len(rawBodyFlags) > 0 {
						rawFlag = rawBodyFlags[0].Name
					}
					return fmt.Errorf("missing required body flag(s): %s (or provide --%s with raw JSON)", strings.Join(missing, ", "), rawFlag)
				}
			}
		}

		// Handle file upload via multipart if a file was provided
		if fileFlagPath != "" {
			builder.WithFileUpload("file", fileFlagPath)
			// Also check for any additional multipart form fields
			for _, flagMeta := range meta.Flags {
				if flagMeta.Kind == FlagKindMultipart {
					if cmd.Flag(flagMeta.Name) != nil && cmd.Flag(flagMeta.Name).Changed {
						val, _ := l.getFlagValue(cmd, flagMeta)
						if val != nil {
							builder.WithMultipartField(flagMeta.Name, fmt.Sprintf("%v", val))
						}
					}
				}
			}
			// Skip query/body — use multipart body
			return executeAndReadResponse(builder, cmd, ctx)
		}

		// QA-MUSTER-041: multipart operation with text fields but no --file.
		// Sending the fields beats the old behavior of falling through to a
		// body path that had nothing to send (Content-Length: 0).
		if meta.ContentType == request.ContentTypeMultipart {
			fieldsSet := false
			builder.WithContentType(request.ContentTypeMultipart)
			for _, flagMeta := range meta.Flags {
				if flagMeta.Kind != FlagKindMultipart {
					continue
				}
				if cmd.Flag(flagMeta.Name) == nil || !cmd.Flag(flagMeta.Name).Changed {
					continue
				}
				val, _ := l.getFlagValue(cmd, flagMeta)
				if val != nil {
					builder.WithMultipartField(flagMeta.Name, fmt.Sprintf("%v", val))
					fieldsSet = true
				}
			}
			if fieldsSet {
				return executeAndReadResponse(builder, cmd, ctx)
			}
		}

		// Add query parameters from query-kind flags — only if explicitly set by user
		for _, flagMeta := range queryFlags {
			if cmd.Flag(flagMeta.Name) == nil || !cmd.Flag(flagMeta.Name).Changed {
				continue
			}
			value, err := l.getFlagValue(cmd, flagMeta)
			if err != nil {
				continue
			}
			if value != nil {
				builder.WithQueryParam(flagMeta.Name, fmt.Sprintf("%v", value))
			}
		}

		// Add header-kind flags
		for _, flagMeta := range meta.Flags {
			if flagMeta.Kind == FlagKindHeader {
				if cmd.Flag(flagMeta.Name) != nil {
					val, _ := cmd.Flags().GetString(flagMeta.Name)
					if val != "" {
						builder.SetHeader(flagMeta.Name, val)
					}
				}
			}
		}

		// Raw body override (DF-MUSTER-5): when a raw-body flag
		// (--body/-b) is set, its validated payload is sent verbatim as the
		// request body and every per-property body flag is ignored. This
		// mirrors pkg/generator.createRunHandler behavior on persisted
		// (reloaded) commands. QA-MUSTER-041: the payload is urlencoded —
		// not JSON — for a form-encoded operation, and carries its own
		// Content-Type.
		for _, flagMeta := range rawBodyFlags {
			if cmd.Flag(flagMeta.Name) == nil || !cmd.Flag(flagMeta.Name).Changed {
				continue
			}
			raw, _ := cmd.Flags().GetString(flagMeta.Name)
			if meta.ContentType == request.ContentTypeFormEncoded {
				if _, err := request.ParseFormBody(raw); err != nil {
					return fmt.Errorf("invalid body for --%s: %w", flagMeta.Name, err)
				}
				builder.RawBody(raw, request.ContentTypeFormEncoded)
				return executeAndReadResponse(builder, cmd, ctx)
			}
			if !json.Valid([]byte(raw)) {
				return fmt.Errorf("invalid JSON for --%s: value is not valid JSON (value: %q)", flagMeta.Name, raw)
			}
			builder.JSONBody(raw)
			builder.SetHeader("Content-Type", "application/json")
			return executeAndReadResponse(builder, cmd, ctx)
		}

		// Build the request body from body-kind flags (POST/PUT/PATCH).
		// QA-MUSTER-041: the encoding follows meta.ContentType — a
		// form-encoded operation must be urlencoded, never JSON.
		if len(bodyFlags) > 0 &&
			(meta.Method == http.MethodPost || meta.Method == http.MethodPut || meta.Method == http.MethodPatch) {
			bodyMap := make(map[string]interface{})
			hasBody := false

			for _, bodyFlag := range bodyFlags {
				if cmd.Flag(bodyFlag.Name) == nil || !cmd.Flag(bodyFlag.Name).Changed {
					continue
				}

				val, err := l.getFlagValue(cmd, bodyFlag)
				if err != nil {
					continue
				}
				if val != nil {
					jsonKey := bodyFlag.Name
					if bodyFlag.JSONName != "" {
						jsonKey = bodyFlag.JSONName
					}
					bodyMap[jsonKey] = val
					hasBody = true
				}
			}

			if hasBody {
				if meta.ContentType == request.ContentTypeFormEncoded {
					builder.FormBody(request.FormValuesFromMap(bodyMap))
				} else {
					bodyJSON, err := json.Marshal(bodyMap)
					if err != nil {
						return fmt.Errorf("marshal request body: %w", err)
					}
					builder.JSONBody(string(bodyJSON))
					builder.SetHeader("Content-Type", "application/json")
				}
			}
		}

		return executeAndReadResponse(builder, cmd, ctx)
	}
}

// executeAndReadResponse sends the request and prints the response.
// DF-016: honors --raw and --output (json/yaml/table) on the reloaded path,
// mirroring the in-session generated path (pkg/generator.executeAndPrint):
// --raw dumps the body untouched; otherwise a JSON body is rendered per
// --output (default table), and non-JSON bodies fall back to the raw dump.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/cmdstore/loader.go
func executeAndReadResponse(builder *request.Builder, cmd *cobra.Command, ctx context.Context) error {
	resp, err := builder.Execute(ctx)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	// QA-MUSTER-036: log EVERY completed HTTP response — success or error —
	// before any body handling, so the INF line stays on stderr and stdout
	// carries only the payload on error paths too.
	logResponse(ctx, resp)

	out := cmd.OutOrStdout()

	// --raw flag: dump the body untouched regardless of format.
	raw, _ := cmd.Flags().GetBool("raw")
	if raw || len(body) == 0 {
		if len(body) > 0 {
			_, _ = fmt.Fprintln(out, string(body))
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP error: %s", resp.Status)
		}
		return nil
	}

	// --output flag: format (default "table"; the loader registers the flag
	// with that default so it is always present).
	format, _ := cmd.Flags().GetString("output")
	if format == "" {
		format = "table"
	}

	// Try to parse the body as JSON. If it's not valid JSON, dump raw
	// regardless of the requested format (same behavior as the generated
	// path).
	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		// QA-MUSTER-036: a non-JSON ERROR body (e.g. the HTML 400 page a
		// Flask dev server answers with) must never reach stdout — piped
		// consumers (`create-post -o json | jq`) would choke on it. On
		// errors the body travels to stderr as a snippet inside the
		// returned error; successful non-JSON payloads still dump raw.
		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP error: %s: %s", resp.Status, summarizeNonJSONBody(body))
		}
		_, _ = fmt.Fprintln(out, string(body))
		return nil
	}

	// Render the output in the requested format.
	// DF-043: mask credential-shaped values (cs_[a-z]{2}_… session keys,
	// "Bearer <token>") before any formatted rendering, mirroring the
	// DF-039 masking of the generated path. --raw above is the escape
	// hatch that dumps the body untouched.
	masked := maskCredentialValues(data)
	switch format {
	case "table":
		printTableOutput(out, masked)
	case "json":
		pretty, err := json.MarshalIndent(masked, "", "  ")
		if err != nil {
			return fmt.Errorf("format JSON: %w", err)
		}
		_, _ = fmt.Fprintln(out, string(pretty))
	case "yaml":
		yamlBytes, err := yaml.Marshal(masked)
		if err != nil {
			// Fall back to indented JSON if YAML marshalling fails.
			pretty, _ := json.MarshalIndent(masked, "", "  ")
			_, _ = fmt.Fprintln(out, string(pretty))
		} else {
			_, _ = fmt.Fprint(out, string(yamlBytes))
		}
	default:
		return fmt.Errorf("unsupported output format %q: supported formats are table, json, yaml", format)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP error: %s", resp.Status)
	}

	return nil
}

// summarizeNonJSONBody condenses a non-JSON error response body into a short
// single-line snippet for the stderr error message: whitespace collapsed,
// capped at a fixed length (QA-MUSTER-036). Mirrors the generator-side
// helper in pkg/generator.
func summarizeNonJSONBody(body []byte) string {
	s := strings.Join(strings.Fields(string(body)), " ")
	if s == "" {
		return "empty response body"
	}
	if len(s) > 160 {
		s = s[:157] + "..."
	}
	return s
}

// logResponse emits the structured log entry for a completed HTTP response.
func logResponse(ctx context.Context, resp *http.Response) {
	logger := log.FromContext(ctx)
	logger.Info().
		Int("status_code", resp.StatusCode).
		Str("status", resp.Status).
		Msg("HTTP response received")
}

// printTableOutput renders JSON data as an aligned table using tabwriter.
// Arrays of objects become rows with key headers; single objects become
// key/value rows; other types are printed as-is. Mirrors
// pkg/generator.printTableOutput so the reloaded path renders identically.
func printTableOutput(out io.Writer, data interface{}) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	switch v := data.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			_, _ = fmt.Fprintf(w, "%s	%v	\n", key, tableFormatValue(v[key]))
		}
	case []interface{}:
		if len(v) == 0 {
			_, _ = fmt.Fprintln(out, "(empty)")
			return
		}
		keys := tableCollectKeys(v)
		_, _ = fmt.Fprintf(w, "%s	\n", strings.Join(keys, "	"))
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				var row []string
				for _, k := range keys {
					row = append(row, tableFormatValue(m[k]))
				}
				_, _ = fmt.Fprintf(w, "%s	\n", strings.Join(row, "	"))
			}
		}
	default:
		_, _ = fmt.Fprintf(w, "%v	\n", v)
	}
	_ = w.Flush()
}

// tableCollectKeys returns the ordered list of unique keys across all objects
// in the array.
func tableCollectKeys(items []interface{}) []string {
	seen := make(map[string]bool)
	var keys []string
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			for k := range m {
				if !seen[k] {
					seen[k] = true
					keys = append(keys, k)
				}
			}
		}
	}
	sort.Strings(keys)
	return keys
}

// tableFormatValue converts a value to a display string for table rendering.
func tableFormatValue(v interface{}) string {
	if v == nil {
		return "-"
	}
	// Render non-scalar values (maps/slices) as compact JSON instead of Go %v syntax.
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		b, err := json.Marshal(v)
		if err != nil {
			// Fallback to %v if JSON marshalling fails.
			break
		}
		s := string(b)
		if len(s) > 80 {
			return s[:77] + "..."
		}
		return s
	}
	// Render float64 without scientific notation so large integer IDs
	// (e.g. 90010011) display as whole numbers, not 9.0010011e+07.
	if f, ok := v.(float64); ok {
		s := strconv.FormatFloat(f, 'f', -1, 64)
		if len(s) > 80 {
			return s[:77] + "..."
		}
		return s
	}
	s := fmt.Sprintf("%v", v)
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}

// getFlagValue retrieves a flag value by its type
func (l *Loader) getFlagValue(cmd *cobra.Command, flagMeta FlagMetadata) (interface{}, error) {
	switch flagMeta.Type {
	case "string":
		return cmd.Flags().GetString(flagMeta.Name)
	case "int":
		return cmd.Flags().GetInt(flagMeta.Name)
	case "bool":
		return cmd.Flags().GetBool(flagMeta.Name)
	case "array":
		return cmd.Flags().GetStringSlice(flagMeta.Name)
	default:
		return nil, nil
	}
}
