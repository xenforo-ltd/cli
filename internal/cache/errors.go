package cache

import "errors"

var (
	// errCacheMiss is returned when a requested cache entry does not exist.
	errCacheMiss = errors.New("cache miss")

	// errChecksumMismatch indicates a checksum verification failure.
	errChecksumMismatch = errors.New("checksum mismatch")

	// errInvalidInput indicates invalid input to a cache operation.
	errInvalidInput = errors.New("invalid input")

	// errAuthExpired indicates the authentication token has expired.
	errAuthExpired = errors.New("authentication expired")

	// errDownloadFailed indicates a download operation failed.
	errDownloadFailed = errors.New("download failed")
)
