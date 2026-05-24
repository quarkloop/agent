package runstatesvc

import "errors"

var (
	errInvalidArgument = errors.New("invalid run state request")
	errNotFound        = errors.New("run state record not found")
	errConflict        = errors.New("run state conflict")
)
