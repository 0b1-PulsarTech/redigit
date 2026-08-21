package rewriting

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// Renderer writes a file back holding only the declarations named. It is the
// same contract the indexing stage reads files through, declared again here
// because this is the only thing this stage needs from a language.
type Renderer interface {
	Render(ctx context.Context, source []byte, keep []entities.DeclKey) ([]byte, error)
}

// Source reads a path's final content. Every intermediate version is rendered
// from the final text rather than from an intermediate blob, which is what lets
// a file grow declaration by declaration instead of being born whole.
type Source interface {
	Read(ctx context.Context, path string) ([]byte, error)
}

// History writes an ordered sequence and proves the result. Verify compares
// trees, never commit identifiers: every identifier changes on every rebuild,
// so comparing them is a check that can only ever fail.
type History interface {
	Write(ctx context.Context, ref string, commits []Commit) (string, error)
	Verify(ctx context.Context, gotRef, wantRef string) error
}

//go:generate go tool -modfile=../../../../tools/go.mod mockgen -source=ports.go -destination=ports_mock_test.go -package=rewriting
