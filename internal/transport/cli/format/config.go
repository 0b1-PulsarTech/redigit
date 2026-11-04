package format

import (
	"bytes"
	"fmt"
	"path"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	formatter "github.com/wrapped-owls/redigit/internal/infra/format"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

type tools struct {
	engine *formatter.Engine
	exts   map[string]struct{}
}

func loadTools(configPath string) (tools, error) {
	file, err := bootstrap.LoadConfigFile(configPath)
	if err != nil {
		return tools{}, err //nolint:wrapcheck // a config error names its file once
	}

	if len(file.File.Format) == 0 {
		return tools{}, fmt.Errorf("%w: %s has no \"format\" section", write.ErrUsage, configPath)
	}

	config, err := formatter.Decode(bytes.NewReader(file.File.Format))
	if err != nil {
		return tools{}, fmt.Errorf("%w: %s: format: %w", bootstrap.ErrConfig, configPath, err)
	}

	engine, err := formatter.New(config)
	if err != nil {
		return tools{}, fmt.Errorf("%w: %s: format: %w", bootstrap.ErrConfig, configPath, err)
	}

	exts := map[string]struct{}{}

	for _, language := range config.Languages {
		if len(language.Steps) == 0 {
			continue
		}

		for _, ext := range language.Extensions {
			exts[ext] = struct{}{}
		}
	}

	return tools{engine: engine, exts: exts}, nil
}

func (set tools) covers(name string) bool {
	_, found := set.exts[path.Ext(path.Clean(name))]

	return found
}
