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
	if err.Severity() != SeverityMedium {
		t.Errorf("expected SeverityMedium, got %s", err.Severity())
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
	if err.Error() != "[BAD_REQUEST] bad input" {
		t.Errorf("unexpected Error() string: %s", err.Error())
	}

	inner := New(CodeValidation, "inner")
	outer := Wrap(inner, CodeDatabase, "db failed")
	s := outer.Error()
	if s != "[DATABASE] db failed: [VALIDATION] inner" {
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
