package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type APIError struct {
	Err error `json:"error"`
}

func (ae *APIError) ToJSON() ([]byte, error) {
	return json.Marshal(ae)
}

func (ae *APIError) ToJSONString() (string, error) {
	b, err := ae.ToJSON()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func StatusCode(code Code) int {
	switch code {
	case CodeBadRequest:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	case CodeConflict:
		return http.StatusConflict
	case CodeInternal:
		return http.StatusInternalServerError
	case CodeServiceUnavailable:
		return http.StatusServiceUnavailable
	case CodeTimeout:
		return http.StatusRequestTimeout
	case CodeRateLimitExceeded:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

func HTTPStatus(code Code) int {
	return StatusCode(code)
}

func IsClientError(err error) bool {
	if err == nil {
		return false
	}
	e, ok := err.(*AppError)
	if !ok {
		return false
	}
	switch e.code {
	case CodeBadRequest,
		CodeUnauthorized,
		CodeForbidden,
		CodeNotFound,
		CodeConflict,
		CodeInvalidInput,
		CodeValidation:
		return true
	default:
		return false
	}
}

func IsServerError(err error) bool {
	if err == nil {
		return false
	}
	e, ok := err.(*AppError)
	if !ok {
		return false
	}
	switch e.code {
	case CodeInternal,
		CodeServiceUnavailable,
		CodeTimeout,
		CodeRateLimitExceeded,
		CodeNetwork,
		CodeDatabase,
		CodeResource:
		return true
	default:
		return false
	}
}

func IsRetriable(err error) bool {
	if err == nil {
		return false
	}
	e, ok := err.(*AppError)
	if !ok {
		return false
	}
	switch e.code {
	case CodeTimeout,
		CodeRateLimitExceeded,
		CodeNetwork,
		CodeServiceUnavailable:
		return true
	default:
		return false
	}
}

func Recover() error {
	if r := recover(); r != nil {
		switch v := r.(type) {
		case error:
			return Wrap(v, CodeInternal, "Recovered from panic")
		case string:
			return New(CodeInternal, fmt.Sprintf("Recovered from panic: %s", v))
		default:
			return New(CodeInternal, fmt.Sprintf("Recovered from panic: %v", v))
		}
	}
	return nil
}
