package builtin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/wojons/muster/pkg/request"
	"github.com/spf13/cobra"
)

func newRequestCommand() *cobra.Command {
	var (
		headers  []string
		params   []string
		output   string
		verbose  bool
		timeout  time.Duration
		httpBody string
	)

	cmd := &cobra.Command{
		Use:   "request <method> <url>",
		Short: "Make HTTP requests",
		Long:  `Send HTTP requests with support for various methods, headers, and output formats.`,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			method := strings.ToUpper(args[0])
			targetURL := args[1]

			return handleRequest(method, targetURL, headers, params, output, verbose, timeout, httpBody)
		},
	}

	cmd.Flags().StringSliceVarP(&headers, "header", "H", []string{}, "Add HTTP header (format: 'Key: Value')")
	cmd.Flags().StringSliceVarP(&params, "param", "p", []string{}, "Add query parameter (format: 'key=value')")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format (json, yaml, table)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Request timeout")
	cmd.Flags().StringVarP(&httpBody, "body", "b", "", "Request body for POST/PUT requests")

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

func handleRequest(method, targetURL string, headers, params []string, output string, verbose bool, timeout time.Duration, httpBody string) error {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}

	if err := validateURLScheme(parsedURL.Scheme); err != nil {
		return fmt.Errorf("invalid URL scheme: %w", err)
	}

	builder := request.NewBuilder(parsedURL.Scheme+"://"+parsedURL.Host, parsedURL.Path, method)

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

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Request: %s %s\n", method, targetURL)
		fmt.Fprintf(os.Stderr, "Status: %d %s\n", resp.StatusCode, resp.Status)
		for k, v := range resp.Header {
			fmt.Fprintf(os.Stderr, "%s: %s\n", k, strings.Join(v, ", "))
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
				fmt.Fprintln(os.Stdout, string(body))
			}
		} else {
			fmt.Fprintln(os.Stdout, string(body))
		}
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	return nil
}
