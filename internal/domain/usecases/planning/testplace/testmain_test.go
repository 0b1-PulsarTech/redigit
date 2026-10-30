package testplace

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestEarlierBreaksEveryTieByKey(t *testing.T) {
	t.Parallel()

	fileTie := entities.Decl{Key: entities.DeclKey{
		File: "x:func", Kind: entities.KindType, Name: "T",
	}}
	nameTie := entities.Decl{Key: entities.DeclKey{
		File: "x", Kind: entities.KindFunc, Name: "type:T",
	}}
	order := entities.Ordering{At: map[entities.DeclKey]entities.Position{
		fileTie.Key: 4,
		nameTie.Key: 4,
	}}

	testCases := []struct {
		name  string
		order entities.Ordering
		left  entities.Decl
		right entities.Decl
		want  bool
	}{
		{"a tied position orders tied keys by file", order, nameTie, fileTie, true},
		{"a tied position puts the longer file last", order, fileTie, nameTie, false},
		{"the same declaration is not earlier than itself", order, nameTie, nameTie, false},
		{
			"the lower position wins over the key",
			entities.Ordering{At: map[entities.DeclKey]entities.Position{
				fileTie.Key: 1,
				nameTie.Key: 4,
			}},
			fileTie,
			nameTie,
			true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := earlier(testCase.order, testCase.left, testCase.right); got != testCase.want {
				t.Fatalf("earlier() = %v, want %v", got, testCase.want)
			}
		})
	}
}
