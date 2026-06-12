package builtin

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/openapi-cli/openapi-cli/pkg/auth"
	"github.com/spf13/cobra"
)

const credentialService = "openapi-cli"

type credential struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	TokenURL     string `json:"token_url,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// newAuthCommand creates the auth command group
func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication commands",
		Long:  "Manage authentication for API connections.",
	}

	cmd.AddCommand(newAuthLoginCommand())
	cmd.AddCommand(newAuthLogoutCommand())
	cmd.AddCommand(newAuthStatusCommand())
	cmd.AddCommand(newAuthAddCommand())
	cmd.AddCommand(newAuthListCommand())
	cmd.AddCommand(newAuthRemoveCommand())

	return cmd
}

// newAuthLoginCommand creates the auth login command
func newAuthLoginCommand() *cobra.Command {
	var (
		username string
		password string
		token    string
		oauth2   bool
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with server",
		Long:  "Authenticate with the API server using the specified method.",
		Example: `  # Login with OAuth2
  openapi-cli auth login --oauth2

  # Login with username/password
  openapi-cli auth login --username admin --password secret

  # Login with bearer token
  openapi-cli auth login --token eyJhbGciOiJIUzI1NiJ9...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if oauth2 {
				fmt.Println("Initiating OAuth2 flow...")
				fmt.Println("Please complete authentication in your browser")
				return nil
			}

			if token != "" {
				fmt.Println("Authenticating with token...")
				return nil
			}

			if username != "" {
				fmt.Printf("Authenticating user: %s\n", username)
				return nil
			}

			return fmt.Errorf("please specify authentication method (--oauth2, --token, or --username/--password)")
		},
	}

	cmd.Flags().StringVar(&username, "username", "", "Username for basic auth")
	cmd.Flags().StringVar(&password, "password", "", "Password for basic auth")
	cmd.Flags().StringVar(&token, "token", "", "Bearer token for authentication")
	cmd.Flags().BoolVar(&oauth2, "oauth2", false, "Use OAuth2 authentication")

	return cmd
}

// newAuthLogoutCommand creates the auth logout command
func newAuthLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear authentication",
		Long:  "Clear stored authentication credentials.",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Clearing authentication credentials...")
			return nil
		},
	}
}

// newAuthStatusCommand creates the auth status command
func newAuthStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check authentication status",
		Long:  "Display the current authentication status.",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Authentication Status: Not authenticated")
			fmt.Println("Run 'openapi-cli auth login' to authenticate")
			return nil
		},
	}
}

// newAuthAddCommand creates the auth add command
func newAuthAddCommand() *cobra.Command {
	var (
		name         string
		credType     string
		tokenURL     string
		clientID     string
		clientSecret string
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a stored credential",
		Long:  "Add a credential (OAuth2 token, API key, or basic auth) to the secure keychain.",
		Example: `  # Add an OAuth2 credential
  openapi-cli auth add --name production --type oauth2 --token-url https://auth.example.com/token --client-id myapp --client-secret s3cret

  # Add an API key
  openapi-cli auth add --name github --type api-key --client-secret ghp_xxxxxxxxxxxx

  # Add basic auth credentials
  openapi-cli auth add --name staging --type basic --client-id admin --client-secret password123`,
		RunE: func(cmd *cobra.Command, args []string) error {
			kc, err := auth.NewKeychain()
			if err != nil {
				return fmt.Errorf("initialize keychain: %w", err)
			}

			cred := credential{
				Name:         name,
				Type:         credType,
				TokenURL:     tokenURL,
				ClientID:     clientID,
				ClientSecret: clientSecret,
				CreatedAt:    time.Now().UTC().Format(time.RFC3339),
			}

			data, err := json.Marshal(cred)
			if err != nil {
				return fmt.Errorf("marshal credential: %w", err)
			}

			if err := kc.Set(credentialService, name, string(data)); err != nil {
				return fmt.Errorf("store credential: %w", err)
			}

			fmt.Printf("Credential %q added successfully.\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Credential name")
	cmd.Flags().StringVar(&credType, "type", "", "Credential type (oauth2, api-key, basic)")
	cmd.Flags().StringVar(&tokenURL, "token-url", "", "OAuth2 token URL")
	cmd.Flags().StringVar(&clientID, "client-id", "", "Client ID (for OAuth2 or basic auth)")
	cmd.Flags().StringVar(&clientSecret, "client-secret", "", "Client secret (for OAuth2, API key, or basic auth)")

	return cmd
}

// newAuthListCommand creates the auth list command
func newAuthListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List stored credentials",
		Long:  "List all credentials stored in the secure keychain.",
		Example: `  # List all credentials
  openapi-cli auth list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			kc, err := auth.NewKeychain()
			if err != nil {
				return fmt.Errorf("initialize keychain: %w", err)
			}

			accounts, err := kc.List(credentialService)
			if err != nil {
				return fmt.Errorf("list credentials: %w", err)
			}

			if len(accounts) == 0 {
				fmt.Println("No credentials stored.")
				return nil
			}

			fmt.Println("Stored credentials:")
			for _, account := range accounts {
				data, err := kc.Get(credentialService, account.Account)
				if err != nil {
					fmt.Printf("  - %s (unable to read)\n", account.Account)
					continue
				}

				var cred credential
				if err := json.Unmarshal([]byte(data), &cred); err != nil {
					fmt.Printf("  - %s\n", account.Account)
					continue
				}

				fmt.Printf("  - %s (type: %s, created: %s)\n", cred.Name, cred.Type, cred.CreatedAt)
			}

			return nil
		},
	}
}

// newAuthRemoveCommand creates the auth remove command
func newAuthRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a stored credential",
		Long:  "Remove a credential from the secure keychain by name.",
		Example: `  # Remove a credential
  openapi-cli auth remove production`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			kc, err := auth.NewKeychain()
			if err != nil {
				return fmt.Errorf("initialize keychain: %w", err)
			}

			if err := kc.Delete(credentialService, name); err != nil {
				return fmt.Errorf("remove credential: %w", err)
			}

			fmt.Printf("Credential %q removed successfully.\n", name)
			return nil
		},
	}
}
