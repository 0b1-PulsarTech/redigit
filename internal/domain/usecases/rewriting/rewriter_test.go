package rewriting

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func key(file, name string) entities.DeclKey {
	return entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name}
}

// growingRenderer stands in for a language: it emits one line per declaration
// it was told to keep, which is enough to prove a file grows across commits.
func growingRenderer(ctrl *gomock.Controller) *MockRenderer {
	renderer := NewMockRenderer(ctrl)
	renderer.EXPECT().
		Render(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ []byte, keep []entities.DeclKey) ([]byte, error) {
			var out bytes.Buffer
			for _, decl := range keep {
				fmt.Fprintf(&out, "%s\n", decl.Name)
			}

			return out.Bytes(), nil
		}).
		AnyTimes()

	return renderer
}

func reviewedPlan() entities.Plan {
	return entities.Plan{Commits: []entities.PlannedCommit{
		{Position: 0, Subject: "feat: One", Decls: []entities.DeclKey{key("a.go", "One")}},
		{Position: 1, Subject: "feat: Two", Decls: []entities.DeclKey{key("a.go", "Two")}},
	}}
}

func TestRewriterGrowsAFileAcrossCommits(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	source := NewMockSource(ctrl)
	// The final text is read once and reused: every intermediate version is
	// cut from it, never from an intermediate blob.
	source.EXPECT().Read(gomock.Any(), "a.go").Return([]byte("final"), nil).Times(1)

	var written []Commit

	history := NewMockHistory(ctrl)
	history.EXPECT().
		Write(gomock.Any(), "refs/heads/out", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, commits []Commit) (string, error) {
			written = commits

			return "abc123", nil
		})
	history.EXPECT().Verify(gomock.Any(), "abc123", "refs/heads/target").Return(nil)

	ref, err := New(source, growingRenderer(ctrl), history, Config{
		Out:    "refs/heads/out",
		Target: "refs/heads/target",
	}).Rewrite(context.Background(), reviewedPlan())
	if err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}

	if ref != "abc123" {
		t.Fatalf("ref = %q, want abc123", ref)
	}

	testCases := []struct {
		name  string
		index int
		want  string
	}{
		{"the first commit holds only the first declaration", 0, "One\n"},
		{"the second holds both, so the file grew rather than being born whole", 1, "One\nTwo\n"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := string(written[testCase.index].Writes[0].Content)
			if got != testCase.want {
				t.Fatalf("commit %d wrote %q, want %q", testCase.index, got, testCase.want)
			}
		})
	}
}

func TestRewriterRefusesAPlanItCannotTrust(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		plan entities.Plan
	}{
		{"an empty plan", entities.Plan{}},
		{
			"a declaration in no commit is a hole in the final tree",
			entities.Plan{
				Commits:  reviewedPlan().Commits,
				Unplaced: []entities.DeclKey{key("z.go", "Lost")},
			},
		},
		{
			"a generated subject means nobody reviewed the grouping",
			entities.Plan{Commits: []entities.PlannedCommit{{
				Subject: entities.DraftMarker + "feat(a): One",
				Decls:   []entities.DeclKey{key("a.go", "One")},
			}}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)

			_, err := New(
				NewMockSource(ctrl),
				NewMockRenderer(ctrl),
				NewMockHistory(ctrl),
				Config{Out: "refs/heads/out", Target: "refs/heads/target"},
			).Rewrite(context.Background(), testCase.plan)

			if !errors.Is(err, apperr.ErrIncomplete) {
				t.Fatalf("Rewrite() = %v, want %v", err, apperr.ErrIncomplete)
			}
		})
	}
}

func TestRewriterAllowsDraftsOnlyWhenAsked(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	source := NewMockSource(ctrl)
	source.EXPECT().Read(gomock.Any(), gomock.Any()).Return([]byte("final"), nil)

	history := NewMockHistory(ctrl)
	history.EXPECT().Write(gomock.Any(), gomock.Any(), gomock.Any()).Return("sha", nil)
	history.EXPECT().Verify(gomock.Any(), "sha", gomock.Any()).Return(nil)

	plan := entities.Plan{Commits: []entities.PlannedCommit{{
		Subject: entities.DraftMarker + "feat(a): One",
		Decls:   []entities.DeclKey{key("a.go", "One")},
	}}}

	if _, err := New(source, growingRenderer(ctrl), history, Config{
		Out:                "refs/heads/out",
		Target:             "refs/heads/target",
		AllowDraftSubjects: true,
	}).Rewrite(context.Background(), plan); err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
}

func TestRewriterReportsAFailedInvariant(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	source := NewMockSource(ctrl)
	source.EXPECT().Read(gomock.Any(), gomock.Any()).Return([]byte("final"), nil)

	history := NewMockHistory(ctrl)
	history.EXPECT().Write(gomock.Any(), gomock.Any(), gomock.Any()).Return("sha", nil)
	history.EXPECT().
		Verify(gomock.Any(), "sha", gomock.Any()).
		Return(fmt.Errorf("%w: 2 paths diverge", apperr.ErrInvariant))

	ref, err := New(source, growingRenderer(ctrl), history, Config{
		Out:    "refs/heads/out",
		Target: "refs/heads/target",
	}).Rewrite(context.Background(), reviewedPlan())

	if !errors.Is(err, apperr.ErrInvariant) {
		t.Fatalf("Rewrite() = %v, want %v", err, apperr.ErrInvariant)
	}

	// The ref comes back even so: a caller inspecting what went wrong needs
	// the history that was written, not just the news that it is wrong.
	if ref != "sha" {
		t.Fatalf("ref = %q, want it returned alongside the error", ref)
	}
}

func TestRewriterRefusesACommitThatChangesNothing(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	source := NewMockSource(ctrl)
	source.EXPECT().Read(gomock.Any(), gomock.Any()).Return([]byte("final"), nil).AnyTimes()

	// A renderer that emits nothing stands for a file whose declarations are
	// all still unborn. The commit then carries no change, and an empty commit
	// wearing a feature subject claims work that happened somewhere else.
	renderer := NewMockRenderer(ctrl)
	renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	_, err := New(source, renderer, NewMockHistory(ctrl), Config{
		Out:    "refs/heads/out",
		Target: "refs/heads/target",
	}).Rewrite(context.Background(), reviewedPlan())

	if !errors.Is(err, apperr.ErrIncomplete) {
		t.Fatalf("Rewrite() = %v, want %v", err, apperr.ErrIncomplete)
	}
}

func TestRewriterPropagatesFailures(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")

	testCases := []struct {
		name  string
		setup func(*gomock.Controller) (*MockSource, *MockRenderer, *MockHistory)
	}{
		{
			"a file that cannot be read",
			func(ctrl *gomock.Controller) (*MockSource, *MockRenderer, *MockHistory) {
				source := NewMockSource(ctrl)
				source.EXPECT().Read(gomock.Any(), gomock.Any()).Return(nil, errBoom)

				return source, NewMockRenderer(ctrl), NewMockHistory(ctrl)
			},
		},
		{
			"a file that cannot be rendered",
			func(ctrl *gomock.Controller) (*MockSource, *MockRenderer, *MockHistory) {
				source := NewMockSource(ctrl)
				source.EXPECT().Read(gomock.Any(), gomock.Any()).Return([]byte("final"), nil)

				renderer := NewMockRenderer(ctrl)
				renderer.EXPECT().
					Render(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errBoom)

				return source, renderer, NewMockHistory(ctrl)
			},
		},
		{
			"a history that cannot be written",
			func(ctrl *gomock.Controller) (*MockSource, *MockRenderer, *MockHistory) {
				source := NewMockSource(ctrl)
				source.EXPECT().Read(gomock.Any(), gomock.Any()).Return([]byte("final"), nil)

				history := NewMockHistory(ctrl)
				history.EXPECT().
					Write(gomock.Any(), gomock.Any(), gomock.Any()).
					Return("", errBoom)

				return source, growingRenderer(ctrl), history
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			source, renderer, history := testCase.setup(ctrl)

			_, err := New(source, renderer, history, Config{
				Out:    "refs/heads/out",
				Target: "refs/heads/target",
			}).Rewrite(context.Background(), reviewedPlan())

			if !errors.Is(err, errBoom) {
				t.Fatalf("Rewrite() = %v, want it to carry %v", err, errBoom)
			}
		})
	}
}
