package idiom

import (
	"context"
	"fmt"
	"maps"
	"path"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

type Pack struct {
	provider   Provider
	extensions []string
	built      []rules.Rule
}

var (
	_ indexing.RulePack = Pack{}
	_ indexing.Atomizer = Pack{}
)

func NewPack(provider Provider, extensions []string, built ...rules.Rule) Pack {
	return Pack{provider: provider, extensions: extensions, built: built}
}

func (Pack) Name() string { return "idioms" }

func (pack Pack) Edges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	return collect(ctx, pack, snap, decls, EdgeRule.Edges)
}

func (pack Pack) Anchors(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	return collect(ctx, pack, snap, decls, AnchorRule.Anchors)
}

func (pack Pack) Atoms(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) (map[entities.DeclKey]string, error) {
	groups, err := collect(ctx, pack, snap, decls, atomGroup)
	if err != nil {
		return nil, err
	}

	atoms := map[entities.DeclKey]string{}
	for _, group := range groups {
		maps.Copy(atoms, group)
	}

	return atoms, nil
}

func atomGroup(
	rule AtomRule,
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
	facts Facts,
) ([]map[entities.DeclKey]string, error) {
	found, err := rule.Atoms(ctx, snap, decls, facts)
	if err != nil {
		return nil, fmt.Errorf("atoms: %w", err)
	}

	return []map[entities.DeclKey]string{found}, nil
}

func (pack Pack) Gates(ctx context.Context, snap entities.Snapshot) []auditing.Gate {
	var (
		gates []auditing.Gate
		at    []int
	)

	for idx, rule := range pack.built {
		if gater, found := rule.(Gater); found {
			gates = append(gates, gater.Gate(func(decls []entities.Decl) (Facts, error) {
				// The gate outlives this call, so the context may be gone by the time it measures.
				if err := ctx.Err(); err != nil {
					return nil, fmt.Errorf("context: %w", err)
				}

				return pack.facts(ctx, snap, pack.scope(decls))
			}))

			at = append(at, idx)
		}
	}

	if len(gates) > 1 {
		for pos, gate := range gates {
			label := fmt.Sprintf("rules[%d] (%s)", at[pos], pack.built[at[pos]].Type())
			gates[pos] = labelled{inner: gate, label: label}
		}
	}

	return gates
}

func collect[Capable rules.Rule, Out any](
	ctx context.Context,
	pack Pack,
	snap entities.Snapshot,
	decls []entities.Decl,
	run func(
		rule Capable,
		ctx context.Context,
		snap entities.Snapshot,
		decls []entities.Decl,
		facts Facts,
	) ([]Out, error),
) ([]Out, error) {
	var (
		out    []Out
		shared Facts
		scoped = pack.scope(decls)
	)

	for idx, built := range pack.built {
		rule, capable := built.(Capable)
		if !capable {
			continue
		}

		// Facts are indexed on the first rule that can use them, never for a pack that has none.
		if shared == nil {
			var err error

			if shared, err = pack.facts(ctx, snap, scoped); err != nil {
				return nil, err
			}
		}

		found, err := run(rule, ctx, snap, scoped, shared)
		if err != nil {
			return nil, fmt.Errorf("rules[%d] (%s): %w", idx, built.Type(), err)
		}

		out = append(out, found...)
	}

	return out, nil
}

//nolint:ireturn // the port's own Facts is what every rule consumes.
func (pack Pack) facts(
	ctx context.Context,
	snap entities.Snapshot,
	scoped []entities.Decl,
) (Facts, error) {
	indexed, err := pack.provider.Facts(ctx, snap, scoped)
	if err != nil {
		return nil, fmt.Errorf("idiom facts: %w", err)
	}

	return indexed, nil
}

func (pack Pack) scope(decls []entities.Decl) []entities.Decl {
	return slices.DeleteFunc(slices.Clone(decls), func(decl entities.Decl) bool {
		return !slices.Contains(pack.extensions, path.Ext(decl.Key.File))
	})
}
