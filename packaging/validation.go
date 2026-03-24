// Package packaging implements validation for Debian, RPM, Alpine, and Arch package formats
package packaging

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ValidationResult holds the result of a package validation
type ValidationResult struct {
	PackagePath string
	PackageType string
	Valid       bool
	Errors      []error
	Warnings    []string
	Checks      map[string]bool
	Metadata    PackageMetadata
}

// PackageMetadata contains extracted package metadata
type PackageMetadata struct {
	Name         string
	Version      string
	Release      string
	Architecture string
	Maintainer   string
	Description  string
	License      string
	Size         int64
}

// Validator provides package validation across all formats
type Validator struct {
}

// NewValidator creates a new package validator
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateDebianPackage validates a .deb package using dpkg-deb
func ValidateDebianPackage(debPath string) (*ValidationResult, error) {
	result := &ValidationResult{
		PackagePath: debPath,
		PackageType: "deb",
		Checks:      make(map[string]bool),
	}

	if _, err := os.Stat(debPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("deb package file missing: %s", debPath)
	}

	if !hasCommand("dpkg-deb") {
		return nil, errors.New("dpkg-deb command not found - install dpkg")
	}

	cmd := exec.Command("dpkg-deb", "--info", debPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("dpkg-deb --info failed: %w", err))
		result.Checks["control_info"] = false
	} else {
		result.Checks["control_info"] = true
		parseDebianInfo(output, &result.Metadata)
	}

	cmd = exec.Command("dpkg-deb", "--field", debPath)
	if err := cmd.Run(); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("dpkg-deb --field failed: %w", err))
		result.Checks["control_fields"] = false
	} else {
		result.Checks["control_fields"] = true
	}

	cmd = exec.Command("dpkg-deb", "--contents", debPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("dpkg-deb --contents failed: %w", err))
		result.Checks["package_contents"] = false
	} else {
		result.Checks["package_contents"] = true
		if err := validateDebContents(output); err != nil {
			result.Warnings = append(result.Warnings, err.Error())
		}
	}

	if err := validateDebArchitecture(debPath); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}

	if err := validateDebControlTar(debPath); err != nil {
		result.Errors = append(result.Errors, err)
		result.Checks["control_tar"] = false
	} else {
		result.Checks["control_tar"] = true
	}

	if err := ValidateBinary(debPath); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("binary validation: %v", err))
	} else {
		result.Checks["binary_valid"] = true
	}

	result.Valid = len(result.Errors) == 0
	return result, nil
}

// ValidateRPM validates an RPM package using rpmlint and rpm tools
func ValidateRPM(rpmPath string) (*ValidationResult, error) {
	result := &ValidationResult{
		PackagePath: rpmPath,
		PackageType: "rpm",
		Checks:      make(map[string]bool),
	}

	if _, err := os.Stat(rpmPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("rpm package file missing: %s", rpmPath)
	}

	if !hasCommand("rpm") {
		return nil, errors.New("rpm command not found - install rpm")
	}

	cmd := exec.Command("rpm", "-qip", rpmPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("rpm -qip failed: %w", err))
		result.Checks["package_info"] = false
	} else {
		result.Checks["package_info"] = true
		parseRPMInfo(output, &result.Metadata)
	}

	cmd = exec.Command("rpm", "-qlp", rpmPath)
	if err := cmd.Run(); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("rpm -qlp failed: %w", err))
		result.Checks["package_list"] = false
	} else {
		result.Checks["package_list"] = true
	}

	if hasCommand("rpmlint") {
		cmd = exec.Command("rpmlint", rpmPath)
		output, err = cmd.CombinedOutput()
		if err != nil {
			result.Warnings = append(result.Warnings, parseRpmlintOutput(output))
		}
		result.Checks["rpmlint"] = true
	} else {
		result.Warnings = append(result.Warnings, "rpmlint not installed - skipping lint checks")
	}

	if err := validateRPMHeader(rpmPath); err != nil {
		result.Errors = append(result.Errors, err)
		result.Checks["rpm_header"] = false
	} else {
		result.Checks["rpm_header"] = true
	}

	if err := ValidateBinary(rpmPath); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("binary validation: %v", err))
	} else {
		result.Checks["binary_valid"] = true
	}

	result.Valid = len(result.Errors) == 0
	return result, nil
}

// ValidateAlpine validates an Alpine APK package
func ValidateAlpine(apkPath string) (*ValidationResult, error) {
	result := &ValidationResult{
		PackagePath: apkPath,
		PackageType: "apk",
		Checks:      make(map[string]bool),
	}

	if _, err := os.Stat(apkPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("apk package file missing: %s", apkPath)
	}

	if !hasCommand("apk") {
		return nil, errors.New("apk command not found - install apk-tools")
	}

	cmd := exec.Command("apk", "info", "--script", apkPath)
	if err := cmd.Run(); err != nil {
		result.Warnings = append(result.Warnings, "apk info --script not available")
	} else {
		result.Checks["scripts"] = true
	}

	cmd = exec.Command("apk", "info", "--depends", apkPath)
	if err := cmd.Run(); err != nil {
		result.Warnings = append(result.Warnings, "apk info --depends not available")
	} else {
		result.Checks["dependencies"] = true
	}

	if err := validateAPKStructure(apkPath); err != nil {
		result.Errors = append(result.Errors, err)
		result.Checks["apk_structure"] = false
	} else {
		result.Checks["apk_structure"] = true
	}

	if err := validateAPKSignature(apkPath); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("signature: %v", err))
	} else {
		result.Checks["signature"] = true
	}

	if err := ValidateBinary(apkPath); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("binary validation: %v", err))
	} else {
		result.Checks["binary_valid"] = true
	}

	result.Valid = len(result.Errors) == 0
	return result, nil
}

// ValidateArch validates an Arch Linux package using namcap
func ValidateArch(pkgPath string) (*ValidationResult, error) {
	result := &ValidationResult{
		PackagePath: pkgPath,
		PackageType: "arch",
		Checks:      make(map[string]bool),
	}

	if _, err := os.Stat(pkgPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("arch package file missing: %s", pkgPath)
	}

	if !hasCommand("namcap") {
		return nil, errors.New("namcap command not found - install namcap")
	}

	cmd := exec.Command("namcap", pkgPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 0 || len(output) > 0 {
				result.Warnings = append(result.Warnings, parseNamcapOutput(output))
			} else {
				result.Errors = append(result.Errors, fmt.Errorf("namcap failed: %w", err))
			}
		}
	} else {
		result.Warnings = append(result.Warnings, parseNamcapOutput(output))
	}
	result.Checks["namcap"] = true

	cmd = exec.Command("tar", "-tzf", pkgPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to list package contents: %w", err))
		result.Checks["package_contents"] = false
	} else {
		result.Checks["package_contents"] = true
		if err := validateArchContents(output); err != nil {
			result.Warnings = append(result.Warnings, err.Error())
		}
	}

	if err := validateArchMetadata(pkgPath); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("metadata: %v", err))
	} else {
		result.Checks["metadata"] = true
	}

	if err := ValidateBinary(pkgPath); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("binary validation: %v", err))
	} else {
		result.Checks["binary_valid"] = true
	}

	result.Valid = len(result.Errors) == 0
	return result, nil
}

// VerifySignature verifies the GPG signature of a package
func VerifySignature(packagePath string) (*ValidationResult, error) {
	result := &ValidationResult{
		PackagePath: packagePath,
		Checks:      make(map[string]bool),
	}

	ext := filepath.Ext(packagePath)
	switch ext {
	case ".deb":
		return verifyDebianSignature(packagePath, result)
	case ".rpm":
		return verifyRPMSignature(packagePath, result)
	case ".apk":
		return verifyAPKSignature(packagePath, result)
	case ".pkg.tar.zst", ".pkg.tar.xz", ".pkg.tar.gz":
		return verifyArchSignature(packagePath, result)
	default:
		return nil, fmt.Errorf("unsupported package format: %s", ext)
	}
}

// ValidateBinary validates the ELF headers of binaries within a package
func ValidateBinary(packagePath string) error {
	if _, err := os.Stat(packagePath); os.IsNotExist(err) {
		return fmt.Errorf("package file missing: %s", packagePath)
	}

	var binaries [][]byte
	var err error

	ext := filepath.Ext(packagePath)
	switch ext {
	case ".deb":
		binaries, err = extractBinariesFromDeb(packagePath)
	case ".rpm":
		binaries, err = extractBinariesFromRPM(packagePath)
	case ".apk":
		binaries, err = extractBinariesFromAPK(packagePath)
	case ".pkg.tar.zst", ".pkg.tar.xz", ".pkg.tar.gz":
		binaries, err = extractBinariesFromArch(packagePath)
	default:
		return fmt.Errorf("unsupported package format: %s", ext)
	}

	if err != nil {
		return err
	}

	for _, binData := range binaries {
		if len(binData) < 4 || binData[0] != 0x7F || binData[1] != 'E' || binData[2] != 'L' || binData[3] != 'F' {
			return fmt.Errorf("invalid ELF header found")
		}
		if err := validateELFArchitecture(binData); err != nil {
			return err
		}
	}

	return nil
}

// ValidateAll orchestrates validation across all package formats
func ValidateAll(packagePaths []string) ([]*ValidationResult, error) {
	var results []*ValidationResult

	for _, pkgPath := range packagePaths {
		ext := filepath.Ext(pkgPath)
		var result *ValidationResult
		var err error

		switch ext {
		case ".deb":
			result, err = ValidateDebianPackage(pkgPath)
		case ".rpm":
			result, err = ValidateRPM(pkgPath)
		case ".apk":
			result, err = ValidateAlpine(pkgPath)
		case ".pkg.tar.zst", ".pkg.tar.xz", ".pkg.tar.gz":
			result, err = ValidateArch(pkgPath)
		default:
			results = append(results, &ValidationResult{
				PackagePath: pkgPath,
				Valid:       false,
				Errors:      []error{fmt.Errorf("unknown package format: %s", ext)},
			})
			continue
		}

		if err != nil {
			results = append(results, &ValidationResult{
				PackagePath: pkgPath,
				Valid:       false,
				Errors:      []error{err},
			})
			continue
		}

		results = append(results, result)
	}

	return results, nil
}

// Helper functions for signature verification
func verifyDebianSignature(debPath string, result *ValidationResult) (*ValidationResult, error) {
	if !hasCommand("dpkg-sig") {
		result.Warnings = append(result.Warnings, "dpkg-sig not installed - skipping signature verification")
		result.Valid = true
		return result, nil
	}

	cmd := exec.Command("dpkg-sig", "--verify", debPath)
	err := cmd.Run()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("signature verification failed: %w", err))
		result.Checks["signature"] = false
		result.Valid = false
	} else {
		result.Checks["signature"] = true
		result.Valid = true
	}

	return result, nil
}

func verifyRPMSignature(rpmPath string, result *ValidationResult) (*ValidationResult, error) {
	if !hasCommand("rpm") {
		result.Warnings = append(result.Warnings, "rpm not installed - skipping signature verification")
		result.Valid = true
		return result, nil
	}

	cmd := exec.Command("rpm", "-K", rpmPath)
	output, err := cmd.CombinedOutput()

	if err != nil || !strings.Contains(string(output), "digests signatures OK") {
		result.Errors = append(result.Errors, fmt.Errorf("RPM signature verification failed"))
		result.Checks["signature"] = false
		result.Valid = false
	} else {
		result.Checks["signature"] = true
		result.Valid = true
	}

	return result, nil
}

func verifyAPKSignature(apkPath string, result *ValidationResult) (*ValidationResult, error) {
	if !hasCommand("apk") {
		result.Warnings = append(result.Warnings, "apk not installed - skipping signature verification")
		result.Valid = true
		return result, nil
	}

	cmd := exec.Command("apk", "verify", apkPath)
	if err := cmd.Run(); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("APK signature verification failed: %w", err))
		result.Checks["signature"] = false
		result.Valid = false
	} else {
		result.Checks["signature"] = true
		result.Valid = true
	}

	return result, nil
}

func verifyArchSignature(pkgPath string, result *ValidationResult) (*ValidationResult, error) {
	if !hasCommand("pacman-key") {
		result.Warnings = append(result.Warnings, "pacman-key not installed - skipping signature verification")
		result.Valid = true
		return result, nil
	}

	cmd := exec.Command("pacman-key", "-v", pkgPath)
	if err := cmd.Run(); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("arch signature verification failed: %w", err))
		result.Checks["signature"] = false
		result.Valid = false
	} else {
		result.Checks["signature"] = true
		result.Valid = true
	}

	return result, nil
}

// Extraction helpers
func extractBinariesFromDeb(debPath string) ([][]byte, error) {
	tmpDir, err := os.MkdirTemp("", "deb-extract-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cmd := exec.Command("dpkg-deb", "-x", debPath, tmpDir)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to extract deb: %w", err)
	}

	return findELFBinaries(tmpDir)
}

func extractBinariesFromRPM(rpmPath string) ([][]byte, error) {
	tmpDir, err := os.MkdirTemp("", "rpm-extract-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cmd := exec.Command("rpm2cpio", rpmPath)
	cpioOut, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to convert rpm to cpio: %w", err)
	}

	cpioCmd := exec.Command("cpio", "-idmv", "--quiet")
	cpioCmd.Dir = tmpDir
	cpioCmd.Stdin = bytes.NewReader(cpioOut)
	if err := cpioCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to extract cpio: %w", err)
	}

	return findELFBinaries(tmpDir)
}

func extractBinariesFromAPK(apkPath string) ([][]byte, error) {
	tmpDir, err := os.MkdirTemp("", "apk-extract-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	file, err := os.Open(apkPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read apk gzip: %w", err)
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		target := filepath.Join(tmpDir, header.Name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}

		if header.Typeflag == tar.TypeReg {
			outFile, err := os.Create(target)
			if err != nil {
				return nil, err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				_ = outFile.Close()
				return nil, err
			}
			_ = outFile.Close()
		}
	}

	return findELFBinaries(tmpDir)
}

func extractBinariesFromArch(pkgPath string) ([][]byte, error) {
	tmpDir, err := os.MkdirTemp("", "arch-extract-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cmd := exec.Command("tar", "-xzf", pkgPath, "-C", tmpDir)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to extract arch package: %w", err)
	}

	return findELFBinaries(tmpDir)
}

func findELFBinaries(dir string) ([][]byte, error) {
	var binaries [][]byte

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		if len(data) >= 4 && data[0] == 0x7F && data[1] == 'E' && data[2] == 'L' && data[3] == 'F' {
			binaries = append(binaries, data)
		}

		return nil
	})

	return binaries, err
}

// Validation helpers
func validateELFArchitecture(data []byte) error {
	if len(data) < 19 {
		return fmt.Errorf("ELF header too short")
	}

	class := data[4]
	machine := uint16(data[18]) | uint16(data[19])<<8

	expectedMachine := getExpectedMachine()
	if machine != expectedMachine {
		return fmt.Errorf("ELF architecture mismatch: got %d, expected %d", machine, expectedMachine)
	}

	if class != 2 && class != 1 {
		return fmt.Errorf("invalid ELF class: %d", class)
	}

	return nil
}

func getExpectedMachine() uint16 {
	switch runtime.GOARCH {
	case "amd64":
		return 62
	case "arm64":
		return 183
	case "386":
		return 3
	case "arm":
		return 40
	default:
		return 0
	}
}

func validateDebContents(output []byte) error {
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "/usr/share/doc/") && strings.HasSuffix(strings.TrimSpace(line), "copyright") {
			return nil
		}
	}
	return fmt.Errorf("missing copyright file in /usr/share/doc/")
}

func validateDebArchitecture(debPath string) error {
	cmd := exec.Command("dpkg-deb", "--field", debPath, "Architecture")
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	arch := strings.TrimSpace(string(output))
	expectedArch := getDebArchitecture()

	if arch != expectedArch && arch != "all" {
		return fmt.Errorf("architecture mismatch: package has %s, expected %s", arch, expectedArch)
	}

	return nil
}

func getDebArchitecture() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "386":
		return "i386"
	case "arm":
		return "armhf"
	default:
		return runtime.GOARCH
	}
}

func validateDebControlTar(debPath string) error {
	tmpDir, err := os.MkdirTemp("", "deb-control-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cmd := exec.Command("dpkg-deb", "-e", debPath, tmpDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract control: %w", err)
	}

	requiredFiles := []string{"control", "md5sums"}
	for _, file := range requiredFiles {
		if _, err := os.Stat(filepath.Join(tmpDir, file)); os.IsNotExist(err) {
			return fmt.Errorf("missing control file: %s", file)
		}
	}

	return nil
}

func validateRPMHeader(rpmPath string) error {
	cmd := exec.Command("rpm", "-qp", "--qf", "%{NAME}", rpmPath)
	nameOut, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to read RPM name: %w", err)
	}

	if len(nameOut) == 0 {
		return fmt.Errorf("RPM name is empty")
	}

	cmd = exec.Command("rpm", "-qp", "--qf", "%{VERSION}", rpmPath)
	verOut, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to read RPM version: %w", err)
	}

	if len(verOut) == 0 {
		return fmt.Errorf("RPM version is empty")
	}

	return nil
}

func validateAPKStructure(apkPath string) error {
	file, err := os.Open(apkPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("invalid APK gzip format: %w", err)
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)
	hasDescription := false

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Name == ".PKGINFO" {
			hasDescription = true
		}
	}

	if !hasDescription {
		return fmt.Errorf("missing .PKGINFO in APK")
	}

	return nil
}

func validateAPKSignature(apkPath string) error {
	file, err := os.Open(apkPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return nil
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil
		}

		if header.Name == ".SIGN.RSA" || header.Name == ".SIGN_ECDSA" {
			return nil
		}
	}

	return fmt.Errorf("no signature found in APK")
}

func validateArchContents(output []byte) error {
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, ".PKGINFO") {
			return nil
		}
	}
	return fmt.Errorf("missing .PKGINFO in Arch package")
}

func validateArchMetadata(pkgPath string) error {
	cmd := exec.Command("tar", "-xzf", pkgPath, "-O", ".PKGINFO")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to extract .PKGINFO: %w", err)
	}

	if len(output) == 0 {
		return fmt.Errorf(".PKGINFO is empty")
	}

	return nil
}

// Output parsers
func parseDebianInfo(output []byte, metadata *PackageMetadata) {
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Package:") {
			metadata.Name = strings.TrimSpace(strings.TrimPrefix(line, "Package:"))
		}
		if strings.HasPrefix(line, "Version:") {
			metadata.Version = strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
		}
		if strings.HasPrefix(line, "Architecture:") {
			metadata.Architecture = strings.TrimSpace(strings.TrimPrefix(line, "Architecture:"))
		}
		if strings.HasPrefix(line, "Maintainer:") {
			metadata.Maintainer = strings.TrimSpace(strings.TrimPrefix(line, "Maintainer:"))
		}
		if strings.HasPrefix(line, "Description:") {
			metadata.Description = strings.TrimSpace(strings.TrimPrefix(line, "Description:"))
		}
	}
}

func parseRPMInfo(output []byte, metadata *PackageMetadata) {
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				metadata.Name = strings.TrimSpace(parts[1])
			}
		}
		if strings.HasPrefix(line, "Version") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				metadata.Version = strings.TrimSpace(parts[1])
			}
		}
		if strings.HasPrefix(line, "Release") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				metadata.Release = strings.TrimSpace(parts[1])
			}
		}
		if strings.HasPrefix(line, "Architecture") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				metadata.Architecture = strings.TrimSpace(parts[1])
			}
		}
		if strings.HasPrefix(line, "Summary") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				metadata.Description = strings.TrimSpace(parts[1])
			}
		}
	}
}

func parseRpmlintOutput(output []byte) string {
	lines := strings.Split(string(output), "\n")
	var warnings []string
	for _, line := range lines {
		if strings.Contains(line, "W:") || strings.Contains(line, "E:") {
			warnings = append(warnings, strings.TrimSpace(line))
		}
	}
	return strings.Join(warnings, "; ")
}

func parseNamcapOutput(output []byte) string {
	return strings.TrimSpace(string(output))
}

func parseDebianSigOutput(output []byte) error {
	return errors.New(strings.TrimSpace(string(output)))
}

// Utility functions
func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
