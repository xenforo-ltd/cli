package devoutput_test

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// sourceFile describes a development-output file and its metadata.
type sourceFile struct {
	name     string
	content  string
	metadata map[string]any
}

// newFixture creates an add-on directory with empty development output.
func newFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "_output"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// writeFile writes a fixture file, creating parent directories as needed.
func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readFile reads a fixture file, failing t on error.
func readFile(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// writeOutput writes a type's source files and prepared JSON Lines metadata.
// Explicit metadata fields may override the generated filename and hash.
func writeOutput(t *testing.T, root, directory string, files ...sourceFile) {
	t.Helper()
	var metadata bytes.Buffer
	for _, file := range files {
		writeFile(t, root, "_output/"+directory+"/"+file.name, file.content)
		digest := md5.Sum(bytes.ReplaceAll([]byte(file.content), []byte{'\r'}, nil))
		record := map[string]any{"_file": file.name, "hash": fmt.Sprintf("%x", digest)}
		maps.Copy(record, file.metadata)
		if err := json.NewEncoder(&metadata).Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, root, "_output/"+directory+"/_metadata.jsonl", metadata.String())
}

// requireError fails t unless err is non-nil and its message contains want.
func requireError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), filepath.FromSlash(want)) {
		t.Fatalf("error = %v, want containing %q", err, want)
	}
}

// requireMissing fails t unless name does not exist beneath root.
func requireMissing(t *testing.T, root, name string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); !os.IsNotExist(err) {
		t.Fatalf("%s: expected missing file, got %v", name, err)
	}
}

// symlink creates a test link, skipping t if symlinks are unavailable.
func symlink(t *testing.T, target, name string) {
	t.Helper()
	if err := os.Symlink(target, name); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

// xmlNode records XML names, ordered attributes, text, and children.
type xmlNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Text     string     `xml:",chardata"`
	Children []xmlNode  `xml:",any"`
}

// parseXML decodes XML, removing formatting whitespace from parent elements
// while preserving leaf text.
func parseXML(t *testing.T, data string) xmlNode {
	t.Helper()
	var node xmlNode
	if err := xml.Unmarshal([]byte(data), &node); err != nil {
		t.Fatalf("decoding XML: %v\n%s", err, data)
	}
	var trim func(*xmlNode)
	trim = func(node *xmlNode) {
		if len(node.Children) > 0 {
			node.Text = strings.TrimSpace(node.Text)
		}
		for i := range node.Children {
			trim(&node.Children[i])
		}
	}
	trim(&node)
	return node
}

// requireXML compares a release XML file with want, ignoring parent indentation.
func requireXML(t *testing.T, root, container, want string) {
	t.Helper()
	data := readFile(t, root, "_data/"+container+".xml")
	got := parseXML(t, data)
	expected := parseXML(t, want)
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("%s.xml =\n%s\nwant:\n%s", container, data, want)
	}
}
