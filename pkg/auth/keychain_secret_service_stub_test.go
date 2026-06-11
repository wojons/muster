//go:build !linux
// +build !linux

package auth

import (
	"testing"
)

func TestSecretServiceKeychain_Fallback(t *testing.T) {
	kc, err := NewSecretServiceKeychain()
	if err == nil {
		t.Error("Expected NewSecretServiceKeychain to return error on this platform")
	}
	if err != ErrSecretServiceNotAvailable {
		t.Logf("Note: Got error %v (expected ErrSecretServiceNotAvailable on non-Linux)", err)
	}
	if kc != nil {
		t.Error("Expected nil SecretServiceKeychain on error")
	}
}
