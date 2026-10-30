package format

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"runtime"
)

type (
	File struct {
		Path    string
		Content []byte
	}

	language struct {
		marker  []byte
		steps   []tool
		isolate []string
	}

	Engine struct {
		byExt      map[string]*language
		workers    int
		batchSize  int
		configRoot string
		configs    []configFile
		copyNames  map[string]struct{}
	}
)

func New(config Config) (*Engine, error) {
	const defaultBatchSize = 200

	engine := &Engine{
		byExt:      map[string]*language{},
		copyNames:  map[string]struct{}{},
		workers:    config.Workers,
		batchSize:  config.BatchSize,
		configRoot: config.ConfigRoot,
	}
	if engine.workers <= 0 {
		engine.workers = runtime.NumCPU()
	}

	if engine.batchSize <= 0 {
		engine.batchSize = defaultBatchSize
	}

	configs, err := readConfigs(config.ConfigRoot, config.CopyFiles)
	if err != nil {
		return nil, err
	}

	engine.configs = configs

	for _, name := range config.CopyFiles {
		engine.copyNames[fold(path.Clean(filepath.ToSlash(name)))] = struct{}{}
	}

	for _, spec := range config.Languages {
		compiled := &language{marker: []byte(spec.SkipMarker)}

		for _, pattern := range spec.Isolate {
			if err = checkIsolate(pattern, spec.Name); err != nil {
				return nil, err
			}

			compiled.isolate = append(compiled.isolate, pattern)
		}

		for _, step := range spec.Steps {
			resolved, stepErr := compileStep(step, config.ConfigRoot)
			if stepErr != nil {
				return nil, fmt.Errorf("language %q: %w", spec.Name, stepErr)
			}

			compiled.steps = append(compiled.steps, resolved)
		}

		for _, ext := range spec.Extensions {
			if ext == "" || ext[0] != '.' {
				return nil, fmt.Errorf(
					"%w: extension %q of %q needs a leading dot",
					ErrConfig,
					ext,
					spec.Name,
				)
			}

			if _, taken := engine.byExt[ext]; taken {
				return nil, fmt.Errorf("%w: extension %q is claimed twice", ErrConfig, ext)
			}

			engine.byExt[ext] = compiled
		}
	}

	return engine, nil
}

func (engine *Engine) FormatAll(ctx context.Context, files []File) ([]File, []*Failure) {
	out := make([]File, len(files))
	copy(out, files)

	grouped := map[*language][]job{}

	var order []*language

	// the language comes from each file own extension, never from a neighbour
	for index, file := range files {
		clean := path.Clean(file.Path)

		lang, found := engine.byExt[path.Ext(clean)]
		if !found || len(lang.steps) == 0 || isSkipped(lang, file.Content) {
			continue
		}

		if _, seen := grouped[lang]; !seen {
			order = append(order, lang)
		}

		grouped[lang] = append(
			grouped[lang],
			job{
				index: index, path: file.Path, clean: clean, content: file.Content,
				alone: engine.isolated(lang, clean),
			},
		)
	}

	var failures []*Failure

	for _, lang := range order {
		jobs := grouped[lang]

		for _, step := range lang.steps {
			results := engine.runStep(ctx, step, jobs)
			next := jobs[:0:0]

			for position, result := range results {
				if result.failure != nil {
					failures = append(failures, result.failure)

					continue
				}

				jobs[position].content = result.content
				next = append(next, jobs[position])
			}

			jobs = next
		}

		for _, done := range jobs {
			out[done.index].Content = done.content
		}
	}

	return out, failures
}
