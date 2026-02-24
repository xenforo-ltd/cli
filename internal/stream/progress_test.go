package stream

import (
	"errors"
	"io"
	"strings"
	"testing"
)

var errReadFailed = errors.New("read failed")

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errReadFailed
}

func TestProgressReaderReportsProgress(t *testing.T) {
	var calls int
	var gotDownloaded int64
	var gotTotal int64

	r := &ProgressReader{
		Reader: strings.NewReader("hello"),
		Total:  5,
		OnProgress: func(downloaded, total int64) {
			calls++
			gotDownloaded = downloaded
			gotTotal = total
		},
	}

	buf := make([]byte, 3)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if n != 3 {
		t.Fatalf("n = %d, want 3", n)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if gotDownloaded != 3 || gotTotal != 5 {
		t.Fatalf("progress = (%d,%d), want (3,5)", gotDownloaded, gotTotal)
	}
}

func TestProgressReaderNoProgressOnZeroBytes(t *testing.T) {
	var calls int

	r := &ProgressReader{
		Reader: errReader{},
		Total:  100,
		OnProgress: func(downloaded, total int64) {
			calls++
		},
	}

	buf := make([]byte, 4)
	_, err := r.Read(buf)
	if err == nil {
		t.Fatal("expected read error")
	}
	if calls != 0 {
		t.Fatalf("calls = %d, want 0", calls)
	}
}

func TestProgressReaderEOFStillReturnsData(t *testing.T) {
	var downloaded int64

	r := &ProgressReader{
		Reader: strings.NewReader("x"),
		Total:  1,
		OnProgress: func(d, _ int64) {
			downloaded = d
		},
	}

	buf := make([]byte, 8)
	n, err := io.ReadFull(r, buf[:1])
	if err != nil {
		t.Fatalf("ReadFull failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("n = %d, want 1", n)
	}
	if downloaded != 1 {
		t.Fatalf("downloaded = %d, want 1", downloaded)
	}
}
