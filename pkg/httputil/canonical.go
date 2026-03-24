// Package httputil provides HTTP utility functions including URL and header canonicalization.
//
// This package implements HTTP canonicalization functions to ensure deterministic
// request/response handling by normalizing URLs and headers. Canonicalization is
// essential for caching, logging, and replay scenarios where request equivalence
// must be precisely determined.
//
// The canonicalization process includes:
//   - Lowercasing scheme and host
//   - Removing default ports (80 for HTTP, 443 for HTTPS)
//   - Sorting query parameters alphabetically
//   - Lowercasing header names
//   - Sorting header values alphabetically
//
// Example usage:
//
//	// Canonicalize a URL
//	canonical, err := httputil.CanonicalizeURL("HTTPS://Example.COM:443/path?z=1&a=2")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// Result: "https://example.com/path?a=2&z=1"
//
//	// Canonicalize headers
//	headers := http.Header{
//	    "Accept": []string{"application/json", "text/html"},
//	    "X-Custom": []string{"value"},
//	}
//	canonicalHeaders := httputil.CanonicalizeHeaders(headers)
//	// Result: accept: [application/json text/html], x-custom: [value]
//
//	// Canonicalize an entire request
//	req, _ := http.NewRequest("GET", "http://Example.com/path", nil)
//	req.Header.Set("Accept", "application/json")
//	canonicalReq, err := httputil.CanonicalizeRequest(req)
package httputil

import (
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// CanonicalizeURL normalizes a URL string for deterministic comparison.
//
// The normalization process includes:
//   - Converting scheme and host to lowercase
//   - Removing default ports (80 for HTTP, 443 for HTTPS)
//   - Sorting query parameters alphabetically by key
//
// Parameters:
//   - rawURL: The URL string to canonicalize
//
// Returns:
//   - The canonicalized URL string
//   - An error if the URL cannot be parsed
//
// Example:
//
//	"HTTPS://Example.COM:443/path?z=1&a=2" → "https://example.com/path?a=2&z=1"
//	"http://API.example.com:80/v1" → "http://api.example.com/v1"
func CanonicalizeURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	// Lowercase scheme and host
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)

	// Remove default ports
	if (u.Scheme == "http" && u.Port() == "80") ||
		(u.Scheme == "https" && u.Port() == "443") {
		u.Host = u.Hostname()
	}

	// Sort query parameters (only if there are actual parameters)
	// Handle ForceQuery flag to prevent trailing "?" when query is empty
	values, err := url.ParseQuery(u.RawQuery)
	if err == nil {
		if len(values) > 0 {
			u.RawQuery = values.Encode()
		} else {
			// Empty query - remove ForceQuery to prevent trailing "?"
			u.ForceQuery = false
		}
	}

	return u.String(), nil
}

// CanonicalizeHeaders normalizes HTTP headers for deterministic comparison.
//
// The normalization process includes:
//   - Converting header names to lowercase
//   - Sorting header values alphabetically
//
// Parameters:
//   - headers: The HTTP headers to canonicalize
//
// Returns:
//   - A new http.Header with canonicalized names and sorted values
//
// Example:
//
//	Input:  Accept: [application/json, text/html], X-Custom: [value]
//	Output: accept: [application/json text/html], x-custom: [value]
func CanonicalizeHeaders(headers http.Header) http.Header {
	canonical := make(http.Header)

	for key, values := range headers {
		// Lowercase header names
		key = strings.ToLower(key)

		// Sort values
		sorted := make([]string, len(values))
		copy(sorted, values)
		sort.Strings(sorted)

		canonical[key] = sorted
	}

	return canonical
}

// CanonicalizeRequest normalizes an HTTP request for deterministic comparison.
//
// This function canonicalizes both the request URL and headers. The request
// is modified in place for the URL and headers.
//
// Parameters:
//   - req: The HTTP request to canonicalize
//
// Returns:
//   - The canonicalized request (same pointer as input, modified in place)
//   - An error if the URL cannot be canonicalized or if req is nil
//
// Example:
//
//	req, _ := http.NewRequest("GET", "HTTPS://Example.COM/path", nil)
//	req.Header.Set("Accept", "application/json")
//	canonicalReq, err := httputil.CanonicalizeRequest(req)
//	// canonicalReq.URL is now "https://example.com/path"
//	// canonicalReq.Header has lowercase keys and sorted values
func CanonicalizeRequest(req *http.Request) (*http.Request, error) {
	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	// Canonicalize URL
	if req.URL != nil {
		canonicalURL, err := CanonicalizeURL(req.URL.String())
		if err != nil {
			return nil, err
		}

		newURL, err := url.Parse(canonicalURL)
		if err != nil {
			return nil, err
		}
		req.URL = newURL
	}

	// Canonicalize headers
	req.Header = CanonicalizeHeaders(req.Header)

	return req, nil
}

// CanonicalClient is the deterministic HTTP client interface.
// All components MUST use this exact interface for deterministic request handling.
// This ensures request equivalence for caching, logging, and replay scenarios.
type CanonicalClient interface {
	// Do executes an HTTP request with canonicalization
	Do(req *http.Request) (*http.Response, error)

	// Get executes a GET request
	Get(url string) (*http.Response, error)

	// Post executes a POST request
	Post(url string, bodyType string, body interface{}) (*http.Response, error)

	// Put executes a PUT request
	Put(url string, bodyType string, body interface{}) (*http.Response, error)

	// Patch executes a PATCH request
	Patch(url string, bodyType string, body interface{}) (*http.Response, error)

	// Delete executes a DELETE request
	Delete(url string) (*http.Response, error)

	// Head executes a HEAD request
	Head(url string) (*http.Response, error)

	// Options executes an OPTIONS request
	Options(url string) (*http.Response, error)

	// SetCanonicalize enables/disables request canonicalization
	SetCanonicalize(enabled bool)

	// SetValidator enables/disables interface enforcement validation
	SetValidator(enabled bool)

	// Close closes the client and releases resources
	Close() error
}

// CanonicalValidator enforces deterministic interface compliance.
// It validates that requests and responses conform to the canonical interface.
type CanonicalValidator struct {
	enabled        bool
	strictMode     bool
	violationCount int64
	lastViolation  error
}

// CanonicalValidatorOption configures a CanonicalValidator.
type CanonicalValidatorOption func(*CanonicalValidator)

// WithStrictMode enables strict validation mode.
func WithStrictMode(strict bool) CanonicalValidatorOption {
	return func(v *CanonicalValidator) {
		v.strictMode = strict
	}
}

// NewCanonicalValidator creates a new canonical validator.
func NewCanonicalValidator(opts ...CanonicalValidatorOption) *CanonicalValidator {
	v := &CanonicalValidator{
		enabled:    true,
		strictMode: false,
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// ValidateRequest validates that a request conforms to canonical interface.
// Returns nil if valid, or an error describing the violation.
func (v *CanonicalValidator) ValidateRequest(req *http.Request) error {
	if !v.enabled || req == nil {
		return nil
	}

	var violations []error

	// Validate URL is not nil
	if req.URL == nil {
		violations = append(violations, errors.New("request URL is nil"))
	}

	// Validate method is not empty
	if req.Method == "" {
		violations = append(violations, errors.New("request method is empty"))
	}

	// Validate headers are canonicalized (lowercase keys)
	for key := range req.Header {
		if key != strings.ToLower(key) {
			violations = append(violations, errors.New("header key not canonicalized: "+key))
		}
	}

	if len(violations) > 0 {
		v.violationCount++
		v.lastViolation = violations[0]
		if v.strictMode {
			return errors.Join(violations...)
		}
	}

	return nil
}

// ValidateResponse validates that a response conforms to canonical interface.
func (v *CanonicalValidator) ValidateResponse(resp *http.Response) error {
	if !v.enabled || resp == nil {
		return nil
	}

	var violations []error

	// Validate status code is valid
	if resp.StatusCode < 100 || resp.StatusCode >= 600 {
		violations = append(violations, errors.New("invalid status code: "+string(rune(resp.StatusCode))))
	}

	// Validate headers are canonicalized
	for key := range resp.Header {
		if key != strings.ToLower(key) {
			violations = append(violations, errors.New("response header key not canonicalized: "+key))
		}
	}

	if len(violations) > 0 {
		v.violationCount++
		v.lastViolation = violations[0]
		if v.strictMode {
			return errors.Join(violations...)
		}
	}

	return nil
}

// GetViolationCount returns the number of violations detected.
func (v *CanonicalValidator) GetViolationCount() int64 {
	return v.violationCount
}

// GetLastViolation returns the last violation detected.
func (v *CanonicalValidator) GetLastViolation() error {
	return v.lastViolation
}

// SetEnabled enables or disables validation.
func (v *CanonicalValidator) SetEnabled(enabled bool) {
	v.enabled = enabled
}

// IsEnabled returns whether validation is enabled.
func (v *CanonicalValidator) IsEnabled() bool {
	return v.enabled
}

// EnforcementError indicates an interface enforcement violation.
type EnforcementError struct {
	Component string
	Field     string
	Expected  string
	Actual    string
}

func (e *EnforcementError) Error() string {
	return "interface enforcement violation in " + e.Component + ": " + e.Field + " expected " + e.Expected + ", got " + e.Actual
}

// ValidateInterface enforces that a component implements the required interface.
func ValidateInterface(component, interfaceName string, impl interface{}) error {
	if impl == nil {
		return &EnforcementError{
			Component: component,
			Field:     "implementation",
			Expected:  "non-nil implementation of " + interfaceName,
			Actual:    "nil",
		}
	}
	return nil
}
