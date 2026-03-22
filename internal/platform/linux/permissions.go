package linux

import (
	"fmt"
	"os"
	"syscall"
)

// SecureFileMode is the recommended permission for sensitive files (0600)
const SecureFileMode os.FileMode = 0600

// SecureDirMode is the recommended permission for sensitive directories (0700)
const SecureDirMode os.FileMode = 0700

// StandardDirMode is the standard permission for regular directories (0755)
const StandardDirMode os.FileMode = 0755

// StandardFileMode is the standard permission for regular files (0644)
const StandardFileMode os.FileMode = 0644

// SecureFile creates a file with restricted permissions (0600)
func SecureFile(path string) (*os.File, error) {
	// Use O_EXCL to prevent race conditions
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, SecureFileMode)
	if err != nil {
		return nil, err
	}

	// Ensure permissions are correct (umask might have affected them)
	if err := os.Chmod(path, SecureFileMode); err != nil {
		f.Close()
		os.Remove(path)
		return nil, err
	}

	return f, nil
}

// SecureMkdirAll creates a directory with specified permissions
func SecureMkdirAll(path string, mode os.FileMode) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}

	// Ensure permissions are correct
	return os.Chmod(path, mode)
}

// IsSecure checks if a file has secure permissions (not world-readable)
func IsSecure(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	mode := info.Mode().Perm()
	// Check if world has any permissions
	if mode&0077 != 0 {
		return false, nil
	}

	return true, nil
}

// FixPermissions corrects file permissions to be secure
func FixPermissions(path string, isDir bool) error {
	var mode os.FileMode
	if isDir {
		mode = SecureDirMode
	} else {
		mode = SecureFileMode
	}

	return os.Chmod(path, mode)
}

// GetOwner returns the UID and GID of a file
func GetOwner(path string) (uid, gid int, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("unable to get file stats")
	}

	return int(stat.Uid), int(stat.Gid), nil
}

// IsOwnedByCurrentUser checks if file is owned by current user
func IsOwnedByCurrentUser(path string) (bool, error) {
	uid, _, err := GetOwner(path)
	if err != nil {
		return false, err
	}

	return uid == os.Getuid(), nil
}

// SecureTempFile creates a temporary file in a secure manner
func SecureTempFile(dir, pattern string) (*os.File, error) {
	if dir == "" {
		dir = os.TempDir()
	}

	// Create with restrictive permissions
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}

	if err := os.Chmod(f.Name(), SecureFileMode); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, err
	}

	return f, nil
}
