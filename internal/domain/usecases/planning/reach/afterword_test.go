package reach

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestIsAfterword(t *testing.T) {
	t.Parallel()

	route := decl("a/handler.go", "GroupName", "a")
	entry := decl("docs/CHANGELOG.md", "<file>#2", "docs")
	nameless := decl("docs/CHANGELOG.md", "<file>#3", "docs")
	graph := entities.NewGraph(
		[]entities.Decl{route, entry, nameless},
		[]entities.Edge{{Dependent: entry.Key, Provider: route.Key, Reason: entities.ReasonProse}},
		nil,
	)

	testCases := []struct {
		name string
		key  entities.DeclKey
		want bool
	}{
		{"an entry that waits for a route", entry.Key, true},
		{"an entry that names nothing", nameless.Key, false},
		{"the route itself", route.Key, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := IsAfterword(graph, testCase.key); got != testCase.want {
				t.Fatalf("isAfterword() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestCodeWiringDropsADocumentThatNamesTheProgram(t *testing.T) {
	t.Parallel()

	route := decl("a/handler.go", "GroupName", "a")
	boot := decl("cmd/main.go", "main", "cmd")
	entry := decl("docs/CHANGELOG.md", "<file>#2", "docs")
	graph := entities.NewGraph(
		[]entities.Decl{route, boot, entry},
		[]entities.Edge{{Dependent: entry.Key, Provider: route.Key, Reason: entities.ReasonProse}},
		nil,
	)

	got := CodeWiring(graph, Wiring{entry.Key: true, boot.Key: true})
	if got[entry.Key] || !got[boot.Key] {
		t.Fatalf("codeWiring() = %v, want only the code root", got)
	}
}
