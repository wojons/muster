# HTTP Canonicalization Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: HTTP

## Overview

Complete specification of **HTTP canonicalization** for determinism.

## 1. URL Canonicalization

```go
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
    
    // Sort query parameters
    if u.RawQuery != "" {
        values, _ := url.ParseQuery(u.RawQuery)
        u.RawQuery = values.Encode()
    }
    
    return u.String(), nil
}
```

## 2. Header Canonicalization

```go
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
```

## 3. Request Canonicalization

```go
func CanonicalizeRequest(req *http.Request) (*http.Request, error) {
    // Canonicalize URL
    canonicalURL, err := CanonicalizeURL(req.URL.String())
    if err != nil {
        return nil, err
    }
    
    newURL, _ := url.Parse(canonicalURL)
    req.URL = newURL
    
    // Canonicalize headers
    req.Header = CanonicalizeHeaders(req.Header)
    
    return req, nil
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
