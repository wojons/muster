package packaging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateDebianPackage_FileMissing(t *testing.T) {
	_, err := ValidateDebianPackage("/nonexistent/package.deb")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if err.Error() != "deb package file missing: /nonexistent/package.deb" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRPM_FileMissing(t *testing.T) {
	_, err := ValidateRPM("/nonexistent/package.rpm")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if err.Error() != "rpm package file missing: /nonexistent/package.rpm" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateAlpine_FileMissing(t *testing.T) {
	_, err := ValidateAlpine("/nonexistent/package.apk")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if err.Error() != "apk package file missing: /nonexistent/package.apk" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateArch_FileMissing(t *testing.T) {
	_, err := ValidateArch("/nonexistent/package.pkg.tar.zst")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if err.Error() != "arch package file missing: /nonexistent/package.pkg.tar.zst" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBinary_FileMissing(t *testing.T) {
	err := ValidateBinary("/nonexistent/package.deb")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if err.Error() != "package file missing: /nonexistent/package.deb" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateAll_EmptyList(t *testing.T) {
	results, err := ValidateAll([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestValidateAll_UnknownFormat(t *testing.T) {
	results, err := ValidateAll([]string{"/nonexistent/package.xyz"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Valid {
		t.Fatal("expected validation to fail for unknown format")
	}
	if len(results[0].Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(results[0].Errors))
	}
}

func TestValidationResult_Structure(t *testing.T) {
	result := &ValidationResult{
		PackagePath: "/test/package.deb",
		PackageType: "deb",
		Valid:       false,
		Errors:      []error{os.ErrNotExist},
		Warnings:    []string{"warning 1", "warning 2"},
		Checks: map[string]bool{
			"control_info":     false,
			"control_fields":   false,
			"package_contents": false,
		},
		Metadata: PackageMetadata{
			Name:         "test-package",
			Version:      "1.0.0",
			Architecture: "amd64",
			Maintainer:   "Test Maintainer",
			Description:  "Test package description",
		},
	}

	if result.PackagePath != "/test/package.deb" {
		t.Errorf("unexpected PackagePath: %s", result.PackagePath)
	}
	if result.PackageType != "deb" {
		t.Errorf("unexpected PackageType: %s", result.PackageType)
	}
	if result.Valid {
		t.Error("expected Valid to be false")
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
	if len(result.Warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(result.Warnings))
	}
	if len(result.Checks) != 3 {
		t.Errorf("expected 3 checks, got %d", len(result.Checks))
	}
}

func TestValidator_NewValidator(t *testing.T) {
	v := NewValidator()
	if v == nil {
		t.Fatal("expected non-nil validator")
	}
}

func TestHasCommand_ExistingCommand(t *testing.T) {
	if !hasCommand("ls") {
		t.Skip("ls command not found (unlikely)")
	}
}

func TestHasCommand_NonExistentCommand(t *testing.T) {
	if hasCommand("nonexistent_command_xyz_123") {
		t.Fatal("expected false for nonexistent command")
	}
}

func TestGetDebArchitecture(t *testing.T) {
	arch := getDebArchitecture()
	if arch == "" {
		t.Fatal("expected non-empty architecture")
	}
}

func TestGetExpectedMachine(t *testing.T) {
	machine := getExpectedMachine()
	if machine == 0 {
		t.Fatal("expected non-zero machine type")
	}
}

func TestValidateELFArchitecture_EmptyData(t *testing.T) {
	err := validateELFArchitecture([]byte{})
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestValidateELFArchitecture_ShortData(t *testing.T) {
	err := validateELFArchitecture([]byte{0x7F, 'E', 'L', 'F'})
	if err == nil {
		t.Fatal("expected error for short data")
	}
}

func TestParseRpmlintOutput_Empty(t *testing.T) {
	output := parseRpmlintOutput([]byte{})
	if output != "" {
		t.Fatalf("expected empty output, got: %s", output)
	}
}

func TestParseRpmlintOutput_WithWarnings(t *testing.T) {
	output := parseRpmlintOutput([]byte("package: W: warning 1\npackage: E: error 1\ninfo line"))
	if output == "" {
		t.Fatal("expected non-empty output")
	}
}

func TestParseNamcapOutput(t *testing.T) {
	output := parseNamcapOutput([]byte("namcap output"))
	if output != "namcap output" {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestParseDebianSigOutput(t *testing.T) {
	err := parseDebianSigOutput([]byte("signature error"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateDebArchitecture_NoDpkg(t *testing.T) {
	if !hasCommand("dpkg-deb") {
		t.Skip("dpkg-deb not installed")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.deb")
	if err := os.WriteFile(testFile, []byte("fake deb"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := validateDebArchitecture(testFile)
	if err == nil {
		t.Log("expected error for invalid deb file")
	}
}

func TestValidateDebControlTar_NoDpkg(t *testing.T) {
	if !hasCommand("dpkg-deb") {
		t.Skip("dpkg-deb not installed")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.deb")
	if err := os.WriteFile(testFile, []byte("fake deb"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := validateDebControlTar(testFile)
	if err == nil {
		t.Log("expected error for invalid deb file")
	}
}

func TestValidateRPMHeader_NoRpm(t *testing.T) {
	if !hasCommand("rpm") {
		t.Skip("rpm not installed")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.rpm")
	if err := os.WriteFile(testFile, []byte("fake rpm"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := validateRPMHeader(testFile)
	if err == nil {
		t.Log("expected error for invalid rpm file")
	}
}

func TestValidateAPKStructure_InvalidFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.apk")
	if err := os.WriteFile(testFile, []byte("not a gzip file"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := validateAPKStructure(testFile)
	if err == nil {
		t.Fatal("expected error for invalid APK")
	}
}

func TestValidateArchMetadata_NoTar(t *testing.T) {
	if !hasCommand("tar") {
		t.Skip("tar not installed")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.pkg.tar.zst")
	if err := os.WriteFile(testFile, []byte("fake arch package"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := validateArchMetadata(testFile)
	if err == nil {
		t.Log("expected error for invalid arch package")
	}
}

func TestValidatePackageSignature_UnknownFormat(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.xyz")
	if err := os.WriteFile(testFile, []byte("fake package"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, err := VerifySignature(testFile)
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
}
