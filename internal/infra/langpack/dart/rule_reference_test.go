package dart

import (
	"context"
	"reflect"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// edgesFrom runs one rule over tree and keeps the edges leaving file.
func edgesFrom(t *testing.T, each rule, tree fakeTree, file string) []entities.Edge {
	t.Helper()

	edges, err := each.Edges(context.Background(), tree.space(), declareAll(t, tree))
	if err != nil {
		t.Fatalf("%s Edges() error = %v", each.Name(), err)
	}

	var out []entities.Edge

	for _, edge := range edges {
		if edge.Dependent.File == file {
			out = append(out, edge)
		}
	}

	return out
}

func TestReferenceRuleWaitsForTheBarrelToo(t *testing.T) {
	t.Parallel()

	tree := withLibrary(fakeTree{
		"lib/use.dart": "import 'package:app/app.dart';\nMoney total() => Money();\n",
	})

	total := key("lib/use.dart", entities.KindFunc, "total")
	money := entities.Edge{
		Dependent: total,
		Provider:  key("lib/src/money.dart", entities.KindType, "Money"),
		Reason:    entities.ReasonReference,
	}
	barrel := entities.Edge{
		Dependent: total,
		Provider:  key("lib/app.dart", entities.KindOther, "export src/money.dart"),
		Reason:    entities.ReasonReference,
	}
	want := []entities.Edge{money, barrel, money, barrel}

	if got := edgesFrom(t, referenceRule{}, tree, "lib/use.dart"); !reflect.DeepEqual(got, want) {
		t.Fatalf("edges = %+v, want %+v", got, want)
	}
}

func TestReferenceRuleReadFailure(t *testing.T) {
	t.Parallel()

	failing := newWorkspace([]string{"lib/a.dart"}, fakeTree{}.Read)
	decls := []entities.Decl{{Key: key("lib/a.dart", entities.KindType, "A")}}

	if _, err := (referenceRule{}).Edges(context.Background(), failing, decls); err == nil {
		t.Fatal("Edges() error = nil, want the read failure")
	}
}
