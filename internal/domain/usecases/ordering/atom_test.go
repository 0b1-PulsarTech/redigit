package ordering

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func atomDecl(file, name, atom string) entities.Decl {
	return entities.Decl{
		Key:  entities.DeclKey{File: file, Kind: entities.KindType, Name: name},
		Unit: entities.UnitID("store"),
		Atom: atom,
	}
}

// A tool that writes a file in one pass wrote all of it at once, so no member
// arrives before another: they share the LATEST position any of them reached.
func TestAtomMembersAreBornTogether(t *testing.T) {
	t.Parallel()

	mockType := atomDecl("store/repo_mock_test.go", "MockRepo", "store/repo_mock_test.go")
	recorder := atomDecl("store/repo_mock_test.go", "MockRepoRecorder", "store/repo_mock_test.go")
	late := atomDecl("store/late.go", "Late", "")

	// Only the recorder is pulled late, by something it reads.
	graph := entities.NewGraph(
		[]entities.Decl{mockType, recorder, late},
		[]entities.Edge{{
			Dependent: recorder.Key,
			Provider:  late.Key,
			Reason:    entities.ReasonReference,
		}},
		nil,
	)

	snap := entities.Snapshot{
		Files: []string{mockType.Key.File, late.Key.File},
		History: []entities.SourceTouch{
			{Position: 0, Rev: "a", Touched: []string{mockType.Key.File}},
			{Position: 9, Rev: "b", Touched: []string{late.Key.File}},
		},
	}

	order, err := New(Config{}).Order(graph, snap)
	if err != nil {
		t.Fatalf("Order() = %v", err)
	}

	if got, want := order.Of(mockType.Key), order.Of(recorder.Key); got != want {
		t.Fatalf(
			"the file arrives in pieces: %s at %d and %s at %d",
			mockType.Key.Name, got, recorder.Key.Name, want,
		)
	}

	// Latest, not earliest: the earliest would put the recorder before what
	// it reads.
	if got := order.Of(recorder.Key); got < order.Of(late.Key) {
		t.Fatalf("the recorder landed at %d, before the %d it reads", got, order.Of(late.Key))
	}
}

// A declaration carrying no atom is untouched: most of a repository is written
// by people, a declaration at a time.
func TestOrderingLeavesAtomlessDeclarationsAlone(t *testing.T) {
	t.Parallel()

	early := atomDecl("a.go", "Early", "")
	late := atomDecl("b.go", "Late", "")

	graph := entities.NewGraph([]entities.Decl{early, late}, nil, nil)
	snap := entities.Snapshot{
		Files: []string{"a.go", "b.go"},
		History: []entities.SourceTouch{
			{Position: 0, Rev: "a", Touched: []string{"a.go"}},
			{Position: 7, Rev: "b", Touched: []string{"b.go"}},
		},
	}

	order, err := New(Config{}).Order(graph, snap)
	if err != nil {
		t.Fatalf("Order() = %v", err)
	}

	if order.Of(early.Key) == order.Of(late.Key) {
		t.Fatalf("two unrelated declarations were forced together at %d", order.Of(early.Key))
	}
}
