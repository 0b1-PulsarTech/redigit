package chunking

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

//go:generate go tool -modfile=../../../../tools/go.mod mockgen -source=ports.go -destination=ports_mock_test.go -package=chunking

// Source reads a path's content in the tree being rebuilt.
type Source interface {
	Read(ctx context.Context, path string) ([]byte, error)
}

// Revisions reads a path's content as one commit of the published history
// held it. A path the revision does not hold must report an error rather than
// empty content: a chunk absent from a file and a chunk in a file that is not
// there yet are different answers, and only one of them means "born later".
type Revisions interface {
	ReadAt(ctx context.Context, rev, path string) ([]byte, error)
}

// Chunker splits one file into the pieces that can be born apart. It is
// declared here rather than imported so this use case depends on the shape it
// needs, not on the package that happens to supply it.
type Chunker interface {
	Name() string
	Handles(path string) bool
	Chunks(ctx context.Context, path string, source []byte) ([]entities.Decl, error)
}
