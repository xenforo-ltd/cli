package devoutput_test

import (
	"context"
	"crypto/md5"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/devoutput"
)

func write(t *testing.T, root, name, content string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExportPhrases(t *testing.T) {
	root := t.TempDir()
	text := "Hello <world> & everyone\n"
	write(t, root, "_output/phrases/greeting.txt", text)
	write(t, root, "_output/phrases/unlisted.txt", "not exported")
	write(t, root, "_output/phrases/_metadata.jsonl", fmt.Sprintf(`{"_file":"greeting.txt","hash":"%x","version_id":102,"version_string":"1.2","global_cache":true,"custom_field":"ignored"}`, md5.Sum([]byte(text))))
	result, err := devoutput.Export(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	warnings := strings.Join(result.Warnings, "\n")
	if !strings.Contains(warnings, "phrases") || !strings.Contains(warnings, "custom_field") {
		t.Fatalf("warnings: %v", result.Warnings)
	}
	data, err := os.ReadFile(filepath.Join(root, "_data/phrases.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var phrases struct {
		XMLName xml.Name `xml:"phrases"`
		Entries []struct {
			Title     string `xml:"title,attr"`
			Version   string `xml:"version_string,attr"`
			VersionID int    `xml:"version_id,attr"`
			Cache     int    `xml:"global_cache,attr"`
			Text      string `xml:",chardata"`
		} `xml:"phrase"`
	}
	if err := xml.Unmarshal(data, &phrases); err != nil {
		t.Fatal(err)
	}
	if len(phrases.Entries) != 1 {
		t.Fatalf("phrases: %s", data)
	}
	p := phrases.Entries[0]
	if p.Title != "greeting" || p.Version != "1.2" || p.VersionID != 102 || p.Cache != 1 || p.Text != text {
		t.Fatalf("phrase: %+v", p)
	}
	data, err = os.ReadFile(filepath.Join(root, "_data/templates.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var empty struct {
		XMLName xml.Name   `xml:"templates"`
		Entries []struct{} `xml:"template"`
	}
	if err := xml.Unmarshal(data, &empty); err != nil || len(empty.Entries) != 0 {
		t.Fatalf("empty templates: %s, %v", data, err)
	}
}

func TestExportRejectsInvalidInputsBeforeWriting(t *testing.T) {
	tests := []struct{ name, path, data, message string }{
		{"unsupported type", "_output/custom/entry.json", `{}`, "unsupported"},
		{"missing metadata", "_output/phrases/greeting.txt", "hello", "metadata"},
		{"stale hash", "_output/phrases/_metadata.jsonl", `{"_file":"greeting.txt","hash":"wrong"}`, "stale metadata"},
		{"unsafe filename", "_output/phrases/_metadata.jsonl", `{"_file":"../secret.txt","hash":"x"}`, "filename"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, "_data/phrases.xml", "existing")
			if tc.name == "stale hash" {
				write(t, root, "_output/phrases/greeting.txt", "hello")
			}
			write(t, root, tc.path, tc.data)
			if _, err := devoutput.Export(t.Context(), root); err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("error: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(root, "_data/phrases.xml"))
			if err != nil || string(data) != "existing" {
				t.Fatalf("existing XML changed: %q, %v", data, err)
			}
		})
	}
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := devoutput.Export(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
			t.Fatalf("error: %v", err)
		}
	})
}
