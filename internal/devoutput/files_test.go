package devoutput_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xenforo-ltd/cli/internal/devoutput"
)

// TestExportInvalidAddOn checks missing add-on directories and file paths.
func TestExportInvalidAddOn(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"missing", "file"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if kind == "file" {
				writeFile(t, root, "addon", "not a directory")
			}
			result, err := devoutput.Export(t.Context(), filepath.Join(root, "addon"))
			requireError(t, err, "opening add-on directory")
			if kind == "missing" && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("error = %v, want missing directory", err)
			}
			if result != nil {
				t.Fatalf("failed export returned result: %+v", result)
			}
		})
	}
}

// TestExportInvalidPaths checks unsupported types, invalid data directories,
// missing metadata, and missing or non-regular listed source files.
func TestExportInvalidPaths(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		files   map[string]string
		dirs    []string
		want    string
		missing bool
	}{
		{"missing output", nil, nil, "reading development output", true},
		{"output is a file", map[string]string{"_output": "bad"}, nil, "reading development output", false},
		{"unsupported type", nil, []string{"_output/custom"}, "unsupported development-output type", false},
		{"unsupported source file", map[string]string{"_output/custom.json": "{}"}, nil, "unsupported development-output type or file", false},
		{"type is a file", map[string]string{"_output/api_scopes": "bad"}, nil, "unsupported development-output type or file", false},
		{"special file is a directory", nil, []string{"_output/extension_hint.php"}, "unsupported development-output type or file", false},
		{"data is a file", map[string]string{"_data": "bad"}, nil, "_data must be a directory", false},
		{"unsupported release type", map[string]string{"_data/custom.xml": "previous data"}, nil, "unsupported release data type", false},
		{"missing metadata", map[string]string{"_output/api_scopes/demo.json": "{}"}, nil, "reading _output/api_scopes metadata", true},
		{"missing nested metadata", map[string]string{"_output/templates/public/demo.html": "text"}, nil, "reading _output/templates metadata", true},
		{"legacy metadata", map[string]string{"_output/api_scopes/_metadata.json": "{}"}, nil, "reading _output/api_scopes metadata", true},
		{"metadata is a directory", nil, []string{"_output/api_scopes/_metadata.jsonl"}, "reading _output/api_scopes metadata", false},
		{"missing listed file", map[string]string{"_output/api_scopes/_metadata.jsonl": `{"_file":"missing.json","hash":"ignored"}`}, nil, "reading _output/api_scopes/missing.json", true},
		{"listed directory", map[string]string{"_output/api_scopes/_metadata.jsonl": `{"_file":"directory","hash":"ignored"}`}, []string{"_output/api_scopes/directory"}, "is not a regular file", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := newFixture(t)
			if tc.name == "missing output" || tc.name == "output is a file" {
				if err := os.Remove(filepath.Join(root, "_output")); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range tc.dirs {
				if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(name)), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for name, contents := range tc.files {
				writeFile(t, root, name, contents)
			}
			result, err := devoutput.Export(t.Context(), root)
			requireError(t, err, tc.want)
			if tc.missing && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("error = %v, want missing file or directory", err)
			}
			if result != nil {
				t.Fatalf("failed export returned result: %+v", result)
			}
		})
	}
}

// TestExportOutputFailure checks failed atomic replacement, temporary-file
// cleanup, and preservation of the failed destination and later files.
func TestExportOutputFailure(t *testing.T) {
	t.Parallel()
	for _, container := range []string{"activity_summary_definitions", "options"} {
		t.Run(container, func(t *testing.T) {
			t.Parallel()
			root := newFixture(t)
			name := "_data/" + container + ".xml"
			if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(name)), 0o755); err != nil {
				t.Fatal(err)
			}
			writeFile(t, root, name+"/keep.txt", "keep")
			writeFile(t, root, "_data/widget_positions.xml", "later data")
			result, err := devoutput.Export(t.Context(), root)
			requireError(t, err, "writing "+container+".xml")
			if result != nil {
				t.Fatalf("failed export returned result: %+v", result)
			}
			if got := readFile(t, root, name+"/keep.txt"); got != "keep" {
				t.Fatal("failed replacement changed its destination")
			}
			if got := readFile(t, root, "_data/widget_positions.xml"); got != "later data" {
				t.Fatal("export continued after a write failure")
			}
			matches, err := filepath.Glob(filepath.Join(root, "_data", ".export-*"))
			if err != nil || len(matches) != 0 {
				t.Fatalf("temporary outputs remain: %v (%v)", matches, err)
			}
			if container == "options" {
				requireXML(t, root, "activity_summary_definitions", `<activity_summary_definitions></activity_summary_definitions>`)
			}
		})
	}
}

// TestExportPermissions checks propagation of file-system errors without
// relying on fault injection. Permission checks require a non-root Unix user.
func TestExportPermissions(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix file permissions and a non-root user")
	}
	for _, tc := range []struct {
		name string
		path string
		mode os.FileMode
		want string
	}{
		{"unreadable output", "_output", 0o000, "reading development output"},
		{"unreadable type", "_output/api_scopes", 0o000, "opening _output/api_scopes"},
		{"unreadable metadata", "_output/api_scopes/_metadata.jsonl", 0o000, "reading _output/api_scopes metadata"},
		{"unreadable source", "_output/api_scopes/demo.json", 0o000, "reading _output/api_scopes/demo.json"},
		{"unreadable data", "_data", 0o000, "_data"},
		{"unwritable data", "_data", 0o555, "writing activity_summary_definitions.xml"},
		{"unwritable add-on", ".", 0o555, "_data"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := newFixture(t)
			writeOutput(t, root, "api_scopes", sourceFile{name: "demo.json", content: `{}`})
			if tc.path == "_data" {
				if err := os.Mkdir(filepath.Join(root, "_data"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			name := filepath.Join(root, filepath.FromSlash(tc.path))
			info, err := os.Stat(name)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.Chmod(name, info.Mode().Perm()); err != nil {
					t.Error(err)
				}
			})
			if err := os.Chmod(name, tc.mode); err != nil {
				t.Fatal(err)
			}
			result, err := devoutput.Export(t.Context(), root)
			requireError(t, err, tc.want)
			if !errors.Is(err, os.ErrPermission) || result != nil {
				t.Fatalf("result = %+v, error = %v, want permission error without a result", result, err)
			}
		})
	}
}

// TestExportRejectsSymlinks checks that type and data directories and listed
// source files cannot be links, and metadata cannot escape its type directory.
func TestExportRejectsSymlinks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		want string
	}{
		{"source", "is not a regular file"},
		{"broken source", "is not a regular file"},
		{"escaping source", "is not a regular file"},
		{"type directory", "unsupported development-output type or file"},
		{"data directory", "_data must be a directory"},
		{"broken metadata", "reading _output/api_scopes metadata"},
		{"escaping metadata", "reading _output/api_scopes metadata"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := newFixture(t)
			writeOutput(t, root, "api_scopes", sourceFile{name: "demo.json", content: `{}`})
			name := filepath.Join(root, "_output", "api_scopes", "demo.json")
			target := "target.json"
			switch tc.name {
			case "source":
				writeFile(t, root, "_output/api_scopes/target.json", "{}")
			case "escaping source":
				outside := t.TempDir()
				writeFile(t, outside, "target.json", "{}")
				target = filepath.Join(outside, "target.json")
			case "type directory":
				name = filepath.Join(root, "_output", "api_scopes")
				if err := os.Rename(name, filepath.Join(root, "_output", ".original")); err != nil {
					t.Fatal(err)
				}
				target = ".original"
			case "data directory":
				name = filepath.Join(root, "_data")
				target = t.TempDir()
			case "broken metadata", "escaping metadata":
				name = filepath.Join(root, "_output", "api_scopes", "_metadata.jsonl")
				target = "missing.jsonl"
				if tc.name == "escaping metadata" {
					outside := t.TempDir()
					writeFile(t, outside, "metadata.jsonl", readFile(t, root, "_output/api_scopes/_metadata.jsonl"))
					target = filepath.Join(outside, "metadata.jsonl")
				}
			}
			if tc.name != "type directory" && tc.name != "data directory" {
				if err := os.Remove(name); err != nil {
					t.Fatal(err)
				}
			}
			symlink(t, target, name)
			result, err := devoutput.Export(t.Context(), root)
			requireError(t, err, tc.want)
			if result != nil {
				t.Fatalf("failed export returned result: %+v", result)
			}
		})
	}
}

// TestExportContainedMetadataSymlink checks metadata links within a type directory.
func TestExportContainedMetadataSymlink(t *testing.T) {
	t.Parallel()
	root := newFixture(t)
	writeOutput(t, root, "api_scopes", sourceFile{name: "demo.json", content: `{}`})
	metadata := filepath.Join(root, "_output", "api_scopes", "_metadata.jsonl")
	if err := os.Rename(metadata, filepath.Join(filepath.Dir(metadata), "metadata.jsonl")); err != nil {
		t.Fatal(err)
	}
	symlink(t, "metadata.jsonl", metadata)
	if _, err := devoutput.Export(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	requireXML(t, root, "api_scopes", `<api_scopes><api_scope api_scope_id="demo"></api_scope></api_scopes>`)
}

// TestExportSymlinkedAddOn checks a symlinked add-on path and its result directory.
func TestExportSymlinkedAddOn(t *testing.T) {
	t.Parallel()
	root := newFixture(t)
	alias := filepath.Join(t.TempDir(), "addon")
	symlink(t, root, alias)
	result, err := devoutput.Export(t.Context(), alias)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(alias, "_data"); result.Directory != want {
		t.Fatalf("Directory = %q, want %q", result.Directory, want)
	}
	requireXML(t, alias, "api_scopes", `<api_scopes></api_scopes>`)
}

// TestExportReplacesOutputSymlink checks that atomic publication replaces an
// existing output link without modifying its target.
func TestExportReplacesOutputSymlink(t *testing.T) {
	t.Parallel()
	root := newFixture(t)
	if err := os.Mkdir(filepath.Join(root, "_data"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	writeFile(t, outside, "original.xml", "keep")
	name := filepath.Join(root, "_data", "api_scopes.xml")
	symlink(t, filepath.Join(outside, "original.xml"), name)
	if _, err := devoutput.Export(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, outside, "original.xml"); got != "keep" {
		t.Fatal("export changed the output link's target")
	}
	info, err := os.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("output mode = %v, want a regular file", info.Mode())
	}
	requireXML(t, root, "api_scopes", `<api_scopes></api_scopes>`)
}
