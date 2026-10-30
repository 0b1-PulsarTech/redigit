package verify

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
	"github.com/wrapped-owls/redigit/internal/infra/checkrun"
)

const goName, goMarker = "go", "go.mod"

type (
	plan struct {
		repo      string
		cacheDir  string
		opts      verifying.Options
		languages map[string]checkrun.Language
		markers   map[string]string
	}

	settings struct {
		tags      []string
		exclude   []string
		stride    int
		parallel  int
		languages map[string]checkrun.Language
		markers   map[string]string
	}

	members map[string]func(json.RawMessage) error
)

func resolve(opts options) (plan, error) {
	file := settings{markers: map[string]string{goMarker: goName}}

	if opts.configPath != "" {
		loaded, err := loadSettings(opts.configPath, file)
		if err != nil {
			return plan{}, err
		}

		file = loaded
	}

	tags := file.tags
	if opts.provided["tags"] || file.tags == nil {
		tags = splitList(opts.tags)
	}

	if _, isRedefined := file.languages[goName]; isRedefined && len(tags) > 0 {
		return plan{}, fmt.Errorf(
			"%w: tags cannot apply to a redefined languages.go; put them in its steps",
			bootstrap.ErrConfig,
		)
	}

	languages := map[string]checkrun.Language{goName: checkrun.GoDefault(tags)}
	maps.Copy(languages, file.languages)

	stride, parallel, exclude := file.stride, file.parallel, file.exclude
	if opts.provided["stride"] || file.stride == 0 {
		stride = opts.stride
	}

	if opts.provided["parallel"] || file.parallel == 0 {
		parallel = opts.parallel
	}

	if opts.provided["exclude"] || file.exclude == nil {
		exclude = splitList(opts.exclude)
	}

	return plan{
		repo: opts.repo, cacheDir: opts.cacheDir, languages: languages, markers: file.markers,
		opts: verifying.Options{
			Ref: opts.ref, From: opts.from, To: opts.to,
			Stride: stride, Parallel: parallel, Exclude: exclude,
		},
	}, nil
}

func hasRootExclude(entries []string) bool {
	return slices.ContainsFunc(entries, func(entry string) bool { return path.Clean(entry) == "." })
}

func decodeExclude(raw json.RawMessage, target *[]string) error {
	if err := decodeList(raw, target); err != nil {
		return err
	}

	if hasRootExclude(*target) {
		return errors.New("an entry cannot be empty or name the repository root")
	}

	return nil
}

func splitList(text string) []string {
	parts := strings.FieldsFunc(text, func(char rune) bool { return char == ',' })
	if len(parts) == 0 {
		return nil
	}

	return parts
}

func loadSettings(path string, seed settings) (settings, error) {
	file, err := bootstrap.LoadConfigFile(path)
	if err != nil {
		return settings{}, err //nolint:wrapcheck // a config error names its file once
	}

	if len(file.File.Verify) == 0 {
		return seed, nil
	}

	found := seed

	err = decodeMembers(file.File.Verify, members{
		"tags":    func(raw json.RawMessage) error { return decodeList(raw, &found.tags) },
		"exclude": func(raw json.RawMessage) error { return decodeExclude(raw, &found.exclude) },
		"stride":  func(raw json.RawMessage) error { return decodeCount(raw, &found.stride) },
		"parallel": func(raw json.RawMessage) error {
			return decodeParallel(raw, &found.parallel)
		},
		"languages": func(raw json.RawMessage) error { return decodeLanguages(raw, &found) },
	})
	if err != nil {
		return settings{}, fmt.Errorf("%w: %s: verify: %w", bootstrap.ErrConfig, path, err)
	}

	return found, nil
}
