package plancache

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

type Entry struct {
	Decls     []entities.Decl
	Edges     []entities.Edge
	Anchors   []entities.Anchor
	Blind     []indexing.Blind
	Partial   []indexing.Partial
	Commit    string
	Ordering  entities.Ordering
	OrderNote string
	Plan      entities.Plan
	Unseen    []auditing.Unseen
	Unclaimed []auditing.Unclaimed
}

func Pack(result indexing.Result) Entry {
	return Entry{
		Decls:   result.Graph.Decls(),
		Edges:   result.Graph.Edges(),
		Anchors: result.Graph.Anchors(),
		Blind:   result.Blind,
		Partial: result.Partial,
	}
}

func (entry Entry) Result() indexing.Result {
	return indexing.Result{
		Graph:   entities.NewGraph(entry.Decls, entry.Edges, entry.Anchors),
		Blind:   entry.Blind,
		Partial: entry.Partial,
	}
}
