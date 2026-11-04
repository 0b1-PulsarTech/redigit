package emitting

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func echoRenderer(ctrl *gomock.Controller) *MockRenderer {
	renderer := NewMockRenderer(ctrl)
	renderer.EXPECT().
		Render(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, source []byte, _ []entities.DeclKey) ([]byte, error) {
			return bytes.Clone(source), nil
		}).
		AnyTimes()

	return renderer
}

func TestSplice(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		source  string
		edits   []edit
		want    string
		wantErr error
	}{
		{name: "no edit keeps the text", source: "aaa bbb", want: "aaa bbb"},
		{
			name:   "one span is replaced",
			source: "aaa bbb",
			edits:  []edit{{span: entities.Span{Start: 0, End: 3}, text: "PRE"}},
			want:   "PRE bbb",
		},
		{
			name:   "two spans given out of order, one longer than its original",
			source: "aaa bbb",
			edits: []edit{
				{span: entities.Span{Start: 4, End: 7}, text: "LONGER"},
				{span: entities.Span{Start: 0, End: 3}, text: "x"},
			},
			want: "x LONGER",
		},
		{
			name:   "two declarations sharing one span are replaced once",
			source: "aaa bbb",
			edits: []edit{
				{span: entities.Span{Start: 0, End: 3}, text: "PRE"},
				{span: entities.Span{Start: 0, End: 3}, text: "PRE"},
			},
			want: "PRE bbb",
		},
		{
			name:   "overlapping spans",
			source: "aaa bbb",
			edits: []edit{
				{span: entities.Span{Start: 0, End: 5}, text: "x"},
				{span: entities.Span{Start: 4, End: 7}, text: "y"},
			},
			wantErr: ErrSplice,
		},
		{
			name:    "a span past the end of the file",
			source:  "aaa",
			edits:   []edit{{span: entities.Span{Start: 2, End: 9}, text: "x"}},
			wantErr: ErrSplice,
		},
		{
			name:    "an inverted span",
			source:  "aaa",
			edits:   []edit{{span: entities.Span{Start: 2, End: 1}, text: "x"}},
			wantErr: ErrSplice,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := splice([]byte(testCase.source), testCase.edits)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("splice() error = %v, want %v", err, testCase.wantErr)
			}

			if string(got) != testCase.want {
				t.Fatalf("splice() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func anchoredFixture() (entities.Plan, entities.Graph) {
	first, second, third := key("a.go", "A"), key("a.go", "B"), key("b.go", "C")
	graph := entities.NewGraph(
		[]entities.Decl{
			decl(first, entities.Span{Start: 0, End: 3}),
			decl(second, entities.Span{Start: 4, End: 7}),
			decl(third, entities.Span{Start: 0, End: 3}),
		},
		nil,
		nil,
	)

	plan := entities.Plan{
		Commits: []entities.PlannedCommit{
			{Position: 0, Subject: "feat: A and C", Decls: []entities.DeclKey{first, third}},
			{Position: 1, Subject: "feat: B", Decls: []entities.DeclKey{second}},
			{Position: 2, Subject: "fix: A and B", Changes: []entities.DeclKey{first, second}},
		},
		Versions: map[entities.DeclKey][]entities.Version{
			first:  {{Text: "preA", Until: 2}},
			second: {{Text: "preB", Until: 2}},
		},
	}

	return plan, graph
}

func TestEmitterSplicesPreimagesUntilTheirAnchor(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		position  int
		path      string
		want      string
		wantFiles int
	}{
		{
			name:     "before the anchor the first declaration carries its pre-image",
			position: 0, path: "a.go", want: "preA bbb", wantFiles: 2,
		},
		{
			name:     "a declaration born after the pre-image splice is applied too",
			position: 1, path: "a.go", want: "preA preB", wantFiles: 1,
		},
		{
			name:     "the anchor writes the final text of both declarations",
			position: 2, path: "a.go", want: "aaa bbb", wantFiles: 1,
		},
		{
			name:     "a declaration with no version is left alone",
			position: 0, path: "b.go", want: "aaa bbb", wantFiles: 2,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			plan, graph := anchoredFixture()

			source := NewMockSource(ctrl)
			source.EXPECT().
				Read(gomock.Any(), gomock.Any()).
				Return([]byte("aaa bbb"), FileMode("100644"), nil).
				AnyTimes()

			commits, err := New(source, echoRenderer(ctrl), countingSink(ctrl)).
				Emit(context.Background(), plan, graph)
			if err != nil {
				t.Fatalf("Emit() = %v", err)
			}

			var got string

			for _, blob := range commits[testCase.position].Blobs {
				if blob.Path == testCase.path {
					got = string(blob.Handle)
				}
			}

			if got != testCase.want {
				t.Fatalf("commit %d wrote %q for %s, want %q",
					testCase.position, got, testCase.path, testCase.want)
			}

			if len(commits[testCase.position].Blobs) != testCase.wantFiles {
				t.Fatalf("commit %d wrote %d files, want %d",
					testCase.position, len(commits[testCase.position].Blobs), testCase.wantFiles)
			}
		})
	}
}

func TestEmitterReportsNoRiskForADeclarationHeldAtItsPreimage(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		provider string
		wantWIP  bool
	}{
		{name: "a pre-image does not reach for what the final text needs", provider: "B"},
		{name: "a declaration without a version still does", provider: "C", wantWIP: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			plan, graph := anchoredFixture()
			dependent := key("a.go", "A")

			if testCase.provider == "C" {
				dependent = key("b.go", "C")
			}

			edges := []entities.Edge{{
				Dependent: dependent, Provider: key("a.go", "B"), Reason: entities.ReasonReference,
			}}
			graph = entities.NewGraph(graph.Decls(), edges, nil)

			source := NewMockSource(ctrl)
			source.EXPECT().
				Read(gomock.Any(), gomock.Any()).
				Return([]byte("aaa bbb"), FileMode("100644"), nil).
				AnyTimes()

			commits, err := New(source, echoRenderer(ctrl), countingSink(ctrl)).
				Emit(context.Background(), plan, graph)
			if err != nil {
				t.Fatalf("Emit() = %v", err)
			}

			if commits[0].IsWIP != testCase.wantWIP {
				t.Fatalf("commit 0 wip = %v, want %v", commits[0].IsWIP, testCase.wantWIP)
			}
		})
	}
}

func TestEmitterRefusesAVersionOfAnUnknownDeclaration(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		ghost   entities.DeclKey
		wantErr error
	}{
		{
			name:    "a version of a declaration the graph does not hold",
			ghost:   key("a.go", "Ghost"),
			wantErr: apperr.ErrIncomplete,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			plan, graph := anchoredFixture()
			plan.Versions[testCase.ghost] = []entities.Version{{Text: "x", Until: 2}}
			plan.Commits[0].Decls = append(plan.Commits[0].Decls, testCase.ghost)

			source := NewMockSource(ctrl)
			source.EXPECT().
				Read(gomock.Any(), gomock.Any()).
				Return([]byte("aaa bbb"), FileMode("100644"), nil).
				AnyTimes()

			_, err := New(source, echoRenderer(ctrl), countingSink(ctrl)).
				Emit(context.Background(), plan, graph)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Emit() error = %v, want ErrIncomplete", err)
			}
		})
	}
}
