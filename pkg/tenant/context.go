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
	ErrNoTenantInContext       = errors.New("no tenant in context")
	ErrTenantHeaderMissing     = errors.New("tenant header missing")
	ErrTenantSubdomainMissing  = errors.New("tenant subdomain missing")
	ErrTenantClaimsMissing     = errors.New("tenant claims missing")
	ErrTenantIDInClaimsMissing = errors.New("tenant_id in claims missing")
	ErrNoTenantContext         = errors.New("no tenant context")
	ErrTenantNotFound          = errors.New("tenant not found")
	ErrTenantSuspended         = errors.New("tenant suspended")
	ErrCrossTenantAccessDenied = errors.New("cross-tenant access denied")
	ErrRateLimitExceeded       = errors.New("rate limit exceeded")
	ErrPermissionDenied        = errors.New("permission denied")
)

type TenantID string
type TenantTier string

const (
	TierFree       TenantTier = "free"
	TierPro        TenantTier = "pro"
	TierEnterprise TenantTier = "enterprise"
	TierDedicated  TenantTier = "dedicated"
)

type IsolationLevel int

const (
	IsolationSoft IsolationLevel = iota
	IsolationHard
	IsolationDedicated
)

type TenantContext struct {
	ID             TenantID
	Name           string
	Tier           TenantTier
	IsolationLevel IsolationLevel
	Metadata       map[string]string
}

type Tenant struct {
	ID             TenantID
	Name           string
	Tier           TenantTier
	IsolationLevel IsolationLevel
	Active         bool
	Metadata       map[string]string
	Quotas         *TenantQuotas
}

type TenantQuotas struct {
	MaxRequestsPerSecond int
	MaxStorageBytes      int64
	MaxCacheKeys         int
	MaxDatabaseRows      int
}

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

func WithTenant(ctx context.Context, tenant TenantContext) context.Context {
	ctx = context.WithValue(ctx, tenantIDKey, tenant.ID)
	ctx = context.WithValue(ctx, tenantNameKey, tenant.Name)
	ctx = context.WithValue(ctx, tenantTierKey, tenant.Tier)
	ctx = context.WithValue(ctx, isolationLevelKey, tenant.IsolationLevel)
	return ctx
}

func GetTenantID(ctx context.Context) (TenantID, error) {
	id, ok := ctx.Value(tenantIDKey).(TenantID)
	if !ok {
		return "", ErrNoTenantInContext
	}
	return id, nil
}

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

func GetIsolationLevel(ctx context.Context) IsolationLevel {
	level, ok := ctx.Value(isolationLevelKey).(IsolationLevel)
	if !ok {
		return IsolationSoft
	}
	return level
}

type TenantResolver interface {
	Resolve(r *http.Request) (TenantID, error)
}

type HeaderResolver struct {
	HeaderName string
}

func (hr *HeaderResolver) Resolve(r *http.Request) (TenantID, error) {
	tenantID := r.Header.Get(hr.HeaderName)
	if tenantID == "" {
		return "", ErrTenantHeaderMissing
	}
	return ParseTenantID(tenantID)
}

type SubdomainResolver struct {
	BaseDomain string
}

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

type jwtContextKey struct{}

type JWTResolver struct{}

func NewJWTResolver() *JWTResolver {
	return &JWTResolver{}
}

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

type ChainResolver struct {
	resolvers []TenantResolver
}

func NewChainResolver(resolvers ...TenantResolver) *ChainResolver {
	return &ChainResolver{resolvers: resolvers}
}

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

type TenantValidator interface {
	Validate(ctx context.Context, tenant *Tenant, r *http.Request) error
}

type SimpleTenantValidator struct {
	store TenantStore
}

func NewSimpleTenantValidator(store TenantStore) *SimpleTenantValidator {
	return &SimpleTenantValidator{store: store}
}

func (stv *SimpleTenantValidator) Validate(ctx context.Context, tenant *Tenant, r *http.Request) error {
	if !tenant.Active {
		return ErrTenantSuspended
	}
	return nil
}

type TenantStore interface {
	Get(ctx context.Context, id TenantID) (*Tenant, error)
	Create(ctx context.Context, tenant *Tenant) error
	Update(ctx context.Context, tenant *Tenant) error
	Delete(ctx context.Context, id TenantID) error
}

type MemoryTenantStore struct {
	tenants map[TenantID]*Tenant
	mu      sync.RWMutex
}

func NewMemoryTenantStore() *MemoryTenantStore {
	return &MemoryTenantStore{
		tenants: make(map[TenantID]*Tenant),
	}
}

func (mts *MemoryTenantStore) Get(ctx context.Context, id TenantID) (*Tenant, error) {
	mts.mu.RLock()
	defer mts.mu.RUnlock()

	tenant, exists := mts.tenants[id]
	if !exists {
		return nil, ErrTenantNotFound
	}
	return tenant, nil
}

func (mts *MemoryTenantStore) Create(ctx context.Context, tenant *Tenant) error {
	mts.mu.Lock()
	defer mts.mu.Unlock()

	if _, exists := mts.tenants[tenant.ID]; exists {
		return fmt.Errorf("tenant already exists")
	}

	mts.tenants[tenant.ID] = tenant
	return nil
}

func (mts *MemoryTenantStore) Update(ctx context.Context, tenant *Tenant) error {
	mts.mu.Lock()
	defer mts.mu.Unlock()

	if _, exists := mts.tenants[tenant.ID]; !exists {
		return ErrTenantNotFound
	}

	mts.tenants[tenant.ID] = tenant
	return nil
}

func (mts *MemoryTenantStore) Delete(ctx context.Context, id TenantID) error {
	mts.mu.Lock()
	defer mts.mu.Unlock()

	if _, exists := mts.tenants[id]; !exists {
		return ErrTenantNotFound
	}

	delete(mts.tenants, id)
	return nil
}

type TenantMiddleware struct {
	resolver  TenantResolver
	validator TenantValidator
	store     TenantStore
}

func NewTenantMiddleware(resolver TenantResolver, validator TenantValidator, store TenantStore) *TenantMiddleware {
	return &TenantMiddleware{
		resolver:  resolver,
		validator: validator,
		store:     store,
	}
}

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

type TokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
	mu         sync.Mutex
}

func NewTokenBucket(maxTokens, refillRate float64) *TokenBucket {
	return &TokenBucket{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

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

type TenantRateLimiter struct {
	buckets      map[TenantID]*TokenBucket
	mu           sync.RWMutex
	defaultRate  float64
	defaultBurst int
}

func NewTenantRateLimiter(defaultRate float64, defaultBurst int) *TenantRateLimiter {
	return &TenantRateLimiter{
		buckets:      make(map[TenantID]*TokenBucket),
		defaultRate:  defaultRate,
		defaultBurst: defaultBurst,
	}
}

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

func (trl *TenantRateLimiter) SetRate(tenantID TenantID, rate float64, burst int) {
	trl.mu.Lock()
	defer trl.mu.Unlock()

	trl.buckets[tenantID] = NewTokenBucket(float64(burst), rate)
}

type Permission struct {
	Resource string
	Action   string
	Effect   string
}

type TenantAccessControl struct {
	permissions map[TenantID]map[string][]Permission
	mu          sync.RWMutex
}

func NewTenantAccessControl() *TenantAccessControl {
	return &TenantAccessControl{
		permissions: make(map[TenantID]map[string][]Permission),
	}
}

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

type TenantSecurityEnforcer struct {
	isolationLevel IsolationLevel
}

func NewTenantSecurityEnforcer(level IsolationLevel) *TenantSecurityEnforcer {
	return &TenantSecurityEnforcer{isolationLevel: level}
}

func (tse *TenantSecurityEnforcer) ValidateCrossTenantAccess(ctx context.Context, sourceTenant, targetTenant TenantID) error {
	if tse.isolationLevel >= IsolationHard && sourceTenant != targetTenant {
		return ErrCrossTenantAccessDenied
	}
	return nil
}

type TenantProvisioner struct {
	dbManager    *TenantDBManager
	cacheManager *TenantCacheManager
	storage      *TenantStorageManager
	quotaManager *QuotaManager
}

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

func (tp *TenantProvisioner) Suspend(ctx context.Context, tenantID TenantID) error {
	return tp.dbManager.UpdateTenantStatus(ctx, tenantID, false)
}

func (tp *TenantProvisioner) Resume(ctx context.Context, tenantID TenantID) error {
	return tp.dbManager.UpdateTenantStatus(ctx, tenantID, true)
}

type TenantDBManager struct {
	defaultSchema string
}

func NewTenantDBManager() *TenantDBManager {
	return &TenantDBManager{
		defaultSchema: "public",
	}
}

func (tdb *TenantDBManager) GetTenantSchema(tenantID TenantID) string {
	sanitized := strings.ReplaceAll(string(tenantID), "-", "_")
	return fmt.Sprintf("tenant_%s", sanitized)
}

func (tdb *TenantDBManager) SetTenantSchema(ctx context.Context, tenantID TenantID) error {
	schema := tdb.GetTenantSchema(tenantID)
	query := fmt.Sprintf("SET search_path TO %s", schema)
	return tdb.execute(ctx, query)
}

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

func (tdb *TenantDBManager) DropTenantSchema(ctx context.Context, tenantID TenantID) error {
	schema := tdb.GetTenantSchema(tenantID)
	query := fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema)
	return tdb.execute(ctx, query)
}

func (tdb *TenantDBManager) UpdateTenantStatus(ctx context.Context, tenantID TenantID, active bool) error {
	return nil
}

func (tdb *TenantDBManager) execute(ctx context.Context, query string) error {
	return nil
}

type TenantCacheManager struct {
	caches map[TenantID]*TenantCache
	mu     sync.RWMutex
}

func NewTenantCacheManager() *TenantCacheManager {
	return &TenantCacheManager{
		caches: make(map[TenantID]*TenantCache),
	}
}

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

func (tcm *TenantCacheManager) InvalidateTenant(ctx context.Context, tenantID TenantID) error {
	cache := tcm.GetCache(tenantID)
	return cache.Clear(ctx)
}

type TenantCache struct {
	tenantID TenantID
	prefix   string
	data     map[string]cacheEntry
	mu       sync.RWMutex
}

type cacheEntry struct {
	value     interface{}
	expiresAt time.Time
}

func NewTenantCache(tenantID TenantID) *TenantCache {
	return &TenantCache{
		tenantID: tenantID,
		prefix:   fmt.Sprintf("tenant:%s:", tenantID),
		data:     make(map[string]cacheEntry),
	}
}

func (tc *TenantCache) namespacedKey(key string) string {
	return tc.prefix + key
}

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

func (tc *TenantCache) Delete(ctx context.Context, key string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	namespacedKey := tc.namespacedKey(key)
	delete(tc.data, namespacedKey)
	return nil
}

func (tc *TenantCache) Clear(ctx context.Context) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.data = make(map[string]cacheEntry)
	return nil
}

type TenantStorageManager struct {
	basePath string
}

func NewTenantStorageManager(basePath string) *TenantStorageManager {
	return &TenantStorageManager{basePath: basePath}
}

func (tsm *TenantStorageManager) CreateStorage(ctx context.Context, tenantID TenantID) (*TenantStorage, error) {
	return NewTenantStorage(tsm.basePath, tenantID)
}

func (tsm *TenantStorageManager) DeleteStorage(ctx context.Context, tenantID TenantID) error {
	return nil
}

type TenantStorage struct {
	basePath   string
	tenantID   TenantID
	tenantPath string
}

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

func (ts *TenantStorage) GetTenantPath() string {
	return ts.tenantPath
}

type QuotaManager struct {
	quotas map[TenantID]*TenantQuotas
	mu     sync.RWMutex
}

func NewQuotaManager() *QuotaManager {
	return &QuotaManager{
		quotas: make(map[TenantID]*TenantQuotas),
	}
}

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

func (qm *QuotaManager) DeleteQuotas(ctx context.Context, tenantID TenantID) error {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	delete(qm.quotas, tenantID)
	return nil
}

func (qm *QuotaManager) GetQuotas(tenantID TenantID) (*TenantQuotas, error) {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	quotas, exists := qm.quotas[tenantID]
	if !exists {
		return nil, ErrTenantNotFound
	}
	return quotas, nil
}
