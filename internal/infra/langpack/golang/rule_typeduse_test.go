package golang

import (
	"context"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/testkit"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
)

const typedUseHelper = `package fixture

import "time"

func SecondsToDuration(seconds int64) time.Duration { return time.Duration(seconds) * time.Second }

type Box struct{ Wait time.Duration }

func (b Box) Open() {}

const Limit = 3
`

func TestTypedUseRule(t *testing.T) {
	t.Parallel()

	snap, decls := testkit.Fixture(t, map[string]string{
		"helper.go": typedUseHelper,
		"gen.go": `package fixture

func Convert(seconds int64) Box {
	box := Box{}
	box.Wait = SecondsToDuration(seconds)
	box.Open()

	return box
}

func Cap() int { return Limit }

func Local() int {
	inner := 1

	return inner
}
`,
		"case_test.go": `package fixture

import "testing"

type enrichCase struct{ name string }

func runEnrichCase(t *testing.T, c enrichCase) { t.Log(c.name) }
`,
	})

	edges, err := typedUseRule{source: typed.NewSource(nil)}.Edges(context.Background(), snap, decls)
	if err != nil {
		t.Fatalf("Edges() error = %v", err)
	}

	got := testkit.EdgeSet(edges)
	convert := testkit.DeclKey("gen.go", entities.KindFunc, "Convert")

	reference := func(dependent, provider entities.DeclKey) entities.Edge {
		return entities.Edge{
			Dependent: dependent,
			Provider:  provider,
			Reason:    entities.ReasonReference,
		}
	}

	wants := []entities.Edge{
		reference(convert, testkit.DeclKey("helper.go", entities.KindFunc, "SecondsToDuration")),
		reference(convert, testkit.DeclKey("helper.go", entities.KindType, "Box")),
		reference(convert, testkit.DeclKey("helper.go", entities.KindMethod, "Box.Open")),
		reference(
			testkit.DeclKey("gen.go", entities.KindFunc, "Cap"),
			testkit.DeclKey("helper.go", entities.KindValue, "Limit"),
		),
		reference(
			testkit.DeclKey("case_test.go", entities.KindFunc, "runEnrichCase"),
			testkit.DeclKey("case_test.go", entities.KindType, "enrichCase"),
		),
	}

	for _, want := range wants {
		if !got[want] {
			t.Errorf("Edges() is missing %+v", want)
		}
	}

	for edge := range got {
		if edge.Dependent == edge.Provider || edge.Dependent.Name == "Local" {
			t.Errorf("Edges() holds an edge that orders nothing: %+v", edge)
		}
	}
}
