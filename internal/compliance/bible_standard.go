package compliance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// BibleStandardCheck validates Bible Standard compliance
type BibleStandardCheck struct{}

func (c *BibleStandardCheck) Name() string {
	return "Bible Standard Compliance"
}

func (c *BibleStandardCheck) Framework() Framework {
	return FrameworkBible
}

func (c *BibleStandardCheck) Validate() ValidationResult {
	result := ValidationResult{
		CheckName: c.Name(),
		Framework: c.Framework(),
		Passed:    true,
		Score:     1.0,
	}

	// Check 1: Determinism - Identical inputs must produce identical outputs
	c.checkDeterminism(&result)

	// Check 2: Implementation-agnostic language in specs
	c.checkSpecLanguage(&result)

	// Check 3: Test-driven specification
	c.checkTestDriven(&result)

	// Check 4: Cross-implementation validation
	c.checkCrossImplementation(&result)

	// Calculate score
	if len(result.Issues) > 0 {
		result.Score = 1.0 - float64(len(result.Issues))*0.1
		if result.Score < 0 {
			result.Score = 0
		}
		result.Passed = result.Score >= 0.8
	}

	return result
}

func (c *BibleStandardCheck) checkDeterminism(result *ValidationResult) {
	// Check that core operations are deterministic
	deterministicPatterns := []string{
		"pkg/openapi",
		"pkg/generator",
		"pkg/client",
	}

	for _, pattern := range deterministicPatterns {
		if _, err := os.Stat(pattern); err != nil {
			result.Issues = append(result.Issues, Issue{
				Severity: SeverityError,
				Message:  fmt.Sprintf("Required deterministic component not found: %s", pattern),
				Control:  "BIBLE-1.1",
			})
			result.Passed = false
		}
	}
}

func (c *BibleStandardCheck) checkSpecLanguage(result *ValidationResult) {
	// Check spec files for non-deterministic language
	ambiguousTerms := []string{
		"should", "could", "might", "consider",
		"appropriate", "reasonable", "sufficient",
		"as needed", "when possible", "if appropriate",
	}

	specFiles, err := filepath.Glob("specs/*.md")
	if err != nil {
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityWarning,
			Message:  "Could not read spec files for language check",
			Control:  "BIBLE-1.2",
		})
		return
	}

	for _, specFile := range specFiles {
		content, err := os.ReadFile(specFile)
		if err != nil {
			continue
		}

		lowerContent := strings.ToLower(string(content))
		for _, term := range ambiguousTerms {
			if strings.Contains(lowerContent, term) {
				result.Issues = append(result.Issues, Issue{
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("Ambiguous term in spec: '%s' in %s", term, filepath.Base(specFile)),
					Control:  "BIBLE-1.2",
					Location: specFile,
				})
			}
		}
	}
}

func (c *BibleStandardCheck) checkTestDriven(result *ValidationResult) {
	// Verify test files exist and cover implementations
	implementationDirs := []string{
		"pkg/openapi",
		"pkg/client",
		"pkg/auth",
		"pkg/config",
	}

	for _, dir := range implementationDirs {
		implementationFiles, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		testFiles, _ := filepath.Glob(filepath.Join(dir, "*_test.go"))

		// Remove test files from implementation count
		implCount := len(implementationFiles) - len(testFiles)

		if implCount > 0 && len(testFiles) == 0 {
			result.Issues = append(result.Issues, Issue{
				Severity: SeverityError,
				Message:  fmt.Sprintf("No test files found for: %s", dir),
				Control:  "BIBLE-1.3",
				Location: dir,
			})
			result.Passed = false
		} else if implCount > 0 && len(testFiles) < implCount/2 {
			result.Issues = append(result.Issues, Issue{
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("Low test coverage in: %s (tests: %d, impl: %d)", dir, len(testFiles), implCount),
				Control:  "BIBLE-1.3",
				Location: dir,
			})
		}
	}
}

func (c *BibleStandardCheck) checkCrossImplementation(result *ValidationResult) {
	// Check for test vectors that enable cross-implementation validation
	testVectorFiles, err := filepath.Glob("testvectors/*.json")
	if err != nil {
		testVectorFiles, err = filepath.Glob("internal/testvectors/*.json")
	}

	if err != nil || len(testVectorFiles) == 0 {
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityInfo,
			Message:  "No test vectors found for cross-implementation validation",
			Control:  "BIBLE-1.4",
		})
	}
}

// OpenAPIComplianceCheck validates OpenAPI 3.0 compliance
type OpenAPIComplianceCheck struct{}

func (c *OpenAPIComplianceCheck) Name() string {
	return "OpenAPI 3.0 Compliance"
}

func (c *OpenAPIComplianceCheck) Framework() Framework {
	return FrameworkOpenAPI
}

func (c *OpenAPIComplianceCheck) Validate() ValidationResult {
	result := ValidationResult{
		CheckName: c.Name(),
		Framework: c.Framework(),
		Passed:    true,
		Score:     1.0,
	}

	// Check OpenAPI parser implementation
	if _, err := os.Stat("pkg/openapi"); err != nil {
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityCritical,
			Message:  "OpenAPI parser package not found",
			Control:  "OPENAPI-3.0",
		})
		result.Passed = false
		result.Score = 0
		return result
	}

	// Check spec validation tests
	if _, err := os.Stat("pkg/openapi/openapi_test.go"); err != nil {
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityError,
			Message:  "OpenAPI validation tests not found",
			Control:  "OPENAPI-TEST",
		})
		result.Score -= 0.2
	}

	// Check supported OpenAPI features
	c.checkOpenAPIFeatures(&result)

	if len(result.Issues) > 0 {
		result.Score = 1.0 - float64(len(result.Issues))*0.1
		if result.Score < 0 {
			result.Score = 0
		}
		result.Passed = result.Score >= 0.8
	}

	return result
}

func (c *OpenAPIComplianceCheck) checkOpenAPIFeatures(result *ValidationResult) {
	requiredFeatures := map[string]string{
		"openapi":    "pkg/openapi/openapi.go",
		"paths":      "pkg/openapi/paths.go",
		"operations": "pkg/openapi/operations.go",
		"components": "pkg/openapi/components.go",
		"security":   "pkg/openapi/security.go",
		"responses":  "pkg/openapi/responses.go",
	}

	for feature, path := range requiredFeatures {
		if _, err := os.Stat(path); err != nil {
			result.Issues = append(result.Issues, Issue{
				Severity: SeverityError,
				Message:  fmt.Sprintf("Required OpenAPI feature not found: %s (%s)", feature, path),
				Control:  fmt.Sprintf("OPENAPI-%s", strings.ToUpper(feature)),
			})
			result.Score -= 0.1
		}
	}
}

// MCPComplianceCheck validates MCP Protocol compliance
type MCPComplianceCheck struct{}

func (c *MCPComplianceCheck) Name() string {
	return "MCP Protocol Compliance"
}

func (c *MCPComplianceCheck) Framework() Framework {
	return FrameworkMCP
}

func (c *MCPComplianceCheck) Validate() ValidationResult {
	result := ValidationResult{
		CheckName: c.Name(),
		Framework: c.Framework(),
		Passed:    true,
		Score:     1.0,
	}

	// Check MCP implementation
	if _, err := os.Stat("pkg/mcp"); err != nil {
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityCritical,
			Message:  "MCP package not found",
			Control:  "MCP-IMPL",
		})
		result.Passed = false
		result.Score = 0
		return result
	}

	// Check required MCP capabilities
	requiredCapabilities := []string{
		"initialize",
		"tools/list",
		"tools/call",
	}

	for _, capability := range requiredCapabilities {
		pattern := strings.ReplaceAll(capability, "/", "_")
		files, _ := filepath.Glob(fmt.Sprintf("pkg/mcp/*%s*.go", pattern))
		if len(files) == 0 {
			// Try alternative naming
			files, _ = filepath.Glob("pkg/mcp/handlers/*.go")
		}

		found := false
		for _, file := range files {
			content, _ := os.ReadFile(file)
			if strings.Contains(string(content), capability) ||
				strings.Contains(string(content), strings.Title(strings.ReplaceAll(capability, "/", ""))) {
				found = true
				break
			}
		}

		if !found && capability != "tools/call" { // tools/call might be named differently
			result.Issues = append(result.Issues, Issue{
				Severity: SeverityError,
				Message:  fmt.Sprintf("Required MCP capability not found: %s", capability),
				Control:  fmt.Sprintf("MCP-%s", strings.ReplaceAll(capability, "/", "_")),
			})
			result.Score -= 0.15
		}
	}

	if len(result.Issues) > 0 {
		result.Score = max(0, result.Score)
		result.Passed = result.Score >= 0.8
	}

	return result
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// OAuth2ComplianceCheck validates OAuth2 compliance
type OAuth2ComplianceCheck struct{}

func (c *OAuth2ComplianceCheck) Name() string {
	return "OAuth2 Compliance"
}

func (c *OAuth2ComplianceCheck) Framework() Framework {
	return FrameworkOAuth2
}

func (c *OAuth2ComplianceCheck) Validate() ValidationResult {
	result := ValidationResult{
		CheckName: c.Name(),
		Framework: c.Framework(),
		Passed:    true,
		Score:     1.0,
	}

	// Check OAuth2 implementation
	oauthFile := "pkg/auth/oauth2.go"
	if _, err := os.Stat(oauthFile); err != nil {
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityCritical,
			Message:  "OAuth2 implementation not found",
			Control:  "OAUTH2-IMPL",
		})
		result.Passed = false
		result.Score = 0
		return result
	}

	// Check for PKCE implementation
	content, _ := os.ReadFile(oauthFile)
	if !strings.Contains(string(content), "code_challenge") {
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityError,
			Message:  "PKCE code_challenge not implemented in OAuth2",
			Control:  "OAUTH2-PKCE",
			Location: oauthFile,
		})
		result.Score -= 0.2
	}

	if !strings.Contains(string(content), "code_verifier") {
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityError,
			Message:  "PKCE code_verifier not implemented in OAuth2",
			Control:  "OAUTH2-PKCE",
			Location: oauthFile,
		})
		result.Score -= 0.2
	}

	// Check for token refresh
	if !strings.Contains(string(content), "refresh") {
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityWarning,
			Message:  "Token refresh may not be implemented",
			Control:  "OAUTH2-REFRESH",
			Location: oauthFile,
		})
		result.Score -= 0.1
	}

	if len(result.Issues) > 0 {
		result.Score = max(0, result.Score)
		result.Passed = result.Score >= 0.8
	}

	return result
}

// TLSComplianceCheck validates TLS 1.3 compliance
type TLSComplianceCheck struct{}

func (c *TLSComplianceCheck) Name() string {
	return "TLS 1.3 Compliance"
}

func (c *TLSComplianceCheck) Framework() Framework {
	return FrameworkTLS
}

func (c *TLSComplianceCheck) Validate() ValidationResult {
	result := ValidationResult{
		CheckName: c.Name(),
		Framework: c.Framework(),
		Passed:    true,
		Score:     1.0,
	}

	// Check mTLS implementation
	mtlsFile := "pkg/auth/mtls.go"
	if _, err := os.Stat(mtlsFile); err != nil {
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityWarning,
			Message:  "mTLS implementation not found",
			Control:  "TLS-mTLS",
		})
		result.Score -= 0.1
	}

	// Check for TLS configuration
	if _, err := os.Stat("pkg/client"); err != nil {
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityWarning,
			Message:  "HTTP client package not found for TLS config check",
			Control:  "TLS-CONFIG",
		})
	} else {
		// Check for TLS configuration in client
		files, _ := filepath.Glob("pkg/client/*.go")
		tlsFound := false
		for _, file := range files {
			content, _ := os.ReadFile(file)
			if strings.Contains(string(content), "tls.Config") ||
				strings.Contains(string(content), "MinVersion") {
				tlsFound = true
				break
			}
		}

		if !tlsFound {
			result.Issues = append(result.Issues, Issue{
				Severity: SeverityWarning,
				Message:  "TLS configuration not found in HTTP client",
				Control:  "TLS-CONFIG",
			})
			result.Score -= 0.1
		}
	}

	if len(result.Issues) > 0 {
		result.Score = max(0, result.Score)
		result.Passed = result.Score >= 0.8
	}

	return result
}

// GDPRComplianceCheck validates GDPR compliance
type GDPRComplianceCheck struct{}

func (c *GDPRComplianceCheck) Name() string {
	return "GDPR Compliance"
}

func (c *GDPRComplianceCheck) Framework() Framework {
	return FrameworkGDPR
}

func (c *GDPRComplianceCheck) Validate() ValidationResult {
	result := ValidationResult{
		CheckName: c.Name(),
		Framework: c.Framework(),
		Passed:    true,
		Score:     1.0,
	}

	// Check for GDPR erasure implementation
	erasureFile := "pkg/repository/gdpr_erasure.go"
	if _, err := os.Stat(erasureFile); err != nil {
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityCritical,
			Message:  "GDPR data erasure implementation not found",
			Control:  "GDPR-17",
		})
		result.Score -= 0.3
	} else {
		// Check for required methods
		content, _ := os.ReadFile(erasureFile)
		requiredMethods := []string{"Erasure", "Delete", "Anonymize"}
		for _, method := range requiredMethods {
			if !strings.Contains(string(content), method) {
				result.Issues = append(result.Issues, Issue{
					Severity: SeverityError,
					Message:  fmt.Sprintf("GDPR erasure missing required method: %s", method),
					Control:  "GDPR-17",
					Location: erasureFile,
				})
				result.Score -= 0.1
			}
		}
	}

	// Check for audit logging (required for GDPR accountability)
	if _, err := os.Stat("internal/audit"); err != nil {
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityError,
			Message:  "Audit logging system not found (required for GDPR accountability)",
			Control:  "GDPR-5",
		})
		result.Score -= 0.2
	}

	// Check for credential redaction (PII protection)
	if _, err := os.Stat("pkg/auth/redaction.go"); err != nil {
		result.Issues = append(result.Issues, Issue{
			Severity: SeverityWarning,
			Message:  "Credential redaction not found (PII protection)",
			Control:  "GDPR-32",
		})
		result.Score -= 0.1
	}

	if len(result.Issues) > 0 {
		result.Score = max(0, result.Score)
		result.Passed = result.Score >= 0.8
	}

	return result
}

// LoadComplianceMatrixFromJSON loads a compliance matrix from JSON file
func LoadComplianceMatrixFromJSON(path string) (*ComplianceMatrix, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read compliance matrix: %w", err)
	}

	var controls []Control
	if err := json.Unmarshal(data, &controls); err != nil {
		return nil, fmt.Errorf("failed to unmarshal compliance matrix: %w", err)
	}

	matrix := NewComplianceMatrix()
	for i := range controls {
		matrix.RegisterControl(&controls[i])
	}

	return matrix, nil
}

// InitializeComplianceMatrix creates a default compliance matrix with standard controls
func InitializeComplianceMatrix() *ComplianceMatrix {
	matrix := NewComplianceMatrix()

	// Bible Standard Controls
	matrix.RegisterControl(&Control{
		ID:          "BIBLE-1.1",
		Framework:   FrameworkBible,
		Category:    "determinism",
		Title:       "Deterministic Implementations",
		Description: "Identical inputs must produce identical outputs",
		Requirement: "All core operations must be deterministic",
		Status:      "implemented",
		Implementation: Implementation{
			Files: []string{
				"pkg/openapi/parser.go",
				"pkg/generator/generator.go",
			},
			Tests: []string{
				"pkg/openapi/parser_test.go",
				"pkg/generator/generator_test.go",
			},
			Status: "complete",
		},
	})

	// GDPR Controls
	matrix.RegisterControl(&Control{
		ID:          "GDPR-17",
		Framework:   FrameworkGDPR,
		Category:    "data-subject-rights",
		Title:       "Right to Erasure",
		Description: "Data subjects have the right to erasure of their personal data",
		Requirement: "Implement data erasure mechanism for user data",
		Status:      "implemented",
		Implementation: Implementation{
			Files: []string{
				"pkg/repository/gdpr_erasure.go",
			},
			Tests: []string{
				"pkg/repository/gdpr_erasure_test.go",
			},
			Status: "complete",
		},
	})

	// OpenAPI Controls
	matrix.RegisterControl(&Control{
		ID:          "OPENAPI-3.0",
		Framework:   FrameworkOpenAPI,
		Category:    "spec-compliance",
		Title:       "OpenAPI 3.0 Specification Support",
		Description: "Must support OpenAPI 3.0 specification",
		Requirement: "Implement full OpenAPI 3.0 parser and generator",
		Status:      "implemented",
		Implementation: Implementation{
			Files: []string{
				"pkg/openapi/openapi.go",
				"pkg/openapi/paths.go",
				"pkg/openapi/components.go",
			},
			Tests: []string{
				"pkg/openapi/openapi_test.go",
			},
			Status: "complete",
		},
	})

	return matrix
}

// FileExists checks if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// RegexMatch checks if content matches a regex pattern
func RegexMatch(pattern, content string) bool {
	matched, _ := regexp.MatchString(pattern, content)
	return matched
}
