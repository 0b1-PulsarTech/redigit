package planning

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSplitLateKeepsWhatReadsAMemberWithTheFeature(t *testing.T) {
	t.Parallel()

	own := decl("a/boot.go", "EngineConfig", "a")
	statement := decl("a/boot.go", "boot#1", "a")
	chained := decl("a/boot.go", "boot#2", "a")
	kernel := decl("kernel/id.go", "NewID", "kernel")

	graph := entities.NewGraph(
		[]entities.Decl{own, statement, chained, kernel},
		[]entities.Edge{needs(statement, own), needs(chained, statement), needs(chained, kernel)},
		nil,
	)

	early, late := splitLate(
		graph,
		[]entities.DeclKey{kernel.Key, statement.Key, chained.Key},
		[]entities.DeclKey{own.Key},
	)

	if want := []entities.DeclKey{kernel.Key}; !slices.Equal(early, want) {
		t.Fatalf("early = %v, want %v", early, want)
	}

	if want := []entities.DeclKey{statement.Key, chained.Key}; !slices.Equal(late, want) {
		t.Fatalf("late = %v, want %v", late, want)
	}
}

func TestSplitLateSendsAnAtomLateAsOne(t *testing.T) {
	t.Parallel()

	own := decl("a/boot.go", "helper", "a")
	head := decl("a/boot.go", "build", "a")
	last := decl("a/boot.go", "build#9", "a")
	head.Atom, last.Atom = "build#atom", "build#atom"

	graph := entities.NewGraph(
		[]entities.Decl{own, head, last},
		[]entities.Edge{needs(last, own)},
		nil,
	)

	early, late := splitLate(
		graph,
		[]entities.DeclKey{head.Key, last.Key},
		[]entities.DeclKey{own.Key},
	)

	if len(early) != 0 || !slices.Equal(late, []entities.DeclKey{head.Key, last.Key}) {
		t.Fatalf("early = %v, late = %v, want the atom late whole", early, late)
	}
}
