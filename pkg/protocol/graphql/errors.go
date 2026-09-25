package graphql

import (
	"fmt"
	"strings"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// GraphQL error extension codes this adapter classifies (the codes GraphQL
// servers conventionally publish in errors[].extensions.code).
const (
	codeUnauthenticated        = "UNAUTHENTICATED"
	codeForbidden              = "FORBIDDEN"
	codeBadUserInput           = "BAD_USER_INPUT"
	codeParseFailed            = "GRAPHQL_PARSE_FAILED"
	codeValidationFailed       = "GRAPHQL_VALIDATION_FAILED"
	codePersistedQueryNotFound = "PERSISTED_QUERY_NOT_FOUND"
	codeNotFound               = "NOT_FOUND"
	codeInternalServerError    = "INTERNAL_SERVER_ERROR"
	codeTooManyRequests        = "TOO_MANY_REQUESTS"
	introspectionMarker        = "introspection"
)

// GraphQLErrorLocation is one entry of an error's locations array.
type GraphQLErrorLocation struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// GraphQLError is one entry of a GraphQL response's errors array. GraphQL
// allows partial success: a response may carry data AND errors, which is why
// the executor surfaces that combination instead of collapsing it.
type GraphQLError struct {
	Message    string                 `json:"message"`
	Path       []interface{}          `json:"path"`
	Locations  []GraphQLErrorLocation `json:"locations"`
	Extensions map[string]interface{} `json:"extensions"`
}

// ExtensionCode returns the error's extensions.code, or "" when absent.
func (e GraphQLError) ExtensionCode() string {
	if e.Extensions == nil {
		return ""
	}
	code, _ := e.Extensions["code"].(string)
	return code
}

// PathString renders the error's response path ("user.profile.bio").
func (e GraphQLError) PathString() string {
	if len(e.Path) == 0 {
		return ""
	}
	parts := make([]string, 0, len(e.Path))
	for _, p := range e.Path {
		parts = append(parts, fmt.Sprint(p))
	}
	return strings.Join(parts, ".")
}

// mapExtensionCode maps a GraphQL extensions.code onto an AppError code per
// specs/PROTOCOLS.md §4.2. Unknown and absent codes map to CodeGeneric, which
// is the spec's default row.
func mapExtensionCode(code string) errors.Code {
	switch code {
	case codeUnauthenticated, codeForbidden:
		return errors.CodeUnauthorized
	case codeBadUserInput:
		return errors.CodeValidation
	case codeParseFailed, codeValidationFailed:
		return errors.CodeBadRequest
	case codePersistedQueryNotFound, codeNotFound:
		return errors.CodeNotFound
	case codeInternalServerError:
		return errors.CodeInternal
	case codeTooManyRequests:
		return errors.CodeRateLimit
	default:
		return errors.CodeGeneric
	}
}

// mapGraphQLError maps one GraphQL error onto an AppError, wrapping the original
// error so errors.IsError keeps working and --verbose shows the cause.
func mapGraphQLError(e GraphQLError) *errors.AppError {
	cause := fmt.Errorf("graphql error: %s", e.Message)
	appErr := errors.Wrap(cause, mapExtensionCode(e.ExtensionCode()),
		fmt.Sprintf("graphql: remote operation failed (%s)", e.Message))
	appErr = appErr.WithDetail("protocol", string(protocol.ProtocolGraphQL))
	if code := e.ExtensionCode(); code != "" {
		appErr = appErr.WithDetail("graphql_code", code)
	}
	if path := e.PathString(); path != "" {
		appErr = appErr.WithDetail("path", path)
	}
	return appErr
}

// errorsAppError maps a response's errors array onto a single AppError (the
// first error's classification, the whole array in details).
func errorsAppError(errs []GraphQLError) error {
	if len(errs) == 0 {
		return nil
	}
	appErr := mapGraphQLError(errs[0])
	if len(errs) > 1 {
		messages := make([]string, 0, len(errs))
		for _, e := range errs {
			messages = append(messages, e.Message)
		}
		appErr = appErr.WithDetail("errors", messages)
	}
	return appErr
}

// looksLikeIntrospectionRefusal reports whether a response's errors are the
// server refusing introspection (production hardening in Apollo, Hasura,
// graphql-java, ...: the message names __schema/__type or introspection).
func looksLikeIntrospectionRefusal(errs []GraphQLError) bool {
	for _, e := range errs {
		msg := strings.ToLower(e.Message)
		if strings.Contains(msg, "__schema") || strings.Contains(msg, "__type") ||
			strings.Contains(msg, introspectionMarker) {
			return true
		}
	}
	return false
}

// introspectionDisabledError builds the actionable error for a server that
// refuses introspection. It always names the SDL fallback so the operator has a
// next step, and never degrades into an empty command set (board row PROTO-005
// acceptance 5).
func introspectionDisabledError(endpoint string, errs []GraphQLError) error {
	server := ""
	if len(errs) > 0 {
		server = strings.TrimSpace(errs[0].Message)
	}

	target := endpoint
	if target == "" {
		target = "the endpoint"
	}

	appErr := errors.New(errors.CodeBadRequest, fmt.Sprintf(
		"graphql: %s refused the introspection query, so no schema could be read (server said: %s) — introspection is disabled on this server; ingest the schema as SDL instead (e.g. --schema schema.graphql), or point --schema at a recorded introspection JSON document",
		target, server))
	appErr = appErr.WithDetail("protocol", string(protocol.ProtocolGraphQL))
	appErr = appErr.WithDetail("introspection", "disabled")
	if len(errs) > 0 && errs[0].ExtensionCode() != "" {
		appErr = appErr.WithDetail("graphql_code", errs[0].ExtensionCode())
	}
	return appErr
}
