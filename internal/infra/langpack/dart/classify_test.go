package dart

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// declTestCase is one item that declares something, with what it declares.
type declTestCase struct {
	name     string
	source   string
	wantDecl entities.DeclKind
	wantName string
}

func runDeclTestCases(t *testing.T, testCases []declTestCase) {
	t.Helper()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := classify(lex(tc.source))
			if got.kind != itemDecl || got.declKind != tc.wantDecl || got.name != tc.wantName {
				t.Fatalf("classify() = %v %q %q, want a declaration %q %q",
					got.kind, got.declKind, got.name, tc.wantDecl, tc.wantName)
			}
		})
	}
}

func TestClassifyTypes(t *testing.T) {
	t.Parallel()

	runDeclTestCases(t, []declTestCase{
		{
			"class with modifiers and generics",
			"abstract interface class A<T> extends B {}",
			entities.KindType,
			"A",
		},
		{"sealed class", "sealed class S {}", entities.KindType, "S"},
		{"mixin", "mixin M on A {}", entities.KindType, "M"},
		{"mixin class", "base mixin class BM {}", entities.KindType, "BM"},
		{"enum", "enum Color { red, green }", entities.KindType, "Color"},
		{"record typedef", "typedef Pair = ({int a, String b});", entities.KindType, "Pair"},
		{"old-style typedef", "typedef void Cb(int x);", entities.KindType, "Cb"},
		{"extension type", "extension type Id(String value) {}", entities.KindType, "Id"},
		{
			"unnamed extension",
			"extension on String { int get size => 1; }",
			entities.KindType,
			"extension on String",
		},
	})
}

func TestClassifyDirectives(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name     string
		source   string
		wantKind itemKind
		wantDecl entities.DeclKind
		wantName string
	}

	testCases := []testCase{
		{"library", "library x;", itemFrame, "", ""},
		{"part of", "part of 'a.dart';", itemFrame, "", ""},
		{"part", "part 'b.dart';", itemPart, "", ""},
		{"import", "import 'a.dart' as p;", itemImport, "", ""},
		{"export", "export 'a.dart';", itemExport, entities.KindOther, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := classify(lex(tc.source))
			if got.kind != tc.wantKind || got.declKind != tc.wantDecl || got.name != tc.wantName {
				t.Fatalf("classify() = %v %q %q, want %v %q %q",
					got.kind, got.declKind, got.name, tc.wantKind, tc.wantDecl, tc.wantName)
			}
		})
	}
}
