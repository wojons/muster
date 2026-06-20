// Package linux provides Linux-specific platform implementations.
// This file extends the keyring_secret_service.go with Credential type
// and context-based methods needed by pkg/auth.
package linux

import (
	"context"
	"fmt"
)

// Credential represents a stored credential in the keyring.
type Credential struct {
	Type     string
	Value    string
	Username string
}

// Set stores a credential in the keyring under the given key.
func (k *SecretServiceKeyring) Set(ctx context.Context, key string, cred *Credential) error {
	if !k.connected {
		return ErrNotConnected
	}
	return k.SetSecret(key, []byte(cred.Value))
}

// Get retrieves a credential from the keyring by key.
func (k *SecretServiceKeyring) Get(ctx context.Context, key string) (*Credential, error) {
	if !k.connected {
		return nil, ErrNotConnected
	}
	data, err := k.GetSecret(key)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCredentialNotFound, err)
	}
	return &Credential{
		Type:  "password",
		Value: string(data),
	}, nil
}

// Delete removes a credential from the keyring by key.
func (k *SecretServiceKeyring) Delete(ctx context.Context, key string) error {
	if !k.connected {
		return ErrNotConnected
	}
	return k.DeleteSecret(key)
}

// ErrNotConnected is returned when the keyring is not connected.
var ErrNotConnected = fmt.Errorf("secret service not connected")

// ErrCredentialNotFound is returned when a credential lookup fails.
var ErrCredentialNotFound = fmt.Errorf("credential not found")
