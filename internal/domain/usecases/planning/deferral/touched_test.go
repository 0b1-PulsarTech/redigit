package deferral

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/owners"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/planning/reach"
)

func TestTouched(t *testing.T) {
	t.Parallel()

	fresh := decl("a/code.go", "Code", "a")
	old := decl("a/old.go", "Old", "a")
	first := testDecl("a/gen_test.go", "First", "a")
	second := testDecl("a/gen_test.go", "Second", "a")
	reader := testDecl("a/reader_test.go", "TestReader", "a")
	atomFirst, atomSecond := first, second
	atomFirst.Atom, atomSecond.Atom = "mock:a", "mock:a"

	testCases := []struct {
		name  string
		decls []entities.Decl
		edges []entities.Edge
		ready []entities.Decl
		want  []entities.DeclKey
	}{
		{
			name:  "a test reading fresh code is touched",
			decls: []entities.Decl{fresh, first},
			edges: []entities.Edge{needs(first, fresh)},
			ready: []entities.Decl{first},
			want:  []entities.DeclKey{first.Key},
		},
		{
			name:  "a test reading only old code is not",
			decls: []entities.Decl{fresh, old, first},
			edges: []entities.Edge{needs(first, old)},
			ready: []entities.Decl{first},
		},
		{
			name:  "a mate of a touched member is touched with it",
			decls: []entities.Decl{fresh, old, atomFirst, atomSecond},
			edges: []entities.Edge{needs(atomFirst, fresh), needs(atomSecond, old)},
			ready: []entities.Decl{atomFirst, atomSecond},
			want:  []entities.DeclKey{atomFirst.Key, atomSecond.Key},
		},
		{
			name:  "a test reading a touched mate is touched in turn",
			decls: []entities.Decl{fresh, old, atomFirst, atomSecond, reader},
			edges: []entities.Edge{
				needs(atomFirst, fresh), needs(atomSecond, old), needs(reader, atomSecond),
			},
			ready: []entities.Decl{atomFirst, atomSecond, reader},
			want:  []entities.DeclKey{atomFirst.Key, atomSecond.Key, reader.Key},
		},
		{
			name:  "a mate that is not ready does not touch the atom",
			decls: []entities.Decl{fresh, old, atomFirst, atomSecond},
			edges: []entities.Edge{needs(atomSecond, fresh), needs(atomFirst, old)},
			ready: []entities.Decl{atomFirst},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph := entities.NewGraph(testCase.decls, testCase.edges, nil)
			links := touchLinks{
				subjects: sharedSubjects(graph, owners.Ownership{}),
				atoms:    reach.AtomGroups(graph),
			}
			ready := map[entities.DeclKey]bool{}

			for _, member := range testCase.ready {
				ready[member.Key] = true
			}

			got := owners.SortedKeys(touched(
				graph, links, owners.Ownership{}, ready, map[entities.DeclKey]bool{fresh.Key: true},
			))
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("touched() = %v, want %v", got, testCase.want)
			}
		})
	}
}
