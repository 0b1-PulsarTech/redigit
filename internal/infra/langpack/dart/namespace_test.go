package dart

import (
	"context"
	"reflect"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

var library = fakeTree{
	"pubspec.yaml": "name: app\n",
	"lib/app.dart": "export 'src/money.dart' show Money;\nexport 'src/hidden.dart' hide Secret;\n",
	"lib/src/money.dart": "class Money {}\nclass Other {}\nint _cents = 0;\n" +
		"extension Labels on Money { String label() => ''; }\n",
	"lib/src/hidden.dart":  "class Secret {}\nclass Shown {}\n",
	"lib/src/cycle_a.dart": "export 'cycle_b.dart';\nclass CA {}\n",
	"lib/src/cycle_b.dart": "export 'cycle_a.dart';\nclass CB {}\n",
	"lib/l10n.dart":        "export 'src/l10n.dart';\n",
	"lib/src/l10n.dart":    "export 'catalog.g.dart';\nclass AppLocale {}\n",
}

func TestExported(t *testing.T) {
	t.Parallel()

	barrelMoney := key("lib/app.dart", entities.KindOther, "export src/money.dart")
	barrelHidden := key("lib/app.dart", entities.KindOther, "export src/hidden.dart")

	type testCase struct {
		name string
		file string
		want map[string]provider
	}

	testCases := []testCase{
		{
			"a barrel passes on what its show and hide lists let through",
			"lib/app.dart",
			map[string]provider{
				"Money": {
					key: key("lib/src/money.dart", entities.KindType, "Money"),
					via: []entities.DeclKey{barrelMoney},
				},
				"Shown": {
					key: key("lib/src/hidden.dart", entities.KindType, "Shown"),
					via: []entities.DeclKey{barrelHidden},
				},
			},
		},
		{
			"a private name stays out",
			"lib/src/money.dart",
			map[string]provider{
				"Money":  {key: key("lib/src/money.dart", entities.KindType, "Money")},
				"Other":  {key: key("lib/src/money.dart", entities.KindType, "Other")},
				"Labels": {key: key("lib/src/money.dart", entities.KindType, "Labels")},
			},
		},
		{
			"an export cycle ends",
			"lib/src/cycle_a.dart",
			map[string]provider{
				"CA": {key: key("lib/src/cycle_a.dart", entities.KindType, "CA")},
				"CB": {
					key: key("lib/src/cycle_b.dart", entities.KindType, "CB"),
					via: []entities.DeclKey{
						key("lib/src/cycle_a.dart", entities.KindOther, "export cycle_b.dart"),
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := library.space().exported(context.Background(), tc.file)
			if err != nil {
				t.Fatalf("exported() error = %v", err)
			}

			if !reflect.DeepEqual(got.names, tc.want) {
				t.Fatalf("names = %+v, want %+v", got.names, tc.want)
			}
		})
	}
}

func TestExportedMembersAndOpenExports(t *testing.T) {
	t.Parallel()

	space := library.space()

	barrel, err := space.exported(context.Background(), "lib/app.dart")
	if err != nil {
		t.Fatalf("exported() error = %v", err)
	}

	if got := barrel.members["label"].key; got != key(
		"lib/src/money.dart",
		entities.KindType,
		"Labels",
	) {
		t.Fatalf(
			"members[label] = %v, want the extension: a show list filters names, not members",
			got,
		)
	}

	l10n, err := space.exported(context.Background(), "lib/l10n.dart")
	if err != nil {
		t.Fatalf("exported() error = %v", err)
	}

	generated := key("lib/src/l10n.dart", entities.KindOther, "export catalog.g.dart")
	want := map[entities.DeclKey]provider{
		generated: {
			key: generated,
			via: []entities.DeclKey{
				key("lib/l10n.dart", entities.KindOther, "export src/l10n.dart"),
			},
		},
	}

	if !reflect.DeepEqual(l10n.open, want) {
		t.Fatalf("open = %+v, want %+v", l10n.open, want)
	}
}
