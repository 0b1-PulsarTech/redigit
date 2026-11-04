package idiom

import (
	"reflect"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestMergedAtoms(t *testing.T) {
	t.Parallel()

	first := entities.DeclKey{File: "a.go", Kind: entities.KindStmt, Name: "Setup#1"}
	second := entities.DeclKey{File: "a.go", Kind: entities.KindStmt, Name: "Setup#2"}

	testCases := []struct {
		name   string
		groups []map[entities.DeclKey]string
		want   map[entities.DeclKey]string
	}{
		{name: "no rule gives no atom", want: map[entities.DeclKey]string{}},
		{
			name:   "the first rule keeps its labels",
			groups: []map[entities.DeclKey]string{{first: "shared"}},
			want:   map[entities.DeclKey]string{first: "shared"},
		},
		{
			name: "a later rule never shares a label with an earlier one",
			groups: []map[entities.DeclKey]string{
				{first: "shared"}, {second: "shared"}, {second: "other"},
			},
			want: map[entities.DeclKey]string{
				first: "shared", second: "other#rules[2]",
			},
		},
		{
			name: "a later rule labels a statement the first also labelled",
			groups: []map[entities.DeclKey]string{
				{first: "shared"}, {first: "shared", second: "shared"},
			},
			want: map[entities.DeclKey]string{
				first: "shared#rules[1]", second: "shared#rules[1]",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := mergedAtoms(testCase.groups)
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("mergedAtoms() = %v, want %v", got, testCase.want)
			}
		})
	}
}
