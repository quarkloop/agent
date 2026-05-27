package gatewaysvc

import (
	"errors"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
)

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

func providerServiceError(err error) error {
	if err == nil {
		return nil
	}
	var boundaryErr *boundary.Error
	if errors.As(err, &boundaryErr) {
		return err
	}
	var providerErr *plugin.ProviderError
	if errors.As(err, &providerErr) {
		switch providerErr.Category {
		case plugin.ProviderErrorAuth:
			return serviceerrors.Auth(providerErr.Error())
		case plugin.ProviderErrorRateLimit:
			return serviceerrors.RateLimit(providerErr.Error())
		case plugin.ProviderErrorModelUnavailable:
			return serviceerrors.NotFound(providerErr.Error())
		case plugin.ProviderErrorContextOverflow:
			return serviceerrors.ContextOverflow(providerErr.Error())
		case plugin.ProviderErrorInvalidRequest:
			return serviceerrors.InvalidArgument(providerErr.Error())
		default:
			return serviceerrors.Unavailable(providerErr.Error())
		}
	}
	return serviceerrors.Internal(err.Error())
}
