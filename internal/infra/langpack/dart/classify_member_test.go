package dart

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestClassifyMembers(t *testing.T) {
	t.Parallel()

	runDeclTestCases(t, []declTestCase{
		{
			"generic function",
			"Map<String, int> count<T>(List<T> xs) { return {}; }",
			entities.KindFunc,
			"count",
		},
		{
			"function returning a record",
			"({int a, int b}) pair() => (a: 1, b: 2);",
			entities.KindFunc,
			"pair",
		},
		{"getter", "String get title => '';", entities.KindFunc, "title"},
		{"setter", "set title(String v) {}", entities.KindFunc, "title="},
		{"typed constant", "const Map<String, int> table = {'a': 1};", entities.KindValue, "table"},
		{
			"function-typed variable",
			"void Function(int) handler = (_) {};",
			entities.KindValue,
			"handler",
		},
		{
			"annotated external variable",
			"@Deprecated('x') external int z;",
			entities.KindValue,
			"z",
		},
	})
}

func TestClassifyNames(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name        string
		source      string
		wantNames   []string
		wantMembers []string
	}

	testCases := []testCase{
		{"a variable list binds every name", "final a = 1, b = f(2, 3);", []string{"a", "b"}, nil},
		{
			"an extension offers its methods and accessors as members",
			"extension H on V { String tab() => ''; String get label => ''; bool operator ==(Object o) => true; }",
			[]string{"H"},
			[]string{"tab", "label"},
		},
		{
			"an unnamed extension binds no name",
			"extension on String { int get size => 1; }",
			nil,
			[]string{"size"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := classify(lex(tc.source))
			if !slices.Equal(got.names, tc.wantNames) && len(got.names)+len(tc.wantNames) > 0 {
				t.Fatalf("names = %q, want %q", got.names, tc.wantNames)
			}

			if !slices.Equal(got.members, tc.wantMembers) &&
				len(got.members)+len(tc.wantMembers) > 0 {
				t.Fatalf("members = %q, want %q", got.members, tc.wantMembers)
			}
		})
	}
}
