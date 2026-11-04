package anchorprobe

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
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
