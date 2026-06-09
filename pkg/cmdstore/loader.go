package cmdstore

import (
	"context"
	"encoding/json"
	"fmt"
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

		// Add query parameters from flags
		for _, flagMeta := range meta.Flags {
			value, err := l.getFlagValue(cmd, flagMeta)
			if err != nil {
				continue
			}
			if value != nil {
				builder.WithQueryParam(flagMeta.Name, fmt.Sprintf("%v", value))
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
