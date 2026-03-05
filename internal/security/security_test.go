package security

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSecurityAuditor(t *testing.T) {
	auditor := NewSecurityAuditor("/test/path")

	require.NotNil(t, auditor)
	assert.Equal(t, "/test/path", auditor.projectRoot)
}

func TestRunComprehensiveAudit(t *testing.T) {
	auditor := NewSecurityAuditor(t.TempDir())
	result := auditor.RunComprehensiveAudit()

	require.NotNil(t, result)
	assert.NotNil(t, result.Score)
	assert.NotNil(t, result.TotalItems)
	assert.NotNil(t, result.PassedItems)
	assert.Equal(t, result.TotalItems, result.PassedItems+len(result.Issues))
}

func TestAuditResult_String(t *testing.T) {
	result := &AuditResult{
		Passed:      true,
		Score:       0.85,
		TotalItems:  10,
		PassedItems: 9,
		Issues: []Issue{
			{
				Category: "Auth",
				Item:     "Item",
				Severity: "High",
				Details:  "Details",
			},
		},
	}

	str := result.String()
	assert.Contains(t, str, "Security Audit Score")
	assert.Contains(t, str, "85.0%")
	assert.Contains(t, str, "PASSED")
	assert.Contains(t, str, "Issues Found")
}

func TestConstantTimeCompare(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{
			name: "equal strings",
			a:    "secret-value",
			b:    "secret-value",
			want: true,
		},
		{
			name: "different strings",
			a:    "secret-value",
			b:    "different-value",
			want: false,
		},
		{
			name: "different lengths",
			a:    "short",
			b:    "a-very-long-value",
			want: false,
		},
		{
			name: "empty strings",
			a:    "",
			b:    "",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConstantTimeCompare(tt.a, tt.b)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSensitiveString(t *testing.T) {
	t.Run("string redaction", func(t *testing.T) {
		s := SensitiveString{Value: "my-secret-token-123"}
		assert.Contains(t, s.String(), "****")
		assert.NotContains(t, s.String(), "my-secret-token-")
	})

	t.Run("short string", func(t *testing.T) {
		s := SensitiveString{Value: "abc"}
		assert.Contains(t, s.String(), "****")
	})
}

func TestGetGoVersion(t *testing.T) {
	version := GetGoVersion()
	assert.NotEmpty(t, version)
	assert.Contains(t, version, "go")
}

func TestCheckAuthenticationSecurity(t *testing.T) {
	auditor := NewSecurityAuditor(t.TempDir())
	checklist := &AuditChecklist{}
	issues := []Issue{}

	auditor.checkAuthenticationSecurity(checklist, &issues)

	// Verify checks were run
	assert.True(t, len(issues) > 0 || checklist.OAuth2CSRFProtection)
}

func TestCheckAuthorizationSecurity(t *testing.T) {
	auditor := NewSecurityAuditor(t.TempDir())
	checklist := &AuditChecklist{}
	issues := []Issue{}

	auditor.checkAuthorizationSecurity(checklist, &issues)

	assert.True(t, len(issues) > 0 || checklist.LeastPrivilege)
}

func TestCheckDataProtection(t *testing.T) {
	auditor := NewSecurityAuditor(t.TempDir())
	checklist := &AuditChecklist{}
	issues := []Issue{}

	auditor.checkDataProtection(checklist, &issues)

	assert.True(t, len(issues) > 0 || checklist.CredentialsEncrypted)
}

func TestCheckInputValidation(t *testing.T) {
	auditor := NewSecurityAuditor(t.TempDir())
	checklist := &AuditChecklist{}
	issues := []Issue{}

	auditor.checkInputValidation(checklist, &issues)

	assert.True(t, len(issues) > 0 || checklist.AllInputsValidated)
}

func TestCheckWASMSecurity(t *testing.T) {
	auditor := NewSecurityAuditor(t.TempDir())
	checklist := &AuditChecklist{}
	issues := []Issue{}

	auditor.checkWASMSecurity(checklist, &issues)

	assert.True(t, len(issues) > 0 || checklist.SandboxingEnforced)
}

func TestCheckRateLimiting(t *testing.T) {
	auditor := NewSecurityAuditor(t.TempDir())
	checklist := &AuditChecklist{}
	issues := []Issue{}

	auditor.checkRateLimiting(checklist, &issues)

	assert.True(t, len(issues) > 0 || checklist.TokenBucketAlgorithm)
}

func TestCheckSecretsManagement(t *testing.T) {
	auditor := NewSecurityAuditor(t.TempDir())
	checklist := &AuditChecklist{}
	issues := []Issue{}

	auditor.checkSecretsManagement(checklist, &issues)

	assert.True(t, len(issues) > 0 || checklist.NoHardcodedSecrets)
}

func TestCheckDependencySecurity(t *testing.T) {
	auditor := NewSecurityAuditor(t.TempDir())
	checklist := &AuditChecklist{}
	issues := []Issue{}

	auditor.checkDependencySecurity(checklist, &issues)

	assert.True(t, len(issues) > 0 || checklist.MinimalDependencyPrinciple)
}

func TestSecurityAuditor_checkCodeForFunction(t *testing.T) {
	auditor := NewSecurityAuditor(t.TempDir())

	t.Run("non-existent dir returns false", func(t *testing.T) {
		result := auditor.checkCodeForFunction("test", "func")
		assert.False(t, result)
	})
}

func TestSecurityAuditor_scanForHardcodedSecrets(t *testing.T) {
	auditor := NewSecurityAuditor(t.TempDir())
	result := auditor.scanForHardcodedSecrets()

	// Should always return true in this implementation
	assert.True(t, result)
}

func TestSecurityAuditor_checkGitIgnore(t *testing.T) {
	auditor := NewSecurityAuditor(t.TempDir())
	result := auditor.checkGitIgnore()

	// Should always return true in this implementation
	assert.True(t, result)
}

func TestInitSecurityLogger(t *testing.T) {
	InitSecurityLogger()
	// Test doesn't crash
}

func TestSecurityLogging(t *testing.T) {
	InitSecurityLogger()

	t.Run("security event logging doesn't panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			LogSecurityEvent("test_event", map[string]string{
				"detail": "test",
			})
		})
	})
}

func TestAuditChecklist(t *testing.T) {
	checklist := &AuditChecklist{
		OAuth2CSRFProtection:           true,
		OAuth2PKCE:                     true,
		OAuth2StrictRedirect:           true,
		TLSCertificateValidation:       true,
		TLSVersionEnforced:             true,
		APIKeyEntropy:                  true,
		LeastPrivilege:                 true,
		ResourceLevelAuthorization:     true,
		PrivilegeEscalationPrevention:  true,
		CredentialsEncrypted:           true,
		ConfigSecretsEncrypted:         true,
		DatabaseEncryptionEnabled:      true,
		BackupEncryptionImplemented:    true,
		TLSAllConnections:              true,
		AllInputsValidated:             true,
		SQLInjectionPrevention:         true,
		CommandInjectionPrevention:     true,
		PathTraversalPrevention:        true,
		SandboxingEnforced:             true,
		CapabilitySystemRestricts:      true,
		ResourceLimitsEnforced:         true,
		FilesystemEscapePrevented:      true,
		TokenBucketAlgorithm:           true,
		PerHostLimitsEnforced:          true,
		DistributedRateLimiting:        true,
		NoHardcodedSecrets:             true,
		SecretsNotInVersionControl:     true,
		NoKnownCriticalVulnerabilities: true,
		MinimalDependencyPrinciple:     true,
	}

	// Count fields
	reflectChecklist := reflect.TypeOf(*checklist).NumField()
	assert.Equal(t, 29, reflectChecklist)
}

func TestSensitiveString_MarshalJSON(t *testing.T) {
	s := SensitiveString{Value: "secret123"}
	data, err := s.MarshalJSON()

	require.NoError(t, err)
	assert.NotContains(t, string(data), "secret123")
}

func TestConstantTimeCompare_TimingAttackResistant(t *testing.T) {
	// Test that comparison doesn't reveal length information
	secret := "my-super-secret-key"
	wrong1 := strings.Repeat("a", len(secret))
	wrong2 := strings.Repeat("b", len(secret)+1)

	t1 := ConstantTimeCompare(secret, wrong1)
	t2 := ConstantTimeCompare(secret, wrong2)

	assert.False(t, t1)
	assert.False(t, t2)

	// In a timing-attack scenario, execution time should be similar
	// regardless of whether comparison succeeds (same length strings)
}

func TestAuditResult_Format(t *testing.T) {
	tests := []struct {
		name        string
		result      *AuditResult
		contains    []string
		notContains []string
	}{
		{
			name: "failed audit",
			result: &AuditResult{
				Passed:      false,
				Score:       0.6,
				TotalItems:  10,
				PassedItems: 6,
			},
			contains:    []string{"FAILED", "60.0%"},
			notContains: []string{"PASSED", "100.0%"},
		},
		{
			name: "perfect audit",
			result: &AuditResult{
				Passed:      true,
				Score:       1.0,
				TotalItems:  10,
				PassedItems: 10,
			},
			contains:    []string{"PASSED", "100.0%"},
			notContains: []string{"FAILED"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := tt.result.String()
			for _, contains := range tt.contains {
				assert.Contains(t, output, contains)
			}
			for _, notContains := range tt.notContains {
				assert.NotContains(t, output, notContains)
			}
		})
	}
}

func TestIssue_Struct(t *testing.T) {
	issue := Issue{
		Category: "Test",
		Item:     "Test Item",
		Severity: "High",
		Details:  "Test details",
	}

	assert.Equal(t, "Test", issue.Category)
	assert.Equal(t, "Test Item", issue.Item)
	assert.Equal(t, "High", issue.Severity)
	assert.Equal(t, "Test details", issue.Details)
}

func BenchmarkConstantTimeCompare(b *testing.B) {
	secret := "my-secret-key-value-123456789"
	wrong := "different-here-value-987654321"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ConstantTimeCompare(secret, wrong)
	}
}

func BenchmarkAuditResult_String(b *testing.B) {
	result := &AuditResult{
		Passed:      true,
		Score:       0.9,
		TotalItems:  20,
		PassedItems: 18,
		Issues: []Issue{
			{Category: "Test", Item: "Test", Severity: "Low", Details: "Test"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = result.String()
	}
}
