package checkrun

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

var (
	ErrToolMissing = errors.New("tool not found")
	ErrNoSteps     = errors.New("no steps for language")
	ErrVersion     = errors.New("version probe failed")
	ErrAmbient     = errors.New("probe saw a module or workspace around it")
	ErrModuleDir   = errors.New("module directory unreadable")
	ErrNoCacheDir  = errors.New("no cache directory configured")
	ErrCacheProg   = errors.New("GOCACHEPROG would bypass the per-environment build cache")

	_ verifying.Builder = (*Runner)(nil)
)

type (
	Step struct {
		Argv     []string
		Tolerate []string
		Timeout  time.Duration
	}

	Language struct {
		Steps   []Step
		Version []string
		Env     []string
	}

	Config struct {
		Languages map[string]Language
		CacheDir  string
	}

	Runner struct {
		cfg      Config
		mutex    sync.Mutex
		versions map[string]string
		used     map[string]struct{}
	}
)

func New(cfg Config) (*Runner, error) {
	if cfg.CacheDir == "" {
		return nil, ErrNoCacheDir
	}

	// Go refuses a relative GOCACHE, and steps run in the module directory, not here.
	root, err := filepath.Abs(cfg.CacheDir)
	if err != nil {
		return nil, fmt.Errorf("resolve cache dir: %w", err)
	}

	cfg.CacheDir = root

	return &Runner{cfg: cfg, versions: map[string]string{}, used: map[string]struct{}{}}, nil
}

func GoDefault(tags []string) Language {
	flags := []string{}
	if len(tags) > 0 {
		flags = []string{"-tags", strings.Join(tags, ",")}
	}

	step := func(verb string, tolerate ...string) Step {
		argv := append([]string{"go", verb}, flags...)

		return Step{Argv: append(argv, "./..."), Tolerate: tolerate, Timeout: defaultTimeout}
	}

	// A module holding only go.mod (a tools module) has nothing to vet, and vet exits 1 for it.
	return Language{
		Steps: []Step{
			step("build"),
			step("vet", "no packages to vet", `go: warning: "./..." matched no packages`),
		},
		Version: []string{"go", "version"},
	}
}

func (runner *Runner) Check(
	ctx context.Context,
	slotDir string,
	unit verifying.Unit,
) (verifying.Outcome, error) {
	language, err := runner.language(unit.Language)
	if err != nil {
		return verifying.Outcome{}, err
	}

	fileFlags, err := runner.fileFlags(ctx, unit)
	if err != nil {
		return verifying.Outcome{}, err
	}

	if unit.Language == goLanguage {
		if _, err = runner.goEnv(ctx, unit); err != nil {
			return verifying.Outcome{}, err
		}
	}

	env, keyed := stepEnv(unit, language.Env, fileFlags)

	// A custom step may run go too, so every language builds into its own cache.
	env, cacheDir, err := withGoCache(runner.cfg.CacheDir, env, keyed)
	if err != nil {
		return verifying.Outcome{}, err
	}

	runner.mutex.Lock()
	runner.used[cacheDir] = struct{}{}
	runner.mutex.Unlock()

	var last string

	for _, step := range language.Steps {
		outcome, runErr := runStep(ctx, stepRun{
			dir: filepath.Join(slotDir, unit.Dir), env: env, step: step,
		})
		if runErr != nil || outcome.IsFailed {
			return outcome, runErr
		}

		last = outcome.Step
	}

	return verifying.Outcome{Step: last}, nil
}

func (runner *Runner) language(name string) (Language, error) {
	found, isKnown := runner.cfg.Languages[name]
	if !isKnown || len(found.Steps) == 0 {
		return Language{}, fmt.Errorf("%w: %s", ErrNoSteps, name)
	}

	return found, nil
}
