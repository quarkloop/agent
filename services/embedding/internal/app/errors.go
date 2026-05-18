package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type ErrorCategory string

const (
	CategoryAuth               ErrorCategory = "auth"
	CategoryQuota              ErrorCategory = "quota"
	CategoryModelUnavailable   ErrorCategory = "model_unavailable"
	CategoryInvalidRequest     ErrorCategory = "invalid_request"
	CategoryDimensionMismatch  ErrorCategory = "dimension_mismatch"
	CategoryTransport          ErrorCategory = "transport"
	CategoryInvalidConfig      ErrorCategory = "invalid_config"
	CategoryProviderResponse   ErrorCategory = "provider_response"
	CategoryProvidersExhausted ErrorCategory = "providers_exhausted"
)

type ProviderError struct {
	Category   ErrorCategory
	Provider   string
	Model      string
	StatusCode int
	Err        error
}

func (e *ProviderError) Error() string {
	parts := []string{string(e.Category)}
	if e.Provider != "" {
		parts = append(parts, "provider="+e.Provider)
	}
	if e.Model != "" {
		parts = append(parts, "model="+e.Model)
	}
	if e.StatusCode != 0 {
		parts = append(parts, fmt.Sprintf("status=%d", e.StatusCode))
	}
	if e.Err != nil {
		parts = append(parts, e.Err.Error())
	}
	return strings.Join(parts, ": ")
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

func providerError(category ErrorCategory, provider, model string, statusCode int, err error) *ProviderError {
	return &ProviderError{
		Category:   category,
		Provider:   provider,
		Model:      strings.TrimSpace(model),
		StatusCode: statusCode,
		Err:        err,
	}
}

func fallbackAllowed(err error) bool {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return false
	}
	switch providerErr.Category {
	case CategoryAuth, CategoryQuota, CategoryModelUnavailable, CategoryTransport:
		return true
	default:
		return false
	}
}

func categoryForHTTPStatus(statusCode int) ErrorCategory {
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return CategoryAuth
	case statusCode == http.StatusTooManyRequests:
		return CategoryQuota
	case statusCode == http.StatusNotFound || statusCode == http.StatusGone:
		return CategoryModelUnavailable
	case statusCode == http.StatusBadRequest:
		return CategoryInvalidRequest
	case statusCode >= http.StatusInternalServerError:
		return CategoryTransport
	default:
		return CategoryProviderResponse
	}
}
