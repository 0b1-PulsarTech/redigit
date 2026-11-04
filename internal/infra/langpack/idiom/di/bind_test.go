package di

import (
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

func TestBuildModelBinding(t *testing.T) {
	t.Parallel()

	site, ctor, wrap := fnKey("Boot"), fnKey("NewRepo"), fnKey("Wrap")
	testCases := []modelCase{
		{
			name: "the type arguments of a generic receiver are not the bound type",
			cfg: Config{
				RegisterFuncs: []string{"*.Register*"}, InjectorTypes: []string{"Registry"},
			},
			decls: []entities.Decl{declOf(site)},
			world: world{calls: map[entities.DeclKey][]facts.Call{
				site: {methodOn(register(site, "Register", "string"), "x/di.Registry")},
			}},
			want: model{untyped: 1},
		},
		{
			name: "the injector is never a need of the constructor it is passed to",
			cfg: Config{
				RegisterFuncs: []string{"Register*"}, InjectorTypes: []string{"Injector"},
			},
			decls: []entities.Decl{declOf(site)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					site: {register(site, "RegisterConstructor", "p.Repo", ctor)},
				},
				ctors: map[entities.DeclKey][]facts.TypeRef{
					ctor: {typeRef("*x/di.Injector"), typeRef("p.DB"), typeRef("x/di.Injector")},
				},
			},
			want: model{registrations: []registration{
				{site: site, typ: "p.Repo", needs: refs("p.DB")},
			}},
		},
		{
			name:  "a wrapper ahead of the constructor is not the constructor",
			cfg:   baseConfig(),
			decls: []entities.Decl{declOf(site)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					site: {register(site, "RegisterConstructor", "p.Repo", wrap, ctor)},
				},
				ctors: map[entities.DeclKey][]facts.TypeRef{
					wrap: {typeRef("p.Opt")},
					ctor: {typeRef("p.DB")},
				},
			},
			want: model{registrations: []registration{
				{site: site, typ: "p.Repo", needs: refs("p.DB")},
			}},
		},
		{
			name:  "a trailing function the packages do not hold is skipped",
			cfg:   baseConfig(),
			decls: []entities.Decl{declOf(site)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					site: {register(site, "RegisterConstructor", "p.Repo", ctor, fnKey("Outside"))},
				},
				ctors: map[entities.DeclKey][]facts.TypeRef{ctor: {typeRef("p.DB")}},
			},
			want: model{registrations: []registration{
				{site: site, typ: "p.Repo", needs: refs("p.DB")},
			}},
		},
	}

	requireModels(t, testCases)
}

func TestBuildModelInterfaces(t *testing.T) {
	t.Parallel()

	site, iface, pg := fnKey("Boot"), typeKey("Store"), typeKey("Pg")
	impls := map[entities.DeclKey][]entities.DeclKey{iface: {pg}}
	testCases := []modelCase{
		{
			name:  "an interface resolve with no registration asks for its implementors",
			cfg:   baseConfig(),
			decls: []entities.Decl{declOf(site)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{site: {
					keyed(register(site, "Register", "p.Pg"), pg),
					keyed(get(site, "Get", "p.Store"), iface),
				}},
				impls: impls,
			},
			want: model{
				registrations: []registration{{site: site, typ: "p.Pg", key: pg}},
				resolves:      []resolve{{site: site, typ: "p.Store", key: iface}},
				impls:         impls,
			},
		},
		{
			name:  "GetAll and its variants resolve every implementor",
			cfg:   Config{RegisterFuncs: []string{"Register"}, ResolveFuncs: []string{"*Get*"}},
			decls: []entities.Decl{declOf(site)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{site: {
					keyed(get(site, "GetAll", "p.Store"), iface),
					keyed(get(site, "MustGetAllWithContext", "p.Store"), iface),
					keyed(get(site, "GetWithContext", "p.Store"), iface),
				}},
				impls: impls,
			},
			want: model{
				resolves: []resolve{
					{site: site, typ: "p.Store", key: iface, all: true},
					{site: site, typ: "p.Store", key: iface, all: true},
					{site: site, typ: "p.Store", key: iface},
				},
				impls: impls,
			},
		},
		{
			name:  "a type that is registered itself asks for nothing",
			cfg:   baseConfig(),
			decls: []entities.Decl{declOf(site)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{site: {
					keyed(register(site, "Register", "p.Store"), iface),
					keyed(get(site, "Get", "p.Store"), iface),
				}},
				impls: impls,
			},
			want: model{
				registrations: []registration{{site: site, typ: "p.Store", key: iface}},
				resolves:      []resolve{{site: site, typ: "p.Store", key: iface}},
			},
		},
		{
			name:  "a slice or generic instance of an interface carries no key",
			cfg:   baseConfig(),
			decls: []entities.Decl{declOf(site)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{site: {
					keyed(get(site, "Get", "[]p.Store"), iface),
					keyed(get(site, "Get", "p.Store[int]"), iface),
				}},
				impls: impls,
			},
			want: model{resolves: []resolve{
				{site: site, typ: "[]p.Store"}, {site: site, typ: "p.Store[int]"},
			}},
		},
	}

	requireModels(t, testCases)
}
