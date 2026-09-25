package auth

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// credentialServiceName is the keychain service identifier the CLI's
// `auth add` command writes credentials under.
const credentialServiceName = "muster-cli"

// CredentialConfigDir returns the CLI config directory where
// credentials.json lives, matching pkg/config's platform layout
// ($XDG_CONFIG_HOME/muster, %APPDATA%\muster, or ~/.config/muster).
func CredentialConfigDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "muster")
	}
	if runtime.GOOS == "windows" {
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, "muster")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "muster")
}

// credentialHeaderName returns the HTTP header a stored credential of the
// given type is injected as.
//
//	bearer   → Authorization
//	api-key  → X-API-Key
//	basic    → Authorization
func credentialHeaderName(credType string) string {
	switch strings.ToLower(strings.TrimSpace(credType)) {
	case "bearer", "basic":
		return "Authorization"
	default:
		// "api-key" and any future type default to the conventional
		// X-API-Key header.
		return "X-API-Key"
	}
}

// credentialHeaderValue formats the secret for its header. bearer becomes
// "Bearer <secret>", basic is expected to carry a pre-encoded
// "user:password" value and becomes "Basic <secret>", api-key is sent raw.
func credentialHeaderValue(credType, secret string) string {
	switch strings.ToLower(strings.TrimSpace(credType)) {
	case "bearer":
		return "Bearer " + secret
	case "basic":
		return "Basic " + secret
	default:
		return secret
	}
}

// CredentialHeader is the resolved credential: the header to set, its
// formatted value, and a masked rendering safe for verbose/log output
// (the secret is never included).
type CredentialHeader struct {
	Name     string
	Value    string
	Shielded string
}

// shieldValue masks a formatted header value: "Bearer abc123" renders as
// "Bearer ***" (scheme kept for humans); anything else becomes "***".
func shieldValue(value string) string {
	if i := strings.Index(value, " "); i > 0 {
		return value[:i+1] + "***"
	}
	return "***"
}

// ResolveCredentialHeader looks up the named credential in the keychain
// (service muster-cli — the same service `auth add` writes) and formats it
// for attachment to an outgoing request. The credential type comes from the
// credentials.json metadata in the CLI config dir. It returns an error when
// the credential name is unknown or the keychain is unavailable, and never
// echoes the secret in any error message.
func ResolveCredentialHeader(kc Keychain, name string) (CredentialHeader, error) {
	return ResolveCredentialHeaderFrom(kc, CredentialConfigDir(), name)
}

// ResolveCredentialHeaderFrom is ResolveCredentialHeader with an explicit
// metadata directory (tests, alternate profiles).
func ResolveCredentialHeaderFrom(kc Keychain, dir, name string) (CredentialHeader, error) {
	if strings.TrimSpace(name) == "" {
		return CredentialHeader{}, fmt.Errorf("credential name is empty")
	}
	secret, err := kc.Get(credentialServiceName, name)
	if err != nil {
		if err == ErrNotFound {
			return CredentialHeader{}, fmt.Errorf("credential %q not found: add it first with 'auth add --name %s --type <type> --value <secret>'", name, name)
		}
		return CredentialHeader{}, fmt.Errorf("read credential %q: %w", name, err)
	}

	credType := LoadCredentialType(dir, name)
	header := credentialHeaderName(credType)
	value := credentialHeaderValue(credType, secret)
	return CredentialHeader{
		Name:     header,
		Value:    value,
		Shielded: shieldValue(value),
	}, nil
}

// ResolveStored resolves the named credential using the platform keychain
// and the CLI config dir — the convenience entry point for command
// execution paths (request command, generated and reloaded typed commands).
func ResolveStored(name string) (CredentialHeader, error) {
	kc, err := NewKeychain()
	if err != nil {
		return CredentialHeader{}, fmt.Errorf("init keychain: %w", err)
	}
	return ResolveCredentialHeader(kc, name)
}

// AttachCredential sets the resolved credential header on the request.
// Because http.Header.Set replaces any prior value, callers that honor
// explicit user headers must resolve/attach BEFORE applying -H headers so
// an explicit manual header keeps precedence over the stored credential.
func AttachCredential(req *http.Request, cred CredentialHeader) {
	if req == nil || cred.Name == "" {
		return
	}
	req.Header.Set(cred.Name, cred.Value)
}
