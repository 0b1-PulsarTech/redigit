package config

import (
	"context"
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// Chunker splits one file into the pieces that can be born apart. It is the
// shape internal/infra/chunk implements, declared here so this package depends
// on what it needs rather than on the package that supplies it.
type Chunker interface {
	Name() string
	Handles(path string) bool
	Chunks(ctx context.Context, path string, source []byte) ([]entities.Decl, error)
}

// Renderer is the chunker that writes its own file back, for a format where
// concatenating a subset of the spans does not produce a valid file.
type Renderer interface {
	Render(source []byte, keep []entities.DeclKey) ([]byte, error)
}

// Syntax reads the files a repository needs to build but that no language
// server understands - go.mod, go.work, query files - answering Declarations
// by chunking and Render by concatenating the pieces a commit keeps.
type Syntax struct {
	chunkers []Chunker
}

var _ indexing.Syntax = Syntax{}

// NewSyntax tries chunkers in order and gives the first that claims a path the
// whole file.
func NewSyntax(chunkers []Chunker) Syntax { return Syntax{chunkers: chunkers} }

// Declarations cuts path into its pieces. A path no chunker claims yields no
// declaration, which leaves the file out of the graph rather than inventing a
// shape for it.
func (s Syntax) Declarations(
	ctx context.Context,
	path string,
	source []byte,
) ([]entities.Decl, error) {
	for _, chunker := range s.chunkers {
		if !chunker.Handles(path) {
			continue
		}

		decls, err := chunker.Chunks(ctx, path, source)
		if err != nil {
			return nil, fmt.Errorf("chunk %s with %s: %w", path, chunker.Name(), err)
		}

		return decls, nil
	}

	return nil, nil
}

// Render writes back the file holding only keep. A chunker implementing
// Renderer writes its own file, because a format with BLOCK structure cannot
// be rebuilt by concatenation. Everything else tiles end to end, so
// concatenating the kept spans in order is the whole of it.
func (s Syntax) Render(
	_ context.Context,
	source []byte,
	keep []entities.DeclKey,
) ([]byte, error) {
	if len(keep) == 0 {
		return nil, nil
	}

	if renderer, own := s.rendererFor(keep[0].File); own {
		out, err := renderer.Render(source, keep)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", keep[0].File, err)
		}

		return out, nil
	}

	spans, err := s.spansOf(source, keep)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(spans, func(a, b entities.Span) int { return a.Start - b.Start })

	out := make([]byte, 0, len(source))
	for _, span := range spans {
		out = append(out, source[span.Start:span.End]...)
	}

	return out, nil
}

// spansOf re-chunks source and returns the span of each kept key. Re-chunking
// rather than trusting a span handed in: the caller's spans were measured
// against the FINAL file, which is the file Render is given, and only
// re-chunking proves the two agree.
func (s Syntax) spansOf(source []byte, keep []entities.DeclKey) ([]entities.Span, error) {
	if len(keep) == 0 {
		return nil, nil
	}

	decls, err := s.Declarations(context.Background(), keep[0].File, source)
	if err != nil {
		return nil, err
	}

	byKey := make(map[entities.DeclKey]entities.Span, len(decls))
	for _, decl := range decls {
		byKey[decl.Key] = decl.Span
	}

	spans := make([]entities.Span, 0, len(keep))

	for _, key := range keep {
		if span, found := byKey[key]; found {
			spans = append(spans, span)
		}
	}

	return spans, nil
}

// rendererFor reports the chunker that claims path and writes its own file.
//
//nolint:ireturn // the interface is what the registry holds.
func (s Syntax) rendererFor(path string) (Renderer, bool) {
	for _, chunker := range s.chunkers {
		if !chunker.Handles(path) {
			continue
		}

		renderer, writes := chunker.(Renderer)

		return renderer, writes
	}

	return nil, false
}
