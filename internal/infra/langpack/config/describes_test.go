package config

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSortedDeclsOrdersKeysWhoseTextIsEqual(t *testing.T) {
	t.Parallel()

	fileTie := entities.DeclKey{File: "x:func", Kind: entities.KindType, Name: "T"}
	nameTie := entities.DeclKey{File: "x", Kind: entities.KindFunc, Name: "type:T"}
	other := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "A"}

	testCases := []struct {
		name string
		set  map[entities.DeclKey]bool
		want []entities.DeclKey
	}{
		{
			"tied keys order by file",
			map[entities.DeclKey]bool{fileTie: true, nameTie: true},
			[]entities.DeclKey{nameTie, fileTie},
		},
		{
			"tied keys sort after an earlier file",
			map[entities.DeclKey]bool{fileTie: true, other: true, nameTie: true},
			[]entities.DeclKey{other, nameTie, fileTie},
		},
		{"an empty set yields nothing", map[entities.DeclKey]bool{}, []entities.DeclKey{}},
	}

	const runs = 256

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for range runs {
				if got := sortedDecls(testCase.set); !slices.Equal(got, testCase.want) {
					t.Fatalf("sortedDecls() = %v, want %v", got, testCase.want)
				}
			}
		})
	}
}

func TestIsProse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		file string
		want bool
	}{
		{"docs/specs/campaigns.md", true},
		{"modules/wabapi/README.md", true},
		{"docs/SERVICE_FLOW.mmd", true},
		{"notes/design.txt", true},
		{"docs/local/.gitignore", true},
		{"modules/x/doc/guide.yml", true},
		{"modules/x/config.yml", false},
		{"modules/x/documents/a.bin", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.file, func(t *testing.T) {
			t.Parallel()

			if got := isProse(testCase.file); got != testCase.want {
				t.Errorf("isProse(%q) = %v, want %v", testCase.file, got, testCase.want)
			}
		})
	}
}
