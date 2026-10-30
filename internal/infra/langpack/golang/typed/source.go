package typed

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/loadonce"
)

type Source struct {
	tags []string

	loaded loadonce.Once[[][]*packages.Package]
	adhoc  loadonce.Once[[][]*packages.Package]

	guard sync.Mutex
	facts map[uint64]Facts
	short []Shortfall
}

func NewSource(tags []string) *Source {
	return &Source{tags: tags, facts: map[uint64]Facts{}}
}

func (source *Source) Facts(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) (Facts, error) {
	loaded, err := source.load(ctx, snap)
	if err != nil {
		return Facts{}, err
	}

	source.guard.Lock()
	defer source.guard.Unlock()

	shape := shapeOf(decls)
	if known, found := source.facts[shape]; found {
		return known, nil
	}

	// Packages stay loaded: the same facts are asked again per statement, each one a dependent.
	locator := newDeclLocator(decls)

	var facts Facts

	for _, pkgs := range loaded {
		facts = facts.merged(collectFacts(pkgs, snap.Root, locator))
	}

	source.facts[shape] = facts.sorted()

	return source.facts[shape], nil
}

func (source *Source) Close(ctx context.Context) error {
	var closeErr error

	if err := source.loaded.Close(ctx); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("stop typed load: %w", err))
	}

	if err := source.adhoc.Close(ctx); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("stop ad hoc load: %w", err))
	}

	return closeErr
}

func shapeOf(decls []entities.Decl) uint64 {
	hash := fnv.New64a()

	for _, decl := range decls {
		_, _ = fmt.Fprintf(hash, "%s|%d|%d\n", decl.Key, decl.Span.Start, decl.Span.End)
	}

	return hash.Sum64()
}

func (source *Source) check(
	ctx context.Context,
	snap entities.Snapshot,
) ([][]*packages.Package, error) {
	modules, err := modulesOf(snap)
	if err != nil {
		return nil, err
	}

	if len(modules) == 0 {
		return nil, nil
	}

	passes, err := hiddenPasses(snap, source.tags, modules)
	if err != nil {
		return nil, err
	}

	passes = append([]loadPass{{tags: source.tags, patterns: allPatterns(modules)}}, passes...)

	loaded := make([][]*packages.Package, 0, len(passes))

	for _, pass := range passes {
		pkgs, loadErr := typeCheck(ctx, snap.Root, pass)
		if loadErr != nil {
			return nil, loadErr
		}

		// A package that does not type-check still yields what was checked before
		// the error, so one broken package costs its own edges and not the run's.
		// Kept as data, because a tree full of them answers almost nothing.
		source.noteBroken(pkgs, pass.tags)
		loaded = append(loaded, pkgs)
	}

	return loaded, nil
}

type loadPass struct {
	tags     []string
	patterns []string
}

func typeCheck(
	ctx context.Context,
	root string,
	pass loadPass,
) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Context: ctx,
		Dir:     root,
		Tests:   true,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo |
			packages.NeedTypesSizes,
	}

	// Under -trimpath export data names files by module-relative paths no position maps back.
	cfg.BuildFlags = []string{"-trimpath=false"}

	if len(pass.tags) > 0 {
		cfg.BuildFlags = append(cfg.BuildFlags, "-tags="+strings.Join(pass.tags, ","))
	}

	pkgs, err := packages.Load(cfg, pass.patterns...)
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}

	return pkgs, nil
}

func brokenPackages(pkgs []*packages.Package) int {
	broken := 0

	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			broken++
		}
	}

	return broken
}
