package router

import (
	"errors"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

var errNoFunc = errors.New("no such func")

type world struct {
	calls map[entities.DeclKey][]facts.Call
}

func (set world) Calls(site entities.DeclKey) []facts.Call { return set.calls[site] }

func (set world) CtorArgTypes(entities.DeclKey) ([]facts.TypeRef, error) { return nil, errNoFunc }

func (set world) Implementors(entities.DeclKey) []entities.DeclKey { return nil }

func (set world) StmtCallsInto(entities.DeclKey, string) ([]facts.Call, error) { return nil, nil }

func fnKey(name string) entities.DeclKey {
	return entities.DeclKey{File: "api/api.go", Kind: entities.KindFunc, Name: name}
}

func stmtKey(name string, at string) entities.DeclKey {
	return entities.DeclKey{File: "api/api.go", Kind: entities.KindStmt, Name: name + "#" + at}
}

func declAt(key entities.DeclKey, start int) entities.Decl {
	return entities.Decl{Key: key, Span: entities.Span{Start: start, End: start + 1}}
}

type buildCase struct {
	name        string
	cfg         Config
	decls       []entities.Decl
	calls       map[entities.DeclKey][]facts.Call
	want        []Route
	wantMounts  []Mount
	wantBlind   int
	wantInline  int
	wantNonPath int
	wantUnmet   int
}

func requireBuild(t *testing.T, testCases []buildCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			view, err := build(testCase.cfg, testCase.decls, world{calls: testCase.calls})
			if err != nil {
				t.Fatalf("build() = %v", err)
			}

			if !slices.Equal(view.Routes, testCase.want) {
				t.Fatalf("Routes = %+v, want %+v", view.Routes, testCase.want)
			}

			if !slices.Equal(view.Mounts, testCase.wantMounts) {
				t.Fatalf("Mounts = %+v, want %+v", view.Mounts, testCase.wantMounts)
			}

			if view.Blind() != testCase.wantBlind || view.Inline() != testCase.wantInline {
				t.Fatalf("Blind() = %d, Inline() = %d, want %d and %d",
					view.Blind(), view.Inline(), testCase.wantBlind, testCase.wantInline)
			}

			if view.nonPath != testCase.wantNonPath {
				t.Fatalf("nonPath = %d, want %d", view.nonPath, testCase.wantNonPath)
			}

			if view.Unmet != testCase.wantUnmet {
				t.Fatalf("Unmet = %d, want %d", view.Unmet, testCase.wantUnmet)
			}
		})
	}
}

func TestBuildRoutes(t *testing.T) {
	t.Parallel()

	first, second := stmtKey("Setup", "1"), stmtKey("Setup", "2")
	handler := fnKey("List")
	router := facts.TypeRef{Name: "m/router.Router"}
	get := facts.Call{
		Site: first, Callee: "m/router.Router.GET", Receiver: router,
		Literals: []string{"/users"}, FuncArgs: []entities.DeclKey{handler},
	}
	post := facts.Call{
		Site:     second,
		Callee:   "m/router.Router.POST",
		Receiver: router,
		Literals: []string{"/users"},
	}
	blind := facts.Call{Site: second, Callee: "m/router.Router.GET", Receiver: router}
	relative := facts.Call{
		Site: second, Callee: "m/router.Router.GET", Receiver: router, Literals: []string{"rel"},
	}
	inside := facts.Call{
		Site: second, Callee: "m/router.Router.GET", Receiver: router,
		Literals: []string{"/users"}, FuncArgs: []entities.DeclKey{handler},
		Within: &facts.Call{
			Callee: "m/router.Router.Route", Receiver: router, Literals: []string{"/api"},
		},
	}
	cfg := Config{Handle: []string{"*.GET", "*.POST"}, Mount: []string{"*.Route"}}

	testCases := []buildCase{
		{
			name:  "a GET and a POST in source order",
			cfg:   cfg,
			decls: []entities.Decl{declAt(second, 20), declAt(first, 10)},
			calls: map[entities.DeclKey][]facts.Call{first: {get}, second: {post}},
			want: []Route{
				{Method: "GET", Path: "/users", Handler: handler, Site: first},
				{Method: "POST", Path: "/users", Site: second},
			},
			wantInline: 1,
		},
		{
			name:        "a constant that is no path is blind and counted apart",
			cfg:         cfg,
			decls:       []entities.Decl{declAt(second, 20)},
			calls:       map[entities.DeclKey][]facts.Call{second: {relative}},
			want:        []Route{{Method: "GET", Site: second}},
			wantBlind:   1,
			wantInline:  1,
			wantNonPath: 1,
		},
		{
			name:  "a group prefix joins the path",
			cfg:   cfg,
			decls: []entities.Decl{declAt(second, 20)},
			calls: map[entities.DeclKey][]facts.Call{second: {inside}},
			want:  []Route{{Method: "GET", Path: "/api/users", Handler: handler, Site: second}},
		},
		{
			name:       "a non-literal path is blind",
			cfg:        cfg,
			decls:      []entities.Decl{declAt(second, 20)},
			calls:      map[entities.DeclKey][]facts.Call{second: {blind}},
			want:       []Route{{Method: "GET", Site: second}},
			wantBlind:  1,
			wantInline: 1,
		},
	}

	requireBuild(t, testCases)
}

func TestBuildLeavesOutWhatIsNoRoute(t *testing.T) {
	t.Parallel()

	first := stmtKey("Setup", "1")
	router := facts.TypeRef{Name: "m/router.Router"}
	get := facts.Call{
		Site:     first,
		Callee:   "m/router.Router.GET",
		Receiver: router,
		Literals: []string{"/u"},
	}
	other := facts.Call{
		Site:     first,
		Callee:   "m/router.Router.Use",
		Receiver: router,
		Literals: []string{"/x"},
	}
	typeKey := entities.DeclKey{File: "api/api.go", Kind: entities.KindType, Name: "T"}
	testSide := declAt(stmtKey("Setup", "9"), 0)
	testSide.IsTest = true
	cfg := Config{Handle: []string{"*.GET"}}

	testCases := []buildCase{
		{
			name:  "a call that is no route is left out",
			cfg:   cfg,
			decls: []entities.Decl{declAt(first, 10)},
			calls: map[entities.DeclKey][]facts.Call{first: {other}},
		},
		{
			name:  "a test side site is left out",
			cfg:   cfg,
			decls: []entities.Decl{testSide},
			calls: map[entities.DeclKey][]facts.Call{testSide.Key: {get}},
		},
		{
			name:  "a file outside the paths is left out",
			cfg:   Config{Handle: cfg.Handle, Paths: []string{"other/"}},
			decls: []entities.Decl{declAt(first, 10)},
			calls: map[entities.DeclKey][]facts.Call{first: {get}},
		},
		{
			name:  "a type declaration holds no calls",
			cfg:   cfg,
			decls: []entities.Decl{declAt(typeKey, 1)},
			calls: map[entities.DeclKey][]facts.Call{typeKey: {get}},
		},
	}

	requireBuild(t, testCases)
}

func TestBuildFindsMounts(t *testing.T) {
	t.Parallel()

	first, users := stmtKey("Setup", "1"), fnKey("MountUsers")
	router := facts.TypeRef{Name: "m/router.Router"}
	get := facts.Call{
		Site:     users,
		Callee:   "m/router.Router.GET",
		Receiver: router,
		Literals: []string{"/u"},
	}
	mount := facts.Call{Site: first, Callee: "m/api.MountUsers", CalleeKey: users}
	cfg := Config{Handle: []string{"*.GET"}, Mount: []string{"Mount*"}}

	own := entities.DeclKey{
		File: "router/router.go",
		Kind: entities.KindMethod,
		Name: "Router.Mount",
	}
	onRouter := facts.Call{
		Site: first, Callee: "m/router.Router.Mount", Receiver: router, CalleeKey: own,
	}
	ownGet := get
	ownGet.Site = own

	testCases := []buildCase{
		{
			name: "a method of the type that receives route calls is no mount",
			cfg:  Config{Handle: cfg.Handle, Mount: []string{"*.Mount"}},
			decls: []entities.Decl{
				declAt(first, 10), declAt(users, 20), declAt(own, 30),
			},
			calls: map[entities.DeclKey][]facts.Call{
				first: {onRouter}, users: {get}, own: {ownGet},
			},
			want: []Route{
				{Method: "GET", Path: "/u", Site: users},
				{Method: "GET", Path: "/u", Site: own},
			},
			wantInline: 2,
		},
		{
			name:       "a mount call reaching a func that registers routes",
			cfg:        cfg,
			decls:      []entities.Decl{declAt(first, 10), declAt(users, 20)},
			calls:      map[entities.DeclKey][]facts.Call{first: {mount}, users: {get}},
			want:       []Route{{Method: "GET", Path: "/u", Site: users}},
			wantMounts: []Mount{{Fn: users, Caller: first}},
			wantInline: 1,
		},
		{
			name: "a mount called only by a whole func is unmet",
			cfg:  cfg,
			decls: []entities.Decl{
				declAt(fnKey("Setup"), 10), declAt(users, 20),
			},
			calls: map[entities.DeclKey][]facts.Call{
				fnKey("Setup"): {
					{Site: fnKey("Setup"), Callee: "m/api.MountUsers", CalleeKey: users},
				},
				users: {get},
			},
			want:       []Route{{Method: "GET", Path: "/u", Site: users}},
			wantInline: 1,
			wantUnmet:  1,
		},
		{
			name:       "no mount patterns find no mount",
			cfg:        Config{Handle: cfg.Handle},
			decls:      []entities.Decl{declAt(first, 10), declAt(users, 20)},
			calls:      map[entities.DeclKey][]facts.Call{first: {mount}, users: {get}},
			want:       []Route{{Method: "GET", Path: "/u", Site: users}},
			wantInline: 1,
		},
	}

	requireBuild(t, testCases)
}

func TestSourceOrder(t *testing.T) {
	t.Parallel()

	later := entities.DeclKey{File: "b/b.go", Kind: entities.KindFunc, Name: "B"}
	earlier := entities.DeclKey{File: "a/a.go", Kind: entities.KindFunc, Name: "A"}
	decls := []entities.Decl{declAt(later, 1), declAt(earlier, 9), declAt(fnKey("Same"), 4)}

	testCases := []struct {
		name string
		want []entities.DeclKey
	}{
		{
			name: "the file leads the span start",
			want: []entities.DeclKey{earlier, fnKey("Same"), later},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := make([]entities.DeclKey, 0, len(decls))
			for _, site := range sourceOrder(Config{}, decls) {
				got = append(got, site.Key)
			}

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("sourceOrder() = %v, want %v", got, testCase.want)
			}
		})
	}
}
