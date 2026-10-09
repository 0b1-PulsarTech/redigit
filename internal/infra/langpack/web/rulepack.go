package web

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// rule is one independently testable edge source over the run's index.
type rule interface {
	Name() string
	Edges(ctx context.Context, run *index) ([]entities.Edge, error)
	Anchors(ctx context.Context, run *index) ([]entities.Anchor, error)
}

var rules = []rule{importRule{}, routeRule{}, hostsRule{}, testAnchorRule{}}

// RulePack gives the web files their edges from the import graph, which is
// all a component's readers see of it. virtual maps a module a bundler plugin
// makes up, such as "virtual:x", to the file that module is compiled from.
type RulePack struct {
	virtual map[string]string
}

var _ indexing.RulePack = RulePack{}

func NewRulePack(virtual map[string]string) RulePack { return RulePack{virtual: virtual} }

func (RulePack) Name() string { return "web" }

func (p RulePack) Edges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	run := p.index(snap, decls)
	seen := map[entities.Edge]bool{}

	var out []entities.Edge

	for _, each := range rules {
		found, err := each.Edges(ctx, run)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", each.Name(), err)
		}

		for _, edge := range found {
			if !seen[edge] && edge.Dependent != edge.Provider {
				seen[edge] = true

				out = append(out, edge)
			}
		}
	}

	return out, nil
}

func (p RulePack) Anchors(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	run := p.index(snap, decls)

	var out []entities.Anchor

	for _, each := range rules {
		found, err := each.Anchors(ctx, run)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", each.Name(), err)
		}

		out = append(out, found...)
	}

	return out, nil
}

// index is one run's view: the workspace, and every declaration by file in
// source order, whichever pack cut it.
type index struct {
	space   *workspace
	virtual map[string]string
	byFile  map[string][]entities.Decl
	scripts []string
}

func (p RulePack) index(snap entities.Snapshot, decls []entities.Decl) *index {
	run := &index{
		space:   newWorkspace(snap.Files, rootReader(snap.Root)),
		virtual: p.virtual,
		byFile:  map[string][]entities.Decl{},
	}

	for _, decl := range decls {
		run.byFile[decl.Key.File] = append(run.byFile[decl.Key.File], decl)
	}

	for file, own := range run.byFile {
		slices.SortFunc(
			own,
			func(a, b entities.Decl) int { return cmp.Compare(a.Span.Start, b.Span.Start) },
		)

		if isScript(file) {
			run.scripts = append(run.scripts, file)
		}
	}

	slices.Sort(run.scripts)

	return run
}

// first is the declaration standing for a file's existence.
func (run *index) first(file string) (entities.DeclKey, bool) {
	own := run.byFile[file]
	if len(own) == 0 {
		return entities.DeclKey{}, false
	}

	return own[0].Key, true
}

func rootReader(root string) readFunc {
	return func(_ context.Context, rel string) ([]byte, error) {
		// The path comes from the snapshot's own file list, not from input.
		source, err := os.ReadFile(filepath.Join(root, rel)) //nolint:gosec
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}

		return source, nil
	}
}
