package audit

import (
	"fmt"
	"strings"
)

// RedactionRule describes one mandatory sensitive-field rule from
// specs/EVENTS-AUDIT.md ("Credential Redaction & PII Protection").
//
// A rule is keyed by field name, not by value: FieldPaths lists the JSON field
// paths whose *value* is replaced with Replacement. Pattern documents the field
// vocabulary the rule covers and is the spelling used by the spec.
type RedactionRule struct {
	Pattern     string   // Regex pattern naming the sensitive field vocabulary
	Replacement string   // Replacement text written in place of the value
	FieldPaths  []string // JSON field paths whose values are replaced
	Mandatory   bool     // Must be applied (cannot be disabled by configuration)
}

// RedactedValue is the placeholder written in place of every sensitive value.
const RedactedValue = "[REDACTED]"

// MandatoryRedactionRules is the complete, closed set of sensitive fields the
// audit sink refuses to persist. It is mandatory by construction: nothing in
// this package gates it on AuditConfig, so a caller cannot switch it off
// (AuditConfig.RedactSensitive is reported for visibility only).
var MandatoryRedactionRules = []RedactionRule{
	{
		Pattern:     `(?i)\b(?:password|passwd|pwd)\b`,
		Replacement: RedactedValue,
		FieldPaths:  []string{"$..password", "$..passwd", "$..pwd"},
		Mandatory:   true,
	},
	{
		Pattern:     `(?i)\b(?:token|access_token|refresh_token|id_token)\b`,
		Replacement: RedactedValue,
		FieldPaths:  []string{"$..token", "$..access_token", "$..refresh_token", "$..id_token"},
		Mandatory:   true,
	},
	{
		Pattern:     `(?i)\b(?:secret|api_key|apikey|client_secret)\b`,
		Replacement: RedactedValue,
		FieldPaths:  []string{"$..secret", "$..api_key", "$..apikey", "$..client_secret"},
		Mandatory:   true,
	},
	{
		Pattern:     `(?i)\b(?:authorization|auth_header|bearer)\b`,
		Replacement: RedactedValue,
		FieldPaths:  []string{"$..authorization", "$..auth_header"},
		Mandatory:   true,
	},
	{
		Pattern:     `(?i)\b(?:credit_card|cc_number|card_number)\b`,
		Replacement: RedactedValue,
		FieldPaths:  []string{"$..credit_card", "$..cc_number", "$..card_number"},
		Mandatory:   true,
	},
	{
		Pattern:     `(?i)\b(?:ssn|social_security)\b`,
		Replacement: RedactedValue,
		FieldPaths:  []string{"$..ssn", "$..social_security"},
		Mandatory:   true,
	},
}

// sensitiveFields maps a canonical field name to its replacement text. It is
// derived once from MandatoryRedactionRules so the rule table above stays the
// single source of truth for what audit storage redacts.
var sensitiveFields = buildSensitiveFields()

// buildSensitiveFields flattens the mandatory rule table into canonical field
// names: lower case, alphanumerics only, so "API_KEY", "api-key" and "apiKey"
// all resolve to the same entry.
func buildSensitiveFields() map[string]string {
	fields := make(map[string]string, len(MandatoryRedactionRules)*3)
	for _, rule := range MandatoryRedactionRules {
		if !rule.Mandatory {
			continue
		}
		for _, path := range rule.FieldPaths {
			name := canonicalFieldName(path)
			if name == "" {
				continue
			}
			fields[name] = rule.Replacement
		}
	}
	return fields
}

// canonicalFieldName reduces a JSON field path ("$..api_key") or a raw field
// name ("X-Api-Key") to its comparable canonical form ("apikey").
func canonicalFieldName(name string) string {
	name = strings.TrimPrefix(name, "$")
	name = strings.TrimPrefix(name, "..")
	name = strings.ToLower(name)
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sensitiveReplacement reports whether a field name is sensitive and, if so,
// the replacement its value must take.
//
// Matching is by containment on the canonical name so decorated spellings are
// caught ("X-API-Key", "db_password", "Authorization"). Containment errs toward
// redaction: an over-redacted field is a readable log, an under-redacted one is
// a leaked credential.
func sensitiveReplacement(field string) (string, bool) {
	canonical := canonicalFieldName(field)
	if canonical == "" {
		return "", false
	}
	for name, replacement := range sensitiveFields {
		if strings.Contains(canonical, name) {
			return replacement, true
		}
	}
	return "", false
}

// RedactSensitive replaces the value of every sensitive field in data with
// "[REDACTED]", recursing into nested objects and arrays. Redaction happens in
// place, so callers pass the map they are about to persist.
//
// The audit sink calls this on every event it writes (see FileStorage.Write and
// prepareEvent). Redaction is mandatory: there is no configuration switch that
// disables it.
func RedactSensitive(data map[string]interface{}) {
	redactMap(data)
}

// redactMap redacts one object's fields in place.
func redactMap(data map[string]interface{}) {
	if data == nil {
		return
	}
	for key, value := range data {
		if replacement, sensitive := sensitiveReplacement(key); sensitive {
			data[key] = replacement
			continue
		}
		data[key] = redactValue(value)
	}
}

// redactValue redacts nested containers and returns the (possibly rebuilt)
// value for the parent to store back.
func redactValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		redactMap(v)
		return v
	case map[string]string:
		converted := make(map[string]interface{}, len(v))
		for key, item := range v {
			converted[key] = item
		}
		redactMap(converted)
		out := make(map[string]string, len(converted))
		for key, item := range converted {
			if s, ok := item.(string); ok {
				out[key] = s
				continue
			}
			out[key] = fmt.Sprint(item)
		}
		return out
	case []interface{}:
		for i, item := range v {
			v[i] = redactValue(item)
		}
		return v
	case []string:
		for i, item := range v {
			if replacement, sensitive := sensitiveReplacement(item); sensitive {
				v[i] = replacement
			}
		}
		return v
	default:
		return value
	}
}

// redactEvent applies mandatory redaction to the free-form surfaces of an audit
// event via applyRedaction. The sink write path calls applyRedaction directly;
// this wrapper exists for the spec-shaped Redactor handle.
func redactEvent(event *AuditEvent) {
	applyRedaction(event)
}

// RedactMetadata returns a redacted copy of an audit metadata map. The sink
// calls it for Context.Metadata, whose values are strings rather than arbitrary
// JSON, so the generic map redactor needs a typed adapter.
func RedactMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return metadata
	}
	converted := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		converted[key] = value
	}
	RedactSensitive(converted)
	out := make(map[string]string, len(converted))
	for key, value := range converted {
		if s, ok := value.(string); ok {
			out[key] = s
			continue
		}
		out[key] = fmt.Sprint(value)
	}
	return out
}

// Redactor applies the mandatory redaction rules to audit events. It exists as
// a spec-shaped handle (NewRedactor().RedactEvent(...)) for callers that want to
// sanitize an event before handing it anywhere other than the sink; the sink
// itself does not depend on an instance.
type Redactor struct {
	rules []RedactionRule
}

// NewRedactor creates a Redactor carrying the mandatory rules.
func NewRedactor() *Redactor {
	rules := make([]RedactionRule, len(MandatoryRedactionRules))
	copy(rules, MandatoryRedactionRules)
	return &Redactor{rules: rules}
}

// RedactEvent redacts in place every sensitive field of event.
func (r *Redactor) RedactEvent(event *AuditEvent) error {
	if event == nil {
		return fmt.Errorf("audit: cannot redact a nil event")
	}
	redactEvent(event)
	return nil
}

// SensitiveFieldNames returns the canonical field names that audit storage
// redacts, sorted for stable reporting (used by `audit info` output).
func SensitiveFieldNames() []string {
	names := make([]string, 0, len(sensitiveFields))
	for name := range sensitiveFields {
		names = append(names, name)
	}
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}
