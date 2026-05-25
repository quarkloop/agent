package space

import (
	"errors"
	"fmt"
)

// ErrAlreadyExists indicates a conflicting space or space-owned semantic record.
var ErrAlreadyExists = errors.New("space already exists")

// NotFoundError indicates a missing space or space-owned semantic record.
type NotFoundError struct {
	ID string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("space record %q not found", e.ID)
}

func NewNotFoundError(id string) *NotFoundError {
	return &NotFoundError{ID: id}
}

func IsNotFound(err error) bool {
	var target *NotFoundError
	return errors.As(err, &target)
}
