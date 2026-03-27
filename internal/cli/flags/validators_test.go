package flags

import (
	"os"
	"runtime"
	"testing"
)

// URLValidator Tests

func TestURLValidator_Validate_ValidURLs(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"HTTPS URL", "https://example.com"},
		{"HTTP URL", "http://example.com"},
		{"URL with port", "https://example.com:8080"},
		{"URL with path", "https://example.com/path/to/resource"},
		{"URL with query", "https://example.com?foo=bar"},
		{"URL with fragment", "https://example.com#section"},
		{"IPv4 URL", "http://192.168.1.1"},
		{"IPv6 URL", "http://[::1]"},
	}

	validator := NewURLValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if err != nil {
				t.Errorf("Validate(%q) returned error: %v", tt.value, err)
			}
		})
	}
}

func TestURLValidator_Validate_InvalidURLs(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		wantErr  bool
		errMatch string
	}{
		{"Empty string", "", true, "cannot be empty"},
		{"No scheme", "example.com", true, "must have a scheme"},
		{"Invalid characters", "https://exam ple.com", true, "invalid URL"},
		{"Just path", "/path/to/resource", true, "must have a scheme"},
	}

	validator := NewURLValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
				return
			}
			if err != nil && tt.errMatch != "" {
				if !contains(err.Error(), tt.errMatch) {
					t.Errorf("Validate(%q) error = %v, should contain %q", tt.value, err, tt.errMatch)
				}
			}
		})
	}
}

func TestURLValidator_WithAllowedSchemes(t *testing.T) {
	validator := &URLValidator{
		AllowedSchemes: []string{"https", "ftp"},
		RequireHost:    true,
	}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"HTTPS allowed", "https://example.com", false},
		{"FTP allowed", "ftp://example.com", false},
		{"HTTP not allowed", "http://example.com", true},
		{"File not allowed", "file:///etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestURLValidator_WithRelativeAllowed(t *testing.T) {
	validator := &URLValidator{
		AllowRelative: true,
		RequireHost:   false,
	}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"Absolute URL", "https://example.com", false},
		{"Relative path", "/path/to/resource", false},
		{"Relative URL", "path/to/resource", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestURLValidator_Name(t *testing.T) {
	validator := NewURLValidator()
	if got := validator.Name(); got != "url" {
		t.Errorf("Name() = %q, want %q", got, "url")
	}
}

func TestURL(t *testing.T) {
	validator := URL()
	if validator == nil {
		t.Error("URL() returned nil")
	}
	if validator.Name() != "url" {
		t.Errorf("URL().Name() = %q, want %q", validator.Name(), "url")
	}
}

func TestURLWithSchemes(t *testing.T) {
	validator := URLWithSchemes("https", "http")
	if validator == nil {
		t.Error("URLWithSchemes() returned nil")
	}

	// Test allowed scheme
	if err := validator.Validate("https://example.com"); err != nil {
		t.Errorf("Validate(https://example.com) returned error: %v", err)
	}

	// Test disallowed scheme
	if err := validator.Validate("ftp://example.com"); err == nil {
		t.Error("Validate(ftp://example.com) should have returned error")
	}
}

// IPValidator Tests

func TestIPValidator_Validate_ValidIPs(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		validator *IPValidator
	}{
		{"IPv4 localhost", "127.0.0.1", NewIPValidator()},
		{"IPv4 private", "192.168.1.1", NewIPValidator()},
		{"IPv4 public", "8.8.8.8", NewIPValidator()},
		{"IPv6 localhost", "::1", NewIPValidator()},
		{"IPv6 full", "2001:db8::1", NewIPValidator()},
		{"IPv4 only", "192.168.1.1", NewIPv4Validator()},
		{"IPv6 only", "::1", NewIPv6Validator()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validator.Validate(tt.value)
			if err != nil {
				t.Errorf("Validate(%q) returned error: %v", tt.value, err)
			}
		})
	}
}

func TestIPValidator_Validate_InvalidIPs(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		wantErr  bool
		errMatch string
	}{
		{"Empty string", "", true, "cannot be empty"},
		{"Invalid format", "not-an-ip", true, "invalid IP"},
		{"Too many octets", "192.168.1.1.1", true, "invalid IP"},
		{"Invalid characters", "192.168.1.a", true, "invalid IP"},
		{"Empty octet", "192.168..1", true, "invalid IP"},
	}

	validator := NewIPValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
				return
			}
			if err != nil && tt.errMatch != "" {
				if !contains(err.Error(), tt.errMatch) {
					t.Errorf("Validate(%q) error = %v, should contain %q", tt.value, err, tt.errMatch)
				}
			}
		})
	}
}

func TestIPValidator_IPv4Only(t *testing.T) {
	validator := NewIPv4Validator()

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"IPv4 allowed", "192.168.1.1", false},
		{"IPv6 not allowed", "::1", true},
		{"IPv6 full not allowed", "2001:db8::1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestIPValidator_IPv6Only(t *testing.T) {
	validator := NewIPv6Validator()

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"IPv6 allowed", "::1", false},
		{"IPv6 full allowed", "2001:db8::1", false},
		{"IPv4 not allowed", "192.168.1.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestIPValidator_NoLoopback(t *testing.T) {
	validator := &IPValidator{
		AllowIPv4:     true,
		AllowIPv6:     true,
		AllowLoopback: false,
		AllowPrivate:  true,
	}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"IPv4 loopback blocked", "127.0.0.1", true},
		{"IPv6 loopback blocked", "::1", true},
		{"Private allowed", "192.168.1.1", false},
		{"Public allowed", "8.8.8.8", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestIPValidator_NoPrivate(t *testing.T) {
	validator := &IPValidator{
		AllowIPv4:     true,
		AllowIPv6:     true,
		AllowLoopback: true,
		AllowPrivate:  false,
	}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"10.x.x.x blocked", "10.0.0.1", true},
		{"172.16.x.x blocked", "172.16.0.1", true},
		{"192.168.x.x blocked", "192.168.1.1", true},
		{"Loopback allowed", "127.0.0.1", false},
		{"Public allowed", "8.8.8.8", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestIPValidator_Name(t *testing.T) {
	validator := NewIPValidator()
	if got := validator.Name(); got != "ip" {
		t.Errorf("Name() = %q, want %q", got, "ip")
	}
}

func TestIP(t *testing.T) {
	validator := IP()
	if validator == nil {
		t.Error("IP() returned nil")
	}
	if validator.Name() != "ip" {
		t.Errorf("IP().Name() = %q, want %q", validator.Name(), "ip")
	}
}

func TestIPv4(t *testing.T) {
	validator := IPv4()
	if validator == nil {
		t.Error("IPv4() returned nil")
	}
	if err := validator.Validate("192.168.1.1"); err != nil {
		t.Errorf("IPv4() validator failed on valid IPv4: %v", err)
	}
	if err := validator.Validate("::1"); err == nil {
		t.Error("IPv4() validator should reject IPv6 addresses")
	}
}

func TestIPv6(t *testing.T) {
	validator := IPv6()
	if validator == nil {
		t.Error("IPv6() returned nil")
	}
	if err := validator.Validate("::1"); err != nil {
		t.Errorf("IPv6() validator failed on valid IPv6: %v", err)
	}
	if err := validator.Validate("192.168.1.1"); err == nil {
		t.Error("IPv6() validator should reject IPv4 addresses")
	}
}

// EmailValidator Tests

func TestEmailValidator_Validate_ValidEmails(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"Simple email", "test@example.com"},
		{"With dots", "first.last@example.com"},
		{"With plus", "user+tag@example.com"},
		{"With numbers", "user123@example.com"},
		{"Subdomain", "user@mail.example.com"},
		{"Long TLD", "user@example.museum"},
	}

	validator := NewEmailValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if err != nil {
				t.Errorf("Validate(%q) returned error: %v", tt.value, err)
			}
		})
	}
}

func TestEmailValidator_Validate_InvalidEmails(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		wantErr  bool
		errMatch string
	}{
		{"Empty string", "", true, "cannot be empty"},
		{"No @ symbol", "test.example.com", true, "invalid email"},
		{"No domain", "test@", true, "invalid email"},
		{"No local part", "@example.com", true, "invalid email"},
		{"Multiple @", "test@@example.com", true, "invalid email"},
		{"Spaces", "test user@example.com", true, "invalid email"},
		// Note: test*user@example.com is actually valid per RFC 5321 - asterisk is allowed in local part
	}

	validator := NewEmailValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
				return
			}
			if err != nil && tt.errMatch != "" {
				if !contains(err.Error(), tt.errMatch) {
					t.Errorf("Validate(%q) error = %v, should contain %q", tt.value, err, tt.errMatch)
				}
			}
		})
	}
}

func TestEmailValidator_WithAllowedDomains(t *testing.T) {
	validator := &EmailValidator{
		AllowedDomains: []string{"example.com", "test.org"},
		RequireDomain:  true,
	}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"Allowed domain", "user@example.com", false},
		{"Another allowed domain", "user@test.org", false},
		{"Disallowed domain", "user@gmail.com", true},
		{"Case insensitive", "user@EXAMPLE.COM", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestEmailValidator_Name(t *testing.T) {
	validator := NewEmailValidator()
	if got := validator.Name(); got != "email" {
		t.Errorf("Name() = %q, want %q", got, "email")
	}
}

func TestEmail(t *testing.T) {
	validator := Email()
	if validator == nil {
		t.Error("Email() returned nil")
	}
	if validator.Name() != "email" {
		t.Errorf("Email().Name() = %q, want %q", validator.Name(), "email")
	}
}

func TestEmailWithDomains(t *testing.T) {
	validator := EmailWithDomains("example.com")
	if validator == nil {
		t.Error("EmailWithDomains() returned nil")
	}

	// Test allowed domain
	if err := validator.Validate("user@example.com"); err != nil {
		t.Errorf("Validate(user@example.com) returned error: %v", err)
	}

	// Test disallowed domain
	if err := validator.Validate("user@gmail.com"); err == nil {
		t.Error("Validate(user@gmail.com) should have returned error")
	}
}

// PathValidator Tests

func TestPathValidator_Validate_BasicPaths(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"Relative path", "./relative/path", false},
		{"Absolute path Unix", "/absolute/path", false},
		{"Path with dots", "./path/../other", false},
		{"Simple filename", "file.txt", false},
	}

	validator := NewPathValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestPathValidator_Validate_EmptyPath(t *testing.T) {
	validator := NewPathValidator()
	err := validator.Validate("")
	if err == nil {
		t.Error("Validate(\"\") should return error")
	}
	if !contains(err.Error(), "cannot be empty") {
		t.Errorf("Error should mention empty path, got: %v", err)
	}
}

func TestPathValidator_MustExist(t *testing.T) {
	// Create a temp file for testing
	tempFile, err := os.CreateTemp("", "validator-test-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()
	_ = tempFile.Close()

	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "validator-test-dir-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	validator := &PathValidator{
		MustExist:     true,
		AllowRelative: true,
	}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"Existing file", tempFile.Name(), false},
		{"Existing directory", tempDir, false},
		{"Non-existent path", "/path/that/does/not/exist", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestPathValidator_MustBeFile(t *testing.T) {
	// Create a temp file
	tempFile, err := os.CreateTemp("", "validator-test-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()

	// Write some content to ensure it's a valid file
	_, _ = tempFile.WriteString("test content")
	_ = tempFile.Close()

	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "validator-test-dir-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	validator := &PathValidator{
		MustExist:     true,
		MustBeFile:    true,
		AllowRelative: true,
	}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"Existing file", tempFile.Name(), false},
		{"Existing directory", tempDir, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestPathValidator_MustBeDir(t *testing.T) {
	// Create a temp file
	tempFile, err := os.CreateTemp("", "validator-test-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()
	_ = tempFile.Close()

	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "validator-test-dir-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	validator := &PathValidator{
		MustExist:     true,
		MustBeDir:     true,
		AllowRelative: true,
	}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"Existing directory", tempDir, false},
		{"Existing file", tempFile.Name(), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestPathValidator_MustBeReadable(t *testing.T) {
	// Create a readable temp file
	tempFile, err := os.CreateTemp("", "validator-test-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()
	_, _ = tempFile.WriteString("test content")
	_ = tempFile.Close()

	validator := &PathValidator{
		MustExist:      true,
		MustBeReadable: true,
		AllowRelative:  true,
	}

	err = validator.Validate(tempFile.Name())
	if err != nil {
		t.Errorf("Validate(%q) returned error: %v", tempFile.Name(), err)
	}
}

func TestPathValidator_MustBeWritable(t *testing.T) {
	// Create a temp file
	tempFile, err := os.CreateTemp("", "validator-test-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()
	_ = tempFile.Close()

	validator := &PathValidator{
		MustBeWritable: true,
		AllowRelative:  true,
	}

	err = validator.Validate(tempFile.Name())
	if err != nil {
		t.Errorf("Validate(%q) returned error: %v", tempFile.Name(), err)
	}
}

func TestPathValidator_AbsoluteOnly(t *testing.T) {
	validator := &PathValidator{
		AllowRelative: false,
	}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"Absolute Unix", "/absolute/path", false},
		{"Relative path", "./relative/path", true},
		{"Simple name", "filename.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestPathValidator_ResolveSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink tests on Windows")
	}

	// Create a temp file
	tempFile, err := os.CreateTemp("", "validator-test-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()
	_, _ = tempFile.WriteString("test content")
	_ = tempFile.Close()

	// Create a symlink to the temp file
	symlinkPath := tempFile.Name() + "-link"
	err = os.Symlink(tempFile.Name(), symlinkPath)
	if err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}
	defer func() { _ = os.Remove(symlinkPath) }()

	validator := &PathValidator{
		MustExist:       true,
		ResolveSymlinks: true,
		AllowRelative:   true,
	}

	err = validator.Validate(symlinkPath)
	if err != nil {
		t.Errorf("Validate(%q) with symlink resolution returned error: %v", symlinkPath, err)
	}
}

func TestPathValidator_Name(t *testing.T) {
	validator := NewPathValidator()
	if got := validator.Name(); got != "path" {
		t.Errorf("Name() = %q, want %q", got, "path")
	}
}

func TestPath(t *testing.T) {
	validator := Path()
	if validator == nil {
		t.Error("Path() returned nil")
	}
	if validator.Name() != "path" {
		t.Errorf("Path().Name() = %q, want %q", validator.Name(), "path")
	}
}

func TestExistingFile(t *testing.T) {
	validator := ExistingFile()
	if validator == nil {
		t.Error("ExistingFile() returned nil")
	}

	// Create a temp file
	tempFile, err := os.CreateTemp("", "validator-test-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()
	_, _ = tempFile.WriteString("test")
	_ = tempFile.Close()

	// Should validate existing file
	if err := validator.Validate(tempFile.Name()); err != nil {
		t.Errorf("ExistingFile() validator failed on existing file: %v", err)
	}

	// Should fail on non-existent file
	if err := validator.Validate("/non/existent/file.txt"); err == nil {
		t.Error("ExistingFile() validator should fail on non-existent file")
	}
}

func TestExistingDir(t *testing.T) {
	validator := ExistingDir()
	if validator == nil {
		t.Error("ExistingDir() returned nil")
	}

	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "validator-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Should validate existing directory
	if err := validator.Validate(tempDir); err != nil {
		t.Errorf("ExistingDir() validator failed on existing directory: %v", err)
	}

	// Should fail on non-existent directory
	if err := validator.Validate("/non/existent/directory"); err == nil {
		t.Error("ExistingDir() validator should fail on non-existent directory")
	}
}

func TestWritablePath(t *testing.T) {
	validator := WritablePath()
	if validator == nil {
		t.Error("WritablePath() returned nil")
	}

	// Create a temp file
	tempFile, err := os.CreateTemp("", "validator-test-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()
	_ = tempFile.Close()

	// Should validate writable file
	if err := validator.Validate(tempFile.Name()); err != nil {
		t.Errorf("WritablePath() validator failed on writable file: %v", err)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(s[:len(substr)] == substr) ||
		(s[len(s)-len(substr):] == substr) ||
		containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Integration Tests

func TestValidatorWithFlagDefinition(t *testing.T) {
	// Test URL validator with flag definition
	urlDef := &FlagDefinition{
		Name:        "endpoint",
		Type:        FlagTypeURL,
		Description: "API endpoint URL",
		Required:    true,
		Validator:   URL(),
	}

	if err := urlDef.Validate(); err != nil {
		t.Errorf("FlagDefinition with URL validator failed: %v", err)
	}

	// Test IP validator with flag definition
	ipDef := &FlagDefinition{
		Name:        "server-ip",
		Type:        FlagTypeIP,
		Description: "Server IP address",
		Validator:   IP(),
	}

	if err := ipDef.Validate(); err != nil {
		t.Errorf("FlagDefinition with IP validator failed: %v", err)
	}

	// Test email validator with flag definition
	emailDef := &FlagDefinition{
		Name:        "user-email",
		Type:        FlagTypeEmail,
		Description: "User email address",
		Validator:   Email(),
	}

	if err := emailDef.Validate(); err != nil {
		t.Errorf("FlagDefinition with Email validator failed: %v", err)
	}

	// Test path validator with flag definition
	pathDef := &FlagDefinition{
		Name:        "config-file",
		Type:        FlagTypePath,
		Description: "Path to config file",
		Validator:   ExistingFile(),
	}

	if err := pathDef.Validate(); err != nil {
		t.Errorf("FlagDefinition with Path validator failed: %v", err)
	}
}

func TestValidatorWithParser(t *testing.T) {
	parser := NewParser()

	// Add definitions with validators
	definitions := []*FlagDefinition{
		{
			Name:        "api-url",
			Type:        FlagTypeURL,
			Description: "API URL",
			Validator:   URL(),
		},
		{
			Name:        "bind-addr",
			Type:        FlagTypeIP,
			Description: "Bind address",
			Validator:   IP(),
		},
		{
			Name:        "contact",
			Type:        FlagTypeEmail,
			Description: "Contact email",
			Validator:   Email(),
		},
	}

	for _, def := range definitions {
		if err := parser.AddDefinition(def); err != nil {
			t.Errorf("AddDefinition(%q) failed: %v", def.Name, err)
		}
	}

	// Test that definitions were added
	if len(parser.ListDefinitions()) != 3 {
		t.Errorf("Expected 3 definitions, got %d", len(parser.ListDefinitions()))
	}
}

func TestAllValidatorNames(t *testing.T) {
	validators := []struct {
		name      string
		validator Validator
	}{
		{"url", URL()},
		{"ip", IP()},
		{"email", Email()},
		{"path", Path()},
	}

	for _, v := range validators {
		t.Run(v.name, func(t *testing.T) {
			if v.validator.Name() != v.name {
				t.Errorf("Name() = %q, want %q", v.validator.Name(), v.name)
			}
		})
	}
}

// Benchmarks

func BenchmarkURLValidator_Validate(b *testing.B) {
	validator := NewURLValidator()
	url := "https://example.com/path?query=value"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.Validate(url)
	}
}

func BenchmarkIPValidator_Validate(b *testing.B) {
	validator := NewIPValidator()
	ip := "192.168.1.1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.Validate(ip)
	}
}

func BenchmarkEmailValidator_Validate(b *testing.B) {
	validator := NewEmailValidator()
	email := "user@example.com"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.Validate(email)
	}
}

func BenchmarkPathValidator_Validate(b *testing.B) {
	validator := NewPathValidator()
	path := "/path/to/file.txt"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.Validate(path)
	}
}
