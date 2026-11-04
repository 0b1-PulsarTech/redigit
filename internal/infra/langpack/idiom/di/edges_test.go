package di

import (
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
)

func provides(from, to entities.DeclKey) entities.Edge {
	return entities.Edge{Dependent: from, Provider: to, Reason: entities.ReasonProvides}
}

func registered(from, to entities.DeclKey) entities.Edge {
	return entities.Edge{Dependent: from, Provider: to, Reason: entities.ReasonRegistered}
}

type edgeCase struct {
	name  string
	input model
	want  []entities.Edge
}

func TestModelEdges(t *testing.T) {
	t.Parallel()

	repo, store, use := stmtKey("Boot", "1"), stmtKey("Boot", "2"), stmtKey("Boot", "3")
	whole := fnKey("Boot")
	testCases := []edgeCase{
		{
			name: "statement level: a register needs its arguments and a resolve its type",
			input: model{
				registrations: []registration{
					{site: store, typ: "p.Store"},
					{site: repo, typ: "p.Repo", needs: refs("p.Store")},
				},
				resolves: []resolve{{site: use, typ: "p.Repo"}},
			},
			want: []entities.Edge{provides(repo, store), registered(use, repo)},
		},
		{
			name: "function level: one site never waits for itself",
			input: model{
				registrations: []registration{
					{site: whole, typ: "p.Store"},
					{site: whole, typ: "p.Repo", needs: refs("p.Store")},
				},
				resolves: []resolve{{site: whole, typ: "p.Repo"}},
			},
		},
		{
			name: "an unbound argument and an unbound resolve add nothing",
			input: model{
				registrations: []registration{
					{site: repo, typ: "p.Repo", needs: refs("p.Missing")},
				},
				resolves: []resolve{{site: use, typ: "p.Gone"}},
			},
		},
		{
			name: "a type registered twice is needed from both registrations",
			input: model{
				registrations: []registration{
					{site: store, typ: "p.Store"},
					{site: repo, typ: "p.Store"},
				},
				resolves: []resolve{{site: use, typ: "p.Store"}},
			},
			want: []entities.Edge{registered(use, repo), registered(use, store)},
		},
		{
			name: "the same resolve twice yields one edge",
			input: model{
				registrations: []registration{{site: repo, typ: "p.Repo"}},
				resolves: []resolve{
					{site: use, typ: "p.Repo"}, {site: use, typ: "p.Repo"},
				},
			},
			want: []entities.Edge{registered(use, repo)},
		},
		{
			name: "a slice argument is not satisfied by its element",
			input: model{
				registrations: []registration{
					{site: store, typ: "p.Handler"},
					{site: repo, typ: "p.Router", needs: refs("[]p.Handler")},
				},
			},
		},
	}

	requireEdges(t, testCases)
}

func TestModelEdgesForInterfaces(t *testing.T) {
	t.Parallel()

	repo, store, use := stmtKey("Boot", "1"), stmtKey("Boot", "2"), stmtKey("Boot", "3")
	iface, pg, mem := typeKey("Store"), typeKey("Pg"), typeKey("Mem")
	testCases := []edgeCase{
		{
			name: "an interface resolve needs the registration of its one implementor",
			input: model{
				registrations: []registration{{site: repo, typ: "p.Pg", key: pg}},
				resolves:      []resolve{{site: use, typ: "p.Store", key: iface}},
				impls:         map[entities.DeclKey][]entities.DeclKey{iface: {pg}},
			},
			want: []entities.Edge{registered(use, repo)},
		},
		{
			name: "an interface resolve that several types serve is ambiguous and needs none",
			input: model{
				registrations: []registration{
					{site: repo, typ: "p.Pg", key: pg}, {site: store, typ: "p.Mem", key: mem},
				},
				resolves: []resolve{{site: use, typ: "p.Store", key: iface}},
				impls:    map[entities.DeclKey][]entities.DeclKey{iface: {mem, pg}},
			},
		},
		{
			name: "a resolve of all implementors needs every registration",
			input: model{
				registrations: []registration{
					{site: repo, typ: "p.Pg", key: pg}, {site: store, typ: "p.Mem", key: mem},
				},
				resolves: []resolve{{site: use, typ: "p.Store", key: iface, all: true}},
				impls:    map[entities.DeclKey][]entities.DeclKey{iface: {mem, pg}},
			},
			want: []entities.Edge{registered(use, repo), registered(use, store)},
		},
		{
			name: "a resolve of all takes the exact registration and the implementors",
			input: model{
				registrations: []registration{
					{site: repo, typ: "p.Store", key: iface}, {site: store, typ: "p.Pg", key: pg},
				},
				resolves: []resolve{{site: use, typ: "p.Store", key: iface, all: true}},
				impls:    map[entities.DeclKey][]entities.DeclKey{iface: {pg}},
			},
			want: []entities.Edge{registered(use, repo), registered(use, store)},
		},
		{
			name: "a type registered itself is not served by implementors",
			input: model{
				registrations: []registration{
					{site: repo, typ: "p.Store", key: iface}, {site: store, typ: "p.Pg", key: pg},
				},
				resolves: []resolve{{site: use, typ: "p.Store", key: iface}},
				impls:    map[entities.DeclKey][]entities.DeclKey{iface: {pg}},
			},
			want: []entities.Edge{registered(use, repo)},
		},
	}

	requireEdges(t, testCases)
}

func requireEdges(t *testing.T, testCases []edgeCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.input.edges(); !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("edges() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestModelEdgesAreDeterministic(t *testing.T) {
	t.Parallel()

	first, second, third := stmtKey("Boot", "1"), stmtKey("Boot", "2"), stmtKey("Boot", "3")
	registrations := []registration{
		{site: first, typ: "p.A"},
		{site: second, typ: "p.B", needs: refs("p.A")},
		{site: third, typ: "p.C", needs: refs("p.B", "p.A")},
	}
	reversed := []registration{
		{site: third, typ: "p.C", needs: refs("p.A", "p.B")},
		{site: second, typ: "p.B", needs: refs("p.A")},
		{site: first, typ: "p.A"},
	}
	resolves := []resolve{{site: third, typ: "p.A"}, {site: second, typ: "p.A"}}
	flipped := []resolve{{site: second, typ: "p.A"}, {site: third, typ: "p.A"}}
	want := model{registrations: registrations, resolves: resolves}.edges()

	testCases := []struct {
		name  string
		input model
	}{
		{
			name:  "registrations and needs reversed",
			input: model{registrations: reversed, resolves: resolves},
		},
		{name: "resolves reversed", input: model{registrations: registrations, resolves: flipped}},
		{name: "everything reversed", input: model{registrations: reversed, resolves: flipped}},
	}

	if len(want) != 5 {
		t.Fatalf("edges() = %v, want 5", want)
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.input.edges(); !reflect.DeepEqual(got, want) {
				t.Fatalf("edges() = %v, want the same %v", got, want)
			}
		})
	}
}

type chain struct {
	known world
	decls []entities.Decl
	refs  []entities.Edge
	order entities.Ordering
}

func chainOf(count int, grain string) chain {
	built := chain{
		known: world{
			calls: map[entities.DeclKey][]facts.Call{},
			ctors: map[entities.DeclKey][]facts.TypeRef{},
		},
		order: entities.Ordering{At: map[entities.DeclKey]entities.Position{}},
	}

	for idx := range count {
		site := fnKey("Setup" + strconv.Itoa(idx))
		if grain == "sliced" {
			site = stmtKey("Boot", strconv.Itoa(idx))
		}

		ctor, typ := fnKey("New"+strconv.Itoa(idx)), "p.T"+strconv.Itoa(idx)
		built.known.calls[site] = []facts.Call{register(site, "Register", typ, ctor)}
		built.known.ctors[ctor] = nil

		if idx > 0 {
			built.known.ctors[ctor] = refs("p.T" + strconv.Itoa(idx-1))
		}

		built.decls = append(built.decls, declOf(site), declOf(ctor))
		built.refs = append(built.refs,
			entities.Edge{Dependent: site, Provider: ctor, Reason: entities.ReasonReference})
		built.order.At[site], built.order.At[ctor] = entities.Position(idx), entities.Position(idx)

		if idx%4 == 0 {
			use := stmtKey("Run", strconv.Itoa(idx))
			built.known.calls[use] = []facts.Call{get(use, "Get", typ)}
			built.decls = append(built.decls, declOf(use))
			built.order.At[use] = entities.Position(count + idx)
		}
	}

	return built
}

func BenchmarkEdges(b *testing.B) {
	pack := idiom.NewPack(nil, nil)

	for _, size := range []int{250, 1000, 2000} {
		for _, level := range []string{"function", "sliced"} {
			b.Run(strconv.Itoa(size)+"/"+level, func(b *testing.B) {
				input := chainOf(size, level)

				b.ReportAllocs()

				for b.Loop() {
					built, err := buildModel(baseConfig(), input.decls, input.known)
					if err != nil {
						b.Fatalf("buildModel() = %v", err)
					}

					merged := slices.Concat(input.refs, built.edges())
					_ = pack.FilterEdges(entities.Snapshot{}, merged)
				}
			})
		}
	}
}

func (set world) Calls(site entities.DeclKey) []facts.Call { return set.calls[site] }

func (set world) CtorArgTypes(fn entities.DeclKey) ([]facts.TypeRef, error) {
	args, found := set.ctors[fn]
	if !found {
		return nil, errNoFunc
	}

	return args, nil
}

func (set world) Implementors(iface entities.DeclKey) []entities.DeclKey { return set.impls[iface] }

func (set world) StmtCallsInto(entities.DeclKey, string) ([]facts.Call, error) { return nil, nil }
