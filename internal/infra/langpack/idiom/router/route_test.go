package router

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

type routeCase struct {
	name string
	call facts.Call
	want Route
}

func requireRoutes(t *testing.T, testCases []routeCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := routeOf(testCase.call); got != testCase.want {
				t.Fatalf("routeOf() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestRouteOf(t *testing.T) {
	t.Parallel()

	site, handler := stmtKey("Setup", "1"), fnKey("List")
	router := facts.TypeRef{Name: "m/router.Router"}

	testCases := []routeCase{
		{
			name: "a GET with a literal path",
			call: facts.Call{
				Site: site, Callee: "m/router.Router.GET", Receiver: router,
				Literals: []string{"/users"}, FuncArgs: []entities.DeclKey{handler},
			},
			want: Route{Method: "GET", Path: "/users", Handler: handler, Site: site},
		},
		{
			name: "a POST with a literal path",
			call: facts.Call{
				Site: site, Callee: "m/router.Router.POST", Receiver: router,
				Literals: []string{"/users"}, FuncArgs: []entities.DeclKey{handler},
			},
			want: Route{Method: "POST", Path: "/users", Handler: handler, Site: site},
		},
		{
			name: "a method named in camel case reads upper case",
			call: facts.Call{
				Site: site, Callee: "m/router.Router.Get", Receiver: router,
				Literals: []string{"/users"}, FuncArgs: []entities.DeclKey{handler},
			},
			want: Route{Method: "GET", Path: "/users", Handler: handler, Site: site},
		},
		{
			name: "a plain function names its verb",
			call: facts.Call{
				Site: site, Callee: "m/router.DELETE",
				Literals: []string{"/users"}, FuncArgs: []entities.DeclKey{handler},
			},
			want: Route{Method: "DELETE", Path: "/users", Handler: handler, Site: site},
		},
		{
			name: "a pattern that carries its verb",
			call: facts.Call{
				Site: site, Callee: "net/http.ServeMux.HandleFunc",
				Receiver: facts.TypeRef{Name: "net/http.ServeMux"},
				Literals: []string{"PUT /items/{id}"}, FuncArgs: []entities.DeclKey{handler},
			},
			want: Route{Method: "PUT", Path: "/items/{id}", Handler: handler, Site: site},
		},
		{
			name: "a path that is no constant is blind",
			call: facts.Call{
				Site: site, Callee: "m/router.Router.GET", Receiver: router,
				FuncArgs: []entities.DeclKey{handler},
			},
			want: Route{Method: "GET", Handler: handler, Site: site},
		},
	}

	requireRoutes(t, testCases)
}

func TestRouteOfArguments(t *testing.T) {
	t.Parallel()

	site, handler, guard := stmtKey("Setup", "1"), fnKey("List"), fnKey("Auth")
	method := entities.DeclKey{File: "svc.go", Kind: entities.KindMethod, Name: "Service.List"}
	router := facts.TypeRef{Name: "m/router.Router"}

	testCases := []routeCase{
		{
			name: "a literal that is no path is not the path",
			call: facts.Call{
				Site: site, Callee: "m/router.Router.GET", Receiver: router,
				Literals: []string{"users"}, FuncArgs: []entities.DeclKey{handler},
			},
			want: Route{Method: "GET", Handler: handler, Site: site},
		},
		{
			name: "the first literal that is a path wins",
			call: facts.Call{
				Site: site, Callee: "m/router.Router.GET", Receiver: router,
				Literals: []string{"tag", "/a", "/b"}, FuncArgs: []entities.DeclKey{handler},
			},
			want: Route{Method: "GET", Path: "/a", Handler: handler, Site: site},
		},
		{
			name: "a sentence with a slash is not a verb and a path",
			call: facts.Call{
				Site: site, Callee: "m/router.Router.GET", Receiver: router,
				Literals: []string{"see /docs"}, FuncArgs: []entities.DeclKey{handler},
			},
			want: Route{Method: "GET", Handler: handler, Site: site},
		},
		{
			name: "a method value is the handler",
			call: facts.Call{
				Site: site, Callee: "m/router.Router.GET", Receiver: router,
				Literals: []string{"/users"}, FuncArgs: []entities.DeclKey{method},
			},
			want: Route{Method: "GET", Path: "/users", Handler: method, Site: site},
		},
		{
			name: "the last func argument is the handler and the earlier ones are middleware",
			call: facts.Call{
				Site: site, Callee: "m/router.Router.GET", Receiver: router,
				Literals: []string{"/users"}, FuncArgs: []entities.DeclKey{guard, handler},
			},
			want: Route{Method: "GET", Path: "/users", Handler: handler, Site: site},
		},
		{
			name: "a handler that is no named func has none",
			call: facts.Call{
				Site: site, Callee: "m/router.Router.GET", Receiver: router,
				Literals: []string{"/users"},
			},
			want: Route{Method: "GET", Path: "/users", Site: site},
		},
	}

	requireRoutes(t, testCases)
}

func TestPrefixOf(t *testing.T) {
	t.Parallel()

	cfg := Config{Handle: []string{"*.GET"}, Mount: []string{"*.Route"}}
	group := facts.Call{
		Callee:   "m/router.Router.Route",
		Receiver: facts.TypeRef{Name: "m/router.Router"},
	}
	named := group
	named.Literals = []string{"/api"}
	relative := group
	relative.Literals = []string{"api"}

	testCases := []struct {
		name   string
		within *facts.Call
		want   string
	}{
		{name: "no enclosing call", want: ""},
		{name: "an enclosing mount call with a literal", within: &named, want: "/api"},
		{
			name:   "an enclosing mount call with no literal",
			within: &group,
			want:   "",
		},
		{
			name:   "an enclosing mount call whose literal is no path",
			within: &relative,
			want:   "",
		},
		{
			name: "an enclosing call that is no mount",
			within: &facts.Call{
				Callee: "m/router.Router.Use", Receiver: facts.TypeRef{Name: "m/router.Router"},
				Literals: []string{"/api"},
			},
			want: "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			call := facts.Call{Within: testCase.within}
			if got := prefixOf(cfg, call); got != testCase.want {
				t.Fatalf("prefixOf() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestJoinPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		prefix string
		route  string
		want   string
	}{
		{name: "no prefix keeps the route", route: "/x", want: "/x"},
		{name: "a prefix leads the route", prefix: "/api", route: "/x", want: "/api/x"},
		{
			name:   "a trailing slash of the prefix is not doubled",
			prefix: "/api/",
			route:  "/x",
			want:   "/api/x",
		},
		{name: "a blind route stays blind under a prefix", prefix: "/api", want: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := joinPath(testCase.prefix, testCase.route); got != testCase.want {
				t.Fatalf("joinPath() = %q, want %q", got, testCase.want)
			}
		})
	}
}
