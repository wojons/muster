package errors

import (
	"testing"
)

func TestNewAppError(t *testing.T) {
	err := New(CodeBadRequest, "test message")
	if err == nil {
		t.Fatal("expected error to not be nil")
	}

	if err.Code() != CodeBadRequest {
		t.Errorf("expected code to be %s, got %s", CodeBadRequest, err.Code())
	}

	if err.Message() != "test message" {
		t.Errorf("expected message to be 'test message', got '%s'", err.Message())
	}
}

func TestWrapAppError(t *testing.T) {
	inner := New(CodeBadRequest, "inner error")
	outer := Wrap(inner, CodeValidation, "outer error")

	s := outer.Error()
	if s == "" {
		t.Fatal("expected error to not be empty")
	}

	if outer.Code() != CodeValidation {
		t.Errorf("expected code to be %s, got %s", CodeValidation, outer.Code())
	}

	if outer.Unwrap() == nil {
		t.Fatal("expected unwrap to return inner error")
	}

	if outer.Unwrap().Error() != "inner error" {
		t.Errorf("expected unwrap message to be 'inner error', got '%s'", outer.Unwrap().Error())
	}
}

func TestErrorWithDetail(t *testing.T) {
	t.Run("single detail", func(t *testing.T) {
		err := New(CodeBadRequest, "test message")
		err = err.WithDetail("field", "value")

		if err.Details()["field"] != "value" {
			t.Errorf("expected detail field to be 'value', got '%v'", err.Details()["field"])
		}
	})

	t.Run("multiple details", func(t *testing.T) {
		err := New(CodeBadRequest, "test message")
		err = err.WithDetail("field1", "value1").WithDetail("field2", "value2")

		if err.Details()["field1"] != "value1" {
			t.Errorf("expected detail field1 to be 'value1', got '%v'", err.Details()["field1"])
		}
		if err.Details()["field2"] != "value2" {
			t.Errorf("expected detail field2 to be 'value2', got '%v'", err.Details()["field2"])
		}
	})

	t.Run("nil error", func(t *testing.T) {
		err := New(CodeBadRequest, "test message")
		details := err.Details()
		if details == nil {
			t.Errorf("expected details to not be nil")
		}
	})
}

func TestErrorWithSeverity(t *testing.T) {
	err := New(CodeBadRequest, "test message")
	err = err.WithSeverity(SeverityHigh)

	if err.Severity() != SeverityHigh {
		t.Errorf("expected severity to be %s, got %s", SeverityHigh, err.Severity())
	}
}

func TestErrorWithDetails(t *testing.T) {
	err := New(CodeBadRequest, "test message")
	details := map[string]interface{}{
		"field1": "value1",
		"field2": 42,
		"field3": true,
	}
	err = err.WithDetails(details)

	if err.Details()["field1"] != "value1" {
		t.Errorf("expected detail field1 to be 'value1', got '%v'", err.Details()["field1"])
	}
	if err.Details()["field2"] != 42 {
		t.Errorf("expected detail field2 to be 42, got '%v'", err.Details()["field2"])
	}
	if err.Details()["field3"] != true {
		t.Errorf("expected detail field3 to be true, got '%v'", err.Details()["field3"])
	}
}

func TestErrorWithCause(t *testing.T) {
	inner := New(CodeValidation, "inner message")
	err := New(CodeBadRequest, "outer message")
	err = err.WithCause(inner)

	if err.Unwrap() == nil {
		t.Fatal("expected Unwrap to return inner error")
	}
	if err.Unwrap().Error() != "inner message" {
		t.Errorf("expected unwrapped error message to be 'inner message', got '%s'", err.Unwrap().Error())
	}
}

func TestErrorIs(t *testing.T) {
	err1 := New(CodeBadRequest, "test")
	err2 := New(CodeBadRequest, "other message")

	if !err1.Is(err2) {
		t.Error("expected errors with same code to be considered equal")
	}

	err3 := New(CodeNotFound, "test")
	if err1.Is(err3) {
		t.Error("expected errors with different codes to not be considered equal")
	}

	if err1.Is(&AppError{}) {
		t.Error("expected Is to be false for non-AppError target")
	}
}

func TestErrorWithSeverityAllLevels(t *testing.T) {
	levels := []Severity{
		SeverityCritical,
		SeverityHigh,
		SeverityMedium,
		SeverityLow,
		SeverityInfo,
	}

	for _, level := range levels {
		t.Run(string(level), func(t *testing.T) {
			err := New(CodeGeneric, "test")
			err = err.WithSeverity(level)

			if err.Severity() != level {
				t.Errorf("expected severity %s, got %s", level, err.Severity())
			}
		})
	}
}

func TestWrapWithNewCause(t *testing.T) {
	inner := &testError{msg: "test error"}
	outer := Wrap(inner, CodeBadRequest, "wrapper")

	if outer.Unwrap() == nil {
		t.Fatal("expected Unwrap to return cause")
	}
	if outer.Unwrap() != inner {
		t.Error("expected Unwrap to return original error")
	}
}

func TestGetCodeWithNonAppError(t *testing.T) {
	err := &testError{msg: "not an app error"}
	code := GetCode(err)

	if code != CodeGeneric {
		t.Errorf("expected CodeGeneric for non-AppError, got %s", code)
	}
}

func TestFormat(t *testing.T) {
	t.Run("with error", func(t *testing.T) {
		err := New(CodeBadRequest, "test message")
		msg := Format(err)

		if msg == "" {
			t.Error("expected formatted message to not be empty")
		}
		if msg != "test message" {
			t.Errorf("expected formatted message 'test message', got '%s'", msg)
		}
	})

	t.Run("with nil error", func(t *testing.T) {
		msg := Format(nil)

		if msg != "" {
			t.Errorf("expected formatted message to be empty for nil, got '%s'", msg)
		}
	})

	t.Run("with cause", func(t *testing.T) {
		inner := New(CodeValidation, "inner")
		outer := Wrap(inner, CodeBadRequest, "outer")
		msg := Format(outer)

		if msg == "" {
			t.Fatal("expected formatted message to not be empty")
		}
		expected := "outer: inner"
		if msg != expected {
			t.Errorf("expected formatted message '%s', got '%s'", expected, msg)
		}
	})
}

func TestDefaultSeverity(t *testing.T) {
	err := New(CodeGeneric, "test")
	if err.Severity() != SeverityMedium {
		t.Errorf("expected default severity to be %s, got %s", SeverityMedium, err.Severity())
	}
}

func TestErrorUnwrap(t *testing.T) {
	t.Run("without cause", func(t *testing.T) {
		err := New(CodeBadRequest, "test message")
		if err.Unwrap() != nil {
			t.Error("expected Unwrap to return nil for error without cause")
		}
	})
}

func TestErrorWrapping(t *testing.T) {
	// Create a chain: Generic -> Validation -> BadRequest
	inner := New(CodeBadRequest, "inner message")
	middle := Wrap(inner, CodeValidation, "middle message")
	outer := Wrap(middle, CodeGeneric, "outer message")

	if outer.Code() != CodeGeneric {
		t.Errorf("expected outer code to be %s, got %s", CodeGeneric, outer.Code())
	}

	if outer.Unwrap() != middle {
		t.Error("expected outer to unwrap to middle")
	}

	if middle.Unwrap() != inner {
		t.Error("expected middle to unwrap to inner")
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestIsError(t *testing.T) {
	err := New(CodeBadRequest, "test message")

	if !IsError(err, CodeBadRequest) {
		t.Error("expected IsError to return true")
	}

	if IsError(err, CodeNotFound) {
		t.Error("expected IsError to return false")
	}
}

func TestGetCode(t *testing.T) {
	err := New(CodeBadRequest, "test message")

	if GetCode(err) != CodeBadRequest {
		t.Errorf("expected code to be %s, got %s", CodeBadRequest, GetCode(err))
	}

	if GetCode(nil) != CodeGeneric {
		t.Errorf("expected code to be %s for nil error, got %s", CodeGeneric, GetCode(nil))
	}
}

func TestHTTPStatus(t *testing.T) {
	if HTTPStatus(CodeBadRequest) != 400 {
		t.Errorf("expected HTTP status to be 400, got %d", HTTPStatus(CodeBadRequest))
	}

	if HTTPStatus(CodeNotFound) != 404 {
		t.Errorf("expected HTTP status to be 404, got %d", HTTPStatus(CodeNotFound))
	}

	if HTTPStatus(CodeInternal) != 500 {
		t.Errorf("expected HTTP status to be 500, got %d", HTTPStatus(CodeInternal))
	}
}

func TestIsClientError(t *testing.T) {
	if !IsClientError(ErrBadRequest) {
		t.Error("expected BadRequest to be a client error")
	}

	if !IsClientError(ErrUnauthorized) {
		t.Error("expected Unauthorized to be a client error")
	}

	if IsClientError(ErrInternal) {
		t.Error("expected Internal to not be a client error")
	}
}

func TestIsServerError(t *testing.T) {
	if !IsServerError(ErrInternal) {
		t.Error("expected Internal to be a server error")
	}

	if !IsServerError(ErrTimeout) {
		t.Error("expected Timeout to be a server error")
	}

	if IsServerError(ErrBadRequest) {
		t.Error("expected BadRequest to not be a server error")
	}
}

func TestIsRetriable(t *testing.T) {
	if !IsRetriable(ErrTimeout) {
		t.Error("expected Timeout to be retriable")
	}

	if !IsRetriable(ErrRateLimitExceeded) {
		t.Error("expected RateLimitExceeded to be retriable")
	}

	if IsRetriable(ErrBadRequest) {
		t.Error("expected BadRequest to not be retriable")
	}
}
