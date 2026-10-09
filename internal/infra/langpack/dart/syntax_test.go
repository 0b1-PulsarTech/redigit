package dart

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func declare(t *testing.T, file, source string) []entities.Decl {
	t.Helper()

	decls, err := NewSyntax(nil, nil).Declarations(context.Background(), file, []byte(source))
	if err != nil {
		t.Fatalf("Declarations(%s) error = %v", file, err)
	}

	return decls
}

func TestDeclarationsSelectorCountsUTF16(t *testing.T) {
	t.Parallel()

	const source = "// header\n\n@Tag('\U0001D11E') class A {}\n"

	decls := declare(t, "lib/a.dart", source)
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}

	want := entities.Location{File: "lib/a.dart", Line: 2, Char: 17}
	if decls[0].Selector != want {
		t.Fatalf(
			"Selector = %+v, want %+v (the clef is two UTF-16 units, four bytes)",
			decls[0].Selector,
			want,
		)
	}

	if got := source[decls[0].Span.Start:decls[0].Span.End]; got != "@Tag('\U0001D11E') class A {}" {
		t.Fatalf("Span text = %q", got)
	}

	if decls[0].Unit != "lib" {
		t.Fatalf("Unit = %q, want lib", decls[0].Unit)
	}
}

func TestDeclarationsFlags(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name       string
		file       string
		source     string
		wantKey    entities.DeclKey
		wantIsTest bool
		wantAtom   string
	}

	testCases := []testCase{
		{
			"a test file's main is the test",
			"test/a_test.dart", "void main() {}\n",
			key("test/a_test.dart", entities.KindFunc, "main"), true, "",
		},
		{
			"main outside a test file is not",
			"bin/a.dart", "void main() {}\n",
			key("bin/a.dart", entities.KindFunc, "main"), false, "",
		},
		{
			"a file declaring nothing stands as one declaration",
			"lib/a.dart", "library a;\n",
			key("lib/a.dart", entities.KindOther, fileDeclName), false, "",
		},
		{
			"a library with parts is one atom with them",
			"lib/a.dart", "part 'b.dart';\nclass A {}\n",
			key("lib/a.dart", entities.KindType, "A"), false, "lib/a.dart",
		},
		{
			"a part is an atom named after its library",
			"lib/b.dart", "part of 'a.dart';\nclass B {}\n",
			key("lib/b.dart", entities.KindType, "B"), false, "lib/a.dart",
		},
		{
			"a generated file is one atom",
			"lib/g.dart", "// GENERATED CODE - DO NOT MODIFY BY HAND\n\nint x = 1;\n",
			key("lib/g.dart", entities.KindValue, "x"), false, "lib/g.dart",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			decls := declare(t, tc.file, tc.source)
			if len(decls) != 1 {
				t.Fatalf("got %d decls, want 1: %+v", len(decls), decls)
			}

			got := decls[0]
			if got.Key != tc.wantKey || got.IsTest != tc.wantIsTest || got.Atom != tc.wantAtom {
				t.Fatalf("decl = %v test=%v atom=%q, want %v test=%v atom=%q",
					got.Key, got.IsTest, got.Atom, tc.wantKey, tc.wantIsTest, tc.wantAtom)
			}
		})
	}
}
