package cache

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Default policies for different data types.
var DefaultPolicies = map[string]TTLPolicy{
	"openapi_spec": {
		DefaultTTL: 24 * time.Hour,
		MaxTTL:     7 * 24 * time.Hour,
		MinTTL:     1 * time.Hour,
	},
	"wasm_module": {
		DefaultTTL: 7 * 24 * time.Hour,
		MaxTTL:     30 * 24 * time.Hour,
		MinTTL:     24 * time.Hour,
	},
	"inventory": {
		DefaultTTL: 1 * time.Hour,
		MaxTTL:     24 * time.Hour,
		MinTTL:     5 * time.Minute,
	},
	"auth_token": {
		DefaultTTL: 55 * time.Minute,
		MaxTTL:     1 * time.Hour,
		MinTTL:     5 * time.Minute,
	},
}

// Size policies for different cache tiers.
var SizePolicies = map[string]SizePolicy{
	"memory": {
		MaxSize:        100 * 1024 * 1024, // 100MB
		EvictionPolicy: "LRU",
	},
	"disk": {
		MaxSize:        1 * 1024 * 1024 * 1024, // 1GB
		EvictionPolicy: "LRU",
	},
}

// policyMutex protects policy maps.
var policyMutex sync.RWMutex

// GetPolicy returns the TTL policy for the given name.
func GetPolicy(name string) (TTLPolicy, error) {
	policyMutex.RLock()
	defer policyMutex.RUnlock()

	policy, ok := DefaultPolicies[name]
	if !ok {
		return TTLPolicy{}, fmt.Errorf("policy not found: %s", name)
	}
	return policy, nil
}

// SetPolicy sets or updates a TTL policy.
func SetPolicy(name string, policy TTLPolicy) error {
	if policy.MinTTL > policy.MaxTTL {
		return errors.New("min TTL cannot be greater than max TTL")
	}
	if policy.DefaultTTL < policy.MinTTL || policy.DefaultTTL > policy.MaxTTL {
		return errors.New("default TTL must be within min/max bounds")
	}

	policyMutex.Lock()
	defer policyMutex.Unlock()

	DefaultPolicies[name] = policy
	return nil
}

// GetSizePolicy returns the size policy for the given tier.
func GetSizePolicy(tier string) (SizePolicy, error) {
	policyMutex.RLock()
	defer policyMutex.RUnlock()

	policy, ok := SizePolicies[tier]
	if !ok {
		return SizePolicy{}, fmt.Errorf("size policy not found: %s", tier)
	}
	return policy, nil
}

// SetSizePolicy sets or updates a size policy.
func SetSizePolicy(tier string, policy SizePolicy) error {
	if policy.MaxSize <= 0 {
		return errors.New("max size must be positive")
	}
	if policy.EvictionPolicy != "LRU" && policy.EvictionPolicy != "LFU" && policy.EvictionPolicy != "FIFO" {
		return errors.New("eviction policy must be LRU, LFU, or FIFO")
	}

	policyMutex.Lock()
	defer policyMutex.Unlock()

	SizePolicies[tier] = policy
	return nil
}

// ValidateTTL checks if a TTL is valid for the given policy.
func ValidateTTL(policyName string, ttl time.Duration) error {
	policy, err := GetPolicy(policyName)
	if err != nil {
		return err
	}
	return policy.ValidateTTL(ttl)
}

// GetDefaultTTL returns the default TTL for the given policy.
func GetDefaultTTL(policyName string) (time.Duration, error) {
	policy, err := GetPolicy(policyName)
	if err != nil {
		return 0, err
	}
	return policy.DefaultTTL, nil
}

// ParseCacheControl parses an HTTP Cache-Control header.
func ParseCacheControl(header string) CacheControl {
	cc := CacheControl{}
	if header == "" {
		return cc
	}

	directives := strings.Split(header, ",")
	for _, d := range directives {
		d = strings.TrimSpace(d)
		d = strings.ToLower(d)

		switch {
		case d == "no-cache":
			cc.NoCache = true
		case d == "no-store":
			cc.NoStore = true
		case d == "must-revalidate":
			cc.MustRevalidate = true
		case strings.HasPrefix(d, "max-age="):
			if val, err := strconv.Atoi(strings.TrimPrefix(d, "max-age=")); err == nil {
				cc.MaxAge = val
			}
		}
	}

	return cc
}

// ShouldCache returns true if the Cache-Control allows caching.
func (cc CacheControl) ShouldCache() bool {
	return !cc.NoStore
}

// EffectiveTTL returns the effective TTL based on Cache-Control directives.
func (cc CacheControl) EffectiveTTL(defaultTTL time.Duration) time.Duration {
	if cc.NoStore {
		return 0
	}
	if cc.MaxAge > 0 {
		return time.Duration(cc.MaxAge) * time.Second
	}
	if cc.NoCache {
		return 0
	}
	return defaultTTL
}

// ValidateSize checks if an entry size is valid for the given tier.
func ValidateSize(tier string, size int64) error {
	policy, err := GetSizePolicy(tier)
	if err != nil {
		return err
	}
	if size > policy.MaxSize {
		return fmt.Errorf("entry size %d exceeds max size %d for tier %s", size, policy.MaxSize, tier)
	}
	return nil
}

// PolicyEnforcer wraps a cache with policy enforcement.
type PolicyEnforcer struct {
	cache  Cache
	policy string
	tier   string
}

// NewPolicyEnforcer creates a new policy-enforced cache wrapper.
func NewPolicyEnforcer(c Cache, policyName, tier string) (*PolicyEnforcer, error) {
	if _, err := GetPolicy(policyName); err != nil {
		return nil, err
	}
	if _, err := GetSizePolicy(tier); err != nil {
		return nil, err
	}

	return &PolicyEnforcer{
		cache:  c,
		policy: policyName,
		tier:   tier,
	}, nil
}

// SetWithPolicy sets a value with policy enforcement.
func (pe *PolicyEnforcer) SetWithPolicy(ctx context.Context, key string, value Value, policyName string) error {
	policy, err := GetPolicy(policyName)
	if err != nil {
		return err
	}

	// Validate TTL
	if err := policy.ValidateTTL(value.TTL); err != nil {
		return fmt.Errorf("TTL validation failed: %w", err)
	}

	// Validate size
	if err := ValidateSize(pe.tier, int64(len(value.Data))); err != nil {
		return err
	}

	return pe.cache.Set(ctx, key, value, WithTTL(value.TTL), WithTags(value.Tags...))
}

// SetWithCacheControl sets a value based on HTTP Cache-Control headers.
func (pe *PolicyEnforcer) SetWithCacheControl(ctx context.Context, key string, value Value, ccHeader string) error {
	cc := ParseCacheControl(ccHeader)

	if !cc.ShouldCache() {
		return nil // Don't cache if no-store
	}

	ttl := cc.EffectiveTTL(value.TTL)
	if ttl == 0 {
		ttl = value.TTL
	}

	// Validate TTL against policy
	if err := ValidateTTL(pe.policy, ttl); err != nil {
		return err
	}

	// Validate size
	if err := ValidateSize(pe.tier, int64(len(value.Data))); err != nil {
		return err
	}

	return pe.cache.Set(ctx, key, value, WithTTL(ttl), WithTags(value.Tags...))
}
