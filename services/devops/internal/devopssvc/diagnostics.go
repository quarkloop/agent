package devopssvc

import (
	"context"
	"errors"

	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
)

func operationError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return serviceerrors.Canceled(err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return serviceerrors.DeadlineExceeded(err.Error())
	default:
		return serviceerrors.InvalidArgument(err.Error())
	}
}
