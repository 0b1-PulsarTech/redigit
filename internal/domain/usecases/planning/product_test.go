package planning

import (
	"fmt"
	"maps"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func readers(features ...FeatureID) map[FeatureID]bool {
	out := map[FeatureID]bool{}
	for _, feature := range features {
		out[feature] = true
	}

	return out
}

func TestJoinProducts(t *testing.T) {
	t.Parallel()

	shared := map[entities.DeclKey]map[FeatureID]bool{}
	for index := range kinReach + 1 {
		shared[entities.DeclKey{File: "kernel.go", Name: fmt.Sprint(index)}] = readers(
			"crm",
			"chat",
			"routes",
			"repo",
			"worker",
			"x1",
			"x2",
			"x3",
			"x4",
			"x5",
			"x6",
			"x7",
			"x8",
		)
	}

	testCases := []struct {
		name    string
		reached map[entities.DeclKey]map[FeatureID]bool
		want    map[FeatureID]FeatureID
	}{
		{
			"a port read by a root and two tiers ties them to that root",
			map[entities.DeclKey]map[FeatureID]bool{
				{Name: "Port"}: readers("crm", "routes", "repo"),
			},
			map[FeatureID]FeatureID{"routes": "crm", "repo": "crm"},
		},
		{
			"a feature goes to the root it shares the most with",
			map[entities.DeclKey]map[FeatureID]bool{
				{Name: "A"}: readers("crm", "worker"),
				{Name: "B"}: readers("chat", "worker"),
				{Name: "C"}: readers("chat", "worker"),
			},
			map[FeatureID]FeatureID{"worker": "chat"},
		},
		{
			"an equal tie goes to the first name",
			map[entities.DeclKey]map[FeatureID]bool{
				{Name: "A"}: readers("crm", "worker"),
				{Name: "B"}: readers("chat", "worker"),
			},
			map[FeatureID]FeatureID{"worker": "chat"},
		},
		{
			"code read by many features ties nothing",
			shared,
			map[FeatureID]FeatureID{},
		},
		{
			"a declaration read by one feature ties nothing",
			map[entities.DeclKey]map[FeatureID]bool{{Name: "Own"}: readers("routes")},
			map[FeatureID]FeatureID{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := joinProducts(testCase.reached, readers("crm", "chat"))
			if !maps.Equal(got, testCase.want) {
				t.Fatalf("joinProducts() = %v, want %v", got, testCase.want)
			}
		})
	}
}
