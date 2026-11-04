package anchorprobe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

type wiring struct {
	dir         string
	check       func(string, verifying.Unit) (verifying.Outcome, error)
	render      func(context.Context, []byte, []entities.DeclKey) ([]byte, error)
	listErr     error
	isEmpty     bool
	checkoutErr error
	modulesErr  error
}

func passThrough(_ context.Context, source []byte, _ []entities.DeclKey) ([]byte, error) {
	return source, nil
}

func newProbe(t *testing.T, wired wiring) *Probe {
	t.Helper()

	ctrl := gomock.NewController(t)
	last := verifying.Commit{Oid: "last", Tree: "tree"}
	listed := []verifying.Commit{{Oid: "first", Tree: "t0"}, last}

	if wired.isEmpty {
		listed = nil
	}

	commits := NewMockCommits(ctrl)
	commits.EXPECT().List(gomock.Any(), "rev").Return(listed, wired.listErr).AnyTimes()

	checkout := NewMockCheckout(ctrl)
	checkout.EXPECT().
		Materialise(gomock.Any(), 0, last).
		Return(wired.dir, wired.checkoutErr).
		AnyTimes()

	modules := NewMockModules(ctrl)
	modules.EXPECT().
		List(gomock.Any(), last).
		Return(
			verifying.Listing{Modules: []verifying.Module{{Dir: ".", Language: "go"}}},
			wired.modulesErr,
		).
		AnyTimes()

	builder := NewMockBuilder(ctrl)
	builder.EXPECT().
		Check(gomock.Any(), wired.dir, gomock.Any()).
		DoAndReturn(func(_ any, slot string, unit verifying.Unit) (verifying.Outcome, error) {
			return wired.check(slot, unit)
		}).
		AnyTimes()

	render := wired.render
	if render == nil {
		render = passThrough
	}

	renderer := NewMockRenderer(ctrl)
	renderer.EXPECT().
		Render(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(render).
		AnyTimes()

	return New(t.Context(), Config{
		Commits: commits, Checkout: checkout, Modules: modules, Builder: builder,
		Renderer: renderer, Rev: "rev",
	})
}

func writeFile(t *testing.T, dir, text string) string {
	t.Helper()

	path := filepath.Join(dir, "a.go")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	return path
}

func recordingCheck(
	path, source string,
	seen *[]string,
) func(string, verifying.Unit) (verifying.Outcome, error) {
	return func(string, verifying.Unit) (verifying.Outcome, error) {
		content, err := os.ReadFile(path)
		if err != nil {
			return verifying.Outcome{}, fmt.Errorf("read: %w", err)
		}

		if string(content) != source {
			*seen = append(*seen, string(content))
		}

		return verifying.Outcome{Step: "vet"}, nil
	}
}

func TestBuildsChecksTheRenderedFileAndRestoresAfter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		source       string
		edit         anchoring.Edit
		trials       int
		wantSeen     string
		wantKeptName string
	}{
		{
			name:   "two trials each check the rendered edit and leave the tree as it was",
			source: "A last\nB other\n",
			edit: anchoring.Edit{
				File: "a.go", Span: entities.Span{End: 6}, Text: "A one",
				Keep: []entities.DeclKey{{File: "a.go", Kind: entities.KindFunc, Name: "A"}},
			},
			trials:       2,
			wantSeen:     "A one\nB other\nrendered\n",
			wantKeptName: "A",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := writeFile(t, dir, testCase.source)

			var (
				seen []string
				kept [][]entities.DeclKey
			)

			probe := newProbe(t, wiring{
				dir: dir,
				render: func(_ context.Context, source []byte, keep []entities.DeclKey) ([]byte, error) {
					kept = append(kept, keep)

					return append(source, "rendered\n"...), nil
				},
				check: recordingCheck(path, testCase.source, &seen),
			})

			for range testCase.trials {
				verdict, err := probe.Builds(t.Context(), []anchoring.Edit{testCase.edit})
				if err != nil || !verdict.IsBuilt {
					t.Fatalf("Builds() = %+v, %v", verdict, err)
				}
			}

			if len(seen) != testCase.trials {
				t.Fatalf(
					"the check saw the edited tree %d times, want %d", len(seen), testCase.trials,
				)
			}

			for _, content := range seen {
				if content != testCase.wantSeen {
					t.Fatalf("the check saw %q", content)
				}
			}

			if len(kept) != testCase.trials || len(kept[0]) != 1 ||
				kept[0][0].Name != testCase.wantKeptName {
				t.Fatalf("the renderer kept %v", kept)
			}

			after, err := os.ReadFile(path)
			if err != nil || string(after) != testCase.source {
				t.Fatalf("the tree after = %q, %v", after, err)
			}
		})
	}
}

func TestBuildsFailures(t *testing.T) {
	t.Parallel()

	passing := func(string, verifying.Unit) (verifying.Outcome, error) {
		return verifying.Outcome{Step: "vet"}, nil
	}

	testCases := []struct {
		name        string
		file        string
		wired       wiring
		isCancelled bool
		wantErr     error
	}{
		{
			name:    "an edit outside the tree",
			file:    "../a.go",
			wired:   wiring{check: passing},
			wantErr: ErrEdit,
		},
		{
			name: "a file the tree does not hold", file: "gone.go",
			wired: wiring{check: passing}, wantErr: os.ErrNotExist,
		},
		{
			name: "the run is cancelled", file: "a.go", wired: wiring{check: passing},
			isCancelled: true, wantErr: context.Canceled,
		},
		{
			name: "the run is cancelled while the tree cannot be prepared", file: "a.go",
			wired:       wiring{check: passing, listErr: errors.New("list boom")},
			isCancelled: true, wantErr: context.Canceled,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeFile(t, dir, "A last\n")

			testCase.wired.dir = dir
			probe := newProbe(t, testCase.wired)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			if testCase.isCancelled {
				cancel()
			}

			_, err := probe.Builds(ctx, []anchoring.Edit{{
				File: testCase.file, Span: entities.Span{End: 6}, Text: "A one",
				Keep: []entities.DeclKey{{File: testCase.file, Kind: entities.KindFunc, Name: "A"}},
			}})
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Builds() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestBuildsGivesAnUncheckedVerdictWhenTheTreeCannotBePrepared(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		wired wiring
	}{
		{name: "the commits cannot be listed", wired: wiring{listErr: errors.New("list boom")}},
		{name: "the revision has no commit", wired: wiring{isEmpty: true}},
		{
			name:  "the tree cannot be checked out",
			wired: wiring{checkoutErr: errors.New("checkout boom")},
		},
		{
			name:  "the modules cannot be listed",
			wired: wiring{modulesErr: errors.New("modules boom")},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeFile(t, dir, "A last\n")

			testCase.wired.dir = dir
			probe := newProbe(t, testCase.wired)

			for range 2 {
				verdict, err := probe.Builds(t.Context(), []anchoring.Edit{{
					File: "a.go", Span: entities.Span{End: 6}, Text: "A one",
					Keep: []entities.DeclKey{{File: "a.go", Kind: entities.KindFunc, Name: "A"}},
				}})
				if err != nil || !verdict.IsUnchecked || verdict.IsBuilt ||
					!strings.Contains(verdict.Detail, "tree not prepared") {
					t.Fatalf("Builds() = %+v, %v, want an unchecked verdict", verdict, err)
				}
			}
		})
	}
}

func TestCheckGivesNoVerdictWithoutAUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		units []verifying.Unit
	}{
		{name: "no unit at all"},
		{name: "an empty list of units", units: []verifying.Unit{}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			probe := newProbe(t, wiring{dir: t.TempDir()})

			verdict, err := probe.check(t.Context(), t.TempDir(), testCase.units)
			if err != nil {
				t.Fatalf("check() = %v", err)
			}

			if verdict.IsBuilt || !verdict.IsUnchecked {
				t.Fatalf("check() = %+v, want an unchecked verdict", verdict)
			}
		})
	}
}

func TestBuildsReportsATreeThatCannotBeRestored(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		source  string
		wantErr error
	}{
		{
			name:    "a check that removes the tree under the probe",
			source:  "A last\n",
			wantErr: os.ErrNotExist,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "tree")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			writeFile(t, dir, testCase.source)

			probe := newProbe(t, wiring{
				dir: dir,
				check: func(string, verifying.Unit) (verifying.Outcome, error) {
					content, err := os.ReadFile(filepath.Join(dir, "a.go"))
					if err != nil {
						return verifying.Outcome{}, fmt.Errorf("read: %w", err)
					}

					if string(content) == testCase.source {
						return verifying.Outcome{Step: "vet"}, nil
					}

					return verifying.Outcome{Step: "vet"}, os.RemoveAll(dir)
				},
			})

			if _, err := probe.Builds(t.Context(), []anchoring.Edit{{
				File: "a.go", Span: entities.Span{End: 6}, Text: "A one",
				Keep: []entities.DeclKey{{File: "a.go", Kind: entities.KindFunc, Name: "A"}},
			}}); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Builds() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestBuildsGivesAnUnfitVerdict(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		edit   anchoring.Edit
		render func(context.Context, []byte, []entities.DeclKey) ([]byte, error)
	}{
		{
			name: "the renderer refuses the file",
			edit: anchoring.Edit{
				File: "a.go",
				Span: entities.Span{Start: 0, End: 6},
				Text: "A one",
			},
			render: func(context.Context, []byte, []entities.DeclKey) ([]byte, error) {
				return nil, errors.New("render boom")
			},
		},
		{
			name: "the splice does not fit the file",
			edit: anchoring.Edit{
				File: "a.go",
				Span: entities.Span{Start: 0, End: 90},
				Text: "A one",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := writeFile(t, dir, "A last\n")
			probe := newProbe(t, wiring{
				dir:    dir,
				render: testCase.render,
				check: func(string, verifying.Unit) (verifying.Outcome, error) {
					return verifying.Outcome{Step: "vet"}, nil
				},
			})

			verdict, err := probe.Builds(t.Context(), []anchoring.Edit{testCase.edit})
			if err != nil || !verdict.IsUnfit || verdict.IsBuilt || verdict.Detail == "" {
				t.Fatalf("Builds() = %+v, %v, want an unfit verdict", verdict, err)
			}

			after, readErr := os.ReadFile(path)
			if readErr != nil || string(after) != "A last\n" {
				t.Fatalf("the tree after = %q, %v", after, readErr)
			}
		})
	}
}
