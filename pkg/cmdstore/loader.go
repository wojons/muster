package cmdstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wojons/muster/pkg/log"
	"github.com/wojons/muster/pkg/request"
)

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
	cmd := &cobra.Command{
		Use:     meta.Use,
		Short:   meta.Short,
		Long:    meta.Long,
		Example: meta.Example,
	}

	// Add flags
	for _, flagMeta := range meta.Flags {
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

	// Runtime base-URL override: --base-url flag or MUSTER_BASE_URL env var
	// takes precedence over the stored/generate-time URL, so a generated CLI
	// can be pointed at staging/prod without regenerating.
	cmd.Flags().StringP("base-url", "", "", "Override the API base URL at runtime (env: MUSTER_BASE_URL)")

	// Add output flag
	cmd.Flags().StringP("output", "o", "table", "Output format (json, yaml, table)")

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
		var fileFlagPath string

		for _, flagMeta := range meta.Flags {
			switch flagMeta.Kind {
			case FlagKindBody:
				bodyFlags = append(bodyFlags, flagMeta)
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

	if len(body) > 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(body))
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP error: %s", resp.Status)
	}

	logger := log.FromContext(ctx)
	logger.Info().
		Int("status_code", resp.StatusCode).
		Str("status", resp.Status).
		Msg("HTTP response received")

	return nil
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
