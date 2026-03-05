# Profiles Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Configuration

## Overview

Complete specification of **profile system** for environment-specific settings.

## 1. Profile Structure

```go
type Profile struct {
    Name         string            `json:"name"`
    BaseURL      string            `json:"base_url"`
    Auth         AuthProfile       `json:"auth"`
    Defaults     map[string]string `json:"defaults"`
    Headers      map[string]string `json:"headers"`
}

type AuthProfile struct {
    Type   string            `json:"type"`
    Config map[string]string `json:"config"`
}
```

## 2. Profile Examples

```yaml
profiles:
  development:
    base_url: "http://localhost:8080"
    auth:
      type: "api_key"
      config:
        key_header: "X-API-Key"
        key_value: "dev-key-123"
    defaults:
      timeout: "30"
      
  production:
    base_url: "https://api.example.com"
    auth:
      type: "oauth2"
      config:
        provider: "github"
        client_id: "prod-client"
    defaults:
      timeout: "60"
    headers:
      X-Environment: "production"
```

## 3. Profile Selection

```go
func (p *Profile) MergeWith(other *Profile) *Profile {
    merged := &Profile{
        Name:     p.Name,
        BaseURL:  p.BaseURL,
        Defaults: make(map[string]string),
        Headers:  make(map[string]string),
    }
    
    // Merge defaults
    for k, v := range other.Defaults {
        merged.Defaults[k] = v
    }
    for k, v := range p.Defaults {
        merged.Defaults[k] = v
    }
    
    return merged
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
