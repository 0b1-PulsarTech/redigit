package planning

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func manifestLine(file, name string) entities.Decl {
	out := decl(file, name, "manifest")
	out.Key.Kind = entities.KindOther

	return out
}

func TestLooseLines(t *testing.T) {
	t.Parallel()

	header := manifestLine("libs/a/go.mod", "module a")
	use := manifestLine("go.work", "use ./libs/a")
	read := manifestLine("apps/x/go.mod", "require a")
	code := decl("apps/x/main.go", "Main", "x")
	rare := manifestLine("apps/x/go.mod", "require b")
	test := decl("apps/x/main_test.go", "TestMain", "x")
	test.IsTest = true

	graph := entities.NewGraph(
		[]entities.Decl{header, use, read, code, rare, test},
		[]entities.Edge{
			edgeOf(use, header, entities.ReasonWorkspace),
			edgeOf(read, header, entities.ReasonWorkspace),
			edgeOf(code, read, entities.ReasonImport),
			edgeOf(rare, header, entities.ReasonImport),
			edgeOf(test, rare, entities.ReasonImport),
		},
		nil,
	)

	testCases := []struct {
		name string
		key  entities.DeclKey
		want bool
	}{
		{"a workspace entry nothing needs", use.Key, true},
		{"a require that code imports", read.Key, false},
		{"a require only a test imports", rare.Key, true},
		{"the code itself", code.Key, false},
	}

	loose := looseLines(graph)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := loose[testCase.key]; got != testCase.want {
				t.Fatalf("looseLines()[%s] = %v, want %v", testCase.key, got, testCase.want)
			}
		})
	}
}

func TestDeferLibraryEntries(t *testing.T) {
	t.Parallel()

	free := decl("lib/a.go", "A", "lib")
	owned := decl("lib/b.go", "B", "lib")
	own := ownership{
		owner:    map[entities.DeclKey]FeatureID{free.Key: SharedFeature, owned.Key: "f"},
		deferred: wiring{},
		loose:    wiring{},
	}

	deferLibraryEntries(own, []entities.Decl{free, owned})

	if !own.deferred[free.Key] || !own.loose[free.Key] {
		t.Fatalf("the unowned entry %s is not deferred as loose", free.Key)
	}

	if own.deferred[owned.Key] {
		t.Fatalf("the owned entry %s was deferred", owned.Key)
	}
}
