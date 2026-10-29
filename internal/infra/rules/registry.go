package rules

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

var ErrUnknownType = errors.New("unknown type")

type (
	Rule interface {
		Type() string
	}

	Globber interface {
		Globs() []string
	}

	Constructor func(json.RawMessage) (Rule, error)

	Registry struct {
		constructors map[string]Constructor
	}
)

func NewRegistry(constructors map[string]Constructor) Registry {
	return Registry{constructors: maps.Clone(constructors)}
}

func (registry Registry) Build(specs []Spec) ([]Rule, error) {
	built := make([]Rule, 0, len(specs))

	for idx, spec := range specs {
		construct, found := registry.constructors[spec.Type]
		if !found {
			return nil, fmt.Errorf(
				"rules[%d]: %w %q (%s)", idx, ErrUnknownType, spec.Type, registry.known(),
			)
		}

		rule, err := construct(spec.Raw)
		if err != nil {
			return nil, fmt.Errorf("rules[%d] (%s): %w", idx, spec.Type, err)
		}

		built = append(built, rule)
	}

	return built, nil
}

func (registry Registry) known() string {
	if len(registry.constructors) == 0 {
		return "no rule types are registered in this build"
	}

	return "known: " + strings.Join(slices.Sorted(maps.Keys(registry.constructors)), ", ")
}
