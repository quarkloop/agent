package serviceerrors

import (
	"fmt"

	"github.com/quarkloop/pkg/boundary"
)

func New(category boundary.Category, message string) error {
	return boundary.New(boundary.Service, category, "", message)
}

func Newf(category boundary.Category, format string, args ...any) error {
	return New(category, fmt.Sprintf(format, args...))
}

func Wrap(category boundary.Category, err error) error {
	return boundary.Wrap(boundary.Service, category, "", err)
}

func InvalidArgument(message string) error { return New(boundary.InvalidArgument, message) }

func InvalidArgumentf(format string, args ...any) error {
	return Newf(boundary.InvalidArgument, format, args...)
}

func NotFound(message string) error { return New(boundary.NotFound, message) }

func NotFoundf(format string, args ...any) error {
	return Newf(boundary.NotFound, format, args...)
}

func FailedPrecondition(message string) error {
	return New(boundary.Conflict, message)
}

func FailedPreconditionf(format string, args ...any) error {
	return Newf(boundary.Conflict, format, args...)
}

func PermissionDenied(message string) error { return New(boundary.PolicyDenied, message) }

func Auth(message string) error { return New(boundary.Auth, message) }

func RateLimit(message string) error { return New(boundary.RateLimit, message) }

func ContextOverflow(message string) error { return New(boundary.ContextOverflow, message) }

func Internal(message string) error { return New(boundary.Internal, message) }

func Internalf(format string, args ...any) error {
	return Newf(boundary.Internal, format, args...)
}

func Unavailable(message string) error { return New(boundary.Unavailable, message) }

func Unimplemented(message string) error { return New(boundary.Unavailable, message) }

func AlreadyExists(message string) error { return New(boundary.Conflict, message) }

func Canceled(message string) error { return New(boundary.Canceled, message) }

func DeadlineExceeded(message string) error { return New(boundary.Deadline, message) }
