package wasm

import (
	"strings"
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

	_ = cs.Grant("fs.read")

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

	_ = cs.Grant("fs.write.*")
	_ = cs.Revoke("fs.write.*")

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

	_ = cs.Grant("fs.read")
	_ = cs.Grant("fs.write")
	_ = cs.Grant("net.http")

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
		{"exact match", "read", "read", true},
		{"wildcard all", "*", "anything", true},
		{"prefix wildcard", "read./tmp/**", "read./tmp/file", true},
		{"no match", "write", "read", false},
		{"partial match", "read.*", "read.tmp", true},
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

	_ = cs.Grant("fs.**")

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

// TestSanitizeResourceName tests the sanitizeResourceName function
func TestSanitizeResourceName(t *testing.T) {
	tests := []struct {
		name        string
		resource    string
		want        string
		wantErr     bool
		errContains string
	}{
		{"valid path", "/tmp/file", "/tmp/file", false, ""},
		{"empty string", "", "", true, "empty"},
		{"path traversal with ..", "../../../etc/passwd", "", true, "traversal"},
		{"path traversal in middle", "/tmp/../etc/passwd", "", true, "traversal"},
		{"backslash", "C:\\Windows\\System32", "", true, "backslash"},
		{"double dots alone", "..", "", true, "traversal"},
		{"valid with dots", "/var/log/test.log", "/var/log/test.log", false, ""},
		{"normalize redundant slashes", "//tmp///file", "/tmp/file", false, ""},
		{"normalize trailing slash", "/tmp/", "/tmp", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizeResourceName(tt.resource)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizeResourceName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("sanitizeResourceName() error = %v, want error containing %q", err, tt.errContains)
				}
			}
			if got != tt.want {
				t.Errorf("sanitizeResourceName() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSanitizePattern tests the sanitizePattern function
func TestSanitizePattern(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		want        string
		wantErr     bool
		errContains string
	}{
		{"simple pattern", "/tmp", "/tmp", false, ""},
		{"wildcard all", "*", "*", false, ""},
		{"prefix wildcard", "/tmp/**", "/tmp/**", false, ""},
		{"middle wildcard", "/var/*/log", "/var*/log", false, ""},
		{"empty pattern", "", "", true, "empty"},
		{"backslash in pattern", "C:\\**", "", true, "backslash"},
		{"traversal in prefix", "../**", "", true, "traversal"},
		{"traversal in middle", "/var/foo/../etc/**", "", true, "traversal"},
		{"normalize redundant slashes", "//tmp//**", "/tmp/**", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizePattern(tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizePattern() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("sanitizePattern() error = %v, want error containing %q", err, tt.errContains)
				}
			}
			if got != tt.want {
				t.Errorf("sanitizePattern() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMatchWildcardPathTraversal tests that path traversal attacks are blocked
func TestMatchWildcardPathTraversal(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		resource string
		want     bool
		reason   string
	}{
		// Legitimate matches
		{"legitimate prefix", "read./tmp/**", "read./tmp/file", true, "should match legitimate files"},
		{"legitimate wildcard", "read./var/*", "read./var/log", true, "should match single level"},

		// Path traversal attacks - MUST BE BLOCKED
		{"traversal escape", "/tmp/**", "/tmp/../../../etc/passwd", false, "should block path traversal"},
		{"traversal with dots", "/tmp/**", "/tmp/../../etc/shadow", false, "should block traversal to /etc"},
		{"hidden traversal", "/home/user/**", "/home/user/../root/.ssh/id_rsa", false, "should block hidden traversal"},
		{"double traversal", "/tmp/**", "/tmp/foo/../../bar/../../etc/passwd", false, "should block multiple traversals"},

		// Backslash attacks - MUST BE BLOCKED
		{"backslash escape", "C:\\**", "C:\\Windows\\System32", false, "should block backslash paths"},

		// Normalized paths should still work
		{"normalized redundant slashes", "read.//tmp//**", "read.//tmp//file", true, "should normalize redundant slashes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchWildcard(tt.pattern, tt.resource)
			if got != tt.want {
				t.Errorf("matchWildcard(%q, %q) = %v, want %v - %s", tt.pattern, tt.resource, got, tt.want, tt.reason)
			}
		})
	}
}

// TestCapabilitySetPathTraversal tests that CapabilitySet blocks path traversal
func TestCapabilitySetPathTraversal(t *testing.T) {
	cs := NewCapabilitySet()

	// Grant access to /tmp only
	err := cs.Grant("fs./tmp/**")
	if err != nil {
		t.Fatalf("failed to grant capability: %v", err)
	}

	tests := []struct {
		capability string
		want       bool
		reason     string
	}{
		// Legitimate access - SHOULD ALLOW
		{"fs./tmp/file.txt", true, "should allow access to /tmp files"},
		{"fs./tmp/subdir/file", true, "should allow access to /tmp subdirectories"},

		// Path traversal attacks - MUST BLOCK
		{"fs./tmp/../../../etc/passwd", false, "should block traversal to /etc/passwd"},
		{"fs./tmp/../../root/.ssh/id_rsa", false, "should block traversal to root SSH keys"},
		{"fs./tmp/foo/../../etc/shadow", false, "should block traversal to /etc/shadow"},
		{"fs./tmp/../../../home/user/.bashrc", false, "should block traversal to home directories"},
	}

	for _, tt := range tests {
		t.Run(tt.capability, func(t *testing.T) {
			got := cs.Check(tt.capability)
			if got != tt.want {
				t.Errorf("Check(%q) = %v, want %v - %s", tt.capability, got, tt.want, tt.reason)
			}
		})
	}
}

// TestWildcardPrivilegeEscalation_Prevention specifically tests the SEC-ADV-014 vulnerability fix
func TestWildcardPrivilegeEscalation_Prevention(t *testing.T) {
	// This test proves that the privilege escalation vulnerability is fixed

	// Scenario 1: Grant access to /tmp, try to escape to /etc
	t.Run("tmp_to_etc_escape", func(t *testing.T) {
		cs := NewCapabilitySet()
		_ = cs.Grant("fs./tmp/**")

		// Attempt privilege escalation via path traversal
		if cs.Check("fs./tmp/../../../etc/passwd") {
			t.Error("SEC-ADV-014 VULNERABILITY: Path traversal escape to /etc/passwd succeeded!")
		}

		if cs.Check("fs./tmp/../../etc/shadow") {
			t.Error("SEC-ADV-014 VULNERABILITY: Path traversal escape to /etc/shadow succeeded!")
		}
	})

	// Scenario 2: Grant access to user home, try to escape to root
	t.Run("home_to_root_escape", func(t *testing.T) {
		cs := NewCapabilitySet()
		_ = cs.Grant("fs./home/user/**")

		// Attempt privilege escalation to root directory
		if cs.Check("fs./home/user/../../root/.ssh/id_rsa") {
			t.Error("SEC-ADV-014 VULNERABILITY: Path traversal escape to root SSH key succeeded!")
		}

		if cs.Check("fs./home/user/../../../etc/passwd") {
			t.Error("SEC-ADV-014 VULNERABILITY: Path traversal escape to /etc/passwd succeeded!")
		}
	})

	// Scenario 3: Grant access to var/log, try to escape to var/secrets
	t.Run("log_to_secrets_escape", func(t *testing.T) {
		cs := NewCapabilitySet()
		_ = cs.Grant("fs./var/log/**")

		// Attempt privilege escalation
		if cs.Check("fs./var/log/../../secrets/key.pem") {
			t.Error("SEC-ADV-014 VULNERABILITY: Path traversal escape to secrets succeeded!")
		}
	})

	// Scenario 4: Verify legitimate access still works
	t.Run("legitimate_access_still_works", func(t *testing.T) {
		cs := NewCapabilitySet()
		_ = cs.Grant("fs./tmp/**")

		// These should still work
		if !cs.Check("fs./tmp/test.txt") {
			t.Error("REGRESSION: Legitimate access to /tmp/test.txt blocked!")
		}

		if !cs.Check("fs./tmp/subdir/nested/file") {
			t.Error("REGRESSION: Legitimate nested access blocked!")
		}
	})
}
