package owners

import (
	"fmt"
	"maps"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func readers(features ...reach.FeatureID) map[reach.FeatureID]bool {
	out := map[reach.FeatureID]bool{}
	for _, feature := range features {
		out[feature] = true
	}

	return out
}

func TestJoinProducts(t *testing.T) {
	t.Parallel()

	shared := map[entities.DeclKey]map[reach.FeatureID]bool{}
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
		reached map[entities.DeclKey]map[reach.FeatureID]bool
		want    map[reach.FeatureID]reach.FeatureID
	}{
		{
			"a port read by a root and two tiers ties them to that root",
			map[entities.DeclKey]map[reach.FeatureID]bool{
				{Name: "Port"}: readers("crm", "routes", "repo"),
			},
			map[reach.FeatureID]reach.FeatureID{"routes": "crm", "repo": "crm"},
		},
		{
			"a feature goes to the root it shares the most with",
			map[entities.DeclKey]map[reach.FeatureID]bool{
				{Name: "A"}: readers("crm", "worker"),
				{Name: "B"}: readers("chat", "worker"),
				{Name: "C"}: readers("chat", "worker"),
			},
			map[reach.FeatureID]reach.FeatureID{"worker": "chat"},
		},
		{
			"an equal tie goes to the first name",
			map[entities.DeclKey]map[reach.FeatureID]bool{
				{Name: "A"}: readers("crm", "worker"),
				{Name: "B"}: readers("chat", "worker"),
			},
			map[reach.FeatureID]reach.FeatureID{"worker": "chat"},
		},
		{
			"code read by many features ties nothing",
			shared,
			map[reach.FeatureID]reach.FeatureID{},
		},
		{
			"a declaration read by one feature ties nothing",
			map[entities.DeclKey]map[reach.FeatureID]bool{{Name: "Own"}: readers("routes")},
			map[reach.FeatureID]reach.FeatureID{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for range 64 {
				got := joinProducts(testCase.reached, readers("crm", "chat"))
				if !maps.Equal(got, testCase.want) {
					t.Fatalf("joinProducts() = %v, want %v", got, testCase.want)
				}
			}
		})
	}
}

func TestJoinProductsSumOrder(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		reached map[entities.DeclKey]map[reach.FeatureID]bool
		want    map[reach.FeatureID]reach.FeatureID
	}{
		{
			"a real tie is settled by name whatever the summing order",
			map[entities.DeclKey]map[reach.FeatureID]bool{
				{Name: "a1"}: readers("crm", "worker"),
				{Name: "a2"}: readers("crm", "worker"),
				{Name: "a3"}: readers("crm", "worker", "x1", "x2"),
				{Name: "b1"}: readers("chat", "worker"),
				{Name: "b2"}: readers("chat", "worker"),
				{Name: "b3"}: readers("chat", "worker", "x3", "x4"),
			},
			map[reach.FeatureID]reach.FeatureID{
				"worker": "chat", "x1": "crm", "x2": "crm", "x3": "chat", "x4": "chat",
			},
		},
		{
			"a tie whose sums would round apart by key order goes to the first name",
			map[entities.DeclKey]map[reach.FeatureID]bool{
				{Name: "a1"}: readers("crm", "worker"),
				{Name: "a2"}: readers("crm", "worker"),
				{Name: "a3"}: readers("crm", "worker", "x1", "x2"),
				{Name: "b1"}: readers("chat", "worker", "x3", "x4"),
				{Name: "b2"}: readers("chat", "worker"),
				{Name: "b3"}: readers("chat", "worker"),
			},
			map[reach.FeatureID]reach.FeatureID{
				"worker": "chat", "x1": "crm", "x2": "crm", "x3": "chat", "x4": "chat",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for range 64 {
				got := joinProducts(testCase.reached, readers("crm", "chat"))
				if !maps.Equal(got, testCase.want) {
					t.Fatalf("joinProducts() = %v, want %v", got, testCase.want)
				}
			}
		})
	}
}

func TestTieScaleDividesEveryWeight(t *testing.T) {
	t.Parallel()

	for readersMinusOne := 1; readersMinusOne < kinReach; readersMinusOne++ {
		if tieScale%readersMinusOne != 0 {
			t.Fatalf("tieScale %% %d = %d, want 0", readersMinusOne, tieScale%readersMinusOne)
		}
	}
}
