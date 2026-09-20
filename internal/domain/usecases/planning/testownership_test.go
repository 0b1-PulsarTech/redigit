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

func TestAssignDoesNotReguessAnAlreadyAnchoredTest(t *testing.T) {
	t.Parallel()

	entryA := decl("a/main.go", "MainA", "a")
	entryB := decl("b/main.go", "MainB", "b")
	shared := decl("kernel/normalize.go", "Normalize", "kernel")
	entryC := decl("kernel/main.go", "MainC", "kernel")
	twinSibling := decl("kernel/normalize.go", "helper", "kernel")
	suite := testDecl("kernel/normalize_test.go", "TestNormalize", "kernel")

	graph := entities.NewGraph(
		[]entities.Decl{entryA, entryB, shared, entryC, twinSibling, suite},
		[]entities.Edge{
			needs(entryA, shared),
			needs(entryB, shared),
			needs(entryC, twinSibling),
		},
		[]entities.Anchor{{Decl: suite.Key, To: shared.Key, Reason: entities.ReasonTestOf}},
	)

	own := assign(graph)

	if got := own.owner[suite.Key]; got != SharedFeature {
		t.Fatalf(
			"owner = %q, want the shared bucket: the anchor already settled this, "+
				"the twin file (owned by %q) must not reopen it",
			got, own.owner[twinSibling.Key],
		)
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

func TestAssignDoesNotAnchorATestWhoseOwnNeedCrossesFeatures(t *testing.T) {
	t.Parallel()

	entryA := decl("a/main.go", "MainA", "a")
	subjectA := decl("a/handler.go", "Handle", "a")
	entryB := decl("b/main.go", "MainB", "b")
	helperB := decl("b/helper.go", "Helper", "b")
	suite := testDecl("a/handler_test.go", "TestHandle", "a")

	graph := entities.NewGraph(
		[]entities.Decl{entryA, subjectA, entryB, helperB, suite},
		[]entities.Edge{needs(entryA, subjectA), needs(entryB, helperB), needs(suite, helperB)},
		[]entities.Anchor{{Decl: suite.Key, To: subjectA.Key, Reason: entities.ReasonTestOf}},
	)

	own := assign(graph)

	if got := own.owner[suite.Key]; got != SharedFeature {
		t.Fatalf(
			"owner = %q, want the shared bucket: honoring the anchor would cycle a with b",
			got,
		)
	}
}

func TestAssignAlwaysInheritsASharedAnchorTarget(t *testing.T) {
	t.Parallel()

	entryA := decl("a/main.go", "MainA", "a")
	entryB := decl("b/main.go", "MainB", "b")
	shared := decl("kernel/normalize.go", "Normalize", "kernel")
	helperB := decl("b/helper.go", "Helper", "b")
	suite := testDecl("kernel/normalize_test.go", "TestNormalize", "kernel")

	graph := entities.NewGraph(
		[]entities.Decl{entryA, entryB, shared, helperB, suite},
		[]entities.Edge{
			needs(entryA, shared),
			needs(entryB, shared),
			needs(entryB, helperB),
			needs(suite, helperB),
		},
		[]entities.Anchor{{Decl: suite.Key, To: shared.Key, Reason: entities.ReasonTestOf}},
	)

	own := assign(graph)

	if got := own.owner[suite.Key]; got != SharedFeature {
		t.Fatalf(
			"owner = %q, want the shared bucket: a shared target is always safe to inherit",
			got,
		)
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
