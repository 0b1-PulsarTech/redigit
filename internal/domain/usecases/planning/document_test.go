package planning

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestIsDocument(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		key  entities.DeclKey
		want bool
	}{
		{"a spec", entities.DeclKey{File: "docs/specs/a.md", Kind: entities.KindOther}, true},
		{
			"a module readme",
			entities.DeclKey{File: "modules/a/README.md", Kind: entities.KindOther},
			false,
		},
		{
			"a Go file under docs",
			entities.DeclKey{File: "docs/x.go", Kind: entities.KindFunc},
			false,
		},
		{"a diagram", entities.DeclKey{File: "docs/flow.mmd", Kind: entities.KindOther}, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := isDocument(testCase.key); got != testCase.want {
				t.Fatalf("isDocument() = %v, want %v", got, testCase.want)
			}
		})
	}
}

// A document scattered between stretches of code made a commit of its own for
// each stretch; gathered, they make one.
func TestGatherDocuments(t *testing.T) {
	t.Parallel()

	spec := func(name string) entities.Decl {
		return entities.Decl{
			Key: entities.DeclKey{File: "docs/" + name + ".md", Kind: entities.KindOther},
		}
	}
	code := func(name string) entities.Decl {
		return entities.Decl{
			Key: entities.DeclKey{File: "a/" + name + ".go", Kind: entities.KindFunc, Name: name},
		}
	}

	first, second, late := spec("first"), spec("second"), spec("late")
	one, two, three := code("one"), code("two"), code("three")
	graph := entities.NewGraph(
		[]entities.Decl{first, second, late, one, two, three},
		[]entities.Edge{{Dependent: late.Key, Provider: two.Key, Reason: entities.ReasonProse}},
		nil,
	)

	got := gatherDocuments(
		graph,
		[]entities.DeclKey{one.Key, first.Key, two.Key, second.Key, late.Key, three.Key},
	)
	want := []entities.DeclKey{first.Key, second.Key, one.Key, two.Key, late.Key, three.Key}

	if !slices.Equal(got, want) {
		t.Fatalf("gatherDocuments() = %v, want %v", got, want)
	}
}
