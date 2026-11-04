package anchorprobe

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

type (
	Commits interface {
		List(ctx context.Context, ref string) ([]verifying.Commit, error)
	}

	Checkout interface {
		Materialise(ctx context.Context, slot int, commit verifying.Commit) (string, error)
	}

	Modules interface {
		List(ctx context.Context, commit verifying.Commit) (verifying.Listing, error)
	}

	Builder interface {
		Check(ctx context.Context, slotDir string, unit verifying.Unit) (verifying.Outcome, error)
	}

	Renderer interface {
		Render(ctx context.Context, source []byte, keep []entities.DeclKey) ([]byte, error)
	}
)

//go:generate go tool -modfile=../../../tools/go.mod mockgen -source=ports.go -destination=ports_mock_test.go -package=anchorprobe
