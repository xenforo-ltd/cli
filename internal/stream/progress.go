// Package stream provides utilities for tracking download and streaming progress.
package stream

import (
	"errors"
	"fmt"
	"io"
)

// ProgressReader tracks bytes read from Reader and calls OnProgress after each
// nonempty read with the cumulative count and Total, when the callback is set.
type ProgressReader struct {
	Reader     io.Reader
	Total      int64
	OnProgress func(downloaded, total int64)

	downloaded int64
}

// Read reads from the source and reports progress; it preserves EOF and wraps
// other read errors.
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	if n > 0 {
		pr.downloaded += int64(n)
		if pr.OnProgress != nil {
			pr.OnProgress(pr.downloaded, pr.Total)
		}
	}

	if err != nil {
		if errors.Is(err, io.EOF) {
			return n, io.EOF
		}

		return n, fmt.Errorf("failed to read progress stream: %w", err)
	}

	return n, nil
}
