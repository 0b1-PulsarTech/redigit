package config

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestEarlierBreaksEveryTieByKey(t *testing.T) {
	t.Parallel()

	fileTie := entities.DeclKey{File: "x:func", Kind: entities.KindType, Name: "T"}
	nameTie := entities.DeclKey{File: "x", Kind: entities.KindFunc, Name: "type:T"}

	testCases := []struct {
		name      string
		first     entities.Position
		firstKey  entities.DeclKey
		second    entities.Position
		secondKey entities.DeclKey
		want      bool
	}{
		{"a set position beats an unset one", 3, fileTie, entities.NoPosition, nameTie, true},
		{"an unset position loses to a set one", entities.NoPosition, nameTie, 3, fileTie, false},
		{"the lower position wins", 2, fileTie, 3, nameTie, true},
		{"equal positions order tied keys by file", 3, nameTie, 3, fileTie, true},
		{"equal positions put the longer file last", 3, fileTie, 3, nameTie, false},
		{"the same key is not earlier than itself", 3, nameTie, 3, nameTie, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := earlier(testCase.first, testCase.firstKey, testCase.second, testCase.secondKey)
			if got != testCase.want {
				t.Fatalf("earlier() = %v, want %v", got, testCase.want)
			}
		})
	}
}
