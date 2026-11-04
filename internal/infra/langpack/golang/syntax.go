package golang

import (
	"context"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/render"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/slicing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
)

type Syntax struct{}

func NewSyntax() Syntax { return Syntax{} }

var (
	_ indexing.Syntax = Syntax{}
	_ indexing.Slicer = Syntax{}
)

func (Syntax) Declarations(
	ctx context.Context,
	path string,
	source []byte,
) ([]entities.Decl, error) {
	decls, err := syntax.Declarations(ctx, path, source)
	if err != nil {
		return nil, fmt.Errorf("go declarations: %w", err)
	}

	return decls, nil
}

func (Syntax) Render(ctx context.Context, source []byte, keep []entities.DeclKey) ([]byte, error) {
	rendered, err := render.Render(ctx, source, keep)
	if err != nil {
		return nil, fmt.Errorf("go render: %w", err)
	}

	return rendered, nil
}

func (Syntax) Slice(
	ctx context.Context,
	fn entities.Decl,
	source []byte,
) (indexing.Sliced, bool, error) {
	sliced, found, err := slicing.Slice(ctx, fn, source)
	if err != nil {
		return indexing.Sliced{}, false, fmt.Errorf("go slice: %w", err)
	}

	return sliced, found, nil
}

func (Syntax) Needed(source []byte, text string) []string {
	return render.Needed(source, text)
}

func (Syntax) Admit(_ context.Context, source []byte, specs []string) ([]byte, error) {
	admitted, err := render.Admit(source, specs)
	if err != nil {
		return nil, fmt.Errorf("go imports: %w", err)
	}

	return admitted, nil
}
