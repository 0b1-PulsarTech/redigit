package pinrule

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrConfig       = errors.New("malformed pin rule")
	ErrMissingField = errors.New("pin names no decl or no after")
	ErrBadPattern   = errors.New("malformed pin pattern")
	ErrStmtPattern  = errors.New("pin names a cut statement")
)

type wire struct {
	Type  string `json:"type"`
	Decl  string `json:"decl"`
	After string `json:"after"`
}

func decode(raw json.RawMessage) (Rule, error) {
	var parsed wire

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&parsed); err != nil {
		return Rule{}, fmt.Errorf("%w: %w", ErrConfig, err)
	}

	if parsed.Decl == "" || parsed.After == "" {
		return Rule{}, ErrMissingField
	}

	decl, err := parseTarget(parsed.Decl)
	if err != nil {
		return Rule{}, fmt.Errorf("decl %w", err)
	}

	after, err := parseTarget(parsed.After)
	if err != nil {
		return Rule{}, fmt.Errorf("after %w", err)
	}

	return Rule{decl: decl, after: after}, nil
}
