package bootstrap

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning"
)

func birthKey(file, name string) entities.DeclKey {
	return entities.DeclKey{File: file, Kind: entities.KindFunc, Name: name}
}

func TestCapabilitiesFrom(t *testing.T) {
	t.Parallel()

	early := birthKey("lead/repo.go", "Insert")
	late := birthKey("lead/repo.go", "Archive")
	orphan := birthKey("tools/gen.go", "Main")

	births := indexing.Births{At: map[entities.DeclKey]entities.Position{
		early:  5,
		late:   300,
		orphan: 900,
	}}

	features := entities.NewFeatures([]entities.Feature{
		{Name: "captacao", At: []entities.Position{5, 6, 7}},
		{Name: "arquivamento", At: []entities.Position{300}},
	})

	got := capabilitiesFrom(births, features)

	testCases := []struct {
		name string
		key  entities.DeclKey
		want planning.FeatureID
	}{
		{
			// The two declarations share a FILE and belong to different
			// slices: dating them by the file would file both under the
			// first, which is the whole reason the birth is per declaration.
			name: "an early declaration of a file",
			key:  early,
			want: "captacao",
		},
		{
			name: "a declaration the same file grew later",
			key:  late,
			want: "arquivamento",
		},
		{
			name: "a declaration born where no slice reaches",
			key:  orphan,
			want: "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got[testCase.key] != testCase.want {
				t.Fatalf("capability = %q, want %q", got[testCase.key], testCase.want)
			}
		})
	}
}

func TestCapabilitiesFromWithNoFeaturesIsNil(t *testing.T) {
	t.Parallel()

	births := indexing.Births{At: map[entities.DeclKey]entities.Position{
		birthKey("a.go", "A"): 1,
	}}

	if got := capabilitiesFrom(births, entities.Features{}); got != nil {
		t.Fatalf("capabilitiesFrom() = %v, want nil so the planner derives its own", got)
	}
}
