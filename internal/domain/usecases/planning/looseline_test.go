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

	loose := LooseLines(graph)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := loose[testCase.key]; got != testCase.want {
				t.Fatalf("looseLines()[%s] = %v, want %v", testCase.key, got, testCase.want)
			}
		})
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
		fresh Wiring
		want  bool
	}{
		{"the manifest is fresh, the module is not", Wiring{header.Key: true}, false},
		{"the module it names is fresh", Wiring{module.Key: true}, true},
		{"nothing is fresh", Wiring{}, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := TiedToFresh(graph, use.Key, testCase.fresh, Wiring{})
			if got != testCase.want {
				t.Fatalf("tiedToFresh() = %v, want %v", got, testCase.want)
			}
		})
	}
}
