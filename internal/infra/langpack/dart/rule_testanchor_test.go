package dart

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestTestAnchorRule(t *testing.T) {
	t.Parallel()

	tree := fakeTree{
		"pubspec.yaml":         "name: app\n",
		"lib/app.dart":         "export 'src/money.dart';\nexport 'src/rate.dart';\n",
		"lib/src/money.dart":   "class Money {}\nint cents(Money m) => 0;\n",
		"lib/src/rate.dart":    "class Rate {}\n",
		"test/money_test.dart": "import 'package:app/app.dart';\nvoid main() { Rate(); cents(Money()); }\n",
		"test/rate_test.dart":  "void main() {}\n",
		"test/app_test.dart":   "void main() {}\n",
		"test/other_test.dart": "import 'package:app/app.dart';\nvoid main() { Money(); }\n",
		"test/empty_test.dart": "void main() {}\n",
	}

	anchors, err := testAnchorRule{}.Anchors(
		context.Background(),
		tree.space(),
		declareAll(t, tree),
	)
	if err != nil {
		t.Fatalf("Anchors() error = %v", err)
	}

	got := map[string]entities.DeclKey{}

	for _, anchor := range anchors {
		if anchor.Reason != entities.ReasonTestOf || anchor.Decl.Name != "main" {
			t.Fatalf("anchor = %+v, want a test-of anchor from main", anchor)
		}

		got[anchor.Decl.File] = anchor.To
	}

	want := map[string]entities.DeclKey{
		"test/money_test.dart": key("lib/src/money.dart", entities.KindFunc, "cents"),
		"test/rate_test.dart":  key("lib/src/rate.dart", entities.KindType, "Rate"),
		"test/app_test.dart":   key("lib/app.dart", entities.KindOther, "export src/money.dart"),
		"test/other_test.dart": key("lib/src/money.dart", entities.KindType, "Money"),
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("anchors = %+v, want %+v", got, want)
	}
}

func TestMirrors(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		file string
		want []string
	}

	testCases := []testCase{
		{
			"a nested package test",
			"packages/core/test/a/b_test.dart",
			[]string{"packages/core/lib/src/a/b.dart", "packages/core/lib/a/b.dart"},
		},
		{"a root package test", "test/x_test.dart", []string{"lib/src/x.dart", "lib/x.dart"}},
		{"a test helper mirrors nothing", "test/helper.dart", nil},
		{"a library file mirrors nothing", "lib/x_test.dart", nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := mirrors(tc.file); !slices.Equal(got, tc.want) {
				t.Fatalf("mirrors() = %q, want %q", got, tc.want)
			}
		})
	}
}
