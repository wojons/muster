package wasm

import (
	"testing"
)

func TestNewCapabilitySet(t *testing.T) {
	cs := NewCapabilitySet()
	if cs == nil {
		t.Fatal("expected capability set to not be nil")
	}
}

func TestGrant(t *testing.T) {
	cs := NewCapabilitySet()

	err := cs.Grant("fs.read")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !cs.Check("fs.read") {
		t.Error("expected capability to be granted")
	}
}

func TestGrantInvalid(t *testing.T) {
	cs := NewCapabilitySet()

	err := cs.Grant("invalid")
	if err == nil {
		t.Error("expected error for invalid capability")
	}
}

func TestGrantWildcard(t *testing.T) {
	cs := NewCapabilitySet()

	err := cs.Grant("fs.read.*")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !cs.Check("fs.read./tmp/file") {
		t.Error("expected wildcard capability to allow file access")
	}

	if !cs.Check("fs.read./var/log/test") {
		t.Error("expected wildcard capability to allow any read")
	}
}

func TestCheckNotGranted(t *testing.T) {
	cs := NewCapabilitySet()

	if cs.Check("fs.read") {
		t.Error("expected capability check to return false for ungranted capability")
	}
}

func TestRevoke(t *testing.T) {
	cs := NewCapabilitySet()

	cs.Grant("fs.read")

	err := cs.Revoke("fs.read")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if cs.Check("fs.read") {
		t.Error("expected capability to be revoked")
	}
}

func TestRevokeWildcard(t *testing.T) {
	cs := NewCapabilitySet()

	cs.Grant("fs.write.*")
	cs.Revoke("fs.write.*")

	if cs.Check("fs.write./tmp/file") {
		t.Error("expected wildcard capability to be revoked")
	}
}

func TestRevokeNotFound(t *testing.T) {
	cs := NewCapabilitySet()

	err := cs.Revoke("fs.read")
	if err == nil {
		t.Error("expected error for revoking non-existent capability")
	}
}

func TestList(t *testing.T) {
	cs := NewCapabilitySet()

	cs.Grant("fs.read")
	cs.Grant("fs.write")
	cs.Grant("net.http")

	caps := cs.List()
	if len(caps) != 3 {
		t.Errorf("expected 3 capabilities, got %d", len(caps))
	}
}

func TestListEmpty(t *testing.T) {
	cs := NewCapabilitySet()

	caps := cs.List()
	if len(caps) != 0 {
		t.Errorf("expected 0 capabilities, got %d", len(caps))
	}
}

func TestMatchWildcard(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		resource string
		want     bool
	}{
		{"exact match", "fs.read", "fs.read", true},
		{"wildcard all", "*", "anything", true},
		{"prefix wildcard", "fs.read.**", "fs.read./tmp/file", true},
		{"no match", "fs.write", "fs.read", false},
		{"partial match", "fs.read.*", "fs.read./tmp", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchWildcard(tt.pattern, tt.resource)
			if got != tt.want {
				t.Errorf("matchWildcard() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseCapability(t *testing.T) {
	tests := []struct {
		name    string
		capStr  string
		wantErr bool
	}{
		{"valid capability", "fs.read", false},
		{"with resource", "fs.read./tmp", false},
		{"invalid", "", true},
		{"no category", "read", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCapability(tt.capStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCapability() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMultipleCapabilities(t *testing.T) {
	cs := NewCapabilitySet()

	caps := []string{
		"fs.read",
		"fs.write",
		"net.http",
		"env.read",
	}

	for _, cap := range caps {
		err := cs.Grant(cap)
		if err != nil {
			t.Errorf("unexpected error granting %s: %v", cap, err)
		}
	}

	for _, cap := range caps {
		if !cs.Check(cap) {
			t.Errorf("expected capability %s to be granted", cap)
		}
	}

	if cs.Check("fs.execute") {
		t.Error("expected ungranted capability to be denied")
	}
}

func TestWildcardPatterns(t *testing.T) {
	cs := NewCapabilitySet()

	cs.Grant("fs.**")

	tests := []struct {
		capability string
		want       bool
	}{
		{"fs.read", true},
		{"fs.write", true},
		{"fs.read./tmp", true},
		{"fs.execute./bin/sh", true},
		{"net.http", false},
	}

	for _, tt := range tests {
		t.Run(tt.capability, func(t *testing.T) {
			got := cs.Check(tt.capability)
			if got != tt.want {
				t.Errorf("Check(%s) = %v, want %v", tt.capability, got, tt.want)
			}
		})
	}
}
