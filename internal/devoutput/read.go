package devoutput

import (
	"bytes"
	"cmp"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// readMetadata reads _metadata.jsonl from a type directory and indexes its records
// by filename. A missing metadata file is allowed only if root contains no files,
// including in subdirectories.
func readMetadata(ctx context.Context, root *os.Root) (map[string]record, error) {
	filename := "_metadata.jsonl"
	data, err := root.ReadFile(filename)
	if errors.Is(err, fs.ErrNotExist) {
		missing := err
		err = fs.WalkDir(root.FS(), ".", func(_ string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if !entry.IsDir() {
				return missing
			}
			return nil
		})
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%s contains invalid UTF-8", filename)
	}
	metadata, err := ndjsonMetadata(data)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", filename, err)
	}
	return metadata, nil
}

// ndjsonMetadata decodes JSON metadata records indexed by their _file names.
// It rejects missing, invalid, or duplicate names and removes _file from each record.
func ndjsonMetadata(data []byte) (map[string]record, error) {
	result := make(map[string]record)
	d := json.NewDecoder(bytes.NewReader(data))
	for {
		var r record
		if err := d.Decode(&r); err == io.EOF {
			return result, nil
		} else if err != nil {
			return nil, err
		}
		var name string
		if err := json.Unmarshal(r["_file"], &name); err != nil || name == "" {
			return nil, errors.New("metadata record has no filename")
		}
		if !fs.ValidPath(name) || strings.ContainsAny(name, `\:`) {
			return nil, fmt.Errorf("invalid metadata filename %q", name)
		}
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("duplicate metadata for %s", name)
		}
		delete(r, "_file")
		result[name] = r
	}
}

// read loads and converts the metadata-listed files for d beneath root/_output.
// It validates known fields, collects warnings for unknown fields, and returns
// elements in the definition's sort order, using keys when no order is defined.
func (d definition) read(ctx context.Context, root *os.Root, warnings map[string]bool) ([]xmlElement, error) {
	directory := filepath.Join("_output", d.directory)
	source, err := root.OpenRoot(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", directory, err)
	}
	defer source.Close()
	metadata, err := readMetadata(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("reading %s metadata: %w", directory, err)
	}
	order := d.order
	if len(order) == 0 {
		order = d.keys
	}
	type sortValue struct {
		text   string
		number int64
	}
	type row struct {
		name   string
		values []sortValue
		node   xmlElement
	}
	rows := make([]row, 0, len(metadata))
	allowed := make(map[string]bool)
	d.element.sources(allowed)
	if d.suffix != "" {
		allowed["hash"] = true
	}
	for _, key := range slices.Concat(d.keys, d.ignored) {
		allowed[key] = true
	}
	for name, meta := range metadata {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		file := filepath.FromSlash(name)
		filename := filepath.Join(directory, file)
		info, err := source.Lstat(file)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", filename, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a regular file", filename)
		}
		data, err := source.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", filename, err)
		}
		if !utf8.Valid(data) {
			return nil, fmt.Errorf("%s contains invalid UTF-8", filename)
		}
		r, err := d.record(name, data, meta)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", filename, err)
		}
		for key := range r {
			if !allowed[key] {
				warnings[fmt.Sprintf("%s: ignoring unknown field %q", d.directory, key)] = true
			}
		}
		node, err := d.element.convert(r)
		if err != nil {
			return nil, fmt.Errorf("converting %s: %w", filename, err)
		}
		if node == nil {
			continue
		}
		values := make([]sortValue, len(order))
		for i, key := range order {
			value, _, err := (field{source: key}).value(r)
			if err != nil {
				return nil, fmt.Errorf("sorting %s: %w", filename, err)
			}
			values[i].text = value
			if slices.Contains(d.numeric, key) {
				number, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("%s: invalid numeric %s: %w", filename, key, err)
				}
				values[i].number = number
			}
		}
		rows = append(rows, row{name: name, values: values, node: *node})
	}
	slices.SortFunc(rows, func(a, b row) int {
		for i, key := range order {
			var comparison int
			if slices.Contains(d.numeric, key) {
				comparison = cmp.Compare(a.values[i].number, b.values[i].number)
			} else {
				comparison = strings.Compare(a.values[i].text, b.values[i].text)
			}
			if comparison != 0 {
				return comparison
			}
		}
		return strings.Compare(a.name, b.name)
	})
	nodes := make([]xmlElement, 0, len(rows))
	for _, row := range rows {
		nodes = append(nodes, row.node)
	}
	return nodes, nil
}

// record verifies the content hash and builds an export record from a file.
// JSON files supply the record fields; text files use metadata and store their
// contents as $text. Missing keys are derived from the filename.
func (d definition) record(name string, contents []byte, metadata record) (record, error) {
	normalized := contents
	if bytes.IndexByte(contents, '\r') >= 0 {
		normalized = bytes.ReplaceAll(contents, []byte{'\r'}, nil)
	}
	digest := md5.Sum(normalized)
	hash, _ := scalarValue(metadata["hash"])
	if hash != hex.EncodeToString(digest[:]) {
		return nil, errors.New("stale metadata; prepare development output with XenForo before exporting")
	}
	var r record
	stem := strings.TrimSuffix(name, ".json")
	if d.suffix != "" {
		stem = strings.TrimSuffix(name, d.suffix)
		r = make(record, len(metadata)+1)
		for key, value := range metadata {
			if slices.Contains(d.keys, key) {
				return nil, fmt.Errorf("unsupported metadata field %q", key)
			}
			r[key] = value
		}
		r["$text"], _ = json.Marshal(string(contents))
	} else {
		if !strings.HasSuffix(name, ".json") {
			return nil, errors.New("expected a JSON file")
		}
		if err := json.Unmarshal(contents, &r); err != nil {
			return nil, err
		}
		if r == nil {
			return nil, errors.New("expected a JSON object")
		}
	}
	if len(d.keys) > 0 {
		parts := []string{stem}
		if len(d.keys) > 1 {
			parts = strings.SplitN(stem, d.separator, len(d.keys))
		}
		if len(parts) != len(d.keys) {
			return nil, fmt.Errorf("filename %q must contain %s", name, strings.Join(d.keys, " and "))
		}
		for i, key := range d.keys {
			if _, exists := r[key]; !exists {
				r[key], _ = json.Marshal(parts[i])
			}
		}
	}
	return r, nil
}

// sources adds the input field names used by e and its children to result.
// Repeated children contribute their collection field rather than per-entry fields.
func (e element) sources(result map[string]bool) {
	for _, f := range e.attributes {
		result[f.source] = true
	}
	if e.content.source != "" {
		result[e.content.source] = true
	}
	for _, child := range e.children {
		if child.repeat != "" {
			result[child.repeat] = true
		} else {
			child.sources(result)
		}
	}
}
