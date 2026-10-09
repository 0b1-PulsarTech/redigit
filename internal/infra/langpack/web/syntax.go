package web

import (
	"context"
	"path"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// fileDeclName matches the whole-file declaration the opaque pack gives these
// files, so moving them under this pack keeps their keys.
const fileDeclName = "<file>"

const (
	tsExt  = ".ts"
	jsExt  = ".js"
	mjsExt = ".mjs"
)

// Tree reads a path of the final tree: whether a script under test/ is a test
// or a helper depends on whether another file imports it.
type Tree interface {
	Read(ctx context.Context, path string) ([]byte, error)
}

// Syntax gives each .svelte, .ts and Node script one whole-file declaration.
// A component is one unit to its readers, and cutting TypeScript would need a
// parser this tool does not have.
type Syntax struct {
	tree *treeIndex
}

var _ indexing.Syntax = Syntax{}

func NewSyntax(files []string, tree Tree) Syntax {
	return Syntax{
		tree: &treeIndex{
			space: newWorkspace(files, func(ctx context.Context, file string) ([]byte, error) {
				return tree.Read(ctx, file)
			}),
		},
	}
}

func Extensions() []string { return []string{".svelte", tsExt, mjsExt, jsExt} }

func (s Syntax) Declarations(
	ctx context.Context,
	file string,
	source []byte,
) ([]entities.Decl, error) {
	if err := s.tree.load(ctx); err != nil {
		return nil, err
	}

	return []entities.Decl{{
		Key:      entities.DeclKey{File: file, Kind: entities.KindOther, Name: fileDeclName},
		Unit:     entities.UnitID(path.Dir(file)),
		Span:     entities.Span{Start: 0, End: len(source)},
		Selector: entities.Location{File: file},
		IsTest:   s.tree.isTest(file),
		Atom:     s.tree.atoms[file],
	}}, nil
}

func (Syntax) Render(_ context.Context, source []byte, keep []entities.DeclKey) ([]byte, error) {
	if len(keep) == 0 {
		return nil, nil
	}

	return source, nil
}
