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
//   - An error if the URL cannot be canonicalized
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
		return nil, nil
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
