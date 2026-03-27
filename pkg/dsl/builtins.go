package dsl

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"crypto/tls"

	"go.starlark.net/starlark"
)

// MaxResponseBodySize is the maximum allowed HTTP response body size (10MB)
// to prevent DoS attacks via memory exhaustion
const MaxResponseBodySize = 10 * 1024 * 1024 // 10MB

// safeReadAll reads from r with a size limit and returns an error if the limit
// is exceeded or if the read operation fails
func safeReadAll(r io.Reader, limit int64) ([]byte, error) {
	limitedReader := io.LimitReader(r, limit+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response body exceeds maximum size of %d bytes", limit)
	}
	return data, nil
}

var (
	workspaceDir             string
	defaultWorkspaceDir      string
	disableNetworkValidation uint32
)

func init() {
	if dir, err := os.Getwd(); err == nil {
		defaultWorkspaceDir = dir
	}
}

var DefaultBuiltins = map[string]interface{}{
	"http_get":      builtinHTTPGet,
	"http_post":     builtinHTTPPost,
	"http_put":      builtinHTTPPut,
	"http_delete":   builtinHTTPDelete,
	"json_encode":   builtinJSONEncode,
	"json_decode":   builtinJSONDecode,
	"fail":          builtinFail,
	"print":         builtinPrint,
	"format":        builtinFormat,
	"env":           builtinEnv,
	"secret":        builtinSecret,
	"base64_encode": builtinBase64Encode,
	"base64_decode": builtinBase64Decode,
	"sha256":        builtinSHA256,
	"md5":           builtinMD5,
	"file_read":     builtinFileRead,
	"file_write":    builtinFileWrite,
	"file_exists":   builtinFileExists,
}

func builtinHTTPGet(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var urlStr string
	var headers, params *starlark.Dict
	var timeout = 30

	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"url", &urlStr,
		"headers?", &headers,
		"params?", &params,
		"timeout?", &timeout,
	); err != nil {
		return nil, err
	}

	validation, err := validateNetworkURLWithCache(urlStr)
	if err != nil {
		return nil, err
	}

	headerMap := make(http.Header)
	if headers != nil {
		for _, item := range headers.Items() {
			key, _ := starlark.AsString(item[0])
			value, _ := starlark.AsString(item[1])
			headerMap.Set(key, value)
		}
	}

	// Create HTTP client with DNS pinning to prevent rebinding attacks
	client := createHTTPClientWithDNSPin(validation, time.Duration(timeout)*time.Second)
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header = headerMap

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := safeReadAll(resp.Body, MaxResponseBodySize)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	responseDict := starlark.NewDict(3)
	_ = responseDict.SetKey(starlark.String("status_code"), starlark.MakeInt(resp.StatusCode))
	_ = responseDict.SetKey(starlark.String("body"), starlark.String(body))

	return responseDict, nil
}

func builtinHTTPPost(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var urlStr, body string
	var headers *starlark.Dict

	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"url", &urlStr,
		"body", &body,
		"headers?", &headers,
	); err != nil {
		return nil, err
	}

	validation, err := validateNetworkURLWithCache(urlStr)
	if err != nil {
		return nil, err
	}

	headerMap := make(http.Header)
	if headers != nil {
		for _, item := range headers.Items() {
			key, _ := starlark.AsString(item[0])
			value, _ := starlark.AsString(item[1])
			headerMap.Set(key, value)
		}
	}

	// Create HTTP client with DNS pinning to prevent rebinding attacks (30s timeout)
	client := createHTTPClientWithDNSPin(validation, 30*time.Second)
	req, err := http.NewRequest("POST", urlStr, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header = headerMap

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	resBody, err := safeReadAll(resp.Body, MaxResponseBodySize)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	responseDict := starlark.NewDict(2)
	_ = responseDict.SetKey(starlark.String("status_code"), starlark.MakeInt(resp.StatusCode))
	_ = responseDict.SetKey(starlark.String("body"), starlark.String(resBody))

	return responseDict, nil
}

func builtinHTTPPut(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var urlStr, body string
	var headers *starlark.Dict

	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"url", &urlStr,
		"body", &body,
		"headers?", &headers,
	); err != nil {
		return nil, err
	}

	validation, err := validateNetworkURLWithCache(urlStr)
	if err != nil {
		return nil, err
	}

	headerMap := make(http.Header)
	if headers != nil {
		for _, item := range headers.Items() {
			key, _ := starlark.AsString(item[0])
			value, _ := starlark.AsString(item[1])
			headerMap.Set(key, value)
		}
	}

	// Create HTTP client with DNS pinning to prevent rebinding attacks (30s timeout)
	client := createHTTPClientWithDNSPin(validation, 30*time.Second)
	req, err := http.NewRequest("PUT", urlStr, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header = headerMap

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	resBody, err := safeReadAll(resp.Body, MaxResponseBodySize)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	responseDict := starlark.NewDict(2)
	_ = responseDict.SetKey(starlark.String("status_code"), starlark.MakeInt(resp.StatusCode))
	_ = responseDict.SetKey(starlark.String("body"), starlark.String(resBody))

	return responseDict, nil
}

func builtinHTTPDelete(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var urlStr string
	var headers *starlark.Dict

	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"url", &urlStr,
		"headers?", &headers,
	); err != nil {
		return nil, err
	}

	validation, err := validateNetworkURLWithCache(urlStr)
	if err != nil {
		return nil, err
	}

	headerMap := make(http.Header)
	if headers != nil {
		for _, item := range headers.Items() {
			key, _ := starlark.AsString(item[0])
			value, _ := starlark.AsString(item[1])
			headerMap.Set(key, value)
		}
	}

	// Create HTTP client with DNS pinning to prevent rebinding attacks (30s timeout)
	client := createHTTPClientWithDNSPin(validation, 30*time.Second)
	req, err := http.NewRequest("DELETE", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header = headerMap

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	resBody, err := safeReadAll(resp.Body, MaxResponseBodySize)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	responseDict := starlark.NewDict(2)
	_ = responseDict.SetKey(starlark.String("status_code"), starlark.MakeInt(resp.StatusCode))
	_ = responseDict.SetKey(starlark.String("body"), starlark.String(resBody))

	return responseDict, nil
}

func builtinJSONEncode(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var value starlark.Value
	var indent = -1

	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"value", &value,
		"indent?", &indent,
	); err != nil {
		return nil, err
	}

	goValue, err := FromStarlark(value)
	if err != nil {
		return nil, err
	}

	var data []byte
	if indent >= 0 {
		data, err = json.MarshalIndent(goValue, "", strings.Repeat(" ", indent))
	} else {
		data, err = json.Marshal(goValue)
	}

	if err != nil {
		return nil, fmt.Errorf("json encode failed: %w", err)
	}

	return starlark.String(data), nil
}

func builtinJSONDecode(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var data string

	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "data", &data); err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil, fmt.Errorf("json decode failed: %w", err)
	}

	return ToStarlark(result)
}

func builtinFail(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "message", &msg); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("script failure: %s", msg)
}

func builtinPrint(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var sep, end string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"sep?", &sep,
		"end?", &end,
	); err != nil {
		return nil, err
	}
	if sep == "" {
		sep = " "
	}
	if end == "" {
		end = "\n"
	}

	var strs []string
	for _, arg := range args {
		str, ok := arg.(starlark.String)
		if !ok {
			return nil, fmt.Errorf("print argument must be string")
		}
		strs = append(strs, string(str))
	}

	fmt.Print(strings.Join(strs, sep) + end)
	return starlark.None, nil
}

func builtinFormat(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("format requires at least a template")
	}

	template, ok := args[0].(starlark.String)
	if !ok {
		return nil, fmt.Errorf("template must be string")
	}

	formatStr := string(template)
	var formatArgs []interface{}
	for i := 1; i < len(args); i++ {
		val, err := FromStarlark(args[i])
		if err != nil {
			return nil, err
		}
		formatArgs = append(formatArgs, val)
	}

	return starlark.String(fmt.Sprintf(formatStr, formatArgs...)), nil
}

func builtinEnv(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, defaultValue string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"name", &name,
		"default?", &defaultValue,
	); err != nil {
		return nil, err
	}

	value := os.Getenv(name)
	if value == "" {
		value = defaultValue
	}

	return starlark.String(value), nil
}

func builtinSecret(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "key", &key); err != nil {
		return nil, err
	}

	if key == "" {
		return nil, fmt.Errorf("secret key cannot be empty")
	}

	value := os.Getenv(key)
	if value == "" {
		value = os.Getenv("SECRET_" + key)
	}

	if value == "" {
		return nil, fmt.Errorf("secret not found for key '%s'", key)
	}

	return starlark.String(value), nil
}

func builtinBase64Encode(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var data string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "data", &data); err != nil {
		return nil, err
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(data))
	return starlark.String(encoded), nil
}

func builtinBase64Decode(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var data string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "data", &data); err != nil {
		return nil, err
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %w", err)
	}

	return starlark.String(decoded), nil
}

func builtinSHA256(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var data string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "data", &data); err != nil {
		return nil, err
	}

	hash := sha256.Sum256([]byte(data))
	return starlark.String(fmt.Sprintf("%x", hash)), nil
}

func builtinMD5(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var data string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "data", &data); err != nil {
		return nil, err
	}

	hash := md5.Sum([]byte(data))
	return starlark.String(fmt.Sprintf("%x", hash)), nil
}

func builtinFileRead(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}

	cleanPath, err := validateFilePath(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("file read failed: %w", err)
	}

	return starlark.String(data), nil
}

func builtinFileWrite(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path, content string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"path", &path,
		"content", &content,
	); err != nil {
		return nil, err
	}

	cleanPath, err := validateFilePath(path)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(cleanPath, []byte(content), 0600); err != nil {
		return nil, fmt.Errorf("file write failed: %w", err)
	}

	return starlark.None, nil
}

func builtinFileExists(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}

	cleanPath, err := validateFilePath(path)
	if err != nil {
		return nil, err
	}

	_, err = os.Stat(cleanPath)
	return starlark.Bool(err == nil), nil
}

func validateFilePath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}

	// First check if path is blocked before any transformations
	if isPathBlocked(absPath) {
		return "", fmt.Errorf("access to system file blocked: %s", path)
	}

	targetDir := workspaceDir
	if targetDir == "" {
		targetDir = defaultWorkspaceDir
	}

	// CRITICAL FIX: Ensure workspace is not root to prevent system-wide access
	if targetDir == "" || targetDir == "/" {
		return "", fmt.Errorf("workspace cannot be root directory")
	}

	targetDirAbs, err := filepath.Abs(targetDir)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}

	// CRITICAL FIX: Validate containment BEFORE resolving symlinks
	// This handles the case where both workspace and file are under a symlinked directory (e.g., /var -> /private/var on macOS)
	relPath, err := filepath.Rel(targetDirAbs, absPath)
	if err != nil {
		return "", fmt.Errorf("file outside workspace: %s", path)
	}

	if strings.HasPrefix(relPath, "..") {
		return "", fmt.Errorf("file outside workspace: %s", path)
	}

	// CRITICAL FIX: NOW check if the file is a symlink that escapes the workspace
	// This prevents TOCTOU attacks where a symlink inside workspace points outside
	fileInfo, statErr := os.Lstat(absPath)
	if statErr == nil {
		// File exists - check if it's a symlink
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			// It's a symlink - resolve it and re-validate
			resolvedPath, err := filepath.EvalSymlinks(absPath)
			if err != nil {
				return "", fmt.Errorf("resolve symlink: %w", err)
			}

			// CRITICAL: Resolve workspace too to avoid symlink path mismatches
			// (e.g., /var -> /private/var on macOS)
			resolvedWorkspace := targetDirAbs
			if resolved, err := filepath.EvalSymlinks(targetDirAbs); err == nil {
				resolvedWorkspace = resolved
			}

			// Re-check workspace containment for the resolved path using resolved workspace
			relResolved, err := filepath.Rel(resolvedWorkspace, resolvedPath)
			if err != nil || strings.HasPrefix(relResolved, "..") {
				return "", fmt.Errorf("file outside workspace: %s (symlink escape detected)", path)
			}

			// Return the resolved path for actual file operations
			return resolvedPath, nil
		}
	}
	// For regular files or non-existing files, return the absolute path
	return absPath, nil
}

// System file paths that should never be accessible
var blockedSystemPaths = []string{
	"/etc/passwd",
	"/etc/shadow",
	"/etc/hosts",
	"/etc/group",
	"/root",
	"/root/.ssh",
	"/root/.config",
	"/root/.gnupg",
}

// Network ranges that should never be accessible
var blockedNetworks = []string{
	"0.0.0.0/8",          // Current network (broadcast)
	"10.0.0.0/8",         // Private
	"127.0.0.0/8",        // Loopback
	"172.16.0.0/12",      // Private
	"192.168.0.0/16",     // Private
	"169.254.169.254/32", // AWS/GCP metadata service
}

func isPathBlocked(path string) bool {
	cleanPath := filepath.Clean(path)
	for _, blocked := range blockedSystemPaths {
		if strings.HasPrefix(cleanPath, blocked) {
			return true
		}
	}
	// Also check for common config directories in home directory
	if strings.Contains(cleanPath, "/.ssh/") || strings.Contains(cleanPath, "/.config/") || strings.Contains(cleanPath, "/.gnupg/") {
		return true
	}
	return false
}

func SetTestingNetworkValidationDisabled(disabled bool) bool {
	var oldDisabled bool
	if atomic.LoadUint32(&disableNetworkValidation) == 1 {
		oldDisabled = true
	}
	var newVal uint32
	if disabled {
		newVal = 1
	}
	atomic.StoreUint32(&disableNetworkValidation, newVal)
	return oldDisabled
}

// networkValidationResult holds the result of network validation including cached IP
type networkValidationResult struct {
	originalHost string
	cachedIP     net.IP
	port         string
	scheme       string
}

func validateNetworkURL(urlStr string) error {
	_, err := validateNetworkURLWithCache(urlStr)
	return err
}

func validateNetworkURLWithCache(urlStr string) (*networkValidationResult, error) {
	if atomic.LoadUint32(&disableNetworkValidation) == 1 {
		// Return a result with no cached IP when validation is disabled
		parsedURL, err := url.Parse(urlStr)
		if err != nil {
			return nil, fmt.Errorf("invalid URL: %w", err)
		}
		return &networkValidationResult{
			originalHost: parsedURL.Hostname(),
			port:         parsedURL.Port(),
			scheme:       parsedURL.Scheme,
		}, nil
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	host := parsedURL.Hostname()

	if host == "" {
		return nil, fmt.Errorf("URL has no hostname")
	}

	result := &networkValidationResult{
		originalHost: host,
		port:         parsedURL.Port(),
		scheme:       parsedURL.Scheme,
	}

	// CRITICAL FIX: Resolve DNS and cache IP to prevent DNS rebinding attacks
	// Check if it's an IP literal
	ip := net.ParseIP(host)
	if ip != nil {
		if err := validateIPAddress(ip); err != nil {
			return nil, err
		}
		result.cachedIP = ip
	} else {
		// It's a hostname - resolve it, check all IPs, and cache one for connection
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve hostname: %w", err)
		}

		// Check each resolved IP and cache the first valid one
		var validIP net.IP
		for _, ipAddr := range ips {
			if err := validateIPAddress(ipAddr.IP); err != nil {
				continue // Skip invalid IPs, try others
			}
			validIP = ipAddr.IP
			break
		}

		if validIP == nil {
			return nil, fmt.Errorf("no valid IP addresses for hostname %s", host)
		}
		result.cachedIP = validIP
	}

	// Check for .local domains (mDNS/Zeroconf)
	if strings.HasSuffix(host, ".local") || host == "localhost" {
		return nil, fmt.Errorf("access to localhost blocked: %s", host)
	}

	return result, nil
}

// createHTTPClientWithDNSPin creates an HTTP client that connects to a specific IP
// while preserving the original hostname in the Host header (prevents DNS rebinding)
func createHTTPClientWithDNSPin(validation *networkValidationResult, timeout time.Duration) *http.Client {
	if validation == nil || validation.cachedIP == nil {
		// No DNS pinning, use default client
		return &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
		}
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				// Set ServerName for SNI to match original hostname
				ServerName: validation.originalHost,
			},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				// Replace the resolved IP with our cached IP
				// Use cachedIP:port instead of the resolved addr
				dialAddr := net.JoinHostPort(validation.cachedIP.String(), validation.port)
				var d net.Dialer
				return d.DialContext(ctx, network, dialAddr)
			},
		},
	}
}

// validateIPAddress checks if an IP address is in a blocked network range
func validateIPAddress(ip net.IP) error {
	isBlocked := false

	// Check IPv4 networks
	for _, block := range blockedNetworks {
		_, network, _ := net.ParseCIDR(block)
		if network != nil && network.Contains(ip) {
			isBlocked = true
			break
		}
	}

	if isBlocked {
		return fmt.Errorf("access to blocked network: %s", ip)
	}

	// Additional IPv6 private network checks
	if ip.To4() == nil { // IPv6
		isIPv6Localhost := ip.IsLoopback()
		isIPv6UniqueLocal := strings.HasPrefix(ip.String(), "fd") || // Unique local (fc00::/7)
			strings.HasPrefix(ip.String(), "fc")
		isIPv6LinkLocal := strings.HasPrefix(ip.String(), "fe80:") // Link-local (fe80::/10)

		if isIPv6Localhost || isIPv6UniqueLocal || isIPv6LinkLocal {
			return fmt.Errorf("access to IPv6 loopback/private network blocked: %s", ip)
		}
	}

	// Check for metadata service IPs (full /16 range for link-local)
	linkLocalPrefix := net.ParseIP("169.254.0.0")
	linkLocalMask := net.CIDRMask(16, 32)
	linkLocalNetwork := &net.IPNet{
		IP:   linkLocalPrefix,
		Mask: linkLocalMask,
	}

	if linkLocalNetwork.Contains(ip) {
		return fmt.Errorf("access to link-local network blocked: %s", ip)
	}

	return nil
}
