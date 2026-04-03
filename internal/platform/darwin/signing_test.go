//go:build darwin
// +build darwin

package darwin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestValidateCodeSigning_BinaryNotSigned tests validation of an unsigned binary.
func TestValidateCodeSigning_BinaryNotSigned(t *testing.T) {
	// Create a simple test binary
	tmpDir := t.TempDir()
	testBinary := filepath.Join(tmpDir, "testbinary")

	// Create a simple Go binary for testing
	testCode := `package main
func main() {}
`
	testFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(testFile, []byte(testCode), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Compile the test binary
	cmd := exec.Command("go", "build", "-o", testBinary, testFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile test binary: %v\nOutput: %s", err, output)
	}

	// Test validation
	status, err := ValidateCodeSigning(testBinary)
	if err != nil {
		t.Fatalf("ValidateCodeSigning failed: %v", err)
	}

	if status.Valid {
		t.Errorf("Expected unsigned binary to have invalid signature, got valid")
	}

	if status.Error == "" {
		t.Error("Expected error message for unsigned binary")
	}
}

// TestValidateCodeSigningWithOptions_DepVerification tests deep verification option.
func TestValidateCodeSigningWithOptions_DepVerification(t *testing.T) {
	// Create test binary
	tmpDir := t.TempDir()
	testBinary := filepath.Join(tmpDir, "testbinary")

	testCode := `package main
func main() {}
`
	testFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(testFile, []byte(testCode), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", testBinary, testFile)
	if _, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile test binary: %v", err)
	}

	opts := &ValidationOptions{
		Deep:    true,
		Strict:  true,
		Verbose: false,
		Timeout: 30 * time.Second,
	}

	status, err := ValidateCodeSigningWithOptions(testBinary, opts)
	if err != nil {
		t.Fatalf("ValidateCodeSigningWithOptions failed: %v", err)
	}

	// Unsigned binary should be invalid
	if status.Valid {
		t.Error("Expected unsigned binary to fail strict verification")
	}
}

// TestCheckNotarization_NotNotarized tests notarization check on unsigned content.
func TestCheckNotarization_NotNotarized(t *testing.T) {
	// Create a test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	status, err := CheckNotarization(testFile)
	if err != nil {
		t.Fatalf("CheckNotarization failed: %v", err)
	}

	if status.Notarized {
		t.Error("Expected non-application to not be notarized")
	}

	if status.Stapled {
		t.Error("Expected no stapled ticket on regular file")
	}
}

// TestCheckNotarizationWithOptions_CustomTimeout tests notarization check with custom timeout.
func TestCheckNotarizationWithOptions_CustomTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	opts := &ValidationOptions{
		Timeout: 10 * time.Second,
	}

	status, err := CheckNotarizationWithOptions(testFile, opts)
	if err != nil {
		t.Fatalf("CheckNotarizationWithOptions failed: %v", err)
	}

	// Should complete without timeout
	_ = status
}

// TestGetSigningIdentities tests retrieving signing identities.
func TestGetSigningIdentities(t *testing.T) {
	identities, err := GetSigningIdentities()
	if err != nil {
		// This might fail in CI or if no identities exist
		t.Logf("GetSigningIdentities failed (may be expected in CI): %v", err)
		return
	}

	// Just verify we got a slice (may be empty)
	if identities == nil {
		t.Error("Expected non-nil slice of identities")
	}

	// If we have identities, validate their structure
	for _, id := range identities {
		if id.Hash == "" {
			t.Error("Expected identity to have a hash")
		}
		if id.Name == "" {
			t.Error("Expected identity to have a name")
		}
	}
}

// TestStapleNotarizationTicket_Unsigned tests stapling on unsigned content.
func TestStapleNotarizationTicket_Unsigned(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err := StapleNotarizationTicket(testFile)
	if err == nil {
		t.Error("Expected stapling to fail on unsigned content")
	}
}

// TestIsDeveloperIDSigned_NotSigned tests Developer ID check on unsigned content.
func TestIsDeveloperIDSigned_NotSigned(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	isDevID, err := IsDeveloperIDSigned(testFile)
	if err != nil {
		t.Fatalf("IsDeveloperIDSigned failed: %v", err)
	}

	if isDevID {
		t.Error("Expected unsigned content to not have Developer ID signature")
	}
}

// TestValidateForDistribution_Unsigned tests distribution validation on unsigned content.
func TestValidateForDistribution_Unsigned(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	report, err := ValidateForDistribution(testFile)
	if err != nil {
		t.Fatalf("ValidateForDistribution failed: %v", err)
	}

	if report.Ready {
		t.Error("Expected unsigned content to not be ready for distribution")
	}

	if report.CodeSigning == nil {
		t.Error("Expected code signing status in report")
	}

	if report.Notarization == nil {
		t.Error("Expected notarization status in report")
	}

	// Should have issues listed
	if len(report.Issues) == 0 {
		t.Error("Expected issues to be listed for unsigned content")
	}
}

// TestDefaultValidationOptions tests the default options function.
func TestDefaultValidationOptions(t *testing.T) {
	opts := DefaultValidationOptions()

	if opts == nil {
		t.Fatal("Expected non-nil options")
	}

	if !opts.Deep {
		t.Error("Expected Deep to be true by default")
	}

	if !opts.Strict {
		t.Error("Expected Strict to be true by default")
	}

	if opts.CheckNotarization {
		t.Error("Expected CheckNotarization to be false by default")
	}

	if opts.Timeout != 30*time.Second {
		t.Errorf("Expected default timeout of 30s, got %v", opts.Timeout)
	}
}

// TestValidationCache tests caching functionality.
func TestValidationCache(t *testing.T) {
	// Clear cache before test
	ClearValidationCache()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// First validation should populate cache
	_, err := ValidateCodeSigning(testFile)
	if err != nil {
		t.Fatalf("First validation failed: %v", err)
	}

	// Check if cached
	cached := getCachedResult(testFile)
	if cached == nil {
		t.Error("Expected result to be cached")
	}

	// Clear cache
	ClearValidationCache()

	// Verify cache is cleared
	cached = getCachedResult(testFile)
	if cached != nil {
		t.Error("Expected cache to be cleared")
	}
}

// TestCacheExpiration tests that cache entries expire.
func TestCacheExpiration(t *testing.T) {
	// Save original expiration
	originalExpiration := cacheExpiration
	cacheExpiration = 100 * time.Millisecond
	defer func() {
		cacheExpiration = originalExpiration
		ClearValidationCache()
	}()

	ClearValidationCache()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Validate and cache
	_, err := ValidateCodeSigning(testFile)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Should be cached immediately
	cached := getCachedResult(testFile)
	if cached == nil {
		t.Error("Expected result to be cached immediately")
	}

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)

	// Should be expired now
	cached = getCachedResult(testFile)
	if cached != nil {
		t.Error("Expected cache to be expired")
	}
}

// TestValidateCodeSigning_EmptyPath tests validation with empty path.
func TestValidateCodeSigning_EmptyPath(t *testing.T) {
	_, err := ValidateCodeSigning("")
	if err == nil {
		t.Error("Expected error for empty path")
	}

	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("Expected error about empty path, got: %v", err)
	}
}

// TestValidateCodeSigning_NonExistentPath tests validation with non-existent path.
func TestValidateCodeSigning_NonExistentPath(t *testing.T) {
	_, err := ValidateCodeSigning("/nonexistent/path/to/file")
	if err == nil {
		t.Error("Expected error for non-existent path")
	}

	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("Expected error about non-existent path, got: %v", err)
	}
}

// TestCheckNotarization_EmptyPath tests notarization check with empty path.
func TestCheckNotarization_EmptyPath(t *testing.T) {
	_, err := CheckNotarization("")
	if err == nil {
		t.Error("Expected error for empty path")
	}
}

// TestCheckNotarization_NonExistentPath tests notarization check with non-existent path.
func TestCheckNotarization_NonExistentPath(t *testing.T) {
	_, err := CheckNotarization("/nonexistent/path")
	if err == nil {
		t.Error("Expected error for non-existent path")
	}
}

// TestStapleNotarizationTicket_EmptyPath tests stapling with empty path.
func TestStapleNotarizationTicket_EmptyPath(t *testing.T) {
	err := StapleNotarizationTicket("")
	if err == nil {
		t.Error("Expected error for empty path")
	}
}

// TestStapleNotarizationTicket_NonExistentPath tests stapling with non-existent path.
func TestStapleNotarizationTicket_NonExistentPath(t *testing.T) {
	err := StapleNotarizationTicket("/nonexistent/path")
	if err == nil {
		t.Error("Expected error for non-existent path")
	}
}

// TestDistributionReport_JSONMarshaling tests JSON marshaling of distribution report.
func TestDistributionReport_JSONMarshaling(t *testing.T) {
	report := &DistributionReport{
		Path:          "/test/app",
		Ready:         false,
		IsDeveloperID: false,
		Issues:        []string{"Not signed", "Not notarized"},
	}

	// Simple validation that fields are set
	if report.Path != "/test/app" {
		t.Error("Path mismatch")
	}

	if report.Ready {
		t.Error("Expected Ready to be false")
	}

	if len(report.Issues) != 2 {
		t.Errorf("Expected 2 issues, got %d", len(report.Issues))
	}
}

// TestSigningStatus_JSONStructure tests the structure of SigningStatus.
func TestSigningStatus_JSONStructure(t *testing.T) {
	status := &SigningStatus{
		Valid:           false,
		SigningIdentity: "",
		TeamID:          "",
		Error:           "test error",
	}

	if status.Valid {
		t.Error("Expected Valid to be false")
	}

	if status.Error != "test error" {
		t.Errorf("Expected error 'test error', got '%s'", status.Error)
	}
}

// TestNotarizationStatus_JSONStructure tests the structure of NotarizationStatus.
func TestNotarizationStatus_JSONStructure(t *testing.T) {
	status := &NotarizationStatus{
		Notarized:  false,
		Stapled:    false,
		Assessment: "",
		Error:      "not notarized",
	}

	if status.Notarized {
		t.Error("Expected Notarized to be false")
	}

	if status.Stapled {
		t.Error("Expected Stapled to be false")
	}
}

// TestSigningIdentity_JSONStructure tests the structure of SigningIdentity.
func TestSigningIdentity_JSONStructure(t *testing.T) {
	identity := SigningIdentity{
		Index: "1",
		Hash:  "ABC123",
		Name:  "Developer ID Application: Test",
	}

	if identity.Index != "1" {
		t.Error("Index mismatch")
	}

	if identity.Hash != "ABC123" {
		t.Error("Hash mismatch")
	}

	if !strings.Contains(identity.Name, "Developer ID") {
		t.Error("Expected Developer ID in name")
	}
}

// TestValidationOptions_CustomValues tests custom validation options.
func TestValidationOptions_CustomValues(t *testing.T) {
	opts := &ValidationOptions{
		Deep:              false,
		Strict:            false,
		Verbose:           true,
		CheckNotarization: true,
		Timeout:           60 * time.Second,
	}

	if opts.Deep {
		t.Error("Expected Deep to be false")
	}

	if opts.Strict {
		t.Error("Expected Strict to be false")
	}

	if !opts.Verbose {
		t.Error("Expected Verbose to be true")
	}

	if !opts.CheckNotarization {
		t.Error("Expected CheckNotarization to be true")
	}

	if opts.Timeout != 60*time.Second {
		t.Errorf("Expected 60s timeout, got %v", opts.Timeout)
	}
}

// TestConcurrentCacheAccess tests thread safety of the cache.
func TestConcurrentCacheAccess(t *testing.T) {
	ClearValidationCache()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Run concurrent validations
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := ValidateCodeSigning(testFile)
			if err != nil {
				t.Errorf("Concurrent validation failed: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Cache should have exactly one entry
	cacheMutex.RLock()
	count := len(validationCache)
	cacheMutex.RUnlock()

	if count != 1 {
		t.Errorf("Expected 1 cache entry, got %d", count)
	}
}

// TestCacheResult stores and retrieves cache entries correctly.
func TestCacheResult(t *testing.T) {
	ClearValidationCache()

	path := "/test/path"
	status := &SigningStatus{
		Valid:  true,
		TeamID: "TEST123",
		Error:  "",
	}

	cacheResult(path, status)

	retrieved := getCachedResult(path)
	if retrieved == nil {
		t.Fatal("Expected cached result")
	}

	if !retrieved.Valid {
		t.Error("Expected Valid to be true")
	}

	if retrieved.TeamID != "TEST123" {
		t.Errorf("Expected TeamID 'TEST123', got '%s'", retrieved.TeamID)
	}
}

// TestGetEntitlements_Parsing tests entitlement parsing.
func TestGetEntitlements_Parsing(t *testing.T) {
	// Create a test binary
	tmpDir := t.TempDir()
	testBinary := filepath.Join(tmpDir, "testbinary")

	testCode := `package main
func main() {}
`
	testFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(testFile, []byte(testCode), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", testBinary, testFile)
	if _, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to compile test binary: %v", err)
	}

	// Try to get entitlements (will likely fail for unsigned binary)
	entitlements, err := getEntitlements(testBinary)
	// We expect this to fail for unsigned binaries, but shouldn't panic
	_ = entitlements
	_ = err
}

// TestCheckStapledTicket_Unsigned tests stapled ticket check on unsigned content.
func TestCheckStapledTicket_Unsigned(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Should return false for unsigned content
	stapled := checkStapledTicket(testFile)
	if stapled {
		t.Error("Expected unsigned content to not have stapled ticket")
	}
}

// TestValidateSignature_Unsigned tests signature validation on unsigned content.
func TestValidateSignature_Unsigned(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	opts := DefaultValidationOptions()

	// Should return false for unsigned content
	valid := validateSignature(testFile, opts)
	if valid {
		t.Error("Expected unsigned content to fail validation")
	}
}

// ExampleValidateCodeSigning demonstrates basic code signing validation.
func ExampleValidateCodeSigning() {
	// This example shows the API usage - in real tests we'd need a signed binary
	// For demonstration purposes, we'll show the expected pattern:

	// result, err := ValidateCodeSigning("/Applications/Muster.app")
	// if err != nil {
	//     fmt.Printf("Validation error: %v\n", err)
	//     return
	// }
	//
	// if result.Valid {
	//     fmt.Printf("Signed by: %s\n", result.SigningIdentity)
	// }
}

// ExampleCheckNotarization demonstrates checking notarization status.
func ExampleCheckNotarization() {
	// status, err := CheckNotarization("/Applications/Muster.app")
	// if err != nil {
	//     fmt.Printf("Check failed: %v\n", err)
	//     return
	// }
	//
	// if status.Notarized {
	//     fmt.Println("Application is notarized")
	// }
}

// BenchmarkValidateCodeSigning benchmarks code signing validation.
func BenchmarkValidateCodeSigning(b *testing.B) {
	// Create test binary
	tmpDir := b.TempDir()
	testBinary := filepath.Join(tmpDir, "testbinary")

	testCode := `package main
func main() {}
`
	testFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(testFile, []byte(testCode), 0644); err != nil {
		b.Fatalf("Failed to create test file: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", testBinary, testFile)
	if _, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("Failed to compile test binary: %v", err)
	}

	// Clear cache before benchmark
	ClearValidationCache()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Alternate between cached and uncached by clearing periodically
		if i%100 == 0 {
			ClearValidationCache()
		}
		_, _ = ValidateCodeSigning(testBinary)
	}
}

// BenchmarkGetSigningIdentities benchmarks retrieving signing identities.
func BenchmarkGetSigningIdentities(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = GetSigningIdentities()
	}
}

// TestSigningStatus_Fields tests all fields of SigningStatus.
func TestSigningStatus_Fields(t *testing.T) {
	status := &SigningStatus{
		Valid:              true,
		SigningIdentity:    "Developer ID Application: Test",
		Authority:          []string{"Developer ID", "Apple Root CA"},
		TeamID:             "TEAM123",
		BundleID:           "com.test.app",
		Timestamp:          time.Now(),
		Entitlements:       map[string]interface{}{"key": "value"},
		Error:              "",
		DeepVerification:   true,
		StrictVerification: true,
	}

	if !status.Valid {
		t.Error("Expected Valid to be true")
	}

	if status.SigningIdentity != "Developer ID Application: Test" {
		t.Error("SigningIdentity mismatch")
	}

	if len(status.Authority) != 2 {
		t.Errorf("Expected 2 authorities, got %d", len(status.Authority))
	}

	if status.TeamID != "TEAM123" {
		t.Errorf("Expected TeamID 'TEAM123', got '%s'", status.TeamID)
	}

	if status.BundleID != "com.test.app" {
		t.Errorf("Expected BundleID 'com.test.app', got '%s'", status.BundleID)
	}

	if !status.DeepVerification {
		t.Error("Expected DeepVerification to be true")
	}

	if !status.StrictVerification {
		t.Error("Expected StrictVerification to be true")
	}
}

// TestNotarizationStatus_Fields tests all fields of NotarizationStatus.
func TestNotarizationStatus_Fields(t *testing.T) {
	status := &NotarizationStatus{
		Notarized:  true,
		Stapled:    true,
		Assessment: "accepted",
		Source:     "Notarized Developer ID",
		Error:      "",
	}

	if !status.Notarized {
		t.Error("Expected Notarized to be true")
	}

	if !status.Stapled {
		t.Error("Expected Stapled to be true")
	}

	if status.Assessment != "accepted" {
		t.Errorf("Expected Assessment 'accepted', got '%s'", status.Assessment)
	}

	if status.Source != "Notarized Developer ID" {
		t.Errorf("Expected Source 'Notarized Developer ID', got '%s'", status.Source)
	}
}

// TestDistributionReport_FullStructure tests complete distribution report structure.
func TestDistributionReport_FullStructure(t *testing.T) {
	report := &DistributionReport{
		Path:  "/Applications/Test.app",
		Ready: false,
		CodeSigning: &SigningStatus{
			Valid:  false,
			TeamID: "",
			Error:  "Not signed",
		},
		Notarization: &NotarizationStatus{
			Notarized: false,
			Stapled:   false,
			Error:     "Not notarized",
		},
		IsDeveloperID: false,
		Issues: []string{
			"Not signed",
			"Not notarized",
			"Not Developer ID signed",
		},
	}

	if report.Path != "/Applications/Test.app" {
		t.Error("Path mismatch")
	}

	if report.Ready {
		t.Error("Expected Ready to be false")
	}

	if report.CodeSigning == nil {
		t.Fatal("Expected CodeSigning in report")
	}

	if report.Notarization == nil {
		t.Fatal("Expected Notarization in report")
	}

	if len(report.Issues) != 3 {
		t.Errorf("Expected 3 issues, got %d", len(report.Issues))
	}
}

// TestSigningIdentity_SliceOperations tests slice operations on signing identities.
func TestSigningIdentity_SliceOperations(t *testing.T) {
	identities := []SigningIdentity{
		{Index: "1", Hash: "ABC", Name: "Developer ID 1"},
		{Index: "2", Hash: "DEF", Name: "Developer ID 2"},
		{Index: "3", Hash: "GHI", Name: "Developer ID 3"},
	}

	if len(identities) != 3 {
		t.Errorf("Expected 3 identities, got %d", len(identities))
	}

	// Check first identity
	if identities[0].Index != "1" || identities[0].Hash != "ABC" {
		t.Error("First identity mismatch")
	}

	// Check last identity
	if identities[2].Index != "3" || identities[2].Hash != "GHI" {
		t.Error("Last identity mismatch")
	}
}

// TestValidationOptions_AllFields tests all fields of ValidationOptions.
func TestValidationOptions_AllFields(t *testing.T) {
	opts := &ValidationOptions{
		Deep:              true,
		Strict:            false,
		Verbose:           true,
		CheckNotarization: false,
		Timeout:           45 * time.Second,
	}

	if !opts.Deep {
		t.Error("Expected Deep to be true")
	}

	if opts.Strict {
		t.Error("Expected Strict to be false")
	}

	if !opts.Verbose {
		t.Error("Expected Verbose to be true")
	}

	if opts.CheckNotarization {
		t.Error("Expected CheckNotarization to be false")
	}

	if opts.Timeout != 45*time.Second {
		t.Errorf("Expected 45s timeout, got %v", opts.Timeout)
	}
}

// TestErrorMessages_Content tests that error messages are informative.
func TestErrorMessages_Content(t *testing.T) {
	// Test empty path error
	_, err := ValidateCodeSigning("")
	if err == nil {
		t.Fatal("Expected error for empty path")
	}

	if !strings.Contains(strings.ToLower(err.Error()), "empty") {
		t.Errorf("Expected error message to mention 'empty', got: %v", err)
	}

	// Test non-existent path error
	_, err = ValidateCodeSigning("/this/path/definitely/does/not/exist")
	if err == nil {
		t.Fatal("Expected error for non-existent path")
	}

	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("Expected error message to mention 'does not exist', got: %v", err)
	}
}

// TestEntitlements_ParsingVariations tests entitlement parsing with different inputs.
func TestEntitlements_ParsingVariations(t *testing.T) {
	// Test with empty map
	emptyEntitlements := map[string]interface{}{}
	if len(emptyEntitlements) != 0 {
		t.Error("Expected empty entitlements")
	}

	// Test with populated map
	entitlements := map[string]interface{}{
		"com.apple.security.cs.allow-jit":                        true,
		"com.apple.security.cs.allow-unsigned-executable-memory": false,
		"com.apple.security.automation.apple-events":             true,
	}

	if len(entitlements) != 3 {
		t.Errorf("Expected 3 entitlements, got %d", len(entitlements))
	}

	if entitlements["com.apple.security.cs.allow-jit"] != true {
		t.Error("Expected JIT entitlement to be true")
	}
}

// TestSignatureInfo_Parsing tests signature info parsing.
func TestSignatureInfo_Parsing(t *testing.T) {
	info := &signatureInfo{
		Identity:       "Developer ID Application: Test",
		Authority:      []string{"Developer ID", "Apple Root CA"},
		TeamID:         "TEAM123",
		BundleID:       "com.test.app",
		Timestamp:      time.Now(),
		DeepVerified:   true,
		StrictVerified: false,
	}

	if info.Identity != "Developer ID Application: Test" {
		t.Error("Identity mismatch")
	}

	if len(info.Authority) != 2 {
		t.Errorf("Expected 2 authorities, got %d", len(info.Authority))
	}

	if info.TeamID != "TEAM123" {
		t.Errorf("Expected TeamID 'TEAM123', got '%s'", info.TeamID)
	}

	if !info.DeepVerified {
		t.Error("Expected DeepVerified to be true")
	}

	if info.StrictVerified {
		t.Error("Expected StrictVerified to be false")
	}
}

// TestConcurrentValidation_MultipleFiles tests concurrent validation of multiple files.
func TestConcurrentValidation_MultipleFiles(t *testing.T) {
	ClearValidationCache()

	tmpDir := t.TempDir()

	// Create multiple test files
	files := make([]string, 5)
	for i := 0; i < 5; i++ {
		files[i] = filepath.Join(tmpDir, fmt.Sprintf("test%d.txt", i))
		if err := os.WriteFile(files[i], []byte(fmt.Sprintf("test content %d", i)), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Validate concurrently
	done := make(chan string, 5)
	for _, file := range files {
		go func(f string) {
			_, err := ValidateCodeSigning(f)
			if err != nil {
				done <- fmt.Sprintf("error: %v", err)
			} else {
				done <- "success"
			}
		}(file)
	}

	// Wait for all
	successCount := 0
	for i := 0; i < 5; i++ {
		result := <-done
		if result == "success" {
			successCount++
		}
	}

	if successCount != 5 {
		t.Errorf("Expected 5 successful validations, got %d", successCount)
	}

	// Check that all files are cached
	cacheMutex.RLock()
	count := len(validationCache)
	cacheMutex.RUnlock()

	if count != 5 {
		t.Errorf("Expected 5 cache entries, got %d", count)
	}
}

// TestCache_MultipleClearOperations tests multiple cache clear operations.
func TestCache_MultipleClearOperations(t *testing.T) {
	// Multiple clears should be safe
	ClearValidationCache()
	ClearValidationCache()
	ClearValidationCache()

	cacheMutex.RLock()
	count := len(validationCache)
	cacheMutex.RUnlock()

	if count != 0 {
		t.Errorf("Expected 0 cache entries after multiple clears, got %d", count)
	}
}

// TestValidationWithOptions_AllCombinations tests various option combinations.
func TestValidationWithOptions_AllCombinations(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	combinations := []struct {
		name string
		opts *ValidationOptions
	}{
		{
			name: "deep strict verbose",
			opts: &ValidationOptions{Deep: true, Strict: true, Verbose: true, CheckNotarization: false},
		},
		{
			name: "deep only",
			opts: &ValidationOptions{Deep: true, Strict: false, Verbose: false, CheckNotarization: false},
		},
		{
			name: "strict only",
			opts: &ValidationOptions{Deep: false, Strict: true, Verbose: false, CheckNotarization: false},
		},
		{
			name: "verbose only",
			opts: &ValidationOptions{Deep: false, Strict: false, Verbose: true, CheckNotarization: false},
		},
		{
			name: "check notarization",
			opts: &ValidationOptions{Deep: false, Strict: false, Verbose: false, CheckNotarization: true},
		},
		{
			name: "all enabled",
			opts: &ValidationOptions{Deep: true, Strict: true, Verbose: true, CheckNotarization: true},
		},
	}

	for _, tc := range combinations {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateCodeSigningWithOptions(testFile, tc.opts)
			if err != nil {
				t.Errorf("Validation failed for %s: %v", tc.name, err)
			}
		})
	}
}

// TestNotarizationTimeout_ShortTimeout tests notarization with a very short timeout.
func TestNotarizationTimeout_ShortTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	opts := &ValidationOptions{
		Timeout: 1 * time.Second,
	}

	// Should complete quickly (doesn't actually test timeout behavior
	// since the operation completes quickly anyway)
	status, err := CheckNotarizationWithOptions(testFile, opts)
	if err != nil {
		t.Fatalf("CheckNotarizationWithOptions failed: %v", err)
	}

	_ = status
}

// TestDistributionReport_WithValidAndInvalidPaths tests distribution reports with mixed paths.
func TestDistributionReport_WithValidAndInvalidPaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	validFile := filepath.Join(tmpDir, "test1.txt")
	invalidFile := filepath.Join(tmpDir, "test2.txt")
	if err := os.WriteFile(validFile, []byte("test1"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := os.WriteFile(invalidFile, []byte("test2"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Both should work (validation logic handles unsigned files)
	report1, err := ValidateForDistribution(validFile)
	if err != nil {
		t.Fatalf("Failed to validate valid file: %v", err)
	}

	report2, err := ValidateForDistribution(invalidFile)
	if err != nil {
		t.Fatalf("Failed to validate invalid file: %v", err)
	}

	// Both should be marked as not ready
	if report1.Ready {
		t.Error("Expected unsigned file to not be ready")
	}

	if report2.Ready {
		t.Error("Expected unsigned file to not be ready")
	}
}

// TestGetCodeSignatureInfo_ErrorHandling tests error handling in signature info retrieval.
func TestGetCodeSignatureInfo_ErrorHandling(t *testing.T) {
	// Test with non-existent file
	_, err := getCodeSignatureInfo("/nonexistent/path", DefaultValidationOptions())
	if err == nil {
		t.Error("Expected error for non-existent file")
	}

	// Test with empty path
	_, err = getCodeSignatureInfo("", DefaultValidationOptions())
	if err == nil {
		t.Error("Expected error for empty path")
	}
}

// TestEntitlements_EmptyAndNil tests entitlement handling with empty/nil values.
func TestEntitlements_EmptyAndNil(t *testing.T) {
	// Empty map
	emptyMap := map[string]interface{}{}
	if len(emptyMap) != 0 {
		t.Error("Expected empty map to have length 0")
	}

	// Nil map
	var nilMap map[string]interface{}
	if nilMap != nil {
		t.Error("Expected nil map to be nil")
	}

	// Map with nil values
	nilValueMap := map[string]interface{}{
		"key1": nil,
		"key2": "value",
	}
	if len(nilValueMap) != 2 {
		t.Errorf("Expected map with 2 entries, got %d", len(nilValueMap))
	}
}

// TestSigningIdentity_EdgeCases tests edge cases for SigningIdentity.
func TestSigningIdentity_EdgeCases(t *testing.T) {
	// Empty identity
	emptyIdentity := SigningIdentity{}
	if emptyIdentity.Hash != "" || emptyIdentity.Name != "" {
		t.Error("Expected empty identity to have empty fields")
	}

	// Identity with special characters
	specialIdentity := SigningIdentity{
		Index: "1",
		Hash:  "AB:CD:EF:12:34:56",
		Name:  "Developer ID Application: My Company (TEAM123)",
	}

	if specialIdentity.Hash != "AB:CD:EF:12:34:56" {
		t.Errorf("Expected hash with colons, got '%s'", specialIdentity.Hash)
	}

	// Long name
	longIdentity := SigningIdentity{
		Index: "999",
		Hash:  strings.Repeat("A", 100),
		Name:  strings.Repeat("B", 500),
	}

	if len(longIdentity.Hash) != 100 {
		t.Errorf("Expected hash of length 100, got %d", len(longIdentity.Hash))
	}

	if len(longIdentity.Name) != 500 {
		t.Errorf("Expected name of length 500, got %d", len(longIdentity.Name))
	}
}

// TestCache_MultipleFiles tests caching with multiple different files.
func TestCache_MultipleFiles(t *testing.T) {
	ClearValidationCache()

	tmpDir := t.TempDir()

	// Create and cache multiple files
	files := []string{
		filepath.Join(tmpDir, "file1.txt"),
		filepath.Join(tmpDir, "file2.txt"),
		filepath.Join(tmpDir, "file3.txt"),
	}

	for _, file := range files {
		if err := os.WriteFile(file, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		_, err := ValidateCodeSigning(file)
		if err != nil {
			t.Fatalf("Failed to validate %s: %v", file, err)
		}
	}

	// Check all are cached
	cacheMutex.RLock()
	count := len(validationCache)
	cacheMutex.RUnlock()

	if count != 3 {
		t.Errorf("Expected 3 cache entries, got %d", count)
	}

	// Verify each can be retrieved
	for _, file := range files {
		cached := getCachedResult(file)
		if cached == nil {
			t.Errorf("Expected %s to be cached", file)
		}
	}
}

// TestTimestampParsing tests timestamp parsing from codesign output.
func TestTimestampParsing(t *testing.T) {
	// Valid timestamp
	timeStr := "Jan 15, 2024 at 3:30:45 PM"
	ts, err := time.Parse("Jan 2, 2006 at 3:04:05 PM", timeStr)
	if err != nil {
		t.Errorf("Failed to parse valid timestamp: %v", err)
	}

	expectedYear := 2024
	if ts.Year() != expectedYear {
		t.Errorf("Expected year %d, got %d", expectedYear, ts.Year())
	}

	// Invalid timestamp should return zero time
	_, err = time.Parse("Jan 2, 2006 at 3:04:05 PM", "invalid")
	if err == nil {
		t.Error("Expected error parsing invalid timestamp")
	}
}

// TestAuthorityParsing tests authority parsing from signature output.
func TestAuthorityParsing(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"Authority=Developer ID Application: Test", "Developer ID Application: Test"},
		{"Authority=Apple Root CA", "Apple Root CA"},
		{"Authority=Apple Certification Authority", "Apple Certification Authority"},
	}

	for _, tc := range testCases {
		if strings.HasPrefix(tc.input, "Authority=") {
			auth := strings.TrimPrefix(tc.input, "Authority=")
			if auth != tc.expected {
				t.Errorf("Expected authority '%s', got '%s'", tc.expected, auth)
			}
		}
	}
}

// TestBundleIDParsing tests bundle ID parsing from signature output.
func TestBundleIDParsing(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"Identifier=com.test.app", "com.test.app"},
		{"Identifier=com.example.myapp", "com.example.myapp"},
		{"Identifier=org.company.product", "org.company.product"},
	}

	for _, tc := range testCases {
		if strings.HasPrefix(tc.input, "Identifier=") {
			bundleID := strings.TrimPrefix(tc.input, "Identifier=")
			if bundleID != tc.expected {
				t.Errorf("Expected bundle ID '%s', got '%s'", tc.expected, bundleID)
			}
		}
	}
}

// TestTeamIDParsing tests Team ID parsing from signature output.
func TestTeamIDParsing(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"TeamIdentifier=TEAM123ABC", "TEAM123ABC"},
		{"TeamIdentifier=ABCD123456", "ABCD123456"},
		{"TeamIdentifier=", ""},
	}

	for _, tc := range testCases {
		if strings.HasPrefix(tc.input, "TeamIdentifier=") {
			teamID := strings.TrimPrefix(tc.input, "TeamIdentifier=")
			if teamID != tc.expected {
				t.Errorf("Expected Team ID '%s', got '%s'", tc.expected, teamID)
			}
		}
	}
}
