package dart

import (
	"context"
	"errors"
	"maps"
	"path"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

var errMissing = errors.New("missing")

// fakeTree is a final tree held in memory: the rules and Render read only
// through a workspace, so no test needs the disk except the pack's own.
type fakeTree map[string]string

func (t fakeTree) Read(_ context.Context, file string) ([]byte, error) {
	source, found := t[file]
	if !found {
		return nil, errMissing
	}

	return []byte(source), nil
}

func (t fakeTree) files() []string { return slices.Sorted(maps.Keys(t)) }

func (t fakeTree) space() *workspace { return newWorkspace(t.files(), t.Read) }

// declareAll reads every Dart file of the tree the way an indexing run would,
// and stands every other file for one whole-file declaration.
func declareAll(t *testing.T, tree fakeTree) []entities.Decl {
	t.Helper()

	var all []entities.Decl

	for _, file := range tree.files() {
		if path.Ext(file) != ".dart" {
			all = append(all, entities.Decl{Key: key(file, entities.KindOther, fileDeclName)})

			continue
		}

		decls, err := NewSyntax(
			nil,
			nil,
		).Declarations(context.Background(), file, []byte(tree[file]))
		if err != nil {
			t.Fatalf("Declarations(%s) error = %v", file, err)
		}

		all = append(all, decls...)
	}

	return all
}

// withLibrary is the library tree plus the files of extra.
func withLibrary(extra fakeTree) fakeTree {
	out := maps.Clone(library)
	maps.Copy(out, extra)

	return out
}

func key(file string, kind entities.DeclKind, name string) entities.DeclKey {
	return entities.DeclKey{File: file, Kind: kind, Name: name}
}

func texts(toks []token) []string {
	out := make([]string, 0, len(toks))

	for _, tok := range toks {
		out = append(out, tok.text)
	}

	return out
}
