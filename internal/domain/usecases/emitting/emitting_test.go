package emitting

import (
	"bytes"
	"context"
	"fmt"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// key builds a func declaration key, which is all most test cases need to
// name a piece of a file.
func key(file, name string) entities.DeclKey {
	return entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name}
}

// decl attaches a span to a key so it can seed a graph that chunksFor reads
// Span and Unit from.
func decl(k entities.DeclKey, span entities.Span) entities.Decl {
	return entities.Decl{Key: k, Unit: entities.UnitID(k.File), Span: span}
}

// growingRenderer stands in for a language: it emits one line per declaration
// it was told to keep, which is enough to prove a file grows across commits
// without depending on a real syntax tree.
func growingRenderer(ctrl *gomock.Controller) *MockRenderer {
	renderer := NewMockRenderer(ctrl)
	renderer.EXPECT().
		Render(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ []byte, keep []entities.DeclKey) ([]byte, error) {
			var out bytes.Buffer
			for _, d := range keep {
				fmt.Fprintf(&out, "%s\n", d.Name)
			}

			return out.Bytes(), nil
		}).
		AnyTimes()

	return renderer
}

// countingSink hands back a Handle built from the content it received, so a
// test can tell which write produced which blob without inspecting a call log.
func countingSink(ctrl *gomock.Controller) *MockSink {
	sink := NewMockSink(ctrl)
	sink.EXPECT().
		Write(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, content []byte, _ FileMode) (Handle, error) {
			return Handle(content), nil
		}).
		AnyTimes()

	return sink
}

func reviewedPlan() entities.Plan {
	return entities.Plan{Commits: []entities.PlannedCommit{
		{Position: 0, Subject: "feat: One", Decls: []entities.DeclKey{key("a.go", "One")}},
		{
			Position: 1,
			Subject:  "feat: Two",
			Decls:    []entities.DeclKey{key("a.go", "Two")},
		},
	}}
}
