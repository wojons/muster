//go:build linux
// +build linux

package auth

import (
	"context"
	"fmt"

	"github.com/wojons/muster/internal/platform/linux"
)

type SecretServiceKeychain struct {
	keyring *linux.SecretServiceKeyring
}

func NewSecretServiceKeychain() (*SecretServiceKeychain, error) {
	keyring, err := linux.NewSecretServiceKeyring()
	if err != nil {
		return nil, fmt.Errorf("failed to create secret service keyring: %w", err)
	}
	return &SecretServiceKeychain{keyring: keyring}, nil
}

func (s *SecretServiceKeychain) Set(service, account string, data []byte) error {
	key := fmt.Sprintf("%s:%s", service, account)
	cred := &linux.Credential{
		Type:     "password",
		Value:    string(data),
		Username: account,
	}
	return s.keyring.Set(context.Background(), key, cred)
}

func (s *SecretServiceKeychain) Get(service, account string) ([]byte, error) {
	key := fmt.Sprintf("%s:%s", service, account)
	cred, err := s.keyring.Get(context.Background(), key)
	if err != nil {
		return nil, err
	}
	return []byte(cred.Value), nil
}

func (s *SecretServiceKeychain) Delete(service, account string) error {
	key := fmt.Sprintf("%s:%s", service, account)
	return s.keyring.Delete(context.Background(), key)
}

func (s *SecretServiceKeychain) List(service string) ([]Account, error) {
	return nil, fmt.Errorf("list operation not supported by Secret Service")
}
