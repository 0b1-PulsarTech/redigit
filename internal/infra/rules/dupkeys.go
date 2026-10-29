package rules

import (
	"bytes"
	"encoding/json"
	"errors"
)

var ErrDuplicateKey = errors.New("duplicate key")

type objectScope struct {
	// keys is nil for an array.
	keys      map[string]struct{}
	expectKey bool
}

func checkDuplicateKeys(doc []byte, lead int) error {
	// Plain decoding keeps the last of a repeated key; syntax errors are left to it.
	decoder := json.NewDecoder(bytes.NewReader(doc))

	var stack []objectScope

	// A value is done once it is a scalar or its container closes.
	valueDone := func() {
		if top := len(stack) - 1; top >= 0 && stack[top].keys != nil {
			stack[top].expectKey = true
		}
	}

	for {
		token, err := decoder.Token()
		if err != nil {
			return nil //nolint:nilerr // syntax errors are reported by the decoder proper
		}

		switch value := token.(type) {
		case json.Delim:
			switch value {
			case '{':
				stack = append(stack, objectScope{keys: map[string]struct{}{}, expectKey: true})
			case '[':
				stack = append(stack, objectScope{})
			default:
				stack = stack[:len(stack)-1]

				valueDone()
			}
		case string:
			top := len(stack) - 1
			if top < 0 || !stack[top].expectKey {
				valueDone()

				continue
			}

			if _, seen := stack[top].keys[value]; seen {
				return malformed("%w %q at offset %d", ErrDuplicateKey, value,
					lead+int(decoder.InputOffset()))
			}

			stack[top].keys[value] = struct{}{}
			stack[top].expectKey = false
		default:
			valueDone()
		}
	}
}
