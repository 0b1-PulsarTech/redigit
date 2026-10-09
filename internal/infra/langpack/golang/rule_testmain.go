package golang

import (
	"context"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// testMainRule makes TestMain and the first test of its package one unit.
// TestMain owns the package's setup, and a package whose only test is born a
// commit later runs its setup over nothing; a package whose first test is born
// first runs without the setup it was written against.
type testMainRule struct{}

func (testMainRule) Name() string { return "test-main" }

func (testMainRule) Edges(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Edge, error) {
	return nil, nil
}

func (testMainRule) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

func (testMainRule) Atoms(
	_ context.Context,
	_ entities.Snapshot,
	decls []entities.Decl,
) (map[entities.DeclKey]string, error) {
	mains := map[entities.UnitID]entities.Decl{}
	firsts := map[entities.UnitID]entities.Decl{}

	for _, decl := range decls {
		if decl.Key.Kind != entities.KindFunc || !strings.HasSuffix(decl.Key.File, "_test.go") {
			continue
		}

		switch {
		case decl.Key.Name == "TestMain":
			mains[decl.Unit] = decl
		case strings.HasPrefix(decl.Key.Name, "Test"):
			if first, found := firsts[decl.Unit]; !found || comesBefore(decl, first) {
				firsts[decl.Unit] = decl
			}
		}
	}

	atoms := map[entities.DeclKey]string{}

	for unit, main := range mains {
		first, found := firsts[unit]
		if !found {
			continue
		}

		label := "test-main:" + string(unit)
		atoms[main.Key] = label
		atoms[first.Key] = label
	}

	return atoms, nil
}

func comesBefore(left, right entities.Decl) bool {
	if left.Key.File != right.Key.File {
		return left.Key.File < right.Key.File
	}

	return left.Span.Start < right.Span.Start
}
