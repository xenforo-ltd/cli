package xf

import "errors"

var (
	// ErrInvalidInput indicates invalid input was provided.
	ErrInvalidInput = errors.New("invalid input")

	// ErrMetadataNotFound is returned when the metadata file cannot be found.
	ErrMetadataNotFound = errors.New("metadata not found")
)
