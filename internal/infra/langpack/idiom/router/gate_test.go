package router

import (
	"errors"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
)

type gateCase struct {
	name       string
	calls      map[entities.DeclKey][]facts.Call
	positions  map[entities.DeclKey]entities.Position
	factsErr   error
	wantValue  int
	wantTotal  int
	wantText   []string
	wantAbsent []string
}

func getCall(site entities.DeclKey, verb, path string, args ...entities.DeclKey) facts.Call {
	call := facts.Call{
		Site: site, Callee: "m/router.Router." + verb, FuncArgs: args,
		Receiver: facts.TypeRef{Name: "m/router.Router"},
	}

	if path != "" {
		call.Literals = []string{path}
	}

	return call
}

func requireGate(t *testing.T, testCases []gateCase) {
	t.Helper()

	decls := []entities.Decl{
		declAt(stmtKey("Setup", "1"), 10), declAt(stmtKey("Setup", "2"), 20),
		declAt(fnKey("List"), 1), declAt(fnKey("MountUsers"), 2),
		declAt(stmtKey("Other", "1"), 30), declAt(stmtKey("Other", "2"), 35),
		declAt(fnKey("Whole"), 40),
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			probe := gate{
				name: "go-router",
				cfg:  Config{Handle: []string{"*.GET", "*.POST"}, Mount: []string{"Mount*"}},
				factsOf: func([]entities.Decl) (idiom.Facts, error) {
					return world{calls: testCase.calls}, testCase.factsErr
				},
			}

			measured := probe.Measure(
				entities.NewGraph(decls, nil, nil), entities.Ordering{At: testCase.positions},
			)
			if measured.Value != testCase.wantValue || measured.Total != testCase.wantTotal {
				t.Fatalf("Measure() = %d of %d, want %d of %d",
					measured.Value, measured.Total, testCase.wantValue, testCase.wantTotal)
			}

			if err := measured.Validate(); err != nil {
				t.Fatalf("Validate() = %v", err)
			}

			text := strings.Join(measured.Excluded, "\n")
			for _, want := range testCase.wantText {
				if !strings.Contains(text, want) {
					t.Fatalf("Excluded lacks %q:\n%s", want, text)
				}
			}

			for _, unwanted := range testCase.wantAbsent {
				if strings.Contains(text, unwanted) {
					t.Fatalf("Excluded holds %q:\n%s", unwanted, text)
				}
			}
		})
	}
}

func TestGateMeasureRoutes(t *testing.T) {
	t.Parallel()

	first := stmtKey("Setup", "1")
	handler := fnKey("List")

	testCases := []gateCase{
		{
			name: "a route placed after its handler is clean",
			calls: map[entities.DeclKey][]facts.Call{
				first: {getCall(first, "GET", "/u", handler)},
			},
			positions:  map[entities.DeclKey]entities.Position{first: 3, handler: 1},
			wantValue:  1,
			wantTotal:  1,
			wantText:   []string{"0 of 1 routes are placed before their handler"},
			wantAbsent: []string{"handle matched no call site"},
		},
		{
			name: "a handler the ordering never placed is not early",
			calls: map[entities.DeclKey][]facts.Call{
				first: {getCall(first, "GET", "/u", handler)},
			},
			positions:  map[entities.DeclKey]entities.Position{first: 3},
			wantValue:  1,
			wantTotal:  1,
			wantText:   []string{"0 of 1 routes are placed before their handler"},
			wantAbsent: []string{"1 of 1 routes are placed before their handler"},
		},
		{
			name: "a route at the position of its handler is clean",
			calls: map[entities.DeclKey][]facts.Call{
				first: {getCall(first, "GET", "/u", handler)},
			},
			positions: map[entities.DeclKey]entities.Position{first: 3, handler: 3},
			wantValue: 1,
			wantTotal: 1,
			wantText:  []string{"0 of 1 routes are placed before their handler"},
		},
		{
			name: "a route placed before its handler is counted",
			calls: map[entities.DeclKey][]facts.Call{
				first: {getCall(first, "GET", "/u", handler)},
			},
			positions: map[entities.DeclKey]entities.Position{first: 1, handler: 3},
			wantValue: 0,
			wantTotal: 1,
			wantText:  []string{"1 of 1 routes are placed before their handler"},
		},
		{
			name: "a route nothing placed is left out",
			calls: map[entities.DeclKey][]facts.Call{
				first: {getCall(first, "GET", "/u", handler)},
			},
			positions: map[entities.DeclKey]entities.Position{handler: 1},
			wantText:  []string{"0 mounts and 1 routes the ordering never placed"},
		},
	}

	requireGate(t, testCases)
}

func TestGateMeasureRepeats(t *testing.T) {
	t.Parallel()

	first, second := stmtKey("Setup", "1"), stmtKey("Setup", "2")
	other, otherAgain := stmtKey("Other", "1"), stmtKey("Other", "2")
	handler := fnKey("List")

	testCases := []gateCase{
		{
			name: "a route repeating a method and path is counted",
			calls: map[entities.DeclKey][]facts.Call{
				first: {
					getCall(first, "GET", "/u", handler),
				},
				second: {getCall(second, "GET", "/u", handler)},
			},
			positions: map[entities.DeclKey]entities.Position{first: 3, second: 4, handler: 1},
			wantValue: 1, wantTotal: 2,
			wantText: []string{"1 of 2 routes repeat the method and path of an earlier route"},
		},
		{
			name: "a route repeating one of another function is not judged as a repeat",
			calls: map[entities.DeclKey][]facts.Call{
				first: {getCall(first, "GET", "/u", handler)},
				other: {getCall(other, "GET", "/u", handler)},
			},
			positions: map[entities.DeclKey]entities.Position{first: 3, other: 4, handler: 1},
			wantValue: 2, wantTotal: 2,
			wantText: []string{
				"0 of 2 routes repeat the method and path of an earlier route",
				"1 routes repeat the method and path of an earlier route of another function: not judged",
			},
		},
		{
			name: "a repeat inside another function is counted apart from the crossed one",
			calls: map[entities.DeclKey][]facts.Call{
				first:      {getCall(first, "GET", "/u", handler)},
				other:      {getCall(other, "GET", "/u", handler)},
				otherAgain: {getCall(otherAgain, "GET", "/u", handler)},
			},
			positions: map[entities.DeclKey]entities.Position{
				first: 3, other: 4, otherAgain: 5, handler: 1,
			},
			wantValue: 2, wantTotal: 3,
			wantText: []string{
				"1 of 3 routes repeat the method and path of an earlier route in the same function",
				"1 routes repeat the method and path of an earlier route of another function: not judged",
			},
		},
		{
			name: "another method on the path is no repeat",
			calls: map[entities.DeclKey][]facts.Call{
				first: {
					getCall(first, "GET", "/u", handler),
				},
				second: {getCall(second, "POST", "/u", handler)},
			},
			positions: map[entities.DeclKey]entities.Position{first: 3, second: 4, handler: 1},
			wantValue: 2, wantTotal: 2,
		},
	}

	requireGate(t, testCases)
}

func TestGateMeasureMounts(t *testing.T) {
	t.Parallel()

	first := stmtKey("Setup", "1")
	handler, users := fnKey("List"), fnKey("MountUsers")
	mount := facts.Call{Site: first, Callee: "m/api.MountUsers", CalleeKey: users}
	inner := map[entities.DeclKey][]facts.Call{
		first: {mount}, users: {getCall(users, "GET", "/m", handler)},
	}

	testCases := []gateCase{
		{
			name:      "a mount placed with its caller is clean",
			calls:     inner,
			positions: map[entities.DeclKey]entities.Position{first: 3, users: 3, handler: 1},
			wantValue: 2, wantTotal: 2,
			wantText: []string{"0 of 1 mounts are placed apart from their first caller"},
		},
		{
			name:      "a mount whose caller the ordering never placed is not counted",
			calls:     inner,
			positions: map[entities.DeclKey]entities.Position{users: 3, handler: 1},
			wantValue: 1, wantTotal: 1,
			wantText: []string{"1 mounts and 0 routes the ordering never placed"},
		},
		{
			name: "a mount called only by a whole func is counted as unmet",
			calls: map[entities.DeclKey][]facts.Call{
				fnKey("Whole"): {
					{Site: fnKey("Whole"), Callee: "m/api.MountUsers", CalleeKey: users},
				},
				users: {getCall(users, "GET", "/m", handler)},
			},
			positions: map[entities.DeclKey]entities.Position{users: 3, handler: 1},
			wantValue: 1, wantTotal: 1,
			wantText: []string{
				"1 mounts are called only from a func that was not cut into statements: " +
					"not compared with a caller",
			},
		},
		{
			name:      "a mount the ordering never placed is not counted",
			calls:     inner,
			positions: map[entities.DeclKey]entities.Position{first: 3, handler: 1},
			wantText:  []string{"1 mounts and 1 routes the ordering never placed"},
		},
		{
			name:      "a mount placed apart from its caller is counted",
			calls:     inner,
			positions: map[entities.DeclKey]entities.Position{first: 5, users: 2, handler: 1},
			wantValue: 1, wantTotal: 2,
			wantText: []string{"1 of 1 mounts are placed apart from their first caller"},
		},
	}

	requireGate(t, testCases)
}

func TestGateMeasureBlindness(t *testing.T) {
	t.Parallel()

	first, second := stmtKey("Setup", "1"), stmtKey("Setup", "2")
	handler := fnKey("List")
	errFacts := errors.New("no facts")

	testCases := []gateCase{
		{
			name: "a path that is no constant stays out of the total",
			calls: map[entities.DeclKey][]facts.Call{
				first: {
					getCall(first, "GET", "", handler),
				},
				second: {getCall(second, "GET", "", handler)},
			},
			positions: map[entities.DeclKey]entities.Position{first: 3, second: 4, handler: 1},
			wantValue: 0, wantTotal: 0,
			wantText: []string{"2 routes whose path is no string constant: not in the total"},
		},
		{
			name: "a constant that is no path is counted apart from a path no constant gives",
			calls: map[entities.DeclKey][]facts.Call{
				first:  {getCall(first, "GET", "rel", handler)},
				second: {getCall(second, "GET", "", handler)},
			},
			positions: map[entities.DeclKey]entities.Position{first: 3, second: 4, handler: 1},
			wantText: []string{
				"1 routes whose path is no string constant: not in the total",
				"1 routes whose constants are no path (relative, empty, a host): not in the total",
			},
		},
		{
			name:      "a handler no fact names stays out of the total",
			calls:     map[entities.DeclKey][]facts.Call{first: {getCall(first, "GET", "/u")}},
			positions: map[entities.DeclKey]entities.Position{first: 3},
			wantValue: 0, wantTotal: 0,
			wantText: []string{
				"1 routes with no handler the facts name (a func literal, a wrapped handler or a func value): " +
					"not in the total",
			},
		},
		{
			name:     "facts that fail leave the measure unmeasured",
			factsErr: errFacts,
			wantText: []string{"not measured: " + errFacts.Error()},
		},
		{
			name: "a handle matching no call site says how a pattern matches",
			wantText: []string{
				"handle matched no call site: a pattern matches the short name, " +
					"such as Router.GET, or the qualified one, such as github.com/go-fuego/fuego.Get, " +
					"and * does not cross /, so *.Get misses that func",
			},
		},
	}

	requireGate(t, testCases)
}

func TestGateName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		gate gate
		want string
	}{
		{name: "the rule's own name", gate: gate{name: "go-router"}, want: "go-router"},
		{name: "another name the constructor gave", gate: gate{name: "routes"}, want: "routes"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.gate.Name(); got != testCase.want {
				t.Fatalf("Name() = %q, want %q", got, testCase.want)
			}
		})
	}
}
