package boundary

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/quarkloop/pkg/plugin"
)

type Boundary string

const (
	Provider   Boundary = "provider"
	Tool       Boundary = "tool"
	Service    Boundary = "service"
	Indexer    Boundary = "indexer"
	Supervisor Boundary = "supervisor"
	Runtime    Boundary = "runtime"
	CLI        Boundary = "cli"
	E2E        Boundary = "e2e"
)

type Category string

const (
	Auth             Category = "auth"
	RateLimit        Category = "rate_limit"
	NotFound         Category = "not_found"
	Conflict         Category = "conflict"
	InvalidArgument  Category = "invalid_argument"
	Unavailable      Category = "unavailable"
	ContextOverflow  Category = "context_overflow"
	Transport        Category = "transport"
	ApprovalRequired Category = "approval_required"
	PolicyDenied     Category = "policy_denied"
	Deadline         Category = "deadline"
	Canceled         Category = "canceled"
	Internal         Category = "internal"
	Unknown          Category = "unknown"
)

type Error struct {
	Boundary   Boundary
	Category   Category
	Operation  string
	StatusCode int
	Message    string
	Err        error
}

func New(boundary Boundary, category Category, operation, message string) *Error {
	return &Error{
		Boundary:  boundary,
		Category:  category,
		Operation: strings.TrimSpace(operation),
		Message:   strings.TrimSpace(message),
	}
}

func Wrap(boundary Boundary, category Category, operation string, err error) *Error {
	if err == nil {
		return nil
	}
	return &Error{
		Boundary:  boundary,
		Category:  category,
		Operation: strings.TrimSpace(operation),
		Message:   err.Error(),
		Err:       err,
	}
}

func FromError(boundary Boundary, operation string, err error) *Error {
	if err == nil {
		return nil
	}
	var existing *Error
	if errors.As(err, &existing) {
		return existing
	}
	var providerErr *plugin.ProviderError
	if errors.As(err, &providerErr) {
		return &Error{
			Boundary:   Provider,
			Category:   categoryFromProvider(providerErr.Category),
			Operation:  strings.TrimSpace(operation),
			StatusCode: providerErr.StatusCode,
			Message:    providerErr.Error(),
			Err:        err,
		}
	}
	return Wrap(boundary, Unknown, operation, err)
}

func FromHTTPStatus(boundary Boundary, operation string, statusCode int, message string) *Error {
	return &Error{
		Boundary:   boundary,
		Category:   categoryFromHTTPStatus(statusCode),
		Operation:  strings.TrimSpace(operation),
		StatusCode: statusCode,
		Message:    strings.TrimSpace(message),
	}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	parts := []string{string(e.Boundary), string(e.Category)}
	if e.Operation != "" {
		parts = append(parts, "operation="+e.Operation)
	}
	if e.StatusCode != 0 {
		parts = append(parts, fmt.Sprintf("status=%d", e.StatusCode))
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	return strings.Join(parts, ": ")
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func IsCategory(err error, category Category) bool {
	var boundaryErr *Error
	return errors.As(err, &boundaryErr) && boundaryErr.Category == category
}

func StreamPayload(err error, boundary Boundary, operation string) map[string]any {
	boundaryErr := FromError(boundary, operation, err)
	message := ""
	if err != nil {
		message = err.Error()
	}
	if boundaryErr != nil && boundaryErr.Message != "" {
		message = boundaryErr.Message
	}
	diagnostic := DiagnosticFromError(err, boundary, operation)
	return map[string]any{
		"message":    message,
		"boundary":   string(boundaryErr.Boundary),
		"category":   string(boundaryErr.Category),
		"operation":  boundaryErr.Operation,
		"diagnostic": diagnostic,
	}
}

func categoryFromProvider(category plugin.ProviderErrorCategory) Category {
	switch category {
	case plugin.ProviderErrorAuth:
		return Auth
	case plugin.ProviderErrorRateLimit:
		return RateLimit
	case plugin.ProviderErrorModelUnavailable:
		return Unavailable
	case plugin.ProviderErrorContextOverflow:
		return ContextOverflow
	case plugin.ProviderErrorTransport:
		return Transport
	case plugin.ProviderErrorInvalidRequest:
		return InvalidArgument
	default:
		return Unknown
	}
}

func categoryFromHTTPStatus(statusCode int) Category {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return Auth
	case http.StatusNotFound:
		return NotFound
	case http.StatusConflict:
		return Conflict
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return InvalidArgument
	case http.StatusRequestEntityTooLarge:
		return ContextOverflow
	case http.StatusTooManyRequests:
		return RateLimit
	case http.StatusGatewayTimeout, http.StatusRequestTimeout:
		return Deadline
	default:
		if statusCode >= http.StatusInternalServerError {
			return Unavailable
		}
		return Unknown
	}
}
