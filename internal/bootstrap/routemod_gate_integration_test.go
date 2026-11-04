//go:build integration

package bootstrap

import (
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func routeGate(report entities.Report) (entities.Measure, bool) {
	at := slices.IndexFunc(report.Integrity, func(measure entities.Measure) bool {
		return strings.HasPrefix(measure.Name, "route registrations")
	})
	if at < 0 {
		return entities.Measure{}, false
	}

	return report.Integrity[at], true
}

func TestGoRouterGroupsTheStatementsOfOneResource(t *testing.T) {
	t.Parallel()

	unrelated := entities.DeclKey{File: "app/mounts.go", Kind: entities.KindFunc, Name: "Unrelated"}

	testCases := []struct {
		name      string
		raw       string
		wantSizes []int
	}{
		{
			name:      "groupMethods joins the statements of a path",
			raw:       routerFull,
			wantSizes: []int{3, 5},
		},
		{
			name: "groupMethods off leaves the statements alone",
			raw: `{"type":"go-router","handle":["*.GET","*.POST"],` +
				`"mount":["Mount*","*.Mount","*.Route"]}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph, _, _ := routemodResult(t, testCase.raw)

			var sizes []int

			for _, keys := range atomMembers(graph) {
				onlyStatements := !slices.ContainsFunc(keys, func(key entities.DeclKey) bool {
					return key.Kind != entities.KindStmt
				})
				if onlyStatements {
					sizes = append(sizes, len(keys))
				}
			}

			slices.Sort(sizes)

			if !slices.Equal(sizes, testCase.wantSizes) {
				t.Fatalf("statement-only atoms have sizes %v, want %v: %v",
					sizes, testCase.wantSizes, atomMembers(graph))
			}

			if decl, _ := graph.Decl(unrelated); decl.Atom != "" {
				t.Fatalf("Unrelated is in atom %q, want none", decl.Atom)
			}
		})
	}
}

func TestGoRouterWithoutMountPatternsMakesNoMountAtom(t *testing.T) {
	t.Parallel()

	mount := entities.DeclKey{File: "app/mounts.go", Kind: entities.KindFunc, Name: "MountUsers"}

	testCases := []struct {
		name     string
		raw      string
		wantAtom bool
	}{
		{
			name: "handle patterns alone",
			raw:  `{"type":"go-router","handle":["*.GET","*.POST"],"groupMethods":true}`,
		},
		{name: "handle and mount patterns", raw: routerFull, wantAtom: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph, _, _ := routemodResult(t, testCase.raw)

			if decl, _ := graph.Decl(mount); (decl.Atom != "") != testCase.wantAtom {
				t.Fatalf("MountUsers is in atom %q, want an atom: %t", decl.Atom, testCase.wantAtom)
			}
		})
	}
}

func TestGoRouterGateOnTheRoutemod(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		raw       string
		wantGate  bool
		wantValue int
		wantTotal int
		wantLines []string
	}{
		{name: "without the rule there is no route gate"},
		{
			name:      "the full rule measures the routes and the mounts",
			raw:       routerFull,
			wantGate:  true,
			wantValue: 21,
			wantTotal: 24,
			wantLines: []string{
				"0 of 3 mounts are placed apart from their first caller",
				"3 of 21 routes repeat the method and path of an earlier route",
				"0 of 21 routes are placed before their handler",
				"1 routes whose path is no string constant: not in the total",
				"2 routes with no handler the facts name (a func literal, a wrapped handler " +
					"or a func value): not in the total",
				"0 mounts and 0 routes the ordering never placed",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, _, report := routemodResult(t, testCase.raw)

			measure, found := routeGate(report)
			if found != testCase.wantGate {
				t.Fatalf("route gate found = %t, want %t in %v",
					found, testCase.wantGate, report.Integrity)
			}

			if measure.Value != testCase.wantValue || measure.Total != testCase.wantTotal {
				t.Fatalf("route gate = %d of %d, want %d of %d",
					measure.Value, measure.Total, testCase.wantValue, testCase.wantTotal)
			}

			text := strings.Join(measure.Excluded, "\n")
			for _, want := range testCase.wantLines {
				if !strings.Contains(text, want) {
					t.Fatalf("Excluded lacks %q:\n%s", want, text)
				}
			}
		})
	}
}

func TestGoRouterGateOnAMountingHistory(t *testing.T) {
	t.Parallel()

	_, _, withoutRule := atkResult(t, "")
	if len(withoutRule.Integrity) == 0 {
		t.Fatalf("no integrity report without the rule")
	}

	graph, order, report := atkResult(t, routerFull)

	measure, found := routeGate(report)
	if !found {
		t.Fatalf("no route gate in %v", report.Integrity)
	}

	if want := "0 of 3 mounts are placed apart from their first caller"; !strings.Contains(
		strings.Join(measure.Excluded, "\n"), want,
	) {
		t.Fatalf("Excluded lacks %q:\n%v", want, measure.Excluded)
	}

	if order.Last() < 2 {
		t.Fatalf("the ordering ends at %d, want a history of several commits", order.Last())
	}

	callers := map[entities.Position]bool{}

	for _, decl := range graph.Decls() {
		if decl.Key.Kind == entities.KindStmt && strings.HasPrefix(decl.Key.Name, "Routes#") {
			callers[order.Of(decl.Key)] = true
		}
	}

	testCases := []struct {
		name string
		pkg  string
	}{
		{name: "users", pkg: "users"},
		{name: "orders", pkg: "orders"},
		{name: "billing", pkg: "billing"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			mount := entities.DeclKey{
				File: testCase.pkg + "/" + testCase.pkg + ".go",
				Kind: entities.KindFunc,
				Name: "Mount",
			}
			if !callers[order.Of(mount)] {
				t.Fatalf("%s at %d, callers at %v", mount, order.Of(mount), callers)
			}
		})
	}
}
