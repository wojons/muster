//go:build !linux
// +build !linux

package auth

type SecretServiceKeychain struct{}

func NewSecretServiceKeychain() (*SecretServiceKeychain, error) {
	return nil, ErrSecretServiceNotAvailable
}

func (s *SecretServiceKeychain) Set(service, account string, data []byte) error {
	return ErrSecretServiceNotAvailable
}

func (s *SecretServiceKeychain) Get(service, account string) ([]byte, error) {
	return nil, ErrSecretServiceNotAvailable
}

func (s *SecretServiceKeychain) Delete(service, account string) error {
	return ErrSecretServiceNotAvailable
}

func (s *SecretServiceKeychain) List(service string) ([]Account, error) {
	return nil, ErrSecretServiceNotAvailable
}
