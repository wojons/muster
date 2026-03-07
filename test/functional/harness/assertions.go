// Copyright 2025 OpenAPI CLI Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package harness

import (
	"testing"
)

// AssertEqual asserts that two values are equal.
func AssertEqual[T comparable](t *testing.T, name string, expected, actual T) {
	t.Helper()
	if expected != actual {
		t.Errorf("%s: expected %v, got %v", name, expected, actual)
	}
}

// AssertNotEqual asserts that two values are not equal.
func AssertNotEqual[T comparable](t *testing.T, name string, expected, actual T) {
	t.Helper()
	if expected == actual {
		t.Errorf("%s: expected %v to not equal %v", name, expected, actual)
	}
}

// AssertTrue asserts that a condition is true.
func AssertTrue(t *testing.T, name string, condition bool) {
	t.Helper()
	if !condition {
		t.Errorf("%s: expected true, got false", name)
	}
}

// AssertFalse asserts that a condition is false.
func AssertFalse(t *testing.T, name string, condition bool) {
	t.Helper()
	if condition {
		t.Errorf("%s: expected false, got true", name)
	}
}

// AssertNil asserts that a value is nil.
func AssertNil(t *testing.T, name string, value interface{}) {
	t.Helper()
	if value != nil {
		t.Errorf("%s: expected nil, got %v", name, value)
	}
}

// AssertNotNil asserts that a value is not nil.
func AssertNotNil(t *testing.T, name string, value interface{}) {
	t.Helper()
	if value == nil {
		t.Errorf("%s: expected non-nil, got nil", name)
	}
}

// AssertContains asserts that a slice contains a value.
func AssertContains[T comparable](t *testing.T, name string, slice []T, value T) {
	t.Helper()
	for _, v := range slice {
		if v == value {
			return
		}
	}
	t.Errorf("%s: expected slice %v to contain %v", name, slice, value)
}

// AssertNotEmpty asserts that a value is not empty.
func AssertNotEmpty(t *testing.T, name string, value string) {
	t.Helper()
	if value == "" {
		t.Errorf("%s: expected non-empty string", name)
	}
}

// AssertLength asserts that a value has the expected length.
func AssertLength(t *testing.T, name string, value interface{}, expected int) {
	t.Helper()
	actual := 0

	switch v := value.(type) {
	case []interface{}:
		actual = len(v)
	case []string:
		actual = len(v)
	case string:
		actual = len(v)
	default:
		t.Errorf("%s: unsupported type for length assertion: %T", name, value)
		return
	}

	if actual != expected {
		t.Errorf("%s: expected length %d, got %d", name, expected, actual)
	}
}
