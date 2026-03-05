package testvectors

import (
	"encoding/json"
	"testing"
)

// TestVector represents a single test case
type TestVector struct {
	ID          string          `json:"id"`
	Description string          `json:"description"`
	Input       json.RawMessage `json:"input"`
	Expected    json.RawMessage `json:"expected"`
}

// RunTestVector executes a test vector
func RunTestVector(t *testing.T, vector TestVector, executor func(input json.RawMessage) (json.RawMessage, error)) {
	t.Helper()
	t.Run(vector.ID, func(t *testing.T) {
		result, err := executor(vector.Input)
		if err != nil {
			t.Fatalf("Execution failed: %v", err)
		}

		if !jsonEqual(result, vector.Expected) {
			t.Errorf("Result mismatch:\nGot:      %s\nExpected: %s", result, vector.Expected)
		}
	})
}

// jsonEqual compares two JSON values for equality
func jsonEqual(a, b json.RawMessage) bool {
	var objA, objB interface{}
	if err := json.Unmarshal(a, &objA); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &objB); err != nil {
		return false
	}

	normalizedA, _ := json.Marshal(objA)
	normalizedB, _ := json.Marshal(objB)

	return string(normalizedA) == string(normalizedB)
}

// TestSuite represents a collection of test vectors
type TestSuite struct {
	Name     string       `json:"name"`
	Vectors  []TestVector `json:"vectors"`
	Executor func(json.RawMessage) (json.RawMessage, error)
}

// RunSuite executes all vectors in a test suite
func (suite *TestSuite) Run(t *testing.T) {
	t.Run(suite.Name, func(t *testing.T) {
		for _, vector := range suite.Vectors {
			RunTestVector(t, vector, suite.Executor)
		}
	})
}
