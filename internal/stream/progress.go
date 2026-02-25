// Package stream provides utilities for tracking download and streaming progress.
package stream

import "io"

// ProgressReader tracks read progress from a source reader.
type ProgressReader struct {
	Reader     io.Reader
	Total      int64
	OnProgress func(downloaded, total int64)

	downloaded int64
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	if n > 0 {
		pr.downloaded += int64(n)
		if pr.OnProgress != nil {
			pr.OnProgress(pr.downloaded, pr.Total)
		}
	}

	return n, err
}
