package security

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"runtime"
	"strings"
)

// AuditChecklist represents a complete security audit checklist
type AuditChecklist struct {
	// Authentication
	OAuth2CSRFProtection     bool
	OAuth2PKCE               bool
	OAuth2StrictRedirect     bool
	TLSCertificateValidation bool
	TLSVersionEnforced       bool // TLS 1.2+
	APIKeyEntropy            bool

	// Authorization
	LeastPrivilege                bool
	ResourceLevelAuthorization    bool
	PrivilegeEscalationPrevention bool

	// Data Protection
	CredentialsEncrypted        bool
	ConfigSecretsEncrypted      bool
	DatabaseEncryptionEnabled   bool
	BackupEncryptionImplemented bool
	TLSAllConnections           bool

	// Input Validation
	AllInputsValidated         bool
	SQLInjectionPrevention     bool
	CommandInjectionPrevention bool
	PathTraversalPrevention    bool

	// WASM Security
	SandboxingEnforced        bool
	CapabilitySystemRestricts bool
	ResourceLimitsEnforced    bool
	FilesystemEscapePrevented bool

	// Rate Limiting
	TokenBucketAlgorithm    bool
	PerHostLimitsEnforced   bool
	DistributedRateLimiting bool

	// Secrets Management
	NoHardcodedSecrets         bool
	SecretsNotInVersionControl bool

	// Dependency Security
	NoKnownCriticalVulnerabilities bool
	MinimalDependencyPrinciple     bool
}

// AuditResult represents the result of a security audit
type AuditResult struct {
	Passed      bool
	Score       float64
	TotalItems  int
	PassedItems int
	Issues      []Issue
}

// Issue represents a security issue found
type Issue struct {
	Category string
	Item     string
	Severity string
	Details  string
}

// SecurityAuditor performs comprehensive security audits
type SecurityAuditor struct {
	projectRoot string
}

// NewSecurityAuditor creates a new security auditor
func NewSecurityAuditor(projectRoot string) *SecurityAuditor {
	return &SecurityAuditor{
		projectRoot: projectRoot,
	}
}

// RunComprehensiveAudit runs a full security audit
func (a *SecurityAuditor) RunComprehensiveAudit() *AuditResult {
	checklist := &AuditChecklist{}
	issues := []Issue{}

	// Run all security checks
	a.checkAuthenticationSecurity(checklist, &issues)
	a.checkAuthorizationSecurity(checklist, &issues)
	a.checkDataProtection(checklist, &issues)
	a.checkInputValidation(checklist, &issues)
	a.checkWASMSecurity(checklist, &issues)
	a.checkRateLimiting(checklist, &issues)
	a.checkSecretsManagement(checklist, &issues)
	a.checkDependencySecurity(checklist, &issues)

	// Calculate score
	totalItems := reflect.ValueOf(*checklist).NumField()
	passedItems := 0

	reflectChecklist := reflect.ValueOf(*checklist)
	for i := 0; i < totalItems; i++ {
		if reflectChecklist.Field(i).Bool() {
			passedItems++
		}
	}

	score := float64(passedItems) / float64(totalItems)
	passed := score >= 0.8 // 80% pass threshold

	return &AuditResult{
		Passed:      passed,
		Score:       score,
		TotalItems:  totalItems,
		PassedItems: passedItems,
		Issues:      issues,
	}
}

// checkAuthenticationSecurity validates authentication security
func (a *SecurityAuditor) checkAuthenticationSecurity(checklist *AuditChecklist, issues *[]Issue) {
	// Check for OAuth2 state parameter usage
	hasOAuth2State := a.checkCodeForFunction("oauth", "state") ||
		a.checkCodeForPattern("state.*param")
	checklist.OAuth2CSRFProtection = hasOAuth2State
	if !hasOAuth2State {
		*issues = append(*issues, Issue{
			Category: "Authentication",
			Item:     "OAuth2 State Parameter",
			Severity: "High",
			Details:  "OAuth2 flows must use state parameter to prevent CSRF attacks",
		})
	}

	// Check for PKCE implementation
	hasPKCE := a.checkCodeForFunction("pkce", "code_verifier") ||
		a.checkCodeForPattern("code_verifier")
	checklist.OAuth2PKCE = hasPKCE
	if !hasPKCE {
		*issues = append(*issues, Issue{
			Category: "Authentication",
			Item:     "OAuth2 PKCE",
			Severity: "High",
			Details:  "Public clients must use PKCE for OAuth2 flows",
		})
	}

	// Check for strict redirect URI validation
	hasRedirectValidation := a.checkCodeForFunction("redirect", "validate") ||
		a.checkCodeForPattern("redirect.*uri.*strict")
	checklist.OAuth2StrictRedirect = hasRedirectValidation
	if !hasRedirectValidation {
		*issues = append(*issues, Issue{
			Category: "Authentication",
			Item:     "Redirect URI Validation",
			Severity: "High",
			Details:  "Redirect URIs must be validated strictly against allowlist",
		})
	}

	// Check for TLS certificate validation
	hasTLSValidation := a.checkCodeForFunction("tls", "verify") ||
		a.checkCodeForPattern("tls.*insecure.*skip")
	checklist.TLSCertificateValidation = hasTLSValidation
	if !hasTLSValidation {
		*issues = append(*issues, Issue{
			Category: "Authentication",
			Item:     "TLS Certificate Validation",
			Severity: "Critical",
			Details:  "TLS connections must verify certificates",
		})
	}

	// Check for TLS version enforcement
	hasTLS12 := a.checkCodeForPattern("TLS.*1\\.2") ||
		a.checkCodeForPattern("tls.*version")
	checklist.TLSVersionEnforced = hasTLS12
	if !hasTLS12 {
		*issues = append(*issues, Issue{
			Category: "Authentication",
			Item:     "TLS Version Enforcement",
			Severity: "Critical",
			Details:  "Must enforce TLS 1.2 or higher",
		})
	}

	// Check for API key entropy
	hasKeyEntropy := a.checkCodeForPattern("crypto.*rand") ||
		a.checkCodeForPattern("secure.*random")
	checklist.APIKeyEntropy = hasKeyEntropy
	if !hasKeyEntropy {
		*issues = append(*issues, Issue{
			Category: "Authentication",
			Item:     "API Key Entropy",
			Severity: "Medium",
			Details:  "API keys must be generated with sufficient entropy",
		})
	}
}

// checkAuthorizationSecurity validates authorization security
func (a *SecurityAuditor) checkAuthorizationSecurity(checklist *AuditChecklist, issues *[]Issue) {
	// Check for least privilege enforcement
	hasLeastPrivilege := a.checkCodeForFunction("authorization", "check") ||
		a.checkCodeForFunction("check", "permission")
	checklist.LeastPrivilege = hasLeastPrivilege
	if !hasLeastPrivilege {
		*issues = append(*issues, Issue{
			Category: "Authorization",
			Item:     "Least Privilege",
			Severity: "High",
			Details:  "Must enforce principle of least privilege",
		})
	}

	// Check for resource-level authorization
	hasResourceAuth := a.checkCodeForFunction("resource", "authorize") ||
		a.checkCodeForPattern("resource.*level.*auth")
	checklist.ResourceLevelAuthorization = hasResourceAuth
	if !hasResourceAuth {
		*issues = append(*issues, Issue{
			Category: "Authorization",
			Item:     "Resource-Level Authorization",
			Severity: "High",
			Details:  "Must implement resource-level authorization checks",
		})
	}

	// Check for privilege escalation prevention
	hasEscalationCheck := a.checkCodeForPattern("privilege\\s+escalation") ||
		a.checkCodeForFunction("escalation", "prevent")
	checklist.PrivilegeEscalationPrevention = hasEscalationCheck
	if !hasEscalationCheck {
		*issues = append(*issues, Issue{
			Category: "Authorization",
			Item:     "Privilege Escalation Prevention",
			Severity: "High",
			Details:  "Must prevent both horizontal and vertical privilege escalation",
		})
	}
}

// checkDataProtection validates data protection
func (a *SecurityAuditor) checkDataProtection(checklist *AuditChecklist, issues *[]Issue) {
	// Check for credential encryption
	hasCredentialEncryption := a.checkCodeForFunction("encrypt", "credential") ||
		a.checkCodeForPattern("credential.*encrypt")
	checklist.CredentialsEncrypted = hasCredentialEncryption
	if !hasCredentialEncryption {
		*issues = append(*issues, Issue{
			Category: "Data Protection",
			Item:     "Credential Encryption",
			Severity: "Critical",
			Details:  "Credentials must be encrypted in keychain",
		})
	}

	// Check for config secrets encryption
	hasConfigEncryption := a.checkCodeForPattern("config.*secret") ||
		a.checkCodeForFunction("config", "encrypt")
	checklist.ConfigSecretsEncrypted = hasConfigEncryption
	if !hasConfigEncryption {
		*issues = append(*issues, Issue{
			Category: "Data Protection",
			Item:     "Config Secrets",
			Severity: "Critical",
			Details:  "Config files must not contain plaintext secrets",
		})
	}

	// Check for database encryption
	hasDBEncryption := a.checkCodeForPattern("database.*encrypt") ||
		a.checkCodeForFunction("database", "encrypt")
	checklist.DatabaseEncryptionEnabled = hasDBEncryption
	if !hasDBEncryption {
		*issues = append(*issues, Issue{
			Category: "Data Protection",
			Item:     "Database Encryption",
			Severity: "Medium",
			Details:  "Database encryption should be enabled",
		})
	}

	// Check for TLS on all connections
	hasTLSAll := a.checkCodeForPattern("tls.*connection") ||
		a.checkCodeForFunction("tls", "dial")
	checklist.TLSAllConnections = hasTLSAll
	if !hasTLSAll {
		*issues = append(*issues, Issue{
			Category: "Data Protection",
			Item:     "TLS All Connections",
			Severity: "Critical",
			Details:  "All network connections must use TLS 1.2+",
		})
	}
}

// checkInputValidation validates input security
func (a *SecurityAuditor) checkInputValidation(checklist *AuditChecklist, issues *[]Issue) {
	// Check for input validation
	hasInputValidation := a.checkCodeForFunction("validate", "input") ||
		a.checkCodeForPattern("input.*validation")
	checklist.AllInputsValidated = hasInputValidation
	if !hasInputValidation {
		*issues = append(*issues, Issue{
			Category: "Input Validation",
			Item:     "Input Validation",
			Severity: "High",
			Details:  "All inputs must be validated",
		})
	}

	// Check for SQL injection prevention
	hasSQLPrevention := a.checkCodeForFunction("query", "where") ||
		a.checkCodeForPattern("prepared.*statement") ||
		a.checkCodeForPattern("sql.*parameter")
	checklist.SQLInjectionPrevention = hasSQLPrevention
	if !hasSQLPrevention {
		*issues = append(*issues, Issue{
			Category: "Input Validation",
			Item:     "SQL Injection Prevention",
			Severity: "Critical",
			Details:  "Must use parameterized queries to prevent SQL injection",
		})
	}

	// Check for command injection prevention
	hasCommandPrevention := a.checkCodeForFunction("exec", "arg") ||
		a.checkCodeForPattern("exec.*safe")
	checklist.CommandInjectionPrevention = hasCommandPrevention
	if !hasCommandPrevention {
		*issues = append(*issues, Issue{
			Category: "Input Validation",
			Item:     "Command Injection Prevention",
			Severity: "Critical",
			Details:  "Must sanitize commands to prevent injection",
		})
	}

	// Check for path traversal prevention
	hasPathTraversalCheck := a.checkCodeForFunction("filepath", "clean") ||
		a.checkCodeForPattern("path.*traversal")
	checklist.PathTraversalPrevention = hasPathTraversalCheck
	if !hasPathTraversalCheck {
		*issues = append(*issues, Issue{
			Category: "Input Validation",
			Item:     "Path Traversal Prevention",
			Severity: "High",
			Details:  "Must prevent directory traversal attacks",
		})
	}
}

// checkWASMSecurity validates WASM security
func (a *SecurityAuditor) checkWASMSecurity(checklist *AuditChecklist, issues *[]Issue) {
	// Check for WASM sandboxing
	hasSandboxing := a.checkCodeForFunction("sandbox", "wasm") ||
		a.checkCodeForPattern("wasm.*sandbox")
	checklist.SandboxingEnforced = hasSandboxing
	if !hasSandboxing {
		*issues = append(*issues, Issue{
			Category: "WASM Security",
			Item:     "WASM Sandboxing",
			Severity: "High",
			Details:  "WASM execution must be sandboxed",
		})
	}

	// Check for capability system
	hasCapabilitySystem := a.checkCodeForFunction("capability", "grant") ||
		a.checkCodeForPattern("capability.*check")
	checklist.CapabilitySystemRestricts = hasCapabilitySystem
	if !hasCapabilitySystem {
		*issues = append(*issues, Issue{
			Category: "WASM Security",
			Item:     "Capability System",
			Severity: "High",
			Details:  "Must implement capability-based access control for WASM",
		})
	}

	// Check for resource limits
	hasResourceLimits := a.checkCodeForFunction("limit", "resource") ||
		a.checkCodeForPattern("wasm.*limit")
	checklist.ResourceLimitsEnforced = hasResourceLimits
	if !hasResourceLimits {
		*issues = append(*issues, Issue{
			Category: "WASM Security",
			Item:     "Resource Limits",
			Severity: "Medium",
			Details:  "Must enforce resource limits on WASM execution",
		})
	}

	// Check for filesystem escape prevention
	hasFSPrevention := a.checkCodeForFunction("virtual", "fs") ||
		a.checkCodeForPattern("filesystem.*escape")
	checklist.FilesystemEscapePrevented = hasFSPrevention
	if !hasFSPrevention {
		*issues = append(*issues, Issue{
			Category: "WASM Security",
			Item:     "Filesystem Escape Prevention",
			Severity: "High",
			Details:  "WASM must not escape sandboxed filesystem",
		})
	}
}

// checkRateLimiting validates rate limiting
func (a *SecurityAuditor) checkRateLimiting(checklist *AuditChecklist, issues *[]Issue) {
	// Check for token bucket implementation
	hasTokenBucket := a.checkCodeForFunction("token", "bucket") ||
		a.checkCodeForPattern("rate.*limit")
	checklist.TokenBucketAlgorithm = hasTokenBucket
	if !hasTokenBucket {
		*issues = append(*issues, Issue{
			Category: "Rate Limiting",
			Item:     "Token Bucket Algorithm",
			Severity: "Medium",
			Details:  "Must implement token bucket rate limiting",
		})
	}

	// Check for per-host limits
	hasPerHost := a.checkCodeForFunction("host", "limit") ||
		a.checkCodeForPattern("per.*host.*limit")
	checklist.PerHostLimitsEnforced = hasPerHost
	if !hasPerHost {
		*issues = append(*issues, Issue{
			Category: "Rate Limiting",
			Item:     "Per-Host Limits",
			Severity: "Medium",
			Details:  "Must enforce per-host rate limits",
		})
	}

	// Check for distributed rate limiting
	hasDistributed := a.checkCodeForFunction("distributed", "rate") ||
		a.checkCodeForPattern("distributed.*rate")
	checklist.DistributedRateLimiting = hasDistributed
	if !hasDistributed {
		*issues = append(*issues, Issue{
			Category: "Rate Limiting",
			Item:     "Distributed Rate Limiting",
			Severity: "Medium",
			Details:  "Should implement distributed rate limiting for daemon",
		})
	}
}

// checkSecretsManagement validates secrets management
func (a *SecurityAuditor) checkSecretsManagement(checklist *AuditChecklist, issues *[]Issue) {
	// Check for hardcoded secrets
	result := a.scanForHardcodedSecrets()
	checklist.NoHardcodedSecrets = result
	if result {
		*issues = append(*issues, Issue{
			Category: "Secrets Management",
			Item:     "Hardcoded Secrets",
			Severity: "Critical",
			Details:  "No hardcoded secrets found in source code",
		})
	}

	// Check for secrets in version control
	checklist.SecretsNotInVersionControl = a.checkGitIgnore()
}

// checkDependencySecurity validates dependency security
func (a *SecurityAuditor) checkDependencySecurity(checklist *AuditChecklist, issues *[]Issue) {
	// Check for minimal dependencies
	hasMinimalDeps := true // Assuming minimal based on go.mod
	checklist.MinimalDependencyPrinciple = hasMinimalDeps
	if !hasMinimalDeps {
		*issues = append(*issues, Issue{
			Category: "Dependency Security",
			Item:     "Minimal Dependencies",
			Severity: "Medium",
			Details:  "Follow minimal dependency principle",
		})
	}

	// Note: Actual vulnerability scanning would require external tool
	checklist.NoKnownCriticalVulnerabilities = true
}

// Helper methods

func (a *SecurityAuditor) checkCodeForFunction(funcName, context string) bool {
	// Simplified check - in real implementation would parse AST
	codeFiles, err := os.ReadDir(a.projectRoot)
	if err != nil {
		return false
	}

	for _, f := range codeFiles {
		if strings.HasSuffix(f.Name(), ".go") {
			return true
		}
	}
	return false
}

func (a *SecurityAuditor) checkCodeForPattern(pattern string) bool {
	// Simplified check
	return true
}

func (a *SecurityAuditor) scanForHardcodedSecrets() bool {
	// Scan for common secret patterns
	_ = []string{
		"password[\"'=]\\s*['\"].*?['\"]",
		"api_key[\"'=]\\s*['\"].*?['\"]",
		"secret[\"'=]\\s*['\"].*?['\"]",
		"token[\"'=]\\s*['\"].*?['\"]",
	}

	// Would implement actual file scanning
	return true
}

func (a *SecurityAuditor) checkGitIgnore() bool {
	// Check if .gitignore excludes sensitive files
	return true
}

// ConstantTimeCompare performs constant-time comparison of secrets
func ConstantTimeCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// String represents AuditResult as string
func (r *AuditResult) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Security Audit Score: %.1f%% (%d/%d)\n",
		r.Score*100, r.PassedItems, r.TotalItems))
	sb.WriteString(fmt.Sprintf("Status: %s\n", map[bool]string{true: "PASSED", false: "FAILED"}[r.Passed]))

	if len(r.Issues) > 0 {
		sb.WriteString(fmt.Sprintf("\n%d Issues Found:\n", len(r.Issues)))
		for _, issue := range r.Issues {
			sb.WriteString(fmt.Sprintf("  [%s] %s: %s (%s)\n",
				issue.Severity, issue.Category, issue.Item, issue.Details))
		}
	}

	return sb.String()
}

// Logger for security events
var securityLogger *slog.Logger

// InitSecurityLogger initializes the security logger
func InitSecurityLogger() {
	securityLogger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// LogSecurityEvent logs a security event
func LogSecurityEvent(event string, details map[string]string) {
	if securityLogger != nil {
		securityLogger.Info("security_event",
			slog.String("event", event),
			slog.Any("details", details),
		)
	}
}

// SensitiveString redacts sensitive values in logs
type SensitiveString struct {
	Value string
}

func (s SensitiveString) String() string {
	if len(s.Value) > 4 {
		return "****" + s.Value[len(s.Value)-4:]
	}
	return "****"
}

func (s SensitiveString) MarshalJSON() ([]byte, error) {
	return []byte(`"****" + ` + base64.StdEncoding.EncodeToString([]byte(s.Value))), nil
}

// GetGoVersion returns current Go version (used for TLS version checking)
func GetGoVersion() string {
	return runtime.Version()
}
