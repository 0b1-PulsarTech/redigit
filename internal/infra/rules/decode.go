package rules

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"unicode/utf8"
)

func topLevel(content []byte) (map[string]json.RawMessage, error) {
	// Windows editors write a BOM; it counts toward offsets but is no JSON.
	const utf8BOM = "\xEF\xBB\xBF"

	before := len(content)
	content, _ = bytes.CutPrefix(content, []byte(utf8BOM))
	skipped := before - len(content)

	if !utf8.Valid(content) {
		return nil, malformed("not valid UTF-8 at offset %d", skipped+firstInvalid(content)+1)
	}

	const blanks = " \t\n\r"

	trimmed := bytes.Trim(content, blanks)
	if len(trimmed) == 0 {
		return nil, decodeError{ErrEmpty}
	}

	// Offsets count from the start of the file, not of the trimmed content.
	lead := skipped + len(content) - len(bytes.TrimLeft(content, blanks))

	if err := checkDuplicateKeys(trimmed, lead); err != nil {
		return nil, err
	}

	var top map[string]json.RawMessage

	if err := json.Unmarshal(trimmed, &top); err != nil {
		if syntax, ok := errors.AsType[*json.SyntaxError](err); ok {
			return nil, malformed("invalid JSON at offset %d", int64(lead)+syntax.Offset)
		}

		// Valid JSON that is not an object (an array, a scalar).
		return nil, malformed("expected a JSON object")
	}

	// A null decodes to a nil map without error, and is no config either.
	if top == nil {
		return nil, malformed("expected a JSON object")
	}

	return top, nil
}

func asObject(raw json.RawMessage, path string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage

	// The input is already valid JSON, so a failure here is a wrong kind; null leaves nil.
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, malformed("%s: expected an object", path)
	}

	return fields, nil
}

func decodeLanguages(raw json.RawMessage) (map[string]map[string]json.RawMessage, error) {
	sections, err := asObject(raw, "languages")
	if err != nil {
		return nil, err
	}

	languages := make(map[string]map[string]json.RawMessage, len(sections))

	for _, name := range slices.Sorted(maps.Keys(sections)) {
		var settings map[string]json.RawMessage

		if settings, err = asObject(sections[name], "languages."+name); err != nil {
			return nil, err
		}

		languages[name] = settings
	}

	return languages, nil
}

func decodeRules(raw json.RawMessage) ([]Spec, error) {
	var items []json.RawMessage

	if err := json.Unmarshal(raw, &items); err != nil || items == nil {
		return nil, malformed("rules: expected an array")
	}

	specs := make([]Spec, len(items))

	for idx, item := range items {
		spec, err := decodeSpec(item, idx)
		if err != nil {
			return nil, err
		}

		specs[idx] = spec
	}

	return specs, nil
}

func decodeSpec(item json.RawMessage, idx int) (Spec, error) {
	fields, err := asObject(item, fmt.Sprintf("rules[%d]", idx))
	if err != nil {
		return Spec{}, err
	}

	rawType, found := fields["type"]
	if !found {
		return Spec{}, malformed("rules[%d]: %w", idx, ErrMissingType)
	}

	var typeName string

	if err = json.Unmarshal(rawType, &typeName); err != nil || isNull(rawType) {
		return Spec{}, malformed("rules[%d].type: expected a string", idx)
	}

	if typeName == "" {
		return Spec{}, malformed("rules[%d]: %w", idx, ErrMissingType)
	}

	return Spec{Type: typeName, Raw: bytes.Clone(item)}, nil
}

func isNull(raw json.RawMessage) bool { return bytes.Equal(raw, []byte("null")) }

func firstInvalid(content []byte) int {
	for pos := 0; pos < len(content); {
		decoded, size := utf8.DecodeRune(content[pos:])
		if decoded == utf8.RuneError && size == 1 {
			return pos
		}

		pos += size
	}

	return len(content)
}
