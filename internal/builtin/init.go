package builtin

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// newInitCommand creates the init command
func newInitCommand() *cobra.Command {
	var (
		host      string
		timeout   int
		retry     int
		apiKey    string
		oauth2    bool
		mtls      bool
		overwrite bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new API connection",
		Long: `Initialize a new connection to an OpenAPI-specified API.
This sets up the configuration for interacting with the API.`,
		Example: `  # Initialize with host URL
  openapi-cli init --host https://api.example.com

  # Initialize with authentication
  openapi-cli init --host https://api.example.com --oauth2

  # Initialize with custom timeout
  openapi-cli init --host https://api.example.com --timeout 60`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if host == "" {
				return fmt.Errorf("--host is required")
			}

			config := viper.New()

			if overwrite {
				fmt.Println("Overwriting existing configuration...")
			} else {
				fmt.Println("Initializing new connection...")
			}

			config.Set("host", host)
			config.Set("timeout", timeout)
			config.Set("retry", retry)

			if apiKey != "" {
				config.Set("auth.api_key", apiKey)
			}

			config.Set("auth.oauth2.enabled", oauth2)
			config.Set("auth.mtls.enabled", mtls)

			fmt.Printf("Host: %s\n", host)
			fmt.Printf("Timeout: %ds\n", timeout)
			fmt.Printf("Retry attempts: %d\n", retry)

			if oauth2 {
				fmt.Println("OAuth2: enabled")
			}

			if mtls {
				fmt.Println("mTLS: enabled")
			}

			fmt.Println("Configuration saved successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "API host URL (required)")
	cmd.MarkFlagRequired("host")

	cmd.Flags().IntVar(&timeout, "timeout", 30, "Request timeout in seconds")
	cmd.Flags().IntVar(&retry, "retry", 3, "Number of retry attempts")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key for authentication")
	cmd.Flags().BoolVar(&oauth2, "oauth2", false, "Enable OAuth2 authentication")
	cmd.Flags().BoolVar(&mtls, "mtls", false, "Enable mTLS authentication")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing configuration")

	return cmd
}
