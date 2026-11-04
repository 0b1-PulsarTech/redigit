package emitting

import (
	"bytes"
	"context"
	"errors"
	"reflect"
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
		name   string
		source string
		edits  []Piece
		want   string
	}{
		{name: "no edit keeps the text", source: "aaa bbb", want: "aaa bbb"},
		{
			name:   "one span is replaced",
			source: "aaa bbb",
			edits:  []Piece{{Span: entities.Span{Start: 0, End: 3}, Text: "PRE"}},
			want:   "PRE bbb",
		},
		{
			name:   "two spans given out of order, one longer than its original",
			source: "aaa bbb",
			edits: []Piece{
				{Span: entities.Span{Start: 4, End: 7}, Text: "LONGER"},
				{Span: entities.Span{Start: 0, End: 3}, Text: "x"},
			},
			want: "x LONGER",
		},
		{
			name:   "two declarations sharing one span are replaced once",
			source: "aaa bbb",
			edits: []Piece{
				{Span: entities.Span{Start: 0, End: 3}, Text: "PRE"},
				{Span: entities.Span{Start: 0, End: 3}, Text: "PRE"},
			},
			want: "PRE bbb",
		},
		{
			name:   "adjacent spans",
			source: "aaa bbb ccc",
			edits: []Piece{
				{Span: entities.Span{Start: 0, End: 3}, Text: "1"},
				{Span: entities.Span{Start: 3, End: 4}, Text: "-"},
			},
			want: "1-bbb ccc",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := splice([]byte(testCase.source), testCase.edits)
			if err != nil || string(got) != testCase.want {
				t.Fatalf("splice() = %q, %v, want %q", got, err, testCase.want)
			}
		})
	}
}

func TestSpliceRefusals(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
		edits  []Piece
	}{
		{
			name:   "two declarations sharing one span with differing text",
			source: "aaa bbb",
			edits: []Piece{
				{Span: entities.Span{Start: 0, End: 3}, Text: "1"},
				{Span: entities.Span{Start: 0, End: 3}, Text: "2"},
			},
		},
		{
			name:   "overlapping spans",
			source: "aaa bbb",
			edits: []Piece{
				{Span: entities.Span{Start: 0, End: 5}, Text: "x"},
				{Span: entities.Span{Start: 4, End: 7}, Text: "y"},
			},
		},
		{
			name:   "a span past the end of the file",
			source: "aaa",
			edits:  []Piece{{Span: entities.Span{Start: 2, End: 9}, Text: "x"}},
		},
		{
			name:   "an inverted span",
			source: "aaa",
			edits:  []Piece{{Span: entities.Span{Start: 2, End: 1}, Text: "x"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := splice(
				[]byte(testCase.source),
				testCase.edits,
			); !errors.Is(
				err,
				ErrSplice,
			) {
				t.Fatalf("splice() error = %v, want %v", err, ErrSplice)
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

type admittingRenderer struct {
	*MockRenderer

	files   []string
	specs   [][]string
	failure error
}

func (renderer *admittingRenderer) Admit(
	_ context.Context,
	file string,
	source []byte,
	specs []string,
) ([]byte, error) {
	renderer.files = append(renderer.files, file)
	renderer.specs = append(renderer.specs, specs)

	return append([]byte("+"), source...), renderer.failure
}

func admitFixture(ctrl *gomock.Controller) (entities.Plan, entities.Graph, *MockSource) {
	plan, graph := anchoredFixture()
	plan.Versions[key("a.go", "A")][0].Imports = []string{"bytes"}
	plan.Versions[key("a.go", "B")][0].Imports = []string{"bytes", `str "strings"`}

	source := NewMockSource(ctrl)
	source.EXPECT().
		Read(gomock.Any(), gomock.Any()).
		Return([]byte("aaa bbb"), FileMode("100644"), nil).
		AnyTimes()

	return plan, graph, source
}

type admitCase struct {
	name      string
	position  int
	failure   error
	plain     bool
	noImports bool
	want      string
	wantSpecs [][]string
	wantErr   error
}

func TestEmitterAdmitsThePreimageImports(t *testing.T) {
	t.Parallel()

	failure := errors.New("clash")
	both := [][]string{{"bytes"}, {"bytes", `str "strings"`}}

	testCases := []admitCase{
		{
			name:      "the imports of the active pre-images are admitted before the render",
			position:  1,
			want:      "+preA preB",
			wantSpecs: both,
		},
		{
			name: "one active pre-image admits only its own imports", position: 0,
			want: "+preA bbb", wantSpecs: both,
		},
		{
			name: "no admission once the anchor writes the final text", position: 2,
			want: "aaa bbb", wantSpecs: both,
		},
		{
			name: "a pre-image with no imports admits nothing", position: 1, noImports: true,
			want: "preA preB",
		},
		{
			name: "a renderer that cannot admit leaves the splice as it is", position: 1,
			plain: true, want: "preA preB",
		},
		{
			name: "a refused admission is reported", position: 1, failure: failure,
			wantErr: failure,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			plan, graph, source := admitFixture(ctrl)

			if testCase.noImports {
				dropImports(plan)
			}

			admitting := &admittingRenderer{
				MockRenderer: echoRenderer(ctrl),
				failure:      testCase.failure,
			}

			var renderer Renderer = admitting
			if testCase.plain {
				renderer = admitting.MockRenderer
			}

			commits, err := New(source, renderer, countingSink(ctrl)).
				Emit(context.Background(), plan, graph)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Emit() error = %v, want %v", err, testCase.wantErr)
			}

			if err != nil {
				return
			}

			assertAdmitted(t, commits[testCase.position], admitting, testCase)
		})
	}
}

func assertAdmitted(
	t *testing.T,
	commit Commit,
	admitting *admittingRenderer,
	testCase admitCase,
) {
	t.Helper()

	for _, blob := range commit.Blobs {
		if blob.Path == "a.go" && string(blob.Handle) != testCase.want {
			t.Fatalf("a.go = %q, want %q", blob.Handle, testCase.want)
		}
	}

	if !reflect.DeepEqual(admitting.specs, testCase.wantSpecs) {
		t.Fatalf("Admit() specs = %v, want %v", admitting.specs, testCase.wantSpecs)
	}
}

func dropImports(plan entities.Plan) {
	for _, versions := range plan.Versions {
		versions[0].Imports = nil
	}
}
