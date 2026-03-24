package inventory

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProtocolCache(t *testing.T) {
	t.Run("default TTL", func(t *testing.T) {
		cache := NewProtocolCache(0)
		require.NotNil(t, cache)
		assert.Equal(t, 1*time.Hour, cache.ttl)
	})

	t.Run("custom TTL", func(t *testing.T) {
		cache := NewProtocolCache(30 * time.Minute)
		require.NotNil(t, cache)
		assert.Equal(t, 30*time.Minute, cache.ttl)
	})
}

func TestProtocolCache_GetSet(t *testing.T) {
	cache := NewProtocolCache(1 * time.Hour)

	now := time.Now()
	data := &InventoryProtocolResponse{
		Version:       "1.0.0",
		MinCLIVersion: "0.9.0",
		GeneratedAt:   now,
	}

	t.Run("set and get", func(t *testing.T) {
		cache.Set("test-url", "etag-123", data)

		entry := cache.Get("test-url", "")
		require.NotNil(t, entry)
		assert.Equal(t, "etag-123", entry.ETag)
		assert.Equal(t, data, entry.Data)
		assert.WithinDuration(t, time.Now(), entry.Timestamp, 1*time.Second)
	})

	t.Run("etag mismatch", func(t *testing.T) {
		entry := cache.Get("test-url", "different-etag")
		assert.Nil(t, entry)
	})

	t.Run("non-existent key", func(t *testing.T) {
		entry := cache.Get("non-existent", "")
		assert.Nil(t, entry)
	})

	t.Run("delete", func(t *testing.T) {
		cache.Delete("test-url")
		entry := cache.Get("test-url", "")
		assert.Nil(t, entry)
	})
}

func TestProtocolCache_TTL(t *testing.T) {
	cache := NewProtocolCache(100 * time.Millisecond)

	data := &InventoryProtocolResponse{Version: "1.0.0"}
	cache.Set("test-url", "etag", data)

	t.Run("not expired", func(t *testing.T) {
		entry := cache.Get("test-url", "")
		require.NotNil(t, entry)
		assert.Equal(t, data, entry.Data)
	})

	time.Sleep(150 * time.Millisecond)

	t.Run("expired", func(t *testing.T) {
		entry := cache.Get("test-url", "")
		assert.Nil(t, entry)
	})
}

func TestNewProtocolClient(t *testing.T) {
	t.Run("default options", func(t *testing.T) {
		client := NewProtocolClient(ProtocolClientOptions{
			BaseURL:       "https://api.example.com",
			ClientVersion: "1.2.3",
			Fingerprint:   "test-fp",
		})

		require.NotNil(t, client)
		assert.Equal(t, "https://api.example.com", client.baseURL)
		assert.Equal(t, "1.2.3", client.clientVersion)
		assert.Equal(t, "test-fp", client.fingerprint)
		assert.Equal(t, 30*time.Second, client.timeout)
		assert.Equal(t, 3, client.maxRetries)
		assert.Equal(t, 5, client.maxRedirects)
		require.NotNil(t, client.cache)
	})

	t.Run("custom options", func(t *testing.T) {
		customCache := NewProtocolCache(2 * time.Hour)
		client := NewProtocolClient(ProtocolClientOptions{
			BaseURL:       "https://api.test.com",
			ClientVersion: "2.0.0",
			Fingerprint:   "custom-fp",
			Token:         "test-token",
			Timeout:       60 * time.Second,
			MaxRetries:    5,
			MaxRedirects:  10,
			Cache:         customCache,
		})

		require.NotNil(t, client)
		assert.Equal(t, "https://api.test.com", client.baseURL)
		assert.Equal(t, "test-token", client.token)
		assert.Equal(t, 60*time.Second, client.timeout)
		assert.Equal(t, 5, client.maxRetries)
		assert.Equal(t, 10, client.maxRedirects)
		assert.Equal(t, customCache, client.cache)
	})

	t.Run("trims trailing slash", func(t *testing.T) {
		client := NewProtocolClient(ProtocolClientOptions{
			BaseURL: "https://api.example.com/",
		})
		assert.Equal(t, "https://api.example.com", client.baseURL)
	})
}

func TestProtocolClient_FetchInventory_Success(t *testing.T) {
	now := time.Now()
	response := &InventoryProtocolResponse{
		Version:       "2.1.0",
		MinCLIVersion: "1.0.0",
		GeneratedAt:   now,
		OpenAPI: &OpenAPIReference{
			URL:    "https://api.example.com/_openapi.json",
			SHA256: "abc123",
		},
		ConfigDefaults: ConfigDefaults{
			TimeoutMS:  30000,
			MaxRetries: 3,
			Features:   map[string]bool{"streaming": true},
		},
		Auth: &AuthConfig{
			Required: true,
			Type:     "bearer",
			Scopes:   []string{"read", "inventory"},
		},
		Meta: &ServerMeta{
			ServerTime:  now,
			Environment: "production",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/_inventory.json", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		assert.Equal(t, "1.2.3", r.Header.Get("X-Client-Version"))
		assert.Equal(t, "test-fingerprint", r.Header.Get("X-Fingerprint"))
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		w.Header().Set("ETag", "test-etag")
		w.Header().Set("X-Inventory-Version", "1.0")
		w.WriteHeader(http.StatusOK)

		data, _ := json.Marshal(response)
		_, _ = w.Write(data)
	}))
	defer server.Close()

	client := NewProtocolClient(ProtocolClientOptions{
		BaseURL:       server.URL,
		ClientVersion: "1.2.3",
		Fingerprint:   "test-fingerprint",
		Token:         "test-token",
	})

	ctx := context.Background()
	result, err := client.FetchInventory(ctx, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "2.1.0", result.Version)
	assert.Equal(t, "1.0.0", result.MinCLIVersion)
	assert.Equal(t, "test-etag", client.cache.entries[server.URL+"/_inventory.json"].ETag)
}

func TestProtocolClient_FetchInventory_Cached(t *testing.T) {
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("ETag", "test-etag")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(&InventoryProtocolResponse{Version: "1.0.0"})
	}))
	defer server.Close()

	client := NewProtocolClient(ProtocolClientOptions{
		BaseURL:       server.URL,
		ClientVersion: "1.2.3",
		Fingerprint:   "fp",
	})

	ctx := context.Background()

	result, err := client.FetchInventory(ctx, &ProtocolFetchOptions{})
	require.NoError(t, err)
	require.NotNil(t, result)

	result2, err := client.FetchInventory(ctx, &ProtocolFetchOptions{ETag: "cached-etag"})
	require.NoError(t, err)
	assert.Equal(t, result, result2)
}

func TestProtocolClient_FetchInventory_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(&InventoryErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required",
			AuthURL: "https://auth.example.com/login",
		})
	}))
	defer server.Close()

	client := NewProtocolClient(ProtocolClientOptions{
		BaseURL:       server.URL,
		ClientVersion: "1.2.3",
		Fingerprint:   "fp",
	})

	ctx := context.Background()
	_, err := client.FetchInventory(ctx, nil)

	require.Error(t, err)
	var unauthorized *UnauthorizedError
	require.ErrorAs(t, err, &unauthorized)
	assert.Equal(t, "Authentication required", unauthorized.Message)
	assert.Equal(t, "https://auth.example.com/login", unauthorized.URL)
}

func TestProtocolClient_FetchInventory_Forbidden(t *testing.T) {
	revokedAt := time.Now().Add(-24 * time.Hour)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(&InventoryErrorResponse{
			Error:           "forbidden",
			Message:         "Client version revoked",
			Code:            "VERSION_REVOKED",
			RevokedAt:       &revokedAt,
			UpgradeRequired: true,
		})
	}))
	defer server.Close()

	client := NewProtocolClient(ProtocolClientOptions{
		BaseURL:       server.URL,
		ClientVersion: "1.2.3",
		Fingerprint:   "fp",
	})

	ctx := context.Background()
	_, err := client.FetchInventory(ctx, nil)

	require.Error(t, err)
	var forbidden *ForbiddenError
	require.ErrorAs(t, err, &forbidden)
	assert.Equal(t, "Client version revoked", forbidden.Message)
	assert.Equal(t, "VERSION_REVOKED", forbidden.Code)
	assert.True(t, forbidden.UpgradeRequired)
}

func TestProtocolClient_FetchInventory_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(&InventoryErrorResponse{
			Error:   "not_found",
			Message: "Inventory endpoint not available",
		})
	}))
	defer server.Close()

	client := NewProtocolClient(ProtocolClientOptions{
		BaseURL:       server.URL,
		ClientVersion: "1.2.3",
		Fingerprint:   "fp",
	})

	ctx := context.Background()
	_, err := client.FetchInventory(ctx, nil)

	require.Error(t, err)
	var notFound *NotFoundError
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, "Inventory endpoint not available", notFound.Message)
}

func TestProtocolClient_FetchInventory_ValidationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(&InventoryErrorResponse{
			Error:          "validation_error",
			Message:        "Client version too old",
			MinVersion:     "1.5.0",
			CurrentVersion: "1.2.3",
			UpgradeURL:     "https://docs.example.com/upgrade",
		})
	}))
	defer server.Close()

	client := NewProtocolClient(ProtocolClientOptions{
		BaseURL:       server.URL,
		ClientVersion: "1.2.3",
		Fingerprint:   "fp",
	})

	ctx := context.Background()
	_, err := client.FetchInventory(ctx, nil)

	require.Error(t, err)
	var validation *ValidationError
	require.ErrorAs(t, err, &validation)
	assert.Equal(t, "Client version too old", validation.Message)
	assert.Equal(t, "1.5.0", validation.MinVersion)
	assert.Equal(t, "1.2.3", validation.CurrentVersion)
	assert.Equal(t, "https://docs.example.com/upgrade", validation.UpgradeURL)
}

func TestProtocolClient_FetchInventory_Redirect(t *testing.T) {
	redirectCount := 0
	var finalURL string

	mainServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		w.Header().Set("Location", finalURL)
		w.WriteHeader(http.StatusPermanentRedirect)
	}))
	defer mainServer.Close()

	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(&InventoryProtocolResponse{Version: "2.0.0"})
	}))
	defer finalServer.Close()

	finalURL = finalServer.URL

	client := NewProtocolClient(ProtocolClientOptions{
		BaseURL:       mainServer.URL,
		ClientVersion: "1.2.3",
		Fingerprint:   "fp",
		MaxRedirects:  5,
	})

	ctx := context.Background()
	result, err := client.FetchInventory(ctx, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "2.0.0", result.Version)
	assert.GreaterOrEqual(t, redirectCount, 1)
}

func TestProtocolClient_FetchInventory_TooManyRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", r.URL.String()+"/redirect")
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer server.Close()

	client := NewProtocolClient(ProtocolClientOptions{
		BaseURL:       server.URL,
		ClientVersion: "1.2.3",
		Fingerprint:   "fp",
		MaxRedirects:  5,
	})

	ctx := context.Background()
	_, err := client.FetchInventory(ctx, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many redirects")
}

func TestProtocolClient_VerifyWASMSignatures(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	module := WASMModule{
		Name:    "test-module",
		URL:     "https://example.com/module.wasm",
		SHA256:  "abc123",
		Version: "1.0.0",
	}

	message := module.Name + ":" + module.SHA256 + ":" + module.URL + ":" + module.Version
	signature := ed25519.Sign(privateKey, []byte(message))

	module.Signature = &WMASignature{
		Algorithm: "Ed25519",
		PublicKey: base64.StdEncoding.EncodeToString(publicKey),
		Signature: base64.StdEncoding.EncodeToString(signature),
	}

	t.Run("valid signature", func(t *testing.T) {
		client := NewProtocolClient(ProtocolClientOptions{
			BaseURL:       "https://api.example.com",
			ClientVersion: "1.2.3",
			Fingerprint:   "fp",
		})

		err := client.verifyWASMSignatures([]WASMModule{module})
		require.NoError(t, err)
	})

	t.Run("unsupported algorithm", func(t *testing.T) {
		module.Signature.Algorithm = "RSA"

		client := NewProtocolClient(ProtocolClientOptions{
			BaseURL:       "https://api.example.com",
			ClientVersion: "1.2.3",
			Fingerprint:   "fp",
		})

		err := client.verifyWASMSignatures([]WASMModule{module})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported algorithm")
	})

	t.Run("invalid signature", func(t *testing.T) {
		module.Signature.Algorithm = "Ed25519"
		module.Signature.Signature = base64.StdEncoding.EncodeToString([]byte("invalid"))

		client := NewProtocolClient(ProtocolClientOptions{
			BaseURL:       "https://api.example.com",
			ClientVersion: "1.2.3",
			Fingerprint:   "fp",
		})

		err := client.verifyWASMSignatures([]WASMModule{module})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "module test-module")
	})

	t.Run("nil signature", func(t *testing.T) {
		module.Signature = nil

		client := NewProtocolClient(ProtocolClientOptions{
			BaseURL:       "https://api.example.com",
			ClientVersion: "1.2.3",
			Fingerprint:   "fp",
		})

		err := client.verifyWASMSignatures([]WASMModule{module})
		require.NoError(t, err)
	})
}

func TestProtocolClient_UpdateToken(t *testing.T) {
	client := NewProtocolClient(ProtocolClientOptions{
		BaseURL:       "https://api.example.com",
		ClientVersion: "1.2.3",
		Fingerprint:   "fp",
		Token:         "old-token",
	})

	assert.Equal(t, "old-token", client.token)

	client.UpdateToken("new-token")
	assert.Equal(t, "new-token", client.token)
}

func TestProtocolClient_GetBaseURL(t *testing.T) {
	client := NewProtocolClient(ProtocolClientOptions{
		BaseURL:       "https://api.example.com",
		ClientVersion: "1.2.3",
		Fingerprint:   "fp",
	})

	assert.Equal(t, "https://api.example.com", client.GetBaseURL())
}

func TestProtocolClient_ClearCache(t *testing.T) {
	client := NewProtocolClient(ProtocolClientOptions{
		BaseURL:       "https://api.example.com",
		ClientVersion: "1.2.3",
		Fingerprint:   "fp",
	})

	client.cache.Set("test", "etag", &InventoryProtocolResponse{Version: "1.0.0"})
	require.NotNil(t, client.cache.Get("test", ""))

	client.ClearCache()

	assert.Nil(t, client.cache.Get("test", ""))
}

func TestVerifySignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	message := "test-message"
	signature := ed25519.Sign(privateKey, []byte(message))

	publicKeyB64 := base64.StdEncoding.EncodeToString(publicKey)
	signatureB64 := base64.StdEncoding.EncodeToString(signature)

	t.Run("valid signature", func(t *testing.T) {
		valid, err := VerifySignature(message, publicKeyB64, signatureB64)
		require.NoError(t, err)
		assert.True(t, valid)
	})

	t.Run("invalid public key", func(t *testing.T) {
		valid, err := VerifySignature(message, "invalid-base64", signatureB64)
		require.Error(t, err)
		assert.False(t, valid)
		assert.Contains(t, err.Error(), "invalid public key")
	})

	t.Run("invalid signature", func(t *testing.T) {
		valid, err := VerifySignature(message, publicKeyB64, "invalid-base64")
		require.Error(t, err)
		assert.False(t, valid)
		assert.Contains(t, err.Error(), "invalid signature")
	})

	t.Run("tampered message", func(t *testing.T) {
		valid, err := VerifySignature("tampered-message", publicKeyB64, signatureB64)
		require.NoError(t, err)
		assert.False(t, valid)
	})
}

func TestCalculateSHA256(t *testing.T) {
	data := []byte("test-data")
	hash := CalculateSHA256(data)

	assert.Equal(t, "a186000422feab857329c684e9fe91412b1a5db084100b37a98cfc95b62aa867", hash)
}

func TestCompressDecompressGZIP(t *testing.T) {
	original := []byte("test data to compress")

	compressed, err := CompressGZIP(original)
	require.NoError(t, err)
	require.NotNil(t, compressed)

	decompressed, err := DecompressGZIP(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestParseRetryAfter(t *testing.T) {
	t.Run("seconds", func(t *testing.T) {
		duration, err := ParseRetryAfter("60")
		require.NoError(t, err)
		assert.Equal(t, 60*time.Second, duration)
	})

	t.Run("date format", func(t *testing.T) {
		future := time.Now().Add(120 * time.Second)
		duration, err := ParseRetryAfter(future.Format(time.RFC1123))
		require.NoError(t, err)
		assert.InDelta(t, 120.0, duration.Seconds(), 5.0)
	})

	t.Run("empty header", func(t *testing.T) {
		_, err := ParseRetryAfter("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty Retry-After header")
	})

	t.Run("invalid format", func(t *testing.T) {
		_, err := ParseRetryAfter("invalid")
		require.Error(t, err)
	})
}

func TestProtocolClient_ShouldRetry(t *testing.T) {
	client := NewProtocolClient(ProtocolClientOptions{
		BaseURL:       "https://api.example.com",
		ClientVersion: "1.2.3",
		Fingerprint:   "fp",
	})

	t.Run("network error", func(t *testing.T) {
		assert.True(t, client.shouldRetry(assert.AnError))
	})

	t.Run("unauthorized", func(t *testing.T) {
		err := &UnauthorizedError{Message: "auth required"}
		assert.False(t, client.shouldRetry(err))
	})

	t.Run("forbidden", func(t *testing.T) {
		err := &ForbiddenError{Message: "access denied"}
		assert.False(t, client.shouldRetry(err))
	})

	t.Run("not found", func(t *testing.T) {
		err := &NotFoundError{Message: "not found"}
		assert.False(t, client.shouldRetry(err))
	})

	t.Run("validation error", func(t *testing.T) {
		err := &ValidationError{Message: "invalid"}
		assert.False(t, client.shouldRetry(err))
	})
}

func TestProtocolClient_CalculateBackoff(t *testing.T) {
	client := NewProtocolClient(ProtocolClientOptions{
		BaseURL:       "https://api.example.com",
		ClientVersion: "1.2.3",
		Fingerprint:   "fp",
	})

	t.Run("attempt 0", func(t *testing.T) {
		delay := client.calculateBackoff(0, nil)
		assert.InDelta(t, 1.0, delay.Seconds(), 0.1)
	})

	t.Run("attempt 1", func(t *testing.T) {
		delay := client.calculateBackoff(1, nil)
		assert.InDelta(t, 2.0, delay.Seconds(), 0.2)
	})

	t.Run("attempt 2", func(t *testing.T) {
		delay := client.calculateBackoff(2, nil)
		assert.InDelta(t, 4.0, delay.Seconds(), 0.4)
	})
}

func TestProtocolClient_ResolveURL(t *testing.T) {
	client := NewProtocolClient(ProtocolClientOptions{
		BaseURL:       "https://api.example.com/v1",
		ClientVersion: "1.2.3",
		Fingerprint:   "fp",
	})

	t.Run("absolute URL", func(t *testing.T) {
		url := client.resolveURL("https://other.com/path")
		assert.Equal(t, "https://other.com/path", url)
	})

	t.Run("path starting with slash", func(t *testing.T) {
		url := client.resolveURL("/path")
		assert.Equal(t, "https://api.example.com/v1/path", url)
	})

	t.Run("relative path", func(t *testing.T) {
		url := client.resolveURL("path")
		assert.Equal(t, "https://api.example.com/v1/path", url)
	})
}

func TestProtocolClient_ExtractBaseURL(t *testing.T) {
	client := NewProtocolClient(ProtocolClientOptions{
		BaseURL:       "https://api.example.com/v1",
		ClientVersion: "1.2.3",
		Fingerprint:   "fp",
	})

	t.Run("full URL", func(t *testing.T) {
		base := client.extractBaseURL("https://new-api.example.com/v2/path")
		assert.Equal(t, "https://new-api.example.com", base)
	})

	t.Run("URL without path", func(t *testing.T) {
		base := client.extractBaseURL("https://example.com")
		assert.Equal(t, "https://example.com", base)
	})
}
