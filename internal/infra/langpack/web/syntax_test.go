package web

import (
	"context"
	"errors"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestDeclarations(t *testing.T) {
	t.Parallel()

	type testCase struct {
		file   string
		unit   entities.UnitID
		isTest bool
		atom   string
	}

	testCases := []testCase{
		{file: "web/src/routes/+page.svelte", unit: "web/src/routes"},
		{file: "web/src/lib/a.ts", unit: "web/src/lib", atom: "web/src/lib/a.ts"},
		{file: "web/src/lib/b.ts", unit: "web/src/lib", atom: "web/src/lib/a.ts"},
		{file: "web/test/page.mjs", unit: "web/test", isTest: true},
		{file: "web/test/harness.mjs", unit: "web/test"},
	}

	syntax := NewSyntax(app.files(), app)

	for _, tc := range testCases {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()

			source := []byte(app[tc.file])

			decls, err := syntax.Declarations(context.Background(), tc.file, source)
			if err != nil {
				t.Fatalf("Declarations() error = %v", err)
			}

			want := entities.Decl{
				Key:      fileKey(tc.file),
				Unit:     tc.unit,
				Span:     entities.Span{Start: 0, End: len(source)},
				Selector: entities.Location{File: tc.file},
				IsTest:   tc.isTest,
				Atom:     tc.atom,
			}
			if len(decls) != 1 || decls[0] != want {
				t.Errorf("Declarations() = %+v, want [%+v]", decls, want)
			}
		})
	}
}

func TestDeclarationsUnreadableTree(t *testing.T) {
	t.Parallel()

	syntax := NewSyntax([]string{"a.ts"}, fakeTree{})

	if _, err := syntax.Declarations(
		context.Background(),
		"a.ts",
		nil,
	); !errors.Is(
		err,
		errMissing,
	) {
		t.Errorf("Declarations() error = %v, want %v", err, errMissing)
	}
}

func TestRender(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		keep []entities.DeclKey
		want string
	}

	testCases := []testCase{
		{
			name: "kept whole",
			keep: []entities.DeclKey{fileKey("a.ts")},
			want: "export const a = 1;\n",
		},
		{name: "not born yet", keep: nil, want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := Syntax{}.Render(
				context.Background(),
				[]byte("export const a = 1;\n"),
				tc.keep,
			)
			if err != nil || string(got) != tc.want {
				t.Errorf("Render() = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
