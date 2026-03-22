package wasm

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SandboxedFS provides sandboxed filesystem access.
type SandboxedFS struct {
	mu             sync.RWMutex
	allowedDirs    map[string]bool
	readOnlyDirs   map[string]bool
	virtualFiles   map[string][]byte
	restrictAccess bool
	root           string
	networkPolicy  *NetworkPolicy
}

// NewSandboxedFS creates a new sandboxed filesystem.
func NewSandboxedFS(root string) *SandboxedFS {
	absRoot, _ := filepath.Abs(root)
	return &SandboxedFS{
		allowedDirs:  make(map[string]bool),
		readOnlyDirs: make(map[string]bool),
		virtualFiles: make(map[string][]byte),
		root:         absRoot,
	}
}

// AllowDirectory adds an allowed directory.
func (fs *SandboxedFS) AllowDirectory(dir string) {
	absPath, _ := filepath.Abs(dir)
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.allowedDirs[absPath] = true
	fs.restrictAccess = true
}

// AllowSubdirectory allows all subdirectories.
func (fs *SandboxedFS) AllowSubdirectory(baseDir string) {
	absBase, _ := filepath.Abs(baseDir)
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.allowedDirs[absBase] = true

	_ = filepath.Walk(absBase, func(path string, info os.FileInfo, err error) error {
		if info != nil && info.IsDir() {
			fs.allowedDirs[path] = true
		}
		return nil
	})
}

// SetReadOnly marks a directory as read-only.
func (fs *SandboxedFS) SetReadOnly(dir string) {
	absPath, _ := filepath.Abs(dir)
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.readOnlyDirs[absPath] = true
}

// Disallow removes a directory from allowed list.
func (fs *SandboxedFS) Disallow(dir string) {
	absPath, _ := filepath.Abs(dir)
	fs.mu.Lock()
	defer fs.mu.Unlock()

	absPath = filepath.Clean(absPath)
	delete(fs.allowedDirs, absPath)
	delete(fs.readOnlyDirs, absPath)
}

// SetVirtualFile sets a virtual file in memory.
func (fs *SandboxedFS) SetVirtualFile(path string, data []byte) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.virtualFiles[path] = data
}

// GetVirtualFile retrieves a virtual file.
func (fs *SandboxedFS) GetVirtualFile(path string) []byte {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.virtualFiles[path]
}

// Open opens a file for reading.
func (fs *SandboxedFS) Open(path string) (*os.File, error) {
	cleanPath, err := fs.normalizePath(path)
	if err != nil {
		return nil, fmt.Errorf("path normalization failed: %w", err)
	}

	if !fs.isPathAllowed(cleanPath) {
		return nil, fmt.Errorf("path not allowed: %s", cleanPath)
	}

	if fs.isReadOnly(cleanPath) {
		return nil, fmt.Errorf("path is read-only: %s", cleanPath)
	}

	if data, ok := fs.getVirtual(cleanPath); ok {
		tmpFile, err := os.CreateTemp("", "wasm-sandbox-*.tmp")
		if err != nil {
			return nil, err
		}
		if _, err := tmpFile.Write(data); err != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpFile.Name())
			return nil, err
		}
		_, _ = tmpFile.Seek(0, 0)
		return tmpFile, nil
	}

	absPath := filepath.Join(fs.root, cleanPath)
	return os.Open(absPath)
}

// Create creates a file for writing.
func (fs *SandboxedFS) Create(path string) (*os.File, error) {
	cleanPath, err := fs.normalizePath(path)
	if err != nil {
		return nil, fmt.Errorf("path normalization failed: %w", err)
	}

	if !fs.isPathAllowed(cleanPath) {
		return nil, fmt.Errorf("path not allowed: %s", cleanPath)
	}

	if fs.isReadOnly(cleanPath) {
		return nil, fmt.Errorf("path is read-only: %s", cleanPath)
	}

	absPath := filepath.Join(fs.root, cleanPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return nil, err
	}

	return os.Create(absPath)
}

// Stat returns file information.
func (fs *SandboxedFS) Stat(path string) (os.FileInfo, error) {
	cleanPath, err := fs.normalizePath(path)
	if err != nil {
		return nil, fmt.Errorf("path normalization failed: %w", err)
	}

	if !fs.isPathAllowed(cleanPath) {
		return nil, fmt.Errorf("path not allowed: %s", cleanPath)
	}

	if data, ok := fs.getVirtual(cleanPath); ok {
		return &virtualFileInfo{name: filepath.Base(cleanPath), size: int64(len(data))}, nil
	}

	absPath := filepath.Join(fs.root, cleanPath)
	return os.Stat(absPath)
}

// Remove removes a file.
func (fs *SandboxedFS) Remove(path string) error {
	cleanPath, err := fs.normalizePath(path)
	if err != nil {
		return fmt.Errorf("path normalization failed: %w", err)
	}

	if !fs.isPathAllowed(cleanPath) {
		return fmt.Errorf("path not allowed: %s", cleanPath)
	}

	if fs.isReadOnly(cleanPath) {
		return fmt.Errorf("path is read-only: %s", cleanPath)
	}

	absPath := filepath.Join(fs.root, cleanPath)
	return os.Remove(absPath)
}

// ReadDir reads a directory.
func (fs *SandboxedFS) ReadDir(path string) ([]os.FileInfo, error) {
	cleanPath, err := fs.normalizePath(path)
	if err != nil {
		return nil, fmt.Errorf("path normalization failed: %w", err)
	}

	if !fs.isPathAllowed(cleanPath) {
		return nil, fmt.Errorf("path not allowed: %s", cleanPath)
	}

	absPath := filepath.Join(fs.root, cleanPath)
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, err
	}

	var infos []os.FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		infos = append(infos, info)
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name() < infos[j].Name()
	})

	return infos, nil
}

// normalizePath normalizes a file path.
func (fs *SandboxedFS) normalizePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		relPath, err := filepath.Rel(fs.root, path)
		if err != nil {
			return "", fmt.Errorf("failed to convert to relative path: %w", err)
		}
		return filepath.Clean(relPath), nil
	}

	cleanPath := filepath.Clean(path)

	if strings.HasPrefix(cleanPath, "..") {
		return "", fmt.Errorf("path escapes root: %s", cleanPath)
	}

	return cleanPath, nil
}

// isPathAllowed checks if a path is allowed.
func (fs *SandboxedFS) isPathAllowed(path string) bool {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	if fs.restrictAccess {
		absPath := filepath.Join(fs.root, path)
		for allowedDir := range fs.allowedDirs {
			if strings.HasPrefix(absPath, allowedDir+"/") || absPath == allowedDir {
				return true
			}
		}

		_, ok := fs.virtualFiles[path]
		return ok
	}

	return true
}

// isReadOnly checks if a path is read-only.
func (fs *SandboxedFS) isReadOnly(path string) bool {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	for roDir := range fs.readOnlyDirs {
		if strings.HasPrefix(path, roDir) || path == roDir {
			return true
		}
	}

	return false
}

// getVirtual retrieves a virtual file.
func (fs *SandboxedFS) getVirtual(path string) ([]byte, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	data, ok := fs.virtualFiles[path]
	return data, ok
}

// virtualFileInfo implements os.FileInfo for virtual files.
type virtualFileInfo struct {
	name string
	size int64
	mode os.FileMode
}

func (vfi *virtualFileInfo) Name() string       { return vfi.name }
func (vfi *virtualFileInfo) Size() int64        { return vfi.size }
func (vfi *virtualFileInfo) Mode() os.FileMode  { return vfi.mode }
func (vfi *virtualFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (vfi *virtualFileInfo) IsDir() bool        { return false }
func (vfi *virtualFileInfo) Sys() interface{}   { return nil }

// SetNetworkPolicy sets the network policy for sandboxed network access.
func (fs *SandboxedFS) SetNetworkPolicy(policy *NetworkPolicy) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.networkPolicy = policy
}

// GetNetworkPolicy returns the current network policy.
func (fs *SandboxedFS) GetNetworkPolicy() *NetworkPolicy {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	if fs.networkPolicy == nil {
		return nil
	}
	// Return a copy to prevent external modification
	policyCopy := &NetworkPolicy{
		AllowedHosts: make([]string, len(fs.networkPolicy.AllowedHosts)),
		AllowedPorts: make([]int, len(fs.networkPolicy.AllowedPorts)),
		BlockedHosts: make([]string, len(fs.networkPolicy.BlockedHosts)),
	}
	copy(policyCopy.AllowedHosts, fs.networkPolicy.AllowedHosts)
	copy(policyCopy.AllowedPorts, fs.networkPolicy.AllowedPorts)
	copy(policyCopy.BlockedHosts, fs.networkPolicy.BlockedHosts)
	return policyCopy
}

// CheckNetworkAccess verifies if network access to a host:port is allowed.
// Returns true if no policy is set (permissive default) or if the policy allows the access.
func (fs *SandboxedFS) CheckNetworkAccess(host string, port int) bool {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	if fs.networkPolicy == nil {
		return true // Permissive default when no policy is set
	}

	return fs.networkPolicy.IsAllowed(host, port)
}
