//go:build linux
// +build linux

package linux

import (
	"net/url"
	"os"
	"strings"
)

// GetProxyFromEnvironment detects proxy settings from environment variables
func GetProxyFromEnvironment() (*url.URL, error) {
	// Check proxy environment variables in order of precedence
	envVars := []string{
		"OPENAPI_PROXY",
		"MUSTER_PROXY",
		"HTTPS_PROXY",
		"https_proxy",
		"HTTP_PROXY",
		"http_proxy",
	}

	for _, envVar := range envVars {
		if proxyURL := os.Getenv(envVar); proxyURL != "" {
			return url.Parse(proxyURL)
		}
	}

	// Check for AUTOCONFIG_URL for PAC files
	if pacURL := os.Getenv("AUTOCONFIG_URL"); pacURL != "" {
		return parsePAC(pacURL)
	}

	return nil, nil
}

// GetNoProxyHosts returns the list of hosts that should bypass proxy
func GetNoProxyHosts() []string {
	noProxy := os.Getenv("NO_PROXY")
	if noProxy == "" {
		noProxy = os.Getenv("no_proxy")
	}

	if noProxy == "" {
		return nil
	}

	return strings.Split(noProxy, ",")
}

// ShouldUseProxy checks if a URL should use proxy based on NO_PROXY settings
func ShouldUseProxy(targetURL string, noProxyHosts []string) bool {
	if len(noProxyHosts) == 0 {
		return true
	}

	u, err := url.Parse(targetURL)
	if err != nil {
		return true
	}

	host := u.Hostname()

	for _, pattern := range noProxyHosts {
		pattern = strings.TrimSpace(pattern)

		// Exact match
		if host == pattern {
			return false
		}

		// Wildcard suffix match (e.g., .example.com)
		if strings.HasPrefix(pattern, ".") && strings.HasSuffix(host, pattern) {
			return false
		}

		// Wildcard prefix match (e.g., *.example.com)
		if strings.HasPrefix(pattern, "*.") &&
			strings.HasSuffix(host, pattern[1:]) {
			return false
		}

		// CIDR notation (simplified - real implementation needs netip package)
		if strings.Contains(pattern, "/") {
			// Would need pkg/github.com/apparentlymart/go-cidrcheck/cidrcheck
			// For now, continue with string matching
		}
	}

	return true
}

// parsePAC parses a PAC (proxy auto-config) URL
// This is a simplified implementation - real PAC parsing requires JavaScript engine
func parsePAC(pacURL string) (*url.URL, error) {
	// For production, would need to:
	// 1. Download PAC file
	// 2. Execute JavaScript to find proxy
	// 3. Parse result and return proxy URL
	//
	// For now, return nil to indicate no proxy
	// Could add dependency on github.com/robertkrimen/otto for JS execution
	return nil, nil
}
