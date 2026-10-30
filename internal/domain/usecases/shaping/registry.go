package shaping

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

var (
	ErrMalformed     = errors.New("malformed pass")
	ErrMissingType   = errors.New("missing type")
	ErrUnknownType   = errors.New("unknown type")
	ErrDuplicateType = errors.New("duplicate type")
)

type (
	Constructor func(json.RawMessage) (Pass, error)

	Entry struct {
		Type string
		New  Constructor
	}

	Registry struct {
		constructors map[string]Constructor
	}
)

func NewRegistry(entries ...Entry) (Registry, error) {
	constructors := make(map[string]Constructor, len(entries))

	for _, entry := range entries {
		if entry.Type == "" {
			return Registry{}, ErrMissingType
		}

		if entry.New == nil {
			return Registry{}, fmt.Errorf("%w: %q has no constructor", ErrMalformed, entry.Type)
		}

		if _, taken := constructors[entry.Type]; taken {
			return Registry{}, fmt.Errorf("%w %q", ErrDuplicateType, entry.Type)
		}

		constructors[entry.Type] = entry.New
	}

	return Registry{constructors: constructors}, nil
}

func (registry Registry) Build(raw json.RawMessage) ([]Pass, error) {
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}

	var specs []json.RawMessage
	if err := json.Unmarshal(raw, &specs); err != nil {
		return nil, fmt.Errorf("passes: %w: expected an array of objects", ErrMalformed)
	}

	built := make([]Pass, 0, len(specs))

	for index, spec := range specs {
		var head struct {
			Type string `json:"type"`
		}

		if err := json.Unmarshal(spec, &head); err != nil {
			return nil, fmt.Errorf("passes[%d]: %w: expected an object", index, ErrMalformed)
		}

		if head.Type == "" {
			return nil, fmt.Errorf("passes[%d]: %w", index, ErrMissingType)
		}

		construct, found := registry.constructors[head.Type]
		if !found {
			return nil, fmt.Errorf(
				"passes[%d]: %w %q (%s)", index, ErrUnknownType, head.Type, registry.known(),
			)
		}

		pass, err := construct(spec)
		if err != nil {
			return nil, fmt.Errorf("passes[%d] (%s): %w", index, head.Type, err)
		}

		built = append(built, pass)
	}

	return built, nil
}

func DecodeStrict(raw json.RawMessage, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("%w: %w", ErrMalformed, err)
	}

	return nil
}

func (registry Registry) known() string {
	if len(registry.constructors) == 0 {
		return "no pass types are registered"
	}

	return "known: " + strings.Join(slices.Sorted(maps.Keys(registry.constructors)), ", ")
}
