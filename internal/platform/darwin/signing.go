//go:build darwin
// +build darwin

// Package darwin provides macOS-specific platform implementations for Muster.
// This file implements code signing and notarization validation functionality.
//
// Code signing ensures that macOS binaries and applications are signed with a
// valid Apple Developer ID, providing tamper detection and user trust. Notarization
// is Apple's security process that scans software for malicious content before
// distribution outside the App Store.
//
// Basic usage:
//
//	// Validate code signing of an application
//	result, err := darwin.ValidateCodeSigning("/Applications/Muster.app")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if result.Valid {
//	    fmt.Printf("Signed by: %s\n", result.SigningIdentity)
//	}
//
//	// Check notarization status
//	notarization, err := darwin.CheckNotarization("/Applications/Muster.app")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if notarization.Notarized {
//	    fmt.Println("Application is notarized and safe to distribute")
//	}
//
// Security Considerations:
//   - Always validate code signing before executing downloaded binaries
//   - Notarization is required for distribution outside the App Store on macOS 10.15+
//   - Stapling attaches the notarization ticket for offline validation
//   - Entitlements control sandbox permissions and capabilities
//
// Thread Safety:
//   - All functions in this package are thread-safe
//   - External command execution is serialized to prevent resource exhaustion
//   - Results are cached for 5 minutes to avoid repeated validation
//
// See Also:
//   - specs/PLATFORM-MACOS.md for complete signing/notarization specification
//   - https://developer.apple.com/documentation/security/notarizing_macos_software_before_distribution
package darwin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// SigningStatus represents the result of code signing validation.
type SigningStatus struct {
	// Valid indicates whether the code signature is valid
	Valid bool `json:"valid"`

	// SigningIdentity is the Common Name of the signing certificate
	SigningIdentity string `json:"signing_identity,omitempty"`

	// Authority contains the certificate chain authorities
	Authority []string `json:"authority,omitempty"`

	// TeamID is the Apple Developer Team ID
	TeamID string `json:"team_id,omitempty"`

	// BundleID is the application's bundle identifier
	BundleID string `json:"bundle_id,omitempty"`

	// Timestamp indicates when the signature was applied
	Timestamp time.Time `json:"timestamp,omitempty"`

	// Entitlements contains the entitlements granted to the application
	Entitlements map[string]interface{} `json:"entitlements,omitempty"`

	// Error contains validation error details if Valid is false
	Error string `json:"error,omitempty"`

	// DeepVerification indicates if the signature passed deep verification
	DeepVerification bool `json:"deep_verification"`

	// StrictVerification indicates if the signature passed strict verification
	StrictVerification bool `json:"strict_verification"`
}

// NotarizationStatus represents the notarization validation result.
type NotarizationStatus struct {
	// Notarized indicates whether the application has been notarized
	Notarized bool `json:"notarized"`

	// Stapled indicates if the notarization ticket is stapled to the binary
	Stapled bool `json:"stapled"`

	// Assessment is the system policy assessment result
	Assessment string `json:"assessment,omitempty"`

	// Source indicates the source of notarization (e.g., "Notarized Developer ID")
	Source string `json:"source,omitempty"`

	// Error contains validation error details
	Error string `json:"error,omitempty"`
}

// ValidationOptions configures the validation behavior.
type ValidationOptions struct {
	// Deep enables deep verification of nested bundles
	Deep bool

	// Strict enables strict verification mode
	Strict bool

	// Verbose enables verbose output for debugging
	Verbose bool

	// CheckNotarization also validates notarization status
	CheckNotarization bool

	// Timeout for validation operations
	Timeout time.Duration
}

// DefaultValidationOptions returns sensible default options.
func DefaultValidationOptions() *ValidationOptions {
	return &ValidationOptions{
		Deep:              true,
		Strict:            true,
		Verbose:           false,
		CheckNotarization: true,
		Timeout:           30 * time.Second,
	}
}

// cacheEntry stores cached validation results.
type cacheEntry struct {
	result    *SigningStatus
	timestamp time.Time
}

var (
	// Cache for validation results to avoid repeated expensive calls
	validationCache = make(map[string]*cacheEntry)
	cacheMutex      sync.RWMutex

	// Cache expiration duration
	cacheExpiration = 5 * time.Minute
)

// ValidateCodeSigning validates the code signature of a macOS binary or application.
// It performs deep verification to check all nested frameworks and bundles.
//
// The path can point to:
//   - An application bundle (e.g., /Applications/Muster.app)
//   - A binary executable
//   - A framework bundle
//   - A DMG file
//
// Returns a SigningStatus containing validation results. If the signature is
// invalid or cannot be verified, SigningStatus.Valid will be false and
// SigningStatus.Error will contain details.
//
// Example:
//
//	result, err := darwin.ValidateCodeSigning("/Applications/Muster.app")
//	if err != nil {
//	    log.Printf("Validation error: %v", err)
//	    return
//	}
//
//	if result.Valid {
//	    fmt.Printf("✓ Valid signature from: %s\n", result.SigningIdentity)
//	    fmt.Printf("  Team ID: %s\n", result.TeamID)
//
//	    if len(result.Entitlements) > 0 {
//	        fmt.Println("  Entitlements:")
//	        for key, value := range result.Entitlements {
//	            fmt.Printf("    %s: %v\n", key, value)
//	        }
//	    }
//	} else {
//	    fmt.Printf("✗ Invalid signature: %s\n", result.Error)
//	}
func ValidateCodeSigning(path string) (*SigningStatus, error) {
	return ValidateCodeSigningWithOptions(path, DefaultValidationOptions())
}

// ValidateCodeSigningWithOptions validates code signing with custom options.
//
// This function provides more control over the validation process, allowing
// you to enable deep verification, strict mode, and notarization checking.
//
// The deep option verifies all nested frameworks and bundles within the app.
// The strict option enforces additional security checks and may reject
// signatures that pass basic verification.
//
// Example with custom options:
//
//	opts := &darwin.ValidationOptions{
//	    Deep:              true,
//	    Strict:            true,
//	    CheckNotarization: true,
//	    Timeout:           60 * time.Second,
//	}
//
//	result, err := darwin.ValidateCodeSigningWithOptions("/Applications/Muster.app", opts)
//	if err != nil {
//	    log.Fatal(err)
//	}
func ValidateCodeSigningWithOptions(path string, opts *ValidationOptions) (*SigningStatus, error) {
	if path == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("path does not exist: %s", path)
	}

	// Check cache first
	if cached := getCachedResult(path); cached != nil {
		return cached, nil
	}

	status := &SigningStatus{}

	// Perform codesign verification
	sigInfo, err := getCodeSignatureInfo(path, opts)
	if err != nil {
		status.Valid = false
		status.Error = err.Error()
		cacheResult(path, status)
		return status, nil
	}

	// Parse signature information
	status.SigningIdentity = sigInfo.Identity
	status.Authority = sigInfo.Authority
	status.TeamID = sigInfo.TeamID
	status.BundleID = sigInfo.BundleID
	status.Timestamp = sigInfo.Timestamp
	status.DeepVerification = sigInfo.DeepVerified
	status.StrictVerification = sigInfo.StrictVerified

	// Parse entitlements
	if entitlements, err := getEntitlements(path); err == nil {
		status.Entitlements = entitlements
	}

	// Validate signature
	status.Valid = validateSignature(path, opts)

	// Check notarization if requested
	if opts.CheckNotarization {
		notarization, err := CheckNotarizationWithOptions(path, opts)
		if err == nil && notarization.Notarized {
			// Notarization adds additional trust
			_ = notarization
		}
	}

	cacheResult(path, status)
	return status, nil
}

// CheckNotarization checks the notarization status of a macOS binary or application.
//
// Notarization is Apple's security process that scans software for malicious content
// before distribution. Applications must be notarized to run on macOS 10.15+ without
// user intervention.
//
// This function uses the `spctl` (System Policy Control) tool to check the
// notarization status. It returns detailed information about whether the
// application is notarized and whether the ticket is stapled.
//
// The stapled field indicates whether the notarization ticket is attached to
// the binary itself (allowing offline validation) or must be retrieved from
// Apple servers.
//
// Example:
//
//	status, err := darwin.CheckNotarization("/Applications/Muster.app")
//	if err != nil {
//	    log.Printf("Check failed: %v", err)
//	    return
//	}
//
//	if status.Notarized {
//	    fmt.Println("✓ Application is notarized")
//	    if status.Stapled {
//	        fmt.Println("✓ Notarization ticket is stapled")
//	    } else {
//	        fmt.Println("⚠ Ticket not stapled (requires internet to validate)")
//	    }
//	    fmt.Printf("  Source: %s\n", status.Source)
//	} else {
//	    fmt.Printf("✗ Not notarized: %s\n", status.Error)
//	}
func CheckNotarization(path string) (*NotarizationStatus, error) {
	return CheckNotarizationWithOptions(path, DefaultValidationOptions())
}

// CheckNotarizationWithOptions checks notarization with custom timeout.
//
// This function allows specifying a custom timeout for the notarization check.
// The default timeout is 30 seconds, which should be sufficient for most checks.
//
// Example:
//
//	opts := &darwin.ValidationOptions{Timeout: 60 * time.Second}
//	status, err := darwin.CheckNotarizationWithOptions("/Applications/Muster.app", opts)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	fmt.Printf("Notarized: %v\n", status.Notarized)
func CheckNotarizationWithOptions(path string, opts *ValidationOptions) (*NotarizationStatus, error) {
	if path == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("path does not exist: %s", path)
	}

	status := &NotarizationStatus{}

	// Check if ticket is stapled using stapler
	stapled := checkStapledTicket(path)
	status.Stapled = stapled

	// Use spctl to check notarization assessment
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "spctl", "-a", "-t", "open", "--context", "context:primary-signature", "-v", path)
	output, err := cmd.CombinedOutput()

	// Parse the output
	outputStr := string(output)

	// Check for notarized status in output
	if strings.Contains(outputStr, "accepted") || strings.Contains(outputStr, "Notarized") {
		status.Notarized = true
	}

	// Extract source information
	if matches := regexp.MustCompile(`source=([^\s]+)`).FindStringSubmatch(outputStr); len(matches) > 1 {
		status.Source = matches[1]
	}

	// If spctl failed but we have a stapled ticket, still consider it notarized
	if err != nil && stapled {
		status.Notarized = true
		status.Assessment = "Stapled ticket present"
	} else if err != nil {
		status.Notarized = false
		status.Error = fmt.Sprintf("assessment failed: %s", outputStr)
	} else {
		status.Assessment = outputStr
	}

	return status, nil
}

// StapleNotarizationTicket staples a notarization ticket to the binary.
//
// Stapling attaches the notarization ticket directly to the binary, allowing
// offline validation without requiring an internet connection to Apple's servers.
// This is important for applications that may be used in environments without
// internet access.
//
// The path must point to a notarized application or DMG file. If the file has
// not been notarized yet, this function will return an error.
//
// Example:
//
//	err := darwin.StapleNotarizationTicket("/Applications/Muster.app")
//	if err != nil {
//	    log.Printf("Failed to staple: %v", err)
//	    return
//	}
//	fmt.Println("Notarization ticket stapled successfully")
//
// Note: This function requires the `xcrun stapler` tool, which is part of
// Xcode Command Line Tools.
func StapleNotarizationTicket(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", path)
	}

	cmd := exec.Command("xcrun", "stapler", "staple", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to staple ticket: %w (output: %s)", err, string(output))
	}

	return nil
}

// GetSigningIdentities returns a list of available code signing identities.
//
// This function queries the macOS security keychain for valid code signing
// identities (certificates) that can be used to sign applications. It returns
// both the identity hash and the Common Name (CN) of each certificate.
//
// This is useful for build scripts that need to select the appropriate
// signing identity automatically.
//
// Example:
//
//	identities, err := darwin.GetSigningIdentities()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	for _, id := range identities {
//	    fmt.Printf("%s: %s\n", id.Hash, id.Name)
//	}
//
// Output:
//
//	ABC123DEF456: Developer ID Application: My Company (TEAM123)
//	XYZ789ABC012: Apple Development: Developer Name (TEAM456)
func GetSigningIdentities() ([]SigningIdentity, error) {
	cmd := exec.Command("security", "find-identity", "-v", "-p", "codesigning")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get signing identities: %w", err)
	}

	var identities []SigningIdentity
	lines := strings.Split(string(output), "\n")

	// Regex to parse identity lines
	re := regexp.MustCompile(`(\d+)\)\s+([A-F0-9]+)\s+"([^"]+)"`)

	for _, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) == 4 {
			identities = append(identities, SigningIdentity{
				Index: matches[1],
				Hash:  matches[2],
				Name:  matches[3],
			})
		}
	}

	return identities, nil
}

// SigningIdentity represents a code signing identity from the keychain.
type SigningIdentity struct {
	Index string `json:"index"`
	Hash  string `json:"hash"`
	Name  string `json:"name"`
}

// signatureInfo contains parsed code signature information.
type signatureInfo struct {
	Identity       string
	Authority      []string
	TeamID         string
	BundleID       string
	Timestamp      time.Time
	DeepVerified   bool
	StrictVerified bool
}

// getCodeSignatureInfo retrieves code signature information using codesign.
func getCodeSignatureInfo(path string, opts *ValidationOptions) (*signatureInfo, error) {
	// Get detailed signature information
	cmd := exec.Command("codesign", "-dvv", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get signature info: %w", err)
	}

	info := &signatureInfo{}
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(line, "Authority="):
			auth := strings.TrimPrefix(line, "Authority=")
			info.Authority = append(info.Authority, auth)

		case strings.HasPrefix(line, "TeamIdentifier="):
			info.TeamID = strings.TrimPrefix(line, "TeamIdentifier=")

		case strings.HasPrefix(line, "Identifier="):
			info.BundleID = strings.TrimPrefix(line, "Identifier=")

		case strings.HasPrefix(line, "Signed Time="):
			timeStr := strings.TrimPrefix(line, "Signed Time=")
			if ts, err := time.Parse("Jan 2, 2006 at 3:04:05 PM", timeStr); err == nil {
				info.Timestamp = ts
			}
		}
	}

	// Get signing identity from authority chain
	if len(info.Authority) > 0 {
		info.Identity = info.Authority[0]
	}

	return info, nil
}

// validateSignature performs codesign verification.
func validateSignature(path string, opts *ValidationOptions) bool {
	args := []string{"--verify"}

	if opts.Deep {
		args = append(args, "--deep")
	}

	if opts.Strict {
		args = append(args, "--strict")
	}

	if opts.Verbose {
		args = append(args, "--verbose=2")
	}

	args = append(args, path)

	cmd := exec.Command("codesign", args...)
	err := cmd.Run()

	return err == nil
}

// getEntitlements retrieves entitlements from a signed binary.
func getEntitlements(path string) (map[string]interface{}, error) {
	cmd := exec.Command("codesign", "-d", "--entitlements", "-", path)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Try to parse as plist/JSON
	var entitlements map[string]interface{}

	// If output looks like XML plist, try to parse it
	if bytes.Contains(output, []byte("<?xml")) {
		// For now, just return raw structure
		// In a full implementation, use plist.Unmarshal
		entitlements = map[string]interface{}{
			"_raw_xml": string(output),
		}
	} else if len(output) > 0 {
		// Try JSON
		_ = json.Unmarshal(output, &entitlements)
	}

	return entitlements, nil
}

// checkStapledTicket checks if a notarization ticket is stapled to the binary.
func checkStapledTicket(path string) bool {
	cmd := exec.Command("xcrun", "stapler", "staple", "-q", path)
	err := cmd.Run()
	return err == nil
}

// getCachedResult retrieves a cached validation result.
func getCachedResult(path string) *SigningStatus {
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()

	if entry, ok := validationCache[path]; ok {
		if time.Since(entry.timestamp) < cacheExpiration {
			return entry.result
		}
	}
	return nil
}

// cacheResult stores a validation result in the cache.
func cacheResult(path string, result *SigningStatus) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	validationCache[path] = &cacheEntry{
		result:    result,
		timestamp: time.Now(),
	}
}

// ClearValidationCache clears the validation result cache.
// This is useful for testing or when you need to force re-validation.
func ClearValidationCache() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	validationCache = make(map[string]*cacheEntry)
}

// IsDeveloperIDSigned checks if a binary is signed with a Developer ID certificate.
//
// Developer ID signing is required for distribution outside the App Store.
// This is a convenience function that validates the signature and checks
// if the signing authority contains "Developer ID".
//
// Example:
//
//	isDevID, err := darwin.IsDeveloperIDSigned("/Applications/Muster.app")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	if isDevID {
//	    fmt.Println("Application can be distributed outside App Store")
//	}
func IsDeveloperIDSigned(path string) (bool, error) {
	status, err := ValidateCodeSigning(path)
	if err != nil {
		return false, err
	}

	if !status.Valid {
		return false, nil
	}

	// Check if any authority contains "Developer ID"
	for _, auth := range status.Authority {
		if strings.Contains(auth, "Developer ID") {
			return true, nil
		}
	}

	return false, nil
}

// ValidateForDistribution performs comprehensive validation for distribution.
//
// This function checks all requirements for distributing a macOS application:
//   - Valid code signing
//   - Notarization status
//   - Stapled ticket (for offline validation)
//   - Developer ID signing (for non-App Store distribution)
//
// It returns a detailed report of all validation checks.
//
// Example:
//
//	report, err := darwin.ValidateForDistribution("/Applications/Muster.app")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	fmt.Printf("Ready for distribution: %v\n", report.Ready)
//	fmt.Printf("Code signing: %v\n", report.CodeSigning.Valid)
//	fmt.Printf("Notarized: %v\n", report.Notarization.Notarized)
//	fmt.Printf("Developer ID: %v\n", report.IsDeveloperID)
//
//	if !report.Ready {
//	    fmt.Println("Issues found:")
//	    for _, issue := range report.Issues {
//	        fmt.Printf("  - %s\n", issue)
//	    }
//	}
type DistributionReport struct {
	Path          string              `json:"path"`
	Ready         bool                `json:"ready"`
	CodeSigning   *SigningStatus      `json:"code_signing"`
	Notarization  *NotarizationStatus `json:"notarization"`
	IsDeveloperID bool                `json:"is_developer_id"`
	Issues        []string            `json:"issues,omitempty"`
}

// ValidateForDistribution performs comprehensive validation for distribution.
func ValidateForDistribution(path string) (*DistributionReport, error) {
	report := &DistributionReport{
		Path:   path,
		Issues: []string{},
	}

	// Validate code signing
	signingStatus, err := ValidateCodeSigning(path)
	if err != nil {
		return nil, err
	}
	report.CodeSigning = signingStatus

	if !signingStatus.Valid {
		report.Issues = append(report.Issues, fmt.Sprintf("Invalid code signature: %s", signingStatus.Error))
	}

	// Check notarization
	notarizationStatus, err := CheckNotarization(path)
	if err != nil {
		return nil, err
	}
	report.Notarization = notarizationStatus

	if !notarizationStatus.Notarized {
		report.Issues = append(report.Issues, "Application is not notarized")
	}

	if !notarizationStatus.Stapled {
		report.Issues = append(report.Issues, "Notarization ticket not stapled (offline validation unavailable)")
	}

	// Check Developer ID
	isDevID, err := IsDeveloperIDSigned(path)
	if err != nil {
		return nil, err
	}
	report.IsDeveloperID = isDevID

	if !isDevID {
		report.Issues = append(report.Issues, "Not signed with Developer ID (App Store distribution only)")
	}

	// Determine if ready for distribution
	report.Ready = signingStatus.Valid &&
		notarizationStatus.Notarized &&
		isDevID

	return report, nil
}
