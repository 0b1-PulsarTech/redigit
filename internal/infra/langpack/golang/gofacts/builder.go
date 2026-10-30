package gofacts

import (
	"context"
	"fmt"
	"sync"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

type Builder struct {
	source *typed.Source

	guard sync.Mutex
	built map[uint64]*Index
}

func New(source *typed.Source) *Builder {
	return &Builder{source: source, built: map[uint64]*Index{}}
}

func (builder *Builder) Index(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) (*Index, error) {
	// Pass 0 of the one load the source already made; no second load is ever started here.
	pkgs, err := builder.source.Packages(ctx, snap)
	if err != nil {
		return nil, fmt.Errorf("packages for facts: %w", err)
	}

	builder.guard.Lock()
	defer builder.guard.Unlock()

	shape := typed.ShapeOf(decls)
	if known, found := builder.built[shape]; found {
		return known, nil
	}

	where := typed.NewPositions(snap.Root, typed.NewDeclLocator(decls))
	index := &Index{
		calls: map[entities.DeclKey][]facts.Call{},
		ctors: map[entities.DeclKey][]facts.TypeRef{},
		impl:  newImplementors(pkgs, where),
	}

	index.walk(pkgs, where)
	builder.built[shape] = index

	return index, nil
}
