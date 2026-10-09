package dart

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestUnlisted(t *testing.T) {
	t.Parallel()

	found := scopeFor(
		t,
		"import 'l10n.dart';\nimport 'l10n.dart' as l;\nimport 'l10n.dart' show AppLocale;\n",
	)

	type testCase struct {
		name   string
		lookup func() []reference
		want   []string
	}

	testCases := []testCase{
		{
			"a name nothing declares reaches the export leaving the tree",
			func() []reference { return found.lookup("messagesFor") },
			[]string{"export catalog.g.dart@0"},
		},
		{"a keyword never does", func() []reference { return found.lookup("return") }, nil},
		{
			"a declared name resolves first",
			func() []reference { return found.lookup("AppLocale") },
			[]string{"AppLocale@0", "AppLocale@2"},
		},
		{
			"a prefix reaches its own open exports",
			func() []reference { return found.prefixed("l", "messagesFor") },
			[]string{"export catalog.g.dart@1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := refNames(tc.lookup()); !slices.Equal(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}

	refs := found.lookup("messagesFor")
	if len(refs) != 1 || !slices.Equal(refs[0].via, []entities.DeclKey{
		key("lib/l10n.dart", entities.KindOther, "export src/l10n.dart"),
	}) {
		t.Fatalf(
			"via = %+v, want the barrel's export: the reader needs the barrel to pass it on",
			refs,
		)
	}
}
