package modelservice

import (
	"errors"

	"github.com/quarkloop/pkg/plugin"
)

type failureInfo struct {
	category string
	resetAt  string
}

func providerFailureInfo(err error) failureInfo {
	var providerErr *plugin.ProviderError
	if errors.As(err, &providerErr) {
		return failureInfo{
			category: string(providerErr.Category),
			resetAt:  providerErr.ResetAt,
		}
	}
	return failureInfo{category: string(plugin.ProviderErrorResponse)}
}

func canFallbackAfter(err error) bool {
	var providerErr *plugin.ProviderError
	if !errors.As(err, &providerErr) {
		return false
	}
	switch providerErr.Category {
	case plugin.ProviderErrorAuth,
		plugin.ProviderErrorRateLimit,
		plugin.ProviderErrorModelUnavailable,
		plugin.ProviderErrorContextOverflow,
		plugin.ProviderErrorTransport:
		return true
	default:
		return false
	}
}

func canRetryAfter(err error) bool {
	var providerErr *plugin.ProviderError
	if !errors.As(err, &providerErr) {
		return false
	}
	return providerErr.Category == plugin.ProviderErrorTransport
}
