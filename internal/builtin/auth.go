package builtin

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/openapi-cli/openapi-cli/pkg/auth"
	"github.com/openapi-cli/openapi-cli/pkg/config"
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
	cmd.AddCommand(newAuthMTLSCommand())

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

// newAuthMTLSCommand creates the auth mtls command group
func newAuthMTLSCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mtls",
		Short: "Manage mTLS certificates",
		Long:  "Generate, view, and configure mTLS client certificates.",
	}

	cmd.AddCommand(newAuthMTLSGenerateCommand())
	cmd.AddCommand(newAuthMTLSInfoCommand())
	cmd.AddCommand(newAuthMTLSConfigureCommand())

	return cmd
}

func mtlsConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".config", "openapi-cli", "mtls"), nil
}

// newAuthMTLSGenerateCommand creates the auth mtls generate command
func newAuthMTLSGenerateCommand() *cobra.Command {
	var commonName string

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate self-signed client certificate",
		Long:  "Generate a self-signed mTLS client certificate and private key.",
		Example: `  # Generate with default common name
  openapi-cli auth mtls generate

  # Generate with custom common name
  openapi-cli auth mtls generate --common-name my-client`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := mtlsConfigDir()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(dir, 0700); err != nil {
				return fmt.Errorf("create mtls directory: %w", err)
			}

			priv, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				return fmt.Errorf("generate key: %w", err)
			}

			serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
			if err != nil {
				return fmt.Errorf("generate serial: %w", err)
			}

			now := time.Now()
			template := &x509.Certificate{
				SerialNumber: serial,
				Subject: pkix.Name{
					CommonName: commonName,
				},
				NotBefore:             now,
				NotAfter:              now.Add(365 * 24 * time.Hour),
				BasicConstraintsValid: true,
				KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
				ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			}

			certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
			if err != nil {
				return fmt.Errorf("create certificate: %w", err)
			}

			certPath := filepath.Join(dir, "client.crt")
			keyPath := filepath.Join(dir, "client.key")

			certFile, err := os.Create(certPath)
			if err != nil {
				return fmt.Errorf("create cert file: %w", err)
			}
			defer certFile.Close()
			if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
				return fmt.Errorf("encode cert: %w", err)
			}

			keyFile, err := os.Create(keyPath)
			if err != nil {
				return fmt.Errorf("create key file: %w", err)
			}
			defer keyFile.Close()
			if err := pem.Encode(keyFile, &pem.Block{
				Type:  "RSA PRIVATE KEY",
				Bytes: x509.MarshalPKCS1PrivateKey(priv),
			}); err != nil {
				return fmt.Errorf("encode key: %w", err)
			}

			cert, err := x509.ParseCertificate(certDER)
			if err != nil {
				return fmt.Errorf("parse cert: %w", err)
			}

			fmt.Printf("Certificate generated:\n")
			fmt.Printf("  Cert path: %s\n", certPath)
			fmt.Printf("  Key path:  %s\n", keyPath)
			fmt.Printf("  Common Name: %s\n", commonName)
			fmt.Printf("  Expiration: %s\n", cert.NotAfter.Format(time.RFC3339))
			return nil
		},
	}

	cmd.Flags().StringVar(&commonName, "common-name", "muster-client", "Certificate common name")

	return cmd
}

// newAuthMTLSInfoCommand creates the auth mtls info command
func newAuthMTLSInfoCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "info <cert-path>",
		Short: "Display certificate information",
		Long:  "Load and display detailed information about an mTLS certificate.",
		Example: `  # Show certificate info
  openapi-cli auth mtls info ~/.config/openapi-cli/mtls/client.crt`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			certPEM, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read certificate: %w", err)
			}

			block, _ := pem.Decode(certPEM)
			if block == nil {
				return fmt.Errorf("failed to parse certificate PEM")
			}

			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return fmt.Errorf("parse certificate: %w", err)
			}

			fmt.Printf("Common Name: %s\n", cert.Subject.CommonName)
			fmt.Printf("Valid From: %s\n", cert.NotBefore.Format(time.RFC3339))
			fmt.Printf("Valid To:   %s\n", cert.NotAfter.Format(time.RFC3339))
			fmt.Printf("Issuer:     %s\n", cert.Issuer.CommonName)
			fmt.Printf("Serial:     %s\n", cert.SerialNumber.String())

			var keyUsage []string
			if cert.KeyUsage&x509.KeyUsageDigitalSignature != 0 {
				keyUsage = append(keyUsage, "Digital Signature")
			}
			if cert.KeyUsage&x509.KeyUsageKeyEncipherment != 0 {
				keyUsage = append(keyUsage, "Key Encipherment")
			}
			if cert.KeyUsage&x509.KeyUsageCertSign != 0 {
				keyUsage = append(keyUsage, "Certificate Sign")
			}
			fmt.Printf("Key Usage:  %v\n", keyUsage)

			var extKeyUsage []string
			for _, eku := range cert.ExtKeyUsage {
				switch eku {
				case x509.ExtKeyUsageServerAuth:
					extKeyUsage = append(extKeyUsage, "Server Auth")
				case x509.ExtKeyUsageClientAuth:
					extKeyUsage = append(extKeyUsage, "Client Auth")
				case x509.ExtKeyUsageCodeSigning:
					extKeyUsage = append(extKeyUsage, "Code Signing")
				default:
					extKeyUsage = append(extKeyUsage, fmt.Sprintf("Unknown (%d)", eku))
				}
			}
			fmt.Printf("Extended Key Usage: %v\n", extKeyUsage)

			expired := time.Now().After(cert.NotAfter)
			if expired {
				fmt.Println("Status: EXPIRED")
			} else {
				fmt.Println("Status: Valid")
			}

			return nil
		},
	}
}

// newAuthMTLSConfigureCommand creates the auth mtls configure command
func newAuthMTLSConfigureCommand() *cobra.Command {
	var certPath, keyPath, caPath string

	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Configure mTLS certificate paths",
		Long:  "Store mTLS certificate, key, and CA paths for use in API requests.",
		Example: `  # Configure mTLS paths
  openapi-cli auth mtls configure --cert ~/.config/openapi-cli/mtls/client.crt --key ~/.config/openapi-cli/mtls/client.key --ca /path/to/ca.crt`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if certPath == "" || keyPath == "" {
				return fmt.Errorf("--cert and --key are required")
			}

			cm, err := config.NewConfigManager()
			if err != nil {
				return fmt.Errorf("initialize config: %w", err)
			}

			if err := cm.Set("auth.mtls.enabled", true); err != nil {
				return fmt.Errorf("set mtls enabled: %w", err)
			}
			if err := cm.Set("auth.mtls.cert_path", certPath); err != nil {
				return fmt.Errorf("set cert path: %w", err)
			}
			if err := cm.Set("auth.mtls.key_path", keyPath); err != nil {
				return fmt.Errorf("set key path: %w", err)
			}
			if caPath != "" {
				if err := cm.Set("auth.mtls.ca_path", caPath); err != nil {
					return fmt.Errorf("set ca path: %w", err)
				}
			}

			fmt.Println("mTLS configuration saved:")
			fmt.Printf("  Cert path: %s\n", certPath)
			fmt.Printf("  Key path:  %s\n", keyPath)
			if caPath != "" {
				fmt.Printf("  CA path:   %s\n", caPath)
			}
			fmt.Println("mTLS: enabled")
			return nil
		},
	}

	cmd.Flags().StringVar(&certPath, "cert", "", "Path to client certificate file")
	cmd.Flags().StringVar(&keyPath, "key", "", "Path to client private key file")
	cmd.Flags().StringVar(&caPath, "ca", "", "Path to CA certificate file")

	return cmd
}
