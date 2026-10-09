package dart

import (
	"reflect"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestParseDirective(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		source string
		want   directive
	}

	testCases := []testCase{
		{"bare import", "import 'a.dart';", directive{uri: "a.dart"}},
		{
			"prefixed import",
			"import 'package:p/p.dart' as p;",
			directive{uri: "package:p/p.dart", prefix: "p"},
		},
		{
			"show and hide lists",
			"import 'a.dart' show A, B hide C;",
			directive{uri: "a.dart", show: []string{"A", "B"}, hide: []string{"C"}},
		},
		{
			"a configurable import keeps its first URI",
			"import 'a.dart' if (dart.library.js) 'b.dart';",
			directive{uri: "a.dart"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := parseDirective(lex(tc.source)); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseDirective() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseFileKeys(t *testing.T) {
	t.Parallel()

	const file = "lib/a.dart"

	parsed := parseFile(file, "import 'b.dart';\nexport 'c.dart';\nvoid f() {}\nvoid f() {}\n;\n")

	got := make([]entities.DeclKey, 0, len(parsed.decls()))

	for _, it := range parsed.decls() {
		got = append(got, it.key)
	}

	want := []entities.DeclKey{
		key(file, entities.KindOther, "export c.dart"),
		key(file, entities.KindFunc, "f"),
		key(file, entities.KindFunc, "f#2"),
		key(file, entities.KindOther, "<item 4>"),
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keys = %+v, want %+v", got, want)
	}
}
