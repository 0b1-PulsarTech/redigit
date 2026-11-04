package verify

import (
	"fmt"
	"maps"
	"strings"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
	"github.com/wrapped-owls/redigit/internal/infra/checkrun"
	"github.com/wrapped-owls/redigit/internal/infra/verifyconf"
)

type plan struct {
	repo      string
	cacheDir  string
	opts      verifying.Options
	languages map[string]checkrun.Language
	markers   map[string]string
}

func resolve(opts options) (plan, error) {
	file := verifyconf.Seed()

	if opts.configPath != "" {
		loaded, err := loadSettings(opts.configPath, file)
		if err != nil {
			return plan{}, err
		}

		file = loaded
	}

	tags := file.Tags
	if opts.provided["tags"] || file.Tags == nil {
		tags = splitList(opts.tags)
	}

	if _, isRedefined := file.Languages[verifyconf.GoName]; isRedefined && len(tags) > 0 {
		return plan{}, fmt.Errorf(
			"%w: tags cannot apply to a redefined languages.go; put them in its steps",
			bootstrap.ErrConfig,
		)
	}

	languages := map[string]checkrun.Language{verifyconf.GoName: checkrun.GoDefault(tags)}
	maps.Copy(languages, file.Languages)

	stride, parallel, exclude := file.Stride, file.Parallel, file.Exclude
	if opts.provided["stride"] || file.Stride == 0 {
		stride = opts.stride
	}

	if opts.provided["parallel"] || file.Parallel == 0 {
		parallel = opts.parallel
	}

	if opts.provided["exclude"] || file.Exclude == nil {
		exclude = splitList(opts.exclude)
	}

	return plan{
		repo: opts.repo, cacheDir: opts.cacheDir, languages: languages, markers: file.Markers,
		opts: verifying.Options{
			Ref: opts.ref, From: opts.from, To: opts.to,
			Stride: stride, Parallel: parallel, Exclude: exclude,
		},
	}, nil
}

func splitList(text string) []string {
	parts := strings.FieldsFunc(text, func(char rune) bool { return char == ',' })
	if len(parts) == 0 {
		return nil
	}

	return parts
}

func loadSettings(path string, seed verifyconf.Settings) (verifyconf.Settings, error) {
	file, err := bootstrap.LoadConfigFile(path)
	if err != nil {
		return verifyconf.Settings{}, err //nolint:wrapcheck // a config error names its file once
	}

	if len(file.File.Verify) == 0 {
		return seed, nil
	}

	found, err := verifyconf.Decode(file.File.Verify, seed)
	if err != nil {
		return verifyconf.Settings{}, fmt.Errorf("%w: %s: %w", bootstrap.ErrConfig, path, err)
	}

	return found, nil
}
