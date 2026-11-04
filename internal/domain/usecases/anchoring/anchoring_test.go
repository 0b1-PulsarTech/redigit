package anchoring

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/lineage"
)

var errBroken = errors.New("broken blob")

type linesParser struct{}

func (linesParser) Declarations(
	_ context.Context,
	path string,
	src []byte,
) ([]entities.Decl, error) {
	var decls []entities.Decl

	offset := 0

	for line := range strings.Lines(string(src)) {
		text := strings.TrimSuffix(line, "\n")
		if strings.HasPrefix(text, "BROKEN") {
			return nil, errBroken
		}

		if name, _, found := strings.Cut(text, " "); found {
			decls = append(decls, entities.Decl{
				Key:  entities.DeclKey{File: path, Kind: entities.KindFunc, Name: name},
				Span: entities.Span{Start: offset, End: offset + len(text)},
			})
		}

		offset += len(line)
	}

	return decls, nil
}

func goLanguages() []lineage.Language {
	return []lineage.Language{{Extensions: []string{".go"}, Parser: linesParser{}}}
}

func fkey(name string) entities.DeclKey {
	return entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: name}
}

func graphOf(files map[string]string) entities.Graph {
	var decls []entities.Decl

	for path, text := range files {
		parsed, _ := linesParser{}.Declarations(context.Background(), path, []byte(text))
		decls = append(decls, parsed...)
	}

	return entities.NewGraph(decls, nil, nil)
}

func mapBlobs(ctrl *gomock.Controller, blobs map[string]string) *MockRevisions {
	revisions := NewMockRevisions(ctrl)
	revisions.EXPECT().
		Blob(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, oid string) ([]byte, error) {
			return []byte(blobs[oid]), nil
		}).
		AnyTimes()

	return revisions
}

func mapFinals(ctrl *gomock.Controller, files map[string]string) *MockFinals {
	finals := NewMockFinals(ctrl)
	finals.EXPECT().
		Read(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, path string) ([]byte, error) {
			return []byte(files[path]), nil
		}).
		AnyTimes()

	return finals
}
