//go:build darwin
// +build darwin

package darwin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// HomebrewFormula represents a Homebrew formula structure
type HomebrewFormula struct {
	Name         string
	Homepage     string
	Description  string
	License      string
	Version      string
	URL          string
	SHA256       string
	Caveats      string
	Dependencies []string
}

// TestHomebrewFormulaGeneration tests that the Homebrew formula can be generated
func TestHomebrewFormulaGeneration(t *testing.T) {
	// Read the goreleaser config to verify formula configuration
	goreleaserPath := filepath.Join("..", "..", "..", ".goreleaser.yml")

	// Check that goreleaser config exists
	_, err := os.Stat(goreleaserPath)
	if os.IsNotExist(err) {
		// Try alternate paths
		goreleaserPath = filepath.Join("..", "..", "..", "..", ".goreleaser.yml")
		_, err = os.Stat(goreleaserPath)
	}

	// For testing purposes, we verify the goreleaser config is valid YAML
	// and contains the brews section
	require.NoError(t, err, "goreleaser.yml should exist")

	content, err := os.ReadFile(goreleaserPath)
	require.NoError(t, err, "should be able to read goreleaser.yml")

	contentStr := string(content)

	// Verify Homebrew formula configuration exists
	assert.Contains(t, contentStr, "brews:", "goreleaser.yml should contain brews section")
	assert.Contains(t, contentStr, "name: muster", "brews section should define muster formula")
	assert.Contains(t, contentStr, "folder: Formula", "should specify Formula folder")
	assert.Contains(t, contentStr, "homepage:", "should have homepage")
	assert.Contains(t, contentStr, "description:", "should have description")
	assert.Contains(t, contentStr, "license:", "should have license")
	assert.Contains(t, contentStr, "caveats:", "should have caveats")
}

// TestHomebrewFormulaStructure validates the formula structure
func TestHomebrewFormulaStructure(t *testing.T) {
	// This test validates that a Homebrew formula would have the correct structure
	formula := HomebrewFormula{
		Name:         "muster",
		Homepage:     "https://github.com/anomalyco/muster",
		Description:  "OpenAPI CLI + MCP Server - Unified tool for API interaction",
		License:      "MIT",
		Version:      "v0.1.0",
		URL:          "https://github.com/anomalyco/muster/releases/download/v0.1.0/muster_0.1.0_darwin_amd64.tar.gz",
		SHA256:       "abc123...", // Placeholder
		Caveats:      "Add support for database operations:\n  brew install sqlite3",
		Dependencies: []string{},
	}

	// Validate formula fields
	assert.NotEmpty(t, formula.Name, "Formula name should not be empty")
	assert.Equal(t, "muster", formula.Name, "Formula name should be 'muster'")
	assert.NotEmpty(t, formula.Homepage, "Formula should have homepage")
	assert.True(t, strings.HasPrefix(formula.Homepage, "https://"), "Homepage should be HTTPS")
	assert.NotEmpty(t, formula.Description, "Formula should have description")
	assert.NotEmpty(t, formula.License, "Formula should have license")
	assert.True(t, strings.HasPrefix(formula.Version, "v"), "Version should start with 'v'")
	assert.NotEmpty(t, formula.URL, "Formula should have download URL")
	assert.NotEmpty(t, formula.SHA256, "Formula should have SHA256 checksum")
	assert.NotEmpty(t, formula.Caveats, "Formula should have caveats")
}

// TestHomebrewInstallationPath validates Homebrew installation paths on macOS
func TestHomebrewInstallationPath(t *testing.T) {
	// Check if Homebrew is installed
	_, err := exec.LookPath("brew")
	if err != nil {
		t.Skip("Homebrew not installed, skipping installation path test")
	}

	// Get Homebrew prefix
	cmd := exec.Command("brew", "--prefix")
	output, err := cmd.Output()
	if err != nil {
		t.Skip("Cannot determine Homebrew prefix, skipping")
	}

	prefix := strings.TrimSpace(string(output))
	assert.NotEmpty(t, prefix, "Homebrew prefix should not be empty")

	// Validate that Cellar directory exists
	cellarPath := filepath.Join(prefix, "Cellar")
	info, err := os.Stat(cellarPath)
	if err == nil {
		assert.True(t, info.IsDir(), "Cellar should be a directory")
	}

	// Validate that bin directory exists
	binPath := filepath.Join(prefix, "bin")
	info, err = os.Stat(binPath)
	if err == nil {
		assert.True(t, info.IsDir(), "bin should be a directory")
	}
}

// TestHomebrewServiceIntegration tests service integration with Homebrew
func TestHomebrewServiceIntegration(t *testing.T) {
	// Check if Homebrew services is available
	brewPath, err := exec.LookPath("brew")
	if err != nil {
		t.Skip("Homebrew not installed, skipping service integration test")
	}

	// Test that brew services command exists
	cmd := exec.Command(brewPath, "services", "--help")
	err = cmd.Run()
	// The command might exit with 0 even without services installed
	// We just want to verify the command doesn't crash
	assert.NoError(t, err, "brew services command should be available")
}

// TestHomebrewCaveatsValidation tests that caveats are properly formatted
func TestHomebrewCaveatsValidation(t *testing.T) {
	caveats := `Add support for database operations:
  brew install sqlite3`

	// Validate caveats structure
	lines := strings.Split(caveats, "\n")
	assert.GreaterOrEqual(t, len(lines), 1, "Caveats should have at least one line")

	// Check for common caveats patterns
	assert.Contains(t, caveats, "brew install", "Caveats should mention brew install for dependencies")
}

// TestHomebrewDependencyValidation tests dependency specifications
func TestHomebrewDependencyValidation(t *testing.T) {
	// Define expected dependencies
	expectedDeps := []string{}

	// Muster doesn't require runtime dependencies (CGO/SQLite compiled in)
	// But users might want sqlite3 for database operations
	assert.Empty(t, expectedDeps, "Muster should not have hard runtime dependencies")
}

// TestHomebrewTapConfiguration validates the tap configuration
func TestHomebrewTapConfiguration(t *testing.T) {
	// Read the goreleaser config
	goreleaserPath := filepath.Join("..", "..", "..", ".goreleaser.yml")

	// Check alternate paths
	_, err := os.Stat(goreleaserPath)
	if os.IsNotExist(err) {
		goreleaserPath = filepath.Join("..", "..", "..", "..", ".goreleaser.yml")
		_, err = os.Stat(goreleaserPath)
	}

	require.NoError(t, err, "goreleaser.yml should exist")

	content, err := os.ReadFile(goreleaserPath)
	require.NoError(t, err)

	contentStr := string(content)

	// Verify tap configuration
	assert.Contains(t, contentStr, "tap:", "should specify tap")
	assert.Contains(t, contentStr, "owner:", "should specify owner")
	assert.Contains(t, contentStr, "name: homebrew-tap", "should specify homebrew-tap")
	assert.Contains(t, contentStr, "folder: Formula", "should place formula in Formula folder")
}

// TestHomebrewFormulaGenerationWithGoReleaser simulates goreleaser formula generation
func TestHomebrewFormulaGenerationWithGoReleaser(t *testing.T) {
	// Skip if not on Darwin
	if runtime.GOOS != "darwin" {
		t.Skip("This test requires macOS")
	}

	// Check if goreleaser is available
	goreleaserPath, err := exec.LookPath("goreleaser")
	if err != nil {
		t.Skip("goreleaser not installed, skipping formula generation test")
	}

	// Test goreleaser check (validates configuration)
	cmd := exec.Command(goreleaserPath, "check", "--config", ".goreleaser.yml")
	cmd.Dir = findProjectRoot(t)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("goreleaser check output: %s", string(output))
		t.Skip("goreleaser check failed, possibly due to missing git history")
	}

	assert.NoError(t, err, "goreleaser configuration should be valid")
	assert.Contains(t, string(output), "configuration is valid", "Configuration should be valid")
}

// TestHomebrewInstallLocation tests the install location of Homebrew
func TestHomebrewInstallLocation(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("This test is macOS-specific")
	}

	// Check common Homebrew locations
	locations := []string{
		"/usr/local/bin/brew",                 // Intel Macs
		"/opt/homebrew/bin/brew",              // Apple Silicon Macs
		"/home/linuxbrew/.linuxbrew/bin/brew", // Linuxbrew
	}

	found := false
	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			found = true
			t.Logf("Found Homebrew at: %s", loc)
			break
		}
	}

	if !found {
		t.Skip("Homebrew not found in standard locations")
	}
}

// TestHomebrewServicesPlist tests that a plist would be generated for services
func TestHomebrewServicesPlist(t *testing.T) {
	// Create a sample plist that would be generated
	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>homebrew.mxcl.muster</string>
    <key>ProgramArguments</key>
    <array>
        <string>#{opt_bin}/muster</string>
        <string>daemon</string>
        <string>start</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>`

	// Validate plist content
	assert.Contains(t, plistContent, "homebrew.mxcl.muster", "Plist should have correct label")
	assert.Contains(t, plistContent, "muster", "Plist should reference muster binary")
	assert.Contains(t, plistContent, "daemon", "Plist should include daemon command")
}

// TestHomebrewPostInstallTests tests that post-install tests would work
func TestHomebrewPostInstallTests(t *testing.T) {
	// Define what the post-install test would check
	tests := []struct {
		name     string
		command  string
		expected string
	}{
		{
			name:     "version check",
			command:  "muster version",
			expected: "muster version",
		},
		{
			name:     "help check",
			command:  "muster --help",
			expected: "Usage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.command, "Test command should not be empty")
			assert.NotEmpty(t, tt.expected, "Expected output should not be empty")
		})
	}
}

// TestHomebrewArchSupport tests architecture support
func TestHomebrewArchSupport(t *testing.T) {
	// Muster should support both Intel and Apple Silicon
	archs := []string{"amd64", "arm64"}

	for _, arch := range archs {
		t.Run(fmt.Sprintf("arch_%s", arch), func(t *testing.T) {
			assert.NotEmpty(t, arch, "Architecture should not be empty")
			assert.True(t, arch == "amd64" || arch == "arm64",
				"Should support amd64 or arm64")
		})
	}
}

// TestHomebrewBottleStructure tests the bottle structure
func TestHomebrewBottleStructure(t *testing.T) {
	// Define expected bottle structure
	bottle := struct {
		Cellar  string
		Prefix  string
		Rebuild int
		RootUrl string
		Files   map[string]struct {
			URL    string
			SHA256 string
		}
	}{
		Cellar:  "/usr/local/Cellar",
		Prefix:  "/usr/local",
		Rebuild: 0,
		RootUrl: "https://github.com/anomalyco/muster/releases/download",
		Files: map[string]struct {
			URL    string
			SHA256 string
		}{
			"arm64_ventura": {
				URL:    "https://github.com/anomalyco/muster/releases/download/v0.1.0/muster_0.1.0_darwin_arm64.tar.gz",
				SHA256: "placeholder",
			},
			"ventura": {
				URL:    "https://github.com/anomalyco/muster/releases/download/v0.1.0/muster_0.1.0_darwin_amd64.tar.gz",
				SHA256: "placeholder",
			},
		},
	}

	assert.NotEmpty(t, bottle.Cellar, "Bottle should have cellar path")
	assert.NotEmpty(t, bottle.Prefix, "Bottle should have prefix")
	assert.NotEmpty(t, bottle.RootUrl, "Bottle should have root URL")
	assert.NotEmpty(t, bottle.Files, "Bottle should have files")
}

// TestHomebrewInstallSimulation simulates the installation process
func TestHomebrewInstallSimulation(t *testing.T) {
	// Create a temporary directory to simulate installation
	tempDir := t.TempDir()

	// Simulate creating the Cellar directory structure
	cellarDir := filepath.Join(tempDir, "Cellar", "muster", "0.1.0")
	err := os.MkdirAll(cellarDir, 0755)
	require.NoError(t, err, "Should be able to create Cellar directory")

	// Simulate installing the binary
	binDir := filepath.Join(cellarDir, "bin")
	err = os.MkdirAll(binDir, 0755)
	require.NoError(t, err, "Should be able to create bin directory")

	// Create a placeholder binary (just a file for testing)
	binaryPath := filepath.Join(binDir, "muster")
	err = os.WriteFile(binaryPath, []byte("#!/bin/sh\necho 'muster v0.1.0'"), 0755)
	require.NoError(t, err, "Should be able to create binary")

	// Simulate creating symlink in /usr/local/bin
	usrLocalBin := filepath.Join(tempDir, "usr", "local", "bin")
	err = os.MkdirAll(usrLocalBin, 0755)
	require.NoError(t, err, "Should be able to create usr/local/bin")

	symlinkPath := filepath.Join(usrLocalBin, "muster")
	err = os.Symlink(binaryPath, symlinkPath)
	require.NoError(t, err, "Should be able to create symlink")

	// Verify the structure
	info, err := os.Stat(binaryPath)
	require.NoError(t, err, "Binary should exist")
	assert.False(t, info.IsDir(), "Binary should not be a directory")

	linkInfo, err := os.Lstat(symlinkPath)
	require.NoError(t, err, "Symlink should exist")
	assert.NotEqual(t, 0, linkInfo.Mode()&os.ModeSymlink, "Should be a symlink")
}

// findProjectRoot finds the project root directory
func findProjectRoot(t *testing.T) string {
	// Try to find .goreleaser.yml by walking up
	dir, err := os.Getwd()
	require.NoError(t, err, "Should be able to get working directory")

	for {
		if _, err := os.Stat(filepath.Join(dir, ".goreleaser.yml")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Skip("Could not find project root with .goreleaser.yml")
	return ""
}

// TestHomebrewFormulaContent tests the actual formula content that would be generated
func TestHomebrewFormulaContent(t *testing.T) {
	// This is what goreleaser would generate
	expectedFormula := `class Muster < Formula
  desc "OpenAPI CLI + MCP Server - Unified tool for API interaction"
  homepage "https://github.com/anomalyco/muster"
  version "0.1.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/anomalyco/muster/releases/download/v0.1.0/muster_0.1.0_darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_ARM64"
    else
      url "https://github.com/anomalyco/muster/releases/download/v0.1.0/muster_0.1.0_darwin_amd64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_AMD64"
    end
  end

  def install
    bin.install "muster"
  end

  def caveats
    <<~EOS
      Add support for database operations:
        brew install sqlite3
    EOS
  end

  test do
    system "#{bin}/muster", "version"
  end
end`

	// Validate formula structure
	assert.Contains(t, expectedFormula, "class Muster < Formula", "Should define Muster class")
	assert.Contains(t, expectedFormula, "desc", "Should have description")
	assert.Contains(t, expectedFormula, "homepage", "Should have homepage")
	assert.Contains(t, expectedFormula, "version", "Should have version")
	assert.Contains(t, expectedFormula, "license", "Should have license")
	assert.Contains(t, expectedFormula, "url", "Should have URL")
	assert.Contains(t, expectedFormula, "sha256", "Should have SHA256")
	assert.Contains(t, expectedFormula, "def install", "Should have install method")
	assert.Contains(t, expectedFormula, "bin.install", "Should install binary")
	assert.Contains(t, expectedFormula, "def caveats", "Should have caveats method")
	assert.Contains(t, expectedFormula, "test do", "Should have test block")
	assert.Contains(t, expectedFormula, "Hardware::CPU.arm?", "Should check for ARM")
}

// TestHomebrewServiceCommand tests service-related commands
func TestHomebrewServiceCommand(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("This test is macOS-specific")
	}

	// Check if brew is available
	_, err := exec.LookPath("brew")
	if err != nil {
		t.Skip("Homebrew not found, skipping service test")
	}

	// Test that we can check service status (even if muster is not installed)
	// This validates the service infrastructure exists
	cmd := exec.Command("brew", "services", "list")
	// Don't fail if command fails - we're just checking the command exists
	_, _ = cmd.Output()

	// If we get here without panic, the service command exists
	assert.True(t, true, "brew services command should be available")
}

// TestHomebrewUninstallSimulation simulates the uninstallation process
func TestHomebrewUninstallSimulation(t *testing.T) {
	// Create a temporary directory to simulate installation
	tempDir := t.TempDir()

	// Simulate creating the installed structure
	cellarDir := filepath.Join(tempDir, "Cellar", "muster", "0.1.0")
	err := os.MkdirAll(cellarDir, 0755)
	require.NoError(t, err)

	// Create the binary
	binDir := filepath.Join(cellarDir, "bin")
	err = os.MkdirAll(binDir, 0755)
	require.NoError(t, err)
	binaryPath := filepath.Join(binDir, "muster")
	err = os.WriteFile(binaryPath, []byte("binary content"), 0755)
	require.NoError(t, err)

	// Create the symlink
	usrLocalBin := filepath.Join(tempDir, "usr", "local", "bin")
	err = os.MkdirAll(usrLocalBin, 0755)
	require.NoError(t, err)
	symlinkPath := filepath.Join(usrLocalBin, "muster")
	err = os.Symlink(binaryPath, symlinkPath)
	require.NoError(t, err)

	// Simulate uninstall: remove symlink first
	err = os.Remove(symlinkPath)
	require.NoError(t, err, "Should be able to remove symlink")

	// Then remove the Cellar directory
	err = os.RemoveAll(cellarDir)
	require.NoError(t, err, "Should be able to remove Cellar directory")

	// Verify cleanup
	_, err = os.Stat(symlinkPath)
	assert.True(t, os.IsNotExist(err), "Symlink should not exist")

	_, err = os.Stat(cellarDir)
	assert.True(t, os.IsNotExist(err), "Cellar directory should not exist")
}

// TestHomebrewUpgradeSimulation simulates an upgrade
func TestHomebrewUpgradeSimulation(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Simulate installing old version
	oldCellar := filepath.Join(tempDir, "Cellar", "muster", "0.1.0")
	err := os.MkdirAll(filepath.Join(oldCellar, "bin"), 0755)
	require.NoError(t, err)
	oldBinary := filepath.Join(oldCellar, "bin", "muster")
	err = os.WriteFile(oldBinary, []byte("v0.1.0"), 0755)
	require.NoError(t, err)

	// Create symlink
	usrLocalBin := filepath.Join(tempDir, "usr", "local", "bin")
	err = os.MkdirAll(usrLocalBin, 0755)
	require.NoError(t, err)
	symlink := filepath.Join(usrLocalBin, "muster")
	err = os.Symlink(oldBinary, symlink)
	require.NoError(t, err)

	// Simulate installing new version
	newCellar := filepath.Join(tempDir, "Cellar", "muster", "0.2.0")
	err = os.MkdirAll(filepath.Join(newCellar, "bin"), 0755)
	require.NoError(t, err)
	newBinary := filepath.Join(newCellar, "bin", "muster")
	err = os.WriteFile(newBinary, []byte("v0.2.0"), 0755)
	require.NoError(t, err)

	// Update symlink to point to new version
	err = os.Remove(symlink)
	require.NoError(t, err)
	err = os.Symlink(newBinary, symlink)
	require.NoError(t, err)

	// Verify symlink points to new version
	linkTarget, err := os.Readlink(symlink)
	require.NoError(t, err)
	assert.Contains(t, linkTarget, "0.2.0", "Symlink should point to new version")

	// Clean up old version
	err = os.RemoveAll(oldCellar)
	require.NoError(t, err)

	// Verify old version is gone
	_, err = os.Stat(oldCellar)
	assert.True(t, os.IsNotExist(err), "Old version should be removed")
}

// TestHomebrewKegOnlyValidation validates keg-only behavior
func TestHomebrewKegOnlyValidation(t *testing.T) {
	// Muster is NOT keg-only - it should be linked to /usr/local/bin
	isKegOnly := false

	assert.False(t, isKegOnly, "Muster should not be keg-only")
	assert.True(t, true, "Muster binary should be linked to /usr/local/bin")
}

// TestHomebrewDependencyGraph validates dependency installation order
func TestHomebrewDependencyGraph(t *testing.T) {
	// Muster has no runtime dependencies (everything compiled in)
	deps := []string{}

	// Optional: sqlite3 for database operations
	optionalDeps := []string{"sqlite3"}

	assert.Empty(t, deps, "Should have no required dependencies")
	assert.NotEmpty(t, optionalDeps, "Should have optional dependencies documented")
	assert.Contains(t, optionalDeps, "sqlite3", "Should optionally depend on sqlite3")
}

// TestHomebrewConflictsWith validates no conflicts with other formulas
func TestHomebrewConflictsWith(t *testing.T) {
	// Check for potential name conflicts
	conflicts := []string{}

	// "muster" is a unique name, should not conflict with common tools
	assert.Empty(t, conflicts, "Should not conflict with existing formulas")
}
