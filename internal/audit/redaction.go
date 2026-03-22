package audit

import (
	"encoding/json"
	"regexp"
	"strings"
)

// RedactionRule defines a pattern-based rule for redacting sensitive data
// from audit events. Rules can be mandatory (always applied) or optional
// (configurable by users).
type RedactionRule struct {
	Pattern     string   // Regex pattern to match sensitive data within values
	Replacement string   // Replacement text (e.g., "[REDACTED]")
	FieldPaths  []string // JSON field paths to apply (e.g., "$..password")
	Mandatory   bool     // Must be redacted (cannot be disabled by users)
}

// MandatoryRedactionRules defines the set of rules that must always be applied
// to audit events to prevent sensitive data leakage. These rules cover common
// credential patterns and cannot be disabled.
var MandatoryRedactionRules = []RedactionRule{
	{
		Pattern:     `(?i)\b(?:password|passwd|pwd)\b`,
		Replacement: "[REDACTED]",
		FieldPaths:  []string{"$..password", "$..passwd", "$..pwd"},
		Mandatory:   true,
	},
	{
		Pattern:     `(?i)\b(?:token|access_token|refresh_token|id_token)\b`,
		Replacement: "[REDACTED]",
		FieldPaths:  []string{"$..token", "$..access_token", "$..refresh_token", "$..id_token"},
		Mandatory:   true,
	},
	{
		Pattern:     `(?i)\b(?:secret|api_key|apikey|client_secret)\b`,
		Replacement: "[REDACTED]",
		FieldPaths:  []string{"$..secret", "$..api_key", "$..apikey", "$..client_secret"},
		Mandatory:   true,
	},
	{
		Pattern:     `(?i)\b(?:credit_card|cc_number|card_number)\b`,
		Replacement: "[REDACTED]",
		FieldPaths:  []string{"$..credit_card", "$..cc_number", "$..card_number"},
		Mandatory:   true,
	},
	{
		Pattern:     `(?i)\b(?:ssn|social_security)\b`,
		Replacement: "[REDACTED]",
		FieldPaths:  []string{"$..ssn", "$..social_security"},
		Mandatory:   true,
	},
}

// Redactor applies redaction rules to audit events to remove sensitive data
// before logging or storage. It pre-compiles regex patterns for performance
// and supports recursive JSON path traversal.
type Redactor struct {
	rules    []RedactionRule
	compiled []*regexp.Regexp
}

// NewRedactor creates a new Redactor initialized with mandatory redaction rules.
// The rules are compiled at initialization time for efficient matching during
// event processing. If a pattern fails to compile, a case-insensitive fallback
// is used.
func NewRedactor() *Redactor {
	r := &Redactor{rules: MandatoryRedactionRules}
	for _, rule := range r.rules {
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			// Use case-insensitive fallback
			re = regexp.MustCompile(`(?i)` + rule.Pattern)
		}
		r.compiled = append(r.compiled, re)
	}
	return r
}

// RedactEvent applies all configured redaction rules to an audit event.
// It serializes the event to JSON, applies redactions to matching field paths,
// and deserializes the result back into the event struct.
// Returns an error if JSON marshaling/unmarshaling fails.
func (r *Redactor) RedactEvent(event *AuditEvent) error {
	// Convert to map for JSON path processing
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	// Apply each rule
	for _, rule := range r.rules {
		for _, path := range rule.FieldPaths {
			r.redactPath(m, path, rule.Replacement)
		}
	}

	// Convert back
	newData, err := json.Marshal(m)
	if err != nil {
		return err
	}

	return json.Unmarshal(newData, event)
}

// redactPath applies redaction to a specific JSON path in the data map.
// It supports JSONPath-style syntax with recursive descent (e.g., "$..field").
func (r *Redactor) redactPath(
	m map[string]interface{},
	path string,
	replacement string,
) {
	// Parse the path to determine if it's recursive or direct
	path = strings.TrimPrefix(path, "$")

	if strings.HasPrefix(path, "..") {
		// Recursive descent: find field anywhere in the structure
		field := strings.TrimPrefix(path, "..")
		r.redactFieldRecursive(m, field, replacement)
	} else if strings.HasPrefix(path, ".") {
		// Direct path: traverse step by step
		path = strings.TrimPrefix(path, ".")
		parts := strings.Split(path, ".")
		r.redactDirectPath(m, parts, replacement)
	} else {
		// Just a field name at root level
		r.redactSingleField(m, path, replacement)
	}
}

// redactFieldRecursive searches for a field anywhere in the data structure
// and replaces its entire value with the replacement string.
// Field matching is case-insensitive.
func (r *Redactor) redactFieldRecursive(
	data interface{},
	field string,
	replacement string,
) {
	fieldLower := strings.ToLower(field)
	switch v := data.(type) {
	case map[string]interface{}:
		// Check if this map has the target field (case-insensitive)
		for k := range v {
			if strings.ToLower(k) == fieldLower {
				v[k] = replacement
			}
		}
		// Recurse into all nested maps
		for _, val := range v {
			r.redactFieldRecursive(val, field, replacement)
		}
	case []interface{}:
		// Recurse into array elements
		for _, item := range v {
			r.redactFieldRecursive(item, field, replacement)
		}
	}
}

// redactDirectPath traverses a direct path like "metadata.user.password"
// and redacts the value at the end of the path.
func (r *Redactor) redactDirectPath(
	data interface{},
	parts []string,
	replacement string,
) {
	if len(parts) == 0 {
		return
	}

	if len(parts) == 1 {
		// At the target field
		r.redactSingleField(data, parts[0], replacement)
		return
	}

	// Navigate deeper
	switch v := data.(type) {
	case map[string]interface{}:
		if next, ok := v[parts[0]]; ok {
			r.redactDirectPath(next, parts[1:], replacement)
		}
	case []interface{}:
		// Try each element in the array
		for _, item := range v {
			r.redactDirectPath(item, parts, replacement)
		}
	}
}

// redactSingleField replaces a specific field's value if it exists in the data.
// Field matching is case-insensitive.
func (r *Redactor) redactSingleField(
	data interface{},
	field string,
	replacement string,
) {
	if m, ok := data.(map[string]interface{}); ok {
		fieldLower := strings.ToLower(field)
		for k := range m {
			if strings.ToLower(k) == fieldLower {
				m[k] = replacement
			}
		}
	}
}
