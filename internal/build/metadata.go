package build

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// parseMetadata decodes addon.json as an object, retaining fields and numeric
// precision for command placeholders.
func parseMetadata(data []byte) (map[string]any, error) {
	var metadata map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&metadata); err != nil {
		return nil, fmt.Errorf("parsing addon.json: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("parsing addon.json: unexpected data after metadata")
	}
	if metadata == nil {
		return nil, errors.New("parsing addon.json: expected an object")
	}

	return metadata, nil
}
