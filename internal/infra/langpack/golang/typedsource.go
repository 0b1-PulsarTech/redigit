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
// The first call loads and every later one reuses the result, which is only
// sound because one run indexes one snapshot.
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
		pkgs, err := typeCheck(ctx, snap, s.tags)
		if err != nil {
			s.err = err

			return
		}

		s.facts = collectFacts(pkgs, snap.Root, newDeclLocator(decls))
	})

	return s.facts, s.err
}

// typeCheck loads every package of every module the snapshot holds. A tree with
// no go.mod has nothing to load, which is an answer and not a failure.
func typeCheck(
	ctx context.Context,
	snap entities.Snapshot,
	tags []string,
) ([]*packages.Package, error) {
	patterns, err := modulePatterns(snap)
	if err != nil {
		return nil, err
	}

	if len(patterns) == 0 {
		return nil, nil
	}

	cfg := &packages.Config{
		Context: ctx,
		Dir:     snap.Root,
		Tests:   true,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedImports |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo |
			packages.NeedTypesSizes,
	}

	if len(tags) > 0 {
		cfg.BuildFlags = []string{"-tags=" + strings.Join(tags, ",")}
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}

	// A package that does not type-check still yields what was checked before
	// the error, so one broken package costs its own edges and not the run's.
	// Said out loud, because a tree full of them answers almost nothing.
	if broken := brokenPackages(pkgs); broken > 0 {
		slog.Warn("some packages did not type-check; their type facts are partial",
			"count", broken, "of", len(pkgs))
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

// modulePatterns is "<module path>/..." for each go.mod in the snapshot, which
// is how a workspace of several modules is named to the go command.
func modulePatterns(snap entities.Snapshot) ([]string, error) {
	var patterns []string

	for _, file := range snap.Files {
		if path.Base(file) != "go.mod" {
			continue
		}

		source, err := readSource(snap, file)
		if err != nil {
			return nil, err
		}

		if module := modfile.ModulePath(source); module != "" {
			patterns = append(patterns, module+"/...")
		}
	}

	slices.Sort(patterns)

	return patterns, nil
}
