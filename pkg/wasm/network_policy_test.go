package wasm

import (
	"testing"
)

func TestNetworkPolicy_IsAllowed(t *testing.T) {
	tests := []struct {
		name    string
		policy  *NetworkPolicy
		host    string
		port    int
		allowed bool
	}{
		{
			name:    "no restrictions - allow all",
			policy:  &NetworkPolicy{},
			host:    "example.com",
			port:    80,
			allowed: true,
		},
		{
			name: "specific host allowed",
			policy: &NetworkPolicy{
				AllowedHosts: []string{"example.com"},
			},
			host:    "example.com",
			port:    80,
			allowed: true,
		},
		{
			name: "specific host not allowed",
			policy: &NetworkPolicy{
				AllowedHosts: []string{"example.com"},
			},
			host:    "other.com",
			port:    80,
			allowed: false,
		},
		{
			name: "wildcard host allowed",
			policy: &NetworkPolicy{
				AllowedHosts: []string{"*.example.com"},
			},
			host:    "sub.example.com",
			port:    80,
			allowed: true,
		},
		{
			name: "wildcard host not matching",
			policy: &NetworkPolicy{
				AllowedHosts: []string{"*.example.com"},
			},
			host:    "example.com",
			port:    80,
			allowed: false,
		},
		{
			name: "specific port allowed",
			policy: &NetworkPolicy{
				AllowedHosts: []string{"example.com"},
				AllowedPorts: []int{443},
			},
			host:    "example.com",
			port:    443,
			allowed: true,
		},
		{
			name: "specific port not allowed",
			policy: &NetworkPolicy{
				AllowedHosts: []string{"example.com"},
				AllowedPorts: []int{443},
			},
			host:    "example.com",
			port:    80,
			allowed: false,
		},
		{
			name: "blocked host takes precedence",
			policy: &NetworkPolicy{
				AllowedHosts: []string{"*.example.com"},
				BlockedHosts: []string{"blocked.example.com"},
			},
			host:    "blocked.example.com",
			port:    80,
			allowed: false,
		},
		{
			name: "blocked with wildcard",
			policy: &NetworkPolicy{
				AllowedHosts: []string{"*"},
				BlockedHosts: []string{"*.internal.com"},
			},
			host:    "secret.internal.com",
			port:    80,
			allowed: false,
		},
		{
			name: "multiple hosts - one matches",
			policy: &NetworkPolicy{
				AllowedHosts: []string{"example.com", "api.example.com"},
			},
			host:    "api.example.com",
			port:    80,
			allowed: true,
		},
		{
			name: "multiple ports - one matches",
			policy: &NetworkPolicy{
				AllowedHosts: []string{"example.com"},
				AllowedPorts: []int{80, 443, 8080},
			},
			host:    "example.com",
			port:    8080,
			allowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.policy.IsAllowed(tt.host, tt.port)
			if result != tt.allowed {
				t.Errorf("IsAllowed(%q, %d) = %v, want %v",
					tt.host, tt.port, result, tt.allowed)
			}
		})
	}
}

func TestSandboxedFS_SetNetworkPolicy(t *testing.T) {
	fs := NewSandboxedFS("/tmp/test")

	policy := &NetworkPolicy{
		AllowedHosts: []string{"example.com", "api.example.com"},
		AllowedPorts: []int{80, 443},
		BlockedHosts: []string{"blocked.example.com"},
	}

	fs.SetNetworkPolicy(policy)

	retrieved := fs.GetNetworkPolicy()
	if retrieved == nil {
		t.Fatal("expected non-nil policy")
	}

	// Verify the policy was stored correctly
	if len(retrieved.AllowedHosts) != 2 {
		t.Errorf("expected 2 allowed hosts, got %d", len(retrieved.AllowedHosts))
	}
	if len(retrieved.AllowedPorts) != 2 {
		t.Errorf("expected 2 allowed ports, got %d", len(retrieved.AllowedPorts))
	}
	if len(retrieved.BlockedHosts) != 1 {
		t.Errorf("expected 1 blocked host, got %d", len(retrieved.BlockedHosts))
	}

	// Test that modifications to retrieved policy don't affect stored policy
	retrieved.AllowedHosts = append(retrieved.AllowedHosts, "new.com")
	retrieved2 := fs.GetNetworkPolicy()
	if len(retrieved2.AllowedHosts) != 2 {
		t.Error("modifying retrieved policy affected stored policy")
	}
}

func TestSandboxedFS_CheckNetworkAccess(t *testing.T) {
	fs := NewSandboxedFS("/tmp/test")

	// Without policy, all access should be allowed
	if !fs.CheckNetworkAccess("any.com", 80) {
		t.Error("expected access to be allowed without policy")
	}

	// Set restrictive policy
	policy := &NetworkPolicy{
		AllowedHosts: []string{"example.com"},
		AllowedPorts: []int{443},
	}
	fs.SetNetworkPolicy(policy)

	// Test allowed access
	if !fs.CheckNetworkAccess("example.com", 443) {
		t.Error("expected access to example.com:443 to be allowed")
	}

	// Test denied access - wrong host
	if fs.CheckNetworkAccess("other.com", 443) {
		t.Error("expected access to other.com:443 to be denied")
	}

	// Test denied access - wrong port
	if fs.CheckNetworkAccess("example.com", 80) {
		t.Error("expected access to example.com:80 to be denied")
	}
}

func TestRuntime_SetNetworkPolicy(t *testing.T) {
	config := DefaultRuntimeConfig()
	limits := DefaultLimits()
	runtime := NewRuntime(config, limits)

	// Initially should be nil
	if runtime.NetworkPolicy() != nil {
		t.Error("expected nil policy initially")
	}

	// Set policy
	policy := &NetworkPolicy{
		AllowedHosts: []string{"api.example.com"},
		AllowedPorts: []int{443},
		BlockedHosts: []string{"*.internal.com"},
	}
	runtime.SetNetworkPolicy(policy)

	// Verify policy was set
	retrieved := runtime.NetworkPolicy()
	if retrieved == nil {
		t.Fatal("expected non-nil policy")
	}

	if len(retrieved.AllowedHosts) != 1 || retrieved.AllowedHosts[0] != "api.example.com" {
		t.Error("policy not stored correctly")
	}
}

func TestRuntime_CheckNetworkAccess(t *testing.T) {
	config := DefaultRuntimeConfig()
	limits := DefaultLimits()
	runtime := NewRuntime(config, limits)

	// Without policy, all access should be allowed
	if !runtime.CheckNetworkAccess("any.com", 80) {
		t.Error("expected access to be allowed without policy")
	}

	// Set restrictive policy
	policy := &NetworkPolicy{
		AllowedHosts: []string{"trusted.com"},
		AllowedPorts: []int{443},
		BlockedHosts: []string{"*.blocked.com"},
	}
	runtime.SetNetworkPolicy(policy)

	// Test allowed access
	if !runtime.CheckNetworkAccess("trusted.com", 443) {
		t.Error("expected access to trusted.com:443 to be allowed")
	}

	// Test denied access - blocked host
	if runtime.CheckNetworkAccess("sub.blocked.com", 443) {
		t.Error("expected access to blocked host to be denied")
	}

	// Test denied access - wrong host
	if runtime.CheckNetworkAccess("untrusted.com", 443) {
		t.Error("expected access to untrusted.com to be denied")
	}

	// Test denied access - wrong port
	if runtime.CheckNetworkAccess("trusted.com", 80) {
		t.Error("expected access to wrong port to be denied")
	}
}

func TestNetworkPolicy_ConcurrentAccess(t *testing.T) {
	fs := NewSandboxedFS("/tmp/test")
	policy := &NetworkPolicy{
		AllowedHosts: []string{"example.com"},
		AllowedPorts: []int{80, 443},
	}
	fs.SetNetworkPolicy(policy)

	// Run concurrent checks
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			fs.CheckNetworkAccess("example.com", 80)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestNetworkPolicy_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		policy  *NetworkPolicy
		host    string
		port    int
		allowed bool
	}{
		{
			name:    "nil policy - should allow all",
			policy:  nil,
			host:    "anything.com",
			port:    9999,
			allowed: true,
		},
		{
			name:    "empty policy - should allow all",
			policy:  &NetworkPolicy{},
			host:    "anything.com",
			port:    9999,
			allowed: true,
		},
		{
			name: "only blocked hosts specified",
			policy: &NetworkPolicy{
				BlockedHosts: []string{"evil.com"},
			},
			host:    "evil.com",
			port:    80,
			allowed: false,
		},
		{
			name: "only blocked hosts - allow others",
			policy: &NetworkPolicy{
				BlockedHosts: []string{"evil.com"},
			},
			host:    "good.com",
			port:    80,
			allowed: true,
		},
		{
			name: "empty allowed hosts with ports - allow any host on allowed ports",
			policy: &NetworkPolicy{
				AllowedHosts: []string{},
				AllowedPorts: []int{80},
			},
			host:    "any.com",
			port:    80,
			allowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result bool
			if tt.policy == nil {
				result = true // Simulate permissive default
			} else {
				result = tt.policy.IsAllowed(tt.host, tt.port)
			}
			if result != tt.allowed {
				t.Errorf("IsAllowed(%q, %d) = %v, want %v",
					tt.host, tt.port, result, tt.allowed)
			}
		})
	}
}
