package idiom

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

//go:generate go tool -modfile=../../../../tools/go.mod mockgen -source=ports.go -destination=ports_mock_test.go -package=idiom

type (
	FactsOf func(decls []entities.Decl) (Facts, error)

	Provider interface {
		Facts(ctx context.Context, snap entities.Snapshot, decls []entities.Decl) (Facts, error)
	}

	EdgeRule interface {
		rules.Rule
		Edges(
			ctx context.Context,
			snap entities.Snapshot,
			decls []entities.Decl,
			facts Facts,
		) ([]entities.Edge, error)
	}

	AnchorRule interface {
		rules.Rule
		Anchors(
			ctx context.Context,
			snap entities.Snapshot,
			decls []entities.Decl,
			facts Facts,
		) ([]entities.Anchor, error)
	}

	AtomRule interface {
		rules.Rule
		Atoms(
			ctx context.Context,
			snap entities.Snapshot,
			decls []entities.Decl,
			facts Facts,
		) (map[entities.DeclKey]string, error)
	}

	Gater interface {
		rules.Rule
		Gate(factsOf FactsOf) auditing.Gate
	}
)
