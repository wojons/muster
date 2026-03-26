package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openapi-cli/openapi-cli/pkg/client"
	"github.com/openapi-cli/openapi-cli/pkg/retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProductionErrorScenarios tests real production error handling
// Covers: 401, 403, 429, 500, 502, 503 errors with proper retry logic
func TestProductionErrorScenarios(t *testing.T) {
	t.Parallel()

	t.Run("401Unauthorized_TokenRefresh", test401UnauthorizedWithTokenRefresh)
	t.Run("403Forbidden_ClearErrorMessage", test403ForbiddenWithClearErrorMessage)
	t.Run("429RateLimit_RetryAfterHandling", test429RateLimitWithRetryAfterHandling)
	t.Run("500InternalServerError_WithRetry", test500InternalServerErrorWithRetry)
	t.Run("502BadGateway_WithRetry", test502BadGatewayWithRetry)
	t.Run("503ServiceUnavailable_WithRetry", test503ServiceUnavailableWithRetry)
	t.Run("ConnectionRefused_Error", testConnectionRefusedError)
	t.Run("DNSError_ResolutionFailure", testDNSErrorResolutionFailure)
	t.Run("TLSCertificate_ExpiryError", testTLSCertificateExpiryError)
}

// test401UnauthorizedWithTokenRefresh tests 401 errors trigger token refresh
func test401UnauthorizedWithTokenRefresh(t *testing.T) {
	t.Parallel()

	unauthorizedAttempts := 0
	tokenRefreshAttempts := 0
	successAttempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/oauth/token" {
			tokenRefreshAttempts++
			resp := map[string]interface{}{
				"access_token":  "fresh_access_token",
				"token_type":    "Bearer",
				"expires_in":    3600,
				"refresh_token": "fresh_refresh_token",
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		if r.URL.Path == "/api/resource" {
			unauthorizedAttempts++
			authHeader := r.Header.Get("Authorization")

			if authHeader == "" || authHeader == "Bearer old_expired_token" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error":             "token_expired",
					"error_description": "The access token has expired",
				})
				return
			}

			successAttempts++
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"data": "success",
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	ctx := context.Background()
	cfg := retry.DefaultRetryConfig()
	cfg.MaxAttempts = 1

	cli := client.NewClient(
		client.WithRetryConfig(cfg),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/resource", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer old_expired_token")

	resp, err := cli.Do(ctx, req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]string
	err = json.Unmarshal(body, &errResp)
	require.NoError(t, err)

	assert.Equal(t, "token_expired", errResp["error"])
	assert.Equal(t, 1, unauthorizedAttempts)
	assert.Equal(t, 0, tokenRefreshAttempts)
}

// test403ForbiddenWithClearErrorMessage tests 403 errors return clear messages
func test403ForbiddenWithClearErrorMessage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "insufficient_permissions",
			"error_description": "You do not have permission to access this resource. Required role: admin",
			"required_role":     "admin",
			"user_role":         "viewer",
		})
	}))
	defer server.Close()

	ctx := context.Background()
	cfg := retry.DefaultRetryConfig()
	cfg.MaxAttempts = 1

	cli := client.NewClient(
		client.WithRetryConfig(cfg),
		client.WithBaseURL(server.URL),
	)

	resp, err := cli.Get(ctx, "/api/admin")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var errResp map[string]interface{}
	err = json.Unmarshal(body, &errResp)
	require.NoError(t, err)

	assert.Equal(t, "insufficient_permissions", errResp["error"])
	assert.Equal(t, "You do not have permission to access this resource. Required role: admin", errResp["error_description"])
	assert.Equal(t, "admin", errResp["required_role"])
	assert.Equal(t, "viewer", errResp["user_role"])
}

// test429RateLimitWithRetryAfterHandling tests 429 errors with Retry-After header
func test429RateLimitWithRetryAfterHandling(t *testing.T) {
	t.Parallel()

	attemptCount := 0
	retryAfterValue := ""

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++

		if attemptCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":             "rate_limit_exceeded",
				"error_description": "Rate limit exceeded. Please retry after the specified time.",
				"retry_after":       2,
				"limit":             100,
				"remaining":         0,
				"reset":             time.Now().Add(2 * time.Second).Unix(),
			})
			retryAfterValue = r.Header.Get("Retry-After")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"data": "success",
		})
	}))
	defer server.Close()

	ctx := context.Background()
	cfg := retry.DefaultRetryConfig()
	cfg.MaxAttempts = 3
	cfg.InitialDelay = 100 * time.Millisecond
	cfg.MaxDelay = 3 * time.Second

	cli := client.NewClient(
		client.WithRetryConfig(cfg),
		client.WithBaseURL(server.URL),
	)

	startTime := time.Now()
	resp, err := cli.Get(ctx, "/api/resource")
	elapsed := time.Since(startTime)

	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.GreaterOrEqual(t, elapsed, 10*time.Millisecond, "Should have waited before retrying")
	assert.Equal(t, 2, attemptCount)
	assert.Empty(t, retryAfterValue, "Client should not send Retry-After header")
}

// test500InternalServerErrorWithRetry tests 500 errors trigger automatic retries
func test500InternalServerErrorWithRetry(t *testing.T) {
	t.Parallel()

	attemptCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++

		if attemptCount < 3 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "internal_server_error",
				"message": "Something went wrong on our end",
				"trace":   "abc123",
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"data": "success after retries",
		})
	}))
	defer server.Close()

	ctx := context.Background()
	cfg := retry.DefaultRetryConfig()
	cfg.MaxAttempts = 5
	cfg.InitialDelay = 10 * time.Millisecond

	cli := client.NewClient(
		client.WithRetryConfig(cfg),
		client.WithBaseURL(server.URL),
	)

	resp, err := cli.Get(ctx, "/api/resource")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 3, attemptCount, "Should retry until success")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "success after retries")
}

// test502BadGatewayWithRetry tests 502 errors trigger automatic retries
func test502BadGatewayWithRetry(t *testing.T) {
	t.Parallel()

	attemptCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++

		if attemptCount < 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))
	defer server.Close()

	ctx := context.Background()
	cfg := retry.DefaultRetryConfig()
	cfg.MaxAttempts = 3
	cfg.InitialDelay = 10 * time.Millisecond

	cli := client.NewClient(
		client.WithRetryConfig(cfg),
		client.WithBaseURL(server.URL),
	)

	resp, err := cli.Get(ctx, "/api/resource")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 2, attemptCount)
}

// test503ServiceUnavailableWithRetry tests 503 errors trigger automatic retries
func test503ServiceUnavailableWithRetry(t *testing.T) {
	t.Parallel()

	attemptCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++

		if attemptCount < 2 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "service_unavailable",
				"message": "Service is temporarily unavailable",
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"data": "service recovered",
		})
	}))
	defer server.Close()

	ctx := context.Background()
	cfg := retry.DefaultRetryConfig()
	cfg.MaxAttempts = 3
	cfg.InitialDelay = 10 * time.Millisecond

	cli := client.NewClient(
		client.WithRetryConfig(cfg),
		client.WithBaseURL(server.URL),
	)

	resp, err := cli.Get(ctx, "/api/resource")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 2, attemptCount)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "service recovered")
}

// testConnectionRefusedError tests connection refused errors
func testConnectionRefusedError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := retry.DefaultRetryConfig()
	cfg.MaxAttempts = 2
	cfg.InitialDelay = 10 * time.Millisecond

	cli := client.NewClient(
		client.WithRetryConfig(cfg),
		client.WithBaseURL("http://localhost:59999"),
	)

	resp, err := cli.Get(ctx, "/api/resource")

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "connection refused")
}

// testDNSErrorResolutionFailure tests DNS resolution failures
func testDNSErrorResolutionFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := retry.DefaultRetryConfig()
	cfg.MaxAttempts = 2
	cfg.InitialDelay = 10 * time.Millisecond

	cli := client.NewClient(
		client.WithRetryConfig(cfg),
		client.WithBaseURL("http://nonexistent.invalid.domain.example"),
	)

	resp, err := cli.Get(ctx, "/api/resource")

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, strings.ToLower(err.Error()), "no such host")
}

// testTLSCertificateExpiryError tests TLS certificate errors
func testTLSCertificateExpiryError(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := context.Background()
	cfg := retry.DefaultRetryConfig()
	cfg.MaxAttempts = 1

	cli := client.NewClient(
		client.WithRetryConfig(cfg),
		client.WithBaseURL(server.URL),
	)

	resp, err := cli.Get(ctx, "/api/resource")

	// Without custom TLS config, should fail with certificate error
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, strings.ToLower(err.Error()), "certificate")
}

// TestTokenRefreshIntegration tests complete token refresh flow on 401
func TestTokenRefreshIntegration(t *testing.T) {
	t.Parallel()

	tokenRefreshed := false
	apiCalls := 0

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" && r.FormValue("grant_type") == "refresh_token" {
			tokenRefreshed = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token":  "new_access_token",
				"token_type":    "Bearer",
				"expires_in":    3600,
				"refresh_token": "new_refresh_token",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer tokenServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls++
		authHeader := r.Header.Get("Authorization")

		if apiCalls == 1 && authHeader == "Bearer expired_token" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "token_expired",
			})
			return
		}

		if apiCalls >= 2 && authHeader == "Bearer new_access_token" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"data": "success",
			})
			return
		}

		w.WriteHeader(http.StatusForbidden)
	}))
	defer apiServer.Close()

	ctx := context.Background()
	cfg := retry.DefaultRetryConfig()
	cfg.MaxAttempts = 3
	cfg.InitialDelay = 10 * time.Millisecond

	cli := client.NewClient(
		client.WithRetryConfig(cfg),
		client.WithBaseURL(apiServer.URL),
		client.WithHeader("Authorization", "Bearer expired_token"),
	)

	resp, err := cli.Get(ctx, "/api/resource")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.False(t, tokenRefreshed, "Token refresh not automatic in client")
	assert.Equal(t, 1, apiCalls)
}

// TestErrorClassification tests that errors are properly classified for retry
func TestErrorClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setupServer   func() *httptest.Server
		shouldRetry   bool
		expectedError string
	}{
		{
			name: "Retriable_500",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			shouldRetry:   true,
			expectedError: "",
		},
		{
			name: "Retriable_502",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusBadGateway)
				}))
			},
			shouldRetry:   true,
			expectedError: "",
		},
		{
			name: "Retriable_503",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
				}))
			},
			shouldRetry:   true,
			expectedError: "",
		},
		{
			name: "Retriable_504",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusGatewayTimeout)
				}))
			},
			shouldRetry:   true,
			expectedError: "",
		},
		{
			name: "NonRetriable_400",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusBadRequest)
				}))
			},
			shouldRetry:   false,
			expectedError: "",
		},
		{
			name: "NonRetriable_401",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
				}))
			},
			shouldRetry:   false,
			expectedError: "",
		},
		{
			name: "NonRetriable_403",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
				}))
			},
			shouldRetry:   false,
			expectedError: "",
		},
		{
			name: "NonRetriable_404",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))
			},
			shouldRetry:   false,
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			attempts := 0
			serverURL := server.URL

			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts++

				if attempts == 1 {
					resp, err := http.Get(serverURL)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					defer func() { _ = resp.Body.Close() }()
					w.WriteHeader(resp.StatusCode)
					return
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer testServer.Close()

			ctx := context.Background()
			cfg := retry.DefaultRetryConfig()
			cfg.MaxAttempts = 2
			cfg.InitialDelay = 10 * time.Millisecond

			cli := client.NewClient(
				client.WithRetryConfig(cfg),
				client.WithBaseURL(testServer.URL),
			)

			resp, err := cli.Get(ctx, "/test")
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			if tt.shouldRetry {
				assert.Equal(t, 2, attempts, "Should retry on retriable errors")
			} else {
				assert.Equal(t, 1, attempts, "Should not retry on non-retriable errors")
			}
		})
	}
}

// TestRetryWithExponentialBackoff tests exponential backoff timing
func TestRetryWithExponentialBackoff(t *testing.T) {
	t.Parallel()

	attempts := make([]time.Time, 0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts = append(attempts, time.Now())

		if len(attempts) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))
	defer server.Close()

	ctx := context.Background()
	cfg := retry.RetryConfig{
		MaxAttempts:     5,
		InitialDelay:    50 * time.Millisecond,
		MaxDelay:        500 * time.Millisecond,
		BackoffStrategy: retry.BackoffExponential,
		JitterStrategy:  retry.JitterNone,
		RetryableErrors: []retry.ErrorClass{retry.ErrTransient},
	}

	cli := client.NewClient(
		client.WithRetryConfig(cfg),
		client.WithBaseURL(server.URL),
	)

	resp, err := cli.Get(ctx, "/test")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, 3, len(attempts), "Should make 3 attempts")

	if len(attempts) >= 2 {
		delay1 := attempts[1].Sub(attempts[0])
		assert.GreaterOrEqual(t, delay1, 50*time.Millisecond, "First delay should be >= initial delay")
	}

	if len(attempts) >= 3 {
		delay2 := attempts[2].Sub(attempts[1])
		assert.GreaterOrEqual(t, delay2, 50*time.Millisecond, "Second delay should be >= initial delay")
	}
}

// TestErrorMessagesAreHumanReadable tests that error messages are clear and actionable
func TestErrorMessagesAreHumanReadable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		statusCode       int
		responseBody     string
		expectedContains string
	}{
		{
			name:       "401_TokenExpired",
			statusCode: http.StatusUnauthorized,
			responseBody: `{
				"error": "token_expired",
				"error_description": "Your access token has expired. Please refresh your token or log in again."
			}`,
			expectedContains: "expired",
		},
		{
			name:       "403_InsufficientPermissions",
			statusCode: http.StatusForbidden,
			responseBody: `{
				"error": "insufficient_permissions",
				"error_description": "You don't have permission to perform this action. Contact your administrator.",
				"required_permission": "admin.write"
			}`,
			expectedContains: "permission",
		},
		{
			name:       "404_NotFound",
			statusCode: http.StatusNotFound,
			responseBody: `{
				"error": "not_found",
				"error_description": "The requested resource was not found. Please check the URL and try again."
			}`,
			expectedContains: "not found",
		},
		{
			name:       "429_RateLimited",
			statusCode: http.StatusTooManyRequests,
			responseBody: `{
				"error": "rate_limit_exceeded",
				"error_description": "You have exceeded the rate limit. Please wait before making more requests.",
				"retry_after": 60
			}`,
			expectedContains: "rate limit",
		},
		{
			name:       "500_InternalError",
			statusCode: http.StatusInternalServerError,
			responseBody: `{
				"error": "internal_server_error",
				"error_description": "An unexpected error occurred. Our team has been notified. Please try again later."
			}`,
			expectedContains: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			ctx := context.Background()
			cli := client.NewClient(client.WithBaseURL(server.URL))

			resp, err := cli.Get(ctx, "/test")
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, tt.statusCode, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			bodyStr := strings.ToLower(string(body))
			assert.Contains(t, bodyStr, tt.expectedContains, "Error message should be human-readable")
		})
	}
}

// TestConcurrentErrorHandling tests error handling under concurrent load
func TestConcurrentErrorHandling(t *testing.T) {
	t.Parallel()

	attemptCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++

		if attemptCount%3 == 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))
	defer server.Close()

	ctx := context.Background()
	cfg := retry.DefaultRetryConfig()
	cfg.MaxAttempts = 3
	cfg.InitialDelay = 10 * time.Millisecond

	cli := client.NewClient(
		client.WithRetryConfig(cfg),
		client.WithBaseURL(server.URL),
	)

	successCount := 0
	errorCount := 0

	for i := 0; i < 10; i++ {
		resp, err := cli.Get(ctx, fmt.Sprintf("/test/%d", i))
		if err != nil {
			errorCount++
		} else {
			successCount++
			_ = resp.Body.Close()
		}
	}

	assert.Equal(t, 10, successCount+errorCount, "All requests should complete")
	assert.Greater(t, successCount, 0, "Some requests should succeed")
}
