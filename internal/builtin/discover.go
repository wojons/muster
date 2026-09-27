package builtin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"
	"github.com/wojons/muster/pkg/client"
	"github.com/wojons/muster/pkg/openapi"
)

// newDiscoverCommand creates the "discover" subcommand.
// It fetches an OpenAPI spec from a URL and displays all operations in a readable format,
// grouped by tag with operationId shown inline. Does not persist commands.
func newDiscoverCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discover <path-or-url>",
		Short: "Discover API operations from an OpenAPI spec",
		Long: `Read an OpenAPI 3.x specification and display all operations
grouped by tag, with method, path, and operationId.

Swagger 2.0 documents are also accepted and converted to the
OpenAPI 3.x model before parsing (the output notes the conversion).

Accepts either a local file path or a remote URL (http/https).
No commands are persisted.

Examples:
  openapi-cli discover demo/openapi-specs/petstore.yaml
  openapi-cli discover https://petstore3.swagger.io/api/v3/openapi.json
  openapi-cli discover http://localhost:9800/openapi.json`,
		Args: cobra.ExactArgs(1),
		RunE: runDiscover,
	}

	return cmd
}

// runDiscover is the RunE function for the discover command.
func runDiscover(cmd *cobra.Command, args []string) error {
	input := args[0]

	// Determine whether to load from file or URL (mirrors generate.go)
	var data []byte
	var err error
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		// Fetch the spec from the URL using pkg/client
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		httpClient := client.NewClient()
		resp, err := httpClient.Get(ctx, input)
		if err != nil {
			return fmt.Errorf("fetch spec from %q: %w", input, err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status %d from %q: %s", resp.StatusCode, input, resp.Status)
		}

		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response body from %q: %w", input, err)
		}
	} else {
		// Local file path
		data, err = os.ReadFile(input)
		if err != nil {
			return fmt.Errorf("read spec file %q: %w", input, err)
		}
	}

	// Parse the spec using pkg/openapi
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	opts := openapi.DefaultParseOptions()

	parser := openapi.NewParser()
	result, err := parser.Parse(ctx, data, opts)
	if err != nil {
		return fmt.Errorf("parse spec from %q (not valid OpenAPI?): %w", input, err)
	}

	title := result.Document.Info.Title
	version := result.Version
	if title == "" {
		title = "Unnamed API"
	}

	// Extract all operations
	operations, err := parser.ExtractOperations(result.Document)
	if err != nil {
		return fmt.Errorf("extract operations from %q: %w", input, err)
	}

	// Print header
	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "API: %s (v%s)\n", title, version)
	_, _ = fmt.Fprintf(w, "Base URL: %s\n", displayBaseURL(result.Document, input))
	// PROTO-006: state the conversion explicitly — a Swagger 2.0 document
	// was transparently upgraded to the OpenAPI 3.x model before parsing.
	if result.ConvertedFromSwagger2 {
		_, _ = fmt.Fprintln(w, "Note: converted from Swagger/OpenAPI 2.0 to OpenAPI 3.x")
	}

	if len(operations) == 0 {
		_, _ = fmt.Fprintln(w, "\nNo operations found in the spec.")
		return nil
	}

	_, _ = fmt.Fprintln(w)

	// Sort operations: by tag, then path, then method
	sortOperations(operations)

	// Print grouped by tag
	var currentTag string
	for _, op := range operations {
		tag := firstTag(op.Tags)
		if tag == "" {
			tag = "Other"
		}

		if tag != currentTag {
			currentTag = tag
			_, _ = fmt.Fprintln(w, tag)
		}

		// Format:   METHOD  /path          operationId
		_, _ = fmt.Fprintf(w, "  %-6s %-40s %s\n", op.Method, op.Path, op.ID)
	}

	return nil
}

// displayBaseURL returns the spec's root server URL when the spec declares a
// servers list, falling back to the scheme+host derived from the spec source
// (URL or local file path). GAP-016: a local spec previously printed its FILE
// PATH as the Base URL instead of the actual server URL.
func displayBaseURL(doc *openapi3.T, input string) string {
	if doc != nil && len(doc.Servers) > 0 && doc.Servers[0] != nil && doc.Servers[0].URL != "" {
		return doc.Servers[0].URL
	}
	return getBaseURL(input)
}

// getBaseURL extracts the scheme+host from a full URL for display.
func getBaseURL(rawURL string) string {
	schemeEnd := strings.Index(rawURL, "://")
	if schemeEnd < 0 {
		return rawURL
	}
	afterScheme := rawURL[schemeEnd+3:]
	slashIdx := strings.Index(afterScheme, "/")
	if slashIdx < 0 {
		// Just http://host with no path
		return rawURL
	}
	return rawURL[:schemeEnd+3+slashIdx]
}

// sortOperations sorts operations by tag, then path, then method for predictable output.
func sortOperations(ops []*openapi.Operation) {
	sort.Slice(ops, func(i, j int) bool {
		ti := firstTag(ops[i].Tags)
		tj := firstTag(ops[j].Tags)
		if ti != tj {
			return ti < tj
		}
		if ops[i].Path != ops[j].Path {
			return ops[i].Path < ops[j].Path
		}
		return methodOrder(ops[i].Method) < methodOrder(ops[j].Method)
	})
}

// firstTag returns the first tag or an empty string.
func firstTag(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return tags[0]
}

// fetchSpec fetches the content from a URL using net/http.
// Used by both discover and generate commands to fetch specs from remote URLs.
func fetchSpec(url string) ([]byte, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("invalid URL: must start with http:// or https://")
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	return body, nil
}

// methodOrder returns a sort order for HTTP methods.
func methodOrder(method string) int {
	switch strings.ToUpper(method) {
	case "GET":
		return 0
	case "POST":
		return 1
	case "PUT":
		return 2
	case "PATCH":
		return 3
	case "DELETE":
		return 4
	case "HEAD":
		return 5
	case "OPTIONS":
		return 6
	default:
		return 9
	}
}
