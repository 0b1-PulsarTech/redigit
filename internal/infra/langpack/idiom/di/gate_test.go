package di

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
)

type gateCase struct {
	name      string
	roots     []string
	positions map[entities.DeclKey]entities.Position
	edges     []entities.Edge
	factsErr  error
	wantValue int
	wantTotal int
	wantText  []string
}

func TestGateMeasureWindows(t *testing.T) {
	t.Parallel()

	head, early, late := fnKey("Boot"), stmtKey("Boot", "1"), stmtKey("Boot", "2")
	testCases := []gateCase{
		{
			name:      "a registration ahead of its resolve site leaves no window",
			positions: map[entities.DeclKey]entities.Position{early: 1, late: 2, head: 1},
			wantValue: 1, wantTotal: 1,
			wantText: []string{"dynamic registration"},
		},
		{
			name:      "a registration in the same commit leaves no window",
			positions: map[entities.DeclKey]entities.Position{early: 3, late: 3, head: 3},
			wantValue: 1, wantTotal: 1,
			wantText: []string{
				"so a window through them stays hidden",
				"constructor is a func literal has no needs or pulls",
				"a wrapper that is no configured register function reads as a direct call of what it was handed",
				"a func literal handed to a registration is taken to run when the container builds",
				"so one stored for boot code to run later reads as folded",
				"under configured roots a package var or init",
				"counts as boot only when a root reaches it",
				"a root in a test file opens nothing, so a resolve only a test reaches counts as outside the roots",
				"a func converted to no func type, as any(helper), hands nothing on",
				"a root glob also opens a plain function a constructor calls",
			},
		},
		{
			name:      "a late registration is a boot window",
			positions: map[entities.DeclKey]entities.Position{early: 5, late: 2, head: 1},
			wantValue: 0, wantTotal: 1,
		},
		{
			name:      "an unplaced registration is a boot window",
			positions: map[entities.DeclKey]entities.Position{late: 2, head: 1},
			wantValue: 0, wantTotal: 1,
		},
		{
			name:      "a resolve site nothing orders is left out",
			positions: map[entities.DeclKey]entities.Position{early: 1},
			wantValue: 0, wantTotal: 0,
			wantText: []string{"resolve sites the ordering never placed: 1"},
		},
	}

	requireGate(t, testCases)
}

func TestGateMeasureRoots(t *testing.T) {
	t.Parallel()

	head, early, late := fnKey("Boot"), stmtKey("Boot", "1"), stmtKey("Boot", "2")
	entry, viaRef := fnKey("Helper"), entities.ReasonReference
	testRoot := entities.DeclKey{File: "boot/boot_test.go", Kind: entities.KindFunc, Name: "Test"}
	testCases := []gateCase{
		{
			name:      "a root keeps the sites it reaches through its statements",
			roots:     []string{"Boot"},
			positions: map[entities.DeclKey]entities.Position{early: 5, late: 2, head: 1},
			wantValue: 0, wantTotal: 1,
		},
		{
			name:      "a root leaves out the sites it cannot reach",
			roots:     []string{"Helper"},
			positions: map[entities.DeclKey]entities.Position{early: 5, late: 2, head: 1, entry: 1},
			wantText: []string{
				"1 resolve sites outside the code reachable from the configured roots",
			},
		},
		{
			name:      "a root reaches through the graph edges",
			roots:     []string{"Helper"},
			positions: map[entities.DeclKey]entities.Position{early: 1, late: 2, head: 1, entry: 1},
			edges:     []entities.Edge{{Dependent: entry, Provider: head, Reason: viaRef}},
			wantValue: 1, wantTotal: 1,
		},
		{
			name:      "a root in a test file reaches nothing",
			roots:     []string{"Test"},
			positions: map[entities.DeclKey]entities.Position{early: 1, late: 2, head: 1},
			edges:     []entities.Edge{{Dependent: testRoot, Provider: head, Reason: viaRef}},
			wantText:  []string{"1 resolve sites outside the code reachable"},
		},
		{
			name:      "a facts failure is reported as not measured",
			positions: map[entities.DeclKey]entities.Position{early: 1, late: 2},
			factsErr:  errNoFunc,
			wantText:  []string{"not measured: " + errNoFunc.Error()},
		},
	}

	requireGate(t, testCases)
}

func requireGate(t *testing.T, testCases []gateCase) {
	t.Helper()

	head, early, late := fnKey("Boot"), stmtKey("Boot", "1"), stmtKey("Boot", "2")
	entry := fnKey("Helper")
	testRoot := entities.DeclKey{File: "boot/boot_test.go", Kind: entities.KindFunc, Name: "Test"}
	decls := []entities.Decl{
		declOf(head), declOf(early), declOf(late), declOf(entry), declOf(testRoot),
	}
	known := world{calls: map[entities.DeclKey][]facts.Call{
		early: {register(early, "Register", "p.Repo")},
		late:  {get(late, "Get", "p.Repo")},
	}}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			graph := entities.NewGraph(decls, testCase.edges, nil)
			order := entities.Ordering{At: testCase.positions}
			mock := known.facts(gomock.NewController(t))

			cfg := baseConfig()
			cfg.Roots = testCase.roots

			measured := gate{cfg: cfg, factsOf: func([]entities.Decl) (idiom.Facts, error) {
				if testCase.factsErr != nil {
					return nil, testCase.factsErr
				}

				return mock, nil
			}}.Measure(graph, order)

			if err := measured.Validate(); err != nil {
				t.Fatalf("Measure() is not valid: %v", err)
			}

			if measured.Value != testCase.wantValue || measured.Total != testCase.wantTotal {
				t.Fatalf("Measure() = %d of %d, want %d of %d",
					measured.Value, measured.Total, testCase.wantValue, testCase.wantTotal)
			}

			joined := strings.Join(measured.Excluded, "|")
			for _, text := range testCase.wantText {
				if !strings.Contains(joined, text) {
					t.Fatalf("Excluded = %q, want it to hold %q", joined, text)
				}
			}
		})
	}
}

func TestGateCountsResolveSitesOfUnboundTypes(t *testing.T) {
	t.Parallel()

	site, boot := stmtKey("Boot", "2"), stmtKey("Boot", "1")
	testCases := []struct {
		name      string
		calls     map[entities.DeclKey][]facts.Call
		wantTotal int
		wantText  string
	}{
		{
			name: "two calls on a type nothing binds are one site",
			calls: map[entities.DeclKey][]facts.Call{
				site: {get(site, "Get", "p.Gone"), get(site, "MustGet", "p.Gone")},
			},
			wantText: "1 resolve sites whose type no registration binds",
		},
		{
			name: "a type some registration binds is measured",
			calls: map[entities.DeclKey][]facts.Call{
				boot: {register(boot, "Register", "p.Gone")},
				site: {get(site, "Get", "p.Gone")},
			},
			wantTotal: 1,
			wantText:  "0 resolve sites whose type no registration binds",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			known := world{calls: testCase.calls}
			graph := entities.NewGraph([]entities.Decl{declOf(boot), declOf(site)}, nil, nil)
			order := entities.Ordering{At: map[entities.DeclKey]entities.Position{boot: 1, site: 1}}
			mock := known.facts(gomock.NewController(t))

			probe := gate{cfg: baseConfig(), factsOf: func([]entities.Decl) (idiom.Facts, error) {
				return mock, nil
			}}

			measured := probe.Measure(graph, order)
			if measured.Total != testCase.wantTotal ||
				!strings.Contains(strings.Join(measured.Excluded, "|"), testCase.wantText) {
				t.Fatalf("Measure() = %+v, want %d sites and %q",
					measured, testCase.wantTotal, testCase.wantText)
			}
		})
	}
}

func TestGateName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		gate gate
		want string
	}{
		{name: "a named gate", gate: gate{name: "go-di"}, want: "go-di"},
		{name: "a gate with no name", gate: gate{}},
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

func BenchmarkGate(b *testing.B) {
	pack := idiom.NewPack(nil, nil)

	for _, size := range []int{250, 1000, 2000} {
		for _, level := range []string{"function", "sliced"} {
			b.Run(strconv.Itoa(size)+"/"+level, func(b *testing.B) {
				input := chainOf(size, level)

				built, err := buildModel(baseConfig(), input.decls, input.known)
				if err != nil {
					b.Fatalf("buildModel() = %v", err)
				}

				merged := slices.Concat(input.refs, built.edges())
				graph := entities.NewGraph(
					input.decls, pack.FilterEdges(entities.Snapshot{}, merged), nil,
				)
				probe := gate{
					cfg: baseConfig(),
					factsOf: func([]entities.Decl) (idiom.Facts, error) {
						return input.known, nil
					},
				}

				b.ReportAllocs()

				for b.Loop() {
					if measured := probe.Measure(graph, input.order); measured.Total == 0 {
						b.Fatalf("Measure() = %+v, want measured sites", measured)
					}
				}
			})
		}
	}
}

func TestGateMeasuresPulledResolvesThroughTheirRegistration(t *testing.T) {
	t.Parallel()

	setup, newA, use := fnKey("Setup"), fnKey("NewA"), fnKey("Use")
	pulledText := "1 resolve sites inside constructors, folded into the needs of their registration"

	testCases := []struct {
		name      string
		positions map[entities.DeclKey]entities.Position
		wantValue int
		wantTotal int
	}{
		{
			name:      "a constructor placed ahead of its registration opens no window",
			positions: map[entities.DeclKey]entities.Position{newA: 1, setup: 2, use: 3},
			wantValue: 1, wantTotal: 1,
		},
		{
			name:      "a site built before the constructor pulls is a window",
			positions: map[entities.DeclKey]entities.Position{newA: 1, setup: 3, use: 2},
			wantValue: 0, wantTotal: 1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			known := world{
				calls: map[entities.DeclKey][]facts.Call{
					setup: {
						register(setup, "Register", "p.A", newA),
						register(setup, "Register", "p.B"),
					},
					newA: {get(newA, "Get", "p.B")},
					use:  {get(use, "Get", "p.A")},
				},
				ctors: map[entities.DeclKey][]facts.TypeRef{newA: {}},
			}
			graph := entities.NewGraph(
				[]entities.Decl{declOf(setup), declOf(newA), declOf(use)}, nil, nil,
			)
			mock := known.facts(gomock.NewController(t))
			cfg := Config{
				RegisterFuncs: []string{"Register"}, ResolveFuncs: []string{"Get"},
				InjectorTypes: []string{"Injector"},
			}

			measured := gate{cfg: cfg, factsOf: func([]entities.Decl) (idiom.Facts, error) {
				return mock, nil
			}}.Measure(graph, entities.Ordering{At: testCase.positions})

			if measured.Value != testCase.wantValue || measured.Total != testCase.wantTotal {
				t.Fatalf("Measure() = %d of %d, want %d of %d",
					measured.Value, measured.Total, testCase.wantValue, testCase.wantTotal)
			}

			joined := strings.Join(measured.Excluded, "|")
			if !strings.Contains(joined, pulledText) {
				t.Fatalf("Excluded = %q, want it to hold %q", joined, pulledText)
			}
		})
	}
}

func TestGateMeasuresHelpersTheConstructorSharesWithOtherCode(t *testing.T) {
	t.Parallel()

	setup, newA, dig := fnKey("Setup"), fnKey("NewA"), fnKey("Dig")
	warm, use := fnKey("Warm"), fnKey("Use")
	cfg := Config{
		RegisterFuncs: []string{"Register"}, ResolveFuncs: []string{"Get"},
		InjectorTypes: []string{"Injector"},
	}
	ctors := map[entities.DeclKey][]facts.TypeRef{newA: {}, dig: {typeRef("*x/di.Injector")}}
	registers := []facts.Call{
		register(setup, "Register", "p.A", newA), register(setup, "Register", "p.B"),
	}
	viaDig := facts.Call{Site: newA, Callee: "p.Dig", CalleeKey: dig}
	text := []string{"0 resolve sites inside constructors"}

	testCases := []readyCase{
		{
			name: "a helper boot code runs too keeps its window and the resolve of the type",
			cfg:  cfg,
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					setup: registers, newA: {viaDig}, dig: {get(dig, "Get", "p.B")},
					warm: {{Site: warm, Callee: "p.Dig", CalleeKey: dig}},
					use:  {get(use, "Get", "p.A")},
				},
				ctors: ctors,
			},
			positions: map[entities.DeclKey]entities.Position{
				dig: 1, warm: 1, newA: 1, setup: 5, use: 6,
			},
			wantValue: 1, wantTotal: 2, wantText: text,
		},
		{
			name: "a helper handed on as a function value is run by its holder too",
			cfg:  cfg,
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					setup: registers, newA: {viaDig}, dig: {get(dig, "Get", "p.B")},
					warm: {{Site: warm, FuncArgs: []entities.DeclKey{dig}}},
				},
				ctors: ctors,
			},
			positions: map[entities.DeclKey]entities.Position{dig: 1, warm: 1, newA: 1, setup: 5},
			wantValue: 0, wantTotal: 1, wantText: text,
		},
	}

	requireReadiness(t, testCases)
}

func TestGateMeasuresConstructorResolvesByTheRegistrationsThatRun(t *testing.T) {
	t.Parallel()

	setup, oldSetup, entry, newA := fnKey("Setup"), fnKey("OldSetup"), fnKey("main"), fnKey("NewA")
	cfg := Config{RegisterFuncs: []string{"Register"}, ResolveFuncs: []string{"Get"}}
	ctors := map[entities.DeclKey][]facts.TypeRef{newA: {}}
	byHand := facts.Call{Site: entry, Callee: "p.NewA", CalleeKey: newA}
	registers := func(site entities.DeclKey) []facts.Call {
		return []facts.Call{
			register(site, "Register", "p.A", newA), register(site, "Register", "p.B"),
		}
	}

	testCases := []readyCase{
		{
			name: "a dead registration leaves the constructor resolve to the code that calls it",
			cfg: Config{
				RegisterFuncs: cfg.RegisterFuncs,
				ResolveFuncs:  cfg.ResolveFuncs,
				Roots:         []string{"main"},
			},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					oldSetup: registers(oldSetup), newA: {get(newA, "Get", "p.B")}, entry: {byHand},
				},
				ctors: ctors,
			},
			positions: map[entities.DeclKey]entities.Position{newA: 1, entry: 1, oldSetup: 5},
			edges: []entities.Edge{
				{Dependent: entry, Provider: newA, Reason: entities.ReasonReference},
			},
			wantValue: 0, wantTotal: 1,
			wantText: []string{"0 resolve sites inside constructors"},
		},
		{
			name: "a constructor called by hand leaves its resolve in the count",
			cfg:  cfg,
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					setup: registers(setup), newA: {get(newA, "Get", "p.B")}, entry: {byHand},
				},
				ctors: ctors,
			},
			positions: map[entities.DeclKey]entities.Position{newA: 1, entry: 1, setup: 5},
			wantValue: 0, wantTotal: 1,
			wantText: []string{"0 resolve sites inside constructors"},
		},
		{
			name: "a resolve off the roots stays off even when a live registration pulls it",
			cfg: Config{
				RegisterFuncs: cfg.RegisterFuncs,
				ResolveFuncs:  cfg.ResolveFuncs,
				Roots:         []string{"Setup"},
			},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					setup: registers(setup), newA: {get(newA, "Get", "p.B")},
				},
				ctors: ctors,
			},
			positions: map[entities.DeclKey]entities.Position{setup: 1, newA: 2},
			wantText: []string{
				"1 resolve sites outside the code reachable", "0 resolve sites inside constructors",
			},
		},
	}

	requireReadiness(t, testCases)
}
