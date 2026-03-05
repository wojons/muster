# WASM Loader Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: WASM Loading

## Overview

Complete specification of the **WASM module loading** system, responsible for loading, validating, and caching WASM modules from various sources.

## 1. Module Sources

### 1.1 Source Types

```go
type ModuleSourceType string

const (
    SourceLocalFile    ModuleSourceType = "local_file"
    SourceRemoteURL    ModuleSourceType = "remote_url"
    SourceEmbedded     ModuleSourceType = "embedded"
    SourceRegistry     ModuleSourceType = "registry"
    SourceInventory    ModuleSourceType = "inventory"
)

type ModuleSource struct {
    Type    ModuleSourceType `json:"type"`
    Path    string           `json:"path"`
    URL     string           `json:"url,omitempty"`
    Hash    string           `json:"hash,omitempty"`
    Version string           `json:"version,omitempty"`
}
```

### 1.2 Loading from Local File

```go
func (l *WASMLoader) LoadFromFile(path string) (*WASMModule, error) {
    // Validate file exists
    info, err := os.Stat(path)
    if err != nil {
        return nil, fmt.Errorf("file not accessible: %w", err)
    }
    
    // Check size limit (50MB)
    if info.Size() > 50*1024*1024 {
        return nil, fmt.Errorf("module exceeds 50MB limit: %d bytes", info.Size())
    }
    
    // Read file
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read file: %w", err)
    }
    
    return l.loadFromBytes(data, path)
}
```

### 1.3 Loading from Remote URL

```go
func (l *WASMLoader) LoadFromURL(ctx context.Context, url string, opts RemoteLoadOptions) (*WASMModule, error) {
    // Create request with timeout
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }
    
    // Add headers
    req.Header.Set("Accept", "application/wasm")
    
    // Execute request
    client := &http.Client{Timeout: opts.Timeout}
    resp, err := client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }
    
    // Read response
    data, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("failed to read response: %w", err)
    }
    
    return l.loadFromBytes(data, url)
}
```

## 2. Module Validation

### 2.1 WASM Validation

```go
func (l *WASMLoader) validateWASM(data []byte) error {
    // Check minimum size
    if len(data) < 8 {
        return errors.New("WASM module too small")
    }
    
    // Check magic number: \0asm
    magic := binary.LittleEndian.Uint32(data[0:4])
    if magic != 0x6d736100 {
        return fmt.Errorf("invalid WASM magic: 0x%x", magic)
    }
    
    // Check version
    version := binary.LittleEndian.Uint32(data[4:8])
    if version != 1 {
        return fmt.Errorf("unsupported WASM version: %d", version)
    }
    
    return nil
}
```

### 2.2 Section Validation

```go
func (l *WASMLoader) validateSections(data []byte) error {
    offset := 8 // Skip header
    
    for offset < len(data) {
        if offset+1 > len(data) {
            return errors.New("truncated section ID")
        }
        
        sectionID := data[offset]
        offset++
        
        // Read section size
        size, n, err := decodeULEB128(data[offset:])
        if err != nil {
            return fmt.Errorf("invalid section size: %w", err)
        }
        offset += n
        
        if offset+int(size) > len(data) {
            return errors.New("section extends past end of module")
        }
        
        // Validate specific sections
        sectionData := data[offset : offset+int(size)]
        if err := l.validateSection(SectionID(sectionID), sectionData); err != nil {
            return fmt.Errorf("section %d validation failed: %w", sectionID, err)
        }
        
        offset += int(size)
    }
    
    return nil
}
```

## 3. Module Caching

### 3.1 Cache Key Generation

```go
func generateCacheKey(source ModuleSource) string {
    h := sha256.New()
    h.Write([]byte(source.Type))
    h.Write([]byte(source.Path))
    h.Write([]byte(source.URL))
    h.Write([]byte(source.Version))
    return hex.EncodeToString(h.Sum(nil))
}
```

### 3.2 Cache Operations

```go
type ModuleCache struct {
    dir   string
    ttl   time.Duration
}

func (c *ModuleCache) Get(key string) ([]byte, bool) {
    path := filepath.Join(c.dir, key+".wasm")
    
    info, err := os.Stat(path)
    if err != nil {
        return nil, false
    }
    
    // Check TTL
    if time.Since(info.ModTime()) > c.ttl {
        os.Remove(path)
        return nil, false
    }
    
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, false
    }
    
    return data, true
}

func (c *ModuleCache) Set(key string, data []byte) error {
    path := filepath.Join(c.dir, key+".wasm")
    return os.WriteFile(path, data, 0644)
}
```

## 4. Security Validation

```go
func (l *WASMLoader) validateSecurity(data []byte, caps []Capability) error {
    module, err := wasm.DecodeModule(data)
    if err != nil {
        return fmt.Errorf("decode failed: %w", err)
    }
    
    // Check imports against capabilities
    for _, imp := range module.Imports {
        if !l.isImportAllowed(imp, caps) {
            return fmt.Errorf("import not allowed: %s.%s", imp.Module, imp.Name)
        }
    }
    
    return nil
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
