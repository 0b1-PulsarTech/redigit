package planning

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func TestSortFeatures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		features   []FeatureID
		needs      map[FeatureID]map[FeatureID]bool
		at         map[FeatureID]entities.Position
		want       []FeatureID
		wantCyclic int
	}{
		{
			"a dependency wins over chronology",
			[]FeatureID{"api", "core"},
			map[FeatureID]map[FeatureID]bool{"api": {"core": true}},
			map[FeatureID]entities.Position{"api": 0, "core": 9},
			[]FeatureID{"core", "api"},
			0,
		},
		{
			"independent features keep the original chronology",
			[]FeatureID{"late", "early"},
			nil,
			map[FeatureID]entities.Position{"late": 7, "early": 1},
			[]FeatureID{"early", "late"},
			0,
		},
		{
			"equal positions fall back to the name so the result is deterministic",
			[]FeatureID{"b", "a"},
			nil,
			map[FeatureID]entities.Position{"a": 3, "b": 3},
			[]FeatureID{"a", "b"},
			0,
		},
		{
			"a feature that needs only itself is not blocked",
			[]FeatureID{"solo"},
			map[FeatureID]map[FeatureID]bool{"solo": {"solo": true}},
			nil,
			[]FeatureID{"solo"},
			0,
		},
		{
			"mutual need is reported rather than silently ordered",
			[]FeatureID{"a", "b"},
			map[FeatureID]map[FeatureID]bool{"a": {"b": true}, "b": {"a": true}},
			nil,
			nil,
			2,
		},
		{"nothing to sort", nil, nil, nil, nil, 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, cyclic := SortFeatures(
				testCase.features,
				FeatureRules{Needs: testCase.needs, At: testCase.at},
			)
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("sortFeatures() = %v, want %v", got, testCase.want)
			}

			if len(cyclic) != testCase.wantCyclic {
				t.Fatalf("cyclic = %v, want %d entries", cyclic, testCase.wantCyclic)
			}
		})
	}
}

func TestFeatureNeeds(t *testing.T) {
	t.Parallel()

	graph, _, _ := twoFeatures()
	own := owners.Assign(graph)
	needs := FeatureNeeds(graph, own)

	// The lead slice is one feature end to end, so it needs nothing from
	// another feature; only the shared kernel, which is not a feature edge.
	if len(needs["lead/api"]) != 0 {
		t.Fatalf("lead/api records feature dependencies %v, want none", needs["lead/api"])
	}

	// An edge into the shared kernel is not a feature dependency; the kernel is
	// emitted as a layer before whoever needs it.
	for from, targets := range needs {
		if _, shared := targets[reach.SharedFeature]; shared {
			t.Fatalf("%q records a dependency on the shared bucket", from)
		}
	}
}

func TestEarliest(t *testing.T) {
	t.Parallel()

	graph, order, _ := twoFeatures()
	own := owners.Assign(graph)
	at := Earliest(own, order)

	if got := at["lead/api"]; got != 1 {
		t.Fatalf("earliest lead/api = %d, want 1", got)
	}

	unset := Earliest(own, entities.Ordering{})
	if len(unset) != 0 {
		t.Fatalf("with no ordering, earliest = %v, want nothing set", unset)
	}
}

// TestOwnershipNeverBuildsAFeatureCycle pins the property the planner leans on:
// ownership by reachability cannot produce two features that need each other,
// because a provider of another feature's declaration is reached by both callers
// and therefore becomes shared. Trying to construct the cycle is what proved it.
func TestOwnershipNeverBuildsAFeatureCycle(t *testing.T) {
	t.Parallel()

	entryA := decl("a/main.go", "MainA", "a")
	entryB := decl("b/main.go", "MainB", "b")
	midA := decl("a/mid.go", "MidA", "a")
	midB := decl("b/mid.go", "MidB", "b")

	graph := entities.NewGraph(
		[]entities.Decl{entryA, entryB, midA, midB},
		[]entities.Edge{
			needs(entryA, midB),
			needs(entryB, midA),
		},
		nil,
	)

	own := owners.Assign(graph)

	_, cyclic := SortFeatures(
		own.Order,
		FeatureRules{
			Needs: FeatureNeeds(graph, own),
			At:    Earliest(own, entities.Ordering{}),
		},
	)
	if len(cyclic) != 0 {
		t.Fatalf("features %v need each other, which ownership should make impossible", cyclic)
	}
}

// A deferred declaration is shared by being deferred, not by being read by
// several features, so a feature reaching it comes after the features that own
// what it needs.
func TestFeatureNeedsBehindADeferred(t *testing.T) {
	t.Parallel()

	reader := decl("lead/api/handler.go", "Handle", "lead/api")
	registry := decl("wire/wire.go", "Register", "wire")
	bill := decl("bill/api/handler.go", "Handle", "bill/api")
	plain := decl("kernel/id.go", "NewID", "kernel")
	hop := decl("wire/hop.go", "Hop", "wire")

	owner := map[entities.DeclKey]FeatureID{
		reader.Key:   "lead/api",
		bill.Key:     "bill/api",
		registry.Key: reach.SharedFeature,
		hop.Key:      reach.SharedFeature,
		plain.Key:    reach.SharedFeature,
	}

	testCases := []struct {
		name     string
		edges    []entities.Edge
		deferred reach.Wiring
		want     bool
	}{
		{
			"a feature reaching a deferred declaration waits for the features behind it",
			[]entities.Edge{needs(reader, registry), needs(registry, bill)},
			reach.Wiring{registry.Key: true},
			true,
		},
		{
			"through a chain of deferred declarations",
			[]entities.Edge{needs(reader, registry), needs(registry, hop), needs(hop, bill)},
			reach.Wiring{registry.Key: true, hop.Key: true},
			true,
		},
		{
			"a shared declaration that is not deferred hides nothing",
			[]entities.Edge{needs(reader, plain), needs(plain, bill)},
			nil,
			false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph := entities.NewGraph(
				[]entities.Decl{reader, registry, bill, plain, hop},
				testCase.edges,
				nil,
			)
			got := FeatureNeeds(graph, owners.Ownership{Owner: owner, Deferred: testCase.deferred})

			if got["lead/api"]["bill/api"] != testCase.want {
				t.Fatalf("featureNeeds() lead/api -> bill/api = %v, want %v",
					got["lead/api"]["bill/api"], testCase.want)
			}
		})
	}
}
