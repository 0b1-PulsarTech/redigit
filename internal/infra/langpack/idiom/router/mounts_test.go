package router

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

type mountCase struct {
	name    string
	calls   []facts.Call
	bodies  map[entities.DeclKey][]facts.Call
	decls   []entities.Decl
	routers map[string]bool
	want    []Mount
}

func requireMounts(t *testing.T, testCases []mountCase) {
	t.Helper()

	cfg := Config{Handle: []string{"*.GET"}, Mount: []string{"Mount*", "*.Mount"}}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			known := newStitched(world{calls: testCase.bodies}, testCase.decls)
			got := findMounts(cfg, testCase.calls, known, testCase.routers)

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("findMounts() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestFindMounts(t *testing.T) {
	t.Parallel()

	first, second := stmtKey("Setup", "1"), stmtKey("Setup", "2")
	users, orders := fnKey("MountUsers"), fnKey("MountOrders")
	router := facts.TypeRef{Name: "m/router.Router"}
	route := facts.Call{Callee: "m/router.Router.GET", Receiver: router, Literals: []string{"/x"}}
	bodies := map[entities.DeclKey][]facts.Call{users: {route}, orders: {route}}
	callUsers := facts.Call{Site: first, Callee: "m/api.MountUsers", CalleeKey: users}
	callOrders := facts.Call{Site: second, Callee: "m/api.MountOrders", CalleeKey: orders}
	again := facts.Call{Site: second, Callee: "m/api.MountUsers", CalleeKey: users}

	testCases := []mountCase{
		{
			name:   "a plain function that registers routes",
			calls:  []facts.Call{callUsers},
			bodies: bodies,
			want:   []Mount{{Fn: users, Caller: first}},
		},
		{
			name:   "the first caller is the one named",
			calls:  []facts.Call{callUsers, again},
			bodies: bodies,
			want:   []Mount{{Fn: users, Caller: first}},
		},
		{
			name:   "two mounts keep the order of their first call",
			calls:  []facts.Call{callOrders, callUsers},
			bodies: bodies,
			want:   []Mount{{Fn: orders, Caller: second}, {Fn: users, Caller: first}},
		},
		{
			name:   "a function whose routes sit in its cut statements is a mount",
			calls:  []facts.Call{callUsers},
			bodies: map[entities.DeclKey][]facts.Call{stmtKey("MountUsers", "0"): {route}},
			decls:  []entities.Decl{declAt(stmtKey("MountUsers", "0"), 0)},
			want:   []Mount{{Fn: users, Caller: first}},
		},
		{
			name:   "a function that only mounts others is a mount",
			calls:  []facts.Call{callUsers},
			bodies: map[entities.DeclKey][]facts.Call{users: {{Callee: "m/api.MountOrders"}}},
			want:   []Mount{{Fn: users, Caller: first}},
		},
	}

	requireMounts(t, testCases)
}

func TestFindMountsByReceiver(t *testing.T) {
	t.Parallel()

	first, users := stmtKey("Setup", "1"), fnKey("MountUsers")
	router := facts.TypeRef{Name: "m/router.Router"}
	routers := map[string]bool{"m/router.Router": true}
	route := facts.Call{Callee: "m/router.Router.GET", Receiver: router, Literals: []string{"/x"}}
	own := entities.DeclKey{
		File: "router/router.go",
		Kind: entities.KindMethod,
		Name: "Router.Mount",
	}
	bodies := map[entities.DeclKey][]facts.Call{own: {route}, users: {route}}
	onRouter := facts.Call{
		Site:      first,
		Callee:    "m/router.Router.Mount",
		Receiver:  router,
		CalleeKey: own,
	}
	handed := onRouter
	handed.FuncArgs = []entities.DeclKey{users}
	onHandler := facts.Call{
		Site: first, Callee: "m/api.Handler.MountRoutes",
		Receiver: facts.TypeRef{Name: "m/api.Handler"}, CalleeKey: users,
	}

	nameless := onHandler
	nameless.Receiver = facts.TypeRef{}

	testCases := []mountCase{
		{
			name:    "a call with no receiver name is never the router's own",
			calls:   []facts.Call{nameless},
			bodies:  bodies,
			routers: map[string]bool{"": true},
			want:    []Mount{{Fn: users, Caller: first}},
		},
		{
			name:    "a method of the router type is the router's own",
			calls:   []facts.Call{onRouter},
			bodies:  bodies,
			routers: routers,
		},
		{
			name:    "a func handed to a router method is the mount",
			calls:   []facts.Call{handed},
			bodies:  bodies,
			routers: routers,
			want:    []Mount{{Fn: users, Caller: first}},
		},
		{
			name:    "a method of another type is a mount",
			calls:   []facts.Call{onHandler},
			bodies:  bodies,
			routers: routers,
			want:    []Mount{{Fn: users, Caller: first}},
		},
	}

	requireMounts(t, testCases)
}

func TestFindMountsRefuses(t *testing.T) {
	t.Parallel()

	first, users := stmtKey("Setup", "1"), fnKey("MountUsers")
	route := facts.Call{
		Callee: "m/router.Router.GET", Receiver: facts.TypeRef{Name: "m/router.Router"},
		Literals: []string{"/x"},
	}
	testFn := entities.DeclKey{File: "api/api_test.go", Kind: entities.KindFunc, Name: "MountT"}
	inner := stmtKey("MountUsers", "0")

	testCases := []mountCase{
		{
			name:   "a function that registers nothing is no mount",
			calls:  []facts.Call{{Site: first, Callee: "m/api.MountUsers", CalleeKey: users}},
			bodies: map[entities.DeclKey][]facts.Call{users: {{Callee: "fmt.Println"}}},
		},
		{
			name:   "a function that calls itself is no mount of itself",
			calls:  []facts.Call{{Site: inner, Callee: "m/api.MountUsers", CalleeKey: users}},
			bodies: map[entities.DeclKey][]facts.Call{users: {route}},
			decls:  []entities.Decl{declAt(inner, 0)},
		},
		{
			name:   "a callee in a test file is no mount",
			calls:  []facts.Call{{Site: first, Callee: "m/api.MountT", CalleeKey: testFn}},
			bodies: map[entities.DeclKey][]facts.Call{testFn: {route}},
		},
		{
			name: "a caller that is a whole func is no caller",
			calls: []facts.Call{
				{Site: fnKey("Setup"), Callee: "m/api.MountUsers", CalleeKey: users},
			},
			bodies: map[entities.DeclKey][]facts.Call{users: {route}},
		},
		{
			name:   "a call that reaches no func in the tree has no mount",
			calls:  []facts.Call{{Site: first, Callee: "x/y.Mount"}},
			bodies: map[entities.DeclKey][]facts.Call{users: {route}},
		},
	}

	requireMounts(t, testCases)
}

func TestUnmetMounts(t *testing.T) {
	t.Parallel()

	first, users := stmtKey("Setup", "1"), fnKey("MountUsers")
	route := facts.Call{
		Callee: "m/router.Router.GET", Receiver: facts.TypeRef{Name: "m/router.Router"},
		Literals: []string{"/x"},
	}
	testFn := entities.DeclKey{File: "api/api_test.go", Kind: entities.KindFunc, Name: "MountT"}
	bodies := map[entities.DeclKey][]facts.Call{users: {route}, testFn: {route}}
	byFunc := facts.Call{Site: fnKey("Setup"), Callee: "m/api.MountUsers", CalleeKey: users}
	byOther := facts.Call{Site: fnKey("Other"), Callee: "m/api.MountUsers", CalleeKey: users}
	byStmt := facts.Call{Site: first, Callee: "m/api.MountUsers", CalleeKey: users}

	testCases := []struct {
		name  string
		calls []facts.Call
		met   []Mount
		want  int
	}{
		{
			name:  "a mount called only by a whole func is unmet",
			calls: []facts.Call{byFunc},
			want:  1,
		},
		{
			name:  "two whole funcs calling one mount count it once",
			calls: []facts.Call{byFunc, byOther},
			want:  1,
		},
		{
			name:  "a mount a statement also calls is met",
			calls: []facts.Call{byFunc, byStmt},
			met:   []Mount{{Fn: users, Caller: first}},
		},
		{name: "a statement caller leaves nothing unmet", calls: []facts.Call{byStmt}},
		{
			name:  "a callee in a test file is no mount",
			calls: []facts.Call{{Site: fnKey("Setup"), CalleeKey: testFn}},
		},
		{
			name: "a whole func calling its own mount is no unmet mount",
			calls: []facts.Call{
				{Site: users, Callee: "m/api.MountUsers", CalleeKey: users},
			},
		},
		{
			name:  "a func that registers nothing is no mount",
			calls: []facts.Call{{Site: fnKey("Setup"), CalleeKey: fnKey("Quiet")}},
		},
	}

	cfg := Config{Handle: []string{"*.GET"}, Mount: []string{"Mount*", "*.Mount"}}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := unmetMounts(cfg, testCase.calls, world{calls: bodies}, nil, testCase.met)
			if got != testCase.want {
				t.Fatalf("unmetMounts() = %d, want %d", got, testCase.want)
			}
		})
	}
}
