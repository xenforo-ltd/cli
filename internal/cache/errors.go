package cache

import "errors"

var (
	// ErrCacheMiss is returned when a requested cache entry does not exist.
	ErrCacheMiss = errors.New("cache miss")

	// ErrChecksumMismatch indicates a checksum verification failure.
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrInvalidInput indicates invalid input to a cache operation.
	ErrInvalidInput = errors.New("invalid input")

	// ErrAuthExpired indicates the authentication token has expired.
	ErrAuthExpired = errors.New("authentication expired")

	// ErrDownloadFailed indicates a download operation failed.
	ErrDownloadFailed = errors.New("download failed")
)
