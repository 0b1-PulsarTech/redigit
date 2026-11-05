package resolve

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

func hiddenFiles() map[string]string {
	return map[string]string{
		"a.go": "package fixture\n\ntype Namer interface{ Name() string }\n\n" +
			"func One() int { return 1 }\n",
		"h_dev.go": "//go:build dev\n\npackage fixture\n\ntype Hid struct{}\n\n" +
			"func (Hid) Name() string { return \"h\" }\n\nfunc Two() int { return One() }\n\n" +
			"func Three() int { return Two() }\n",
	}
}

func buildOf(t *testing.T, files map[string]string, tags []string) (fixture, []index) {
	t.Helper()

	snap, decls := testkit.Fixture(t, files)
	source := typed.NewSource(tags)
	fix := fixture{
		files: files, source: source, decls: decls, resolver: NewTyped(source, snap),
	}

	built, err := fix.resolver.build(context.Background())
	if err != nil {
		t.Fatalf("build() error = %v", err)
	}

	return fix, built
}

func TestTypedIndexesTheHiddenPasses(t *testing.T) {
	t.Parallel()

	twoPasses := map[string]string{
		"a.go":     "package fixture\n",
		"h_dev.go": "//go:build dev\n\npackage fixture\n\nfunc Dev() int { return 0 }\n",
		"h_alt.go": "//go:build alt\n\npackage fixture\n\nfunc Alt() int { return 0 }\n",
	}

	testCases := []struct {
		name       string
		files      map[string]string
		tags       []string
		wantGroups int
		wantHidden [][]string
	}{
		{
			name:       "a file the tags hide",
			files:      hiddenFiles(),
			wantGroups: 2,
			wantHidden: [][]string{{"h_dev.go"}},
		},
		{
			name:       "tags that hide nothing",
			files:      hiddenFiles(),
			tags:       []string{"dev"},
			wantGroups: 1,
		},
		{
			name:       "one group per hidden pass",
			files:      twoPasses,
			wantGroups: 3,
			wantHidden: [][]string{{"h_alt.go"}, {"h_dev.go"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, built := buildOf(t, testCase.files, testCase.tags)
			if len(built) != testCase.wantGroups {
				t.Fatalf("build() = %d groups, want %d", len(built), testCase.wantGroups)
			}

			var got [][]string
			for _, group := range built[1:] {
				got = append(got, slices.Sorted(maps.Keys(group.covered)))
			}

			if !slices.EqualFunc(got, testCase.wantHidden, slices.Equal[[]string]) {
				t.Fatalf("hidden groups cover %v, want %v", got, testCase.wantHidden)
			}
		})
	}
}

func TestTypedAnswersAcrossAHiddenFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		call  string
		query spot
		want  []spot
	}{
		{
			name:  "a hidden file reads a declaration of the main pass",
			call:  "references",
			query: spot{file: "a.go", needle: "One() int"},
			want:  []spot{{file: "h_dev.go", needle: "One() }"}},
		},
		{
			name:  "a hidden file reads a declaration of a hidden file",
			call:  "references",
			query: spot{file: "h_dev.go", needle: "Two() int"},
			want:  []spot{{file: "h_dev.go", needle: "Two() }"}},
		},
		{
			name:  "a use in a hidden file resolves to its declaration",
			call:  "definition",
			query: spot{file: "h_dev.go", needle: "One() }"},
			want:  []spot{{file: "a.go", needle: "One() int"}},
		},
		{
			name:  "an implementer in a hidden file reaches the interface",
			call:  "implementations",
			query: spot{file: "a.go", needle: "Namer interface"},
			want:  []spot{{file: "h_dev.go", needle: "Hid struct"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix, _ := buildOf(t, hiddenFiles(), nil)
			at := fix.locate(t, testCase.query)
			want := fix.locations(t, testCase.want)

			var (
				got []entities.Location
				err error
			)

			switch testCase.call {
			case "references":
				got, err = fix.resolver.References(context.Background(), at)
			case "implementations":
				got, err = fix.resolver.Implementations(context.Background(), at)
			default:
				var found entities.Location

				found, err = fix.resolver.Definition(context.Background(), at)
				got = []entities.Location{found}
			}

			if err != nil || !slices.Equal(got, want) {
				t.Fatalf("%s = %v, %v; want %v", testCase.call, got, err, want)
			}
		})
	}
}

func TestTypedRefusesTheImplementorsOfAHiddenInterface(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		files   map[string]string
		query   spot
		wantErr error
	}{
		{
			name: "an interface declared in a hidden file",
			files: map[string]string{
				"a.go": "package fixture\n",
				"h_dev.go": "//go:build dev\n\npackage fixture\n\n" +
					"type Spoke interface{ Say() string }\n",
			},
			query:   spot{file: "h_dev.go", needle: "Spoke interface"},
			wantErr: ErrNotCovered,
		},
		{
			name: "an interface declared in the main pass",
			files: map[string]string{
				"a.go": "package fixture\n\ntype Spoke interface{ Say() string }\n",
				"h_dev.go": "//go:build dev\n\npackage fixture\n\n" +
					"type Hid struct{}\n\nfunc (Hid) Say() string { return \"h\" }\n",
			},
			query: spot{file: "a.go", needle: "Spoke interface"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix, _ := buildOf(t, testCase.files, nil)

			_, err := fix.resolver.Implementations(
				context.Background(), fix.locate(t, testCase.query),
			)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Implementations() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestTypedHiddenUnreadDoesNotTaintTheMainPass(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		files          map[string]string
		wantIncomplete bool
		wantLost       bool
		refused        []spot
		answered       []spot
	}{
		{
			name: "a hidden file that does not type-check",
			files: map[string]string{
				"a.go": "package fixture\n\nfunc One() int { return 1 }\n",
				"h_dev.go": "//go:build dev\n\npackage fixture\n\n" +
					"func Two() int { return One() }\n\nfunc Bad() int { return \"s\" }\n",
			},
			wantIncomplete: true,
			refused:        []spot{{file: "h_dev.go", needle: "Two() int"}},
			answered:       []spot{{file: "a.go", needle: "One() int"}},
		},
		{
			name: "a hidden file that reads another package",
			files: map[string]string{
				"a.go": "package fixture\n\nimport \"example.test/fixture/p\"\n\n" +
					"func Use() int { return p.Help() }\n",
				"p/p.go": "package p\n\nfunc Help() int { return 1 }\n",
				"h_dev.go": "//go:build dev\n\npackage fixture\n\n" +
					"import \"example.test/fixture/p\"\n\nfunc Two() int { return p.Help() }\n",
			},
			wantLost: true,
			answered: []spot{
				{file: "p/p.go", needle: "Help() int"},
				{file: "a.go", needle: "Use() int"},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix, built := buildOf(t, testCase.files, nil)
			if len(built) != 2 {
				t.Fatalf("build() = %d groups, want the main pass and the hidden one", len(built))
			}

			hidden := built[1]
			if got := slices.ContainsFunc(
				slices.Collect(maps.Values(hidden.covered)),
				func(file covering) bool { return file.incomplete },
			); got != testCase.wantIncomplete {
				t.Fatalf("hidden incomplete = %v, want %v", got, testCase.wantIncomplete)
			}

			if got := len(hidden.lost) > 0; got != testCase.wantLost {
				t.Fatalf("hidden lost = %v, want %v", hidden.lost, testCase.wantLost)
			}

			if lost := fix.resolver.HiddenLost(); (lost > 0) != testCase.wantLost {
				t.Fatalf("HiddenLost() = %d, want some: %v", lost, testCase.wantLost)
			}

			for _, query := range testCase.refused {
				_, err := fix.resolver.References(context.Background(), fix.locate(t, query))
				if !errors.Is(err, ErrNotCovered) {
					t.Fatalf("References(%+v) error = %v, want %v", query, err, ErrNotCovered)
				}
			}

			for _, query := range testCase.answered {
				if _, err := fix.resolver.References(
					context.Background(), fix.locate(t, query),
				); err != nil {
					t.Fatalf("References(%+v) error = %v, want it answered", query, err)
				}
			}
		})
	}
}

func TestTypedHiddenFileThatIsIncompleteLendsNothing(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		files map[string]string
		want  []spot
	}{
		{
			name: "a hidden file redeclaring a type of its package",
			files: map[string]string{
				"impl/a_tool.go": "//go:build ignore\n\npackage impl\n\ntype X struct{}\n",
				"impl/main.go": "package impl\n\ntype Namer interface{ Name() string }\n\n" +
					"type X struct{}\n\nfunc (X) Name() string { return \"x\" }\n",
			},
			want: []spot{{file: "impl/main.go", needle: "X struct"}},
		},
		{
			name: "a hidden file implementing the interface and compiling",
			files: map[string]string{
				"impl/a_tool.go": "//go:build ignore\n\npackage impl\n\ntype Y struct{}\n\n" +
					"func (Y) Name() string { return \"y\" }\n",
				"impl/main.go": "package impl\n\ntype Namer interface{ Name() string }\n\n" +
					"type X struct{}\n\nfunc (X) Name() string { return \"x\" }\n",
			},
			want: []spot{
				{file: "impl/a_tool.go", needle: "Y struct"},
				{file: "impl/main.go", needle: "X struct"},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fix, _ := buildOf(t, testCase.files, nil)
			query := spot{file: "impl/main.go", needle: "Namer interface"}

			got, err := fix.resolver.Implementations(context.Background(), fix.locate(t, query))
			if want := fix.locations(t, testCase.want); err != nil || !slices.Equal(got, want) {
				t.Fatalf("Implementations() = %v, %v; want %v", got, err, want)
			}
		})
	}
}
