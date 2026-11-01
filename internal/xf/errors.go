package xf

import "errors"

var (
	// errInvalidInput indicates invalid input was provided.
	errInvalidInput = errors.New("invalid input")

	// ErrMetadataNotFound is returned when the metadata file cannot be found.
	ErrMetadataNotFound = errors.New("metadata not found")
)
