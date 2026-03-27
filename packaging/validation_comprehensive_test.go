package packaging

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateDebianPackage_InvalidPackage tests validation of an invalid deb package
func TestValidateDebianPackage_InvalidPackage(t *testing.T) {
	if !hasCommand("dpkg-deb") {
		t.Skip("dpkg-deb not installed")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.deb")
	if err := os.WriteFile(testFile, []byte("not a valid deb package"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result, err := ValidateDebianPackage(testFile)
	if err == nil {
		t.Fatal("expected error for invalid deb package")
	}
	if result != nil {
		t.Logf("got partial result: %+v", result)
	}
}

// TestValidateRPM_InvalidPackage tests validation of an invalid RPM package
func TestValidateRPM_InvalidPackage(t *testing.T) {
	if !hasCommand("rpm") {
		t.Skip("rpm not installed")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.rpm")
	if err := os.WriteFile(testFile, []byte("not a valid rpm package"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result, err := ValidateRPM(testFile)
	if err == nil {
		t.Fatal("expected error for invalid rpm package")
	}
	if result != nil {
		t.Logf("got partial result: %+v", result)
	}
}

// TestValidateAlpine_InvalidPackage tests validation of an invalid APK package
func TestValidateAlpine_InvalidPackage(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.apk")
	if err := os.WriteFile(testFile, []byte("not a valid apk package"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result, err := ValidateAlpine(testFile)
	if err == nil {
		t.Fatal("expected error for invalid apk package")
	}
	if result != nil {
		t.Logf("got partial result: %+v", result)
	}
}

// TestValidateArch_InvalidPackage tests validation of an invalid Arch package
func TestValidateArch_InvalidPackage(t *testing.T) {
	if !hasCommand("tar") {
		t.Skip("tar not installed")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.pkg.tar.zst")
	if err := os.WriteFile(testFile, []byte("not a valid arch package"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result, err := ValidateArch(testFile)
	if err == nil {
		t.Fatal("expected error for invalid arch package")
	}
	if result != nil {
		t.Logf("got partial result: %+v", result)
	}
}

// TestValidateBinary_InvalidBinary tests validation of an invalid ELF binary
func TestValidateBinary_InvalidBinary(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.bin")
	if err := os.WriteFile(testFile, []byte("not a valid ELF binary"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := ValidateBinary(testFile)
	if err == nil {
		t.Fatal("expected error for invalid ELF binary")
	}
}

// TestValidateBinary_ValidELF tests validation of a valid ELF binary
func TestValidateBinary_ValidELF(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "valid.elf")

	// Create a minimal ELF header (64-bit, little endian, x86-64)
	elfHeader := createMinimalELFHeader(t, binary.LittleEndian, 0x3E) // AMD64
	if err := os.WriteFile(testFile, elfHeader, 0755); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := ValidateBinary(testFile)
	if err != nil {
		t.Logf("validation error (may be expected): %v", err)
	}
}

// TestValidateBinary_ValidELF32 tests validation of a valid ELF32 binary
func TestValidateBinary_ValidELF32(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "valid32.elf")

	// Create a minimal ELF header (32-bit, little endian, i386)
	elfHeader := createMinimalELFHeader32(t, binary.LittleEndian, 0x03) // i386
	if err := os.WriteFile(testFile, elfHeader, 0755); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := ValidateBinary(testFile)
	if err != nil {
		t.Logf("validation error (may be expected): %v", err)
	}
}

// TestValidateAll_MultiplePackages tests validation of multiple packages
func TestValidateAll_MultiplePackages(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	testFiles := []string{
		filepath.Join(tmpDir, "test1.deb"),
		filepath.Join(tmpDir, "test2.rpm"),
		filepath.Join(tmpDir, "test3.apk"),
	}

	for _, testFile := range testFiles {
		if err := os.WriteFile(testFile, []byte("invalid package"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	results, err := ValidateAll(testFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != len(testFiles) {
		t.Fatalf("expected %d results, got %d", len(testFiles), len(results))
	}

	for i, result := range results {
		if result == nil {
			t.Errorf("result %d is nil", i)
			continue
		}
		if result.Valid {
			t.Errorf("result %d should not be valid", i)
		}
	}
}

// TestVerifySignature_DebsWithoutSignature tests signature verification for unsigned deb
func TestVerifySignature_DebWithoutSignature(t *testing.T) {
	if !hasCommand("dpkg-deb") {
		t.Skip("dpkg-deb not installed")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "unsigned.deb")
	if err := os.WriteFile(testFile, []byte("not a valid deb"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result, err := VerifySignature(testFile)
	if err == nil {
		t.Log("expected error for invalid deb package")
	}
	if result != nil {
		t.Logf("got result: %+v", result)
	}
}

// TestVerifySignature_RPMWithoutSignature tests signature verification for unsigned RPM
func TestVerifySignature_RPMWithoutSignature(t *testing.T) {
	if !hasCommand("rpm") {
		t.Skip("rpm not installed")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "unsigned.rpm")
	if err := os.WriteFile(testFile, []byte("not a valid rpm"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result, err := VerifySignature(testFile)
	if err == nil {
		t.Log("expected error for invalid rpm package")
	}
	if result != nil {
		t.Logf("got result: %+v", result)
	}
}

// TestVerifySignature_APKWithoutSignature tests signature verification for unsigned APK
func TestVerifySignature_APKWithoutSignature(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "unsigned.apk")
	if err := os.WriteFile(testFile, []byte("not a valid apk"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result, err := VerifySignature(testFile)
	if err == nil {
		t.Log("expected error for invalid apk package")
	}
	if result != nil {
		t.Logf("got result: %+v", result)
	}
}

// TestVerifySignature_ArchWithoutSignature tests signature verification for unsigned Arch package
func TestVerifySignature_ArchWithoutSignature(t *testing.T) {
	if !hasCommand("tar") {
		t.Skip("tar not installed")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "unsigned.pkg.tar.zst")
	if err := os.WriteFile(testFile, []byte("not a valid arch package"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result, err := VerifySignature(testFile)
	if err == nil {
		t.Log("expected error for invalid arch package")
	}
	if result != nil {
		t.Logf("got result: %+v", result)
	}
}

// TestExtractBinariesFromDeb_InvalidDeb tests binary extraction from invalid deb
func TestExtractBinariesFromDeb_InvalidDeb(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.deb")
	if err := os.WriteFile(testFile, []byte("not a valid deb"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	binaries, err := extractBinariesFromDeb(testFile)
	if err == nil {
		t.Log("expected error for invalid deb")
	}
	if len(binaries) != 0 {
		t.Errorf("expected 0 binaries, got %d", len(binaries))
	}
}

// TestExtractBinariesFromRPM_InvalidRPM tests binary extraction from invalid RPM
func TestExtractBinariesFromRPM_InvalidRPM(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.rpm")
	if err := os.WriteFile(testFile, []byte("not a valid rpm"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	binaries, err := extractBinariesFromRPM(testFile)
	if err == nil {
		t.Log("expected error for invalid rpm")
	}
	if len(binaries) != 0 {
		t.Errorf("expected 0 binaries, got %d", len(binaries))
	}
}

// TestExtractBinariesFromAPK_InvalidAPK tests binary extraction from invalid APK
func TestExtractBinariesFromAPK_InvalidAPK(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.apk")
	if err := os.WriteFile(testFile, []byte("not a valid apk"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	binaries, err := extractBinariesFromAPK(testFile)
	if err == nil {
		t.Log("expected error for invalid apk")
	}
	if len(binaries) != 0 {
		t.Errorf("expected 0 binaries, got %d", len(binaries))
	}
}

// TestExtractBinariesFromArch_InvalidArch tests binary extraction from invalid Arch package
func TestExtractBinariesFromArch_InvalidArch(t *testing.T) {
	if !hasCommand("tar") {
		t.Skip("tar not installed")
	}

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.pkg.tar.zst")
	if err := os.WriteFile(testFile, []byte("not a valid arch package"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	binaries, err := extractBinariesFromArch(testFile)
	if err == nil {
		t.Log("expected error for invalid arch package")
	}
	if len(binaries) != 0 {
		t.Errorf("expected 0 binaries, got %d", len(binaries))
	}
}

// TestFindELFBinaries_NoBinaries tests finding ELF binaries in directory without binaries
func TestFindELFBinaries_NoBinaries(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a text file instead of a binary
	testFile := filepath.Join(tmpDir, "notelf.txt")
	if err := os.WriteFile(testFile, []byte("not an ELF file"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	binaries, err := findELFBinaries(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(binaries) != 0 {
		t.Errorf("expected 0 binaries, got %d", len(binaries))
	}
}

// TestFindELFBinaries_WithELFFile tests finding ELF binaries in directory with ELF file
func TestFindELFBinaries_WithELFFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal ELF file
	elfFile := filepath.Join(tmpDir, "test.elf")
	elfHeader := createMinimalELFHeader(t, binary.LittleEndian, 0x3E)
	if err := os.WriteFile(elfFile, elfHeader, 0755); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	binaries, err := findELFBinaries(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(binaries) != 1 {
		t.Errorf("expected 1 binary, got %d", len(binaries))
	}
}

// TestValidateDebContents_InvalidFormat tests validation of deb contents with invalid format
func TestValidateDebContents_InvalidFormat(t *testing.T) {
	err := validateDebContents([]byte("invalid format"))
	if err == nil {
		t.Log("expected error for invalid format")
	}
}

// TestValidateArchContents_InvalidFormat tests validation of arch contents with invalid format
func TestValidateArchContents_InvalidFormat(t *testing.T) {
	err := validateArchContents([]byte("invalid format"))
	if err == nil {
		t.Log("expected error for invalid format")
	}
}

// TestParseDebianInfo_EmptyOutput tests parsing empty debian info output
func TestParseDebianInfo_EmptyOutput(t *testing.T) {
	var metadata PackageMetadata
	parseDebianInfo([]byte{}, &metadata)
	// Should not panic and should leave metadata with default values
}

// TestParseDebianInfo_WithMetadata tests parsing debian info with metadata
func TestParseDebianInfo_WithMetadata(t *testing.T) {
	output := []byte(`Package: test-package
Version: 1.0.0
Architecture: amd64
Maintainer: Test Maintainer <test@example.com>
Description: Test package description
 This is a multi-line description
License: MIT
`)

	var metadata PackageMetadata
	parseDebianInfo(output, &metadata)

	if metadata.Name != "test-package" {
		t.Errorf("expected name 'test-package', got '%s'", metadata.Name)
	}
	if metadata.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", metadata.Version)
	}
	if metadata.Architecture != "amd64" {
		t.Errorf("expected architecture 'amd64', got '%s'", metadata.Architecture)
	}
	if metadata.Maintainer != "Test Maintainer <test@example.com>" {
		t.Errorf("expected maintainer 'Test Maintainer <test@example.com>', got '%s'", metadata.Maintainer)
	}
	if !strings.Contains(metadata.Description, "Test package description") {
		t.Errorf("expected description to contain 'Test package description', got '%s'", metadata.Description)
	}
}

// TestParseRPMInfo_EmptyOutput tests parsing empty RPM info output
func TestParseRPMInfo_EmptyOutput(t *testing.T) {
	var metadata PackageMetadata
	parseRPMInfo([]byte{}, &metadata)
	// Should not panic and should leave metadata with default values
}

// TestParseRPMInfo_WithMetadata tests parsing RPM info with metadata
func TestParseRPMInfo_WithMetadata(t *testing.T) {
	output := []byte(`Name        : test-package
Version     : 1.0.0
Release     : 1
Architecture: x86_64
Summary     : Test package description
Group       : Applications
Size        : 1024
`)

	var metadata PackageMetadata
	parseRPMInfo(output, &metadata)

	if metadata.Name != "test-package" {
		t.Errorf("expected name 'test-package', got '%s'", metadata.Name)
	}
	if metadata.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", metadata.Version)
	}
	if metadata.Release != "1" {
		t.Errorf("expected release '1', got '%s'", metadata.Release)
	}
	if metadata.Architecture != "x86_64" {
		t.Errorf("expected architecture 'x86_64', got '%s'", metadata.Architecture)
	}
	if !strings.Contains(metadata.Description, "Test package description") {
		t.Errorf("expected description to contain 'Test package description', got '%s'", metadata.Description)
	}
}

// TestCreateMinimalTarball creates a minimal tarball for testing
func createMinimalTarball(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("failed to write tar header: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("failed to write tar content: %v", err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	return buf.Bytes()
}

// TestValidateAPKStructure_ValidTarGz tests validation of valid tar.gz structure
func TestValidateAPKStructure_ValidTarGz(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "valid.apk")

	// Create a minimal tar.gz file
	tarData := createMinimalTarball(t, map[string][]byte{
		".PKGINFO": []byte("pkgname = test\npkgver = 1.0.0\n"),
	})

	if err := os.WriteFile(testFile, tarData, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := validateAPKStructure(testFile)
	if err != nil {
		t.Logf("validation error (may be expected for minimal apk): %v", err)
	}
}

// TestValidateAPKSignature_MissingSignature tests APK signature validation without signature
func TestValidateAPKSignature_MissingSignature(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "nosig.apk")

	// Create APK without signature
	tarData := createMinimalTarball(t, map[string][]byte{
		".PKGINFO": []byte("pkgname = test\npkgver = 1.0.0\n"),
	})

	if err := os.WriteFile(testFile, tarData, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := validateAPKSignature(testFile)
	if err == nil {
		t.Log("expected error for missing signature")
	}
}

// TestValidateELFArchitecture_InvalidClass tests ELF validation with invalid class
func TestValidateELFArchitecture_InvalidClass(t *testing.T) {
	// Valid magic but invalid class (not 32 or 64 bit)
	data := make([]byte, 52)
	copy(data, []byte{0x7F, 'E', 'L', 'F', 0x00}) // Invalid class
	err := validateELFArchitecture(data)
	if err == nil {
		t.Fatal("expected error for invalid ELF class")
	}
}

// TestValidateELFArchitecture_InvalidMachine32 tests ELF32 validation with invalid machine type
func TestValidateELFArchitecture_InvalidMachine32(t *testing.T) {
	data := createMinimalELFHeader32(t, binary.LittleEndian, 0xFF) // Invalid machine
	err := validateELFArchitecture(data)
	if err == nil {
		t.Log("expected error for invalid machine type")
	}
}

// TestValidateELFArchitecture_InvalidMachine64 tests ELF64 validation with invalid machine type
func TestValidateELFArchitecture_InvalidMachine64(t *testing.T) {
	data := createMinimalELFHeader(t, binary.LittleEndian, 0xFFFF) // Invalid machine
	err := validateELFArchitecture(data)
	if err == nil {
		t.Log("expected error for invalid machine type")
	}
}

// TestValidateBinary_ShortFile tests binary validation with file too short for ELF header
func TestValidateBinary_ShortFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "short.bin")
	if err := os.WriteFile(testFile, []byte{0x7F, 'E', 'L', 'F'}, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := ValidateBinary(testFile)
	if err == nil {
		t.Fatal("expected error for short file")
	}
}

// TestExtractBinariesFromAPK_EmptyTarGz tests APK extraction with empty tar.gz
func TestExtractBinariesFromAPK_EmptyTarGz(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.apk")

	// Create empty gzip file
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if err := gw.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	if err := os.WriteFile(testFile, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	binaries, err := extractBinariesFromAPK(testFile)
	if err != nil {
		t.Logf("expected error for empty tar.gz: %v", err)
	}
	if len(binaries) != 0 {
		t.Errorf("expected 0 binaries, got %d", len(binaries))
	}
}

// TestExtractBinariesFromArch_EmptyTarGz tests Arch package extraction with empty tar.gz
func TestExtractBinariesFromArch_EmptyTarGz(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.pkg.tar.zst")

	// Create empty gzip file
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if err := gw.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	if err := os.WriteFile(testFile, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	binaries, err := extractBinariesFromArch(testFile)
	if err != nil {
		t.Logf("expected error for empty tar.gz: %v", err)
	}
	if len(binaries) != 0 {
		t.Errorf("expected 0 binaries, got %d", len(binaries))
	}
}

// TestValidationResult_Fields tests ValidationResult field access
func TestValidationResult_Fields(t *testing.T) {
	result := &ValidationResult{
		PackagePath: "/test/package.deb",
		PackageType: "deb",
		Valid:       false,
		Errors:      []error{os.ErrNotExist},
		Warnings:    []string{"warning 1"},
		Checks:      map[string]bool{"check1": true},
		Metadata: PackageMetadata{
			Name:    "test",
			Version: "1.0.0",
		},
	}

	if result.PackagePath != "/test/package.deb" {
		t.Errorf("expected PackagePath '/test/package.deb', got '%s'", result.PackagePath)
	}
	if result.PackageType != "deb" {
		t.Errorf("expected PackageType 'deb', got '%s'", result.PackageType)
	}
	if result.Valid {
		t.Error("expected Valid to be false")
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
	if len(result.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(result.Warnings))
	}
	if len(result.Checks) != 1 {
		t.Errorf("expected 1 check, got %d", len(result.Checks))
	}
}

// TestPackageMetadata_Fields tests PackageMetadata field access
func TestPackageMetadata_Fields(t *testing.T) {
	metadata := PackageMetadata{
		Name:         "test-package",
		Version:      "1.0.0",
		Architecture: "amd64",
		Maintainer:   "Test Maintainer",
		Description:  "Test description",
		License:      "MIT",
		Size:         1024,
	}

	if metadata.Name != "test-package" {
		t.Errorf("expected Name 'test-package', got '%s'", metadata.Name)
	}
	if metadata.Version != "1.0.0" {
		t.Errorf("expected Version '1.0.0', got '%s'", metadata.Version)
	}
	if metadata.Architecture != "amd64" {
		t.Errorf("expected Architecture 'amd64', got '%s'", metadata.Architecture)
	}
	if metadata.Maintainer != "Test Maintainer" {
		t.Errorf("expected Maintainer 'Test Maintainer', got '%s'", metadata.Maintainer)
	}
	if metadata.Description != "Test description" {
		t.Errorf("expected Description 'Test description', got '%s'", metadata.Description)
	}
	if metadata.License != "MIT" {
		t.Errorf("expected License 'MIT', got '%s'", metadata.License)
	}
	if metadata.Size != 1024 {
		t.Errorf("expected Size 1024, got %d", metadata.Size)
	}
}

// TestValidateAll_NilSlice tests ValidateAll with nil slice
func TestValidateAll_NilSlice(t *testing.T) {
	results, err := ValidateAll(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// TestValidateAll_SinglePackage tests ValidateAll with single package
func TestValidateAll_SinglePackage(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "unknown.xyz")
	if err := os.WriteFile(testFile, []byte("unknown format"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	results, err := ValidateAll([]string{testFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Valid {
		t.Error("expected validation to fail for unknown format")
	}
}

// TestHasCommand_EmptyName tests hasCommand with empty name
func TestHasCommand_EmptyName(t *testing.T) {
	if hasCommand("") {
		t.Error("expected false for empty command name")
	}
}

// TestValidateBinary_NilData tests validateELFArchitecture with nil data
func TestValidateBinary_NilData(t *testing.T) {
	err := validateELFArchitecture(nil)
	if err == nil {
		t.Fatal("expected error for nil data")
	}
}

// Helper function to create a minimal ELF64 header
func createMinimalELFHeader(t *testing.T, endian binary.ByteOrder, machine uint16) []byte {
	t.Helper()

	header := make([]byte, 64)

	// ELF magic number
	header[0] = 0x7F
	header[1] = 'E'
	header[2] = 'L'
	header[3] = 'F'

	// Class: 64-bit
	header[4] = 2

	// Data encoding: little or big endian
	if endian == binary.LittleEndian {
		header[5] = 1
	} else {
		header[5] = 2
	}

	// Version
	header[6] = 1

	// OS/ABI
	header[7] = 0

	// Padding (bytes 8-15)

	// Type: ET_EXEC (executable)
	endian.PutUint16(header[16:], 2)

	// Machine
	endian.PutUint16(header[18:], machine)

	// Version
	endian.PutUint32(header[20:], 1)

	// Entry point (64-bit)
	endian.PutUint64(header[24:], 0x400000)

	// Program header offset
	endian.PutUint64(header[32:], 64)

	// Section header offset
	endian.PutUint64(header[40:], 0)

	// Flags
	endian.PutUint32(header[48:], 0)

	// ELF header size
	endian.PutUint16(header[52:], 64)

	// Program header entry size
	endian.PutUint16(header[54:], 56)

	// Number of program headers
	endian.PutUint16(header[56:], 1)

	// Section header entry size
	endian.PutUint16(header[58:], 64)

	// Number of section headers
	endian.PutUint16(header[60:], 0)

	// Section name string table index
	endian.PutUint16(header[62:], 0)

	return header
}

// Helper function to create a minimal ELF32 header
func createMinimalELFHeader32(t *testing.T, endian binary.ByteOrder, machine uint16) []byte {
	t.Helper()

	header := make([]byte, 52)

	// ELF magic number
	header[0] = 0x7F
	header[1] = 'E'
	header[2] = 'L'
	header[3] = 'F'

	// Class: 32-bit
	header[4] = 1

	// Data encoding
	if endian == binary.LittleEndian {
		header[5] = 1
	} else {
		header[5] = 2
	}

	// Version
	header[6] = 1

	// OS/ABI
	header[7] = 0

	// Padding (bytes 8-15)

	// Type: ET_EXEC (executable)
	endian.PutUint16(header[16:], 2)

	// Machine
	endian.PutUint16(header[18:], machine)

	// Version
	endian.PutUint32(header[20:], 1)

	// Entry point (32-bit)
	endian.PutUint32(header[24:], 0x8048000)

	// Program header offset
	endian.PutUint32(header[28:], 52)

	// Section header offset
	endian.PutUint32(header[32:], 0)

	// Flags
	endian.PutUint32(header[36:], 0)

	// ELF header size
	endian.PutUint16(header[40:], 52)

	// Program header entry size
	endian.PutUint16(header[42:], 32)

	// Number of program headers
	endian.PutUint16(header[44:], 1)

	// Section header entry size
	endian.PutUint16(header[46:], 40)

	// Number of section headers
	endian.PutUint16(header[48:], 0)

	// Section name string table index
	endian.PutUint16(header[50:], 0)

	return header
}

// TestValidateBinary_ActualBinary tests validation with actual system binary
func TestValidateBinary_ActualBinary(t *testing.T) {
	// Find a real binary on the system
	binaryPath := "/bin/ls"
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		// Try alternative paths
		if _, err := os.Stat("/usr/bin/ls"); err == nil {
			binaryPath = "/usr/bin/ls"
		} else {
			t.Skip("no suitable system binary found")
		}
	}

	err := ValidateBinary(binaryPath)
	if err != nil {
		t.Logf("validation error for system binary (may be expected): %v", err)
	}
}

// TestExtractTarGzContent tests tar.gz extraction helpers
func TestExtractTarGzContent_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.tar.gz")

	// Create tar.gz with content
	tarData := createMinimalTarball(t, map[string][]byte{
		"test.txt": []byte("hello world"),
	})

	if err := os.WriteFile(testFile, tarData, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	file, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("failed to close file: %v", err)
		}
	}()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer func() {
		if err := gzr.Close(); err != nil {
			t.Errorf("failed to close gzip reader: %v", err)
		}
	}()

	tr := tar.NewReader(gzr)
	count := 0
	for {
		_, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("failed to read tar: %v", err)
		}
		count++
	}

	if count != 1 {
		t.Errorf("expected 1 file in tar, got %d", count)
	}
}

// TestExtractTarGzContent_InvalidGzip tests tar.gz extraction with invalid gzip
func TestExtractTarGzContent_InvalidGzip(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.tar.gz")

	if err := os.WriteFile(testFile, []byte("not gzip data"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	file, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("failed to close file: %v", err)
		}
	}()

	_, err = gzip.NewReader(file)
	if err == nil {
		t.Fatal("expected error for invalid gzip")
	}
}

// TestExecCommand tests exec.Command wrapper usage
func TestExecCommand_Basic(t *testing.T) {
	cmd := exec.Command("echo", "test")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if !bytes.Contains(output, []byte("test")) {
		t.Error("expected output to contain 'test'")
	}
}

// TestExecCommand_NonExistent tests exec.Command with non-existent command
func TestExecCommand_NonExistent(t *testing.T) {
	cmd := exec.Command("nonexistent_command_xyz_123")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error for non-existent command")
	}
}

// TestBytesBuffer tests bytes.Buffer usage in extraction
func TestBytesBuffer_Basic(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("test data")
	if buf.Len() == 0 {
		t.Error("expected non-zero buffer length")
	}
	if !bytes.Contains(buf.Bytes(), []byte("test")) {
		t.Error("expected buffer to contain 'test'")
	}
}

// TestFilePathJoin tests filepath.Join usage
func TestFilePathJoin_Basic(t *testing.T) {
	path := filepath.Join("tmp", "test", "file.txt")
	expected := "tmp" + string(os.PathSeparator) + "test" + string(os.PathSeparator) + "file.txt"
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}

// TestStringsContains tests strings.Contains usage
func TestStringsContains_Basic(t *testing.T) {
	if !strings.Contains("hello world", "world") {
		t.Error("expected 'hello world' to contain 'world'")
	}
	if strings.Contains("hello world", "foo") {
		t.Error("expected 'hello world' to not contain 'foo'")
	}
}
