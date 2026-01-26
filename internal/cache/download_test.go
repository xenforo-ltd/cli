package cache

import (
	"net/http"
	"testing"
)

func TestParseFilenameFromResponse(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{
			"Content-Disposition": []string{`attachment; filename="../../../evil.zip"`},
		},
	}

	name := parseFilenameFromResponse(resp, "https://example.com/downloads/../../../evil.zip")
	if name != "evil.zip" {
		t.Fatalf("expected sanitized filename, got %q", name)
	}
}
