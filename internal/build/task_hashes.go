package build

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
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
func (t *hashesTask) run(ctx context.Context, root *os.Root) error {
	files, err := t.selection.collectFiles(ctx, root)
	if err != nil {
		return fmt.Errorf("selecting files for hashes: %w", err)
	}

	manifest := make(map[string]string, len(files))
	sources := directoryOpener{root: root}
	defer sources.Close()
	buffer := make([]byte, ioBufferSize)
	hasher := sha256.New()
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		name, inside := within(rel, t.selection.prefix)
		if !inside {
			name = rel
		}
		if excluded(t.exclude, name, false) {
			continue
		}

		digest, err := hashFile(ctx, &sources, files[rel], buffer, hasher)
		if err != nil {
			return fmt.Errorf("reading %s for hashing: %w", files[rel], err)
		}

		manifest[rel] = digest
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

// hashFile streams a regular file through reusable storage, removing carriage
// returns in place to match XenForo's hash normalization.
func hashFile(ctx context.Context, source fileSource, name string, buffer []byte, hasher hash.Hash) (string, error) {
	f, _, err := openRegular(ctx, source, name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hasher.Reset()

	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := f.Read(buffer)
		if n > 0 {
			chunk := buffer[:n]
			if bytes.IndexByte(chunk, '\r') >= 0 {
				kept := 0
				for _, b := range chunk {
					if b != '\r' {
						chunk[kept] = b
						kept++
					}
				}
				chunk = chunk[:kept]
			}
			if _, err := hasher.Write(chunk); err != nil {
				return "", err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}

	var digest [sha256.Size]byte
	return hex.EncodeToString(hasher.Sum(digest[:0])), nil
}
