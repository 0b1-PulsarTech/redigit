package planning

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestTestSupportIsWhatOnlyOtherPackagesTestsRead(t *testing.T) {
	t.Parallel()

	checker := decl("apperrtest/check.go", "Require", "apperrtest")
	inner := decl("apperrtest/check.go", "field", "apperrtest")
	subject := decl("lead/create.go", "Create", "lead")
	prod := decl("lead/handler.go", "Handle", "lead")
	leadTest := testDecl("lead/create_test.go", "TestCreate", "lead")
	graph := entities.NewGraph(
		[]entities.Decl{checker, inner, subject, prod, leadTest},
		[]entities.Edge{
			needs(leadTest, checker), needs(leadTest, subject), needs(checker, inner),
			needs(prod, subject),
		},
		nil,
	)

	got := TestSupport(graph)

	if !got[checker.Key] || !got[inner.Key] {
		t.Errorf("testSupport() = %v, want the checker and what only it reads", got)
	}

	if got[subject.Key] || got[prod.Key] {
		t.Errorf("testSupport() = %v, want code under test and production code left out", got)
	}
}

func TestTestSupportLeavesAFunctionOnlyItsOwnTestsReadAsCode(t *testing.T) {
	t.Parallel()

	subject := decl("lead/create.go", "Create", "lead")
	leadTest := testDecl("lead/create_test.go", "TestCreate", "lead")
	graph := entities.NewGraph(
		[]entities.Decl{subject, leadTest},
		[]entities.Edge{needs(leadTest, subject)},
		nil,
	)

	if TestSupport(graph)[subject.Key] {
		t.Fatal("a function only its own package's tests read was taken for test support")
	}
}
