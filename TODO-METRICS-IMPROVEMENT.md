# Idle-Time Spec Conformance Sweep - Gaps Found

**Date**: 2026-03-05
**Spec Analyzed**: specs/CACHE-POLICIES.md
**Status**: ❌ MAJOR GAPS FOUND - Implementation incomplete vs. spec

## Spec Requirements

### 1. TTLPolicy Structure ❌ NOT IMPLEMENTED
**Spec Requires**:
```go
type TTLPolicy struct {
    DefaultTTL time.Duration
    MaxTTL     time.Duration
    MinTTL     time.Duration
}
```

** Current State**: Simple Policy struct with only TTL field (no min/max validation)

### 2. DefaultPolicies Map ❌ NOT IMPLEMENTED
**Spec Requires**:
```go
var DefaultPolicies = map[string]TTLPolicy{
    "openapi_spec":   DefaultTTL:24h, MaxTTL:7d, MinTTL:1h
    "wasm_module":    DefaultTTL:7d, MaxTTL:30d, MinTTL:24h
    "inventory":      DefaultTTL:1h, MaxTTL:24h, MinTTL:5m
    "auth_token":     DefaultTTL:55m, MaxTTL:1h, MinTTL:5m
}
```

**Current State**: No DefaultPolicies map exists

### 3. SizePolicy and Eviction ⚠️ PARTIAL
**Spec Requires**:
```go
type SizePolicy struct {
    MaxSize       int64
    EvictionPolicy string // "LRU", "LFU", "FIFO"
}
```

**Current State**: eviction.go exists but no SizePolicy struct

### 4. SizePolicies Map ❌ NOT IMPLEMENTED
**Spec Requires**:
```go
var SizePolicies = map[string]SizePolicy{
    "memory": {MaxSize:100MB, EvictionPolicy:"LRU"}
    "disk":   {MaxSize:1GB, EvictionPolicy:"LRU"}
}
```

**Current State**: No SizePolicies map exists

### 5. CacheControl Parsing ❌ NOT IMPLEMENTED
**Spec Requires**:
```go
type CacheControl struct {
    MaxAge       int
    NoCache      bool
    NoStore      bool
    MustRevalidate bool
    ETag         string
    LastModified time.Time
}

func ParseCacheControl(header string) CacheControl
```

**Current State**: No CacheControl struct or ParseCacheControl function

### 6. Validation Strategies ❌ NOT IMPLEMENTED
**Spec Requires**:
```go
type ValidationStrategy string
const (
    ValidationETag     ValidationStrategy = "etag"
    ValidationLastMod  ValidationStrategy = "last-modified"
    ValidationChecksum ValidationStrategy = "checksum"
)

func (c *Cache) Validate(ctx context.Context, key string, strategy ValidationStrategy) (bool, error)
```

**Current State**: No validation strategies or Validate method

### 7. Policy Enforcement ❌ NOT IMPLEMENTED
**Spec Requires**:
```go
func (c *Cache) SetWithPolicy(ctx context.Context, key string, value Value, policy string) error {
    // Validates against TTLPolicy (MinTTL, MaxTTL)
    // Enforces size limits (maxEntrySize)
    // Applies policy-specific TTL
}
```

**Current State**: No SetWithPolicy method exists

## Remediation Tasks

### Priority 1: Critical Policy Features

**Task 1: Implement TTLPolicy with Validation**
- [ ] pkg/cache/types.go: Add TTLPolicy struct with DefaultTTL, MaxTTL, MinTTL
- [ ] pkg/cache/setter.go: Add ValidateTTL function to ensure MinTTL <= TTL <= MaxTTL
- [ ] pkg/cache/setter.go: Enforce TTL bounds in Set operations

**Task 2: Implement DefaultPolicies Map**
- [ ] pkg/cache/policies.go: Create DefaultPolicies map with 4 policies
- [ ] pkg/cache/policies.go: Add GetPolicy(name string) (TTLPolicy, error)
- [ ] pkg/cache/policies.go: Add SetPolicy(name string, policy TTLPolicy)
- [ ] pkg/cache/policies.go: Test policy retrieval and setting

**Task 3: Implement SizePolicy and SizePolicies**
- [ ] pkg/cache/types.go: Add SizePolicy struct with MaxSize, EvictionPolicy
- [ ] pkg/cache/policies.go: Create SizePolicies map for memory/disk
- [ ] pkg/cache/setter.go: Validate entry size against policy MaxSize
- [ ] pkg/cache/eviction.go: Integrate with SizePolicy enforcement

**Task 4: Implement CacheControl Parsing**
- [ ] pkg/cache/http.go: Add CacheControl struct
- [ ] pkg/cache/http.go: Implement ParseCacheControl(header string) CacheControl
- [ ] pkg/cache/http.go: Add tests for max-age, no-cache, no-store directives
- [ ] pkg/cache/http.go: Document CacheControl header requirements

### Priority 2: Validation and Enforcement

**Task 5: Implement Validation Strategies**
- [ ] pkg/cache/validation.go: Add ValidationStrategy type and constants
- [ ] pkg/cache/validation.go: Implement Validate(ctx, key, strategy) method
- [ ] pkg/cache/validation.go: Add validateETag, validateLastModified, validateChecksum
- [ ] pkg/cache/validation.go: Add validation tests

**Task 6: Implement SetWithPolicy Method**
- [ ] pkg/cache/setter.go: Add SetWithPolicy(ctx, key, value, policy) error
- [ ] pkg/cache/setter.go: Validate TTLPolicy (MinTTL/TTL/MaxTTL enforcement)
- [ ] pkg/cache/setter.go: Validate SizePolicy (maxEntrySize enforcement)
- [ ] pkg/cache/setter.go: Return descriptive errors for policy violations

### Priority 3: Integration and Testing

**Task 7: Add Comprehensive Tests**
- [ ] pkg/cache/policies_test.go: Test all 4 DefaultPolicies
- [ ] pkg/cache/setter_test.go: Test TTL bounds enforcement
- [ ] pkg/cache/size_test.go: Test size limit enforcement
- [ ] pkg/cache/http_test.go: Test CacheControl parsing
- [ ] pkg/cache/validation_test.go: Test all validation strategies

**Task 8: Integration with Existing Code**
- [ ] Update pkg/cache/manager.go to use new policy system
- [ ] Update pkg/cache/tiered.go to enforce TTLPolicy on Set
- [ ] Add policy-based SetWithPolicy to Cache interface
- [ ] Ensure backward compatibility with existing code

## Estimated Effort

- TTLPolicy + DefaultPolicies: 4 hours
- SizePolicy + SizePolicies: 3 hours
- CacheControl parsing: 2 hours
- Validation strategies: 3 hours
- SetWithPolicy enforcement: 3 hours
- Integration and tests: 5 hours

**Total**: 20 hours (2-3 days work)

## Risk Assessment

**Current Risk Level**: ⚠️ MEDIUM

**Risks**:
1. Cache entries may exceed intended TTL bounds
2. No size enforcement可能导致内存/磁盘溢出
3. No HTTP cache control support breaks web caching
4. No validation strategies increase stale data risk

**Mitigations**:
1. Implement TTLPolicy validation first
2. SizePolicy enforcement protects resources
3. CacheControl parsing enables web standards
4. Validation strategies ensure data freshness

## Next Steps

1. Create pkg/cache/policies.go with TTLPolicy and DefaultPolicies
2. Add pkg/cache/http.go for CacheControl parsing
3. Implement pkg/cache/validation.go for validation strategies
4. Add SetWithPolicy with policy enforcement
5. Write comprehensive tests
6. Update manager/tiered to use new policy system
7. Run tests and verify >70% coverage for pkg/cache
