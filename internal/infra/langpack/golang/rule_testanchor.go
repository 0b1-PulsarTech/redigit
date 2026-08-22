package golang

import (
	"context"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// testAnchorRule pulls a test toward the declaration it exercises. It is an
// Anchor, not an Edge: "not before the function" is only a floor, and with
// only the floor a package's whole suite arrived six hundred commits late.
//
// Finding the subject is two steps. First, the twin source file: the longest
// prefix of the test file's stem, cutting at each "_", that names a real
// file in the same directory. Second, the subject within it: strip the
// Test/Benchmark/Fuzz/Example prefix, tokenize what remains by "_" or (with
// none) by capital letter, and walk the token boundaries left to right,
// checking at each cut whether the growing prefix names a type or the
// shrinking suffix names a method. The LAST match wins, not the first:
// TestFSStorePutGet names the type FS and the methods Put and Get, and a
// scan that stopped at the first match (the type, at the first cut) reported
// a defect where the test was correctly waiting on the method.
type testAnchorRule struct{}

func (testAnchorRule) Name() string { return "test-anchor" }

func (testAnchorRule) Edges(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Edge, error) {
	return nil, nil
}

func (testAnchorRule) Anchors(
	_ context.Context,
	_ entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	byFile := groupByFile(decls)
	byUnit := groupByUnit(decls)

	var anchors []entities.Anchor

	for _, d := range decls {
		if !d.IsTest || d.Key.Kind != entities.KindFunc {
			continue
		}

		prefix, ok := testSubjectPrefix(d.Key.Name)
		if !ok {
			continue
		}

		scope := byUnit[d.Unit]
		if twin, found := twinFile(
			filepath.Dir(d.Key.File),
			filepath.Base(d.Key.File),
			byFile,
		); found {
			scope = byFile[twin]
		}

		if to, found := bestSubject(scope, camelOrSnakeTokens(prefix)); found {
			anchors = append(
				anchors,
				entities.Anchor{Decl: d.Key, To: to, Reason: entities.ReasonTestOf},
			)
		}
	}

	return anchors, nil
}

func groupByUnit(decls []entities.Decl) map[entities.UnitID][]entities.Decl {
	byUnit := make(map[entities.UnitID][]entities.Decl, len(decls))
	for _, d := range decls {
		byUnit[d.Unit] = append(byUnit[d.Unit], d)
	}

	return byUnit
}

// twinFile finds the longest prefix of testBase's stem, cutting at "_", that
// is a real file in dir.
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

// testSubjectPrefix strips the go test binary's recognized prefix, reporting
// false for a name (or a case) it would not have run either.
func testSubjectPrefix(name string) (string, bool) {
	for _, prefix := range [...]string{"Test", "Benchmark", "Fuzz", "Example"} {
		if rest, ok := strings.CutPrefix(name, prefix); ok && rest != "" {
			return rest, true
		}
	}

	return "", false
}

// camelOrSnakeTokens splits on "_" when present, else on capital-letter word
// boundaries, keeping a run of capitals ("FS") together as one token.
func camelOrSnakeTokens(name string) []string {
	if strings.Contains(name, "_") {
		return strings.Split(name, "_")
	}

	runes := []rune(name)

	var tokens []string

	start := 0

	for i := 1; i < len(runes); i++ {
		prevUpper := unicode.IsUpper(runes[i-1])
		curUpper := unicode.IsUpper(runes[i])
		nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])

		if curUpper && (!prevUpper || nextLower) {
			tokens = append(tokens, string(runes[start:i]))
			start = i
		}
	}

	return append(tokens, string(runes[start:]))
}

// bestSubject walks the token boundaries left to right. At each cut, the
// growing left side is checked against a type name and the shrinking right
// side against a bare method name; either match overwrites the previous
// best, so the last (most specific) match along the walk wins.
func bestSubject(scope []entities.Decl, tokens []string) (entities.DeclKey, bool) {
	var best entities.DeclKey

	found := false

	for cut := 1; cut < len(tokens); cut++ {
		left := strings.Join(tokens[:cut], "")
		right := strings.Join(tokens[cut:], "")

		if key, ok := findTypeNamed(scope, left); ok {
			best, found = key, true
		}

		if key, ok := findMethodNamed(scope, right); ok {
			best, found = key, true
		}
	}

	if !found {
		if key, ok := findTypeNamed(scope, strings.Join(tokens, "")); ok {
			return key, true
		}
	}

	return best, found
}

// findMethodNamed looks up a method by its bare method part (after the
// receiver's "."), the shape a test name's suffix names.
func findMethodNamed(decls []entities.Decl, name string) (entities.DeclKey, bool) {
	for _, d := range decls {
		if d.Key.Kind != entities.KindMethod {
			continue
		}

		if _, method, ok := strings.Cut(d.Key.Name, "."); ok && method == name {
			return d.Key, true
		}
	}

	return entities.DeclKey{}, false
}
