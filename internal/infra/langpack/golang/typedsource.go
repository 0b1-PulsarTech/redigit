package golang

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"slices"
	"strings"
	"sync"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// typedSource type-checks the tree a snapshot describes, once, for every rule
// that needs an answer a reference query cannot give: which methods a generic
// constraint forces on a type, and which methods a concrete value must have
// where an interface is expected. Enumerating the syntaxes that hand a type to
// an interface does not converge (four rounds found four), so the question is
// put to the type checker.
//
// Packages of the tree are checked from source, tests included, under the run's
// build tags; what they import from outside the tree comes from export data.
// A file the tags hide is checked in a pass of its own, under tags that show it
// (see hiddenPasses). The first call loads and every later one reuses the
// result, which is only sound because one run indexes one snapshot.
type typedSource struct {
	tags []string

	once  sync.Once
	facts typedFacts
	err   error
}

func newTypedSource(tags []string) *typedSource { return &typedSource{tags: tags} }

// Facts returns what the type checker says about the snapshot's tree.
func (s *typedSource) Facts(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) (typedFacts, error) {
	s.once.Do(func() {
		s.facts, s.err = s.check(ctx, snap, decls)
	})

	return s.facts, s.err
}

func (s *typedSource) check(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) (typedFacts, error) {
	modules, err := modulesOf(snap)
	if err != nil {
		return typedFacts{}, err
	}

	if len(modules) == 0 {
		return typedFacts{}, nil
	}

	passes, err := hiddenPasses(snap, s.tags, modules)
	if err != nil {
		return typedFacts{}, err
	}

	passes = append([]loadPass{{tags: s.tags, patterns: allPatterns(modules)}}, passes...)
	locator := newDeclLocator(decls)

	var facts typedFacts

	for _, pass := range passes {
		pkgs, loadErr := typeCheck(ctx, snap.Root, pass)
		if loadErr != nil {
			return typedFacts{}, loadErr
		}

		facts = facts.merged(collectFacts(pkgs, snap.Root, locator))
	}

	return facts.sorted(), nil
}

// loadPass is one load of the tree: the packages to check, under which tags.
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

// module is a go.mod of the snapshot: the directory it sits in, relative to the
// root, and the path its packages are imported under.
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

		source, err := readSource(snap, file)
		if err != nil {
			return nil, err
		}

		if name := modfile.ModulePath(source); name != "" {
			modules = append(modules, module{dir: path.Dir(file), path: name})
		}
	}

	return modules, nil
}

// allPatterns is "<module path>/..." for each module, which is how a workspace
// of several modules is named to the go command.
func allPatterns(modules []module) []string {
	patterns := make([]string, 0, len(modules))
	for _, found := range modules {
		patterns = append(patterns, found.path+"/...")
	}

	slices.Sort(patterns)

	return patterns
}
