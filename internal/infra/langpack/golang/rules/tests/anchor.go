package tests

import (
	"context"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

type Anchor struct{}

func (Anchor) Name() string { return "test-anchor" }

func (Anchor) Edges(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Edge, error) {
	return nil, nil
}

func (Anchor) Anchors(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	byFile := declsource.GroupByFile(decls)
	byUnit := groupByUnit(decls)
	sources := map[string][]byte{}

	var anchors []entities.Anchor

	for _, decl := range decls {
		if !decl.IsTest || decl.Key.Kind != entities.KindFunc {
			continue
		}

		unit := byUnit[decl.Unit]
		scope := unit

		if twin, found := twinFile(
			filepath.Dir(decl.Key.File),
			filepath.Base(decl.Key.File),
			byFile,
		); found {
			scope = byFile[twin]
		}

		to, found := testSubject(decl, scope, snap, sources)
		if !found && len(scope) != len(unit) {
			to, found = testSubject(decl, unit, snap, sources)
		}

		if !found {
			continue
		}

		anchors = append(
			anchors,
			entities.Anchor{Decl: decl.Key, To: to, Reason: entities.ReasonTestOf},
		)
	}

	return anchors, nil
}

func groupByUnit(decls []entities.Decl) map[entities.UnitID][]entities.Decl {
	byUnit := make(map[entities.UnitID][]entities.Decl, len(decls))
	for _, decl := range decls {
		byUnit[decl.Unit] = append(byUnit[decl.Unit], decl)
	}

	return byUnit
}

func twinFile(dir, testBase string, byFile map[string][]entities.Decl) (string, bool) {
	stem := strings.TrimSuffix(testBase, "_test.go")
	segments := strings.Split(stem, "_")

	for cut := len(segments); cut >= 1; cut-- {
		candidate := filepath.Join(dir, strings.Join(segments[:cut], "_")+".go")
		if _, exists := byFile[candidate]; exists {
			return candidate, true
		}
	}

	return "", false
}

func testSubjectPrefix(name string) (string, bool) {
	for _, prefix := range [...]string{"Test", "Benchmark", "Fuzz", "Example"} {
		if rest, ok := strings.CutPrefix(name, prefix); ok && rest != "" {
			return rest, true
		}
	}

	return "", false
}

func camelOrSnakeTokens(name string) []string {
	if strings.Contains(name, "_") {
		return strings.Split(name, "_")
	}

	runes := []rune(name)

	var tokens []string

	start := 0

	for index := 1; index < len(runes); index++ {
		prevUpper := unicode.IsUpper(runes[index-1])
		curUpper := unicode.IsUpper(runes[index])
		nextLower := index+1 < len(runes) && unicode.IsLower(runes[index+1])

		if curUpper && (!prevUpper || nextLower) {
			tokens = append(tokens, string(runes[start:index]))
			start = index
		}
	}

	return append(tokens, string(runes[start:]))
}
