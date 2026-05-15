package plugin

import (
	"fmt"
	"net/http"
	"strings"
)

type ProviderErrorCategory string

const (
	ProviderErrorAuth             ProviderErrorCategory = "auth"
	ProviderErrorRateLimit        ProviderErrorCategory = "rate_limit"
	ProviderErrorModelUnavailable ProviderErrorCategory = "model_unavailable"
	ProviderErrorContextOverflow  ProviderErrorCategory = "context_overflow"
	ProviderErrorTransport        ProviderErrorCategory = "transport"
	ProviderErrorInvalidRequest   ProviderErrorCategory = "invalid_request"
	ProviderErrorResponse         ProviderErrorCategory = "provider_response"
	ProviderErrorExhausted        ProviderErrorCategory = "providers_exhausted"
)

type ProviderError struct {
	Category   ProviderErrorCategory
	Provider   string
	Model      string
	StatusCode int
	ResetAt    string
	Err        error
}

func NewProviderError(category ProviderErrorCategory, provider, model string, statusCode int, err error) *ProviderError {
	return &ProviderError{
		Category:   category,
		Provider:   strings.TrimSpace(provider),
		Model:      strings.TrimSpace(model),
		StatusCode: statusCode,
		Err:        err,
	}
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
	if e.ResetAt != "" {
		parts = append(parts, "reset_at="+e.ResetAt)
	}
	if e.Err != nil {
		parts = append(parts, e.Err.Error())
	}
	return strings.Join(parts, ": ")
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

func ProviderErrorCategoryForHTTPStatus(statusCode int) ProviderErrorCategory {
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return ProviderErrorAuth
	case statusCode == http.StatusTooManyRequests:
		return ProviderErrorRateLimit
	case statusCode == http.StatusNotFound || statusCode == http.StatusGone:
		return ProviderErrorModelUnavailable
	case statusCode == http.StatusRequestEntityTooLarge:
		return ProviderErrorContextOverflow
	case statusCode >= http.StatusInternalServerError:
		return ProviderErrorTransport
	case statusCode >= http.StatusBadRequest:
		return ProviderErrorInvalidRequest
	default:
		return ProviderErrorResponse
	}
}
