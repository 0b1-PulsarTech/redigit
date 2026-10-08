package emitting

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// listDeriver writes the sorted names of the files feeding it, one per line.
type listDeriver struct{}

func (listDeriver) Output(file string) bool { return file == "m/sum" }

func (listDeriver) Feeds(_, input string) bool { return strings.HasSuffix(input, ".sql") }

func (listDeriver) Derive(_ string, inputs map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(inputs))
	for name, content := range inputs {
		names = append(names, fmt.Sprintf("%s=%d", name, len(content)))
	}

	sort.Strings(names)

	return []byte(strings.Join(names, ",")), nil
}

func TestEmitterRecomputesADerivedFileForTheFilesPresent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	source := NewMockSource(ctrl)
	source.EXPECT().
		Read(gomock.Any(), gomock.Any()).
		Return([]byte("final"), FileMode("100644"), nil).
		AnyTimes()

	graph := entities.NewGraph([]entities.Decl{
		decl(key("m/0001.sql", "A"), entities.Span{}),
		decl(key("m/0002.sql", "B"), entities.Span{}),
		decl(key("m/sum", "Sum"), entities.Span{}),
	}, nil, nil)
	plan := entities.Plan{Commits: []entities.PlannedCommit{
		{
			Position: 0,
			Subject:  "a",
			Decls:    []entities.DeclKey{key("m/0001.sql", "A"), key("m/sum", "Sum")},
		},
		{Position: 1, Subject: "b", Decls: []entities.DeclKey{key("m/0002.sql", "B")}},
	}}

	commits, err := New(source, growingRenderer(ctrl), countingSink(ctrl), listDeriver{}).
		Emit(context.Background(), plan, graph)
	if err != nil {
		t.Fatalf("Emit() = %v", err)
	}

	sums := map[int]string{}

	for i, commit := range commits {
		for _, blob := range commit.Blobs {
			if blob.Path == "m/sum" {
				sums[i] = string(blob.Handle)
			}
		}
	}

	want := map[int]string{
		0: "m/0001.sql=2",
		1: "m/0001.sql=2,m/0002.sql=2",
	}
	for i, text := range want {
		if sums[i] != text {
			t.Errorf("commit %d sum = %q, want %q", i, sums[i], text)
		}
	}
}
