package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/wojons/muster/pkg/auth"
	"github.com/wojons/muster/pkg/cache"
	"github.com/wojons/muster/pkg/client"
	"github.com/wojons/muster/pkg/config"
	"github.com/wojons/muster/pkg/request"
	"github.com/wojons/muster/pkg/wasm"
)

func newRequestCommand(cfg *config.Config) *cobra.Command {
	var (
		headers     []string
		params      []string
		output      string
		verbose     bool
		timeout     time.Duration
		httpBody    string
		wasmPlugin  string
		authName    string
		httpVersion string
	)

	cmd := &cobra.Command{
		Use:   "request <method> <url>",
		Short: "Make HTTP requests",
		Long: `Send HTTP requests with support for various methods, headers, and output formats.

Attach a stored credential (see 'auth add') with --auth NAME: bearer
credentials are sent as an 'Authorization: Bearer ***' header, api-key
credentials as an 'X-API-Key: *** header. An explicit -H header with
the same name takes precedence over the stored credential.

--http-version selects the HTTP transport version policy: 'auto' (default)
negotiates opportunistically, '1.1' and '2' require the named version, and '3'
prefers HTTP/3 (needs an HTTP/3 transport). With -v the version the request
actually used is reported on stderr as an 'HTTP: <version>' line; a required
version that could not be used fails with PROTOCOL_MISMATCH instead of
silently using another one.`,
		Args: cobra.ExactArgs(2),
		// SilenceUsage prevents the full usage block from being dumped when a
		// runtime error occurs (e.g. a 4xx/5xx HTTP response). Flag/arg misuse
		// still surfaces usage via the custom FlagErrorFunc + Args validator
		// below, which temporarily re-enable usage for those error classes.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			method := strings.ToUpper(args[0])
			targetURL := args[1]

			// The flag is the invocation-level override of the configured
			// request.http_version policy (PROTOCOLS.md §4.7 N4). Resolve it
			// onto the in-memory config so the handler reads one source.
			if httpVersion != "" && cfg != nil {
				cfg.Request.HTTPVersion = httpVersion
			}

			return handleRequest(cfg, method, targetURL, headers, params, output, verbose, timeout, httpBody, wasmPlugin, authName)
		},
	}

	// Flag parsing errors (unknown flag, bad value) should still show usage.
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		c.SilenceUsage = false
		return err
	})

	// Arg validation errors (wrong arg count) should still show usage. We wrap
	// the standard ExactArgs(2) validator and un-silence usage on failure.
	cmd.Args = func(c *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(2)(c, args); err != nil {
			c.SilenceUsage = false
			return err
		}
		return nil
	}

	cmd.Flags().StringSliceVarP(&headers, "header", "H", []string{}, "Add HTTP header (format: 'Key: Value')")
	cmd.Flags().StringVar(&authName, "auth", "", "Attach a stored credential by name (see 'auth add')")
	cmd.Flags().StringSliceVarP(&params, "param", "p", []string{}, "Add query parameter (format: 'key=value')")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format (json, yaml, table)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Request timeout")
	cmd.Flags().StringVarP(&httpBody, "body", "b", "", "Request body for POST/PUT requests")
	cmd.Flags().StringVar(&wasmPlugin, "wasm-plugin", "", "Path to WASM plugin for response transformation")
	cmd.Flags().StringVar(&httpVersion, "http-version", "", "HTTP transport version policy: auto (default), 1.1, 2 or 3 (overrides request.http_version)")

	return cmd
}

// validateURLScheme checks if the URL scheme is safe for HTTP requests.
// Only http and https schemes are allowed to prevent SSRF attacks.
func validateURLScheme(scheme string) error {
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported scheme %q - only http and https are allowed", scheme)
	}
	return nil
}

// resolveRequestCredential resolves the named stored credential for the
// request command (DF-023). Package-level var so tests can seam it without
// touching the platform keychain; production always uses the real chain.
var resolveRequestCredential = func(name string) (auth.CredentialHeader, error) {
	kc, err := auth.NewKeychain()
	if err != nil {
		return auth.CredentialHeader{}, fmt.Errorf("init keychain: %w", err)
	}
	return auth.ResolveCredentialHeader(kc, name)
}

func handleRequest(cfg *config.Config, method, targetURL string, headers, params []string, output string, verbose bool, timeout time.Duration, httpBody, wasmPlugin, authName string) error {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}

	if err := validateURLScheme(parsedURL.Scheme); err != nil {
		return fmt.Errorf("invalid URL scheme: %w", err)
	}

	builder := request.NewBuilder(parsedURL.Scheme+"://"+parsedURL.Host, parsedURL.Path, method)

	// DF-023: attach a stored credential BEFORE explicit -H headers so a
	// manual header with the same name keeps precedence (WithHeader
	// overwrites). The resolved credential is kept for the verbose dump.
	var attachedCred *auth.CredentialHeader
	if authName != "" {
		cred, err := resolveRequestCredential(authName)
		if err != nil {
			return fmt.Errorf("resolve --auth: %w", err)
		}
		builder.WithHeader(cred.Name, cred.Value)
		attachedCred = &cred
	}

	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			builder.WithHeader(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}

	for _, p := range params {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			builder.WithQueryParam(kv[0], kv[1])
		}
	}

	if httpBody != "" && (method == "POST" || method == "PUT" || method == "PATCH") {
		builder.JSONBody(httpBody)
	}

	builder.WithTimeout(timeout)
	httpReq, err := builder.Build()
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	// Resolve the HTTP transport version policy (PROTOCOLS.md §4.7 N4) before
	// anything is sent: an unknown value is a validation error naming the
	// accepted set, never a silent fallback to "auto".
	version, err := client.NormalizeHTTPVersion(requestHTTPVersion(cfg))
	if err != nil {
		return err
	}

	// Build client options. WithHTTPVersion installs the base transport and
	// must precede the wrapper options, which wrap whatever transport exists
	// when they run (PROTOCOLS.md §4.7 N2).
	var clientOpts []client.ClientOption
	clientOpts = append(clientOpts, client.WithTimeout(timeout))
	clientOpts = append(clientOpts, client.WithHTTPVersion(version))
	if cfg != nil && cfg.RateLimit.Enabled {
		clientOpts = append(clientOpts, client.WithRateLimiter(cfg.RateLimit.RatePerSec, cfg.RateLimit.Burst))
	}
	if cfg != nil && cfg.Cache.Enabled {
		c := cache.NewMemoryCache(int(cfg.Cache.MaxSize))
		// NOTE: cache is created per-request here; for long-running consumers
		// (daemon, MCP server), a shared cache should be created at startup.
		clientOpts = append(clientOpts, client.WithCache(c, cfg.Cache.TTL))
	}
	httpClient := client.NewClient(clientOpts...)
	resp, err := httpClient.Do(context.Background(), httpReq)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	// Apply WASM plugin transformation if configured
	if wasmPlugin != "" {
		interceptor := wasm.NewInterceptor()
		if err := interceptor.LoadPlugin(context.Background(), wasmPlugin); err != nil {
			return fmt.Errorf("wasm: load plugin %q: %w", wasmPlugin, err)
		}
		defer func() { _ = interceptor.Close(context.Background()) }()

		contentType := resp.Header.Get("Content-Type")
		transformed, err := interceptor.TransformResponseBody(context.Background(), body, contentType, wasmPlugin)
		if err != nil {
			return fmt.Errorf("wasm: transform response: %w", err)
		}
		body = transformed
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Request: %s %s\n", method, targetURL)
		fmt.Fprintf(os.Stderr, "Status: %d %s\n", resp.StatusCode, resp.Status)
		// PROTOCOLS.md §4.7 N7: report the version the request actually used
		// on stderr (never stdout, whose payload fidelity is frozen). The
		// value comes from the client's ProtocolRecorder, not from
		// resp.Proto, which the cache and a local refusal rebuild.
		fmt.Fprintf(os.Stderr, "HTTP: %s\n", formatHTTPVersion(httpClient.Protocol()))
		// DF-023: report the attached credential MASKED — the secret is
		// never echoed in verbose output ("Authorization: Bearer ***").
		if attachedCred != nil {
			fmt.Fprintf(os.Stderr, "%s: %s\n", attachedCred.Name, maskAuthHeaderValue(attachedCred.Value))
		}
		for k, v := range resp.Header {
			joined := strings.Join(v, ", ")
			// DF-023: never echo credential secrets in verbose output —
			// a credential header echoed by the server renders masked.
			if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "X-API-Key") {
				joined = maskAuthHeaderValue(joined)
			}
			fmt.Fprintf(os.Stderr, "%s: %s\n", k, joined)
		}
		fmt.Fprintln(os.Stderr)
	}

	// Print response body to stdout
	if len(body) > 0 {
		if output == "json" {
			var data interface{}
			if err := json.Unmarshal(body, &data); err == nil {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(data)
			} else {
				_, _ = fmt.Fprintln(os.Stdout, string(body))
			}
		} else {
			_, _ = fmt.Fprintln(os.Stdout, string(body))
		}
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	return nil
}

// requestHTTPVersion returns the HTTP transport version policy in force for a
// request: the configured value (request.http_version, PROTOCOLS.md §4.7 N4)
// or the library default when there is no configuration.
func requestHTTPVersion(cfg *config.Config) string {
	if cfg == nil {
		return client.HTTPVersionAuto
	}
	return cfg.Request.HTTPVersion
}

// formatHTTPVersion renders a protocol record for the verbose diagnostic line:
// the version the exchange used and where the response came from, plus the
// policy that was in force. It never invents a version — a response with no
// wire exchange renders as "unknown".
func formatHTTPVersion(rec client.ProtocolRecord) string {
	observed := rec.Observed
	if observed == "" {
		observed = "unknown"
	}
	source := rec.Source
	if source == "" {
		source = "unobserved"
	}
	return fmt.Sprintf("%s (%s, requested %s)", observed, source, rec.Requested)
}

// maskAuthHeaderValue renders an Authorization/X-API-Key header value safe
// for verbose output: "Bearer abc" becomes "Bearer MASKED"; anything else
// becomes "MASKED".
func maskAuthHeaderValue(value string) string {
	if i := strings.Index(value, " "); i > 0 {
		return value[:i+1] + "***"
	}
	return "***"
}
