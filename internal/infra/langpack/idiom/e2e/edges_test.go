package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

const defaultCfg = `{"type":"go-e2e","router":{"handle":["*.GET","*.POST","*.HandleFunc"],` +
	`"mount":["Mount*","*.Mount","*.Route"]},"dirs":["test/"]}`

var errNoFunc = errors.New("no such func")

type world map[entities.DeclKey][]facts.Call

func (set world) Calls(site entities.DeclKey) []facts.Call { return set[site] }

func (set world) CtorArgTypes(entities.DeclKey) ([]facts.TypeRef, error) { return nil, errNoFunc }

func (set world) Implementors(entities.DeclKey) []entities.DeclKey { return nil }

func (set world) StmtCallsInto(entities.DeclKey, string) ([]facts.Call, error) { return nil, nil }

func apiKey(kind entities.DeclKind, name string) entities.DeclKey {
	return entities.DeclKey{File: "api/api.go", Kind: kind, Name: name}
}

func suiteKey(dir, name string) entities.DeclKey {
	return entities.DeclKey{File: dir + "/api_test.go", Kind: entities.KindStmt, Name: name + "#1"}
}

func hitting(site entities.DeclKey, literals ...string) []facts.Call {
	return []facts.Call{{Site: site, Callee: "net/http.Get", Literals: literals}}
}

func edgeTo(dependent, provider entities.DeclKey) entities.Edge {
	return entities.Edge{Dependent: dependent, Provider: provider, Reason: entities.ReasonE2E}
}

type routeSet struct {
	health, healthPost, mounter, ping, orders, users entities.DeclKey
	itemParam, itemMe, plain, typed                  entities.DeclKey
	known                                            world
	decls                                            []entities.Decl
}

func newRouteSet() routeSet {
	var (
		router = facts.TypeRef{Name: "m/router.Router"}
		mux    = facts.TypeRef{Name: "net/http.ServeMux"}
		set    = routeSet{
			health:     apiKey(entities.KindStmt, "Setup#1"),
			healthPost: apiKey(entities.KindStmt, "Setup#4"),
			mounter:    apiKey(entities.KindStmt, "Setup#2"),
			ping:       apiKey(entities.KindStmt, "Setup#3"),
			orders:     apiKey(entities.KindStmt, "Setup#5"),
			itemParam:  apiKey(entities.KindStmt, "Setup#6"),
			itemMe:     apiKey(entities.KindStmt, "Setup#7"),
			plain:      apiKey(entities.KindStmt, "Setup#8"),
			typed:      apiKey(entities.KindStmt, "Setup#9"),
			users:      apiKey(entities.KindFunc, "MountUsers"),
		}
		route = func(callee string, site entities.DeclKey, path string) []facts.Call {
			return []facts.Call{
				{Site: site, Callee: callee, Receiver: router, Literals: []string{path}},
			}
		}
		serve = func(site entities.DeclKey, path string) []facts.Call {
			return []facts.Call{{
				Site: site, Callee: "net/http.ServeMux.HandleFunc", Receiver: mux,
				Literals: []string{path},
			}}
		}
		scope = facts.Call{
			Site:     set.ping,
			Callee:   "m/router.Router.Route",
			Receiver: router,
			Literals: []string{"/api"},
		}
	)

	set.known = world{
		set.health:     route("m/router.Router.GET", set.health, "/health"),
		set.healthPost: route("m/router.Router.POST", set.healthPost, "/health"),
		set.orders:     route("m/router.Router.GET", set.orders, "/orders/{id}"),
		set.itemParam:  route("m/router.Router.GET", set.itemParam, "/items/{id}"),
		set.itemMe:     route("m/router.Router.GET", set.itemMe, "/items/me"),
		set.plain:      serve(set.plain, "/plain"),
		set.typed:      serve(set.typed, "GET /typed"),
		set.users:      route("m/router.Router.GET", set.users, "/users"),
		set.ping: {{
			Site:     set.ping,
			Callee:   "m/router.Router.GET",
			Receiver: router,
			Literals: []string{"/ping"},
			Within:   &scope,
		}},
		set.mounter: {{Site: set.mounter, Callee: "m/app.MountUsers", CalleeKey: set.users}},
	}

	for key := range set.known {
		set.decls = append(set.decls, entities.Decl{Key: key})
	}

	return set
}

type edgeCase struct {
	name     string
	cfg      string
	suite    entities.Decl
	literals []string
	calls    []facts.Call
	want     []entities.Edge
	wantLog  string
}

func requireEdges(t *testing.T, known world, decls []entities.Decl, testCases []edgeCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var logged bytes.Buffer

			cfg := testCase.cfg
			if cfg == "" {
				cfg = defaultCfg
			}

			built, err := Constructor("go-e2e", slog.New(slog.NewTextHandler(&logged, nil)))(
				json.RawMessage(cfg),
			)
			if err != nil {
				t.Fatalf("Constructor() = %v", err)
			}

			rule, _ := built.(*Rule)
			calls := maps.Clone(known)
			calls[testCase.suite.Key] = hitting(testCase.suite.Key, testCase.literals...)

			if testCase.calls != nil {
				calls[testCase.suite.Key] = testCase.calls
			}

			got, err := rule.Edges(
				t.Context(),
				entities.Snapshot{},
				append(slices.Clone(decls), testCase.suite),
				calls,
			)
			if err != nil {
				t.Fatalf("Edges() = %v", err)
			}

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("Edges() = %v, want %v", got, testCase.want)
			}

			if !strings.Contains(logged.String(), testCase.wantLog) {
				t.Fatalf("log = %q, want it to say %q", logged.String(), testCase.wantLog)
			}
		})
	}
}

func TestRuleEdgesWait(t *testing.T) {
	t.Parallel()

	set := newRouteSet()

	requireEdges(t, set.known, set.decls, []edgeCase{
		{
			name:     "a literal url waits for the joined mounted path",
			suite:    entities.Decl{Key: suiteKey("test", "TestPing")},
			literals: []string{"/api/ping"},
			want:     []entities.Edge{edgeTo(suiteKey("test", "TestPing"), set.ping)},
		},
		{
			name:     "a route site that is a whole function waits with the mount caller",
			suite:    entities.Decl{Key: suiteKey("test", "TestUsers")},
			literals: []string{"/users"},
			want: []entities.Edge{
				edgeTo(suiteKey("test", "TestUsers"), set.users),
				edgeTo(suiteKey("test", "TestUsers"), set.mounter),
			},
		},
		{
			name:     "a method prefixed form waits for that method only",
			suite:    entities.Decl{Key: suiteKey("test", "TestPost")},
			literals: []string{"POST /health"},
			want:     []entities.Edge{edgeTo(suiteKey("test", "TestPost"), set.healthPost)},
		},
		{
			name:     "a bare path waits for every method of it",
			suite:    entities.Decl{Key: suiteKey("test", "TestAny")},
			literals: []string{"/health"},
			want: []entities.Edge{
				edgeTo(suiteKey("test", "TestAny"), set.health),
				edgeTo(suiteKey("test", "TestAny"), set.healthPost),
			},
		},
		{
			name:     "a parameter segment and a query",
			suite:    entities.Decl{Key: suiteKey("test", "TestOrder")},
			literals: []string{"/orders/42?full=1"},
			want:     []entities.Edge{edgeTo(suiteKey("test", "TestOrder"), set.orders)},
		},
		{
			name:     "edges come out sorted whatever the order of the urls",
			suite:    entities.Decl{Key: suiteKey("test", "TestBoth")},
			literals: []string{"/orders/42", "/health", "/health"},
			want: []entities.Edge{
				edgeTo(suiteKey("test", "TestBoth"), set.health),
				edgeTo(suiteKey("test", "TestBoth"), set.healthPost),
				edgeTo(suiteKey("test", "TestBoth"), set.orders),
			},
		},
	})
}

func TestRuleEdgesLeaveAlone(t *testing.T) {
	t.Parallel()

	set := newRouteSet()

	requireEdges(t, set.known, set.decls, []edgeCase{
		{
			name:     "an unknown url is reported and adds no edge",
			suite:    entities.Decl{Key: suiteKey("test", "TestGone")},
			literals: []string{"/nowhere"},
			wantLog:  "no known route path",
		},
		{
			name:     "a method the route lacks is unknown",
			suite:    entities.Decl{Key: suiteKey("test", "TestWrong")},
			literals: []string{"POST /orders/42"},
			wantLog:  "POST /orders/42",
		},
		{
			name:     "a dir that is not listed is ignored",
			suite:    entities.Decl{Key: suiteKey("other", "TestHealth")},
			literals: []string{"/health", "/nowhere"},
		},
		{
			name:     "text that is no url is left alone",
			suite:    entities.Decl{Key: suiteKey("test", "TestText")},
			literals: []string{"hello", ""},
		},
		{
			name: "a declaration that is not a test is left alone",
			suite: entities.Decl{Key: entities.DeclKey{
				File: "test/support.go", Kind: entities.KindFunc, Name: "Support",
			}},
			literals: []string{"/health", "/nowhere"},
		},
	})
}

func TestRuleListensToCodeKindsOnly(t *testing.T) {
	t.Parallel()

	set := newRouteSet()
	keyOf := func(kind entities.DeclKind, name string) entities.DeclKey {
		return entities.DeclKey{File: "test/api_test.go", Kind: kind, Name: name}
	}
	suiteOf := func(kind entities.DeclKind, name string) entities.Decl {
		return entities.Decl{Key: keyOf(kind, name), IsTest: true}
	}

	requireEdges(t, set.known, set.decls, []edgeCase{
		{
			name:     "a test function listens",
			suite:    suiteOf(entities.KindFunc, "TestHealth"),
			literals: []string{"/health"},
			want: []entities.Edge{
				edgeTo(keyOf(entities.KindFunc, "TestHealth"), set.health),
				edgeTo(keyOf(entities.KindFunc, "TestHealth"), set.healthPost),
			},
		},
		{
			name:     "a test method listens",
			suite:    suiteOf(entities.KindMethod, "Suite.TestUsers"),
			literals: []string{"/plain"},
			want: []entities.Edge{
				edgeTo(keyOf(entities.KindMethod, "Suite.TestUsers"), set.plain),
			},
		},
		{
			name:     "a test value does not listen",
			suite:    suiteOf(entities.KindValue, "wanted"),
			literals: []string{"/health", "/nowhere"},
		},
		{
			name:     "a test type does not listen",
			suite:    suiteOf(entities.KindType, "suite"),
			literals: []string{"/health", "/nowhere"},
		},
	})
}
