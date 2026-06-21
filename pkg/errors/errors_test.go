package errors

import (
	"fmt"
	"testing"
)

func TestNew(t *testing.T) {
	err := New(CodeBadRequest, "bad input")
	if err.Code() != CodeBadRequest {
		t.Errorf("expected CodeBadRequest, got %s", err.Code())
	}
	if err.Message() != "bad input" {
		t.Errorf("expected 'bad input', got %s", err.Message())
	}
	// New() does not set a default severity — it's the zero value
	if err.Severity() != "" {
		t.Errorf("expected empty severity, got %s", err.Severity())
	}
	if len(err.Details()) != 0 {
		t.Errorf("expected empty details, got %v", err.Details())
	}
	if err.Unwrap() != nil {
		t.Error("expected nil cause")
	}
}

func TestWrap(t *testing.T) {
	inner := fmt.Errorf("inner error")
	outer := Wrap(inner, CodeDatabase, "db failed")

	if outer.Code() != CodeDatabase {
		t.Errorf("expected CodeDatabase, got %s", outer.Code())
	}
	if outer.Unwrap() != inner {
		t.Error("expected inner error to be the cause")
	}
	if !IsError(outer, CodeDatabase) {
		t.Error("expected IsError to return true")
	}
}

func TestIsError(t *testing.T) {
	err := New(CodeNotFound, "not found")
	if !IsError(err, CodeNotFound) {
		t.Error("expected IsError to return true for matching code")
	}
	if IsError(err, CodeBadRequest) {
		t.Error("expected IsError to return false for non-matching code")
	}
	if IsError(nil, CodeNotFound) {
		t.Error("expected IsError to return false for nil")
	}

	// Test chained errors
	inner := New(CodeValidation, "inner")
	outer := Wrap(inner, CodeBadRequest, "outer")
	if !IsError(outer, CodeBadRequest) {
		t.Error("expected IsError to return true for outer code")
	}
	if !IsError(outer, CodeValidation) {
		t.Error("expected IsError to return true for chained inner code")
	}

	// Test non-AppError
	plain := fmt.Errorf("plain error")
	if IsError(plain, CodeGeneric) {
		t.Error("expected IsError to return false for non-AppError")
	}
}

func TestWithDetail(t *testing.T) {
	err := New(CodeValidation, "validation failed").
		WithDetail("field", "username").
		WithDetail("reason", "too short")

	if err.Details()["field"] != "username" {
		t.Errorf("expected detail 'field'='username', got %v", err.Details()["field"])
	}
	if err.Details()["reason"] != "too short" {
		t.Errorf("expected detail 'reason'='too short', got %v", err.Details()["reason"])
	}
}

func TestWithSeverity(t *testing.T) {
	err := New(CodeGeneric, "test").WithSeverity(SeverityCritical)
	if err.Severity() != SeverityCritical {
		t.Errorf("expected SeverityCritical, got %s", err.Severity())
	}
}

func TestErrorString(t *testing.T) {
	err := New(CodeBadRequest, "bad input")
	if err.Error() != "BAD_REQUEST: bad input" {
		t.Errorf("unexpected Error() string: %s", err.Error())
	}

	inner := New(CodeValidation, "inner")
	outer := Wrap(inner, CodeDatabase, "db failed")
	s := outer.Error()
	if s != "DATABASE_ERROR: db failed: VALIDATION_ERROR: inner" {
		t.Errorf("unexpected Error() string: %s", s)
	}
}

func TestAllCodes(t *testing.T) {
	codes := []Code{CodeBadRequest, CodeNotFound, CodeValidation, CodeGeneric, CodeDatabase}
	for _, c := range codes {
		err := New(c, "test")
		if err.Code() != c {
			t.Errorf("expected code %s, got %s", c, err.Code())
		}
	}
}

func TestAllSeverities(t *testing.T) {
	severities := []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo}
	for _, s := range severities {
		err := New(CodeGeneric, "test").WithSeverity(s)
		if err.Severity() != s {
			t.Errorf("expected severity %s, got %s", s, err.Severity())
		}
	}
}

// =============================================================================
// NEW TESTS — edge cases for remaining gaps
// =============================================================================

func TestErrorf(t *testing.T) {
	t.Run("simple format", func(t *testing.T) {
		err := Errorf(CodeBadRequest, "bad %s", "input")
		if err.Code() != CodeBadRequest {
			t.Errorf("expected CodeBadRequest, got %s", err.Code())
		}
		if err.Message() != "bad input" {
			t.Errorf("expected 'bad input', got %s", err.Message())
		}
	})

	t.Run("format with multiple args", func(t *testing.T) {
		err := Errorf(CodeNotFound, "resource %s not found in %s", "user-123", "database")
		if err.Message() != "resource user-123 not found in database" {
			t.Errorf("unexpected message: %s", err.Message())
		}
	})

	t.Run("format with no args", func(t *testing.T) {
		err := Errorf(CodeGeneric, "something happened")
		if err.Message() != "something happened" {
			t.Errorf("unexpected message: %s", err.Message())
		}
	})

	t.Run("format with integers", func(t *testing.T) {
		err := Errorf(CodeValidation, "expected %d items, got %d", 5, 3)
		if err.Message() != "expected 5 items, got 3" {
			t.Errorf("unexpected message: %s", err.Message())
		}
	})
}

func TestNilReceiverMethods(t *testing.T) {
	var err *AppError = nil

	t.Run("Code on nil", func(t *testing.T) {
		if err.Code() != "" {
			t.Error("expected empty code on nil")
		}
	})

	t.Run("Message on nil", func(t *testing.T) {
		if err.Message() != "" {
			t.Error("expected empty message on nil")
		}
	})

	t.Run("Severity on nil", func(t *testing.T) {
		if err.Severity() != "" {
			t.Error("expected empty severity on nil")
		}
	})

	t.Run("Details on nil", func(t *testing.T) {
		if err.Details() != nil {
			t.Error("expected nil details on nil receiver")
		}
	})

	t.Run("Unwrap on nil", func(t *testing.T) {
		if err.Unwrap() != nil {
			t.Error("expected nil return from nil receiver Unwrap")
		}
	})

	t.Run("Error string on nil", func(t *testing.T) {
		if err.Error() != "" {
			t.Error("expected empty error string on nil")
		}
	})

	t.Run("WithDetail on nil", func(t *testing.T) {
		result := err.WithDetail("key", "value")
		if result != nil {
			t.Error("expected nil return from WithDetail on nil")
		}
	})

	t.Run("WithSeverity on nil", func(t *testing.T) {
		result := err.WithSeverity(SeverityHigh)
		if result != nil {
			t.Error("expected nil return from WithSeverity on nil")
		}
	})
}

func TestUnwrap_NoCause(t *testing.T) {
	t.Run("new error has nil cause", func(t *testing.T) {
		err := New(CodeNotFound, "not found")
		if err.Unwrap() != nil {
			t.Error("expected nil cause for new error")
		}
	})
}

func TestIsError_EdgeCases(t *testing.T) {
	t.Run("nil error returns false", func(t *testing.T) {
		if IsError(nil, CodeNotFound) {
			t.Error("expected false for nil error")
		}
	})

	t.Run("non-AppError that is nil", func(t *testing.T) {
		var plain error = nil
		if IsError(plain, CodeGeneric) {
			t.Error("expected false for nil plain error")
		}
	})
}

func TestWithDetail_NilDetails(t *testing.T) {
	t.Run("add detail when details map is nil", func(t *testing.T) {
		err := &AppError{code: CodeGeneric, message: "test"}
		// details map is nil initially
		result := err.WithDetail("key", "value")
		if result != err {
			t.Error("expected same error returned")
		}
		if result.Details()["key"] != "value" {
			t.Error("expected detail to be set")
		}
	})
}

func TestWithSeverity_AllLevels(t *testing.T) {
	levels := map[Severity]string{
		SeverityCritical: "critical",
		SeverityHigh:     "high",
		SeverityMedium:   "medium",
		SeverityLow:      "low",
		SeverityInfo:     "info",
	}

	for sev, name := range levels {
		t.Run(name, func(t *testing.T) {
			err := New(CodeGeneric, "test").WithSeverity(sev)
			if err.Severity() != sev {
				t.Errorf("expected severity %s, got %s", sev, err.Severity())
			}
		})
	}
}
