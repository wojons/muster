package builtin

import (
	"bufio"
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
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/wojons/muster/pkg/auth"
	"github.com/wojons/muster/pkg/config"
)

// authServiceName is the keychain service identifier for CLI credentials.
const authServiceName = "muster-cli"

// newAuthCommand creates the auth management command with add/list/remove
// subcommands per the built-in commands spec.
func newAuthCommand(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication credentials",
		Long: `Manage authentication credentials stored in the system keychain.

Credentials are keyed by name and stored with a type (api-key, bearer,
basic). Secrets live in the OS keychain; metadata is kept in config.

Use a stored credential on any request with --auth:

  openapi-cli request GET https://api.example.com/tasks --auth myapi
  openapi-cli openapi-cli <resource> <op> --auth myapi

bearer credentials attach as 'Authorization: Bearer <secret>', api-key
credentials as 'X-API-Key: <secret>'.`,
	}

	// auth add --name <name> --type <type> [--value <secret>]
	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Store a credential",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("name")
			credType, _ := cmd.Flags().GetString("type")
			value, _ := cmd.Flags().GetString("value")

			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if credType == "" {
				return fmt.Errorf("--type is required (api-key, bearer, basic)")
			}
			if value == "" {
				// Prompt for the secret (hidden) so it never lands in shell history.
				fmt.Print("Secret: ")
				line, err := bufio.NewReader(os.Stdin).ReadString('\n')
				if err != nil {
					return fmt.Errorf("read secret: %w", err)
				}
				value = strings.TrimSpace(line)
			}

			kc, err := auth.NewKeychain()
			if err != nil {
				return fmt.Errorf("init keychain: %w", err)
			}
			if err := kc.Set(authServiceName, name, value); err != nil {
				return fmt.Errorf("store credential: %w", err)
			}
			if err := storeCredentialMeta(cfg, name, credType); err != nil {
				return err
			}
			fmt.Printf("Stored credential %q (type: %s)\n", name, credType)
			return nil
		},
	}
	addCmd.Flags().String("name", "", "Credential name")
	addCmd.Flags().String("type", "", "Credential type (api-key, bearer, basic)")
	addCmd.Flags().String("value", "", "Secret value (prompted if omitted)")
	cmd.AddCommand(addCmd)

	// auth list
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List stored credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds := loadCredentialMeta(cfg)
			if len(creds) == 0 {
				fmt.Println("No credentials stored.")
				return nil
			}
			names := make([]string, 0, len(creds))
			for name := range creds {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				fmt.Printf("%s\t%s\n", name, creds[name])
			}
			return nil
		},
	})

	// auth remove <name>
	removeCmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			kc, err := auth.NewKeychain()
			if err != nil {
				return fmt.Errorf("init keychain: %w", err)
			}
			if err := kc.Delete(authServiceName, args[0]); err != nil {
				return fmt.Errorf("remove credential: %w", err)
			}
			if err := removeCredentialMeta(cfg, args[0]); err != nil {
				return err
			}
			fmt.Printf("Removed credential %q\n", args[0])
			return nil
		},
	}
	cmd.AddCommand(removeCmd)
	cmd.AddCommand(newAuthMTLSCommand(cfg))

	return cmd
}

// ── auth mtls ────────────────────────────────────────────────────────────

// newAuthMTLSCommand creates `auth mtls` with generate/info/configure
// subcommands per the mTLS spec.
func newAuthMTLSCommand(cfg *config.Config) *cobra.Command {
	mtls := &cobra.Command{
		Use:   "mtls",
		Short: "Manage mutual TLS credentials",
		Long:  `Generate and configure mutual TLS client certificates.`,
	}

	// auth mtls generate [--dir <dir>]
	genCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a self-signed RSA 2048-bit client certificate",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("dir")
			certPath, keyPath, err := generateSelfSignedCert(dir)
			if err != nil {
				return err
			}
			fmt.Printf("Certificate: %s\n", certPath)
			fmt.Printf("Private key: %s\n", keyPath)
			return nil
		},
	}
	genCmd.Flags().String("dir", ".", "Directory to write cert.pem and key.pem into")
	mtls.AddCommand(genCmd)

	// auth mtls info [--cert <path>]
	infoCmd := &cobra.Command{
		Use:   "info",
		Short: "Display certificate details (CN, validity, issuer, serial)",
		RunE: func(cmd *cobra.Command, args []string) error {
			certPath, _ := cmd.Flags().GetString("cert")
			if certPath == "" {
				if cfg != nil && cfg.Auth.MTLS.CertPath != "" {
					certPath = cfg.Auth.MTLS.CertPath
				} else {
					certPath = "cert.pem"
				}
			}
			return printCertInfo(certPath)
		},
	}
	infoCmd.Flags().String("cert", "", "Path to the certificate PEM file")
	mtls.AddCommand(infoCmd)

	// auth mtls configure --cert <path> --key <path> --ca <path> [--enable]
	confCmd := &cobra.Command{
		Use:   "configure",
		Short: "Store mTLS paths in config and enable mTLS",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg == nil {
				return fmt.Errorf("config not initialized")
			}
			certPath, _ := cmd.Flags().GetString("cert")
			keyPath, _ := cmd.Flags().GetString("key")
			caPath, _ := cmd.Flags().GetString("ca")
			enable, _ := cmd.Flags().GetBool("enable")
			if certPath != "" {
				cfg.Auth.MTLS.CertPath = certPath
			}
			if keyPath != "" {
				cfg.Auth.MTLS.KeyPath = keyPath
			}
			if caPath != "" {
				cfg.Auth.MTLS.CAPath = caPath
			}
			if enable {
				cfg.Auth.MTLS.Enabled = true
			}
			if cfg.Auth.MTLS.CertPath == "" || cfg.Auth.MTLS.KeyPath == "" {
				return fmt.Errorf("both --cert and --key are required (generate them first with 'auth mtls generate')")
			}
			fmt.Printf("mTLS configured: cert=%s key=%s ca=%s enabled=%v\n",
				cfg.Auth.MTLS.CertPath, cfg.Auth.MTLS.KeyPath, cfg.Auth.MTLS.CAPath, cfg.Auth.MTLS.Enabled)
			return nil
		},
	}
	confCmd.Flags().String("cert", "", "Client certificate path")
	confCmd.Flags().String("key", "", "Client private key path")
	confCmd.Flags().String("ca", "", "CA certificate path")
	confCmd.Flags().Bool("enable", false, "Enable mTLS for outgoing requests")
	mtls.AddCommand(confCmd)

	return mtls
}

// generateSelfSignedCert creates an RSA 2048-bit self-signed certificate
// and key in dir, returning the written paths.
func generateSelfSignedCert(dir string) (string, string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("generate RSA key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", fmt.Errorf("generate serial: %w", err)
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "muster-client", Organization: []string{"Muster"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", fmt.Errorf("create certificate: %w", err)
	}

	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644); err != nil {
		return "", "", err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		return "", "", err
	}
	return certPath, keyPath, nil
}

// printCertInfo loads a PEM certificate and prints its key fields.
func printCertInfo(certPath string) error {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("read certificate: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("no PEM block found in %s", certPath)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse certificate: %w", err)
	}
	fmt.Printf("Subject CN:  %s\n", cert.Subject.CommonName)
	fmt.Printf("Issuer CN:   %s\n", cert.Issuer.CommonName)
	fmt.Printf("Not before:  %s\n", cert.NotBefore.Format(time.RFC3339))
	fmt.Printf("Not after:   %s\n", cert.NotAfter.Format(time.RFC3339))
	fmt.Printf("Serial:      %s\n", cert.SerialNumber.String())
	fmt.Printf("Client auth: %v\n", hasExtKeyUsage(cert, x509.ExtKeyUsageClientAuth))
	return nil
}

func hasExtKeyUsage(cert *x509.Certificate, want x509.ExtKeyUsage) bool {
	for _, u := range cert.ExtKeyUsage {
		if u == want {
			return true
		}
	}
	return false
}

// ── credential metadata ──────────────────────────────────────────────────

// credentialMetaPath returns the JSON metadata file path for credentials.
func credentialMetaPath(cfg *config.Config) string {
	if cfg != nil && cfg.ConfigDir != "" {
		return cfg.ConfigDir + "/" + auth.CredentialMetaFileName
	}
	dir, _ := os.UserConfigDir()
	return dir + "/muster/" + auth.CredentialMetaFileName
}

// loadCredentialMeta reads the name→type map from disk.
func loadCredentialMeta(cfg *config.Config) map[string]string {
	creds := map[string]string{}
	data, err := os.ReadFile(credentialMetaPath(cfg))
	if err != nil {
		return creds
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return map[string]string{}
	}
	return creds
}

// storeCredentialMeta records name→type, preserving existing entries.
func storeCredentialMeta(cfg *config.Config, name, credType string) error {
	creds := loadCredentialMeta(cfg)
	creds[name] = credType
	return writeCredentialMeta(cfg, creds)
}

// removeCredentialMeta deletes a name from the metadata map.
func removeCredentialMeta(cfg *config.Config, name string) error {
	creds := loadCredentialMeta(cfg)
	delete(creds, name)
	return writeCredentialMeta(cfg, creds)
}

func writeCredentialMeta(cfg *config.Config, creds map[string]string) error {
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	path := credentialMetaPath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
