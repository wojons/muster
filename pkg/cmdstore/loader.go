package cmdstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
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

	// Add flags
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

		if flagMeta.Required {
			_ = cmd.MarkFlagRequired(flagMeta.Name)
		}
	}

	// Always add header flag support to every loaded command for auth
	cmd.Flags().StringSliceP("header", "H", nil, "Add HTTP header (format: 'Key: Value')")

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

		// Raw JSON body override (DF-MUSTER-5): when a raw-body flag
		// (--body/-b) is set, its validated JSON is sent verbatim as the
		// request body and every per-property body flag is ignored. This
		// mirrors pkg/generator.createRunHandler behavior on persisted
		// (reloaded) commands.
		for _, flagMeta := range rawBodyFlags {
			if cmd.Flag(flagMeta.Name) == nil || !cmd.Flag(flagMeta.Name).Changed {
				continue
			}
			raw, _ := cmd.Flags().GetString(flagMeta.Name)
			if !json.Valid([]byte(raw)) {
				return fmt.Errorf("invalid JSON for --%s: value is not valid JSON (value: %q)", flagMeta.Name, raw)
			}
			builder.JSONBody(raw)
			builder.SetHeader("Content-Type", "application/json")
			return executeAndReadResponse(builder, cmd, ctx)
		}

		// Build JSON body from body-kind flags (for POST/PUT/PATCH operations)
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
				bodyJSON, err := json.Marshal(bodyMap)
				if err != nil {
					return fmt.Errorf("marshal request body: %w", err)
				}
				builder.JSONBody(string(bodyJSON))
				builder.SetHeader("Content-Type", "application/json")
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
		logResponse(ctx, resp)
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
		_, _ = fmt.Fprintln(out, string(body))
		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP error: %s", resp.Status)
		}
		logResponse(ctx, resp)
		return nil
	}

	// Render the output in the requested format.
	switch format {
	case "table":
		printTableOutput(out, data)
	case "json":
		pretty, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return fmt.Errorf("format JSON: %w", err)
		}
		_, _ = fmt.Fprintln(out, string(pretty))
	case "yaml":
		yamlBytes, err := yaml.Marshal(data)
		if err != nil {
			// Fall back to indented JSON if YAML marshalling fails.
			pretty, _ := json.MarshalIndent(data, "", "  ")
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

	logResponse(ctx, resp)
	return nil
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
		for key, val := range v {
			_, _ = fmt.Fprintf(w, "%s	%v	\n", key, tableFormatValue(val))
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
