package lineage

import (
	"maps"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestFloor(t *testing.T) {
	t.Parallel()

	oldKey := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "Old"}
	newKey := entities.DeclKey{File: "b.go", Kind: entities.KindFunc, Name: "New"}

	testCases := []struct {
		name   string
		result Result
		want   map[entities.DeclKey]entities.Position
	}{
		{"no births is an empty floor", Result{}, map[entities.DeclKey]entities.Position{}},
		{
			"a birth floors at its position",
			Result{Births: map[entities.DeclKey]Birth{oldKey: {At: 3}}},
			map[entities.DeclKey]entities.Position{oldKey: 3},
		},
		{
			"a renamed declaration floors at the position it was first born",
			Result{Births: map[entities.DeclKey]Birth{
				oldKey: {At: 3},
				newKey: {At: 3, Via: []Rename{{At: 9, From: oldKey, To: newKey}}},
			}},
			map[entities.DeclKey]entities.Position{oldKey: 3, newKey: 3},
		},
		{
			"position zero is a real floor",
			Result{Births: map[entities.DeclKey]Birth{oldKey: {At: 0}}},
			map[entities.DeclKey]entities.Position{oldKey: 0},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Floor(testCase.result)

			if got == nil || !maps.Equal(got, testCase.want) {
				t.Fatalf("Floor() = %v, want %v", got, testCase.want)
			}
		})
	}
}
