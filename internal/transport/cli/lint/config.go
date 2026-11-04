package lint

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/linting"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

func loadLimits(path string) (linting.Limits, error) {
	limits := linting.DefaultLimits()
	if path == "" {
		return limits, nil
	}

	file, err := bootstrap.LoadConfigFile(path)
	if err != nil {
		return linting.Limits{}, err //nolint:wrapcheck // a config error names its file once
	}

	if len(file.File.Lint) == 0 {
		return limits, nil
	}

	fields := map[string]*int{
		"sourceAdded": &limits.SourceAdded, "generatedMixed": &limits.GeneratedMixed,
		"rowAdded": &limits.RowAdded,
	}

	if err = decodeFields(file.File.Lint, fields); err != nil {
		return linting.Limits{}, fmt.Errorf("%w: %s: lint: %w", bootstrap.ErrConfig, path, err)
	}

	return limits, nil
}

func decodeFields(raw json.RawMessage, fields map[string]*int) error {
	var members map[string]json.RawMessage
	if err := rules.DecodeStrict(raw, &members, "an object"); err != nil {
		return err //nolint:wrapcheck // the caller names the file and section
	}

	for _, key := range slices.Sorted(maps.Keys(members)) {
		target, known := fields[key]
		if !known {
			return fmt.Errorf("unknown key %q", key)
		}

		if err := rules.DecodeStrict(members[key], target, "an integer"); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}

		if *target < 1 {
			return fmt.Errorf("%s: must be at least 1", key)
		}
	}

	return nil
}
