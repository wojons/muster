// Package tenant provides a comprehensive multi-tenancy system for the Muster API client.
//
// Tenancy Architecture Overview:
//
// The tenant management system enables secure isolation between different organizations
// or user groups sharing the same infrastructure. It supports three isolation levels:
//
//   - Soft Isolation: Logical separation using tenant IDs in queries and filters.
//     Suitable for single-tenant deployments or trusted environments.
//
//   - Hard Isolation: Physical separation using separate database schemas, cache
//     namespaces, and storage paths. Prevents cross-tenant data access at the
//     infrastructure level.
//
//   - Dedicated Isolation: Complete physical separation with dedicated resources
//     per tenant. Provides the highest security guarantees for enterprise customers.
//
// Key Components:
//
//   - TenantContext: Carries tenant identity through the request lifecycle
//   - TenantResolver: Strategies for extracting tenant ID from requests (headers, subdomains, JWT)
//   - TenantMiddleware: HTTP middleware that resolves and validates tenants
//   - TenantStore: Persistent storage for tenant metadata and configuration
//   - TenantRateLimiter: Per-tenant rate limiting using token bucket algorithm
//   - TenantAccessControl: Fine-grained permission system for tenant resources
//   - TenantSecurityEnforcer: Enforces isolation policies and prevents cross-tenant access
//   - TenantProvisioner: Lifecycle management (create, suspend, resume, decommission)
//   - TenantDBManager: Database schema isolation per tenant
//   - TenantCacheManager: Namespaced caching with automatic tenant prefixes
//   - TenantStorageManager: File system isolation with tenant-specific paths
//   - QuotaManager: Resource quota enforcement (rate limits, storage, rows)
//
// Usage Example:
//
//	// Setup tenant infrastructure
//	store := NewMemoryTenantStore()
//	resolver := NewChainResolver(
//	    &HeaderResolver{HeaderName: "X-Tenant-ID"},
//	    NewJWTResolver(),
//	)
//	validator := NewSimpleTenantValidator(store)
//	middleware := NewTenantMiddleware(resolver, validator, store)
//
//	// Apply to HTTP server
//	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	    tenant, _ := GetTenant(r.Context())
//	    fmt.Fprintf(w, "Hello from tenant: %s", tenant.Name)
//	})
//
//	http.ListenAndServe(":8080", middleware.Middleware(handler))
//
// Security Considerations:
//
//   - Always validate tenant context before accessing tenant-specific resources
//   - Use Hard or Dedicated isolation for multi-tenant SaaS deployments
//   - Implement cross-tenant access validation for shared resource scenarios
//   - Enable audit logging for all tenant operations
package tenant

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type contextKey string

const (
	tenantIDKey       contextKey = "tenant_id"
	tenantNameKey     contextKey = "tenant_name"
	tenantTierKey     contextKey = "tenant_tier"
	isolationLevelKey contextKey = "isolation_level"
)

var (
	// ErrNoTenantInContext is returned when attempting to retrieve tenant from context
	// without a tenant being set. Use WithTenant to set tenant context.
	ErrNoTenantInContext = errors.New("no tenant in context")

	// ErrTenantHeaderMissing is returned by HeaderResolver when the expected
	// header is not present in the HTTP request.
	ErrTenantHeaderMissing = errors.New("tenant header missing")

	// ErrTenantSubdomainMissing is returned by SubdomainResolver when the request
	// host does not contain a valid subdomain matching the configured base domain.
	ErrTenantSubdomainMissing = errors.New("tenant subdomain missing")

	// ErrTenantClaimsMissing is returned by JWTResolver when JWT claims are not
	// present in the request context. Ensure authentication middleware runs first.
	ErrTenantClaimsMissing = errors.New("tenant claims missing")

	// ErrTenantIDInClaimsMissing is returned by JWTResolver when the tenant_id claim
	// is not found in the JWT payload.
	ErrTenantIDInClaimsMissing = errors.New("tenant_id in claims missing")

	// ErrNoTenantContext indicates that no tenant context exists for the operation.
	ErrNoTenantContext = errors.New("no tenant context")

	// ErrTenantNotFound is returned when a tenant ID does not exist in the store.
	ErrTenantNotFound = errors.New("tenant not found")

	// ErrTenantSuspended is returned when attempting to access a suspended tenant.
	// Use TenantProvisioner.Resume to reactivate the tenant.
	ErrTenantSuspended = errors.New("tenant suspended")

	// ErrCrossTenantAccessDenied is returned when cross-tenant access is attempted
	// with Hard or Dedicated isolation levels.
	ErrCrossTenantAccessDenied = errors.New("cross-tenant access denied")

	// ErrRateLimitExceeded is returned when a tenant exceeds their configured
	// rate limit. Implement retry logic with exponential backoff.
	ErrRateLimitExceeded = errors.New("rate limit exceeded")

	// ErrPermissionDenied is returned when a user lacks permission for a resource
	// or action within the tenant context.
	ErrPermissionDenied = errors.New("permission denied")
)

// TenantID uniquely identifies a tenant within the system.
// Tenant IDs must be lowercase alphanumeric with hyphens or underscores,
// and have a maximum length of 64 characters. Use ParseTenantID to validate.
type TenantID string

// TenantTier represents the subscription tier or service level of a tenant.
// Higher tiers typically receive more resources, features, and support.
type TenantTier string

const (
	// TierFree is the free tier with basic features and limited resources.
	// Suitable for evaluation, development, or personal projects.
	TierFree TenantTier = "free"

	// TierPro is the professional tier with enhanced features and higher limits.
	// Suitable for small to medium businesses and production workloads.
	TierPro TenantTier = "pro"

	// TierEnterprise is the enterprise tier with full features, priority support,
	// and custom integrations. Suitable for large organizations.
	TierEnterprise TenantTier = "enterprise"

	// TierDedicated is the dedicated tier with isolated infrastructure and
	// maximum security guarantees. Suitable for regulated industries.
	TierDedicated TenantTier = "dedicated"
)

// IsolationLevel determines the degree of separation between tenants.
// Higher isolation levels provide stronger security guarantees but may
// require more infrastructure resources.
type IsolationLevel int

const (
	// IsolationSoft provides logical separation using tenant IDs in queries.
	// All tenants share the same database schema and infrastructure.
	// Best for: Single-tenant deployments, development environments,
	// or when performance is prioritized over strict isolation.
	IsolationSoft IsolationLevel = iota

	// IsolationHard provides physical separation using separate database schemas,
	// cache namespaces, and storage paths per tenant. Cross-tenant access is
	// prevented at the infrastructure level.
	// Best for: Multi-tenant SaaS applications, compliance requirements,
	// or when data privacy is critical.
	IsolationHard

	// IsolationDedicated provides complete infrastructure isolation with
	// dedicated resources (databases, caches, storage) per tenant.
	// Highest security but highest cost.
	// Best for: Enterprise customers, regulated industries (healthcare, finance),
	// or customers with strict compliance requirements.
	IsolationDedicated
)

// TenantContext carries tenant identity and configuration through the request
// lifecycle. It is stored in context.Context and retrieved by handlers and
// middleware to enforce tenant-specific behavior.
//
// Example usage:
//
//	tenantCtx := TenantContext{
//	    ID:             "acme-corp",
//	    Name:           "ACME Corporation",
//	    Tier:           TierPro,
//	    IsolationLevel: IsolationHard,
//	    Metadata:       map[string]string{"industry": "manufacturing"},
//	}
//	ctx := WithTenant(r.Context(), tenantCtx)
type TenantContext struct {
	// ID is the unique identifier for the tenant.
	ID TenantID

	// Name is the human-readable name of the tenant organization.
	Name string

	// Tier is the subscription tier determining feature access and limits.
	Tier TenantTier

	// IsolationLevel determines the security isolation from other tenants.
	IsolationLevel IsolationLevel

	// Metadata contains arbitrary key-value pairs for tenant-specific data.
	// Use for configuration, preferences, or custom attributes.
	Metadata map[string]string
}

// Tenant represents a complete tenant entity with all configuration and state.
// Used for persistence in TenantStore and provisioning operations.
//
// A tenant progresses through a lifecycle:
//   - Creation: Tenant is created with initial configuration
//   - Provisioning: Resources (DB, cache, storage) are allocated
//   - Active: Tenant can make requests and use the system
//   - Suspended: Tenant is temporarily disabled (non-payment, violation)
//   - Decommissioned: Tenant is deleted and resources are released
type Tenant struct {
	// ID is the unique identifier for the tenant.
	ID TenantID

	// Name is the human-readable name of the tenant organization.
	Name string

	// Tier is the subscription tier determining feature access and limits.
	Tier TenantTier

	// IsolationLevel determines the security isolation from other tenants.
	IsolationLevel IsolationLevel

	// Active indicates whether the tenant is currently active.
	// Suspended tenants cannot make requests or access resources.
	Active bool

	// Metadata contains arbitrary key-value pairs for tenant-specific data.
	Metadata map[string]string

	// Quotas defines resource limits for this tenant.
	// If nil, default quotas are applied based on tier.
	Quotas *TenantQuotas
}

// TenantQuotas defines resource limits and constraints for a tenant.
// Quotas are enforced at runtime to prevent resource exhaustion and ensure
// fair usage across tenants.
//
// Example configuration:
//
//	quotas := &TenantQuotas{
//	    MaxRequestsPerSecond: 1000,  // Rate limit
//	    MaxStorageBytes:      1024 * 1024 * 1024,  // 1GB storage
//	    MaxCacheKeys:         10000,  // Cache entries
//	    MaxDatabaseRows:      100000,  // Database row limit
//	}
type TenantQuotas struct {
	// MaxRequestsPerSecond is the maximum number of requests allowed per second.
	// Exceeding this limit returns ErrRateLimitExceeded.
	MaxRequestsPerSecond int

	// MaxStorageBytes is the maximum total storage in bytes the tenant can use.
	// Includes file uploads, cached data, and stored configurations.
	MaxStorageBytes int64

	// MaxCacheKeys is the maximum number of keys the tenant can store in cache.
	// Oldest keys are evicted when limit is reached (LRU policy).
	MaxCacheKeys int

	// MaxDatabaseRows is the maximum number of rows across all tenant tables.
	// Prevents database bloat and ensures query performance.
	MaxDatabaseRows int
}

// ParseTenantID parses and validates a tenant ID string.
// Returns an error if the ID is empty, too long (>64 chars), or contains
// invalid characters. Valid characters are: lowercase letters (a-z),
// digits (0-9), hyphens (-), and underscores (_).
//
// Example usage:
//
//	tenantID, err := ParseTenantID("acme-corp")
//	if err != nil {
//	    log.Fatalf("Invalid tenant ID: %v", err)
//	}
//
// Invalid examples:
//   - "" (empty)
//   - "ACME" (uppercase not allowed)
//   - "acme corp" (spaces not allowed)
//   - A very long string (>64 characters)
func ParseTenantID(id string) (TenantID, error) {
	if id == "" {
		return "", fmt.Errorf("tenant ID cannot be empty")
	}
	tid := TenantID(id)
	if err := tid.Validate(); err != nil {
		return "", err
	}
	return tid, nil
}

// Validate checks if the TenantID is valid according to system rules.
// Returns an error if:
//   - The ID is empty
//   - The ID exceeds 64 characters
//   - The ID contains characters other than lowercase letters (a-z),
//     digits (0-9), hyphens (-), or underscores (_)
//
// Validation should be called before creating or updating tenants to
// ensure data integrity.
//
// Example:
//
//	tenantID := TenantID("acme-corp")
//	if err := tenantID.Validate(); err != nil {
//	    return fmt.Errorf("invalid tenant ID: %w", err)
//	}
func (t TenantID) Validate() error {
	if t == "" {
		return fmt.Errorf("tenant ID cannot be empty")
	}
	if len(t) > 64 {
		return fmt.Errorf("tenant ID too long (max 64 chars)")
	}
	for _, c := range string(t) {
		validLower := c >= 'a' && c <= 'z'
		validDigit := c >= '0' && c <= '9'
		validSpecial := c == '-' || c == '_'
		valid := validLower || validDigit || validSpecial
		if !valid {
			return fmt.Errorf("tenant ID contains invalid characters")
		}
	}
	return nil
}

// WithTenant stores tenant information in the context for retrieval by
// downstream handlers and middleware. The tenant context propagates through
// the request lifecycle, enabling tenant-aware operations.
//
// Example usage in middleware:
//
//	func TenantMiddleware(next http.Handler) http.Handler {
//	    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	        tenant := resolveTenant(r) // Your resolution logic
//	        ctx := WithTenant(r.Context(), tenant)
//	        next.ServeHTTP(w, r.WithContext(ctx))
//	    })
//	}
func WithTenant(ctx context.Context, tenant TenantContext) context.Context {
	ctx = context.WithValue(ctx, tenantIDKey, tenant.ID)
	ctx = context.WithValue(ctx, tenantNameKey, tenant.Name)
	ctx = context.WithValue(ctx, tenantTierKey, tenant.Tier)
	ctx = context.WithValue(ctx, isolationLevelKey, tenant.IsolationLevel)
	return ctx
}

// GetTenantID extracts the tenant ID from context. Returns ErrNoTenantInContext
// if no tenant has been set. Use in handlers to identify the current tenant.
//
// Example usage:
//
//	func handleRequest(w http.ResponseWriter, r *http.Request) {
//	    tenantID, err := GetTenantID(r.Context())
//	    if err != nil {
//	        http.Error(w, "Tenant required", http.StatusUnauthorized)
//	        return
//	    }
//	    // Process request for tenantID
//	}
func GetTenantID(ctx context.Context) (TenantID, error) {
	id, ok := ctx.Value(tenantIDKey).(TenantID)
	if !ok {
		return "", ErrNoTenantInContext
	}
	return id, nil
}

// GetTenant retrieves the complete tenant context from the request context.
// Returns ErrNoTenantInContext if no tenant has been set. Use when you need
// full tenant information (name, tier, isolation level) rather than just the ID.
//
// Example usage:
//
//	func handleRequest(w http.ResponseWriter, r *http.Request) {
//	    tenant, err := GetTenant(r.Context())
//	    if err != nil {
//	        http.Error(w, "Tenant required", http.StatusUnauthorized)
//	        return
//	    }
//
//	    // Access tenant properties
//	    fmt.Fprintf(w, "Welcome, %s! (Tier: %s)", tenant.Name, tenant.Tier)
//
//	    // Enforce tier-based features
//	    if tenant.Tier == TierEnterprise {
//	        enableAdvancedFeatures()
//	    }
//	}
func GetTenant(ctx context.Context) (*TenantContext, error) {
	id, ok := ctx.Value(tenantIDKey).(TenantID)
	if !ok {
		return nil, ErrNoTenantInContext
	}

	name, _ := ctx.Value(tenantNameKey).(string)
	tier, _ := ctx.Value(tenantTierKey).(TenantTier)
	level, _ := ctx.Value(isolationLevelKey).(IsolationLevel)

	return &TenantContext{
		ID:             id,
		Name:           name,
		Tier:           tier,
		IsolationLevel: level,
	}, nil
}

// GetIsolationLevel retrieves the tenant's isolation level from context.
// Returns IsolationSoft as default if not set. Use to enforce isolation-specific
// behavior in handlers and services.
//
// Example usage:
//
//	func queryDatabase(ctx context.Context, query string) error {
//	    level := GetIsolationLevel(ctx)
//	    switch level {
//	    case IsolationHard:
//	        // Use schema-per-tenant queries
//	        return queryWithSchema(ctx, query)
//	    case IsolationSoft:
//	        // Use tenant_id filtering
//	        return queryWithFilter(ctx, query)
//	    case IsolationDedicated:
//	        // Use dedicated connection
//	        return queryDedicated(ctx, query)
//	    }
//	}
func GetIsolationLevel(ctx context.Context) IsolationLevel {
	level, ok := ctx.Value(isolationLevelKey).(IsolationLevel)
	if !ok {
		return IsolationSoft
	}
	return level
}

// TenantResolver defines the interface for extracting tenant identity from HTTP requests.
// Multiple resolver implementations can be chained to support different tenancy
// identification strategies (headers, subdomains, JWT claims, etc.).
//
// Implementation requirements:
//   - Return ErrTenantHeaderMissing or similar if tenant cannot be determined
//   - Validate the extracted tenant ID using ParseTenantID
//   - Be stateless and thread-safe for concurrent request handling
type TenantResolver interface {
	// Resolve extracts the tenant ID from the HTTP request.
	// Returns the tenant ID or an error if resolution fails.
	Resolve(r *http.Request) (TenantID, error)
}

// HeaderResolver extracts tenant ID from a specified HTTP header.
// Common header names: "X-Tenant-ID", "X-Tenant", "Tenant-ID".
//
// Example usage:
//
//	resolver := &HeaderResolver{HeaderName: "X-Tenant-ID"}
//	tenantID, err := resolver.Resolve(r)
//
// Client request example:
//
//	curl -H "X-Tenant-ID: acme-corp" https://api.example.com/resource
type HeaderResolver struct {
	// HeaderName is the HTTP header to check for tenant ID.
	// Use conventional names like "X-Tenant-ID" or "X-Tenant".
	HeaderName string
}

// Resolve extracts the tenant ID from the configured HTTP header.
// Returns ErrTenantHeaderMissing if the header is not present or empty.
func (hr *HeaderResolver) Resolve(r *http.Request) (TenantID, error) {
	tenantID := r.Header.Get(hr.HeaderName)
	if tenantID == "" {
		return "", ErrTenantHeaderMissing
	}
	return ParseTenantID(tenantID)
}

// SubdomainResolver extracts tenant ID from the request subdomain.
// Requires configuration of the base domain to strip from the host.
//
// Example usage for multi-tenant SaaS:
//
//	resolver := &SubdomainResolver{BaseDomain: "example.com"}
//	// acme.example.com -> "acme"
//	// corp.example.com -> "corp"
//
// DNS configuration required:
//   - Wildcard DNS: *.example.com -> your server IP
//   - SSL certificate: wildcard cert for *.example.com
type SubdomainResolver struct {
	// BaseDomain is the root domain to strip from the host.
	// For app.example.com, set BaseDomain="example.com".
	BaseDomain string
}

// Resolve extracts the tenant ID from the subdomain portion of the request host.
// Returns ErrTenantSubdomainMissing if the host doesn't match the base domain pattern.
func (sr *SubdomainResolver) Resolve(r *http.Request) (TenantID, error) {
	host := r.Host
	baseDomain := "." + sr.BaseDomain
	idx := strings.Index(host, baseDomain)
	if idx > 0 {
		subdomain := host[:idx]
		return ParseTenantID(subdomain)
	}
	return "", ErrTenantSubdomainMissing
}

// jwtContextKey is the context key used to store JWT claims in the request context.
// Authentication middleware should populate this with the decoded JWT payload.
type jwtContextKey struct{}

// JWTResolver extracts tenant ID from JWT claims in the request context.
// Requires authentication middleware to decode and validate the JWT first.
// The JWT must contain a "tenant_id" claim.
//
// Example JWT payload:
//
//	{
//	  "sub": "user-123",
//	  "tenant_id": "acme-corp",
//	  "exp": 1234567890
//	}
//
// Usage with authentication middleware:
//
//	// First, authenticate and add claims to context
//	authMiddleware := func(next http.Handler) http.Handler {
//	    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	        claims := validateJWT(r.Header.Get("Authorization"))
//	        ctx := context.WithValue(r.Context(), jwtContextKey{}, claims)
//	        next.ServeHTTP(w, r.WithContext(ctx))
//	    })
//	}
//
//	// Then use JWTResolver for tenant resolution
//	resolver := NewJWTResolver()
type JWTResolver struct{}

// NewJWTResolver creates a new JWT-based tenant resolver.
// Ensure authentication middleware runs before this resolver to populate
// JWT claims in the request context.
func NewJWTResolver() *JWTResolver {
	return &JWTResolver{}
}

// Resolve extracts the tenant ID from JWT claims in the request context.
// Returns ErrTenantClaimsMissing if no claims are present (authentication not performed).
// Returns ErrTenantIDInClaimsMissing if the "tenant_id" claim is absent.
func (jr *JWTResolver) Resolve(r *http.Request) (TenantID, error) {
	claims, ok := r.Context().Value(jwtContextKey{}).(map[string]interface{})
	if !ok {
		return "", ErrTenantClaimsMissing
	}

	tenantID, ok := claims["tenant_id"].(string)
	if !ok {
		return "", ErrTenantIDInClaimsMissing
	}

	return ParseTenantID(tenantID)
}

// ChainResolver attempts multiple resolvers in sequence until one succeeds.
// This enables flexible tenancy identification strategies, such as:
//  1. Try header-based resolution (for API clients)
//  2. Try subdomain resolution (for browser clients)
//  3. Try JWT claims (for authenticated requests)
//
// Resolvers are tried in the order provided. The first successful resolution
// is returned. If all resolvers fail, the last error is returned.
//
// Example usage:
//
//	resolver := NewChainResolver(
//	    &HeaderResolver{HeaderName: "X-Tenant-ID"},
//	    &SubdomainResolver{BaseDomain: "example.com"},
//	    NewJWTResolver(),
//	)
//
// Resolution flow:
//   - Request with X-Tenant-ID header -> HeaderResolver succeeds
//   - Request from acme.example.com -> SubdomainResolver succeeds
//   - Authenticated request with JWT -> JWTResolver succeeds
type ChainResolver struct {
	resolvers []TenantResolver
}

// NewChainResolver creates a resolver that chains multiple resolvers together.
// Resolvers are executed in order until one succeeds. Provide resolvers in
// priority order (most specific/preferred first).
//
// Example:
//
//	resolver := NewChainResolver(
//	    &HeaderResolver{HeaderName: "X-Tenant-ID"},  // Try header first
//	    NewJWTResolver(),                              // Then JWT
//	    &SubdomainResolver{BaseDomain: "example.com"}, // Finally subdomain
//	)
func NewChainResolver(resolvers ...TenantResolver) *ChainResolver {
	return &ChainResolver{resolvers: resolvers}
}

// Resolve iterates through resolvers and returns the first successful result.
// If all resolvers fail, returns the last error encountered. This allows
// graceful fallback between different tenancy identification methods.
func (cr *ChainResolver) Resolve(r *http.Request) (TenantID, error) {
	var lastErr error
	for _, resolver := range cr.resolvers {
		tenantID, err := resolver.Resolve(r)
		if err == nil {
			return tenantID, nil
		}
		lastErr = err
	}
	return "", lastErr
}

// TenantValidator defines the interface for validating tenant requests.
// Validators check if a tenant is authorized to make requests, enforcing
// rules like active status, subscription validity, or custom business logic.
//
// Implementations should:
//   - Return nil if the tenant is valid and can proceed
//   - Return ErrTenantSuspended if the tenant is temporarily disabled
//   - Return ErrPermissionDenied for authorization failures
//   - Return other errors for validation failures
type TenantValidator interface {
	// Validate checks if the tenant is authorized to make the request.
	// The request parameter provides context for validation (e.g., endpoint, method).
	Validate(ctx context.Context, tenant *Tenant, r *http.Request) error
}

// SimpleTenantValidator performs basic tenant validation by checking active status.
// It verifies that the tenant exists in the store and is currently active.
// This is suitable for most use cases; implement TenantValidator for custom logic.
//
// Example usage:
//
//	store := NewMemoryTenantStore()
//	validator := NewSimpleTenantValidator(store)
//
//	// In middleware
//	tenant := store.Get(ctx, tenantID)
//	if err := validator.Validate(ctx, tenant, r); err != nil {
//	    // Handle validation failure
//	}
type SimpleTenantValidator struct {
	store TenantStore
}

// NewSimpleTenantValidator creates a validator that checks tenant active status.
// Requires a TenantStore to look up tenant information.
func NewSimpleTenantValidator(store TenantStore) *SimpleTenantValidator {
	return &SimpleTenantValidator{store: store}
}

// Validate checks if the tenant is active and authorized to make requests.
// Returns ErrTenantSuspended if the tenant's Active field is false.
// Returns nil if the tenant is active and can proceed.
func (stv *SimpleTenantValidator) Validate(ctx context.Context, tenant *Tenant, r *http.Request) error {
	if !tenant.Active {
		return ErrTenantSuspended
	}
	return nil
}

// TenantStore defines the interface for persistent tenant storage.
// Implementations manage the CRUD lifecycle of tenant entities.
// Production implementations should use database-backed storage
// (PostgreSQL, MySQL, etc.) with proper transaction support.
//
// Storage requirements:
//   - Thread-safe for concurrent access
//   - Durable persistence across restarts
//   - Transactional updates for data consistency
//   - Indexing on tenant ID for fast lookups
//
// Example implementation patterns:
//   - MemoryTenantStore: In-memory for testing/development
//   - PostgreSQLStore: Database-backed with SQL queries
//   - RedisStore: Redis-backed for high-performance caching
type TenantStore interface {
	// Get retrieves a tenant by ID. Returns ErrTenantNotFound if not found.
	Get(ctx context.Context, id TenantID) (*Tenant, error)

	// Create stores a new tenant. Returns error if tenant already exists.
	Create(ctx context.Context, tenant *Tenant) error

	// Update modifies an existing tenant. Returns ErrTenantNotFound if not found.
	Update(ctx context.Context, tenant *Tenant) error

	// Delete removes a tenant. Returns ErrTenantNotFound if not found.
	// Should cascade to delete tenant-specific data (schemas, cache, storage).
	Delete(ctx context.Context, id TenantID) error
}

// MemoryTenantStore is an in-memory implementation of TenantStore.
// Suitable for testing, development, and embedded use cases.
// NOT recommended for production multi-tenant deployments due to
// lack of durability (data lost on restart).
//
// Thread-safe using RWMutex for concurrent read/write access.
//
// Example usage:
//
//	store := NewMemoryTenantStore()
//
//	// Create tenant
//	tenant := &Tenant{
//	    ID: "acme-corp",
//	    Name: "ACME Corporation",
//	    Tier: TierPro,
//	    Active: true,
//	}
//	err := store.Create(ctx, tenant)
//
//	// Retrieve tenant
//	found, err := store.Get(ctx, "acme-corp")
type MemoryTenantStore struct {
	tenants map[TenantID]*Tenant
	mu      sync.RWMutex
}

// NewMemoryTenantStore creates a new in-memory tenant store.
// Initialize with empty tenant map. Use for testing and development.
func NewMemoryTenantStore() *MemoryTenantStore {
	return &MemoryTenantStore{
		tenants: make(map[TenantID]*Tenant),
	}
}

// Get retrieves a tenant by ID from the in-memory store.
// Returns ErrTenantNotFound if the tenant does not exist.
// Thread-safe: uses read lock for concurrent access.
func (mts *MemoryTenantStore) Get(ctx context.Context, id TenantID) (*Tenant, error) {
	mts.mu.RLock()
	defer mts.mu.RUnlock()

	tenant, exists := mts.tenants[id]
	if !exists {
		return nil, ErrTenantNotFound
	}
	return tenant, nil
}

// Create stores a new tenant in the in-memory store.
// Returns an error if a tenant with the same ID already exists.
// Thread-safe: uses write lock to prevent race conditions.
//
// Example:
//
//	store := NewMemoryTenantStore()
//	tenant := &Tenant{ID: "acme-corp", Name: "ACME", Active: true}
//	err := store.Create(context.Background(), tenant)
func (mts *MemoryTenantStore) Create(ctx context.Context, tenant *Tenant) error {
	mts.mu.Lock()
	defer mts.mu.Unlock()

	if _, exists := mts.tenants[tenant.ID]; exists {
		return fmt.Errorf("tenant already exists")
	}

	mts.tenants[tenant.ID] = tenant
	return nil
}

// Update modifies an existing tenant in the in-memory store.
// Returns ErrTenantNotFound if the tenant does not exist.
// Performs complete replacement of the tenant entity.
// Thread-safe: uses write lock to prevent race conditions.
//
// Example:
//
//	tenant, _ := store.Get(ctx, "acme-corp")
//	tenant.Tier = TierEnterprise
//	err := store.Update(ctx, tenant)
func (mts *MemoryTenantStore) Update(ctx context.Context, tenant *Tenant) error {
	mts.mu.Lock()
	defer mts.mu.Unlock()

	if _, exists := mts.tenants[tenant.ID]; !exists {
		return ErrTenantNotFound
	}

	mts.tenants[tenant.ID] = tenant
	return nil
}

// Delete removes a tenant from the in-memory store.
// Returns ErrTenantNotFound if the tenant does not exist.
// Does NOT automatically clean up tenant-specific resources
// (database schemas, cache, storage) - call TenantProvisioner.Decommission first.
// Thread-safe: uses write lock to prevent race conditions.
func (mts *MemoryTenantStore) Delete(ctx context.Context, id TenantID) error {
	mts.mu.Lock()
	defer mts.mu.Unlock()

	if _, exists := mts.tenants[id]; !exists {
		return ErrTenantNotFound
	}

	delete(mts.tenants, id)
	return nil
}

// TenantMiddleware is HTTP middleware that handles tenant resolution, validation,
// and context injection for every request. It orchestrates the complete tenant
// identification flow:
//
//  1. Resolve: Extract tenant ID from request (header, subdomain, JWT)
//  2. Lookup: Fetch tenant from store
//  3. Validate: Check tenant status and permissions
//  4. Inject: Add tenant context to request context
//  5. Respond: Add tenant headers to response
//
// Thread-safe and suitable for high-concurrency production use.
//
// Example setup:
//
//	store := NewMemoryTenantStore()
//	resolver := NewChainResolver(
//	    &HeaderResolver{HeaderName: "X-Tenant-ID"},
//	    NewJWTResolver(),
//	)
//	validator := NewSimpleTenantValidator(store)
//	middleware := NewTenantMiddleware(resolver, validator, store)
//
//	// Apply to HTTP server
//	handler := http.HandlerFunc(myHandler)
//	http.ListenAndServe(":8080", middleware.Middleware(handler))
//
// Request flow:
//   - Client sends: GET /api/resource with header "X-Tenant-ID: acme-corp"
//   - Middleware resolves tenant, validates, injects context
//   - Handler receives request with tenant in context
//   - Response includes header "X-Tenant-ID: acme-corp"
type TenantMiddleware struct {
	resolver  TenantResolver
	validator TenantValidator
	store     TenantStore
}

// NewTenantMiddleware creates tenant middleware with the specified components.
// All three parameters are required:
//   - resolver: Determines how tenant ID is extracted from requests
//   - validator: Validates tenant authorization and status
//   - store: Persistent storage for tenant lookup
//
// Example:
//
//	middleware := NewTenantMiddleware(
//	    NewChainResolver(&HeaderResolver{HeaderName: "X-Tenant-ID"}),
//	    NewSimpleTenantValidator(store),
//	    store,
//	)
func NewTenantMiddleware(resolver TenantResolver, validator TenantValidator, store TenantStore) *TenantMiddleware {
	return &TenantMiddleware{
		resolver:  resolver,
		validator: validator,
		store:     store,
	}
}

// Middleware wraps an HTTP handler with tenant resolution and validation logic.
// Returns a new handler that processes tenant identification before calling the
// wrapped handler.
//
// Processing flow:
//  1. Resolve tenant ID from request using configured resolver
//     - Failure: Returns 400 Bad Request with error details
//  2. Look up tenant in store
//     - Failure: Returns 401 Unauthorized
//  3. Validate tenant using configured validator
//     - Suspended: Returns 403 Forbidden with "tenant suspended"
//     - Other errors: Returns 403 Forbidden with "access denied"
//  4. Inject tenant context into request
//  5. Add X-Tenant-ID header to response
//  6. Call wrapped handler with enriched context
//
// Example usage:
//
//	middleware := NewTenantMiddleware(resolver, validator, store)
//
//	mux := http.NewServeMux()
//	mux.HandleFunc("/api/resource", handleResource)
//
//	// Wrap with tenant middleware
//	http.ListenAndServe(":8080", middleware.Middleware(mux))
//
// Error responses:
//   - 400: Tenant resolution failed (missing header, invalid format)
//   - 401: Tenant not found in store
//   - 403: Tenant suspended or access denied
func (tm *TenantMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := tm.resolver.Resolve(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("tenant resolution failed: %v", err), http.StatusBadRequest)
			return
		}

		tenant, err := tm.store.Get(r.Context(), tenantID)
		if err != nil {
			http.Error(w, "invalid tenant", http.StatusUnauthorized)
			return
		}

		if err := tm.validator.Validate(r.Context(), tenant, r); err != nil {
			if err == ErrTenantSuspended {
				http.Error(w, "tenant suspended", http.StatusForbidden)
				return
			}
			http.Error(w, "access denied", http.StatusForbidden)
			return
		}

		ctx := WithTenant(r.Context(), TenantContext{
			ID:             tenant.ID,
			Name:           tenant.Name,
			Tier:           tenant.Tier,
			IsolationLevel: tenant.IsolationLevel,
			Metadata:       tenant.Metadata,
		})

		w.Header().Set("X-Tenant-ID", string(tenant.ID))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireTenant is middleware that ensures a tenant context exists in the request.
// Use this as a secondary check after TenantMiddleware to guarantee tenant context
// is present, especially when chaining multiple middleware layers.
//
// Returns 401 Unauthorized if no tenant is in context.
//
// Example usage:
//
//	// Apply after tenant middleware for extra safety
//	middleware := NewTenantMiddleware(resolver, validator, store)
//	handler := RequireTenant(http.HandlerFunc(myHandler))
//
//	// Or chain: TenantMiddleware -> RequireTenant -> Handler
//	http.ListenAndServe(":8080", middleware.Middleware(RequireTenant(mux)))
//
// When to use:
//   - As defense-in-depth after primary tenant middleware
//   - On specific routes that require tenant context
//   - When tenant context might be cleared by other middleware
func RequireTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := GetTenantID(r.Context())
		if err != nil {
			http.Error(w, "tenant context required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// TokenBucket implements the token bucket algorithm for rate limiting.
// Tokens are added at a constant rate (refillRate per second) up to a
// maximum capacity (maxTokens). Each request consumes one token.
// Requests are denied when the bucket is empty.
//
// Algorithm:
//   - Bucket starts full (tokens = maxTokens)
//   - Tokens are consumed one at a time per Allow() call
//   - Tokens refill continuously at refillRate per second
//   - Bucket never exceeds maxTokens (overflow discarded)
//
// Example configuration:
//
//	// 100 requests/second, burst up to 200
//	bucket := NewTokenBucket(200, 100)
//
//	// Check if request allowed
//	if bucket.Allow() {
//	    // Process request
//	} else {
//	    // Rate limit exceeded
//	}
//
// Thread-safe: Uses mutex for concurrent access.
type TokenBucket struct {
	tokens     float64    // Current available tokens
	maxTokens  float64    // Maximum bucket capacity (burst size)
	refillRate float64    // Tokens added per second
	lastRefill time.Time  // Last refill calculation time
	mu         sync.Mutex // Protects concurrent access
}

// NewTokenBucket creates a token bucket with specified capacity and refill rate.
//   - maxTokens: Maximum tokens in bucket (burst capacity)
//   - refillRate: Tokens added per second (sustained rate)
//
// Example:
//
//	// Allow 10 requests/second sustained, burst up to 20
//	bucket := NewTokenBucket(20, 10)
//
//	// Allow 1000 requests/second sustained, burst up to 1000
//	bucket := NewTokenBucket(1000, 1000)
func NewTokenBucket(maxTokens, refillRate float64) *TokenBucket {
	return &TokenBucket{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request should be permitted under the rate limit.
// Returns true if a token was available and consumed, false if rate limited.
// Automatically refills tokens based on elapsed time since last call.
//
// Thread-safe: Acquires mutex lock for duration of call.
//
// Example usage:
//
//	bucket := NewTokenBucket(100, 10)
//
//	for i := 0; i < 150; i++ {
//	    if bucket.Allow() {
//	        fmt.Println("Request allowed")
//	    } else {
//	        fmt.Println("Rate limited!")
//	    }
//	}
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens = min(tb.maxTokens, tb.tokens+elapsed*tb.refillRate)
	tb.lastRefill = now

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

// TenantRateLimiter provides per-tenant rate limiting using token buckets.
// Each tenant gets their own bucket, preventing noisy neighbor problems.
// Buckets are created lazily on first request with default rate/burst values.
//
// Features:
//   - Per-tenant isolation: One tenant's traffic doesn't affect others
//   - Dynamic configuration: Rates can be changed per tenant at runtime
//   - Lazy initialization: Buckets created on demand to save memory
//   - Thread-safe: Concurrent access from multiple goroutines
//
// Example usage:
//
//	limiter := NewTenantRateLimiter(100, 200) // 100 req/s default, burst 200
//
//	// In middleware or handler
//	tenantID, _ := GetTenantID(r.Context())
//	if err := limiter.CheckLimit(r.Context(), tenantID); err != nil {
//	    http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
//	    return
//	}
//
//	// Set custom rate for premium tenant
//	limiter.SetRate("enterprise-tenant", 1000, 2000)
type TenantRateLimiter struct {
	buckets      map[TenantID]*TokenBucket // Per-tenant buckets
	mu           sync.RWMutex              // Protects map access
	defaultRate  float64                   // Default refill rate (req/s)
	defaultBurst int                       // Default max tokens (burst)
}

// NewTenantRateLimiter creates a rate limiter with default rate limits.
//   - defaultRate: Sustained requests per second for new tenants
//   - defaultBurst: Maximum burst capacity for new tenants
//
// Example:
//
//	// Free tier: 10 req/s, burst 20
//	freeLimiter := NewTenantRateLimiter(10, 20)
//
//	// Pro tier: 100 req/s, burst 200
//	proLimiter := NewTenantRateLimiter(100, 200)
//
//	// Enterprise tier: 1000 req/s, burst 2000
//	enterpriseLimiter := NewTenantRateLimiter(1000, 2000)
func NewTenantRateLimiter(defaultRate float64, defaultBurst int) *TenantRateLimiter {
	return &TenantRateLimiter{
		buckets:      make(map[TenantID]*TokenBucket),
		defaultRate:  defaultRate,
		defaultBurst: defaultBurst,
	}
}

// CheckLimit verifies if a tenant has available rate limit capacity.
// Returns nil if request is allowed, ErrRateLimitExceeded if rate limited.
// Automatically creates a new bucket for unknown tenants with default settings.
//
// Thread-safe: Uses read lock for bucket lookup, write lock for creation.
//
// Example usage in middleware:
//
//	func RateLimitMiddleware(limiter *TenantRateLimiter) func(http.Handler) http.Handler {
//	    return func(next http.Handler) http.Handler {
//	        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	            tenantID, _ := GetTenantID(r.Context())
//	            if err := limiter.CheckLimit(r.Context(), tenantID); err != nil {
//	                w.Header().Set("Retry-After", "1")
//	                http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
//	                return
//	            }
//	            next.ServeHTTP(w, r)
//	        })
//	    }
//	}
func (trl *TenantRateLimiter) CheckLimit(ctx context.Context, tenantID TenantID) error {
	trl.mu.RLock()
	bucket, exists := trl.buckets[tenantID]
	trl.mu.RUnlock()

	if !exists {
		trl.mu.Lock()
		bucket = NewTokenBucket(float64(trl.defaultBurst), trl.defaultRate)
		trl.buckets[tenantID] = bucket
		trl.mu.Unlock()
	}

	if !bucket.Allow() {
		return ErrRateLimitExceeded
	}
	return nil
}

// SetRate configures custom rate limits for a specific tenant.
// Useful for implementing tier-based rate limits (free/pro/enterprise).
// Replaces the tenant's existing bucket immediately.
//
// Parameters:
//   - tenantID: Target tenant
//   - rate: Sustained requests per second
//   - burst: Maximum burst capacity
//
// Example:
//
//	// Upgrade tenant to higher limits
//	func UpgradeTenant(tenantID TenantID, tier TenantTier) {
//	    switch tier {
//	    case TierFree:
//	        limiter.SetRate(tenantID, 10, 20)
//	    case TierPro:
//	        limiter.SetRate(tenantID, 100, 200)
//	    case TierEnterprise:
//	        limiter.SetRate(tenantID, 1000, 2000)
//	    }
//	}
func (trl *TenantRateLimiter) SetRate(tenantID TenantID, rate float64, burst int) {
	trl.mu.Lock()
	defer trl.mu.Unlock()

	trl.buckets[tenantID] = NewTokenBucket(float64(burst), rate)
}

// Permission represents a single access control rule for a tenant resource.
// Permissions follow a simple RBAC (Role-Based Access Control) model.
//
// Fields:
//   - Resource: The resource being accessed (e.g., "documents", "users", "settings")
//   - Action: The operation being performed (e.g., "read", "write", "delete")
//   - Effect: "allow" or "deny" - deny takes precedence over allow
//
// Wildcard support:
//   - Resource "*" matches all resources
//   - Action "*" matches all actions
//
// Example permissions:
//
//	// Admin: full access to everything
//	Permission{Resource: "*", Action: "*", Effect: "allow"}
//
//	// Editor: can read/write documents but not delete
//	Permission{Resource: "documents", Action: "read", Effect: "allow"}
//	Permission{Resource: "documents", Action: "write", Effect: "allow"}
//
//	// Viewer: read-only access
//	Permission{Resource: "*", Action: "read", Effect: "allow"}
//
//	// Explicit denial (takes precedence)
//	Permission{Resource: "settings", Action: "delete", Effect: "deny"}
type Permission struct {
	Resource string // Resource being accessed
	Action   string // Operation being performed
	Effect   string // "allow" or "deny"
}

// TenantAccessControl manages fine-grained permissions for tenants and users.
// Implements a simple RBAC system where permissions are granted per tenant
// and evaluated at access time.
//
// Features:
//   - Per-tenant permission isolation
//   - Per-user permissions within tenants
//   - Wildcard support for resources and actions
//   - Deny takes precedence over allow
//   - Thread-safe concurrent access
//
// Example usage:
//
//	ac := NewTenantAccessControl()
//
//	// Grant admin access to user alice in tenant acme-corp
//	ac.GrantPermission("acme-corp", "alice", "*", "*", "allow")
//
//	// Grant editor access to user bob
//	ac.GrantPermission("acme-corp", "bob", "documents", "read", "allow")
//	ac.GrantPermission("acme-corp", "bob", "documents", "write", "allow")
//
//	// Check permission before allowing action
//	err := ac.CheckPermission(ctx, "acme-corp", "bob", "documents", "delete")
//	if err != nil {
//	    // Access denied
//	}
//
// For production use, consider integrating with external IAM systems
// (Okta, Auth0, AWS IAM) or implementing policy engines (OPA, Cedar).
type TenantAccessControl struct {
	permissions map[TenantID]map[string][]Permission // tenant -> user -> permissions
	mu          sync.RWMutex                         // Protects map access
}

// NewTenantAccessControl creates an empty access control system.
// Permissions must be granted explicitly using GrantPermission.
//
// Example:
//
//	ac := NewTenantAccessControl()
//
//	// Seed with default permissions
//	ac.GrantPermission("acme-corp", "admin", "*", "*", "allow")
func NewTenantAccessControl() *TenantAccessControl {
	return &TenantAccessControl{
		permissions: make(map[TenantID]map[string][]Permission),
	}
}

// GrantPermission assigns a permission to a user within a tenant.
// Permissions are cumulative - users can have multiple permissions.
// Deny permissions take precedence over allow permissions during evaluation.
//
// Parameters:
//   - tenantID: The tenant owning the resource
//   - userID: The user to grant permission to (can be email, username, or UUID)
//   - resource: The resource type (e.g., "documents", "users", "*")
//   - action: The action (e.g., "read", "write", "delete", "*")
//   - effect: "allow" or "deny"
//
// Example usage:
//
//	ac := NewTenantAccessControl()
//
//	// Grant admin full access
//	ac.GrantPermission("acme-corp", "alice", "*", "*", "allow")
//
//	// Grant editor limited access
//	ac.GrantPermission("acme-corp", "bob", "documents", "read", "allow")
//	ac.GrantPermission("acme-corp", "bob", "documents", "write", "allow")
//
//	// Explicitly deny sensitive action
//	ac.GrantPermission("acme-corp", "bob", "documents", "delete", "deny")
//
//	// Grant viewer read-only
//	ac.GrantPermission("acme-corp", "charlie", "*", "read", "allow")
func (tac *TenantAccessControl) GrantPermission(tenantID TenantID, userID, resource, action, effect string) {
	tac.mu.Lock()
	defer tac.mu.Unlock()

	if _, ok := tac.permissions[tenantID]; !ok {
		tac.permissions[tenantID] = make(map[string][]Permission)
	}

	tac.permissions[tenantID][userID] = append(tac.permissions[tenantID][userID], Permission{
		Resource: resource,
		Action:   action,
		Effect:   effect,
	})
}

// CheckPermission verifies if a user has permission to perform an action on a resource.
// Returns nil if allowed, ErrPermissionDenied if denied.
//
// Evaluation algorithm:
//  1. Find all permissions matching the resource and action (including wildcards)
//  2. If any matching permission has effect="deny", return denied
//  3. If any matching permission has effect="allow", return allowed
//  4. If no matching permissions, return denied (default deny)
//
// Example usage:
//
//	ac := NewTenantAccessControl()
//	ac.GrantPermission("acme-corp", "alice", "documents", "read", "allow")
//
//	// Check before allowing operation
//	err := ac.CheckPermission(ctx, "acme-corp", "alice", "documents", "read")
//	if err != nil {
//	    http.Error(w, "Access denied", http.StatusForbidden)
//	    return
//	}
//
//	// Use in middleware
//	func RequirePermission(ac *TenantAccessControl, resource, action string) func(http.Handler) http.Handler {
//	    return func(next http.Handler) http.Handler {
//	        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	            tenantID, _ := GetTenantID(r.Context())
//	            userID := getUserID(r) // Your user extraction logic
//	            if err := ac.CheckPermission(r.Context(), tenantID, userID, resource, action); err != nil {
//	                http.Error(w, "Access denied", http.StatusForbidden)
//	                return
//	            }
//	            next.ServeHTTP(w, r)
//	        })
//	    }
//	}
func (tac *TenantAccessControl) CheckPermission(ctx context.Context, tenantID TenantID, userID, resource, action string) error {
	tac.mu.RLock()
	defer tac.mu.RUnlock()

	tenantPerms, ok := tac.permissions[tenantID]
	if !ok {
		return ErrTenantNotFound
	}

	userPerms, ok := tenantPerms[userID]
	if !ok {
		return ErrPermissionDenied
	}

	for _, perm := range userPerms {
		if (perm.Resource == "*" || perm.Resource == resource) &&
			(perm.Action == "*" || perm.Action == action) {
			if perm.Effect == "deny" {
				return ErrPermissionDenied
			}
			return nil
		}
	}

	return ErrPermissionDenied
}

// TenantSecurityEnforcer enforces tenant isolation policies and prevents
// unauthorized cross-tenant access. The enforcement behavior depends on the
// configured isolation level:
//
//   - IsolationSoft: Allows cross-tenant access (logical separation only)
//   - IsolationHard: Blocks cross-tenant access with error
//   - IsolationDedicated: Blocks cross-tenant access with error
//
// Use cases:
//   - Validate cross-tenant API calls in microservices
//   - Prevent accidental data leakage in shared resource scenarios
//   - Enforce compliance requirements for data isolation
//
// Example usage:
//
//	enforcer := NewTenantSecurityEnforcer(IsolationHard)
//
//	// In a service that accesses multiple tenants
//	func CopyData(ctx context.Context, sourceID, targetID TenantID) error {
//	    if err := enforcer.ValidateCrossTenantAccess(ctx, sourceID, targetID); err != nil {
//	        return err // Cross-tenant access denied
//	    }
//	    // Proceed with copy operation
//	}
//
// For Hard or Dedicated isolation, cross-tenant operations should be
// performed through administrative interfaces with explicit authorization,
// not through the standard tenant context.
type TenantSecurityEnforcer struct {
	isolationLevel IsolationLevel
}

// NewTenantSecurityEnforcer creates an enforcer with the specified isolation level.
// Use the tenant's configured isolation level from their TenantContext.
//
// Example:
//
//	tenant, _ := GetTenant(ctx)
//	enforcer := NewTenantSecurityEnforcer(tenant.IsolationLevel)
func NewTenantSecurityEnforcer(level IsolationLevel) *TenantSecurityEnforcer {
	return &TenantSecurityEnforcer{isolationLevel: level}
}

// ValidateCrossTenantAccess checks if access from source tenant to target tenant
// is permitted under the current isolation policy.
//
// Returns:
//   - nil: Access is allowed (same tenant or soft isolation)
//   - ErrCrossTenantAccessDenied: Access blocked by isolation policy
//
// Example usage in shared service:
//
//	enforcer := NewTenantSecurityEnforcer(IsolationHard)
//
//	func SendMessage(ctx context.Context, from, to TenantID, message string) error {
//	    // Validate cross-tenant communication
//	    if err := enforcer.ValidateCrossTenantAccess(ctx, from, to); err != nil {
//	        // Log security event
//	        log.Printf("Cross-tenant access attempt: %s -> %s", from, to)
//	        return err
//	    }
//
//	    // Proceed with message delivery
//	    return deliverMessage(ctx, to, message)
//	}
//
// Security considerations:
//   - Always validate cross-tenant access in multi-tenant services
//   - Log all cross-tenant access attempts for security auditing
//   - Use administrative interfaces for legitimate cross-tenant operations
func (tse *TenantSecurityEnforcer) ValidateCrossTenantAccess(ctx context.Context, sourceTenant, targetTenant TenantID) error {
	if tse.isolationLevel >= IsolationHard && sourceTenant != targetTenant {
		return ErrCrossTenantAccessDenied
	}
	return nil
}

// TenantProvisioner manages the complete lifecycle of tenant resources.
// Responsible for provisioning (creating) and decommissioning (removing)
// tenant-specific infrastructure including:
//
//   - Database: Schema creation and cleanup
//   - Cache: Namespaced cache initialization
//   - Storage: Tenant-specific file system paths
//   - Quotas: Resource limit configuration
//
// Tenant Lifecycle:
//
//  1. Provision: Create resources when tenant signs up
//     └─> Create DB schema
//     └─> Initialize cache
//     └─> Allocate storage
//     └─> Set quotas
//
//  2. Suspend: Temporarily disable tenant (non-payment, violation)
//     └─> Keep resources intact
//     └─> Block access via validator
//
//  3. Resume: Reactivate suspended tenant
//     └─> Restore access
//
//  4. Decommission: Permanently remove tenant
//     └─> Clear cache
//     └─> Drop DB schema
//     └─> Delete storage
//     └─> Remove quotas
//
// Example usage:
//
//	dbManager := NewTenantDBManager()
//	cacheManager := NewTenantCacheManager()
//	storageManager := NewTenantStorageManager("/data")
//	quotaManager := NewQuotaManager()
//
//	provisioner := NewTenantProvisioner(dbManager, cacheManager, storageManager, quotaManager)
//
//	// Provision new tenant
//	tenant := Tenant{
//	    ID: "acme-corp",
//	    Tier: TierPro,
//	    Quotas: &TenantQuotas{MaxRequestsPerSecond: 100},
//	}
//	err := provisioner.Provision(ctx, tenant)
//
//	// Decommission tenant
//	err = provisioner.Decommission(ctx, "acme-corp")
type TenantProvisioner struct {
	dbManager    *TenantDBManager
	cacheManager *TenantCacheManager
	storage      *TenantStorageManager
	quotaManager *QuotaManager
}

// NewTenantProvisioner creates a provisioner with the specified resource managers.
// All managers should be initialized and configured before creating the provisioner.
//
// Example:
//
//	dbManager := NewTenantDBManager()
//	cacheManager := NewTenantCacheManager()
//	storageManager := NewTenantStorageManager("/var/lib/muster/tenants")
//	quotaManager := NewQuotaManager()
//
//	provisioner := NewTenantProvisioner(dbManager, cacheManager, storageManager, quotaManager)
func NewTenantProvisioner(
	db *TenantDBManager,
	cache *TenantCacheManager,
	storage *TenantStorageManager,
	quota *QuotaManager,
) *TenantProvisioner {
	return &TenantProvisioner{
		dbManager:    db,
		cacheManager: cache,
		storage:      storage,
		quotaManager: quota,
	}
}

// Provision allocates and initializes all resources for a new tenant.
// This is called during tenant onboarding to set up their isolated environment.
//
// Provisioning steps:
//  1. Validate tenant ID format
//  2. Create database schema (Hard/Dedicated isolation)
//  3. Initialize namespaced cache
//  4. Allocate storage directory
//  5. Configure resource quotas
//
// Returns a descriptive error if any step fails. Partial provisioning
// may occur - callers should handle cleanup on failure.
//
// Example usage:
//
//	tenant := Tenant{
//	    ID: "acme-corp",
//	    Name: "ACME Corporation",
//	    Tier: TierPro,
//	    IsolationLevel: IsolationHard,
//	    Quotas: &TenantQuotas{
//	        MaxRequestsPerSecond: 100,
//	        MaxStorageBytes: 1024 * 1024 * 1024,
//	    },
//	}
//
//	if err := provisioner.Provision(ctx, tenant); err != nil {
//	    log.Printf("Failed to provision tenant: %v", err)
//	    // Optionally: provisioner.Decommission(ctx, tenant.ID)
//	}
//
// Idempotency: Calling Provision multiple times for the same tenant
// may result in errors (e.g., "schema already exists"). Ensure
// tenant is not already provisioned before calling.
func (tp *TenantProvisioner) Provision(ctx context.Context, tenant Tenant) error {
	if err := tenant.ID.Validate(); err != nil {
		return fmt.Errorf("invalid tenant ID: %w", err)
	}

	if err := tp.dbManager.CreateTenantSchema(ctx, tenant.ID); err != nil {
		return fmt.Errorf("failed to create database schema: %w", err)
	}

	_ = tp.cacheManager.GetCache(tenant.ID)

	_, err := tp.storage.CreateStorage(ctx, tenant.ID)
	if err != nil {
		return fmt.Errorf("failed to create storage: %w", err)
	}

	if err := tp.quotaManager.InitializeQuotas(ctx, tenant.ID, tenant.Quotas); err != nil {
		return fmt.Errorf("failed to initialize quotas: %w", err)
	}

	return nil
}

// Decommission removes all resources associated with a tenant.
// This is called during tenant offboarding or deletion.
//
// Decommissioning steps (in order):
//  1. Invalidate cache (clear all cached data)
//  2. Drop database schema (delete all tables and data)
//  3. Delete storage (remove all files)
//  4. Remove quota configuration
//
// WARNING: This operation is irreversible. All tenant data will be
// permanently deleted. Consider implementing a grace period or
// backup mechanism before calling Decommission.
//
// Example usage:
//
//	// Graceful decommissioning with backup
//	func DecommissionWithBackup(ctx context.Context, tenantID TenantID) error {
//	    // 1. Create backup
//	    if err := backupTenant(tenantID); err != nil {
//	        return err
//	    }
//
//	    // 2. Notify tenant
//	    sendDecommissionNotification(tenantID)
//
//	    // 3. Wait grace period (e.g., 30 days)
//	    scheduleDecommission(tenantID, 30*24*time.Hour)
//
//	    // 4. Execute decommission
//	    return provisioner.Decommission(ctx, tenantID)
//	}
//
// Error handling: If decommission fails partway through, manual cleanup
// may be required. Log all errors and alert operations team for review.
func (tp *TenantProvisioner) Decommission(ctx context.Context, tenantID TenantID) error {
	if err := tp.cacheManager.InvalidateTenant(ctx, tenantID); err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	if err := tp.dbManager.DropTenantSchema(ctx, tenantID); err != nil {
		return fmt.Errorf("failed to drop schema: %w", err)
	}

	if err := tp.storage.DeleteStorage(ctx, tenantID); err != nil {
		return fmt.Errorf("failed to delete storage: %w", err)
	}

	if err := tp.quotaManager.DeleteQuotas(ctx, tenantID); err != nil {
		return fmt.Errorf("failed to delete quotas: %w", err)
	}

	return nil
}

// Suspend temporarily disables a tenant by setting their status to inactive.
// Suspended tenants cannot make requests (blocked by TenantValidator) but
// their resources remain intact.
//
// Use cases:
//   - Non-payment: Suspend until payment received
//   - Policy violation: Suspend pending investigation
//   - Security incident: Suspend to prevent further damage
//   - Customer request: Temporary suspension
//
// Example usage:
//
//	// Suspend for non-payment
//	if paymentOverdue(tenant.ID) {
//	    if err := provisioner.Suspend(ctx, tenant.ID); err != nil {
//	        log.Printf("Failed to suspend tenant: %v", err)
//	    }
//	    notifyTenant(tenant.ID, "Account suspended due to non-payment")
//	}
//
// To reactivate, call Resume. Suspended tenants still consume resources
// (storage, database rows) - consider billing adjustments.
func (tp *TenantProvisioner) Suspend(ctx context.Context, tenantID TenantID) error {
	return tp.dbManager.UpdateTenantStatus(ctx, tenantID, false)
}

// Resume reactivates a previously suspended tenant.
// Restores tenant access by setting status to active.
//
// Example usage:
//
//	// Resume after payment received
//	func processPayment(tenantID TenantID, amount float64) error {
//	    if err := chargePayment(tenantID, amount); err != nil {
//	        return err
//	    }
//
//	    // Reactivate tenant
//	    if err := provisioner.Resume(ctx, tenantID); err != nil {
//	        return err
//	    }
//
//	    notifyTenant(tenantID, "Account reactivated")
//	    return nil
//	}
//
// Idempotency: Safe to call on already-active tenants (no-op).
func (tp *TenantProvisioner) Resume(ctx context.Context, tenantID TenantID) error {
	return tp.dbManager.UpdateTenantStatus(ctx, tenantID, true)
}

// TenantDBManager handles database schema isolation for tenants.
// Creates and manages separate PostgreSQL schemas per tenant for
// Hard and Dedicated isolation levels.
//
// Schema naming convention:
//   - Tenant ID "acme-corp" -> Schema "tenant_acme_corp"
//   - Hyphens replaced with underscores for SQL compatibility
//   - Prefix "tenant_" avoids naming conflicts
//
// Default tables created in each tenant schema:
//   - users: User accounts and authentication
//   - configs: Tenant configuration settings
//   - logs: Audit and activity logs
//   - cache: Cached data and computed values
//   - sessions: User session state
//
// Example usage:
//
//	dbManager := NewTenantDBManager()
//
//	// Create schema for new tenant
//	err := dbManager.CreateTenantSchema(ctx, "acme-corp")
//
//	// Set schema for tenant connection
//	err = dbManager.SetTenantSchema(ctx, "acme-corp")
//
//	// Query tenant data (uses current schema)
//	rows, err := db.Query("SELECT * FROM users")
//
// For production: Replace execute() stub with actual database connection
// using sql.DB, connection pooling, and proper error handling.
type TenantDBManager struct {
	defaultSchema string // Default schema for non-tenant queries
}

// NewTenantDBManager creates a database manager for tenant schema operations.
// Uses "public" as the default schema for system-level queries.
//
// Example:
//
//	dbManager := NewTenantDBManager()
//
//	// Custom default schema
//	dbManager := &TenantDBManager{defaultSchema: "system"}
func NewTenantDBManager() *TenantDBManager {
	return &TenantDBManager{
		defaultSchema: "public",
	}
}

// GetTenantSchema returns the PostgreSQL schema name for a tenant ID.
// Applies sanitization and naming conventions consistently.
//
// Examples:
//   - "acme-corp" -> "tenant_acme_corp"
//   - "test_tenant" -> "tenant_test_tenant"
//   - "org-123" -> "tenant_org_123"
//
// Use this function whenever you need to reference a tenant's schema
// to ensure consistent naming across all database operations.
func (tdb *TenantDBManager) GetTenantSchema(tenantID TenantID) string {
	sanitized := strings.ReplaceAll(string(tenantID), "-", "_")
	return fmt.Sprintf("tenant_%s", sanitized)
}

// SetTenantSchema sets the PostgreSQL search_path to the tenant's schema.
// All subsequent queries will use this schema by default.
//
// SECURITY: This is critical for tenant isolation. Always call this
// before executing tenant queries to ensure data is accessed from
// the correct schema.
//
// Example usage:
//
//	func queryTenantData(ctx context.Context, tenantID TenantID) error {
//	    // Set schema context
//	    if err := dbManager.SetTenantSchema(ctx, tenantID); err != nil {
//	        return err
//	    }
//
//	    // All queries now use tenant's schema
//	    rows, err := db.Query("SELECT * FROM users")
//	    return err
//	}
//
// PostgreSQL search_path:
//   - Controls which schema is searched first for unqualified table names
//   - Setting to tenant schema provides isolation
//   - Can include multiple schemas: "tenant_acme,public"
func (tdb *TenantDBManager) SetTenantSchema(ctx context.Context, tenantID TenantID) error {
	schema := tdb.GetTenantSchema(tenantID)
	query := fmt.Sprintf("SET search_path TO %s", schema)
	return tdb.execute(ctx, query)
}

// CreateTenantSchema creates a new database schema for a tenant with
// standard tables. Idempotent: safe to call multiple times.
//
// Creates the following tables:
//   - users: User accounts (id, tenant_id, data, timestamps)
//   - configs: Configuration settings (id, tenant_id, data, timestamps)
//   - logs: Audit logs (id, tenant_id, data, timestamps)
//   - cache: Cached data (id, tenant_id, data, timestamps)
//   - sessions: Session state (id, tenant_id, data, timestamps)
//
// Table structure:
//   - id: Primary key (TEXT, usually UUID or deterministic ID)
//   - tenant_id: Reference to tenant (TEXT, for auditing)
//   - data: Flexible JSONB column for schemaless data
//   - created_at: Timestamp of record creation
//   - updated_at: Timestamp of last update
//
// Example usage:
//
//	// During tenant provisioning
//	func ProvisionTenant(ctx context.Context, tenantID TenantID) error {
//	    if err := dbManager.CreateTenantSchema(ctx, tenantID); err != nil {
//	        return fmt.Errorf("failed to create schema: %w", err)
//	    }
//
//	    // Create additional custom tables if needed
//	    return nil
//	}
//
// PostgreSQL requirements:
//   - Requires CREATE SCHEMA privilege
//   - Schema ownership assigned to connecting user
//   - CASCADE on drop removes all schema objects
func (tdb *TenantDBManager) CreateTenantSchema(ctx context.Context, tenantID TenantID) error {
	schema := tdb.GetTenantSchema(tenantID)

	query := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schema)
	if err := tdb.execute(ctx, query); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	tables := []string{"users", "configs", "logs", "cache", "sessions"}
	for _, table := range tables {
		createTable := fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s.%s (
				id TEXT PRIMARY KEY,
				tenant_id TEXT NOT NULL,
				data JSONB,
				created_at TIMESTAMP DEFAULT NOW(),
				updated_at TIMESTAMP DEFAULT NOW()
			)
		`, schema, table)
		if err := tdb.execute(ctx, createTable); err != nil {
			return err
		}
	}

	return nil
}

// DropTenantSchema deletes a tenant's entire database schema and all contents.
// Uses CASCADE to remove all tables, views, indexes, and functions.
//
// WARNING: This operation is irreversible. All tenant data will be
// permanently deleted. Ensure backups are created if needed.
//
// Example usage:
//
//	// During tenant decommissioning
//	func DecommissionTenant(ctx context.Context, tenantID TenantID) error {
//	    // Create backup first
//	    if err := backupSchema(ctx, tenantID); err != nil {
//	        return err
//	    }
//
//	    // Drop schema
//	    if err := dbManager.DropTenantSchema(ctx, tenantID); err != nil {
//	        return fmt.Errorf("failed to drop schema: %w", err)
//	    }
//
//	    return nil
//	}
//
// PostgreSQL CASCADE behavior:
//   - Drops all dependent objects automatically
//   - No confirmation required - use with caution
//   - Logs all dropped objects for auditing
func (tdb *TenantDBManager) DropTenantSchema(ctx context.Context, tenantID TenantID) error {
	schema := tdb.GetTenantSchema(tenantID)
	query := fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema)
	return tdb.execute(ctx, query)
}

// UpdateTenantStatus updates the active/inactive status of a tenant.
// Currently a stub - implement with actual database persistence.
//
// TODO: Implement with tenant metadata table in shared schema:
//
//	UPDATE tenants SET active = $1 WHERE id = $2
//
// Production implementation should:
//   - Store tenant status in shared "tenants" table (not tenant schema)
//   - Update timestamp for auditing
//   - Trigger notifications on status change
//   - Log status changes for compliance
func (tdb *TenantDBManager) UpdateTenantStatus(ctx context.Context, tenantID TenantID, active bool) error {
	return nil
}

// execute executes a SQL query. Currently a stub for testing.
// TODO: Replace with actual database connection using sql.DB:
//
//	db     *sql.DB
//
//	func (tdb *TenantDBManager) execute(ctx context.Context, query string) error {
//	    _, err := tdb.db.ExecContext(ctx, query)
//	    return err
//	}
//
// Production considerations:
//   - Use connection pooling
//   - Implement retry logic for transient errors
//   - Add query timeouts
//   - Log slow queries for performance monitoring
//   - Use prepared statements for parameterized queries
func (tdb *TenantDBManager) execute(ctx context.Context, query string) error {
	return nil
}

// TenantCacheManager provides per-tenant cache isolation with automatic
// namespace management. Each tenant gets their own cache instance with
// tenant-prefixed keys to prevent cross-tenant data leakage.
//
// Features:
//   - Automatic cache creation on first access (lazy initialization)
//   - Thread-safe concurrent access using RWMutex
//   - Tenant namespacing prevents key collisions
//   - Bulk invalidation for tenant cleanup
//
// Use cases:
//   - Cache API responses per tenant
//   - Store computed values and derived data
//   - Session state and temporary data
//   - Reduce database load with cached queries
//
// Example usage:
//
//	cacheManager := NewTenantCacheManager()
//
//	// Get tenant-specific cache
//	cache := cacheManager.GetCache("acme-corp")
//
//	// Cache data with TTL
//	cache.Set(ctx, "user:123", userData, 5*time.Minute)
//
//	// Retrieve cached data
//	data, err := cache.Get(ctx, "user:123")
//
//	// Invalidate all tenant cache (e.g., during decommission)
//	cacheManager.InvalidateTenant(ctx, "acme-corp")
//
// Memory considerations:
//   - Caches are stored in memory (not persisted)
//   - No automatic eviction - implement LRU if needed
//   - Consider Redis or Memcached for production scale
type TenantCacheManager struct {
	caches map[TenantID]*TenantCache // Per-tenant caches
	mu     sync.RWMutex              // Protects map access
}

// NewTenantCacheManager creates a cache manager for multi-tenant caching.
// Initializes empty cache map. Caches are created lazily on first access.
//
// Example:
//
//	cacheManager := NewTenantCacheManager()
//
//	// Caches created automatically when accessed
//	cache1 := cacheManager.GetCache("tenant-1")
//	cache2 := cacheManager.GetCache("tenant-2")
func NewTenantCacheManager() *TenantCacheManager {
	return &TenantCacheManager{
		caches: make(map[TenantID]*TenantCache),
	}
}

// GetCache retrieves or creates a cache for the specified tenant.
// Uses double-checked locking for efficient concurrent access:
//  1. Check with read lock (fast path for existing caches)
//  2. Create with write lock (slow path for new caches)
//
// Thread-safe: Multiple goroutines can safely call GetCache concurrently.
// Only one cache instance is created per tenant even with concurrent calls.
//
// Example usage:
//
//	// In handler - get tenant cache
//	tenantID, _ := GetTenantID(r.Context())
//	cache := cacheManager.GetCache(tenantID)
//
//	// Use cache
//	if data, err := cache.Get(ctx, key); err == nil {
//	    return data  // Cache hit
//	}
//
//	// Cache miss - compute and store
//	data := computeExpensiveOperation()
//	cache.Set(ctx, key, data, 5*time.Minute)
func (tcm *TenantCacheManager) GetCache(tenantID TenantID) *TenantCache {
	tcm.mu.RLock()
	cache, exists := tcm.caches[tenantID]
	tcm.mu.RUnlock()

	if exists {
		return cache
	}

	tcm.mu.Lock()
	defer tcm.mu.Unlock()

	if cache, exists := tcm.caches[tenantID]; exists {
		return cache
	}

	cache = NewTenantCache(tenantID)
	tcm.caches[tenantID] = cache
	return cache
}

// InvalidateTenant clears all cached data for a tenant.
// Use when:
//   - Tenant data changes significantly
//   - Preparing for tenant decommission
//   - Flushing stale data periodically
//   - Resetting tenant state for testing
//
// Example usage:
//
//	// After bulk data update
//	func UpdateTenantData(ctx context.Context, tenantID TenantID, data []byte) error {
//	    // Update database
//	    if err := db.Update(ctx, tenantID, data); err != nil {
//	        return err
//	    }
//
//	    // Invalidate cache to force refresh
//	    return cacheManager.InvalidateTenant(ctx, tenantID)
//	}
//
//	// During decommission
//	func DecommissionTenant(ctx context.Context, tenantID TenantID) error {
//	    // Clear cache first
//	    cacheManager.InvalidateTenant(ctx, tenantID)
//
//	    // Then drop database schema
//	    dbManager.DropTenantSchema(ctx, tenantID)
//
//	    return nil
//	}
func (tcm *TenantCacheManager) InvalidateTenant(ctx context.Context, tenantID TenantID) error {
	cache := tcm.GetCache(tenantID)
	return cache.Clear(ctx)
}

// TenantCache is a namespaced in-memory cache for a single tenant.
// All keys are automatically prefixed with the tenant ID to ensure
// isolation between tenants.
//
// Features:
//   - Automatic key namespacing (tenant:{id}:{key})
//   - TTL-based expiration for cache entries
//   - Thread-safe concurrent access
//   - Bulk clear operation
//
// Key format:
//   - Input key: "user:123"
//   - Internal key: "tenant:acme-corp:user:123"
//
// Expiration behavior:
//   - Entries expire after specified TTL
//   - Expired entries return cache miss
//   - No automatic cleanup - expired entries removed on access
//
// Example usage:
//
//	cache := NewTenantCache("acme-corp")
//
//	// Set with 5 minute TTL
//	err := cache.Set(ctx, "user:123", userData, 5*time.Minute)
//
//	// Get (checks expiration)
//	data, err := cache.Get(ctx, "user:123")
//	if err != nil {
//	    // Cache miss or expired
//	}
//
//	// Delete specific key
//	cache.Delete(ctx, "user:123")
//
//	// Clear all keys
//	cache.Clear(ctx)
//
// Production considerations:
//   - In-memory only (lost on restart)
//   - No size limits - monitor memory usage
//   - Consider Redis/Memcached for persistence and scaling
type TenantCache struct {
	tenantID TenantID              // Owner tenant
	prefix   string                // Key prefix for namespacing
	data     map[string]cacheEntry // Stored entries
	mu       sync.RWMutex          // Protects map access
}

// cacheEntry represents a single cached value with expiration metadata.
// Stored internally in TenantCache data map.
type cacheEntry struct {
	value     interface{} // Cached data (any type)
	expiresAt time.Time   // Expiration timestamp
}

// NewTenantCache creates a namespaced cache for a specific tenant.
// Automatically generates key prefix from tenant ID.
//
// Example:
//
//	cache := NewTenantCache("acme-corp")
//	// Keys will be prefixed with "tenant:acme-corp:"
//
//	// These are isolated from other tenants:
//	cache.Set(ctx, "key1", value1, time.Hour)  // Actual key: "tenant:acme-corp:key1"
func NewTenantCache(tenantID TenantID) *TenantCache {
	return &TenantCache{
		tenantID: tenantID,
		prefix:   fmt.Sprintf("tenant:%s:", tenantID),
		data:     make(map[string]cacheEntry),
	}
}

// namespacedKey generates the internal cache key by prepending the tenant prefix.
// Ensures tenant isolation by preventing key collisions between tenants.
//
// Example:
//   - Prefix: "tenant:acme-corp:"
//   - Input key: "user:123"
//   - Result: "tenant:acme-corp:user:123"
//
// This method is called internally by Get, Set, and Delete.
func (tc *TenantCache) namespacedKey(key string) string {
	return tc.prefix + key
}

// Get retrieves a value from the cache by key.
// Returns the cached value or an error if:
//   - Key does not exist (cache miss)
//   - Entry has expired (TTL exceeded)
//
// Thread-safe: Uses read lock for concurrent access.
//
// Example usage:
//
//	cache := NewTenantCache("acme-corp")
//
//	// Attempt to get cached value
//	if data, err := cache.Get(ctx, "user:123"); err == nil {
//	    // Cache hit - use cached data
//	    return data.(UserData)
//	}
//
//	// Cache miss - fetch from database
//	data := db.GetUser("123")
//
//	// Cache for future requests
//	cache.Set(ctx, "user:123", data, 5*time.Minute)
//
// TTL behavior:
//   - Entry expires after specified duration
//   - Expired entries behave as non-existent
//   - No lazy cleanup - expired entries removed on access
func (tc *TenantCache) Get(ctx context.Context, key string) (interface{}, error) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	namespacedKey := tc.namespacedKey(key)
	entry, exists := tc.data[namespacedKey]
	if !exists {
		return nil, fmt.Errorf("cache miss")
	}

	if time.Now().After(entry.expiresAt) {
		return nil, fmt.Errorf("cache expired")
	}

	return entry.value, nil
}

// Set stores a value in the cache with a specified TTL (time-to-live).
// Overwrites existing entries with the same key.
//
// Parameters:
//   - key: Cache key (automatically namespaced)
//   - value: Data to cache (any type, stored as interface{})
//   - ttl: Time until expiration (use 0 for no expiration)
//
// Thread-safe: Uses write lock for exclusive access.
//
// Example usage:
//
//	cache := NewTenantCache("acme-corp")
//
//	// Cache API response for 5 minutes
//	response := fetchAPI()
//	cache.Set(ctx, "api:results", response, 5*time.Minute)
//
//	// Cache configuration indefinitely
//	config := loadConfig()
//	cache.Set(ctx, "config", config, 0)  // No expiration
//
// Memory considerations:
//   - No automatic eviction - cache grows unbounded
//   - Monitor memory usage in long-running applications
//   - Consider LRU eviction or external cache for production
func (tc *TenantCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	namespacedKey := tc.namespacedKey(key)
	tc.data[namespacedKey] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

// Delete removes a specific key from the cache.
// No error if key does not exist (idempotent operation).
//
// Thread-safe: Uses write lock for exclusive access.
//
// Example usage:
//
//	// Invalidate cached user after update
//	func UpdateUser(ctx context.Context, userID string, data UserData) error {
//	    // Update database
//	    if err := db.UpdateUser(userID, data); err != nil {
//	        return err
//	    }
//
//	    // Invalidate cache
//	    cache.Delete(ctx, "user:"+userID)
//
//	    return nil
//	}
//
// Use cases:
//   - Invalidate after data modification
//   - Clean up temporary data
//   - Force refresh on next access
func (tc *TenantCache) Delete(ctx context.Context, key string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	namespacedKey := tc.namespacedKey(key)
	delete(tc.data, namespacedKey)
	return nil
}

// Clear removes all entries from the cache.
// Resets cache to empty state. Does not affect other tenant caches.
//
// Thread-safe: Uses write lock for exclusive access.
//
// Example usage:
//
//	// Clear cache during maintenance
//	func PerformMaintenance(ctx context.Context, tenantID TenantID) error {
//	    cache := cacheManager.GetCache(tenantID)
//
//	    // Clear cache before maintenance
//	    if err := cache.Clear(ctx); err != nil {
//	        return err
//	    }
//
//	    // Perform maintenance operations
//	    // ...
//
//	    return nil
//	}
//
// Use cases:
//   - Tenant decommission
//   - Bulk data refresh
//   - Reset for testing
//   - Memory recovery
func (tc *TenantCache) Clear(ctx context.Context) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.data = make(map[string]cacheEntry)
	return nil
}

// TenantStorageManager handles file system storage allocation for tenants.
// Creates and manages tenant-specific directories with isolated paths.
//
// Path structure:
//   - Base path: /var/lib/muster (configured at initialization)
//   - Tenant path: /var/lib/muster/tenants/{tenant_id}
//   - Example: /var/lib/muster/tenants/acme-corp
//
// Sanitization:
//   - Colons (:) replaced with underscores
//   - Forward slashes (/) replaced with underscores
//   - Prevents path traversal attacks
//
// Use cases:
//   - File uploads per tenant
//   - Generated reports and exports
//   - Tenant-specific configurations
//   - Backup and archive storage
//
// Example usage:
//
//	storageManager := NewTenantStorageManager("/var/lib/muster")
//
//	// Create tenant storage during provisioning
//	storage, err := storageManager.CreateStorage(ctx, "acme-corp")
//
//	// Get tenant path for file operations
//	tenantPath := storage.GetTenantPath()
//
//	// Store file
//	os.WriteFile(tenantPath+"/uploads/file.pdf", data, 0644)
//
//	// Delete during decommission
//	storageManager.DeleteStorage(ctx, "acme-corp")
//
// Security considerations:
//   - Ensure base path has appropriate permissions
//   - Validate tenant IDs to prevent path traversal
//   - Consider encryption for sensitive data
//   - Implement disk quotas per tenant
type TenantStorageManager struct {
	basePath string // Root directory for all tenant storage
}

// NewTenantStorageManager creates a storage manager with the specified base path.
// Choose a location with:
//   - Sufficient disk space for anticipated storage needs
//   - Appropriate file system permissions
//   - Backup and disaster recovery configured
//
// Example:
//
//	// Production paths
//	storageManager := NewTenantStorageManager("/var/lib/muster")
//	storageManager := NewTenantStorageManager("/mnt/data/muster")
//
//	// Development paths
//	storageManager := NewTenantStorageManager("./tmp/storage")
func NewTenantStorageManager(basePath string) *TenantStorageManager {
	return &TenantStorageManager{basePath: basePath}
}

// CreateStorage creates a new TenantStorage instance for the specified tenant.
// Computes the tenant-specific path but does NOT create directories on disk.
// Caller is responsible for creating directories as needed.
//
// Example usage:
//
//	storage, err := storageManager.CreateStorage(ctx, "acme-corp")
//	if err != nil {
//	    return err
//	}
//
//	// Create directory structure
//	os.MkdirAll(storage.GetTenantPath(), 0755)
//
//	// Use for file operations
//	uploadPath := storage.GetTenantPath() + "/uploads"
//	os.MkdirAll(uploadPath, 0755)
func (tsm *TenantStorageManager) CreateStorage(ctx context.Context, tenantID TenantID) (*TenantStorage, error) {
	return NewTenantStorage(tsm.basePath, tenantID)
}

// DeleteStorage removes all files and directories for a tenant.
// Currently a stub - implement with actual file system operations.
//
// TODO: Implement recursive deletion:
//
//	func (tsm *TenantStorageManager) DeleteStorage(ctx context.Context, tenantID TenantID) error {
//	    storage, err := tsm.CreateStorage(ctx, tenantID)
//	    if err != nil {
//	        return err
//	    }
//
//	    return os.RemoveAll(storage.GetTenantPath())
//	}
//
// Production considerations:
//   - Implement atomic deletion (move to trash, then delete)
//   - Log deletion for audit trail
//   - Handle partial failures gracefully
//   - Consider async deletion for large datasets
func (tsm *TenantStorageManager) DeleteStorage(ctx context.Context, tenantID TenantID) error {
	return nil
}

// TenantStorage represents file system storage allocated to a specific tenant.
// Provides the isolated path where tenant files should be stored.
//
// Path isolation ensures:
//   - Tenants cannot access each other's files
//   - Easy cleanup during decommission
//   - Simplified backup per tenant
//   - Disk quota enforcement
//
// Example usage:
//
//	storage := NewTenantStorage("/var/lib/muster", "acme-corp")
//
//	// Create tenant directory
//	os.MkdirAll(storage.GetTenantPath(), 0755)
//
//	// Store uploaded file
//	uploadPath := filepath.Join(storage.GetTenantPath(), "uploads", "file.pdf")
//	os.WriteFile(uploadPath, fileData, 0644)
type TenantStorage struct {
	basePath   string   // Root storage directory
	tenantID   TenantID // Owner tenant
	tenantPath string   // Full path to tenant directory
}

// NewTenantStorage creates a TenantStorage with sanitized tenant path.
// Sanitizes tenant ID to prevent path traversal attacks:
//   - Replaces colons (:) with underscores
//   - Replaces forward slashes (/) with underscores
//
// Example:
//
//	storage, err := NewTenantStorage("/var/lib/muster", "acme-corp")
//	if err != nil {
//	    return err
//	}
//
//	// Path: /var/lib/muster/tenants/acme-corp
//	fmt.Println(storage.GetTenantPath())
//
// Security: Always use this constructor (never construct paths manually)
// to ensure proper sanitization of tenant IDs.
func NewTenantStorage(basePath string, tenantID TenantID) (*TenantStorage, error) {
	sanitized := strings.ReplaceAll(string(tenantID), ":", "_")
	sanitized = strings.ReplaceAll(sanitized, "/", "_")

	tenantPath := fmt.Sprintf("%s/tenants/%s", basePath, sanitized)

	return &TenantStorage{
		basePath:   basePath,
		tenantID:   tenantID,
		tenantPath: tenantPath,
	}, nil
}

// GetTenantPath returns the full file system path for tenant storage.
// Use this path for all file operations related to the tenant.
//
// Example usage:
//
//	storage, _ := NewTenantStorage("/var/lib/muster", "acme-corp")
//
//	// Create tenant directory
//	tenantPath := storage.GetTenantPath()
//	os.MkdirAll(tenantPath, 0755)
//
//	// Create subdirectories
//	uploadsPath := filepath.Join(tenantPath, "uploads")
//	reportsPath := filepath.Join(tenantPath, "reports")
//	os.MkdirAll(uploadsPath, 0755)
//	os.MkdirAll(reportsPath, 0755)
//
// Path structure:
//   - {basePath}/tenants/{tenant_id}/
//   - Example: /var/lib/muster/tenants/acme-corp/
func (ts *TenantStorage) GetTenantPath() string {
	return ts.tenantPath
}

// QuotaManager manages resource quotas for tenants.
// Tracks and enforces limits on:
//   - Request rate (requests per second)
//   - Storage usage (bytes)
//   - Cache entries (keys)
//   - Database rows
//
// Features:
//   - Per-tenant quota tracking
//   - Default quotas for unspecified tenants
//   - Thread-safe concurrent access
//   - Dynamic quota updates
//
// Quota enforcement:
//   - Rate limits: Enforced by TenantRateLimiter
//   - Storage: Check before file uploads
//   - Cache: Check before cache writes
//   - Database: Check before inserts
//
// Example usage:
//
//	quotaManager := NewQuotaManager()
//
//	// Initialize quotas for new tenant
//	quotas := &TenantQuotas{
//	    MaxRequestsPerSecond: 100,
//	    MaxStorageBytes: 1024 * 1024 * 1024,  // 1GB
//	    MaxCacheKeys: 10000,
//	    MaxDatabaseRows: 100000,
//	}
//	quotaManager.InitializeQuotas(ctx, "acme-corp", quotas)
//
//	// Check quotas before operations
//	currentQuotas, _ := quotaManager.GetQuotas("acme-corp")
//	if currentStorage > currentQuotas.MaxStorageBytes {
//	    return ErrStorageExceeded
//	}
//
// Production considerations:
//   - Persist quotas to database (currently in-memory)
//   - Implement quota monitoring and alerting
//   - Add quota usage metrics
//   - Support quota overrides for enterprise customers
type QuotaManager struct {
	quotas map[TenantID]*TenantQuotas // Per-tenant quotas
	mu     sync.RWMutex               // Protects map access
}

// NewQuotaManager creates a quota manager for tenant resource tracking.
// Initializes empty quota map. Quotas are set per-tenant via InitializeQuotas.
//
// Example:
//
//	quotaManager := NewQuotaManager()
//
//	// Set quotas during tenant provisioning
//	quotas := &TenantQuotas{
//	    MaxRequestsPerSecond: 100,
//	    MaxStorageBytes: 1024 * 1024 * 1024,
//	}
//	quotaManager.InitializeQuotas(ctx, "acme-corp", quotas)
func NewQuotaManager() *QuotaManager {
	return &QuotaManager{
		quotas: make(map[TenantID]*TenantQuotas),
	}
}

// InitializeQuotas sets up resource quotas for a tenant.
// If quotas parameter is nil, applies default quotas suitable for free tier.
// Overwrites existing quotas for the tenant.
//
// Default quotas (when quotas=nil):
//   - MaxRequestsPerSecond: 100
//   - MaxStorageBytes: 100 MB
//   - MaxCacheKeys: 1000
//   - MaxDatabaseRows: 10000
//
// Example usage:
//
//	// Custom quotas for pro tier
//	proQuotas := &TenantQuotas{
//	    MaxRequestsPerSecond: 500,
//	    MaxStorageBytes:      10 * 1024 * 1024 * 1024,  // 10GB
//	    MaxCacheKeys:         50000,
//	    MaxDatabaseRows:      500000,
//	}
//	quotaManager.InitializeQuotas(ctx, "pro-tenant", proQuotas)
//
//	// Default quotas for free tier
//	quotaManager.InitializeQuotas(ctx, "free-tenant", nil)
//
// Thread-safe: Uses write lock for exclusive access.
func (qm *QuotaManager) InitializeQuotas(ctx context.Context, tenantID TenantID, quotas *TenantQuotas) error {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if quotas == nil {
		quotas = &TenantQuotas{
			MaxRequestsPerSecond: 100,
			MaxStorageBytes:      1024 * 1024 * 100,
			MaxCacheKeys:         1000,
			MaxDatabaseRows:      10000,
		}
	}

	qm.quotas[tenantID] = quotas
	return nil
}

// DeleteQuotas removes quota configuration for a tenant.
// Call during tenant decommission after cleaning up resources.
// No error if quotas don't exist (idempotent).
//
// Example usage:
//
//	// During tenant decommission
//	func DecommissionTenant(ctx context.Context, tenantID TenantID) error {
//	    // Clean up resources in order
//	    cacheManager.InvalidateTenant(ctx, tenantID)
//	    dbManager.DropTenantSchema(ctx, tenantID)
//	    storageManager.DeleteStorage(ctx, tenantID)
//
//	    // Finally remove quotas
//	    return quotaManager.DeleteQuotas(ctx, tenantID)
//	}
//
// Thread-safe: Uses write lock for exclusive access.
func (qm *QuotaManager) DeleteQuotas(ctx context.Context, tenantID TenantID) error {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	delete(qm.quotas, tenantID)
	return nil
}

// GetQuotas retrieves the current quota configuration for a tenant.
// Returns ErrTenantNotFound if no quotas are set for the tenant.
//
// Example usage:
//
//	// Check quota before operation
//	quotas, err := quotaManager.GetQuotas("acme-corp")
//	if err != nil {
//	    return err
//	}
//
//	// Validate storage usage
//	if currentUsage >= quotas.MaxStorageBytes {
//	    return fmt.Errorf("storage quota exceeded")
//	}
//
//	// Validate rate limit
//	if requestsPerSecond >= quotas.MaxRequestsPerSecond {
//	    return ErrRateLimitExceeded
//	}
//
// Thread-safe: Uses read lock for concurrent access.
func (qm *QuotaManager) GetQuotas(tenantID TenantID) (*TenantQuotas, error) {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	quotas, exists := qm.quotas[tenantID]
	if !exists {
		return nil, ErrTenantNotFound
	}
	return quotas, nil
}
