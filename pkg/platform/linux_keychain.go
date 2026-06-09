package platform

// LinuxKeychain is the Linux platform implementation of secure credential storage.
// On Linux, it delegates to the Secret Service D-Bus API (GNOME Keyring / KDE Wallet).
// The real D-Bus implementation is in secure_storage_linux.go.
type LinuxKeychain struct{}
