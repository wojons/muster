package wasm

import (
	"testing"
	"time"
)

func TestDefaultLimits(t *testing.T) {
	limits := DefaultLimits()

	if limits.MaxMemory != 128*1024*1024 {
		t.Errorf("expected MaxMemory to be 128MB, got %d", limits.MaxMemory)
	}
	if limits.MaxExecutionTime != 30*time.Second {
		t.Errorf("expected MaxExecutionTime to be 30s, got %v", limits.MaxExecutionTime)
	}
	if limits.MaxFileDescriptors != 64 {
		t.Errorf("expected MaxFileDescriptors to be 64, got %d", limits.MaxFileDescriptors)
	}
}

func TestLimitsValidate(t *testing.T) {
	tests := []struct {
		name    string
		limits  *SandboxLimits
		wantErr bool
	}{
		{"valid", DefaultLimits(), false},
		{"no memory", &SandboxLimits{MaxExecutionTime: 30 * time.Second}, true},
		{"no execution time", &SandboxLimits{MaxMemory: 128 * 1024 * 1024}, true},
		{"no file size", &SandboxLimits{MaxMemory: 128 * 1024 * 1024, MaxExecutionTime: 30 * time.Second}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.limits.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("SandboxLimits.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNetworkPolicy(t *testing.T) {
	policy := &NetworkPolicy{
		AllowedHosts: []string{"api.example.com", "*.internal"},
		AllowedPorts: []int{443, 80},
		BlockedHosts: []string{"evil.com"},
	}

	tests := []struct {
		name string
		host string
		port int
		want bool
	}{
		{"allowed host and port", "api.example.com", 443, true},
		{"wildcard allowed", "sub.internal", 80, true},
		{"blocked host", "evil.com", 80, false},
		{"allowed host wrong port", "api.example.com", 8080, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := policy.IsAllowed(tt.host, tt.port)
			if got != tt.want {
				t.Errorf("NetworkPolicy.IsAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultRuntimeConfig(t *testing.T) {
	cfg := DefaultRuntimeConfig()

	if !cfg.JITEnabled {
		t.Error("expected JIT to be enabled by default")
	}
	if cfg.OptLevel != 2 {
		t.Errorf("expected OptLevel to be 2, got %d", cfg.OptLevel)
	}
	if !cfg.CanonicalizeNaN {
		t.Error("expected CanonicalizeNaN to be true by default")
	}
	if !cfg.Determinism {
		t.Error("expected Determinism to be true by default")
	}
}

func TestNewRuntime(t *testing.T) {
	runtime := NewRuntime(nil, nil)

	if runtime == nil {
		t.Fatal("expected runtime to not be nil")
	}
	if runtime.config == nil {
		t.Error("expected config to be initialized")
	}
	if runtime.limits == nil {
		t.Error("expected limits to be initialized")
	}
	if runtime.capability == nil {
		t.Error("expected capability set to be initialized")
	}
}

func TestNewRuntimeWithDefaults(t *testing.T) {
	config := DefaultRuntimeConfig()
	limits := DefaultLimits()

	runtime := NewRuntime(config, limits)

	if runtime.Config() != config {
		t.Error("expected runtime config to match input")
	}
	if runtime.Limits() != limits {
		t.Error("expected runtime limits to match input")
	}
}

func TestGrantCapability(t *testing.T) {
	runtime := NewRuntime(nil, nil)

	err := runtime.GrantCapability("fs.read")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !runtime.CheckCapability("fs.read") {
		t.Error("expected capability to be granted")
	}
}

func TestCheckCapability(t *testing.T) {
	runtime := NewRuntime(nil, nil)

	if runtime.CheckCapability("fs.read") {
		t.Error("expected capability check to return false for ungranted capability")
	}

	_ = runtime.GrantCapability("fs.read")

	if !runtime.CheckCapability("fs.read") {
		t.Error("expected capability check to return true after granting")
	}
}

func TestConfigReturns(t *testing.T) {
	config := DefaultRuntimeConfig()
	runtime := NewRuntime(config, nil)

	if runtime.Config() != config {
		t.Error("expected Config() to return the same config instance")
	}
}

func TestLimitsReturns(t *testing.T) {
	limits := DefaultLimits()
	runtime := NewRuntime(nil, limits)

	if runtime.Limits() != limits {
		t.Error("expected Limits() to return the same limits instance")
	}
}

func TestCapabilitiesReturns(t *testing.T) {
	runtime := NewRuntime(nil, nil)

	caps := runtime.Capabilities()
	if caps == nil {
		t.Error("expected Capabilities() to return capability set")
	}
	if caps != runtime.capability {
		t.Error("expected Capabilities() to return the same capability set")
	}
}

func TestRuntimeWithCaps(t *testing.T) {
	runtime := NewRuntime(nil, nil)

	_ = runtime.GrantCapability("fs.read")
	_ = runtime.GrantCapability("fs.write")

	if !runtime.CheckCapability("fs.read") {
		t.Error("expected fs.read capability")
	}
	if !runtime.CheckCapability("fs.write") {
		t.Error("expected fs.write capability")
	}
	if runtime.CheckCapability("fs.execute") {
		t.Error("expected fs.execute to not be granted")
	}
}
