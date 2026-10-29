package golang

import (
	"context"

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
	return syntax.Declarations(ctx, path, source)
}

func (Syntax) Render(ctx context.Context, source []byte, keep []entities.DeclKey) ([]byte, error) {
	return render.Render(ctx, source, keep)
}

func (Syntax) Slice(
	ctx context.Context,
	fn entities.Decl,
	source []byte,
) (indexing.Sliced, bool, error) {
	return slicing.Slice(ctx, fn, source)
}
