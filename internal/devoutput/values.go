package devoutput

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// record holds named JSON values for metadata or an export entry.
type record map[string]json.RawMessage

// value formats f's source value from r and reports whether it should be omitted.
// Missing required fields, invalid values, and forbidden class prefixes are errors.
func (f field) value(r record) (string, bool, error) {
	raw, ok := r[f.source]
	if !ok {
		if f.optional {
			return "", true, nil
		}
		return "", false, fmt.Errorf("missing field %q", f.source)
	}
	if f.omit == emptyValue && isEmpty(raw) {
		return "", true, nil
	}
	var value string
	var err error
	switch f.format {
	case scalar:
		value, err = scalarValue(raw)
	case compactJSON, prettyJSON:
		var formatted bytes.Buffer
		if f.format == prettyJSON {
			err = json.Indent(&formatted, raw, "", "    ")
		} else {
			err = json.Compact(&formatted, raw)
		}
		value = formatted.String()
	case lines, commas:
		var values []string
		if err = json.Unmarshal(raw, &values); err == nil {
			separator := "\n"
			if f.format == commas {
				separator = ","
			}
			value = strings.Join(values, separator)
		}
	default:
		err = fmt.Errorf("unknown value format %d", f.format)
	}
	if err != nil {
		return "", false, fmt.Errorf("field %q: %w", f.source, err)
	}
	for _, prefix := range f.forbiddenPrefixes {
		if strings.HasPrefix(strings.ToLower(strings.TrimLeft(value, `\`)), strings.ToLower(prefix)) {
			return "", false, fmt.Errorf("field %q: custom export handler %q is not supported", f.source, value)
		}
	}
	return value, f.omit == emptyString && bytes.Equal(bytes.TrimSpace(raw), []byte(`""`)), nil
}

// scalarValue converts a JSON scalar to XML text. Booleans become "1" or "0",
// null becomes an empty string, and numbers retain their spelling. Arrays and
// objects are rejected. raw must contain valid JSON.
func scalarValue(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", errors.New("expected a scalar")
	}
	switch raw[0] {
	case '"':
		var value string
		err := json.Unmarshal(raw, &value)
		return value, err
	case 't':
		return "1", nil
	case 'f':
		return "0", nil
	case 'n':
		return "", nil
	case '{', '[':
		return "", errors.New("expected a scalar")
	default:
		return string(raw), nil
	}
}

// isEmpty reports whether raw is null, false, 0, "0", an empty string, or an empty
// array or object, matching the empty values used by XenForo's XML exporters.
func isEmpty(raw json.RawMessage) bool {
	var compact bytes.Buffer
	if json.Compact(&compact, raw) != nil {
		return false
	}
	switch compact.String() {
	case "null", "false", "0", `"0"`, `""`, "[]", "{}":
		return true
	}
	return false
}

// validateText checks that value is valid UTF-8 containing only XML 1.0 characters.
func validateText(value string) error {
	if !utf8.ValidString(value) {
		return errors.New("invalid UTF-8")
	}
	for _, r := range value {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
		case r >= 0x20 && r <= 0xd7ff, r >= 0xe000 && r <= 0xfffd, r >= 0x10000 && r <= 0x10ffff:
		default:
			return fmt.Errorf("character U+%04X is not valid in XML", r)
		}
	}
	return nil
}
