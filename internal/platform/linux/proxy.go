package linux

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robertkrimen/otto"
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
		// Would need pkg/github.com/apparentlymart/go-cidrcheck/cidrcheck
		// For now, continue with string matching
		// nolint:staticcheck
		if strings.Contains(pattern, "/") {
		}
	}

	return true
}

// ProxyConfig contains proxy configuration
type ProxyConfig struct {
	URL    string
	SOCKS5 bool
}

// PACResult represents the result of PAC file evaluation
type PACResult struct {
	ProxyType string // "DIRECT", "PROXY", "SOCKS", "SOCKS4", "SOCKS5"
	Host      string
	Port      string
	IsDirect  bool
	IsProxy   bool
	IsSOCKS   bool
}

// PACEvaluator evaluates PAC (Proxy Auto-Config) files
type PACEvaluator struct {
	vm    *otto.Otto
	cache *PACCache
	mu    sync.RWMutex
}

// PACCache caches PAC evaluation results
type PACCache struct {
	data      map[string]*CachedPACResult
	expiresAt time.Time
	mu        sync.RWMutex
	ttl       time.Duration
}

// CachedPACResult holds a cached PAC evaluation result
type CachedPACResult struct {
	Result    *PACResult
	Timestamp time.Time
}

// NewPACEvaluator creates a new PAC evaluator
func NewPACEvaluator() *PACEvaluator {
	return &PACEvaluator{
		vm: otto.New(),
		cache: &PACCache{
			data: make(map[string]*CachedPACResult),
			ttl:  5 * time.Minute,
		},
	}
}

// NewPACEvaluatorWithTTL creates a new PAC evaluator with custom TTL
func NewPACEvaluatorWithTTL(ttl time.Duration) *PACEvaluator {
	eval := NewPACEvaluator()
	eval.cache.ttl = ttl
	return eval
}

// GetSystemProxy retrieves system proxy settings from environment variables
func GetSystemProxy() (*ProxyConfig, error) {
	proxyURL, err := GetProxyFromEnvironment()

	if err != nil {
		return nil, err
	}

	if proxyURL == nil {
		return nil, nil
	}

	config := &ProxyConfig{
		URL:    proxyURL.String(),
		SOCKS5: strings.HasPrefix(proxyURL.Scheme, "socks"),
	}

	return config, nil
}

// ParsePACFile parses a PAC (Proxy Auto-Config) file content and returns an evaluator
func ParsePACFile(content string) (*PACEvaluator, error) {
	eval := NewPACEvaluator()

	if err := eval.Initialize(content); err != nil {
		return nil, err
	}

	return eval, nil
}

// Initialize loads and initializes the PAC file content
func (p *PACEvaluator) Initialize(pacContent string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	script := `
		function FindProxyForURL(url, host) {
			return "DIRECT";
		}
		
		var dnsDomainLevels = function(host) {
			var parts = host.split(".");
			return parts.length - 1;
		};
		
		var isResolvable = function(host) {
			return true;
		};
		
		var isPlainHostName = function(host) {
			return host.indexOf(".") == -1;
		};
		
		var dnsResolve = function(host) {
			return "127.0.0.1";
		};
		
		var myIpAddress = function() {
			return "127.0.0.1";
		};
		
		var isInNet = function(host, pattern, mask) {
			return false;
		};
		
		var shExpMatch = function(str, pattern) {
			var re = pattern.replace(/\./g, "\\.").replace(/\*/g, ".*").replace(/\?/g, ".");
			re = "^" + re + "$";
			return (str.search(re) != -1);
		};
		
		var weekdayRange = function() {
			return false;
		};
		
		var dateRange = function() {
			return false;
		};
		
		var timeRange = function() {
			return false;
		};
		
		var localHostOrDomainIs = function(host, hostdom) {
			return host == hostdom;
		};
	`

	pacScript := script + "\n" + pacContent

	_, err := p.vm.Run(pacScript)
	if err != nil {
		return fmt.Errorf("failed to parse PAC file: %w", err)
	}

	return nil
}

// FindProxyForURL finds the proxy for a given URL using the PAC file
func (p *PACEvaluator) FindProxyForURL(targetURL string) (*PACResult, error) {
	cacheKey := targetURL
	if cached := p.cache.Get(cacheKey); cached != nil {
		return cached, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	u, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	findProxyForURL, err := p.vm.Get("FindProxyForURL")
	if err != nil {
		return nil, fmt.Errorf("FindProxyForURL function not found: %w", err)
	}

	result, err := findProxyForURL.Call(findProxyForURL, targetURL, u.Hostname())
	if err != nil {
		return nil, fmt.Errorf("failed to call FindProxyForURL: %w", err)
	}

	resultStr, err := result.ToString()
	if err != nil {
		return nil, fmt.Errorf("invalid result from FindProxyForURL: %w", err)
	}

	pacResult := p.parseResult(resultStr)
	p.cache.Set(cacheKey, pacResult)

	return pacResult, nil
}

// parseResult parses the PAC result string into a PACResult struct
func (p *PACEvaluator) parseResult(resultStr string) *PACResult {
	resultStr = strings.TrimSpace(resultStr)

	if resultStr == "DIRECT" {
		return &PACResult{
			ProxyType: "DIRECT",
			IsDirect:  true,
		}
	}

	parts := strings.Fields(resultStr)
	if len(parts) == 0 {
		return &PACResult{
			ProxyType: "DIRECT",
			IsDirect:  true,
		}
	}

	proxyType := strings.ToUpper(parts[0])

	if strings.HasPrefix(proxyType, "SOCKS5:") {
		hostPort := strings.TrimPrefix(parts[0], "SOCKS5:")
		hostPort = strings.TrimSuffix(hostPort, ";")
		host, port, _ := strings.Cut(hostPort, ":")
		return &PACResult{
			ProxyType: "SOCKS5",
			Host:      host,
			Port:      port,
			IsSOCKS:   true,
		}
	} else if proxyType == "SOCKS5" && len(parts) >= 2 {
		hostPort := strings.TrimSuffix(parts[1], ";")
		host, port, _ := strings.Cut(hostPort, ":")
		return &PACResult{
			ProxyType: "SOCKS5",
			Host:      host,
			Port:      port,
			IsSOCKS:   true,
		}
	}

	if strings.HasPrefix(proxyType, "SOCKS4:") {
		hostPort := strings.TrimPrefix(parts[0], "SOCKS4:")
		hostPort = strings.TrimSuffix(hostPort, ";")
		host, port, _ := strings.Cut(hostPort, ":")
		return &PACResult{
			ProxyType: "SOCKS4",
			Host:      host,
			Port:      port,
			IsSOCKS:   true,
		}
	} else if proxyType == "SOCKS4" && len(parts) >= 2 {
		hostPort := strings.TrimSuffix(parts[1], ";")
		host, port, _ := strings.Cut(hostPort, ":")
		return &PACResult{
			ProxyType: "SOCKS4",
			Host:      host,
			Port:      port,
			IsSOCKS:   true,
		}
	}

	if strings.HasPrefix(proxyType, "SOCKS:") {
		hostPort := strings.TrimPrefix(parts[0], "SOCKS:")
		hostPort = strings.TrimSuffix(hostPort, ";")
		host, port, _ := strings.Cut(hostPort, ":")
		return &PACResult{
			ProxyType: "SOCKS",
			Host:      host,
			Port:      port,
			IsSOCKS:   true,
		}
	} else if proxyType == "SOCKS" && len(parts) >= 2 {
		hostPort := strings.TrimSuffix(parts[1], ";")
		host, port, _ := strings.Cut(hostPort, ":")
		return &PACResult{
			ProxyType: "SOCKS",
			Host:      host,
			Port:      port,
			IsSOCKS:   true,
		}
	}

	if strings.HasPrefix(proxyType, "PROXY:") {
		hostPort := strings.TrimPrefix(parts[0], "PROXY:")
		hostPort = strings.TrimSuffix(hostPort, ";")
		host, port, _ := strings.Cut(hostPort, ":")
		return &PACResult{
			ProxyType: "PROXY",
			Host:      host,
			Port:      port,
			IsProxy:   true,
		}
	} else if proxyType == "PROXY" && len(parts) >= 2 {
		hostPort := strings.TrimSuffix(parts[1], ";")
		host, port, _ := strings.Cut(hostPort, ":")
		return &PACResult{
			ProxyType: "PROXY",
			Host:      host,
			Port:      port,
			IsProxy:   true,
		}
	}

	if strings.HasPrefix(proxyType, "HTTPS:") {
		hostPort := strings.TrimPrefix(parts[0], "HTTPS:")
		hostPort = strings.TrimSuffix(hostPort, ";")
		host, port, _ := strings.Cut(hostPort, ":")
		return &PACResult{
			ProxyType: "HTTPS",
			Host:      host,
			Port:      port,
			IsProxy:   true,
		}
	} else if proxyType == "HTTPS" && len(parts) >= 2 {
		hostPort := strings.TrimSuffix(parts[1], ";")
		host, port, _ := strings.Cut(hostPort, ":")
		return &PACResult{
			ProxyType: "HTTPS",
			Host:      host,
			Port:      port,
			IsProxy:   true,
		}
	}

	if unquoted, err := strconv.Unquote(parts[0]); err == nil {
		if h, p, found := strings.Cut(unquoted, ":"); found {
			return &PACResult{
				ProxyType: "PROXY",
				Host:      h,
				Port:      p,
				IsProxy:   true,
			}
		}
	}

	return &PACResult{
		ProxyType: "DIRECT",
		IsDirect:  true,
	}
}

// GetCache returns the PAC cache
func (p *PACEvaluator) GetCache() *PACCache {
	return p.cache
}

// Get returns a cached PAC result
func (c *PACCache) Get(key string) *PACResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if time.Now().After(c.expiresAt) {
		return nil
	}

	if entry, ok := c.data[key]; ok {
		return entry.Result
	}

	return nil
}

// Set sets a cached PAC result
func (c *PACCache) Set(key string, result *PACResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.expiresAt = now.Add(c.ttl)

	c.data[key] = &CachedPACResult{
		Result:    result,
		Timestamp: now,
	}
}

// Clear clears the PAC cache
func (c *PACCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[string]*CachedPACResult)
	c.expiresAt = time.Time{}
}

// parsePAC parses a PAC (proxy auto-config) URL and evaluates it
func parsePAC(pacURL string) (*url.URL, error) {
	resp, err := http.Get(pacURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PAC file: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read PAC file: %w", err)
	}

	eval, err := ParsePACFile(string(content))
	if err != nil {
		return nil, err
	}

	targetURL := pacURL
	result, err := eval.FindProxyForURL(targetURL)
	if err != nil {
		return nil, err
	}

	if result.IsDirect {
		return nil, nil
	}

	if result.IsProxy || result.IsSOCKS {
		scheme := "http"
		if result.IsSOCKS {
			scheme = "socks5"
		}

		proxyURL := &url.URL{
			Scheme: scheme,
			Host:   result.Host + ":" + result.Port,
		}

		return proxyURL, nil
	}

	return nil, nil
}
