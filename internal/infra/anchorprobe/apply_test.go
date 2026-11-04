package anchorprobe

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/emitting"
)

type applyCase struct {
	name         string
	edits        []anchoring.Edit
	wantErr      error
	wantInDetail string
}

func TestApply(t *testing.T) {
	t.Parallel()

	testCases := []applyCase{
		{
			name: "every file is rewritten and restored",
			edits: []anchoring.Edit{
				{File: "a.go", Span: entities.Span{Start: 0, End: 1}, Text: "X"},
				{File: "b.go", Span: entities.Span{Start: 0, End: 1}, Text: "Y"},
			},
		},
		{
			name:    "an edit outside the tree is a hard error",
			edits:   []anchoring.Edit{{File: "../a.go", Text: "X"}},
			wantErr: ErrEdit,
		},
		{
			name:    "a file the tree does not hold is a hard error",
			edits:   []anchoring.Edit{{File: "gone.go", Text: "X"}},
			wantErr: os.ErrNotExist,
		},
		{
			name: "a splice past the file is unfit",
			edits: []anchoring.Edit{
				{File: "a.go", Span: entities.Span{Start: 0, End: 90}, Text: "X"},
			},
			wantErr: ErrUnfit,
		},
		{
			name: "a file edited before the failing one is restored",
			edits: []anchoring.Edit{
				{File: "a.go", Span: entities.Span{Start: 0, End: 1}, Text: "X"},
				{File: "b.go", Span: entities.Span{Start: 0, End: 90}, Text: "Y"},
			},
			wantErr:      ErrUnfit,
			wantInDetail: "b.go",
		},
		{
			name: "two declarations sharing one span with differing text are unfit",
			edits: []anchoring.Edit{
				{File: "a.go", Span: entities.Span{Start: 0, End: 1}, Text: "X"},
				{File: "a.go", Span: entities.Span{Start: 0, End: 1}, Text: "Y"},
			},
			wantErr: ErrUnfit,
		},
		{
			name: "the first failing file by name is the one reported",
			edits: []anchoring.Edit{
				{File: "b.go", Span: entities.Span{Start: 0, End: 90}, Text: "X"},
				{File: "a.go", Span: entities.Span{Start: 0, End: 90}, Text: "X"},
			},
			wantErr:      ErrUnfit,
			wantInDetail: "a.go",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assertApply(t, testCase)
		})
	}
}

func assertApply(t *testing.T, testCase applyCase) {
	t.Helper()

	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(
			filepath.Join(dir, name),
			[]byte("A one\n"),
			0o600,
		); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	renderer := NewMockRenderer(gomock.NewController(t))
	renderer.EXPECT().
		Render(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(passThrough).
		AnyTimes()

	restore, err := apply(t.Context(), renderer, dir, testCase.edits)
	if !errors.Is(err, testCase.wantErr) ||
		!strings.Contains(errText(err), testCase.wantInDetail) {
		t.Fatalf(
			"apply() error = %v, want %v naming %q",
			err,
			testCase.wantErr,
			testCase.wantInDetail,
		)
	}

	if err != nil {
		assertUntouched(t, dir)

		return
	}

	if err = restore(); err != nil {
		t.Fatalf("restore() = %v", err)
	}

	assertUntouched(t, dir)
}

func errText(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}

func assertUntouched(t *testing.T, dir string) {
	t.Helper()

	for _, name := range []string{"a.go", "b.go"} {
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(content) != "A one\n" {
			t.Fatalf("%s = %q, %v, want it restored", name, content, err)
		}
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

	return source, renderer.failure
}

func TestApplyAdmitsTheEditImports(t *testing.T) {
	t.Parallel()

	failure := errors.New("clash")
	span := entities.Span{Start: 0, End: 1}

	testCases := []struct {
		name      string
		edits     []anchoring.Edit
		failure   error
		wantFiles []string
		wantSpecs [][]string
		wantErr   error
	}{
		{
			name: "the imports of several edits in a file are admitted once",
			edits: []anchoring.Edit{
				{File: "a.go", Span: span, Text: "X", Imports: []string{"bytes"}},
				{
					File:    "a.go",
					Span:    entities.Span{Start: 2, End: 3},
					Text:    "Y",
					Imports: []string{"os"},
				},
			},
			wantFiles: []string{"a.go"}, wantSpecs: [][]string{{"bytes", "os"}},
		},
		{
			name:  "an edit with no imports admits nothing",
			edits: []anchoring.Edit{{File: "a.go", Span: span, Text: "X"}},
		},
		{
			name: "a refused admission leaves the pre-image unfit",
			edits: []anchoring.Edit{
				{File: "a.go", Span: span, Text: "X", Imports: []string{"bytes"}},
			},
			failure: failure, wantFiles: []string{"a.go"}, wantSpecs: [][]string{{"bytes"}},
			wantErr: ErrUnfit,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			if err := os.WriteFile(
				filepath.Join(dir, "a.go"),
				[]byte("A one\nB two\n"),
				0o600,
			); err != nil {
				t.Fatalf("write: %v", err)
			}

			renderer := &admittingRenderer{
				MockRenderer: NewMockRenderer(gomock.NewController(t)),
				failure:      testCase.failure,
			}
			renderer.EXPECT().
				Render(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(passThrough).
				AnyTimes()

			restore, err := apply(t.Context(), renderer, dir, testCase.edits)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("apply() error = %v, want %v", err, testCase.wantErr)
			}

			if !reflect.DeepEqual(renderer.files, testCase.wantFiles) ||
				!reflect.DeepEqual(renderer.specs, testCase.wantSpecs) {
				t.Fatalf("Admit() saw %v %v, want %v %v",
					renderer.files, renderer.specs, testCase.wantFiles, testCase.wantSpecs)
			}

			if err == nil {
				if err = restore(); err != nil {
					t.Fatalf("restore() = %v", err)
				}
			}
		})
	}
}

func TestApplyWritesWhatTheEmitterComposes(t *testing.T) {
	t.Parallel()

	first := anchoring.Edit{
		File: "a.go", Span: entities.Span{Start: 0, End: 1}, Text: "X", Imports: []string{"bytes"},
	}
	second := anchoring.Edit{
		File: "a.go", Span: entities.Span{Start: 6, End: 7}, Text: "Y", Imports: []string{"os"},
	}

	testCases := []struct {
		name  string
		edits []anchoring.Edit
		want  string
	}{
		{
			name:  "two anchors of different imports",
			edits: []anchoring.Edit{first, second},
			want:  "X one\nY two\n+bytes+os",
		},
		{name: "the anchor alone", edits: []anchoring.Edit{first}, want: "X one\nB two\n+bytes"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			if err := os.WriteFile(
				filepath.Join(dir, "a.go"),
				[]byte("A one\nB two\n"),
				0o600,
			); err != nil {
				t.Fatalf("write: %v", err)
			}

			renderer := &taggingRenderer{MockRenderer: NewMockRenderer(gomock.NewController(t))}
			renderer.EXPECT().
				Render(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(passThrough).
				AnyTimes()

			pieces := make([]emitting.Piece, 0, len(testCase.edits))
			for _, edit := range testCase.edits {
				pieces = append(
					pieces,
					emitting.Piece{Span: edit.Span, Text: edit.Text, Imports: edit.Imports},
				)
			}

			want, err := emitting.Compose(t.Context(), renderer, emitting.Layout{
				File: "a.go", Source: []byte("A one\nB two\n"), Pieces: pieces,
			})
			if err != nil || string(want) != testCase.want {
				t.Fatalf("Compose = %q, %v, want %q", want, err, testCase.want)
			}

			restore, err := apply(t.Context(), renderer, dir, testCase.edits)
			if err != nil {
				t.Fatalf("apply() = %v", err)
			}

			got, err := os.ReadFile(filepath.Join(dir, "a.go"))
			if err != nil || string(got) != string(want) {
				t.Fatalf("probe wrote %q, %v, emitter composes %q", got, err, want)
			}

			if err = restore(); err != nil {
				t.Fatalf("restore() = %v", err)
			}
		})
	}
}

type taggingRenderer struct{ *MockRenderer }

func (renderer *taggingRenderer) Admit(
	_ context.Context, _ string, source []byte, specs []string,
) ([]byte, error) {
	return append(bytes.Clone(source), "+"+strings.Join(specs, "+")...), nil
}
