package linux

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePACFile_BasicPAC(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			return "DIRECT";
		}
	`

	eval, err := ParsePACFile(pacContent)
	require.NoError(t, err)
	require.NotNil(t, eval)
}

func TestParsePACFile_ProxyPAC(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			return "PROXY proxy.example.com:8080";
		}
	`

	eval, err := ParsePACFile(pacContent)
	require.NoError(t, err)
	require.NotNil(t, eval)

	result, err := eval.FindProxyForURL("http://example.com")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "PROXY", result.ProxyType)
	assert.Equal(t, "proxy.example.com", result.Host)
	assert.Equal(t, "8080", result.Port)
	assert.True(t, result.IsProxy)
}

func TestParsePACFile_SocksPAC(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			return "SOCKS5 socks.example.com:1080";
		}
	`

	eval, err := ParsePACFile(pacContent)
	require.NoError(t, err)
	require.NotNil(t, eval)

	result, err := eval.FindProxyForURL("http://example.com")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "SOCKS5", result.ProxyType)
	assert.Equal(t, "socks.example.com", result.Host)
	assert.Equal(t, "1080", result.Port)
	assert.True(t, result.IsSOCKS)
}

func TestParsePACFile_Socks4PAC(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			return "SOCKS4 socks4.example.com:1080";
		}
	`

	eval, err := ParsePACFile(pacContent)
	require.NoError(t, err)
	require.NotNil(t, eval)

	result, err := eval.FindProxyForURL("http://example.com")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "SOCKS4", result.ProxyType)
	assert.Equal(t, "socks4.example.com", result.Host)
	assert.Equal(t, "1080", result.Port)
	assert.True(t, result.IsSOCKS)
}

func TestParsePACFile_InvalidPAC(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			return invalid syntax here
		}
	`

	eval, err := ParsePACFile(pacContent)
	assert.Error(t, err)
	assert.Nil(t, eval)
}

func TestParsePACFile_EmptyPAC(t *testing.T) {
	pacContent := ""

	eval, err := ParsePACFile(pacContent)
	require.NoError(t, err)
	require.NotNil(t, eval)

	result, err := eval.FindProxyForURL("http://example.com")
	require.NoError(t, err)
	assert.Equal(t, "DIRECT", result.ProxyType)
}

func TestPACEvaluator_FindProxyForURL_Direct(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			return "DIRECT";
		}
	`

	eval, err := ParsePACFile(pacContent)
	require.NoError(t, err)

	result, err := eval.FindProxyForURL("http://example.com")
	require.NoError(t, err)
	assert.Equal(t, "DIRECT", result.ProxyType)
	assert.True(t, result.IsDirect)
}

func TestPACEvaluator_FindProxyForURL_MultipleProxies(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			return "PROXY primary.example.com:8080; PROXY backup.example.com:8080; DIRECT";
		}
	`

	eval, err := ParsePACFile(pacContent)
	require.NoError(t, err)

	result, err := eval.FindProxyForURL("http://example.com")
	require.NoError(t, err)
	assert.Equal(t, "PROXY", result.ProxyType)
	assert.Equal(t, "primary.example.com", result.Host)
	assert.Equal(t, "8080", result.Port)
}

func TestPACEvaluator_FindProxyForURL_HTTPS(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			return "HTTPS secure-proxy.example.com:443";
		}
	`

	eval, err := ParsePACFile(pacContent)
	require.NoError(t, err)

	result, err := eval.FindProxyForURL("http://example.com")
	require.NoError(t, err)
	assert.Equal(t, "HTTPS", result.ProxyType)
	assert.Equal(t, "secure-proxy.example.com", result.Host)
	assert.Equal(t, "443", result.Port)
}

func TestPACEvaluator_Cache(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			return "PROXY proxy.example.com:8080";
		}
	`

	eval := NewPACEvaluator()
	err := eval.Initialize(pacContent)
	require.NoError(t, err)

	result1, err := eval.FindProxyForURL("http://example.com")
	require.NoError(t, err)

	result2, err := eval.FindProxyForURL("http://example.com")
	require.NoError(t, err)

	assert.Equal(t, result1.ProxyType, result2.ProxyType)
	assert.Equal(t, result1.Host, result2.Host)
}

func TestPACEvaluator_CacheExpiration(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			return "PROXY proxy.example.com:8080";
		}
	`

	eval := NewPACEvaluatorWithTTL(100 * time.Millisecond)
	err := eval.Initialize(pacContent)
	require.NoError(t, err)

	_, err = eval.FindProxyForURL("http://example.com")
	require.NoError(t, err)

	time.Sleep(150 * time.Millisecond)

	cache := eval.GetCache()
	cached := cache.Get("http://example.com")
	assert.Nil(t, cached)
}

func TestPACCache_Get_Set(t *testing.T) {
	cache := &PACCache{
		data: make(map[string]*CachedPACResult),
		ttl:  5 * time.Minute,
	}

	result := &PACResult{
		ProxyType: "PROXY",
		Host:      "proxy.example.com",
		Port:      "8080",
		IsProxy:   true,
	}

	cache.Set("http://example.com", result)

	cached := cache.Get("http://example.com")
	require.NotNil(t, cached)
	assert.Equal(t, "PROXY", cached.ProxyType)
	assert.Equal(t, "proxy.example.com", cached.Host)
}

func TestPACCache_Clear(t *testing.T) {
	cache := &PACCache{
		data: make(map[string]*CachedPACResult),
		ttl:  5 * time.Minute,
	}

	cache.Set("http://example.com", &PACResult{ProxyType: "PROXY"})
	cache.Clear()

	cached := cache.Get("http://example.com")
	assert.Nil(t, cached)
}

func TestPACCache_Expired(t *testing.T) {
	cache := &PACCache{
		data:      make(map[string]*CachedPACResult),
		ttl:       50 * time.Millisecond,
		expiresAt: time.Now().Add(50 * time.Millisecond),
	}

	cache.Set("http://example.com", &PACResult{ProxyType: "PROXY"})

	time.Sleep(100 * time.Millisecond)

	cached := cache.Get("http://example.com")
	assert.Nil(t, cached)
}

func TestParsePACResult_Direct(t *testing.T) {
	eval := NewPACEvaluator()

	result := eval.parseResult("DIRECT")
	assert.Equal(t, "DIRECT", result.ProxyType)
	assert.True(t, result.IsDirect)
}

func TestParsePACResult_Proxy(t *testing.T) {
	eval := NewPACEvaluator()

	result := eval.parseResult("PROXY proxy.example.com:8080")
	assert.Equal(t, "PROXY", result.ProxyType)
	assert.Equal(t, "proxy.example.com", result.Host)
	assert.Equal(t, "8080", result.Port)
	assert.True(t, result.IsProxy)
}

func TestParsePACResult_Socks(t *testing.T) {
	eval := NewPACEvaluator()

	result := eval.parseResult("SOCKS socks.example.com:1080")
	assert.Equal(t, "SOCKS", result.ProxyType)
	assert.Equal(t, "socks.example.com", result.Host)
	assert.Equal(t, "1080", result.Port)
	assert.True(t, result.IsSOCKS)
}

func TestParsePACResult_Socks4(t *testing.T) {
	eval := NewPACEvaluator()

	result := eval.parseResult("SOCKS4 socks4.example.com:1080")
	assert.Equal(t, "SOCKS4", result.ProxyType)
	assert.Equal(t, "socks4.example.com", result.Host)
	assert.Equal(t, "1080", result.Port)
	assert.True(t, result.IsSOCKS)
}

func TestParsePACResult_Socks5(t *testing.T) {
	eval := NewPACEvaluator()

	result := eval.parseResult("SOCKS5 socks5.example.com:1080")
	assert.Equal(t, "SOCKS5", result.ProxyType)
	assert.Equal(t, "socks5.example.com", result.Host)
	assert.Equal(t, "1080", result.Port)
	assert.True(t, result.IsSOCKS)
}

func TestParsePACResult_Empty(t *testing.T) {
	eval := NewPACEvaluator()

	result := eval.parseResult("")
	assert.Equal(t, "DIRECT", result.ProxyType)
	assert.True(t, result.IsDirect)
}

func TestParsePACResult_Whitespace(t *testing.T) {
	eval := NewPACEvaluator()

	result := eval.parseResult("  DIRECT  ")
	assert.Equal(t, "DIRECT", result.ProxyType)
	assert.True(t, result.IsDirect)
}

func TestParsePACResult_MultipleProxies(t *testing.T) {
	eval := NewPACEvaluator()

	result := eval.parseResult("PROXY primary:8080; PROXY backup:8080; DIRECT")
	assert.Equal(t, "PROXY", result.ProxyType)
	assert.Equal(t, "primary", result.Host)
	assert.Equal(t, "8080", result.Port)
}

func TestPACEvaluator_ComplexPAC(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			if (isPlainHostName(host)) {
				return "DIRECT";
			} else {
				return "PROXY proxy.example.com:8080";
			}
		}
	`

	eval, err := ParsePACFile(pacContent)
	require.NoError(t, err)

	result1, err := eval.FindProxyForURL("http://localhost")
	require.NoError(t, err)
	assert.Equal(t, "DIRECT", result1.ProxyType)

	result2, err := eval.FindProxyForURL("http://example.com")
	require.NoError(t, err)
	assert.Equal(t, "PROXY", result2.ProxyType)
	assert.Equal(t, "proxy.example.com", result2.Host)
}

func TestPACEvaluator_PACWithDNS(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			if (dnsDomainLevels(host) > 1) {
				return "PROXY proxy.example.com:8080";
			}
			return "DIRECT";
		}
	`

	eval, err := ParsePACFile(pacContent)
	require.NoError(t, err)

	result, err := eval.FindProxyForURL("http://sub.example.com")
	require.NoError(t, err)
	assert.Equal(t, "PROXY", result.ProxyType)
	assert.Equal(t, "proxy.example.com", result.Host)
}

func TestPACEvaluator_IsResolvable(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			if (isResolvable(host)) {
				return "DIRECT";
			}
			return "PROXY proxy.example.com:8080";
		}
	`

	eval, err := ParsePACFile(pacContent)
	require.NoError(t, err)

	result, err := eval.FindProxyForURL("http://example.com")
	require.NoError(t, err)
	assert.Equal(t, "DIRECT", result.ProxyType)
}

func TestPACEvaluator_ShExpMatch(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			if (shExpMatch(host, "*.example.com")) {
				return "PROXY proxy.example.com:8080";
			}
			return "DIRECT";
		}
	`

	eval, err := ParsePACFile(pacContent)
	require.NoError(t, err)

	result, err := eval.FindProxyForURL("http://sub.example.com")
	require.NoError(t, err)
	assert.Equal(t, "PROXY", result.ProxyType)
}

func TestPACEvaluator_LocalHostOrDomainIs(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			if (localHostOrDomainIs(host, "example.com")) {
				return "DIRECT";
			}
			return "PROXY proxy.example.com:8080";
		}
	`

	eval, err := ParsePACFile(pacContent)
	require.NoError(t, err)

	result, err := eval.FindProxyForURL("http://example.com")
	require.NoError(t, err)
	assert.Equal(t, "DIRECT", result.ProxyType)
}

func TestPACEvaluator_MyIPAddress(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			if (myIpAddress() == "127.0.0.1") {
				return "DIRECT";
			}
			return "PROXY proxy.example.com:8080";
		}
	`

	eval, err := ParsePACFile(pacContent)
	require.NoError(t, err)

	result, err := eval.FindProxyForURL("http://example.com")
	require.NoError(t, err)
	assert.Equal(t, "DIRECT", result.ProxyType)
}

func TestParsePACFile_ConcurrentAccess(t *testing.T) {
	pacContent := `
		function FindProxyForURL(url, host) {
			return "PROXY proxy.example.com:8080";
		}
	`

	eval, err := ParsePACFile(pacContent)
	require.NoError(t, err)

	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			_, err := eval.FindProxyForURL("http://example.com")
			assert.NoError(t, err)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestPACCache_ConcurrentAccess(t *testing.T) {
	cache := &PACCache{
		data: make(map[string]*CachedPACResult),
		ttl:  5 * time.Minute,
	}

	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			key := "http://example" + string(rune(idx)) + ".com"
			cache.Set(key, &PACResult{ProxyType: "PROXY"})
			cache.Get(key)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
