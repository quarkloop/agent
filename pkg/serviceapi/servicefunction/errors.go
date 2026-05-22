package servicefunction

import (
	"context"
	"errors"
	"strings"

	"github.com/quarkloop/pkg/boundary"
)

func BoundaryError(err error, defaultBoundary boundary.Boundary, operation string) *boundary.Error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return boundary.Wrap(defaultBoundary, boundary.Deadline, operation, err)
	}
	if errors.Is(err, context.Canceled) {
		return boundary.Wrap(defaultBoundary, boundary.Canceled, operation, err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "no responders") {
		return boundary.Wrap(boundary.Service, boundary.Unavailable, operation, err)
	}
	return boundary.FromError(defaultBoundary, operation, err)
}

func ErrorPayloadFromError(err error, defaultBoundary boundary.Boundary, operation string) ErrorPayload {
	boundaryErr := BoundaryError(err, defaultBoundary, operation)
	if boundaryErr == nil {
		return ErrorPayload{}
	}
	return ErrorPayload{
		Boundary:  boundaryErr.Boundary,
		Category:  boundaryErr.Category,
		Operation: boundaryErr.Operation,
		Message:   boundaryErr.Message,
	}
}
