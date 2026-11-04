package e2e

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom/router"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

type Rule struct {
	name     string
	dirs     []string
	requests []string
	handle   []string
	mount    []string
	router   *router.Rule
	logger   *slog.Logger
	reported sync.Once
}

var (
	_ idiom.EdgeRule = (*Rule)(nil)
	_ rules.Globber  = (*Rule)(nil)
)

func Constructor(name string, logger *slog.Logger) rules.Constructor {
	return func(raw json.RawMessage) (rules.Rule, error) {
		cfg, err := decode(raw)
		if err != nil {
			return nil, err
		}

		built, err := router.Constructor("go-router")(cfg.Router)
		if err != nil {
			return nil, fmt.Errorf("router: %w", err)
		}

		routes, isRouter := built.(*router.Rule)
		if !isRouter {
			return nil, fmt.Errorf("%w: router built %T", ErrConfig, built)
		}

		handle, mount, err := routerPatterns(cfg.Router)
		if err != nil {
			return nil, err
		}

		return &Rule{
			name: name, dirs: cfg.Dirs, requests: cfg.Requests,
			handle: handle, mount: mount, router: routes, logger: logger,
		}, nil
	}
}

func (rule *Rule) Type() string { return rule.name }

func (rule *Rule) Globs() []string {
	return slices.Concat(rule.router.Globs(), rule.dirs)
}
