package tenant

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTenantID_Validate(t *testing.T) {
	tests := []struct {
		name      string
		id        TenantID
		wantError bool
	}{
		{"valid simple", "tenant-1", false},
		{"valid with underscore", "tenant_1", false},
		{"valid alphanumeric", "tenant123", false},
		{"empty", "", true},
		{"too long", TenantID("a"), true},
		{"invalid chars", "tenant@1", true},
		{"uppercase", "Tenant1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testID := tt.id
			if tt.name == "too long" {
				testID = TenantID("a")
				for i := 0; i < 65; i++ {
					testID += "a"
				}
			}
			err := testID.Validate()
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseTenantID(t *testing.T) {
	t.Run("valid ID", func(t *testing.T) {
		id, err := ParseTenantID("tenant-1")
		require.NoError(t, err)
		assert.Equal(t, TenantID("tenant-1"), id)
	})

	t.Run("empty ID", func(t *testing.T) {
		id, err := ParseTenantID("")
		require.Error(t, err)
		assert.Equal(t, TenantID(""), id)
	})

	t.Run("invalid characters", func(t *testing.T) {
		id, err := ParseTenantID("tenant@1")
		require.Error(t, err)
		assert.Equal(t, TenantID(""), id)
	})
}

func TestWithTenant(t *testing.T) {
	ctx := context.Background()
	tenant := TenantContext{
		ID:             "tenant-1",
		Name:           "Test Tenant",
		Tier:           TierPro,
		IsolationLevel: IsolationHard,
		Metadata:       map[string]string{"key": "value"},
	}

	newCtx := WithTenant(ctx, tenant)

	id, err := GetTenantID(newCtx)
	require.NoError(t, err)
	assert.Equal(t, TenantID("tenant-1"), id)

	retrieved, err := GetTenant(newCtx)
	require.NoError(t, err)
	assert.Equal(t, TenantID("tenant-1"), retrieved.ID)
	assert.Equal(t, "Test Tenant", retrieved.Name)
	assert.Equal(t, TierPro, retrieved.Tier)
	assert.Equal(t, IsolationHard, retrieved.IsolationLevel)

	level := GetIsolationLevel(newCtx)
	assert.Equal(t, IsolationHard, level)
}

func TestGetTenant_NoTenantInContext(t *testing.T) {
	ctx := context.Background()

	id, err := GetTenantID(ctx)
	require.Error(t, err)
	assert.Equal(t, ErrNoTenantInContext, err)
	assert.Equal(t, TenantID(""), id)

	tenant, err := GetTenant(ctx)
	require.Error(t, err)
	assert.Equal(t, ErrNoTenantInContext, err)
	assert.Nil(t, tenant)
}

func TestHeaderResolver(t *testing.T) {
	t.Run("header present", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Tenant-ID", "tenant-1")

		resolver := &HeaderResolver{HeaderName: "X-Tenant-ID"}
		id, err := resolver.Resolve(req)
		require.NoError(t, err)
		assert.Equal(t, TenantID("tenant-1"), id)
	})

	t.Run("header missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)

		resolver := &HeaderResolver{HeaderName: "X-Tenant-ID"}
		id, err := resolver.Resolve(req)
		require.Error(t, err)
		assert.Equal(t, ErrTenantHeaderMissing, err)
		assert.Equal(t, TenantID(""), id)
	})

	t.Run("header invalid", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Tenant-ID", "tenant@invalid")

		resolver := &HeaderResolver{HeaderName: "X-Tenant-ID"}
		id, err := resolver.Resolve(req)
		require.Error(t, err)
		assert.Equal(t, TenantID(""), id)
	})
}

func TestSubdomainResolver(t *testing.T) {
	t.Run("valid subdomain", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Host = "tenant1.example.com"

		resolver := &SubdomainResolver{BaseDomain: "example.com"}
		id, err := resolver.Resolve(req)
		require.NoError(t, err)
		assert.Equal(t, TenantID("tenant1"), id)
	})

	t.Run("subdomain missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Host = "example.com"

		resolver := &SubdomainResolver{BaseDomain: "example.com"}
		id, err := resolver.Resolve(req)
		require.Error(t, err)
		assert.Equal(t, ErrTenantSubdomainMissing, err)
		assert.Equal(t, TenantID(""), id)
	})

	t.Run("invalid subdomain", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Host = "tenant@invalid.example.com"

		resolver := &SubdomainResolver{BaseDomain: "example.com"}
		id, err := resolver.Resolve(req)
		require.Error(t, err)
		assert.Equal(t, TenantID(""), id)
	})
}

func TestJWTResolver(t *testing.T) {
	t.Run("claims present", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		ctx := context.WithValue(req.Context(), jwtContextKey{}, map[string]interface{}{
			"tenant_id": "tenant-1",
		})
		req = req.WithContext(ctx)

		resolver := NewJWTResolver()
		id, err := resolver.Resolve(req)
		require.NoError(t, err)
		assert.Equal(t, TenantID("tenant-1"), id)
	})

	t.Run("claims missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)

		resolver := NewJWTResolver()
		id, err := resolver.Resolve(req)
		require.Error(t, err)
		assert.Equal(t, ErrTenantClaimsMissing, err)
		assert.Equal(t, TenantID(""), id)
	})

	t.Run("tenant_id in claims missing", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		ctx := context.WithValue(req.Context(), jwtContextKey{}, map[string]interface{}{
			"user_id": "user-1",
		})
		req = req.WithContext(ctx)

		resolver := NewJWTResolver()
		id, err := resolver.Resolve(req)
		require.Error(t, err)
		assert.Equal(t, ErrTenantIDInClaimsMissing, err)
		assert.Equal(t, TenantID(""), id)
	})
}

func TestChainResolver(t *testing.T) {
	t.Run("first resolver succeeds", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Tenant-ID", "tenant-1")

		resolver := NewChainResolver(
			&HeaderResolver{HeaderName: "X-Tenant-ID"},
			&SubdomainResolver{BaseDomain: "example.com"},
		)
		id, err := resolver.Resolve(req)
		require.NoError(t, err)
		assert.Equal(t, TenantID("tenant-1"), id)
	})

	t.Run("second resolver succeeds", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Host = "tenant1.example.com"

		resolver := NewChainResolver(
			&HeaderResolver{HeaderName: "X-Tenant-ID"},
			&SubdomainResolver{BaseDomain: "example.com"},
		)
		id, err := resolver.Resolve(req)
		require.NoError(t, err)
		assert.Equal(t, TenantID("tenant1"), id)
	})

	t.Run("all resolvers fail", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)

		resolver := NewChainResolver(
			&HeaderResolver{HeaderName: "X-Tenant-ID"},
			&SubdomainResolver{BaseDomain: "example.com"},
		)
		id, err := resolver.Resolve(req)
		require.Error(t, err)
		assert.Equal(t, TenantID(""), id)
	})
}

func TestMemoryTenantStore(t *testing.T) {
	store := NewMemoryTenantStore()
	ctx := context.Background()

	t.Run("create tenant", func(t *testing.T) {
		tenant := &Tenant{
			ID:       "tenant-1",
			Name:     "Test Tenant",
			Tier:     TierPro,
			Active:   true,
			Metadata: map[string]string{"key": "value"},
		}
		err := store.Create(ctx, tenant)
		require.NoError(t, err)
	})

	t.Run("get tenant", func(t *testing.T) {
		tenant, err := store.Get(ctx, "tenant-1")
		require.NoError(t, err)
		require.NotNil(t, tenant)
		assert.Equal(t, TenantID("tenant-1"), tenant.ID)
		assert.Equal(t, "Test Tenant", tenant.Name)
	})

	t.Run("get non-existent tenant", func(t *testing.T) {
		tenant, err := store.Get(ctx, "non-existent")
		require.Error(t, err)
		assert.Equal(t, ErrTenantNotFound, err)
		assert.Nil(t, tenant)
	})

	t.Run("update tenant", func(t *testing.T) {
		tenant := &Tenant{
			ID:       "tenant-1",
			Name:     "Updated Tenant",
			Tier:     TierEnterprise,
			Active:   true,
			Metadata: map[string]string{"key": "new-value"},
		}
		err := store.Update(ctx, tenant)
		require.NoError(t, err)

		retrieved, err := store.Get(ctx, "tenant-1")
		require.NoError(t, err)
		assert.Equal(t, "Updated Tenant", retrieved.Name)
		assert.Equal(t, TierEnterprise, retrieved.Tier)
	})

	t.Run("update non-existent tenant", func(t *testing.T) {
		tenant := &Tenant{
			ID:     "non-existent",
			Name:   "Test",
			Active: true,
		}
		err := store.Update(ctx, tenant)
		require.Error(t, err)
		assert.Equal(t, ErrTenantNotFound, err)
	})

	t.Run("delete tenant", func(t *testing.T) {
		err := store.Delete(ctx, "tenant-1")
		require.NoError(t, err)

		_, err = store.Get(ctx, "tenant-1")
		require.Error(t, err)
	})

	t.Run("delete non-existent tenant", func(t *testing.T) {
		err := store.Delete(ctx, "non-existent")
		require.Error(t, err)
		assert.Equal(t, ErrTenantNotFound, err)
	})

	t.Run("create duplicate tenant", func(t *testing.T) {
		tenant := &Tenant{
			ID:     "tenant-1",
			Name:   "Test",
			Active: true,
		}
		err := store.Create(ctx, tenant)
		require.NoError(t, err)

		duplicate := &Tenant{
			ID:     "tenant-1",
			Name:   "Duplicate",
			Active: true,
		}
		err = store.Create(ctx, duplicate)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})
}

func TestTenantMiddleware(t *testing.T) {
	store := NewMemoryTenantStore()
	validator := NewSimpleTenantValidator(store)
	resolver := &HeaderResolver{HeaderName: "X-Tenant-ID"}
	middleware := NewTenantMiddleware(resolver, validator, store)

	ctx := context.Background()
	_ = store.Create(ctx, &Tenant{
		ID:       "tenant-1",
		Name:     "Test Tenant",
		Tier:     TierPro,
		Active:   true,
		Metadata: map[string]string{},
	})

	t.Run("valid tenant", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Tenant-ID", "tenant-1")
		rr := httptest.NewRecorder()

		middleware.Middleware(handler).ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "tenant-1", rr.Header().Get("X-Tenant-ID"))
	})

	t.Run("missing tenant", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()

		middleware.Middleware(handler).ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("suspended tenant", func(t *testing.T) {
		_ = store.Update(ctx, &Tenant{
			ID:     "tenant-1",
			Name:   "Test Tenant",
			Tier:   TierPro,
			Active: false,
		})

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Tenant-ID", "tenant-1")
		rr := httptest.NewRecorder()

		middleware.Middleware(handler).ServeHTTP(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)

		_ = store.Update(ctx, &Tenant{
			ID:     "tenant-1",
			Name:   "Test Tenant",
			Tier:   TierPro,
			Active: true,
		})
	})
}

func TestRequireTenant(t *testing.T) {
	t.Run("tenant in context", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/", nil)
		ctx := WithTenant(req.Context(), TenantContext{
			ID:   "tenant-1",
			Name: "Test",
		})
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		RequireTenant(handler).ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("tenant not in context", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()

		RequireTenant(handler).ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func TestTenantRateLimiter(t *testing.T) {
	limiter := NewTenantRateLimiter(10.0, 5)
	ctx := context.Background()

	t.Run("allow within limit", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			err := limiter.CheckLimit(ctx, "tenant-1")
			assert.NoError(t, err)
		}
	})

	t.Run("exceed limit", func(t *testing.T) {
		err := limiter.CheckLimit(ctx, "tenant-1")
		assert.Error(t, err)
		assert.Equal(t, ErrRateLimitExceeded, err)
	})

	t.Run("different tenant independent", func(t *testing.T) {
		err := limiter.CheckLimit(ctx, "tenant-2")
		assert.NoError(t, err)
	})

	t.Run("set custom rate", func(t *testing.T) {
		limiter.SetRate("tenant-3", 1.0, 2)
		err := limiter.CheckLimit(ctx, "tenant-3")
		assert.NoError(t, err)
		err = limiter.CheckLimit(ctx, "tenant-3")
		assert.NoError(t, err)
		err = limiter.CheckLimit(ctx, "tenant-3")
		assert.Error(t, err)
	})
}

func TestTokenBucket(t *testing.T) {
	t.Run("allows requests within burst", func(t *testing.T) {
		bucket := NewTokenBucket(5, 1.0)
		for i := 0; i < 5; i++ {
			assert.True(t, bucket.Allow())
		}
	})

	t.Run("denies when empty", func(t *testing.T) {
		bucket := NewTokenBucket(1, 0.0)
		assert.True(t, bucket.Allow())
		assert.False(t, bucket.Allow())
	})

	t.Run("refills over time", func(t *testing.T) {
		bucket := NewTokenBucket(1, 10.0)
		assert.True(t, bucket.Allow())
		assert.False(t, bucket.Allow())
		time.Sleep(150 * time.Millisecond)
		assert.True(t, bucket.Allow())
	})
}

func TestTenantAccessControl(t *testing.T) {
	ac := NewTenantAccessControl()
	ctx := context.Background()

	ac.GrantPermission("tenant-1", "user-1", "resource-1", "read", "allow")
	ac.GrantPermission("tenant-1", "user-1", "resource-1", "write", "allow")
	ac.GrantPermission("tenant-1", "user-2", "*", "*", "deny")

	t.Run("allowed permission", func(t *testing.T) {
		err := ac.CheckPermission(ctx, "tenant-1", "user-1", "resource-1", "read")
		assert.NoError(t, err)
	})

	t.Run("denied permission", func(t *testing.T) {
		err := ac.CheckPermission(ctx, "tenant-1", "user-1", "resource-2", "read")
		assert.Error(t, err)
		assert.Equal(t, ErrPermissionDenied, err)
	})

	t.Run("wildcard permission", func(t *testing.T) {
		err := ac.CheckPermission(ctx, "tenant-1", "user-1", "any-resource", "any-action")
		assert.Error(t, err)
	})

	t.Run("explicit deny", func(t *testing.T) {
		err := ac.CheckPermission(ctx, "tenant-1", "user-2", "anything", "anything")
		assert.Error(t, err)
		assert.Equal(t, ErrPermissionDenied, err)
	})

	t.Run("unknown tenant", func(t *testing.T) {
		err := ac.CheckPermission(ctx, "unknown-tenant", "user-1", "resource-1", "read")
		assert.Error(t, err)
		assert.Equal(t, ErrTenantNotFound, err)
	})
}

func TestTenantSecurityEnforcer(t *testing.T) {
	t.Run("soft isolation allows cross-tenant", func(t *testing.T) {
		enforcer := NewTenantSecurityEnforcer(IsolationSoft)
		ctx := context.Background()
		err := enforcer.ValidateCrossTenantAccess(ctx, "tenant-1", "tenant-2")
		assert.NoError(t, err)
	})

	t.Run("hard isolation denies cross-tenant", func(t *testing.T) {
		enforcer := NewTenantSecurityEnforcer(IsolationHard)
		ctx := context.Background()
		err := enforcer.ValidateCrossTenantAccess(ctx, "tenant-1", "tenant-2")
		assert.Error(t, err)
		assert.Equal(t, ErrCrossTenantAccessDenied, err)
	})

	t.Run("hard isolation allows same-tenant", func(t *testing.T) {
		enforcer := NewTenantSecurityEnforcer(IsolationHard)
		ctx := context.Background()
		err := enforcer.ValidateCrossTenantAccess(ctx, "tenant-1", "tenant-1")
		assert.NoError(t, err)
	})

	t.Run("dedicated isolation denies cross-tenant", func(t *testing.T) {
		enforcer := NewTenantSecurityEnforcer(IsolationDedicated)
		ctx := context.Background()
		err := enforcer.ValidateCrossTenantAccess(ctx, "tenant-1", "tenant-2")
		assert.Error(t, err)
		assert.Equal(t, ErrCrossTenantAccessDenied, err)
	})
}

func TestTenantCache(t *testing.T) {
	ctx := context.Background()
	cache := NewTenantCache("tenant-1")

	t.Run("set and get", func(t *testing.T) {
		err := cache.Set(ctx, "key1", "value1", time.Hour)
		require.NoError(t, err)

		value, err := cache.Get(ctx, "key1")
		require.NoError(t, err)
		assert.Equal(t, "value1", value)
	})

	t.Run("cache miss", func(t *testing.T) {
		_, err := cache.Get(ctx, "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cache miss")
	})

	t.Run("delete", func(t *testing.T) {
		_ = cache.Set(ctx, "key2", "value2", time.Hour)
		err := cache.Delete(ctx, "key2")
		require.NoError(t, err)

		_, err = cache.Get(ctx, "key2")
		assert.Error(t, err)
	})

	t.Run("clear", func(t *testing.T) {
		_ = cache.Set(ctx, "key3", "value3", time.Hour)
		_ = cache.Set(ctx, "key4", "value4", time.Hour)
		err := cache.Clear(ctx)
		require.NoError(t, err)

		_, err = cache.Get(ctx, "key3")
		assert.Error(t, err)
		_, err = cache.Get(ctx, "key4")
		assert.Error(t, err)
	})

	t.Run("namespaced key", func(t *testing.T) {
		key := cache.namespacedKey("test-key")
		assert.Equal(t, "tenant:tenant-1:test-key", key)
	})
}

func TestTenantCacheManager(t *testing.T) {
	manager := NewTenantCacheManager()
	ctx := context.Background()

	t.Run("get cache creates new", func(t *testing.T) {
		cache := manager.GetCache("tenant-1")
		require.NotNil(t, cache)
		assert.Equal(t, TenantID("tenant-1"), cache.tenantID)
	})

	t.Run("get cache returns cached", func(t *testing.T) {
		cache1 := manager.GetCache("tenant-1")
		cache2 := manager.GetCache("tenant-1")
		assert.Equal(t, cache1, cache2)
	})

	t.Run("invalidate tenant", func(t *testing.T) {
		cache := manager.GetCache("tenant-2")
		_ = cache.Set(ctx, "key", "value", time.Hour)
		err := manager.InvalidateTenant(ctx, "tenant-2")
		require.NoError(t, err)

		_, err = cache.Get(ctx, "key")
		assert.Error(t, err)
	})
}

func TestQuotaManager(t *testing.T) {
	manager := NewQuotaManager()
	ctx := context.Background()

	t.Run("initialize with custom quotas", func(t *testing.T) {
		quotas := &TenantQuotas{
			MaxRequestsPerSecond: 500,
			MaxStorageBytes:      1024 * 1024 * 1000,
			MaxCacheKeys:         10000,
			MaxDatabaseRows:      100000,
		}
		err := manager.InitializeQuotas(ctx, "tenant-1", quotas)
		require.NoError(t, err)

		retrieved, err := manager.GetQuotas("tenant-1")
		require.NoError(t, err)
		assert.Equal(t, int64(1024*1024*1000), retrieved.MaxStorageBytes)
	})

	t.Run("initialize with default quotas", func(t *testing.T) {
		err := manager.InitializeQuotas(ctx, "tenant-2", nil)
		require.NoError(t, err)

		retrieved, err := manager.GetQuotas("tenant-2")
		require.NoError(t, err)
		assert.Equal(t, int64(1024*1024*100), retrieved.MaxStorageBytes)
		assert.Equal(t, 100, retrieved.MaxRequestsPerSecond)
	})

	t.Run("delete quotas", func(t *testing.T) {
		err := manager.DeleteQuotas(ctx, "tenant-1")
		require.NoError(t, err)

		_, err = manager.GetQuotas("tenant-1")
		assert.Error(t, err)
	})

	t.Run("get non-existent quotas", func(t *testing.T) {
		_, err := manager.GetQuotas("non-existent")
		assert.Error(t, err)
		assert.Equal(t, ErrTenantNotFound, err)
	})
}

func TestTenantStorage(t *testing.T) {
	t.Run("create storage", func(t *testing.T) {
		storage, err := NewTenantStorage("/tmp/test", "tenant-1")
		require.NoError(t, err)
		require.NotNil(t, storage)
		assert.Contains(t, storage.tenantPath, "tenants/tenant-1")
	})

	t.Run("sanitize special chars", func(t *testing.T) {
		storage, err := NewTenantStorage("/tmp/test", "tenant:1/2")
		require.NoError(t, err)
		assert.Contains(t, storage.tenantPath, "tenants/tenant_1_2")
	})

	t.Run("get tenant path", func(t *testing.T) {
		storage, err := NewTenantStorage("/tmp/test", "tenant-1")
		require.NoError(t, err)
		path := storage.GetTenantPath()
		assert.Contains(t, path, "tenants/tenant-1")
	})
}

func TestTenantDBManager(t *testing.T) {
	db := NewTenantDBManager()
	ctx := context.Background()

	t.Run("get tenant schema", func(t *testing.T) {
		schema := db.GetTenantSchema("tenant-1")
		assert.Equal(t, "tenant_tenant_1", schema)
	})

	t.Run("get tenant schema with dashes", func(t *testing.T) {
		schema := db.GetTenantSchema("tenant-with-dashes")
		assert.Equal(t, "tenant_tenant_with_dashes", schema)
	})

	t.Run("create tenant schema", func(t *testing.T) {
		err := db.CreateTenantSchema(ctx, "tenant-1")
		assert.NoError(t, err)
	})

	t.Run("drop tenant schema", func(t *testing.T) {
		err := db.DropTenantSchema(ctx, "tenant-1")
		assert.NoError(t, err)
	})

	t.Run("update tenant status", func(t *testing.T) {
		err := db.UpdateTenantStatus(ctx, "tenant-1", false)
		assert.NoError(t, err)
		err = db.UpdateTenantStatus(ctx, "tenant-1", true)
		assert.NoError(t, err)
	})
}

func TestTenantProvisioner(t *testing.T) {
	db := NewTenantDBManager()
	cache := NewTenantCacheManager()
	storage := NewTenantStorageManager("/tmp/test")
	quota := NewQuotaManager()
	provisioner := NewTenantProvisioner(db, cache, storage, quota)
	ctx := context.Background()

	t.Run("provision tenant", func(t *testing.T) {
		tenant := Tenant{
			ID:       "tenant-new",
			Name:     "New Tenant",
			Tier:     TierPro,
			Active:   true,
			Metadata: map[string]string{},
			Quotas: &TenantQuotas{
				MaxRequestsPerSecond: 100,
				MaxStorageBytes:      1024 * 1024 * 100,
			},
		}
		err := provisioner.Provision(ctx, tenant)
		assert.NoError(t, err)
	})

	t.Run("decommission tenant", func(t *testing.T) {
		err := provisioner.Decommission(ctx, "tenant-new")
		assert.NoError(t, err)
	})

	t.Run("suspend tenant", func(t *testing.T) {
		err := provisioner.Suspend(ctx, "tenant-1")
		assert.NoError(t, err)
	})

	t.Run("resume tenant", func(t *testing.T) {
		err := provisioner.Resume(ctx, "tenant-1")
		assert.NoError(t, err)
	})
}
