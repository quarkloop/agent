package ingestionsvc

import "errors"

var (
	errInvalidArgument = errors.New("invalid ingestion request")
	errNotFound        = errors.New("ingestion record not found")
	errConflict        = errors.New("ingestion state conflict")
)
