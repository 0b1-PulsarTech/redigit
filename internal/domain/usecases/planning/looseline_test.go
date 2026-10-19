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

func TestTiedToFreshCountsOnlyWhatTheLineNames(t *testing.T) {
	t.Parallel()

	header := manifestLine("go.work", "go 1.27")
	module := manifestLine("libs/a/go.mod", "module a")
	use := manifestLine("go.work", "use ./libs/a")
	graph := entities.NewGraph(
		[]entities.Decl{header, module, use},
		[]entities.Edge{
			edgeOf(use, header, entities.ReasonHosts),
			edgeOf(use, module, entities.ReasonWorkspace),
		},
		nil,
	)

	testCases := []struct {
		name  string
		fresh wiring
		want  bool
	}{
		{"the manifest is fresh, the module is not", wiring{header.Key: true}, false},
		{"the module it names is fresh", wiring{module.Key: true}, true},
		{"nothing is fresh", wiring{}, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := tiedToFresh(graph, use.Key, testCase.fresh, wiring{})
			if got != testCase.want {
				t.Fatalf("tiedToFresh() = %v, want %v", got, testCase.want)
			}
		})
	}
}
