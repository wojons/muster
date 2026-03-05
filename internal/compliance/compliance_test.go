package compliance

import (
	"strings"
	"testing"
)

// TestDeterminismCheck tests deterministic language validation
func TestDeterminismCheck(t *testing.T) {
	t.Run("Clean Specification", func(t *testing.T) {
		check := &DeterminismCheck{
			SpecContent: "The system performs this operation. This implementation is deterministic.",
		}

		result := check.Validate()

		if !result.Passed {
			t.Errorf("Expected clean spec to pass")
		}

		if result.Score != 1.0 {
			t.Errorf("Expected perfect score, got %f", result.Score)
		}
	})

	t.Run("Ambiguous Language", func(t *testing.T) {
		check := &DeterminismCheck{
			SpecContent: "The system should perform this operation when appropriate.",
		}

		result := check.Validate()

		if result.Passed {
			t.Errorf("Expected ambiguous spec to fail")
		}

		if len(result.Issues) == 0 {
			t.Errorf("Expected issues found")
		}

		// Check for expected issues
		foundShould := false
		for _, issue := range result.Issues {
			if strings.Contains(issue.Message, "should") {
				foundShould = true
			}
		}

		if !foundShould {
			t.Errorf("Expected 'should' issue not found")
		}
	})
}

// TestCompletenessCheck tests section coverage validation
func TestCompletenessCheck(t *testing.T) {
	t.Run("Complete Specification", func(t *testing.T) {
		check := &CompletenessCheck{
			Sections: map[string]string{
				"Overview":       "An overview",
				"Architecture":   "System architecture",
				"Implementation": "Implementation details",
				"Error Handling": "Error handling",
				"Testing":        "Test coverage",
			},
		}

		result := check.Validate()

		if !result.Passed {
			t.Errorf("Expected complete spec to pass")
		}

		if result.Score != 1.0 {
			t.Errorf("Expected perfect score, got %f", result.Score)
		}
	})

	t.Run("Incomplete Specification", func(t *testing.T) {
		check := &CompletenessCheck{
			Sections: map[string]string{
				"Overview": "An overview",
			},
		}

		result := check.Validate()

		if result.Passed {
			t.Errorf("Expected incomplete spec to fail")
		}

		if len(result.Issues) == 0 {
			t.Errorf("Expected issues found")
		}
	})
}
