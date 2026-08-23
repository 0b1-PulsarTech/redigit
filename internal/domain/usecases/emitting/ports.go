package emitting

import (
	"context"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// FileMode is a git blob mode. The emitter never interprets it, only carries
// it from Source through to Sink unchanged, because a symlink or an
// executable script reconstructed as a plain file is a defect the final tree
// invariant catches far too late to explain.
type FileMode string

// Handle identifies where a blob ended up: a loose git object id, a path on
// disk, anything the sink's owner chose. The emitter treats it as opaque and
// only carries it forward into a Blob.
type Handle string

// Source reads a path's FINAL content and the mode it should be written with.
// Every commit's version of a file renders from this same final text rather
// than from an intermediate blob, which is what lets a file grow declaration
// by declaration across commits instead of being born whole.
type Source interface {
	Read(ctx context.Context, path string) ([]byte, FileMode, error)
}

// Renderer writes a file back holding only the declarations named. It is the
// same contract the rewriting stage renders through, declared again here
// because this package must not import that one.
type Renderer interface {
	Render(ctx context.Context, source []byte, keep []entities.DeclKey) ([]byte, error)
}

// Sink stores one file's content, written with one mode, and reports back a
// Handle for it. Whether that handle names a loose git object or a path on
// disk is a decision the sink's owner makes; the emitter neither knows nor
// cares.
//
// The path travels even though a content-addressed sink ignores it: a sink
// that addresses BY path cannot work at all without it, and a port that only
// serves one of its two implementations is not a seam.
type Sink interface {
	Write(ctx context.Context, path string, content []byte, mode FileMode) (Handle, error)
}

//go:generate go tool -modfile=../../../../tools/go.mod mockgen -source=ports.go -destination=ports_mock_test.go -package=emitting
