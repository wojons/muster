package auth

import (
	"sync"
	"time"
)

// MockKeychain is a thread-safe in-memory implementation of the Keychain interface for testing
type MockKeychain struct {
	mu       sync.RWMutex
	data     map[string]map[string]string
	lastErr  error
	failNext bool
}

// NewMockKeychain creates a new MockKeychain instance
func NewMockKeychain() *MockKeychain {
	return &MockKeychain{
		data: make(map[string]map[string]string),
	}
}

// Set stores a password in the mock keychain
func (m *MockKeychain) Set(service, account, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failNext {
		m.failNext = false
		return m.lastErr
	}

	if m.data[service] == nil {
		m.data[service] = make(map[string]string)
	}
	m.data[service][account] = password
	return nil
}

// Get retrieves a password from the mock keychain
func (m *MockKeychain) Get(service, account string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.failNext {
		m.failNext = false
		return "", m.lastErr
	}

	if accounts, ok := m.data[service]; ok {
		if password, ok := accounts[account]; ok {
			return password, nil
		}
	}
	return "", ErrNotFound
}

// Delete removes a password from the mock keychain
func (m *MockKeychain) Delete(service, account string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failNext {
		m.failNext = false
		return m.lastErr
	}

	if accounts, ok := m.data[service]; ok {
		delete(accounts, account)
	}
	return nil
}

// List returns all accounts for a service in the mock keychain
func (m *MockKeychain) List(service string) ([]Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.failNext {
		m.failNext = false
		return nil, m.lastErr
	}

	var accounts []Account
	if accountsMap, ok := m.data[service]; ok {
		for account := range accountsMap {
			accounts = append(accounts, Account{
				Account: account,
				Created: time.Now(),
			})
		}
	}
	return accounts, nil
}

// SetFailNext configures the mock to fail the next operation with the given error
func (m *MockKeychain) SetFailNext(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastErr = err
	m.failNext = true
}

// Clear removes all data from the mock keychain
func (m *MockKeychain) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make(map[string]map[string]string)
}

// Count returns the number of stored credentials
func (m *MockKeychain) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, accounts := range m.data {
		count += len(accounts)
	}
	return count
}

// Has checks if a service/account exists
func (m *MockKeychain) Has(service, account string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if accounts, ok := m.data[service]; ok {
		_, exists := accounts[account]
		return exists
	}
	return false
}

// MockMacOSKeychain wraps MockKeychain to simulate MacOSKeychain behavior
type MockMacOSKeychain struct {
	mock *MockKeychain
}

// NewMockMacOSKeychain creates a new MockMacOSKeychain for testing MacOS keychain code
func NewMockMacOSKeychain() *MockMacOSKeychain {
	return &MockMacOSKeychain{
		mock: NewMockKeychain(),
	}
}

// Set stores a password
func (m *MockMacOSKeychain) Set(service, account, password string) error {
	return m.mock.Set(service, account, password)
}

// Get retrieves a password
func (m *MockMacOSKeychain) Get(service, account string) (string, error) {
	return m.mock.Get(service, account)
}

// Delete removes a password
func (m *MockMacOSKeychain) Delete(service, account string) error {
	return m.mock.Delete(service, account)
}

// List returns all accounts for a service
func (m *MockMacOSKeychain) List(service string) ([]Account, error) {
	return m.mock.List(service)
}
