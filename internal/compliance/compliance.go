package compliance

import (
	"strings"
)

// Severity represents the severity of a compliance issue
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
	SeverityCritical
)

// Issue represents a compliance issue
type Issue struct {
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Location string   `json:"location,omitempty"`
}

// ValidationResult represents the result of a compliance check
type ValidationResult struct {
	CheckName string  `json:"check_name"`
	Passed    bool    `json:"passed"`
	Issues    []Issue `json:"issues,omitempty"`
	Score     float64 `json:"score"`
}

// Report represents a full compliance report
type Report struct {
	Timestamp  string             `json:"timestamp"`
	Results    []ValidationResult `json:"results"`
	TotalScore float64            `json:"total_score"`
	Passed     bool               `json:"passed"`
}

// Check represents a compliance check
type Check interface {
	Name() string
	Validate() ValidationResult
}

// DeterminismCheck validates that specifications use deterministic language
type DeterminismCheck struct {
	SpecContent string
}

func (c *DeterminismCheck) Name() string {
	return "Determinism"
}

func (c *DeterminismCheck) Validate() ValidationResult {
	result := ValidationResult{
		CheckName: c.Name(),
		Passed:    true,
	}

	ambiguousTerms := []string{
		"should", "could", "might", "consider",
		"appropriate", "reasonable", "sufficient",
	}

	lowerContent := strings.ToLower(c.SpecContent)
	for _, term := range ambiguousTerms {
		if strings.Contains(lowerContent, term) {
			result.Issues = append(result.Issues, Issue{
				Severity: SeverityWarning,
				Message:  "Ambiguous term found: '" + term + "'",
			})
			result.Passed = false
		}
	}

	if len(result.Issues) == 0 {
		result.Score = 1.0
	} else {
		result.Score = 1.0 - (float64(len(result.Issues)) * 0.1)
		if result.Score < 0 {
			result.Score = 0
		}
	}

	return result
}

// CompletenessCheck validates that specifications have all required sections
type CompletenessCheck struct {
	Sections map[string]string
}

func (c *CompletenessCheck) Name() string {
	return "Completeness"
}

func (c *CompletenessCheck) Validate() ValidationResult {
	result := ValidationResult{
		CheckName: c.Name(),
		Passed:    true,
	}

	requiredSections := []string{
		"Overview",
		"Architecture",
		"Implementation",
		"Error Handling",
		"Testing",
	}

	foundCount := 0
	for _, section := range requiredSections {
		if _, has := c.Sections[section]; has {
			foundCount++
		} else {
			result.Issues = append(result.Issues, Issue{
				Severity: SeverityError,
				Message:  "Missing required section: '" + section + "'",
			})
			result.Passed = false
		}
	}

	result.Score = float64(foundCount) / float64(len(requiredSections))

	return result
}
