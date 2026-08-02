// Package linux provides Linux-specific platform implementations
// for secret service keyring integration via D-Bus.
package linux

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

// SecretServiceKeyring provides access to the Linux Secret Service API
// via D-Bus for secure credential storage.
type SecretServiceKeyring struct {
	conn        *dbus.Conn
	service     dbus.BusObject
	sessionPath dbus.ObjectPath
	collection  dbus.ObjectPath
	connected   bool
}

// NewSecretServiceKeyring creates a new Secret Service keyring client.
// It does not connect — call Connect() to establish the D-Bus session.
func NewSecretServiceKeyring() (*SecretServiceKeyring, error) {
	return &SecretServiceKeyring{}, nil
}

// Connect establishes a connection to the Secret Service via D-Bus.
// It connects to the session bus, opens a plain (unencrypted) session,
// creates (or reuses) the "Muster Keyring" collection, and stores the
// collection path for subsequent operations.
func (k *SecretServiceKeyring) Connect() error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("dbus session bus: %w", err)
	}

	service := conn.Object("org.freedesktop.secrets", "/org/freedesktop/secrets")

	// Open a plain (unencrypted) session. The plain algorithm
	// requires no negotiation — the server returns an empty output
	// and a session object path.
	var sessionOutput dbus.Variant
	var sessionPath dbus.ObjectPath
	err = service.Call(
		"org.freedesktop.Secret.Service.OpenSession",
		0,
		"plain",
		dbus.MakeVariant(""),
	).Store(&sessionOutput, &sessionPath)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("open session: %w", err)
	}

	// Always use a dedicated Muster collection — do NOT use the
	// default alias (the system Login keyring is locked in headless
	// environments, which breaks the fallback).
	collectionPath, err := k.findOrCreateCollection(conn, service)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("collection: %w", err)
	}

	k.conn = conn
	k.service = service
	k.sessionPath = sessionPath
	k.collection = collectionPath
	k.connected = true
	return nil
}

// findOrCreateCollection locates an existing "Muster Keyring" collection
// by checking the default alias (gnome-keyring only supports "default"),
// or creates one if it doesn't exist.
func (k *SecretServiceKeyring) findOrCreateCollection(conn *dbus.Conn, service dbus.BusObject) (dbus.ObjectPath, error) {
	// Try the default alias first
	var collectionPath dbus.ObjectPath
	err := service.Call(
		"org.freedesktop.Secret.Service.ReadAlias",
		0,
		"default",
	).Store(&collectionPath)
	if err == nil && collectionPath != "/" && collectionPath != "" {
		// Check if this collection is unlocked
		if k.isCollectionUnlocked(conn, collectionPath) {
			return collectionPath, nil
		}
	}

	// Create a new collection (gnome-keyring only supports "" or "default" alias)
	props := map[string]dbus.Variant{
		"org.freedesktop.Secret.Collection.Label": dbus.MakeVariant("Muster Keyring"),
	}
	var promptPath dbus.ObjectPath
	err = service.Call(
		"org.freedesktop.Secret.Service.CreateCollection",
		0,
		props,
		"", // no alias — track the path ourselves
	).Store(&collectionPath, &promptPath)
	if err != nil {
		return "", err
	}
	// If a prompt was returned, the collection wasn't created yet
	// (gnome-keyring needs desktop interaction to unlock).
	// Dismiss it and fail — the FileKeychain fallback will handle it.
	if promptPath != "/" {
		k.dismissPrompt(conn, promptPath)
		return "", fmt.Errorf("collection creation requires unlock prompt — use file keychain fallback")
	}
	// Sanity check: collection path must be a real object path.
	if collectionPath == "/" || collectionPath == "" {
		return "", fmt.Errorf("collection creation failed — empty path returned")
	}
	return collectionPath, nil
}

// isCollectionUnlocked checks whether a collection is unlocked.
func (k *SecretServiceKeyring) isCollectionUnlocked(conn *dbus.Conn, path dbus.ObjectPath) bool {
	obj := conn.Object("org.freedesktop.secrets", path)
	var locked bool
	err := obj.Call(
		"org.freedesktop.DBus.Properties.Get",
		0,
		"org.freedesktop.Secret.Collection",
		"Locked",
	).Store(&locked)
	if err != nil {
		return false
	}
	return !locked
}

// dismissPrompt dismisses a D-Bus Secret Service prompt.
func (k *SecretServiceKeyring) dismissPrompt(conn *dbus.Conn, promptPath dbus.ObjectPath) {
	prompt := conn.Object("org.freedesktop.secrets", promptPath)
	_ = prompt.Call("org.freedesktop.Secret.Prompt.Dismiss", 0).Store()
}

// Disconnect closes the D-Bus connection and clears state.
func (k *SecretServiceKeyring) Disconnect() error {
	if k.conn != nil {
		_ = k.conn.Close()
	}
	k.conn = nil
	k.service = nil
	k.sessionPath = ""
	k.collection = ""
	k.connected = false
	return nil
}

// IsConnected returns whether the keyring is connected.
func (k *SecretServiceKeyring) IsConnected() bool {
	return k.connected
}

// GetSecret retrieves a secret by its key identifier.
// It searches the collection for an item with attribute "muster-key" = key,
// then decrypts the secret using the session and returns the raw value.
func (k *SecretServiceKeyring) GetSecret(key string) ([]byte, error) {
	if !k.connected {
		return nil, fmt.Errorf("secret service not connected")
	}

	itemPath, err := k.searchItem(key)
	if err != nil {
		return nil, err
	}
	if itemPath == "" {
		return nil, fmt.Errorf("secret %q not found in keyring", key)
	}

	item := k.conn.Object("org.freedesktop.secrets", itemPath)

	// GetSecret returns a struct with signature (oayays):
	// session, parameters, value, content_type
	var secret struct {
		Session     dbus.ObjectPath
		Parameters  []byte
		Value       []byte
		ContentType string
	}
	err = item.Call(
		"org.freedesktop.Secret.Item.GetSecret",
		0,
		k.sessionPath,
	).Store(&secret)
	if err != nil {
		return nil, fmt.Errorf("get secret: %w", err)
	}

	return secret.Value, nil
}

// SetSecret stores a secret with the given key identifier.
// It creates a new item in the collection with the "muster-key" attribute
// and the secret value encrypted with the session.
func (k *SecretServiceKeyring) SetSecret(key string, data []byte) error {
	if !k.connected {
		return fmt.Errorf("secret service not connected")
	}

	collection := k.conn.Object("org.freedesktop.secrets", k.collection)

	props := map[string]dbus.Variant{
		"org.freedesktop.Secret.Item.Label": dbus.MakeVariant(
			fmt.Sprintf("Muster: %s", key),
		),
		"org.freedesktop.Secret.Item.Attributes": dbus.MakeVariant(
			map[string]string{"muster-key": key},
		),
	}

	// Secret struct: (session, parameters, value, content_type)
	// For plain sessions, parameters is empty and value is the raw data.
	secret := struct {
		Session     dbus.ObjectPath
		Parameters  []byte
		Value       []byte
		ContentType string
	}{
		Session:     k.sessionPath,
		Parameters:  []byte{},
		Value:       data,
		ContentType: "text/plain; charset=utf8",
	}

	var itemPath dbus.ObjectPath
	var promptPath dbus.ObjectPath
	err := collection.Call(
		"org.freedesktop.Secret.Collection.CreateItem",
		0,
		props,
		secret,
		true, // replace if exists
	).Store(&itemPath, &promptPath)
	if err != nil {
		return fmt.Errorf("create item: %w", err)
	}

	// Dismiss any prompt (e.g., unlock collection).
	if promptPath != "/" {
		k.dismissPrompt(k.conn, promptPath)
	}

	return nil
}

// DeleteSecret removes a secret by its key identifier.
func (k *SecretServiceKeyring) DeleteSecret(key string) error {
	if !k.connected {
		return fmt.Errorf("secret service not connected")
	}

	itemPath, err := k.searchItem(key)
	if err != nil {
		return err
	}
	if itemPath == "" {
		// Not found — nothing to delete.
		return nil
	}

	item := k.conn.Object("org.freedesktop.secrets", itemPath)
	var promptPath dbus.ObjectPath
	err = item.Call(
		"org.freedesktop.Secret.Item.Delete",
		0,
	).Store(&promptPath)
	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}

	if promptPath != "/" {
		k.dismissPrompt(k.conn, promptPath)
	}

	return nil
}

// searchItem finds an item in the collection with attribute "muster-key" = key.
// Returns the item's object path, or empty string if not found.
func (k *SecretServiceKeyring) searchItem(key string) (dbus.ObjectPath, error) {
	collection := k.conn.Object("org.freedesktop.secrets", k.collection)

	attributes := map[string]string{"muster-key": key}
	var results []dbus.ObjectPath
	var locked []dbus.ObjectPath
	err := collection.Call(
		"org.freedesktop.Secret.Collection.SearchItems",
		0,
		attributes,
	).Store(&results, &locked)
	if err != nil {
		return "", fmt.Errorf("search items: %w", err)
	}

	if len(results) == 0 {
		return "", nil
	}

	return results[0], nil
}
