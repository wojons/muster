package cmdstore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/wojons/muster/pkg/log"
	"github.com/wojons/muster/pkg/request"
	"github.com/spf13/cobra"
)

// Loader reconstructs commands from stored metadata
type Loader struct {
	store   *CommandStore
	baseURL string
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

	// Add output flag
	cmd.Flags().StringP("output", "o", "table", "Output format (json, yaml, table)")

	// Set up the run handler to execute the HTTP request
	cmd.RunE = l.createRunHandler(meta, baseURL)

	return cmd, nil
}

// createRunHandler creates a handler that makes HTTP requests
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/cmdstore/loader.go
func (l *Loader) createRunHandler(meta CommandMetadata, baseURL string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

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

		// Separate flags into body, query, and header groups
		var bodyFlags []FlagMetadata
		var queryFlags []FlagMetadata

		for _, flagMeta := range meta.Flags {
			switch flagMeta.Kind {
			case FlagKindBody:
				bodyFlags = append(bodyFlags, flagMeta)
			case FlagKindHeader:
				// Header flags are handled separately below
			default:
				queryFlags = append(queryFlags, flagMeta)
			}
		}

		// Add query parameters from query-kind flags
		for _, flagMeta := range queryFlags {
			value, err := l.getFlagValue(cmd, flagMeta)
			if err != nil {
				continue
			}
			if value != nil {
				builder.WithQueryParam(flagMeta.Name, fmt.Sprintf("%v", value))
			}
		}

		// Build JSON body from body-kind flags (for POST/PUT/PATCH operations)
		if len(bodyFlags) > 0 &&
			(meta.Method == http.MethodPost || meta.Method == http.MethodPut || meta.Method == http.MethodPatch) {
			bodyMap := make(map[string]interface{})
			hasBody := false

			for _, bodyFlag := range bodyFlags {
				// Only include flags explicitly set by the user (matching generator behavior).
				// Required flags are enforced by cobra so Changed will be true for them.
				if cmd.Flag(bodyFlag.Name) == nil || !cmd.Flag(bodyFlag.Name).Changed {
					continue
				}

				val, err := l.getFlagValue(cmd, bodyFlag)
				if err != nil {
					continue
				}
				if val != nil {
					// Use the original JSON property name if available, otherwise the flag name
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

		// Execute request
		resp, err := builder.Execute(ctx)
		if err != nil {
			return fmt.Errorf("execute request: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP error: %s", resp.Status)
		}

		// Get output format
		output, _ := cmd.Flags().GetString("output")

		// Log response status
		logger := log.FromContext(ctx)
		logger.Info().
			Int("status_code", resp.StatusCode).
			Str("status", resp.Status).
			Msg("HTTP response received")

		if output == "json" || output == "yaml" {
			// Parse and output body
			decoder := json.NewDecoder(resp.Body)
			var result interface{}
			if err := decoder.Decode(&result); err == nil {
				if output == "json" {
					encoder := json.NewEncoder(cmd.OutOrStdout())
					_ = encoder.Encode(result)
				} else {
					// YAML output (simplified)
					logger.Debug().Interface("response", result).Msg("Response body")
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%v\n", result)
				}
			}
		}

		return nil
	}
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
