package resolve

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/loadonce"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

var ErrNotCovered = errors.New("not covered by the typed index")

type (
	index struct {
		usage
		implemented

		covered map[string]covering
		partial map[row]bool
		cgo     bool
	}

	Typed struct {
		source *typed.Source
		snap   entities.Snapshot

		built      loadonce.Once[[]index]
		adhocErr   atomic.Pointer[error]
		hiddenLost atomic.Int64
	}
)

var _ indexing.Resolver = (*Typed)(nil)

func NewTyped(source *typed.Source, snap entities.Snapshot) *Typed {
	return &Typed{source: source, snap: snap}
}

func (resolver *Typed) References(
	ctx context.Context,
	at entities.Location,
) ([]entities.Location, error) {
	built, where, err := resolver.declaration(ctx, at)
	if err != nil {
		return nil, fmt.Errorf("references: %w", err)
	}

	// Unlike gopls, a concrete method omits the calls written against an interface it satisfies.
	// The graph is unaffected: indexing's directCallsOnly drops those call sites anyway.
	return slices.Clone(built.refs[where]), nil
}

func (resolver *Typed) Implementations(
	ctx context.Context,
	at entities.Location,
) ([]entities.Location, error) {
	built, where, err := resolver.declaration(ctx, at)
	// An implementor need not import its interface's package, so a cgo package could be unseen.
	if err == nil && built.refuses(where) {
		err = ErrNotCovered
	}

	if err != nil {
		return nil, fmt.Errorf("implementations: %w", err)
	}

	return slices.Clone(built.implementors[where]), nil
}

func (resolver *Typed) Definition(
	ctx context.Context,
	at entities.Location,
) (entities.Location, error) {
	built, where, err := resolver.lookup(ctx, at)
	if err != nil {
		return entities.Location{}, fmt.Errorf("definition: %w", err)
	}

	found, known := built.defs[where]
	if !known {
		return entities.Location{}, fmt.Errorf("definition: %w", ErrNotCovered)
	}

	return found, nil
}

func (resolver *Typed) Close(ctx context.Context) error {
	if err := resolver.built.Close(ctx); err != nil {
		return fmt.Errorf("close typed index: %w", err)
	}

	return nil
}

func (resolver *Typed) declaration(
	ctx context.Context,
	at entities.Location,
) (index, site, error) {
	built, where, err := resolver.lookup(ctx, at)
	// A package with errors is read from, but a query on its own declarations waits for its owner.
	if err == nil && (built.covered[at.File].incomplete || built.partial[rowOf(at)]) {
		err = ErrNotCovered
	}

	return built, where, err
}

func (resolver *Typed) lookup(
	ctx context.Context,
	at entities.Location,
) (index, site, error) {
	built, err := resolver.built.Do(ctx, resolver.build)
	if err != nil {
		return index{}, site{}, fmt.Errorf("typed index: %w", err)
	}

	for _, group := range built {
		file, covered := group.covered[at.File]
		if !covered {
			continue
		}

		offset, inside := file.offset(at)
		if !inside {
			return index{}, site{}, ErrNotCovered
		}

		return group, site{file: at.File, offset: offset}, nil
	}

	return index{}, site{}, ErrNotCovered
}

func (resolver *Typed) build(ctx context.Context) ([]index, error) {
	// Each group is indexed on its own, a file belonging to one: an interface a main or ad hoc
	// group could meet in another, or one a hidden file declares, is refused. A hidden pass
	// reaches the interfaces of the packages it re-checks and no others.
	main, err := resolver.source.Packages(ctx, resolver.snap)
	if err != nil {
		return nil, fmt.Errorf("typed packages: %w", err)
	}

	// A failed ad hoc pass costs those files their typed answer, not the run: they stay uncovered,
	// so the fallback asks gopls, which loads them itself. A pass Close cut short is an error, as
	// an index of the main group alone would answer without the cross-group refusal.
	apart, adhocErr := resolver.source.Adhoc(ctx, resolver.snap)

	err = ctx.Err()
	if err != nil {
		return nil, fmt.Errorf("typed index: %w", err)
	}

	if adhocErr != nil {
		resolver.adhocErr.Store(&adhocErr)
	}

	// The hidden passes load with the main one, so a load failure there already failed the index.
	hidden, err := resolver.source.Hidden(ctx, resolver.snap)
	if err != nil {
		return nil, fmt.Errorf("typed hidden: %w", err)
	}

	groups := append([][]*packages.Package{main}, apart...)
	where := newPlaces(resolver.snap)
	built := indexGroups(groups, where)
	read := indexHidden(hidden, where, built)

	resolver.hiddenLost.Store(lostRows(read))

	return append(built, read...), nil
}
