package di

import (
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
)

type readyCase struct {
	name      string
	cfg       Config
	world     world
	positions map[entities.DeclKey]entities.Position
	edges     []entities.Edge
	wantValue int
	wantTotal int
	wantText  []string
}

func TestGateReadinessOfRegistrations(t *testing.T) {
	t.Parallel()

	boot := stmtKey("Boot", "1")
	other := stmtKey("Boot", "2")
	use := stmtKey("Boot", "3")
	iface, pg, mem := typeKey("Store"), typeKey("Pg"), typeKey("Mem")
	impls := map[entities.DeclKey][]entities.DeclKey{iface: {mem, pg}}
	testCases := []readyCase{
		{
			name: "an interface resolve is ready once its one implementor is registered",
			cfg:  baseConfig(),
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot: {keyed(register(boot, "Register", "p.Pg"), pg)},
					use:  {keyed(get(use, "Get", "p.Store"), iface)},
				},
				impls: map[entities.DeclKey][]entities.DeclKey{iface: {pg}},
			},
			positions: map[entities.DeclKey]entities.Position{boot: 1, use: 2},
			wantValue: 1, wantTotal: 1,
		},
		{
			name: "an interface resolve two types serve is ambiguous and not ready",
			cfg:  baseConfig(),
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot:  {keyed(register(boot, "Register", "p.Pg"), pg)},
					other: {keyed(register(other, "Register", "p.Mem"), mem)},
					use:   {keyed(get(use, "Get", "p.Store"), iface)},
				},
				impls: impls,
			},
			positions: map[entities.DeclKey]entities.Position{boot: 1, other: 1, use: 2},
			wantValue: 0, wantTotal: 1,
		},
		{
			name: "a resolve of all implementors waits for the last of them",
			cfg:  Config{RegisterFuncs: []string{"Register"}, ResolveFuncs: []string{"GetAll"}},
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					boot:  {keyed(register(boot, "Register", "p.Pg"), pg)},
					other: {keyed(register(other, "Register", "p.Mem"), mem)},
					use:   {keyed(get(use, "GetAll", "p.Store"), iface)},
				},
				impls: impls,
			},
			positions: map[entities.DeclKey]entities.Position{boot: 1, other: 3, use: 2},
			wantValue: 0, wantTotal: 1,
		},
	}

	requireReadiness(t, testCases)
}

func TestGateReadinessThroughNeeds(t *testing.T) {
	t.Parallel()

	top, dep := fnKey("NewTop"), fnKey("NewDep")
	regTop, use, regDep := stmtKey("Boot", "1"), stmtKey("Boot", "2"), stmtKey("Boot", "3")
	topCalls := []facts.Call{register(regTop, "Register", "p.Top", top)}
	chained := world{
		calls: map[entities.DeclKey][]facts.Call{
			regTop: topCalls, use: {get(use, "Get", "p.Top")},
			regDep: {register(regDep, "Register", "p.Dep", dep)},
		},
		ctors: map[entities.DeclKey][]facts.TypeRef{top: {typeRef("p.Dep")}, dep: {}},
	}
	testCases := []readyCase{
		{
			name:      "a resolve waits for the late registration of a constructor need",
			cfg:       baseConfig(),
			world:     chained,
			positions: map[entities.DeclKey]entities.Position{regTop: 1, use: 2, regDep: 5},
			wantValue: 0, wantTotal: 1,
		},
		{
			name:      "a resolve is ready when the constructor need is registered ahead",
			cfg:       baseConfig(),
			world:     chained,
			positions: map[entities.DeclKey]entities.Position{regTop: 1, use: 2, regDep: 1},
			wantValue: 1, wantTotal: 1,
		},
		{
			name: "a constructor need that nothing registers leaves the resolve not ready",
			cfg:  baseConfig(),
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					regTop: topCalls, use: {get(use, "Get", "p.Top")},
				},
				ctors: map[entities.DeclKey][]facts.TypeRef{top: {typeRef("p.Dep")}},
			},
			positions: map[entities.DeclKey]entities.Position{regTop: 1, use: 2},
			wantValue: 0, wantTotal: 1,
		},
		{
			name: "constructors that need each other never become ready and never loop",
			cfg:  baseConfig(),
			world: world{
				calls: map[entities.DeclKey][]facts.Call{
					regTop: topCalls, use: {get(use, "Get", "p.Top")},
					regDep: {register(regDep, "Register", "p.Dep", dep)},
				},
				ctors: map[entities.DeclKey][]facts.TypeRef{
					top: {typeRef("p.Dep")}, dep: {typeRef("p.Top")},
				},
			},
			positions: map[entities.DeclKey]entities.Position{regTop: 1, use: 2, regDep: 1},
			wantValue: 0, wantTotal: 1,
		},
	}

	requireReadiness(t, testCases)
}

func requireReadiness(t *testing.T, testCases []readyCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var decls []entities.Decl

			for site := range testCase.world.calls {
				decls = append(decls, declOf(site))
			}

			for site := range testCase.positions {
				if !slices.ContainsFunc(decls, func(decl entities.Decl) bool {
					return decl.Key == site
				}) {
					decls = append(decls, declOf(site))
				}
			}

			graph := entities.NewGraph(decls, testCase.edges, nil)
			order := entities.Ordering{At: testCase.positions}
			mock := testCase.world.facts(gomock.NewController(t))

			probe := gate{cfg: testCase.cfg, factsOf: func([]entities.Decl) (idiom.Facts, error) {
				return mock, nil
			}}
			measured := probe.Measure(graph, order)

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

func TestReadinessSettlesEntriesThatWaitOnACycle(t *testing.T) {
	t.Parallel()

	regT, regX, regR1, regR2 := fnKey("RegT"), fnKey("RegX"), fnKey("RegR1"), fnKey("RegR2")
	ctor, use := fnKey("NewT"), fnKey("Use")
	pull := resolve{site: ctor, typ: "p.R", all: true}

	testCases := []struct {
		name          string
		registrations []registration
		want          bool
	}{
		{
			name: "an entry that waited only on an in-progress one is built once that one is",
			registrations: []registration{
				{site: regT, typ: "p.T", needs: refs("p.X"), pulls: []resolve{pull}},
				{site: regX, typ: "p.X", needs: refs("p.R")},
				{site: regR1, typ: "p.R", needs: refs("p.X")},
				{site: regR2, typ: "p.R"},
			},
			want: true,
		},
		{
			name: "entries that need each other stay unbuilt",
			registrations: []registration{
				{site: regT, typ: "p.T", needs: refs("p.X"), pulls: []resolve{pull}},
				{site: regX, typ: "p.X", needs: refs("p.R")},
				{site: regR1, typ: "p.R", needs: refs("p.T")},
				{site: regR2, typ: "p.R"},
			},
			want: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			order := entities.Ordering{At: map[entities.DeclKey]entities.Position{
				regT: 1, regX: 1, regR1: 1, regR2: 1, use: 2,
			}}
			built := model{registrations: testCase.registrations}
			check := newReadiness(built.catalog(), order)

			if got := check.readyAt(resolve{site: use, typ: "p.T"}) <= 2; got != testCase.want {
				t.Fatalf("ready() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestReadinessOfAGetOfAllOverACycle(t *testing.T) {
	t.Parallel()

	regA, regC, regD := fnKey("RegA"), fnKey("RegC"), fnKey("RegD")
	regF, regG, use := fnKey("RegF"), fnKey("RegG"), fnKey("Use")
	allU := resolve{site: use, typ: "p.U", all: true}

	testCases := []struct {
		name string
		want entities.Position
	}{
		{name: "a get of all is ready once the cycle settles", want: 1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			order := entities.Ordering{At: map[entities.DeclKey]entities.Position{
				regA: 1, regC: 1, regD: 1, regF: 1, regG: 5, use: 2,
			}}
			built := model{registrations: []registration{
				{site: regA, typ: "p.V", needs: refs("p.U")},
				{site: regC, typ: "p.U", needs: refs("p.X")},
				{site: regD, typ: "p.U"},
				{site: regF, typ: "p.X", needs: refs("p.V")},
				{site: regG, typ: "p.X"},
			}}
			check := newReadiness(built.catalog(), order)

			if got := check.readyAt(allU); got != testCase.want {
				t.Fatalf("readyAt(all U) = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestReadinessTellsTwoRegistrationsOfOneSiteApart(t *testing.T) {
	t.Parallel()

	setup, regL, use := fnKey("Setup"), fnKey("RegL"), fnKey("Use")
	plain := registration{site: setup, typ: "p.S"}
	needy := registration{site: setup, typ: "p.S", needs: refs("p.L")}
	late := registration{site: regL, typ: "p.L"}

	testCases := []struct {
		name          string
		registrations []registration
		all           bool
		want          entities.Position
	}{
		{
			name:          "a get takes the earlier of them, plain first",
			registrations: []registration{plain, needy, late},
			want:          1,
		},
		{
			name:          "a get takes the earlier of them, needy first",
			registrations: []registration{needy, plain, late},
			want:          1,
		},
		{
			name:          "a get of all waits for the late one, plain first",
			registrations: []registration{plain, needy, late},
			all:           true,
			want:          9,
		},
		{
			name:          "a get of all waits for the late one, needy first",
			registrations: []registration{needy, plain, late},
			all:           true,
			want:          9,
		},
		{
			name:          "a get of all with nothing registered is never ready",
			registrations: []registration{late}, all: true, want: never,
		},
		{
			name: "a get of all waits for a pull of all that nothing serves",
			registrations: []registration{
				{site: setup, typ: "p.S", pulls: []resolve{{site: use, typ: "p.Z", all: true}}},
			},
			all: true, want: never,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			order := entities.Ordering{At: map[entities.DeclKey]entities.Position{
				setup: 1, regL: 9, use: 2,
			}}
			built := model{registrations: testCase.registrations}
			check := newReadiness(built.catalog(), order)

			got := check.readyAt(resolve{site: use, typ: "p.S", all: testCase.all})
			if got != testCase.want {
				t.Fatalf("readyAt() = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestReadinessOfALatticeOverACycle(t *testing.T) {
	t.Parallel()

	const depth = 40

	use, wayOut := fnKey("Use"), fnKey("RegWayOut")
	testCases := []struct {
		name      string
		hasWayOut bool
		want      entities.Position
	}{
		{name: "a two-cycle at the bottom never builds anything above", want: never},
		{name: "a way out of the cycle builds the whole lattice", hasWayOut: true, want: 1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			kinds := [...]string{"A", "B"}
			order := entities.Ordering{At: map[entities.DeclKey]entities.Position{use: 2}}
			built := model{}

			for level := range depth + 1 {
				for pos, kind := range kinds {
					site := fnKey("Reg" + kind + strconv.Itoa(level))
					entry := registration{site: site, typ: "p." + kind + strconv.Itoa(level)}
					entry.needs = refs("p.A"+strconv.Itoa(level+1), "p.B"+strconv.Itoa(level+1))

					if level == depth {
						entry.needs = refs("p." + kinds[1-pos] + strconv.Itoa(depth))

						if testCase.hasWayOut {
							built.registrations = append(built.registrations,
								registration{site: wayOut, typ: entry.typ})
						}
					}

					order.At[site], order.At[wayOut] = 1, 1

					built.registrations = append(built.registrations, entry)
				}
			}

			check := newReadiness(built.catalog(), order)
			if got := check.readyAt(resolve{site: use, typ: "p.A0"}); got != testCase.want {
				t.Fatalf("readyAt() = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestReadinessMatchesARoundRobinReference(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		seed uint64
	}{
		{name: "first seed", seed: 1},
		{name: "second seed", seed: 2},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rng := rand.New(rand.NewPCG(testCase.seed, 7))
			typeAt := func() string { return "p.T" + strconv.Itoa(rng.IntN(4)) }

			for range 300 {
				order := entities.Ordering{At: map[entities.DeclKey]entities.Position{}}
				built := model{}

				for site := range 4 {
					order.At[fnKey("Reg"+strconv.Itoa(site))] = entities.Position(rng.IntN(6) - 1)
				}

				for range 1 + rng.IntN(8) {
					entry := registration{
						site: fnKey("Reg" + strconv.Itoa(rng.IntN(4))),
						typ:  typeAt(),
					}
					for range rng.IntN(3) {
						entry.needs = append(entry.needs, typeRef(typeAt()))
					}

					built.registrations = append(built.registrations, entry)
				}

				check := newReadiness(built.catalog(), order)

				for _, use := range []resolve{{typ: typeAt()}, {typ: typeAt(), all: true}} {
					got, want := check.readyAt(use), reference(built, order, use)
					if got != want {
						t.Fatalf("readyAt(%+v) = %d, want %d", use, got, want)
					}
				}
			}
		})
	}
}

func reference(built model, order entities.Ordering, use resolve) entities.Position {
	check := prepared(built.catalog(), order)

	for changed := true; changed; {
		changed = false

		for id := range check.at {
			at := check.own[id]
			for _, term := range check.terms[id] {
				at = max(at, check.lowest(term))
			}

			if at != check.at[id] {
				check.at[id], changed = at, true
			}
		}
	}

	return check.readyAt(use)
}
