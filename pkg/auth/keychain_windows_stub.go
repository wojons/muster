//go:build !windows
// +build !windows

package auth

// windowsKeychainNative is a stub keychain implementation used when the
// native Windows Credential Manager is not available. On Windows, a real
// implementation in keychain_windows.go overrides this stub.
type windowsKeychainNative struct{}

// NewWindowsKeychainNative creates a new Windows keychain native client.
func NewWindowsKeychainNative() (*windowsKeychainNative, error) {
	return &windowsKeychainNative{}, nil
}

// Set stores a credential. Stub implementation returns ErrSecretServiceNotAvailable.
func (w *windowsKeychainNative) Set(service, account, password string) error {
	return ErrSecretServiceNotAvailable
}

// Get retrieves a credential. Stub implementation returns ErrSecretServiceNotAvailable.
func (w *windowsKeychainNative) Get(service, account string) (string, error) {
	return "", ErrSecretServiceNotAvailable
}

// Delete removes a credential. Stub implementation returns ErrSecretServiceNotAvailable.
func (w *windowsKeychainNative) Delete(service, account string) error {
	return ErrSecretServiceNotAvailable
}

// List returns stored accounts. Stub implementation returns nil.
func (w *windowsKeychainNative) List(service string) ([]Account, error) {
	return nil, nil
}
