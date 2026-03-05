package testvectors

import (
	"encoding/json"
	"testing"
)

// TestRunTestVector verifies the test vector runner works correctly
func TestRunTestVector(t *testing.T) {
	t.Run("Successful Execution", func(t *testing.T) {
		vector := TestVector{
			ID:          "test-001",
			Description: "Simple numeric operation",
			Input:       json.RawMessage(`{"value": 42}`),
			Expected:    json.RawMessage(`{"result": 42}`),
		}

		executor := func(input json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"result": 42}`), nil
		}

		RunTestVector(t, vector, executor)
	})

	t.Run("Equality Comparison", func(t *testing.T) {
		tests := []struct {
			name  string
			a     string
			b     string
			equal bool
		}{
			{"Identical", `{"a":1}`, `{"a":1}`, true},
			{"Different", `{"a":1}`, `{"a":2}`, false},
			{"Normalized", `{"z":1,"a":2}`, `{"a":2,"z":1}`, true},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				result := jsonEqual(json.RawMessage(tc.a), json.RawMessage(tc.b))
				if result != tc.equal {
					t.Errorf("Expected %v, got %v", tc.equal, result)
				}
			})
		}
	})
}

// TestRunSuite verifies test suite execution
func TestRunSuite(t *testing.T) {
	suite := &TestSuite{
		Name: "Math Operations",
		Vectors: []TestVector{
			{
				ID:          "add-001",
				Description: "Add two numbers",
				Input:       json.RawMessage(`{"op":"add","a":1,"b":2}`),
				Expected:    json.RawMessage(`{"result":3}`),
			},
		},
		Executor: func(input json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"result":3}`), nil
		},
	}

	suite.Run(t)
}
