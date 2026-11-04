package bootstrap

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
)

type lineagePlan struct {
	graph    entities.Graph
	floor    map[entities.DeclKey]entities.Position
	base     map[entities.DeclKey]entities.Position
	inserted map[entities.DeclKey]bool
	at       map[entities.DeclKey]entities.Position
	limit    entities.Position
}

func insertedIn(snap entities.Snapshot, file string) bool {
	return snap.HasHistory() && !snap.FirstTouch(file).IsSet()
}

func limitOf(snap entities.Snapshot, slack int) entities.Position {
	var last entities.Position

	for _, touch := range snap.History {
		last = max(last, touch.Position)
	}

	return last + entities.Position(slack)
}

func (plan lineagePlan) anchored() map[entities.DeclKey]bool {
	pulled := map[entities.DeclKey]bool{}
	for _, anchor := range plan.graph.Anchors() {
		pulled[anchor.Decl] = true
	}

	return pulled
}

func (plan lineagePlan) belowFloor() []string {
	anchored := plan.anchored()

	var found []string

	for key, floor := range plan.floor {
		if at, placed := plan.at[key]; placed && at < floor && !anchored[key] {
			found = append(found, fmt.Sprintf("%s at %d, floor %d", key, at, floor))
		}
	}

	return found
}

func (plan lineagePlan) edgeBreaks() []string {
	var found []string

	for _, edge := range plan.graph.Edges() {
		_, hasDependent := plan.graph.Decl(edge.Dependent)
		_, hasProvider := plan.graph.Decl(edge.Provider)

		if hasDependent && hasProvider && plan.at[edge.Dependent] < plan.at[edge.Provider] {
			found = append(found, fmt.Sprintf("%s at %d before %s at %d",
				edge.Dependent, plan.at[edge.Dependent], edge.Provider, plan.at[edge.Provider]))
		}
	}

	return found
}

func (plan lineagePlan) delay(dependent, provider entities.DeclKey) entities.Position {
	if plan.inserted[dependent] && !plan.inserted[provider] {
		return 1
	}

	return 0
}

func (plan lineagePlan) unitFloors() map[entities.UnitID]entities.Position {
	floors := map[entities.UnitID]entities.Position{}

	for _, decl := range plan.graph.Decls() {
		for _, edge := range plan.graph.ExternalConsumers(decl.Key) {
			reader := max(plan.at[edge.Dependent]-plan.delay(edge.Dependent, decl.Key), 0)
			if current, found := floors[decl.Unit]; !found || reader < current {
				floors[decl.Unit] = reader
			}
		}
	}

	return floors
}

func (plan lineagePlan) required(
	decl entities.Decl,
	unitFloor entities.Position,
) entities.Position {
	need := max(plan.base[decl.Key], unitFloor)

	for _, edge := range plan.graph.Needs(decl.Key) {
		if _, known := plan.graph.Decl(edge.Provider); known {
			need = max(need, plan.at[edge.Provider]+plan.delay(decl.Key, edge.Provider))
		}
	}

	return need
}

func (plan lineagePlan) allowed() map[entities.DeclKey]entities.Position {
	floors := plan.unitFloors()
	required := map[entities.DeclKey]entities.Position{}
	atomNeed := map[string]entities.Position{}

	for _, decl := range plan.graph.Decls() {
		required[decl.Key] = plan.required(decl, floors[decl.Unit])
		atomNeed[decl.Atom] = max(atomNeed[decl.Atom], required[decl.Key])
	}

	for _, decl := range plan.graph.Decls() {
		if decl.Atom != "" {
			required[decl.Key] = atomNeed[decl.Atom]
		}
	}

	return required
}

func (plan lineagePlan) looseDecls() []string {
	var found []string

	for key, allowed := range plan.allowed() {
		if plan.at[key] > allowed {
			found = append(found, fmt.Sprintf("%s at %d, nothing needs more than %d",
				key, plan.at[key], allowed))
		}
	}

	return found
}

func (plan lineagePlan) belowRequired() []string {
	anchored := plan.anchored()

	var found []string

	for key, allowed := range plan.allowed() {
		if plan.at[key] < allowed && !anchored[key] {
			found = append(found, fmt.Sprintf("%s at %d, needs %d", key, plan.at[key], allowed))
		}
	}

	return found
}

func (plan lineagePlan) unpinnedAtLimit() []string {
	var found []string

	for key, allowed := range plan.allowed() {
		if plan.at[key] >= plan.limit && allowed <= plan.limit {
			found = append(found, fmt.Sprintf("%s at the limit %d, bound only %d",
				key, plan.limit, allowed))
		}
	}

	return found
}

type planCase struct {
	name                           string
	decls                          []entities.Decl
	edges                          []entities.Edge
	anchors                        []entities.Anchor
	base, at                       map[entities.DeclKey]entities.Position
	inserted                       map[entities.DeclKey]bool
	limit                          entities.Position
	wantFloor, wantEdge, wantLoose int
	wantBelow, wantLimit           int
}

func planKey(name string) entities.DeclKey {
	return entities.DeclKey{File: name + ".go", Kind: entities.KindFunc, Name: name}
}

func (testCase planCase) check(t *testing.T) {
	t.Helper()

	plan := lineagePlan{
		graph:    entities.NewGraph(testCase.decls, testCase.edges, testCase.anchors),
		floor:    testCase.base,
		base:     testCase.base,
		inserted: testCase.inserted,
		at:       testCase.at,
		limit:    cmp.Or(testCase.limit, 10),
	}

	if got := len(plan.belowFloor()); got != testCase.wantFloor {
		t.Errorf("below floor = %d, want %d", got, testCase.wantFloor)
	}

	if got := len(plan.edgeBreaks()); got != testCase.wantEdge {
		t.Errorf("edge breaks = %d, want %d", got, testCase.wantEdge)
	}

	if got := len(plan.looseDecls()); got != testCase.wantLoose {
		t.Errorf("loose = %d, want %d", got, testCase.wantLoose)
	}

	if got := len(plan.belowRequired()); got != testCase.wantBelow {
		t.Errorf("below required = %d, want %d", got, testCase.wantBelow)
	}

	if got := len(plan.unpinnedAtLimit()); got != testCase.wantLimit {
		t.Errorf("unpinned at limit = %d, want %d", got, testCase.wantLimit)
	}
}

func TestLineagePlanInvariants(t *testing.T) {
	t.Parallel()

	declA, declB, declC := planKey("A"), planKey("B"), planKey("C")
	needs := []entities.Edge{{Dependent: declC, Provider: declA, Reason: entities.ReasonReference}}
	units := []entities.Decl{
		{Key: declA, Unit: "one"}, {Key: declB, Unit: "one"}, {Key: declC, Unit: "two"},
	}
	pulled := []entities.Anchor{{Decl: declA, To: declB, Reason: entities.ReasonReference}}
	baseUnits := map[entities.DeclKey]entities.Position{declA: 1, declB: 1, declC: 2}
	pair := []entities.Edge{{Dependent: declA, Provider: declB, Reason: entities.ReasonReference}}
	pairUnits := []entities.Decl{{Key: declA, Unit: "one"}, {Key: declB, Unit: "two"}}

	testCases := []planCase{
		{
			name: "a tight plan is clean", decls: units, edges: needs, base: baseUnits,
			at: map[entities.DeclKey]entities.Position{declA: 2, declB: 2, declC: 2},
		},
		{
			name: "a declaration below its floor", decls: units, edges: needs, base: baseUnits,
			at:        map[entities.DeclKey]entities.Position{declA: 0, declB: 2, declC: 2},
			wantFloor: 1, wantBelow: 1,
		},
		{
			name: "an anchored declaration may sit below its floor", decls: units, edges: needs,
			anchors: pulled, base: baseUnits,
			at: map[entities.DeclKey]entities.Position{declA: 0, declB: 2, declC: 2},
		},
		{
			name: "a dependent ahead of its provider", decls: units, edges: needs, base: baseUnits,
			at:       map[entities.DeclKey]entities.Position{declA: 3, declB: 2, declC: 2},
			wantEdge: 1, wantLoose: 1, wantBelow: 1,
		},
		{
			name: "a declaration later than anything needs", decls: units, edges: needs,
			base:      baseUnits,
			at:        map[entities.DeclKey]entities.Position{declA: 2, declB: 5, declC: 2},
			wantLoose: 1,
		},
		{
			name: "a pair that only justify each other at the limit", decls: pairUnits,
			edges: pair, base: map[entities.DeclKey]entities.Position{declA: 1, declB: 1},
			at:        map[entities.DeclKey]entities.Position{declA: 10, declB: 10},
			wantLimit: 2,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			testCase.check(t)
		})
	}
}

func TestLineagePlanAtomsAndInsertion(t *testing.T) {
	t.Parallel()

	declA, declC, declX, declY := planKey("A"), planKey("C"), planKey("X"), planKey("Y")
	needs := []entities.Edge{{Dependent: declC, Provider: declA, Reason: entities.ReasonReference}}
	atoms := []entities.Decl{
		{Key: declX, Unit: "three", Atom: "gen"}, {Key: declY, Unit: "three", Atom: "gen"},
	}
	inserted := []entities.Decl{{Key: declA, Unit: "one"}, {Key: declC, Unit: "two"}}
	together := []entities.Decl{{Key: declA, Unit: "one"}, {Key: declC, Unit: "one"}}
	baseInserted := map[entities.DeclKey]entities.Position{declA: 1}
	baseAtoms := map[entities.DeclKey]entities.Position{declX: 1, declY: 1}
	lateSibling := map[entities.DeclKey]entities.Position{declX: 1, declY: 4}

	testCases := []planCase{
		{
			name: "an atom member rides its sibling", decls: atoms, base: lateSibling,
			at: map[entities.DeclKey]entities.Position{declX: 4, declY: 4},
		},
		{
			name: "an atom member left behind its sibling", decls: atoms, base: lateSibling,
			at:        map[entities.DeclKey]entities.Position{declX: 1, declY: 4},
			wantBelow: 1,
		},
		{
			name: "an atom nothing holds back", decls: atoms, base: baseAtoms,
			at:        map[entities.DeclKey]entities.Position{declX: 4, declY: 4},
			wantLoose: 2,
		},
		{
			name: "an inserted dependent waits one slot", decls: inserted, edges: needs,
			inserted: map[entities.DeclKey]bool{declC: true}, base: baseInserted,
			at: map[entities.DeclKey]entities.Position{declA: 1, declC: 2},
		},
		{
			name: "an inserted dependent past that slot", decls: together, edges: needs,
			inserted: map[entities.DeclKey]bool{declC: true}, base: baseInserted,
			at:        map[entities.DeclKey]entities.Position{declA: 1, declC: 3},
			wantLoose: 1,
		},
		{
			name: "an inserted dependent at its provider's slot", decls: inserted, edges: needs,
			inserted: map[entities.DeclKey]bool{declC: true}, base: baseInserted,
			at:        map[entities.DeclKey]entities.Position{declA: 1, declC: 1},
			wantBelow: 1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			testCase.check(t)
		})
	}
}

func TestLineagePlanInsertedMatchesSeed(t *testing.T) {
	t.Parallel()

	touched := entities.Snapshot{History: []entities.SourceTouch{
		{Position: 3, Touched: []string{"a.go"}},
	}}

	testCases := []struct {
		name      string
		snap      entities.Snapshot
		file      string
		want      bool
		wantLimit entities.Position
	}{
		{
			name: "a file the history never touched", snap: touched, file: "b.go",
			want: true, wantLimit: 11,
		},
		{
			name: "a file the history touched", snap: touched, file: "a.go",
			want: false, wantLimit: 11,
		},
		{
			name: "no history at all", snap: entities.Snapshot{}, file: "a.go",
			want: false, wantLimit: 8,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := insertedIn(testCase.snap, testCase.file); got != testCase.want {
				t.Errorf("insertedIn() = %t, want %t", got, testCase.want)
			}

			if got := limitOf(testCase.snap, 8); got != testCase.wantLimit {
				t.Errorf("limitOf() = %d, want %d", got, testCase.wantLimit)
			}
		})
	}
}

func TestLineageStageWarnsWhenAnchorsAreIgnored(t *testing.T) {
	fix := `{"anchors": [{"rev": "3d2d883", "kind": "fix"}]}`
	testCases := []struct {
		name, section string
		wantWarn      bool
	}{
		{name: "history mode ignores the anchors and says so", section: fix, wantWarn: true},
		{name: "history mode without anchors says nothing"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var buf bytes.Buffer

			restore := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))

			runAnchorStage(t, anchorStageCase{mode: ModeHistory, section: testCase.section})
			slog.SetDefault(restore)

			warned := strings.Contains(buf.String(), "ignored outside")
			if warned != testCase.wantWarn {
				t.Errorf("warned = %v, want %v", warned, testCase.wantWarn)
			}
		})
	}
}

type runKey struct{}

type ctxRevisions struct{ seen *any }

func (stub ctxRevisions) Revision(ctx context.Context, _ string) (anchoring.Revision, error) {
	*stub.seen = ctx.Value(runKey{})

	return anchoring.Revision{}, errRevisionStop
}

func (ctxRevisions) Blob(context.Context, string) ([]byte, error) { return nil, errRevisionStop }

var errRevisionStop = errors.New("revision stop")

func TestLineageStageBindsTheRunContextToTheAnchors(t *testing.T) {
	t.Parallel()

	testCases := []struct{ name, section string }{
		{name: "the run context", section: `{"anchors": [{"rev": "3d2d883", "kind": "fix"}]}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var seen any

			deps := lineageDeps{
				scan: (&fakeLineageScan{}).scan,
				open: func(context.Context, *Config, stageEnv) (anchoring.Config, closers, error) {
					return anchoring.Config{Revisions: ctxRevisions{seen: &seen}}, nil, nil
				},
			}

			cfg := Defaults()
			cfg.Mode = ModeLineage
			cfg.Sections.Lineage = json.RawMessage(testCase.section)

			env := goAndBareEnv()
			env.opened.snapshot.History = []entities.SourceTouch{{Position: 4, Rev: "3d2d883000"}}
			built := identityStages(nil)
			ctx := context.WithValue(t.Context(), runKey{}, "run")

			if err := lineageStage(ctx, &cfg, env, built, deps); err != nil {
				t.Fatalf("lineageStage() = %v", err)
			}

			_, err := built.planner.Plan(entities.Graph{}, entities.Ordering{})
			if !errors.Is(err, errRevisionStop) {
				t.Fatalf("Plan() = %v, want the revision stop", err)
			}

			if seen != "run" {
				t.Fatalf("the anchors read revisions under %v, want the run context", seen)
			}
		})
	}
}
