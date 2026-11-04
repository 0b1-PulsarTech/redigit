package di

import (
	"reflect"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

func TestBuildModelFoldsConstructorResolves(t *testing.T) {
	t.Parallel()

	boot, newA, dig := fnKey("Boot"), fnKey("NewA"), fnKey("Dig")
	inside := stmtKey("NewA", "1")
	viaDig := facts.Call{Site: newA, Callee: "p.Dig", CalleeKey: dig}
	injected := Config{
		RegisterFuncs: []string{"Register"}, ResolveFuncs: []string{"Get"},
		InjectorTypes: []string{"Injector"},
	}
	registers := []facts.Call{
		register(boot, "Register", "p.A", newA), register(boot, "Register", "p.B"),
	}
	testCases := []modelCase{
		{
			name:  "a resolve inside the registered constructor is a need and stays a resolve",
			cfg:   injected,
			decls: []entities.Decl{declOf(boot), declOf(newA)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: registers, newA: {get(newA, "Get", "p.B")},
				},
				ctors: map[entities.DeclKey][]facts.TypeRef{newA: {}},
			},
			want: model{
				resolves: []resolve{{site: newA, typ: "p.B"}},
				registrations: []registration{
					{site: boot, typ: "p.A", pulls: []resolve{{site: newA, typ: "p.B"}}},
					{site: boot, typ: "p.B"},
				},
			},
		},
		{
			name:  "a resolve in a statement cut from the constructor is a need as well",
			cfg:   injected,
			decls: []entities.Decl{declOf(boot), declOf(inside)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: registers, inside: {get(inside, "Get", "p.B")},
				},
				ctors: map[entities.DeclKey][]facts.TypeRef{newA: {}},
			},
			want: model{
				resolves: []resolve{{site: inside, typ: "p.B"}},
				registrations: []registration{
					{site: boot, typ: "p.A", pulls: []resolve{{site: inside, typ: "p.B"}}},
					{site: boot, typ: "p.B"},
				},
			},
		},
		{
			name:  "a resolve in a helper that takes the injector follows the constructor",
			cfg:   injected,
			decls: []entities.Decl{declOf(boot), declOf(newA), declOf(dig)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: registers, newA: {viaDig}, dig: {get(dig, "Get", "p.B")},
				},
				ctors: map[entities.DeclKey][]facts.TypeRef{
					newA: {}, dig: {typeRef("*x/di.Injector")},
				},
			},
			want: model{
				resolves: []resolve{{site: dig, typ: "p.B"}},
				registrations: []registration{
					{site: boot, typ: "p.A", pulls: []resolve{{site: dig, typ: "p.B"}}},
					{site: boot, typ: "p.B"},
				},
			},
		},
	}

	requireModels(t, testCases)
}

func TestBuildModelLeavesResolvesOutsideConstructors(t *testing.T) {
	t.Parallel()

	boot, newA, dig := fnKey("Boot"), fnKey("NewA"), fnKey("Dig")
	viaDig := facts.Call{Site: newA, Callee: "p.Dig", CalleeKey: dig}
	injected := Config{
		RegisterFuncs: []string{"Register"}, ResolveFuncs: []string{"Get"},
		InjectorTypes: []string{"Injector"},
	}
	registers := []facts.Call{
		register(boot, "Register", "p.A", newA), register(boot, "Register", "p.B"),
	}
	testCases := []modelCase{
		{
			name:  "a resolve in a helper without the injector stays where it is",
			cfg:   injected,
			decls: []entities.Decl{declOf(boot), declOf(newA), declOf(dig)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: registers, newA: {viaDig}, dig: {get(dig, "Get", "p.B")},
				},
				ctors: map[entities.DeclKey][]facts.TypeRef{newA: {}, dig: {typeRef("p.Cfg")}},
			},
			want: model{
				registrations: []registration{
					{site: boot, typ: "p.A"}, {site: boot, typ: "p.B"},
				},
				resolves: []resolve{{site: dig, typ: "p.B"}},
			},
		},
		{
			name:  "a resolve outside every constructor stays a resolve",
			cfg:   injected,
			decls: []entities.Decl{declOf(boot), declOf(newA)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: slices.Concat(registers, []facts.Call{get(boot, "Get", "p.A")}),
				},
				ctors: map[entities.DeclKey][]facts.TypeRef{newA: {}},
			},
			want: model{
				registrations: []registration{
					{site: boot, typ: "p.A"}, {site: boot, typ: "p.B"},
				},
				resolves: []resolve{{site: boot, typ: "p.A"}},
			},
		},
		{
			name: "a helper a constructor registers is not run by it", cfg: injected,
			decls: []entities.Decl{declOf(boot), declOf(newA), declOf(dig)},
			world: world{calls: map[entities.DeclKey][]facts.Call{
				boot: registers, newA: {register(newA, "Register", "p.C", dig)},
				dig: {get(dig, "Get", "p.B")},
			}, ctors: map[entities.DeclKey][]facts.TypeRef{newA: {}, dig: {typeRef("*x/di.Injector")}}},
			want: model{resolves: []resolve{{site: dig, typ: "p.B"}}, registrations: []registration{
				{site: boot, typ: "p.A"},
				{site: boot, typ: "p.B"},
				{site: newA, typ: "p.C", pulls: []resolve{{site: dig, typ: "p.B"}}},
			}},
		},
	}

	requireModels(t, testCases)
}

func TestBuildModelMarksHelpersThatOtherCodeRuns(t *testing.T) {
	t.Parallel()

	boot, newA, dig, warm := fnKey("Boot"), fnKey("NewA"), fnKey("Dig"), fnKey("Warm")
	viaDig := facts.Call{Site: newA, Callee: "p.Dig", CalleeKey: dig}
	injected := Config{
		RegisterFuncs: []string{"Register"}, ResolveFuncs: []string{"Get"},
		InjectorTypes: []string{"Injector"},
	}
	registers := []facts.Call{
		register(boot, "Register", "p.A", newA), register(boot, "Register", "p.B"),
	}
	pullOfDig := resolve{site: dig, typ: "p.B"}
	viaWarm := model{
		resolves: []resolve{pullOfDig},
		registrations: []registration{
			{site: boot, typ: "p.A", pulls: []resolve{pullOfDig}},
			{site: boot, typ: "p.B"},
		},
		shared: map[resolve]bool{pullOfDig: true},
	}
	ctors := map[entities.DeclKey][]facts.TypeRef{newA: {}, dig: {typeRef("*x/di.Injector")}}
	wired := []entities.Decl{declOf(boot), declOf(newA), declOf(dig), declOf(warm)}
	everywhere := func(entities.Decl) bool { return true }
	testCases := []modelCase{
		{
			name:  "a helper that other code calls as well is marked as shared",
			cfg:   injected,
			decls: wired,
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: registers, newA: {viaDig}, dig: {get(dig, "Get", "p.B")},
					warm: {{Site: warm, Callee: "p.Dig", CalleeKey: dig}},
				},
				ctors: ctors,
			},
			want: viaWarm,
			seed: everywhere,
		},
		{
			name:  "a helper passed on as a value is shared with its holder",
			cfg:   injected,
			decls: wired,
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: registers, newA: {viaDig}, dig: {get(dig, "Get", "p.B")},
					warm: {{Site: warm, FuncArgs: []entities.DeclKey{dig}}},
				},
				ctors: ctors,
			},
			want: viaWarm,
			seed: everywhere,
		},
	}

	requireModels(t, testCases)
}

func TestBuildModelMarksHelpersBelowASharedOne(t *testing.T) {
	t.Parallel()

	boot, newA, dig, warm := fnKey("Boot"), fnKey("NewA"), fnKey("Dig"), fnKey("Warm")
	deep := fnKey("Deep")
	viaDig := facts.Call{Site: newA, Callee: "p.Dig", CalleeKey: dig}
	injected := Config{
		RegisterFuncs: []string{"Register"}, ResolveFuncs: []string{"Get"},
		InjectorTypes: []string{"Injector"},
	}
	registers := []facts.Call{
		register(boot, "Register", "p.A", newA), register(boot, "Register", "p.B"),
	}
	pullOfDig := resolve{site: dig, typ: "p.B"}
	alone := model{
		resolves: []resolve{pullOfDig},
		registrations: []registration{
			{site: boot, typ: "p.A", pulls: []resolve{pullOfDig}},
			{site: boot, typ: "p.B"},
		},
	}
	ctors := map[entities.DeclKey][]facts.TypeRef{newA: {}, dig: {typeRef("*x/di.Injector")}}
	wired := []entities.Decl{declOf(boot), declOf(newA), declOf(dig), declOf(warm)}
	everywhere := func(entities.Decl) bool { return true }
	offWarm := func(decl entities.Decl) bool { return decl.Key != warm }
	testCases := []modelCase{
		{
			name:  "a helper below a shared helper is shared as well",
			cfg:   injected,
			decls: append(slices.Clone(wired), declOf(deep)),
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: registers, newA: {viaDig}, warm: {{Site: warm, CalleeKey: dig}},
					dig:  {{Site: dig, Callee: "p.Deep", CalleeKey: deep}},
					deep: {get(deep, "Get", "p.B")},
				},
				ctors: map[entities.DeclKey][]facts.TypeRef{
					newA: {}, dig: {typeRef("*x/di.Injector")}, deep: {typeRef("*x/di.Injector")},
				},
			},
			want: model{
				resolves: []resolve{{site: deep, typ: "p.B"}},
				registrations: []registration{
					{site: boot, typ: "p.A", pulls: []resolve{{site: deep, typ: "p.B"}}},
					{site: boot, typ: "p.B"},
				},
				shared: map[resolve]bool{{site: deep, typ: "p.B"}: true},
			},
			seed: everywhere,
		},
		{
			name:  "a caller off the roots does not share the helper",
			cfg:   injected,
			decls: wired,
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: registers, newA: {viaDig}, dig: {get(dig, "Get", "p.B")},
					warm: {{Site: warm, Callee: "p.Dig", CalleeKey: dig}},
				},
				ctors: ctors,
			},
			want: alone,
			seed: offWarm,
		},
	}

	requireModels(t, testCases)
}

func TestBuildModelMarksConstructorsCalledByHand(t *testing.T) {
	t.Parallel()

	boot, newA, dig, warm := fnKey("Boot"), fnKey("NewA"), fnKey("Dig"), fnKey("Warm")
	viaDig := facts.Call{Site: newA, Callee: "p.Dig", CalleeKey: dig}
	injected := Config{
		RegisterFuncs: []string{"Register"}, ResolveFuncs: []string{"Get"},
		InjectorTypes: []string{"Injector"},
	}
	registers := []facts.Call{
		register(boot, "Register", "p.A", newA), register(boot, "Register", "p.B"),
	}
	pullOfDig := resolve{site: dig, typ: "p.B"}
	ctors := map[entities.DeclKey][]facts.TypeRef{newA: {}, dig: {typeRef("*x/di.Injector")}}
	wired := []entities.Decl{declOf(boot), declOf(newA), declOf(dig), declOf(warm)}
	everywhere := func(entities.Decl) bool { return true }
	testCases := []modelCase{
		{
			name:  "a constructor called by hand shares the helpers below it",
			cfg:   injected,
			decls: wired,
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: registers, newA: {viaDig}, dig: {get(dig, "Get", "p.B")},
					warm: {{Site: warm, Callee: "p.NewA", CalleeKey: newA}},
				},
				ctors: ctors,
			},
			want: model{
				resolves: []resolve{pullOfDig},
				registrations: []registration{
					{site: boot, typ: "p.A", pulls: []resolve{pullOfDig}},
					{site: boot, typ: "p.B"},
				},
				shared: map[resolve]bool{pullOfDig: true},
			},
			seed: everywhere,
		},
	}

	requireModels(t, testCases)
}

func TestBuildModelMarksStatementsCutFromHelpers(t *testing.T) {
	t.Parallel()

	boot, newA, dig, warm := fnKey("Boot"), fnKey("NewA"), fnKey("Dig"), fnKey("Warm")
	dug, slice := stmtKey("Dig", "1"), stmtKey("NewA", "1")
	viaDig := facts.Call{Site: newA, Callee: "p.Dig", CalleeKey: dig}
	injected := Config{
		RegisterFuncs: []string{"Register"}, ResolveFuncs: []string{"Get"},
		InjectorTypes: []string{"Injector"},
	}
	registers := []facts.Call{
		register(boot, "Register", "p.A", newA), register(boot, "Register", "p.B"),
	}
	pullOfDug := resolve{site: dug, typ: "p.B"}
	pullOfDig := resolve{site: dig, typ: "p.B"}
	ctors := map[entities.DeclKey][]facts.TypeRef{newA: {}, dig: {typeRef("*x/di.Injector")}}
	everywhere := func(entities.Decl) bool { return true }
	testCases := []modelCase{
		{
			name: "a resolve in a statement cut from a shared helper is shared",
			cfg:  injected,
			decls: []entities.Decl{
				declOf(boot), declOf(newA), declOf(dug), declOf(dig), declOf(warm),
			},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: registers, newA: {viaDig}, dug: {get(dug, "Get", "p.B")},
					warm: {{Site: warm, Callee: "p.Dig", CalleeKey: dig}},
				},
				ctors: ctors,
			},
			want: model{
				resolves: []resolve{pullOfDug},
				registrations: []registration{
					{site: boot, typ: "p.A", pulls: []resolve{pullOfDug}},
					{site: boot, typ: "p.B"},
				},
				shared: map[resolve]bool{pullOfDug: true},
			},
			seed: everywhere,
		},
		{
			name:  "a statement cut from the constructor does not share what the constructor runs",
			cfg:   injected,
			decls: []entities.Decl{declOf(boot), declOf(newA), declOf(slice), declOf(dig)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: registers, newA: {viaDig}, dig: {get(dig, "Get", "p.B")},
					slice: {{Site: slice, Callee: "p.Dig", CalleeKey: dig}},
				},
				ctors: ctors,
			},
			want: model{
				resolves: []resolve{pullOfDig},
				registrations: []registration{
					{site: boot, typ: "p.A", pulls: []resolve{pullOfDig}},
					{site: boot, typ: "p.B"},
				},
			},
			seed: everywhere,
		},
	}

	requireModels(t, testCases)
}

func TestBuildModelLeavesHelpersOfCodeThatDoesNotRun(t *testing.T) {
	t.Parallel()

	boot, newA, dig := fnKey("Boot"), fnKey("NewA"), fnKey("Dig")
	checked := fnKey("Warm")
	checked.File = "boot/warm_test.go"
	injected := Config{
		RegisterFuncs: []string{"Register"}, ResolveFuncs: []string{"Get"},
		InjectorTypes: []string{"Injector"},
	}
	registers := []facts.Call{
		register(boot, "Register", "p.A", newA), register(boot, "Register", "p.B"),
	}
	pull := resolve{site: newA, typ: "p.B"}
	alone := model{
		resolves: []resolve{pull},
		registrations: []registration{
			{site: boot, typ: "p.A", pulls: []resolve{pull}},
			{site: boot, typ: "p.B"},
		},
	}
	ctors := map[entities.DeclKey][]facts.TypeRef{newA: {}, dig: {typeRef("*x/di.Injector")}}
	everywhere := func(entities.Decl) bool { return true }
	testCases := []modelCase{
		{
			name:  "a test caller does not share the constructor it calls",
			cfg:   injected,
			decls: []entities.Decl{declOf(boot), declOf(newA), declOf(checked)},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: registers, newA: {get(newA, "Get", "p.B")},
					checked: {{Site: checked, Callee: "p.NewA", CalleeKey: newA}},
				},
				ctors: ctors,
			},
			want: alone,
			seed: everywhere,
		},
	}

	requireModels(t, testCases)
}

func TestPullsAreProvidesNotRegisteredEdges(t *testing.T) {
	t.Parallel()

	setupA, setupB := fnKey("SetupA"), fnKey("SetupB")
	newA := fnKey("NewA")

	testCases := []struct {
		name  string
		input model
		want  []entities.Edge
	}{
		{
			name: "a pull needs the registration of its type from the register site",
			input: model{registrations: []registration{
				{site: setupA, typ: "p.A", pulls: []resolve{{site: newA, typ: "p.B"}}},
				{site: setupB, typ: "p.B"},
			}},
			want: []entities.Edge{provides(setupA, setupB)},
		},
		{
			name: "a pull answered in the same function adds nothing",
			input: model{registrations: []registration{
				{site: setupA, typ: "p.A", pulls: []resolve{{site: newA, typ: "p.B"}}},
				{site: setupA, typ: "p.B"},
			}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.input.edges(); !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("edges() = %v, want %v", got, testCase.want)
			}
		})
	}
}
