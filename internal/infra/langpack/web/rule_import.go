package web

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// importRule makes a file wait for every file of the tree it imports. A
// virtual module waits for both the file declaring its types and the file it
// is compiled from: the first is what svelte-check reads, the second what the
// bundle runs.
type importRule struct{}

func (importRule) Name() string { return "import" }

func (importRule) Edges(ctx context.Context, run *index) ([]entities.Edge, error) {
	var out []entities.Edge

	for _, file := range run.scripts {
		dependent, _ := run.first(file)

		targets, err := run.targets(ctx, file)
		if err != nil {
			return nil, err
		}

		for _, target := range targets {
			if provider, found := run.first(target); found {
				out = append(out, entities.Edge{
					Dependent: dependent,
					Provider:  provider,
					Reason:    entities.ReasonReference,
				})
			}
		}
	}

	return out, nil
}

func (importRule) Anchors(context.Context, *index) ([]entities.Anchor, error) { return nil, nil }

// targets lists, in import order, the files of the tree file imports.
func (run *index) targets(ctx context.Context, file string) ([]string, error) {
	source, err := run.space.read(ctx, file)
	if err != nil {
		return nil, err
	}

	var out []string

	for _, spec := range specifiers(string(source)) {
		target, resolveErr := run.space.resolve(ctx, file, spec)
		if resolveErr != nil {
			return nil, resolveErr
		}

		for _, each := range []string{target, run.virtual[spec]} {
			if each != "" && run.space.files[each] {
				out = append(out, each)
			}
		}
	}

	return out, nil
}
