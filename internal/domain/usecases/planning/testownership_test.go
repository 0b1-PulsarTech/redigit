package planning

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestAssignGivesATestItsSubjectsOwner(t *testing.T) {
	t.Parallel()

	entry := decl("lead/api/main.go", "Main", "lead/api")
	subject := decl("lead/api/handler.go", "Handle", "lead/api")
	suite := testDecl("lead/api/handler_test.go", "TestHandle", "lead/api")

	graph := entities.NewGraph(
		[]entities.Decl{entry, subject, suite},
		[]entities.Edge{needs(entry, subject)},
		[]entities.Anchor{{Decl: suite.Key, To: subject.Key, Reason: entities.ReasonTestOf}},
	)

	own := assign(graph)

	if got, want := own.owner[suite.Key], own.owner[subject.Key]; got != want {
		t.Fatalf("test owner = %q, subject owner = %q", got, want)
	}
}

func TestAssignLeavesAnUnanchoredTestShared(t *testing.T) {
	t.Parallel()

	entry := decl("lead/api/main.go", "Main", "lead/api")
	orphan := testDecl("lead/api/orphan_test.go", "TestOrphan", "lead/api")

	own := assign(entities.NewGraph([]entities.Decl{entry, orphan}, nil, nil))

	if got := own.owner[orphan.Key]; got != SharedFeature {
		t.Fatalf("owner = %q, want the shared bucket", got)
	}
}

func TestAssignFallsBackToTheTwinFilesOwner(t *testing.T) {
	t.Parallel()

	entry := decl("lead/api/main.go", "Main", "lead/api")
	subject := decl("lead/api/handler.go", "Handle", "lead/api")
	suite := testDecl("lead/api/handler_test.go", "TestSomethingTheAnchorMisses", "lead/api")

	graph := entities.NewGraph(
		[]entities.Decl{entry, subject, suite},
		[]entities.Edge{needs(entry, subject)},
		nil,
	)

	own := assign(graph)

	if got, want := own.owner[suite.Key], own.owner[subject.Key]; got != want {
		t.Fatalf("test owner = %q, twin file's owner = %q", got, want)
	}
}

func TestAssignDoesNotFoldATestWhoseOwnNeedCrossesFeatures(t *testing.T) {
	t.Parallel()

	entryA := decl("a/main.go", "MainA", "a")
	subjectA := decl("a/handler.go", "Handle", "a")
	entryB := decl("b/main.go", "MainB", "b")
	helperB := decl("b/helper.go", "Helper", "b")
	suite := testDecl("a/handler_test.go", "TestSomethingTheAnchorMisses", "a")

	graph := entities.NewGraph(
		[]entities.Decl{entryA, subjectA, entryB, helperB, suite},
		[]entities.Edge{needs(entryA, subjectA), needs(entryB, helperB), needs(suite, helperB)},
		nil,
	)

	own := assign(graph)

	if got := own.owner[suite.Key]; got != SharedFeature {
		t.Fatalf("owner = %q, want the shared bucket: folding it in would cycle a with b", got)
	}
}
