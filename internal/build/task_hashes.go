package build

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
)

// hashesTask selects upload files and writes their hash manifest, applying
// hash-only path patterns. upload records the selection for ZIP creation.
type hashesTask struct {
	selection   uploadSelection
	destination string
	exclude     []filePattern
	upload      map[string]string
}

// run hashes selected upload files and writes the manifest to destination.
func (t *hashesTask) run(ctx context.Context, root *os.Root, collected []file) error {
	files, err := t.selection.selectFiles(ctx, collected)
	if err != nil {
		return fmt.Errorf("selecting files for hashes: %w", err)
	}

	manifest := make(map[string]string, len(files))
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		name, inside := within(rel, t.selection.prefix)
		if !inside {
			name = rel
		}
		if excluded(t.exclude, name, false) {
			continue
		}

		data, err := readFile(ctx, root, files[rel])
		if err != nil {
			return fmt.Errorf("reading %s for hashing: %w", files[rel], err)
		}

		manifest[rel] = computeHash(data)
	}

	data, err := json.MarshalIndent(manifest, "", "\t")
	if err != nil {
		return fmt.Errorf("marshaling hashes: %w", err)
	}

	if err := writeFile(ctx, root, t.destination, data); err != nil {
		return fmt.Errorf("writing hashes.json: %w", err)
	}

	t.upload = files
	return nil
}

// computeHash returns the hexadecimal SHA-256 digest of data with carriage
// returns removed, matching XenForo's hash normalization.
func computeHash(data []byte) string {
	data = bytes.ReplaceAll(data, []byte{'\r'}, nil)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
