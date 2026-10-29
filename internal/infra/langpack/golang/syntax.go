package golang

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/render"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/syntax"
)

// Syntax implements indexing.Syntax and indexing.Slicer for Go source. It
// composes the declaration reader, the renderer and the statement slicer.
type Syntax struct{}

// NewSyntax builds the Go syntax adapter. It carries no state: every call is
// self-contained on the source it is given.
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
