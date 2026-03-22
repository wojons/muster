//go:build linux
// +build linux

package linux

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"
)

const (
	secretServiceBusName      = "org.freedesktop.secrets"
	secretServicePath         = "/org/freedesktop/secrets"
	secretServiceInterface    = "org.freedesktop.secrets.Service"
	secretCollectionInterface = "org.freedesktop.secrets.Collection"
	secretItemInterface       = "org.freedesktop.secrets.Item"
	secretSessionInterface    = "org.freedesktop.secrets.Session"
)

var (
	ErrKeyringNotAvailable = errors.New("keyring not available")
	ErrKeyringLocked       = errors.New("keyring locked")
	ErrCredentialNotFound  = errors.New("credential not found")
)

// SecretServiceKeyring implements keyring using Linux Secret Service API
type SecretServiceKeyring struct {
	conn        *dbus.Conn
	service     dbus.BusObject
	collection  dbus.BusObject
	session     dbus.BusObject
	sessionPath dbus.ObjectPath
}

// Credential represents stored authentication credential
type Credential struct {
	Type     string            `json:"type"`
	Config   map[string]string `json:"config"`
	Value    string            `json:"value"`
	Username string            `json:"username,omitempty"`
	Password string            `json:"password,omitempty"`
	Token    string            `json:"token,omitempty"`
}

// NewSecretServiceKeyring creates a new Secret Service keyring instance
func NewSecretServiceKeyring() (*SecretServiceKeyring, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session bus: %w", err)
	}

	kr := &SecretServiceKeyring{
		conn:    conn,
		service: conn.Object(secretServiceBusName, secretServicePath),
	}

	// Open session for encryption
	if err := kr.openSession(); err != nil {
		conn.Close()
		return nil, err
	}

	// Get default collection
	if err := kr.getDefaultCollection(); err != nil {
		kr.Close()
		return nil, err
	}

	return kr, nil
}

func (kr *SecretServiceKeyring) openSession() error {
	var sessionPath dbus.ObjectPath
	err := kr.service.Call(secretServiceInterface+".OpenSession", 0,
		"plain", dbus.MakeVariant("")).Store(&sessionPath)
	if err != nil {
		return fmt.Errorf("failed to open session: %w", err)
	}

	kr.sessionPath = sessionPath
	kr.session = kr.conn.Object(secretServiceBusName, sessionPath)
	return nil
}

func (kr *SecretServiceKeyring) getDefaultCollection() error {
	var collectionPath dbus.ObjectPath
	err := kr.service.Call(secretServiceInterface+".ReadAlias", 0,
		"default").Store(&collectionPath)
	if err != nil {
		return fmt.Errorf("failed to get default collection: %w", err)
	}

	if collectionPath == "/" {
		// No default collection, use session collection
		collectionPath = "/org/freedesktop/secrets/collection/session"
	}

	kr.collection = kr.conn.Object(secretServiceBusName, collectionPath)
	return nil
}

// Get retrieves a credential from the keyring
func (kr *SecretServiceKeyring) Get(ctx context.Context, key string) (*Credential, error) {
	attributes := map[string]string{
		"service": "muster",
		"key":     key,
	}

	var unlocked []dbus.ObjectPath
	var locked []dbus.ObjectPath
	err := kr.collection.Call(secretCollectionInterface+".SearchItems", 0,
		attributes).Store(&unlocked, &locked)
	if err != nil {
		return nil, fmt.Errorf("failed to search items: %w", err)
	}

	if len(unlocked) == 0 {
		if len(locked) > 0 {
			return nil, ErrKeyringLocked
		}
		return nil, ErrCredentialNotFound
	}

	// Get first unlocked item
	item := kr.conn.Object(secretServiceBusName, unlocked[0])

	var secret []interface{}
	err = item.Call(secretItemInterface+".GetSecret", 0, kr.sessionPath).Store(&secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	// Decode secret value
	secretValue, ok := secret[2].([]byte)
	if !ok {
		return nil, errors.New("invalid secret format")
	}

	var cred Credential
	if err := json.Unmarshal(secretValue, &cred); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credential: %w", err)
	}

	return &cred, nil
}

// Set stores a credential in the keyring
func (kr *SecretServiceKeyring) Set(ctx context.Context, key string, cred *Credential) error {
	data, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("failed to marshal credential: %w", err)
	}

	secret := []interface{}{
		"",              // session path (not used with plain encryption)
		make([]byte, 0), // parameters
		data,            // value
		"text/plain",    // content type
	}

	properties := map[string]dbus.Variant{
		"org.freedesktop.Secret.Item.Label": dbus.MakeVariant(fmt.Sprintf("muster: %s", key)),
		"org.freedesktop.Secret.Item.Attributes": dbus.MakeVariant(map[string]string{
			"service": "muster",
			"key":     key,
		}),
	}

	var itemPath dbus.ObjectPath
	var promptPath dbus.ObjectPath
	err = kr.collection.Call(secretCollectionInterface+".CreateItem", 0,
		properties, secret, true).Store(&itemPath, &promptPath)
	if err != nil {
		return fmt.Errorf("failed to create item: %w", err)
	}

	// Handle prompt if required (for locked collections)
	if promptPath != "/" {
		return kr.handlePrompt(ctx, promptPath)
	}

	return nil
}

// Delete removes a credential from the keyring
func (kr *SecretServiceKeyring) Delete(ctx context.Context, key string) error {
	attributes := map[string]string{
		"service": "muster",
		"key":     key,
	}

	var unlocked []dbus.ObjectPath
	var locked []dbus.ObjectPath
	err := kr.collection.Call(secretCollectionInterface+".SearchItems", 0,
		attributes).Store(&unlocked, &locked)
	if err != nil {
		return fmt.Errorf("failed to search items: %w", err)
	}

	if len(unlocked) == 0 {
		return ErrCredentialNotFound
	}

	// Delete first matching item
	item := kr.conn.Object(secretServiceBusName, unlocked[0])
	err = item.Call(secretItemInterface+".Delete", 0).Store()
	if err != nil {
		return fmt.Errorf("failed to delete item: %w", err)
	}

	return nil
}

func (kr *SecretServiceKeyring) handlePrompt(ctx context.Context, promptPath dbus.ObjectPath) error {
	prompt := kr.conn.Object(secretServiceBusName, promptPath)

	// Connect to prompt completed signal
	sigChan := make(chan *dbus.Signal, 1)
	kr.conn.Signal(sigChan)
	defer kr.conn.RemoveSignal(sigChan)

	matchRule := fmt.Sprintf("type='signal',path='%s',interface='%s',member='Completed'",
		promptPath, "org.freedesktop.Secret.Prompt")
	kr.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule)

	// Trigger prompt
	var promptResult dbus.ObjectPath
	err := prompt.Call("org.freedesktop.Secret.Prompt.Prompt", 0, "").Store(&promptResult)
	if err != nil {
		return err
	}

	// Wait for completion
	select {
	case sig := <-sigChan:
		if len(sig.Body) < 2 {
			return errors.New("invalid prompt response")
		}
		dismissed := sig.Body[0].(bool)
		if dismissed {
			return errors.New("keyring operation cancelled")
		}
		return nil
	case <-ctx.Done():
		prompt.Call("org.freedesktop.Secret.Prompt.Dismiss", 0)
		return ctx.Err()
	}
}

// Close releases resources
func (kr *SecretServiceKeyring) Close() error {
	if kr.session != nil {
		kr.session.Call(secretSessionInterface+".Close", 0)
	}
	if kr.conn != nil {
		return kr.conn.Close()
	}
	return nil
}
