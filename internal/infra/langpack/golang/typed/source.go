package typed

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"path"
	"slices"
	"strings"
	"sync"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

type Source struct {
	tags []string

	once   sync.Once
	loaded [][]*packages.Package
	err    error

	guard sync.Mutex
	facts map[uint64]Facts
}

func NewSource(tags []string) *Source {
	return &Source{tags: tags, facts: map[uint64]Facts{}}
}

func (s *Source) Facts(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) (Facts, error) {
	s.once.Do(func() {
		s.loaded, s.err = s.check(ctx, snap)
	})

	if s.err != nil {
		return Facts{}, s.err
	}

	s.guard.Lock()
	defer s.guard.Unlock()

	shape := shapeOf(decls)
	if known, found := s.facts[shape]; found {
		return known, nil
	}

	locator := newDeclLocator(decls)

	var facts Facts

	for _, pkgs := range s.loaded {
		facts = facts.merged(collectFacts(pkgs, snap.Root, locator))
	}

	s.facts[shape] = facts.sorted()

	return s.facts[shape], nil
}

func shapeOf(decls []entities.Decl) uint64 {
	hash := fnv.New64a()

	for _, decl := range decls {
		_, _ = fmt.Fprintf(hash, "%s|%d|%d\n", decl.Key, decl.Span.Start, decl.Span.End)
	}

	return hash.Sum64()
}

func (s *Source) check(
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

	passes, err := hiddenPasses(snap, s.tags, modules)
	if err != nil {
		return nil, err
	}

	passes = append([]loadPass{{tags: s.tags, patterns: allPatterns(modules)}}, passes...)

	loaded := make([][]*packages.Package, 0, len(passes))

	for _, pass := range passes {
		pkgs, loadErr := typeCheck(ctx, snap.Root, pass)
		if loadErr != nil {
			return nil, loadErr
		}

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
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedImports |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo |
			packages.NeedTypesSizes,
	}

	if len(pass.tags) > 0 {
		cfg.BuildFlags = []string{"-tags=" + strings.Join(pass.tags, ",")}
	}

	pkgs, err := packages.Load(cfg, pass.patterns...)
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}

	// A package that does not type-check still yields what was checked before
	// the error, so one broken package costs its own edges and not the run's.
	// Said out loud, because a tree full of them answers almost nothing.
	if broken := brokenPackages(pkgs); broken > 0 {
		slog.Warn("some packages did not type-check; their type facts are partial",
			"count", broken, "of", len(pkgs), "tags", pass.tags)
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

type module struct {
	dir  string
	path string
}

func modulesOf(snap entities.Snapshot) ([]module, error) {
	var modules []module

	for _, file := range snap.Files {
		if path.Base(file) != "go.mod" {
			continue
		}

		source, err := declsource.ReadSource(snap, file)
		if err != nil {
			return nil, err
		}

		if name := modfile.ModulePath(source); name != "" {
			modules = append(modules, module{dir: path.Dir(file), path: name})
		}
	}

	return modules, nil
}

func allPatterns(modules []module) []string {
	patterns := make([]string, 0, len(modules))
	for _, found := range modules {
		patterns = append(patterns, found.path+"/...")
	}

	slices.Sort(patterns)

	return patterns
}
