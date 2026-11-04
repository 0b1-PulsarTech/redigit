package di

import (
	"reflect"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

func TestBuildModelSharesOnlyWhatBootCodeRuns(t *testing.T) {
	t.Parallel()

	boot, newA, dig, newD := fnKey("Boot"), fnKey("NewA"), fnKey("Dig"), fnKey("NewD")
	run := fnKey("run")
	injected := Config{
		RegisterFuncs: []string{"Register"}, ResolveFuncs: []string{"Get"},
		InjectorTypes: []string{"Injector"},
	}
	regA, regLit := register(boot, "Register", "p.A", newA), register(boot, "Register", "p.C")
	regViaD := register(boot, "Register", "p.C", newD)
	pull, pullOfDig := resolve{site: newA, typ: "p.B"}, resolve{site: dig, typ: "p.B"}
	closed := model{
		resolves: []resolve{pull},
		registrations: []registration{
			{site: boot, typ: "p.A", pulls: []resolve{pull}}, {site: boot, typ: "p.C"},
		},
	}
	ctors := map[entities.DeclKey][]facts.TypeRef{
		newA: {}, newD: {}, dig: {typeRef("*x/di.Injector")},
	}
	everywhere := func(entities.Decl) bool { return true }
	testCases := []modelCase{
		{
			name:  "a constructor called inside a literal handed to a registration is run by it",
			cfg:   injected,
			decls: []entities.Decl{declOf(boot), declOf(newA)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: {regA, regLit, {CalleeKey: newA, Within: &regLit}},
					newA: {get(newA, "Get", "p.B")},
				},
				ctors: ctors,
			},
			want: closed,
			seed: everywhere,
		},
		{
			name:  "a helper called inside such a literal is run by the container too",
			cfg:   injected,
			decls: []entities.Decl{declOf(boot), declOf(newA), declOf(dig)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: {regA, regLit, {CalleeKey: dig, Within: &regLit}},
					newA: {{Site: newA, Callee: "p.Dig", CalleeKey: dig}},
					dig:  {get(dig, "Get", "p.B")},
				},
				ctors: ctors,
			},
			want: model{
				resolves: []resolve{pullOfDig},
				registrations: []registration{
					{site: boot, typ: "p.A", pulls: []resolve{pullOfDig}}, {site: boot, typ: "p.C"},
				},
			},
			seed: everywhere,
		},
		{
			name:  "a helper only a constructor calls is not a seed without the injector",
			cfg:   injected,
			decls: []entities.Decl{declOf(boot), declOf(newA), declOf(newD), declOf(run)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: {regA, regViaD}, newA: {get(newA, "Get", "p.B")},
					newD: {{CalleeKey: run}}, run: {{CalleeKey: newA}},
				},
				ctors: ctors,
			},
			want: closed,
			seed: everywhere,
		},
	}

	requireModels(t, testCases)
}

func TestBuildModelSharesWhatBootCodeHandsOn(t *testing.T) {
	t.Parallel()

	boot, newA, cold := fnKey("Boot"), fnKey("NewA"), fnKey("Cold")
	invoke, hold := fnKey("invoke"), fnKey("do")
	injected := Config{
		RegisterFuncs: []string{"*Register"}, ResolveFuncs: []string{"Get"},
		InjectorTypes: []string{"Injector"},
	}
	regA := register(boot, "Register", "p.A", newA)
	doCall := facts.Call{Callee: "p.do", CalleeKey: hold}
	pull := resolve{site: newA, typ: "p.B"}
	open := model{
		resolves:      []resolve{pull},
		registrations: []registration{{site: boot, typ: "p.A", pulls: []resolve{pull}}},
		shared:        map[resolve]bool{pull: true},
	}
	ctors := map[entities.DeclKey][]facts.TypeRef{newA: {}}
	decls := []entities.Decl{declOf(boot), declOf(newA), declOf(cold)}
	everywhere := func(entities.Decl) bool { return true }
	testCases := []modelCase{
		{
			name:  "a constructor passed to a function that is no registration is run by hand",
			cfg:   injected,
			decls: decls,
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: {regA}, newA: {get(newA, "Get", "p.B")},
					cold: {{
						Callee: "p.invoke", CalleeKey: invoke, FuncArgs: []entities.DeclKey{newA},
					}},
				},
				ctors: ctors,
			},
			want: open,
			seed: everywhere,
		},
		{
			name:  "a constructor called in a literal handed to another call is run by hand",
			cfg:   injected,
			decls: decls,
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: {regA}, newA: {get(newA, "Get", "p.B")},
					cold: {doCall, {CalleeKey: newA, Within: &doCall}},
				},
				ctors: ctors,
			},
			want: open,
			seed: everywhere,
		},
		{
			name:  "a register name on a type that is no injector registers nothing",
			cfg:   injected,
			decls: decls,
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: {regA}, newA: {get(newA, "Get", "p.B")},
					cold: {methodOn(register(cold, "Register", "p.A", newA), "x/p.Cache")},
				},
				ctors: ctors,
			},
			want: open,
			seed: everywhere,
		},
	}

	requireModels(t, testCases)
}

func TestBuildModelSharesWhatABootHelperRuns(t *testing.T) {
	t.Parallel()

	boot, newA, cold, run, other := fnKey("Boot"), fnKey("NewA"), fnKey("Cold"), fnKey("run"),
		fnKey("other")
	injected := Config{
		RegisterFuncs: []string{"Register"}, ResolveFuncs: []string{"Get"},
		InjectorTypes: []string{"Injector"},
	}
	rooted := injected
	rooted.Roots = []string{"Cold"}
	viaCold := resolve{site: cold, typ: "p.B"}
	regA := register(boot, "Register", "p.A", newA)
	pull := resolve{site: newA, typ: "p.B"}
	registrations := []registration{{site: boot, typ: "p.A", pulls: []resolve{pull}}}
	ctors := map[entities.DeclKey][]facts.TypeRef{newA: {}}
	everywhere := func(entities.Decl) bool { return true }
	testCases := []modelCase{
		{
			name:  "a helper boot code calls shares the constructor it calls",
			cfg:   injected,
			decls: []entities.Decl{declOf(boot), declOf(newA), declOf(run), declOf(cold)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: {regA}, newA: {get(newA, "Get", "p.B")},
					cold: {{CalleeKey: run}}, run: {{CalleeKey: newA}},
				},
				ctors: ctors,
			},
			want: model{
				resolves: []resolve{pull}, registrations: registrations,
				shared: map[resolve]bool{pull: true},
			},
			seed: everywhere,
		},
		{
			name:  "a resolve in a helper no constructor runs is not marked as shared",
			cfg:   injected,
			decls: []entities.Decl{declOf(boot), declOf(newA), declOf(cold), declOf(other)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: {regA}, newA: {get(newA, "Get", "p.B")},
					cold: {{CalleeKey: other}}, other: {get(other, "Get", "p.B")},
				},
				ctors: ctors,
			},
			want: model{
				resolves:      []resolve{pull, {site: other, typ: "p.B"}},
				registrations: registrations,
			},
			seed: everywhere,
		},
		{
			name: "a2 a root a constructor calls is shared with the boot code that runs it",
			cfg:  rooted, seed: everywhere,
			decls: []entities.Decl{declOf(boot), declOf(newA), declOf(cold)},
			world: world{calls: map[entities.DeclKey][]facts.Call{
				boot: {regA}, newA: {{CalleeKey: cold}}, cold: {get(cold, "Get", "p.B")},
			}, ctors: map[entities.DeclKey][]facts.TypeRef{
				newA: {}, cold: {typeRef("*x/di.Injector")},
			}},
			want: model{
				resolves: []resolve{viaCold}, shared: map[resolve]bool{viaCold: true},
				registrations: []registration{{site: boot, typ: "p.A", pulls: []resolve{viaCold}}},
			},
		},
	}

	requireModels(t, testCases)
}

func TestRanByCtors(t *testing.T) {
	t.Parallel()

	boot, newA, middle, leaf, other := fnKey("Boot"), fnKey("NewA"), fnKey("middle"),
		fnKey("leaf"), fnKey("Other")
	known := world{calls: map[entities.DeclKey][]facts.Call{
		newA:   {{CalleeKey: middle}, {CalleeKey: entities.DeclKey{}}},
		middle: {{CalleeKey: leaf}, {CalleeKey: newA}},
		other:  {{CalleeKey: boot}},
	}}

	testCases := []struct {
		name   string
		owners map[entities.DeclKey][]int
		want   map[entities.DeclKey]bool
	}{
		{
			name:   "everything a constructor calls runs, through any middle",
			owners: map[entities.DeclKey][]int{newA: {0}},
			want:   map[entities.DeclKey]bool{newA: true, middle: true, leaf: true},
		},
		{
			name:   "a registration without a constructor runs nothing",
			owners: map[entities.DeclKey][]int{{}: {0}},
			want:   map[entities.DeclKey]bool{},
		},
		{name: "no registration runs nothing", want: map[entities.DeclKey]bool{}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := ranByCtors(testCase.owners, known.facts(gomock.NewController(t)))
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("ranByCtors() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestBuildModelSharesThroughCutAndPassedFunctions(t *testing.T) {
	t.Parallel()

	boot, newA, newD, run := fnKey("Boot"), fnKey("NewA"), fnKey("NewD"), fnKey("run")
	cold, big, help := fnKey("Cold"), fnKey("big"), fnKey("help")
	cutD, cutBig := stmtKey("NewD", "1"), stmtKey("big", "1")
	rooted, globbed := baseConfig(), baseConfig()
	rooted.Roots, globbed.Roots = []string{"Cold"}, []string{"New*"}
	regs := []facts.Call{
		register(boot, "Register", "p.A", newA), register(boot, "Register", "p.C", newD),
	}
	pull, pullA := resolve{site: newA, typ: "p.B"}, get(newA, "Get", "p.B")
	closed := model{resolves: []resolve{pull}, registrations: []registration{
		{site: boot, typ: "p.A", pulls: []resolve{pull}}, {site: boot, typ: "p.C"},
	}}
	open := closed
	open.shared = map[resolve]bool{pull: true}
	all := []entities.Decl{
		declOf(boot), declOf(newA), declOf(newD), declOf(run), declOf(help), declOf(cold),
		declOf(big), declOf(cutD), declOf(cutBig),
	}
	ctors := map[entities.DeclKey][]facts.TypeRef{newA: {}, newD: {}}
	toCold := map[entities.DeclKey][]facts.Call{
		boot: regs, newA: {pullA}, newD: {{CalleeKey: cold}}, cold: {{CalleeKey: newA}},
	}
	everywhere := func(entities.Decl) bool { return true }
	testCases := []modelCase{
		{
			name: "st1 a helper a cut constructor runs does not hand call what it calls",
			cfg:  baseConfig(), decls: all, want: closed, seed: everywhere,
			world: world{calls: map[entities.DeclKey][]facts.Call{
				boot: regs, newA: {pullA}, cutD: {{CalleeKey: run}}, run: {{CalleeKey: newA}},
			}, ctors: ctors},
		},
		{
			name: "st3b a cut helper below a constructor boot code calls hands on by hand",
			cfg:  baseConfig(), decls: all, want: open, seed: everywhere,
			world: world{calls: map[entities.DeclKey][]facts.Call{
				boot: regs, newA: {pullA}, cold: {{CalleeKey: newD}}, newD: {{CalleeKey: big}},
				cutBig: {{CalleeKey: newA}},
			}, ctors: ctors},
		},
		{
			name: "r1 a root a constructor calls still hand calls what it calls",
			cfg:  rooted, decls: all, want: open, seed: everywhere,
			world: world{calls: toCold, ctors: map[entities.DeclKey][]facts.TypeRef{
				newA: {}, newD: {}, cold: {typeRef("*x/di.Injector")},
			}},
		},
		{
			name: "r2 the same root without an injector parameter",
			cfg:  rooted, decls: all, want: open, seed: everywhere,
			world: world{calls: toCold, ctors: ctors},
		},
		{
			name: "a root glob that matches constructors does not seed them",
			cfg:  globbed, decls: all, want: closed, seed: everywhere,
			world: world{calls: map[entities.DeclKey][]facts.Call{
				boot: regs, newA: {pullA}, newD: {{CalleeKey: newA}},
			}, ctors: ctors},
		},
		{
			name: "f1 a func value a constructor hands to a helper runs in the constructor",
			cfg:  baseConfig(), decls: all, want: closed, seed: everywhere,
			world: world{calls: map[entities.DeclKey][]facts.Call{
				boot: regs, newA: {pullA}, help: {{CalleeKey: newA}},
				newD: {{CalleeKey: run, FuncArgs: []entities.DeclKey{help}}},
			}, ctors: ctors},
		},
	}

	requireModels(t, testCases)
}

func TestBuildModelPullsThroughCutAndPassedFunctions(t *testing.T) {
	t.Parallel()

	boot, newD, dig, help := fnKey("Boot"), fnKey("NewD"), fnKey("Dig"), fnKey("help")
	run, cutD := fnKey("run"), stmtKey("NewD", "1")
	newM := entities.DeclKey{File: "boot/boot.go", Kind: entities.KindMethod, Name: "Fac.NewM"}
	cutM := stmtKey("Fac.NewM", "1")
	injected := Config{
		RegisterFuncs: []string{"Register"}, ResolveFuncs: []string{"Get"},
		InjectorTypes: []string{"Injector"},
	}
	regs := []facts.Call{
		register(boot, "Register", "p.A"), register(boot, "Register", "p.C", newD),
	}
	viaDig, viaHelp := resolve{site: dig, typ: "p.B"}, resolve{site: help, typ: "p.B"}
	decls := []entities.Decl{
		declOf(boot), declOf(newD), declOf(cutD), declOf(dig), declOf(run), declOf(help),
	}
	ctors := map[entities.DeclKey][]facts.TypeRef{
		newD: {}, dig: {typeRef("*x/di.Injector")}, help: {typeRef("*x/di.Injector")},
	}
	testCases := []modelCase{
		{
			name:  "st2 a resolve in a helper a cut constructor calls is a need",
			cfg:   injected,
			decls: decls,
			world: world{calls: map[entities.DeclKey][]facts.Call{
				boot: regs, cutD: {{CalleeKey: dig}}, dig: {get(dig, "Get", "p.B")},
			}, ctors: ctors},
			want: model{resolves: []resolve{viaDig}, registrations: []registration{
				{site: boot, typ: "p.A"}, {site: boot, typ: "p.C", pulls: []resolve{viaDig}},
			}},
		},
		{
			name:  "st2m a resolve in a helper a cut method constructor calls is a need",
			cfg:   injected,
			decls: []entities.Decl{declOf(boot), declOf(newM), declOf(cutM), declOf(dig)},
			world: world{calls: map[entities.DeclKey][]facts.Call{
				boot: {register(boot, "Register", "p.C", newM)}, cutM: {{CalleeKey: dig}},
				dig: {get(dig, "Get", "p.B")},
			}, ctors: map[entities.DeclKey][]facts.TypeRef{
				newM: {}, dig: {typeRef("*x/di.Injector")},
			}},
			want: model{resolves: []resolve{viaDig}, registrations: []registration{
				{site: boot, typ: "p.C", pulls: []resolve{viaDig}},
			}},
		},
		{
			name:  "f3 a resolve in a func value a constructor hands on is a need",
			cfg:   injected,
			decls: decls,
			world: world{calls: map[entities.DeclKey][]facts.Call{
				boot: regs, newD: {{CalleeKey: run, FuncArgs: []entities.DeclKey{help}}},
				help: {get(help, "Get", "p.B")},
			}, ctors: ctors},
			want: model{resolves: []resolve{viaHelp}, registrations: []registration{
				{site: boot, typ: "p.A"}, {site: boot, typ: "p.C", pulls: []resolve{viaHelp}},
			}},
		},
		{
			name:  "rb1 a resolve in a register function a constructor calls is a need",
			cfg:   injected,
			decls: decls,
			world: world{calls: map[entities.DeclKey][]facts.Call{
				boot: regs,
				newD: {{Callee: "di.Register", CalleeKey: dig, FuncArgs: []entities.DeclKey{help}}},
				dig:  {get(dig, "Get", "p.B")},
			}, ctors: ctors},
			want: model{untyped: 1, resolves: []resolve{viaDig}, registrations: []registration{
				{site: boot, typ: "p.A"}, {site: boot, typ: "p.C", pulls: []resolve{viaDig}},
			}},
		},
	}

	requireModels(t, testCases)
}

func TestBuildModelSeedsOnlyTheHeadThatRan(t *testing.T) {
	t.Parallel()

	boot, newA, newD := fnKey("Boot"), fnKey("NewA"), fnKey("NewD")
	newM := entities.DeclKey{File: "boot/boot.go", Kind: entities.KindMethod, Name: "Fac.NewM"}
	cutD, cutM := stmtKey("NewD", "1"), stmtKey("Fac.NewM", "1")
	globbed, methoded := baseConfig(), baseConfig()
	globbed.Roots, methoded.Roots = []string{"New*"}, []string{"Fac.*"}
	regs := []facts.Call{
		register(boot, "Register", "p.A", newA), register(boot, "Register", "p.C", newD),
	}
	pull, pullA := resolve{site: newA, typ: "p.B"}, get(newA, "Get", "p.B")
	closed := model{resolves: []resolve{pull}, registrations: []registration{
		{site: boot, typ: "p.A", pulls: []resolve{pull}}, {site: boot, typ: "p.C"},
	}}
	ctors := map[entities.DeclKey][]facts.TypeRef{newA: {}, newD: {}, newM: {}}
	everywhere := func(entities.Decl) bool { return true }
	testCases := []modelCase{
		{
			name: "a1 a root glob that matches a cut constructor does not seed its parts",
			cfg:  globbed, seed: everywhere,
			decls: []entities.Decl{declOf(boot), declOf(newA), declOf(newD), declOf(cutD)},
			world: world{calls: map[entities.DeclKey][]facts.Call{
				boot: regs, newA: {pullA}, cutD: {{CalleeKey: newA}},
			}, ctors: ctors},
			want: closed,
		},
		{
			name: "a1m the same under a root glob that matches a cut method constructor",
			cfg:  methoded, seed: everywhere,
			decls: []entities.Decl{declOf(boot), declOf(newA), declOf(newM), declOf(cutM)},
			world: world{calls: map[entities.DeclKey][]facts.Call{
				boot: {regs[0], register(boot, "Register", "p.C", newM)},
				newA: {pullA}, cutM: {{CalleeKey: newA}},
			}, ctors: ctors},
			want: closed,
		},
	}

	requireModels(t, testCases)
}
