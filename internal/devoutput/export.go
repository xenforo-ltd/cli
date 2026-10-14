// Package devoutput converts prepared XenForo development output into release XML.
package devoutput

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Result describes a completed export.
type Result struct {
	// Directory is the absolute path to the release data directory.
	Directory string

	// Files is the number of XML files written, including empty containers.
	Files int

	// Entries is the number of exported records across all XML files.
	Entries int

	// Warnings lists ignored unknown fields, sorted and deduplicated by data type
	// and field name. Intentionally omitted stock fields do not produce warnings.
	Warnings []string
}

// Export reads prepared development output from addOnDir/_output and writes
// release XML to addOnDir/_data, creating the destination directory if needed.
// Missing or empty type directories produce empty XML containers.
//
// Non-empty type directories require _metadata.jsonl. Only listed files are
// exported, and their contents must match the recorded hashes. Template and
// phrase versions are preserved. Unknown record fields are ignored and reported
// in Result.Warnings. Unsupported data types and export-handler class extensions
// are rejected; PHP handlers are not executed.
//
// Export converts all inputs before writing any files. Each XML file is replaced
// atomically, but an I/O error or cancellation may leave a partially written set.
func Export(ctx context.Context, addOnDir string) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(addOnDir)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, fmt.Errorf("opening add-on directory: %w", err)
	}
	defer root.Close()
	if err := checkTypes(root); err != nil {
		return nil, err
	}
	type output struct {
		name string
		data []byte
	}
	var outputs []output
	var entries int
	warnings := make(map[string]bool)
	for _, definition := range definitions {
		nodes, err := definition.read(ctx, root, warnings)
		if err != nil {
			return nil, err
		}
		data, err := render(definition.container, nodes)
		if err != nil {
			return nil, fmt.Errorf("encoding %s: %w", definition.directory, err)
		}
		outputs = append(outputs, output{name: definition.container + ".xml", data: data})
		entries += len(nodes)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := root.MkdirAll("_data", 0o755); err != nil {
		return nil, err
	}
	destination, err := root.OpenRoot("_data")
	if err != nil {
		return nil, err
	}
	defer destination.Close()
	for _, output := range outputs {
		if err := writeXML(ctx, destination, output.name, output.data); err != nil {
			return nil, fmt.Errorf("writing %s: %w", output.name, err)
		}
	}
	return &Result{
		Directory: filepath.Join(absolute, "_data"),
		Files:     len(outputs),
		Entries:   entries,
		Warnings:  slices.Sorted(maps.Keys(warnings)),
	}, nil
}

// checkTypes rejects unsupported _output entries and unknown release XML files.
// The _data directory may be absent but must not be a symlink.
func checkTypes(root *os.Root) error {
	known := make(map[string]bool, len(definitions))
	files := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		known[definition.directory] = true
		files[definition.container+".xml"] = true
	}
	entries, err := fs.ReadDir(root.FS(), "_output")
	if err != nil {
		return fmt.Errorf("reading development output: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if entry.IsDir() && known[name] {
			continue
		}
		if entry.Type().IsRegular() && (name == "extension_hint.php" || name == "option_hint.php" || name == "style_properties.less") {
			continue
		}
		return fmt.Errorf("unsupported development-output type or file %q", name)
	}
	info, err := root.Lstat("_data")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("_data must be a directory, not a file or symbolic link")
	}
	entries, err = fs.ReadDir(root.FS(), "_data")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".xml") && !files[entry.Name()] {
			return fmt.Errorf("unsupported release data type %q", entry.Name())
		}
	}
	return nil
}

// writeXML atomically replaces name within root with data and sets its mode to
// 0644. It checks for cancellation before publishing the file.
func writeXML(ctx context.Context, root *os.Root, name string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	temporary := ".export-" + rand.Text()
	f, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return root.Rename(temporary, name)
}
