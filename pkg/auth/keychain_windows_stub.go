//go:build !windows
// +build !windows

package auth

// Stub types for non-Windows platforms

type windowsKeychainNative struct{}

func NewWindowsKeychainNative() *windowsKeychainNative {
	return nil
}

func (k *windowsKeychainNative) Set(service, account, password string) error {
	return nil
}

func (k *windowsKeychainNative) Get(service, account string) (string, error) {
	return "", nil
}

func (k *windowsKeychainNative) Delete(service, account string) error {
	return nil
}

func (k *windowsKeychainNative) List(service string) ([]Account, error) {
	return nil, nil
}
