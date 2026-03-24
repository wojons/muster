package linux

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSystemProxy_NoProxySet(t *testing.T) {
	// Clear all proxy environment variables
	clearProxyEnv()
	defer restoreEnv()

	config, err := GetSystemProxy()
	require.NoError(t, err)
	assert.Nil(t, config)
}

func TestGetSystemProxy_HTTPProxy(t *testing.T) {
	clearProxyEnv()
	defer restoreEnv()

	os.Setenv("HTTP_PROXY", "http://proxy.example.com:8080")

	config, err := GetSystemProxy()
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, "http://proxy.example.com:8080", config.URL)
	assert.False(t, config.SOCKS5)
}

func TestGetSystemProxy_HTTPSProxy(t *testing.T) {
	clearProxyEnv()
	defer restoreEnv()

	os.Setenv("HTTPS_PROXY", "https://secure-proxy.example.com:8443")

	config, err := GetSystemProxy()
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, "https://secure-proxy.example.com:8443", config.URL)
	assert.False(t, config.SOCKS5)
}

func TestGetSystemProxy_SocksProxy(t *testing.T) {
	clearProxyEnv()
	defer restoreEnv()

	os.Setenv("HTTP_PROXY", "socks5://socks-proxy.example.com:1080")

	config, err := GetSystemProxy()
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, "socks5://socks-proxy.example.com:1080", config.URL)
	assert.True(t, config.SOCKS5)
}

func TestGetSystemProxy_Socks4Proxy(t *testing.T) {
	clearProxyEnv()
	defer restoreEnv()

	os.Setenv("HTTP_PROXY", "socks4://socks4-proxy.example.com:1080")

	config, err := GetSystemProxy()
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, "socks4://socks4-proxy.example.com:1080", config.URL)
	assert.True(t, config.SOCKS5)
}

func TestGetSystemProxy_MusterProxyPrecedence(t *testing.T) {
	clearProxyEnv()
	defer restoreEnv()

	os.Setenv("HTTP_PROXY", "http://http-proxy.example.com:8080")
	os.Setenv("MUSTER_PROXY", "http://muster-proxy.example.com:9090")

	config, err := GetSystemProxy()
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, "http://muster-proxy.example.com:9090", config.URL)
}

func TestGetSystemProxy_OpenAPIProxyPrecedence(t *testing.T) {
	clearProxyEnv()
	defer restoreEnv()

	os.Setenv("HTTP_PROXY", "http://http-proxy.example.com:8080")
	os.Setenv("MUSTER_PROXY", "http://muster-proxy.example.com:9090")
	os.Setenv("OPENAPI_PROXY", "http://openapi-proxy.example.com:3000")

	config, err := GetSystemProxy()
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, "http://openapi-proxy.example.com:3000", config.URL)
}

func TestGetSystemProxy_LowercaseHttpProxy(t *testing.T) {
	clearProxyEnv()
	defer restoreEnv()

	os.Setenv("http_proxy", "http://lowercase-proxy.example.com:8080")

	config, err := GetSystemProxy()
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, "http://lowercase-proxy.example.com:8080", config.URL)
	assert.False(t, config.SOCKS5)
}

func TestGetSystemProxy_InvalidProxyURL(t *testing.T) {
	clearProxyEnv()
	defer restoreEnv()

	os.Setenv("HTTP_PROXY", "://invalid-url")

	config, err := GetSystemProxy()
	require.Error(t, err)
	assert.Nil(t, config)
}

func TestGetNoProxyHosts_Empty(t *testing.T) {
	clearProxyEnv()
	defer restoreEnv()

	hosts := GetNoProxyHosts()
	assert.Nil(t, hosts)
}

func TestGetNoProxyHosts_UpperCase(t *testing.T) {
	clearProxyEnv()
	defer restoreEnv()

	os.Setenv("NO_PROXY", "localhost,127.0.0.1,.example.com")

	hosts := GetNoProxyHosts()
	require.Len(t, hosts, 3)
	assert.Equal(t, "localhost", hosts[0])
	assert.Equal(t, "127.0.0.1", hosts[1])
	assert.Equal(t, ".example.com", hosts[2])
}

func TestGetNoProxyHosts_LowerCase(t *testing.T) {
	clearProxyEnv()
	defer restoreEnv()

	os.Setenv("no_proxy", "localhost,127.0.0.1")

	hosts := GetNoProxyHosts()
	require.Len(t, hosts, 2)
	assert.Equal(t, "localhost", hosts[0])
	assert.Equal(t, "127.0.0.1", hosts[1])
}

func TestShouldUseProxy_NoNoProxy(t *testing.T) {
	result := ShouldUseProxy("http://example.com", nil)
	assert.True(t, result)
}

func TestShouldUseProxy_EmptyNoProxy(t *testing.T) {
	result := ShouldUseProxy("http://example.com", []string{})
	assert.True(t, result)
}

func TestShouldUseProxy_ExactMatch(t *testing.T) {
	result := ShouldUseProxy("http://example.com", []string{"example.com"})
	assert.False(t, result)
}

func TestShouldUseProxy_WildcardSuffix(t *testing.T) {
	result := ShouldUseProxy("http://sub.example.com", []string{".example.com"})
	assert.False(t, result)
}

func TestShouldUseProxy_WildcardPrefix(t *testing.T) {
	result := ShouldUseProxy("http://sub.example.com", []string{"*.example.com"})
	assert.False(t, result)
}

func TestShouldUseProxy_NoMatch(t *testing.T) {
	result := ShouldUseProxy("http://example.com", []string{"other.com"})
	assert.True(t, result)
}

func TestShouldUseProxy_InvalidURL(t *testing.T) {
	result := ShouldUseProxy("://invalid-url", []string{"example.com"})
	assert.True(t, result)
}

// Helper functions

var savedEnv map[string]string

func clearProxyEnv() {
	savedEnv = make(map[string]string)

	proxyVars := []string{
		"OPENAPI_PROXY",
		"MUSTER_PROXY",
		"HTTPS_PROXY",
		"https_proxy",
		"HTTP_PROXY",
		"http_proxy",
		"NO_PROXY",
		"no_proxy",
		"AUTOCONFIG_URL",
	}

	for _, envVar := range proxyVars {
		savedEnv[envVar] = os.Getenv(envVar)
		os.Unsetenv(envVar)
	}
}

func restoreEnv() {
	for envVar, value := range savedEnv {
		if value != "" {
			os.Setenv(envVar, value)
		} else {
			os.Unsetenv(envVar)
		}
	}
}
