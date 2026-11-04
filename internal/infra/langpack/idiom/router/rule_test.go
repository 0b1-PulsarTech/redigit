package router

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
)

func newRule(t *testing.T, raw string) *Rule {
	t.Helper()

	built, err := Constructor("go-router")(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("Constructor() = %v", err)
	}

	rule, isRule := built.(*Rule)
	if !isRule {
		t.Fatalf("Constructor() built %T, want *Rule", built)
	}

	return rule
}

func TestConstructor(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		raw     string
		wantErr error
	}{
		{
			name: "a config that handles builds the rule",
			raw:  `{"type":"go-router","handle":["*.GET"]}`,
		},
		{name: "no handle patterns", raw: `{"type":"go-router"}`, wantErr: ErrNoHandle},
		{
			name:    "an unknown key",
			raw:     `{"type":"go-router","handle":["*.GET"],"x":1}`,
			wantErr: ErrConfig,
		},
		{
			name:    "a malformed pattern",
			raw:     `{"type":"go-router","handle":["*.GE["]}`,
			wantErr: ErrBadPattern,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			built, err := Constructor("go-router")(json.RawMessage(testCase.raw))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Constructor() err = %v, want %v", err, testCase.wantErr)
			}

			if _, isRule := built.(*Rule); isRule != (testCase.wantErr == nil) {
				t.Fatalf("Constructor() built %T with err %v", built, err)
			}
		})
	}
}

func TestRuleTypeAndGlobs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		raw       string
		wantGlobs []string
	}{
		{
			name:      "paths are the globs",
			raw:       `{"type":"go-router","handle":["*.GET"],"paths":["api/","cmd/*/main.go"]}`,
			wantGlobs: []string{"api/", "cmd/*/main.go"},
		},
		{name: "no paths gives no globs", raw: `{"type":"go-router","handle":["*.GET"]}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := newRule(t, testCase.raw)

			if rule.Type() != "go-router" {
				t.Fatalf("Type() = %q, want go-router", rule.Type())
			}

			if !reflect.DeepEqual(rule.Globs(), testCase.wantGlobs) {
				t.Fatalf("Globs() = %v, want %v", rule.Globs(), testCase.wantGlobs)
			}
		})
	}
}

func TestRuleAtomsAndRoutes(t *testing.T) {
	t.Parallel()

	var (
		first, second = stmtKey("Setup", "1"), stmtKey("Setup", "2")
		router        = facts.TypeRef{Name: "m/router.Router"}
		known         = world{calls: map[entities.DeclKey][]facts.Call{
			first: {
				{
					Site:     first,
					Callee:   "m/router.Router.GET",
					Receiver: router,
					Literals: []string{"/u"},
				},
			},
			second: {
				{
					Site:     second,
					Callee:   "m/router.Router.POST",
					Receiver: router,
					Literals: []string{"/u"},
				},
			},
		}}
		decls = []entities.Decl{declAt(first, 1), declAt(second, 2)}
		label = "go-router:" + first.String()
	)

	testCases := []struct {
		name      string
		raw       string
		wantAtoms map[entities.DeclKey]string
	}{
		{
			name:      "groupMethods groups the statements of one path",
			raw:       `{"type":"go-router","handle":["*.GET","*.POST"],"groupMethods":true}`,
			wantAtoms: map[entities.DeclKey]string{first: label, second: label},
		},
		{
			name:      "no groupMethods groups nothing",
			raw:       `{"type":"go-router","handle":["*.GET","*.POST"]}`,
			wantAtoms: map[entities.DeclKey]string{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := newRule(t, testCase.raw)

			atoms, err := rule.Atoms(t.Context(), entities.Snapshot{}, decls, known)
			if err != nil {
				t.Fatalf("Atoms() = %v", err)
			}

			if !reflect.DeepEqual(atoms, testCase.wantAtoms) {
				t.Fatalf("Atoms() = %v, want %v", atoms, testCase.wantAtoms)
			}

			view, err := rule.Routes(decls, known)
			if err != nil || len(view.Routes) != 2 {
				t.Fatalf("Routes() = %+v, %v, want 2 routes", view, err)
			}
		})
	}
}

func TestRuleGateBindsTheConfig(t *testing.T) {
	t.Parallel()

	handler := fnKey("List")
	site := stmtKey("Setup", "1")
	call := facts.Call{
		Site: site, Callee: "m/router.Router.GET", Receiver: facts.TypeRef{Name: "m/router.Router"},
		Literals: []string{"/u"},
	}
	handled := call
	handled.FuncArgs = []entities.DeclKey{handler}

	testCases := []struct {
		name      string
		call      facts.Call
		wantValue int
		wantTotal int
	}{
		{
			name:      "a route with a path and a handler is judged",
			call:      handled,
			wantValue: 1,
			wantTotal: 1,
		},
		{name: "a route with no handler stays out of the total", call: call},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := newRule(t, `{"type":"go-router","handle":["*.GET"]}`)
			known := world{calls: map[entities.DeclKey][]facts.Call{site: {testCase.call}}}

			probe := rule.Gate(func([]entities.Decl) (idiom.Facts, error) { return known, nil })
			measured := probe.Measure(
				entities.NewGraph([]entities.Decl{declAt(site, 1)}, nil, nil),
				entities.Ordering{At: map[entities.DeclKey]entities.Position{site: 2, handler: 1}},
			)

			if probe.Name() != "go-router" || measured.Total != testCase.wantTotal ||
				measured.Value != testCase.wantValue {
				t.Fatalf(
					"gate %q measured %d of %d, want go-router %d of %d",
					probe.Name(),
					measured.Value,
					measured.Total,
					testCase.wantValue,
					testCase.wantTotal,
				)
			}
		})
	}
}
