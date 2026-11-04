package verifyconf

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"

	"github.com/wrapped-owls/redigit/internal/infra/checkrun"
)

const (
	GoName   = "go"
	GoMarker = "go.mod"
)

type (
	Settings struct {
		Tags      []string
		Exclude   []string
		Stride    int
		Parallel  int
		Languages map[string]checkrun.Language
		Markers   map[string]string
	}

	members map[string]func(json.RawMessage) error
)

func Seed() Settings {
	return Settings{Markers: map[string]string{GoMarker: GoName}}
}

func HasRootExclude(entries []string) bool {
	return slices.ContainsFunc(entries, func(entry string) bool { return path.Clean(entry) == "." })
}

func decodeExclude(raw json.RawMessage, target *[]string) error {
	if err := decodeList(raw, target); err != nil {
		return err
	}

	if HasRootExclude(*target) {
		return errors.New("an entry cannot be empty or name the repository root")
	}

	return nil
}

func Decode(raw json.RawMessage, seed Settings) (Settings, error) {
	found := seed

	err := decodeMembers(raw, members{
		"tags":    func(raw json.RawMessage) error { return decodeList(raw, &found.Tags) },
		"exclude": func(raw json.RawMessage) error { return decodeExclude(raw, &found.Exclude) },
		"stride":  func(raw json.RawMessage) error { return decodeCount(raw, &found.Stride) },
		"parallel": func(raw json.RawMessage) error {
			return decodeParallel(raw, &found.Parallel)
		},
		"languages": func(raw json.RawMessage) error { return decodeLanguages(raw, &found) },
	})
	if err != nil {
		return Settings{}, fmt.Errorf("verify: %w", err)
	}

	return found, nil
}
