package anchorprobe

import (
	"errors"
	"reflect"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

func TestSplice(t *testing.T) {
	t.Parallel()

	first := entities.Span{Start: 0, End: 3}
	last := entities.Span{Start: 8, End: 11}

	testCases := []struct {
		name    string
		edits   []anchoring.Edit
		want    string
		wantErr error
	}{
		{name: "no edit leaves the file alone", want: "aaa bbb ccc"},
		{
			name:  "one edit",
			edits: []anchoring.Edit{{Span: entities.Span{Start: 4, End: 7}, Text: "XY"}},
			want:  "aaa XY ccc",
		},
		{
			name:  "two edits given out of order",
			edits: []anchoring.Edit{{Span: last, Text: "3"}, {Span: first, Text: "1"}},
			want:  "1 bbb 3",
		},
		{
			name: "adjacent edits",
			edits: []anchoring.Edit{
				{Span: first, Text: "1"}, {Span: entities.Span{Start: 3, End: 4}, Text: "-"},
			},
			want: "1-bbb ccc",
		},
		{
			name: "two declarations sharing one span give one edit",
			edits: []anchoring.Edit{
				{File: "a.go", Span: first, Text: "1"}, {File: "a.go", Span: first, Text: "1"},
			},
			want: "1 bbb ccc",
		},
		{
			name: "two declarations sharing one span with differing text",
			edits: []anchoring.Edit{
				{File: "a.go", Span: first, Text: "1"}, {File: "a.go", Span: first, Text: "2"},
			},
			wantErr: ErrEdit,
		},
		{
			name: "overlapping edits",
			edits: []anchoring.Edit{
				{Span: entities.Span{Start: 0, End: 5}, Text: "1"},
				{Span: entities.Span{Start: 3, End: 7}, Text: "2"},
			},
			wantErr: ErrEdit,
		},
		{
			name:    "an edit past the file",
			edits:   []anchoring.Edit{{Span: entities.Span{Start: 8, End: 40}, Text: "1"}},
			wantErr: ErrEdit,
		},
		{
			name:    "an edit ending before it starts",
			edits:   []anchoring.Edit{{Span: entities.Span{Start: 5, End: 2}, Text: "1"}},
			wantErr: ErrEdit,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := splice([]byte("aaa bbb ccc"), testCase.edits)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("splice() error = %v, want %v", err, testCase.wantErr)
			}

			if string(got) != testCase.want {
				t.Fatalf("splice() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestAffectedBy(t *testing.T) {
	t.Parallel()

	both := []string{"apps/web", "libs/core"}
	rootOnly := []verifying.Unit{{Module: verifying.Module{Dir: "."}}}
	nested := verifying.Module{Dir: "apps/web"}
	lib := verifying.Module{Dir: "libs/core"}
	units := []verifying.Unit{
		{Module: nested, Deps: []verifying.Module{lib}},
		{Module: lib},
		{Module: verifying.Module{Dir: "tools"}},
	}

	testCases := []struct {
		name  string
		units []verifying.Unit
		files []string
		want  []string
	}{
		{
			name:  "a file of a dependency",
			units: units, files: []string{"libs/core/a.go"}, want: both,
		},
		{
			name:  "a file only the dependent holds",
			units: units, files: []string{"apps/web/a.go"}, want: []string{"apps/web"},
		},
		{
			name:  "a file no module holds",
			units: units, files: []string{"docs/readme.md"},
		},
		{
			name:  "a root module holds every file",
			units: rootOnly, files: []string{"x/y.go"}, want: []string{"."},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var got []string

			for _, unit := range affectedBy(testCase.units, testCase.files) {
				got = append(got, unit.Dir)
			}

			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("affectedBy() = %v, want %v", got, testCase.want)
			}
		})
	}
}
