package di

import (
	"errors"
	"path"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

var errNoFunc = errors.New("no function")

func fnKey(name string) entities.DeclKey {
	return entities.DeclKey{File: "boot/boot.go", Kind: entities.KindFunc, Name: name}
}

func typeKey(name string) entities.DeclKey {
	return entities.DeclKey{File: "p/p.go", Kind: entities.KindType, Name: name}
}

func stmtKey(name string, at string) entities.DeclKey {
	return entities.DeclKey{File: "boot/boot.go", Kind: entities.KindStmt, Name: name + "#" + at}
}

func declOf(key entities.DeclKey) entities.Decl { return entities.Decl{Key: key} }

func typeRef(name string) facts.TypeRef { return facts.TypeRef{Name: name} }

func register(site entities.DeclKey, fn string, bound string, ctor ...entities.DeclKey) facts.Call {
	call := facts.Call{Site: site, Callee: "di." + fn, FuncArgs: ctor}
	if bound != "" {
		call.TypeArgs = []facts.TypeRef{typeRef(bound)}
	}

	return call
}

func get(site entities.DeclKey, fn string, bound string) facts.Call {
	return register(site, fn, bound)
}

func methodOn(call facts.Call, receiver string) facts.Call {
	call.Receiver = typeRef(receiver)
	call.Callee = receiver + strings.TrimPrefix(call.Callee, "di")

	return call
}

type world struct {
	calls map[entities.DeclKey][]facts.Call
	ctors map[entities.DeclKey][]facts.TypeRef
	impls map[entities.DeclKey][]entities.DeclKey
}

func keyed(call facts.Call, key entities.DeclKey) facts.Call {
	call.TypeArgs = []facts.TypeRef{{Name: call.TypeArgs[0].Name, Key: key}}

	return call
}

func baseConfig() Config {
	return Config{RegisterFuncs: []string{"Register*"}, ResolveFuncs: []string{"Get", "MustGet"}}
}

type modelCase struct {
	name  string
	cfg   Config
	decls []entities.Decl
	world world
	want  model
	seed  func(entities.Decl) bool
}

func TestBuildModelRegistrations(t *testing.T) {
	t.Parallel()

	site, ctor, method := fnKey("Boot"), fnKey("NewRepo"), fnKey("Svc.Build")
	testCases := []modelCase{
		{
			name:  "a registration through a type argument",
			cfg:   baseConfig(),
			decls: []entities.Decl{declOf(site)},
			world: world{calls: map[entities.DeclKey][]facts.Call{
				site: {register(site, "Register", "p.Repo")},
			}},
			want: model{registrations: []registration{{site: site, typ: "p.Repo"}}},
		},
		{
			name:  "a registration through a constructor reads its argument types",
			cfg:   baseConfig(),
			decls: []entities.Decl{declOf(site)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					site: {register(site, "RegisterConstructor", "p.Repo", ctor)},
				},
				ctors: map[entities.DeclKey][]facts.TypeRef{
					ctor: {typeRef("p.DB"), typeRef("p.Cfg")},
				},
			},
			want: model{registrations: []registration{
				{site: site, typ: "p.Repo", needs: refs("p.DB", "p.Cfg")},
			}},
		},
		{
			name:  "a method value constructor reads its argument types",
			cfg:   baseConfig(),
			decls: []entities.Decl{declOf(site)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					site: {register(site, "RegisterConstructor", "p.Repo", method)},
				},
				ctors: map[entities.DeclKey][]facts.TypeRef{method: {typeRef("p.DB")}},
			},
			want: model{registrations: []registration{
				{site: site, typ: "p.Repo", needs: refs("p.DB")},
			}},
		},
		{
			name:  "a generic constructor binds the instantiated type",
			cfg:   baseConfig(),
			decls: []entities.Decl{declOf(site)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					site: {register(site, "RegisterConstructor", "p.Repo[int]", ctor)},
				},
				ctors: map[entities.DeclKey][]facts.TypeRef{ctor: {typeRef("p.Box[int]")}},
			},
			want: model{registrations: []registration{
				{site: site, typ: "p.Repo[int]", needs: refs("p.Box[int]")},
			}},
		},
	}

	requireModels(t, testCases)
}

func TestBuildModelBlindSpots(t *testing.T) {
	t.Parallel()

	site, ctor := fnKey("Boot"), fnKey("NewRepo")
	testCases := []modelCase{
		{
			name:  "a slice argument needs the slice, not its element",
			cfg:   baseConfig(),
			decls: []entities.Decl{declOf(site)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					site: {register(site, "RegisterConstructor", "p.Router", ctor)},
				},
				ctors: map[entities.DeclKey][]facts.TypeRef{
					ctor: {{Key: fnKey("Handler"), Name: "[]p.Handler"}},
				},
			},
			want: model{registrations: []registration{
				{
					site: site, typ: "p.Router",
					needs: []facts.TypeRef{{Key: fnKey("Handler"), Name: "[]p.Handler"}},
				},
			}},
		},
		{
			name:  "a constructor outside the loaded packages is counted, not guessed",
			cfg:   baseConfig(),
			decls: []entities.Decl{declOf(site)},
			world: world{calls: map[entities.DeclKey][]facts.Call{
				site: {register(site, "RegisterConstructor", "p.Repo", ctor)},
			}},
			want: model{
				registrations: []registration{{site: site, typ: "p.Repo"}},
				opaque:        1,
			},
		},
		{
			name:  "a registration with no static type is counted, not guessed",
			cfg:   baseConfig(),
			decls: []entities.Decl{declOf(site)},
			world: world{calls: map[entities.DeclKey][]facts.Call{
				site: {register(site, "Register", "")},
			}},
			want: model{untyped: 1},
		},
	}

	requireModels(t, testCases)
}

func TestBuildModelSites(t *testing.T) {
	t.Parallel()

	site, other, test := fnKey("Boot"), fnKey("Other"), fnKey("TestBoot")
	test.File = "boot/boot_test.go"
	other.File = "tools/other.go"
	first, second := stmtKey("Run", "1"), stmtKey("Run", "2")
	testCases := []modelCase{
		{
			name:  "resolution through Get and MustGet",
			cfg:   baseConfig(),
			decls: []entities.Decl{declOf(site)},
			world: world{calls: map[entities.DeclKey][]facts.Call{
				site: {
					get(site, "Get", "p.Repo"),
					get(site, "MustGet", "p.DB"),
					get(site, "Find", "p.X"),
				},
			}},
			want: model{resolves: []resolve{
				{site: site, typ: "p.Repo"}, {site: site, typ: "p.DB"},
			}},
		},
		{
			name:  "a resolve with no static type is counted",
			cfg:   baseConfig(),
			decls: []entities.Decl{declOf(site)},
			world: world{calls: map[entities.DeclKey][]facts.Call{
				site: {get(site, "Get", "")},
			}},
			want: model{untyped: 1},
		},
		{
			name: "the injector filter keeps matching receivers and plain functions",
			cfg: Config{
				RegisterFuncs: []string{
					"Register*",
					"*.Register*",
				},
				InjectorTypes: []string{"Injector"},
			},
			decls: []entities.Decl{declOf(site)},
			world: world{calls: map[entities.DeclKey][]facts.Call{
				site: {
					methodOn(register(site, "RegisterA", "p.A"), "x/di.Injector"),
					methodOn(register(site, "RegisterB", "p.B"), "x/di.Other"),
					register(site, "RegisterC", "p.C"),
				},
			}},
			want: model{registrations: []registration{{site: site, typ: "p.C"}}, untyped: 1},
		},
		{
			name:  "statements are sites of their own",
			cfg:   baseConfig(),
			decls: []entities.Decl{declOf(first), declOf(second)},
			world: world{calls: map[entities.DeclKey][]facts.Call{
				first:  {register(first, "Register", "p.A")},
				second: {get(second, "Get", "p.A")},
			}},
			want: model{
				registrations: []registration{{site: first, typ: "p.A"}},
				resolves:      []resolve{{site: second, typ: "p.A"}},
			},
		},
		{
			name: "test code and files outside the paths are not read",
			cfg: Config{
				RegisterFuncs: []string{"Register*"}, Paths: []string{"boot/"},
			},
			decls: []entities.Decl{declOf(site), declOf(test), declOf(other)},
			world: world{calls: map[entities.DeclKey][]facts.Call{
				site:  {register(site, "Register", "p.A")},
				test:  {register(test, "Register", "p.T")},
				other: {register(other, "Register", "p.O")},
			}},
			want: model{registrations: []registration{{site: site, typ: "p.A"}}},
		},
	}

	requireModels(t, testCases)
}

func requireModels(t *testing.T, testCases []modelCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := buildSeeded(
				testCase.cfg, testCase.decls, testCase.world.facts(gomock.NewController(t)),
				testCase.seed,
			)
			if err != nil {
				t.Fatalf("buildSeeded() = %v", err)
			}

			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("buildSeeded() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestBuildModelRefusesAPatternItCannotMatch(t *testing.T) {
	t.Parallel()

	site := stmtKey("Boot", "1")

	testCases := []struct {
		name string
		cfg  Config
	}{
		{name: "a register pattern", cfg: Config{RegisterFuncs: []string{"Reg[ister"}}},
		{
			name: "a resolve pattern",
			cfg:  Config{RegisterFuncs: []string{"Nothing"}, ResolveFuncs: []string{"Ge[t"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			mock := world{calls: map[entities.DeclKey][]facts.Call{
				site: {get(site, "Get", "p.Repo")},
			}}.facts(gomock.NewController(t))

			_, err := buildModel(testCase.cfg, []entities.Decl{declOf(site)}, mock)
			if !errors.Is(err, path.ErrBadPattern) || !strings.Contains(err.Error(), "calls of") {
				t.Fatalf("buildModel() err = %v, want a bad pattern naming the site", err)
			}
		})
	}
}

func TestModelUnbound(t *testing.T) {
	t.Parallel()

	first, second, third := fnKey("A"), fnKey("B"), fnKey("C")

	testCases := []struct {
		name  string
		input model
		want  []string
	}{
		{
			name: "needs and resolves nothing serves are named once",
			input: model{
				registrations: []registration{
					{site: first, typ: "p.Repo"},
					{site: second, typ: "p.Repo", needs: refs("p.DB", "p.Cfg")},
					{site: third, typ: "p.DB"},
				},
				resolves: []resolve{{site: first, typ: "p.Gone"}, {site: second, typ: "p.Cfg"}},
			},
			want: []string{"p.Cfg", "p.Gone"},
		},
		{
			name:  "a model with no register site leaves every resolve unbound",
			input: model{resolves: []resolve{{site: first, typ: "p.Repo"}}},
			want:  []string{"p.Repo"},
		},
		{
			name: "a pull nothing serves is unbound",
			input: model{registrations: []registration{
				{site: first, typ: "p.Repo", pulls: []resolve{{site: second, typ: "p.Gone"}}},
			}},
			want: []string{"p.Gone"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.input.unbound(); !slices.Equal(got, testCase.want) {
				t.Fatalf("unbound() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestBuildModelUnbound(t *testing.T) {
	t.Parallel()

	boot, newSvc, newPg := stmtKey("Boot", "1"), fnKey("NewSvc"), fnKey("NewPg")
	pg, iface := typeKey("Pg"), typeKey("Store")
	store := facts.TypeRef{Key: iface, Name: "p.Store"}
	registers := []facts.Call{
		register(boot, "Register", "p.Svc", newSvc),
		keyed(register(boot, "Register", "p.Pg", newPg), pg),
	}

	testCases := []struct {
		name  string
		world world
		want  []string
	}{
		{
			name: "a constructor argument of an interface type is served by its implementor",
			world: world{
				calls: map[entities.DeclKey][]facts.Call{boot: registers},
				ctors: map[entities.DeclKey][]facts.TypeRef{newSvc: {store}, newPg: {}},
				impls: map[entities.DeclKey][]entities.DeclKey{iface: {pg}},
			},
		},
		{
			name: "a constructor argument of an interface nothing implements is unbound",
			world: world{
				calls: map[entities.DeclKey][]facts.Call{boot: registers},
				ctors: map[entities.DeclKey][]facts.TypeRef{newSvc: {store}, newPg: {}},
			},
			want: []string{"p.Store"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			built, err := buildModel(
				baseConfig(), []entities.Decl{declOf(boot)},
				testCase.world.facts(gomock.NewController(t)),
			)
			if err != nil {
				t.Fatalf("buildSeeded() = %v", err)
			}

			if got := built.unbound(); !slices.Equal(got, testCase.want) {
				t.Fatalf("unbound() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func refs(names ...string) []facts.TypeRef {
	out := make([]facts.TypeRef, 0, len(names))
	for _, name := range names {
		out = append(out, typeRef(name))
	}

	return out
}

func (set world) facts(ctrl *gomock.Controller) *MockFacts {
	mock := NewMockFacts(ctrl)
	mock.EXPECT().Calls(gomock.Any()).DoAndReturn(
		func(site entities.DeclKey) []facts.Call { return set.calls[site] },
	).AnyTimes()
	mock.EXPECT().CtorArgTypes(gomock.Any()).DoAndReturn(
		func(fn entities.DeclKey) ([]facts.TypeRef, error) {
			args, found := set.ctors[fn]
			if !found {
				return nil, errNoFunc
			}

			return args, nil
		},
	).AnyTimes()
	mock.EXPECT().Implementors(gomock.Any()).DoAndReturn(
		func(iface entities.DeclKey) []entities.DeclKey { return set.impls[iface] },
	).AnyTimes()

	return mock
}
