package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// CredentialMetaFileName is the credentials metadata file that `auth add`
// maintains (name → type map, e.g. {"authsvc":"bearer"}).
const CredentialMetaFileName = "credentials.json"

// credentialTypeUnknown is returned when the metadata file is missing or
// unreadable. The header mapping then falls back to the conventional
// X-API-Key default; a bearer credential stored by an older CLI version
// can override explicitly with -H.
func credentialTypeUnknown() string { return "" }

// LoadCredentialType returns the stored type for the named credential from
// the credentials.json metadata file in dir (usually the CLI config dir).
// Missing/invalid metadata yields "" — never an error, so attaching a
// credential to a request does not hard-fail on stale metadata.
func LoadCredentialType(dir, name string) string {
	if dir == "" || name == "" {
		return credentialTypeUnknown()
	}
	data, err := os.ReadFile(filepath.Join(dir, CredentialMetaFileName))
	if err != nil {
		return credentialTypeUnknown()
	}
	var creds map[string]string
	if err := json.Unmarshal(data, &creds); err != nil {
		return credentialTypeUnknown()
	}
	return creds[name]
}
