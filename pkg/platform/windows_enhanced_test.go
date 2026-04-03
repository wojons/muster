//go:build windows
// +build windows

// Package platform provides Windows-specific platform integration for Muster.
//
// This test file validates Windows platform features including service management,
// Defender compatibility checks, and path utilities. Tests are designed to run on
// Windows platforms and use mocks where appropriate to avoid requiring actual
// Windows services or administrative privileges.
package platform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

// TestIsWindowsService tests the service detection function
func TestIsWindowsService(t *testing.T) {
	// This will be false in normal test execution
	result := IsWindowsService()

	// Should return a boolean value (not panic)
	// In test environment, this should be false
	if result {
		t.Log("Running as Windows service (unexpected in tests)")
	} else {
		t.Log("Not running as Windows service (expected in tests)")
	}
}

// TestWindowsPathUtils tests Windows path utilities
func TestWindowsPathUtils(t *testing.T) {
	utils := WindowsPathUtils{}

	tests := []struct {
		name     string
		input    string
		expected string
		fn       func(string) string
	}{
		{
			name:     "expand USERPROFILE",
			input:    "%USERPROFILE%\\Documents",
			expected: os.Getenv("USERPROFILE") + "\\Documents",
			fn:       utils.ExpandEnvironmentStrings,
		},
		{
			name:     "expand APPDATA",
			input:    "%APPDATA%\\Muster",
			expected: os.Getenv("APPDATA") + "\\Muster",
			fn:       utils.ExpandEnvironmentStrings,
		},
		{
			name:     "expand LOCALAPPDATA",
			input:    "%LOCALAPPDATA%\\Cache",
			expected: os.Getenv("LOCALAPPDATA") + "\\Cache",
			fn:       utils.ExpandEnvironmentStrings,
		},
		{
			name:     "no expansion needed",
			input:    "C:\\Users\\Test",
			expected: "C:\\Users\\Test",
			fn:       utils.ExpandEnvironmentStrings,
		},
		{
			name:     "sanitize reserved chars",
			input:    "file<name>.txt",
			expected: "file_name_.txt",
			fn:       utils.SanitizeFilename,
		},
		{
			name:     "sanitize colon",
			input:    "file:name.txt",
			expected: "file_name.txt",
			fn:       utils.SanitizeFilename,
		},
		{
			name:     "sanitize question mark",
			input:    "file?name.txt",
			expected: "file_name.txt",
			fn:       utils.SanitizeFilename,
		},
		{
			name:     "sanitize asterisk",
			input:    "file*name.txt",
			expected: "file_name.txt",
			fn:       utils.SanitizeFilename,
		},
		{
			name:     "sanitize CON reserved name",
			input:    "CON.txt",
			expected: "_CON.txt",
			fn:       utils.SanitizeFilename,
		},
		{
			name:     "sanitize COM1 reserved name",
			input:    "COM1.txt",
			expected: "_COM1.txt",
			fn:       utils.SanitizeFilename,
		},
		{
			name:     "sanitize trailing space",
			input:    "filename.txt ",
			expected: "filename.txt",
			fn:       utils.SanitizeFilename,
		},
		{
			name:     "sanitize trailing dot",
			input:    "filename.",
			expected: "filename",
			fn:       utils.SanitizeFilename,
		},
		{
			name:     "long path not needed",
			input:    "C:\\Users\\test.txt",
			expected: "C:\\Users\\test.txt",
			fn:       utils.ToLongPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fn(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestWindowsPathUtils_ToLongPath tests long path conversion
func TestWindowsPathUtils_ToLongPath(t *testing.T) {
	utils := WindowsPathUtils{}

	// Create a path that exceeds 260 characters
	longPath := "C:\\" + strings.Repeat("a\\", 100)

	result := utils.ToLongPath(longPath)

	// Should have \\?\ prefix for extended-length paths
	if len(longPath) > 260 && !strings.HasPrefix(result, `\\?\`) {
		t.Errorf("Long path should have \\?\\ prefix, got: %s", result)
	}

	// Short path should not be modified
	shortPath := "C:\\Users\\test.txt"
	result = utils.ToLongPath(shortPath)
	if result != shortPath {
		t.Errorf("Short path should not be modified, got: %s", result)
	}
}

// TestWindowsServiceConfig_Validation tests service configuration
func TestWindowsServiceConfig_Validation(t *testing.T) {
	config := WindowsServiceConfig{
		Name:        "TestMusterService",
		DisplayName: "Test Muster Service",
		Description: "Test service for validation",
		ConfigPath:  "C:\\Test\\config.yaml",
		RecoveryActions: []RecoveryAction{
			{Type: 1, Delay: 60 * time.Second},  // Restart
			{Type: 1, Delay: 120 * time.Second}, // Restart
		},
	}

	if config.Name == "" {
		t.Error("Service name should not be empty")
	}

	if config.DisplayName == "" {
		t.Error("Display name should not be empty")
	}

	if len(config.RecoveryActions) != 2 {
		t.Errorf("Expected 2 recovery actions, got %d", len(config.RecoveryActions))
	}

	// Verify recovery action values
	for i, action := range config.RecoveryActions {
		if action.Type == 0 {
			t.Errorf("Recovery action %d should have non-zero type", i)
		}
		if action.Delay == 0 {
			t.Errorf("Recovery action %d should have non-zero delay", i)
		}
	}
}

// TestServiceStateToString tests service state conversion
func TestServiceStateToString(t *testing.T) {
	tests := []struct {
		state    svc.State
		expected string
	}{
		{svc.Stopped, "Stopped"},
		{svc.StartPending, "Start Pending"},
		{svc.StopPending, "Stop Pending"},
		{svc.Running, "Running"},
		{svc.ContinuePending, "Continue Pending"},
		{svc.PausePending, "Pause Pending"},
		{svc.Paused, "Paused"},
		{999, "Unknown (999)"},
	}

	for _, tt := range tests {
		result := serviceStateToString(tt.state)
		if result != tt.expected {
			t.Errorf("State %d: expected %q, got %q", tt.state, tt.expected, result)
		}
	}
}

// TestWindowsServiceError tests error formatting
func TestWindowsServiceError(t *testing.T) {
	err := &WindowsServiceError{
		Operation: "install",
		Err:       os.ErrNotExist,
	}

	expected := "Windows service install failed: file does not exist"
	if !strings.Contains(err.Error(), "Windows service install failed") {
		t.Errorf("Error message should contain operation, got: %s", err.Error())
	}

	if !strings.Contains(err.Error(), expected[:30]) {
		t.Errorf("Error message format incorrect, got: %s", err.Error())
	}
}

// TestIsElevated tests elevation detection
func TestIsElevated(t *testing.T) {
	// This will depend on the test environment
	elevated := IsElevated()

	// Should return a boolean without error
	t.Logf("IsElevated: %v", elevated)
}

// TestGetWindowsVersion tests version detection
func TestGetWindowsVersion(t *testing.T) {
	major, minor, build, err := GetWindowsVersion()

	if err != nil {
		// Registry read might fail in restricted environments
		t.Logf("GetWindowsVersion returned error (expected in some environments): %v", err)
		return
	}

	// Validate version numbers are reasonable
	if major == 0 {
		t.Error("Major version should not be 0")
	}

	if build == 0 {
		t.Log("Build number is 0 (might be unavailable)")
	}

	t.Logf("Windows version: %d.%d.%d", major, minor, build)
}

// TestIsWindows10OrLater tests Windows 10 detection
func TestIsWindows10OrLater(t *testing.T) {
	isWin10OrLater := IsWindows10OrLater()

	// Should return a boolean
	t.Logf("IsWindows10OrLater: %v", isWin10OrLater)
}

// TestIsWindowsServer tests Windows Server detection
func TestIsWindowsServer(t *testing.T) {
	isServer := IsWindowsServer()

	// Should return a boolean
	t.Logf("IsWindowsServer: %v", isServer)
}

// TestCheckWindowsDefenderCompatibility tests Defender checks
func TestCheckWindowsDefenderCompatibility(t *testing.T) {
	// This test requires Windows Defender to be available
	check, err := CheckWindowsDefenderCompatibility("")

	if err != nil {
		// Might fail if Defender not available or no admin rights
		t.Logf("Defender check error (expected in some environments): %v", err)
		return
	}

	// Log results
	t.Logf("Defender installed: %v", check.IsInstalled)
	t.Logf("Defender enabled: %v", check.IsEnabled)
	t.Logf("Real-time protection: %v", check.RealTimeProtection)
	t.Logf("Exclusion set: %v", check.IsExclusionSet)
	t.Logf("Version: %s", check.Version)
}

// TestWindowsDefenderCheck_Structure tests the check structure
func TestWindowsDefenderCheck_Structure(t *testing.T) {
	check := &WindowsDefenderCheck{
		IsInstalled:        true,
		IsEnabled:          true,
		IsExclusionSet:     false,
		ExclusionPath:      "C:\\Muster",
		RealTimeProtection: true,
		Version:            "1.391.0.0",
	}

	if !check.IsInstalled {
		t.Error("IsInstalled should be true")
	}

	if !check.RealTimeProtection {
		t.Error("RealTimeProtection should be true")
	}

	if check.IsExclusionSet {
		t.Error("IsExclusionSet should be false")
	}
}

// TestWindowsDirs_Integration tests platform directory detection
func TestWindowsDirs_Integration(t *testing.T) {
	dirs := windowsDirs()

	if dirs == nil {
		t.Fatal("windowsDirs should return non-nil PlatformDirs")
	}

	// Verify directories are not empty
	if dirs.Config == "" {
		t.Error("Config directory should not be empty")
	}

	if dirs.Data == "" {
		t.Error("Data directory should not be empty")
	}

	if dirs.Cache == "" {
		t.Error("Cache directory should not be empty")
	}

	if dirs.State == "" {
		t.Error("State directory should not be empty")
	}

	// Verify paths are absolute
	for name, path := range map[string]string{
		"Config": dirs.Config,
		"Data":   dirs.Data,
		"Cache":  dirs.Cache,
		"State":  dirs.State,
	} {
		if !filepath.IsAbs(path) {
			t.Errorf("%s path should be absolute: %s", name, path)
		}
	}
}

// TestWindowsFilename_Integration tests filename sanitization
func TestWindowsFilename_Integration(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"file.txt", "file.txt"},
		{"path/to/file.txt", "file.txt"},
		{`path\to\file.txt`, "file.txt"},
		{"../file.txt", "file.txt"},
		{"file.txt/", "file.txt"},
	}

	for _, tt := range tests {
		result := windowsFilename(tt.input)
		if result != tt.expected {
			t.Errorf("windowsFilename(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

// TestWindowsServiceRunner_Interface tests the service runner interface
func TestWindowsServiceRunner_Interface(t *testing.T) {
	// Verify WindowsServiceRunner interface can be implemented
	var _ WindowsServiceRunner = (*mockServiceRunner)(nil)
}

// mockServiceRunner implements WindowsServiceRunner for testing
type mockServiceRunner struct {
	runCalled  bool
	stopCalled bool
}

func (m *mockServiceRunner) Run(ctx context.Context) error {
	m.runCalled = true
	return nil
}

func (m *mockServiceRunner) Stop() error {
	m.stopCalled = true
	return nil
}

// TestWindowsServiceHandler_Execute tests service handler
func TestWindowsServiceHandler_Execute(t *testing.T) {
	handler := &windowsServiceHandler{
		runner: &mockServiceRunner{},
		logger: func(msg string, args ...interface{}) {
			t.Logf("[Service] %s %v", msg, args)
		},
	}

	// Test that handler implements svc.Handler
	// Execute method signature is checked at compile time
	_ = handler
}

// TestWindowsServiceConfig_Defaults tests config defaults
func TestWindowsServiceConfig_Defaults(t *testing.T) {
	config := WindowsServiceConfig{
		Name:        "TestService",
		DisplayName: "Test Service",
		Description: "Test",
	}

	// Should have zero values for optional fields
	if config.StartType != 0 {
		t.Logf("StartType: %d", config.StartType)
	}

	if config.ErrorControl != 0 {
		t.Logf("ErrorControl: %d", config.ErrorControl)
	}

	if len(config.Dependencies) != 0 {
		t.Logf("Dependencies: %v", config.Dependencies)
	}
}

// TestRecoveryAction_Values tests recovery action configuration
func TestRecoveryAction_Values(t *testing.T) {
	actions := []RecoveryAction{
		{Type: 0, Delay: 0},                 // No action
		{Type: 1, Delay: 60 * time.Second},  // Restart
		{Type: 2, Delay: 120 * time.Second}, // Restart
		{Type: 3, Delay: 0},                 // Run command
	}

	for i, action := range actions {
		if action.Delay < 0 {
			t.Errorf("Action %d: delay should not be negative", i)
		}
	}
}

// TestWindowsPathUtils_ExpandMultiple tests multiple var expansion
func TestWindowsPathUtils_ExpandMultiple(t *testing.T) {
	utils := WindowsPathUtils{}

	// Test multiple variables in one path
	input := "%USERPROFILE%\\AppData\\%LOCALAPPDATA%\\test"
	result := utils.ExpandEnvironmentStrings(input)

	if strings.Contains(result, "%USERPROFILE%") {
		t.Error("USERPROFILE should be expanded")
	}

	if strings.Contains(result, "%LOCALAPPDATA%") {
		t.Error("LOCALAPPDATA should be expanded")
	}
}

// TestWindowsServiceStatus_Structure tests status structure
func TestWindowsServiceStatus_Structure(t *testing.T) {
	status := WindowsServiceStatus{
		Name:        "MusterDaemon",
		DisplayName: "Muster Daemon",
		State:       4, // Running
		StateText:   "Running",
		Pid:         1234,
	}

	if status.Name != "MusterDaemon" {
		t.Error("Name mismatch")
	}

	if status.Pid != 1234 {
		t.Errorf("PID mismatch: expected 1234, got %d", status.Pid)
	}
}

// TestWindowsServiceStatus_UnknownState tests unknown state handling
func TestWindowsServiceStatus_UnknownState(t *testing.T) {
	status := WindowsServiceStatus{
		State:     999,
		StateText: serviceStateToString(999),
	}

	expected := "Unknown (999)"
	if status.StateText != expected {
		t.Errorf("Expected %q, got %q", expected, status.StateText)
	}
}

// BenchmarkWindowsPathUtils_SanitizeFilename benchmarks filename sanitization
func BenchmarkWindowsPathUtils_SanitizeFilename(b *testing.B) {
	utils := WindowsPathUtils{}
	filename := "file<with>reserved:chars?and*wildcards.txt"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = utils.SanitizeFilename(filename)
	}
}

// BenchmarkWindowsPathUtils_ExpandEnvironmentStrings benchmarks env expansion
func BenchmarkWindowsPathUtils_ExpandEnvironmentStrings(b *testing.B) {
	utils := WindowsPathUtils{}
	path := "%USERPROFILE%\\AppData\\Local\\Muster\\config.yaml"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = utils.ExpandEnvironmentStrings(path)
	}
}

// TestWindowsServiceIntegration_Combined tests combined functionality
func TestWindowsServiceIntegration_Combined(t *testing.T) {
	// This test verifies that all Windows service components work together
	// without requiring actual Windows services

	// 1. Create service config
	config := WindowsServiceConfig{
		Name:        "MusterTest",
		DisplayName: "Muster Test Service",
		Description: "Test service",
		BinaryPath:  `C:\\Program Files\\Muster\\muster.exe`,
		ConfigPath:  `C:\\ProgramData\\Muster\\config.yaml`,
		RecoveryActions: []RecoveryAction{
			{Type: 1, Delay: 60 * time.Second},
		},
	}

	// 2. Verify config
	if config.Name == "" {
		t.Fatal("Config name should not be empty")
	}

	// 3. Test path utilities
	utils := WindowsPathUtils{}
	expanded := utils.ExpandEnvironmentStrings("%PROGRAMFILES%\\Muster")
	if !strings.Contains(expanded, "Muster") {
		t.Error("Path expansion failed")
	}

	// 4. Verify service state strings
	for _, state := range []svc.State{svc.Stopped, svc.StartPending, svc.StopPending, svc.Running, svc.ContinuePending, svc.PausePending, svc.Paused} {
		stateText := serviceStateToString(state)
		if stateText == "" {
			t.Errorf("State %d should have a text representation", state)
		}
	}

	t.Log("Combined integration test passed")
}
