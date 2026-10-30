//go:build integration

package bootstrap

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/lineage"
	"github.com/wrapped-owls/redigit/internal/infra/parity"
)

type lineageRun struct {
	result     lineage.Result
	order      entities.Ordering
	birth      map[entities.DeclKey]entities.Position
	firstTouch map[entities.DeclKey]entities.Position
	graph      entities.Graph
	base       map[entities.DeclKey]entities.Position
	inserted   map[entities.DeclKey]bool
	limit      entities.Position
}

func TestLineageOrdersARealRepository(t *testing.T) {
	repo, rev := os.Getenv("REDIGIT_LINEAGE_REPO"), os.Getenv("REDIGIT_LINEAGE_REV")
	if repo == "" || rev == "" {
		t.Skip("REDIGIT_LINEAGE_REPO and REDIGIT_LINEAGE_REV are not set")
	}

	const minimumChanged = 100

	first := runLineage(t, repo, rev, ModeLineage)
	second := runLineage(t, repo, rev, ModeLineage)
	history := runLineage(t, repo, rev, ModeHistory)

	if !maps.Equal(first.order.At, second.order.At) {
		t.Fatal("two lineage runs placed the declarations differently")
	}

	t.Logf("tau(first touch, history plan) = %.4f", tauOf(first.firstTouch, history.order.At))
	t.Logf("tau(first touch, lineage plan) = %.4f", tauOf(first.firstTouch, first.order.At))
	t.Logf("tau(birth, history plan) = %.4f", tauOf(first.birth, history.order.At))

	t.Logf("moved later than birth, lineage plan: %s", displaced(first.birth, first.order.At))
	t.Logf("moved later than birth, history plan: %s", displaced(first.birth, history.order.At))
	t.Logf("moved later than first touch, history plan: %s",
		displaced(first.firstTouch, history.order.At))
	t.Logf("birth later than first touch: %s", displaced(first.firstTouch, first.birth))
	t.Logf("renamed %s", renamed(first.result))

	t.Logf("tau(birth, lineage plan) = %.4f", tauOf(first.birth, first.order.At))

	plan := first.plan()
	for name, found := range map[string][]string{
		"below its floor":       plan.belowFloor(),
		"ahead of its provider": plan.edgeBreaks(),
		"later than required":   plan.looseDecls(),
		"earlier than required": plan.belowRequired(),
		"unpinned at the limit": plan.unpinnedAtLimit(),
	} {
		if len(found) > 0 {
			t.Errorf("%d declaration(s) %s, first: %v",
				len(found), name, found[:min(len(found), 5)])
		}
	}

	changed := 0

	for key, at := range first.order.At {
		if history.order.At[key] != at {
			changed++
		}
	}

	t.Logf("lineage differs from history on %d of %d declarations", changed, len(first.order.At))

	if changed < minimumChanged {
		t.Errorf("only %d declaration(s) differ from the history plan, want at least %d",
			changed, minimumChanged)
	}
}

func (run lineageRun) plan() lineagePlan {
	return lineagePlan{
		graph: run.graph, floor: lineage.Floor(run.result), base: run.base,
		inserted: run.inserted, at: run.order.At, limit: run.limit,
	}
}

func renamed(result lineage.Result) string {
	byName, byText := 0, 0

	for _, birth := range result.Births {
		if len(birth.Via) == 0 {
			continue
		}

		if slices.ContainsFunc(birth.Via, lineage.Rename.ByText) {
			byText++
		} else {
			byName++
		}
	}

	return fmt.Sprintf("by name only: %d, through a text match: %d", byName, byText)
}

func displaced(floor, plan map[entities.DeclKey]entities.Position) string {
	var shifts []int

	for key, at := range floor {
		if plan[key] > at {
			shifts = append(shifts, int(plan[key]-at))
		}
	}

	slices.Sort(shifts)

	median := 0
	if len(shifts) > 0 {
		median = shifts[len(shifts)/2]
	}

	return fmt.Sprintf("%d of %d, median shift %d", len(shifts), len(floor), median)
}

func spread(positions map[entities.DeclKey]entities.Position) string {
	counts := map[entities.Position]int{}
	highest := entities.Position(0)

	for _, at := range positions {
		counts[at]++
		highest = max(highest, at)
	}

	return fmt.Sprintf("%d distinct, %d at the highest (%d)", len(counts), counts[highest], highest)
}

func tauOf(floor, plan map[entities.DeclKey]entities.Position) float64 {
	return parity.KendallTau(parityRows(floor), parityRows(plan)).Tau
}

func parityRows(positions map[entities.DeclKey]entities.Position) []parity.Row {
	rows := make([]parity.Row, 0, len(positions))

	for key, at := range positions {
		rows = append(rows, parity.Row{
			Position: int(at), Chunks: []parity.Chunk{{Key: key.String()}},
		})
	}

	return rows
}

func runLineage(t *testing.T, repo, rev, mode string) lineageRun {
	t.Helper()

	cfg := Defaults()
	cfg.Repo, cfg.Rev, cfg.Mode = repo, rev, mode
	cfg.BuildTags = []string{"integration"}
	cfg.Ordering.Slack = 8

	pipeline, err := Start(t.Context(), cfg)
	if err != nil {
		t.Fatalf("Start() = %v", err)
	}

	t.Cleanup(func() {
		if closeErr := pipeline.Close(context.WithoutCancel(t.Context())); closeErr != nil {
			t.Errorf("Close() = %v", closeErr)
		}
	})

	indexed, err := pipeline.Graph(t.Context())
	if err != nil {
		t.Fatalf("Graph() = %v", err)
	}

	order, err := pipeline.Order(indexed.Graph)
	if err != nil {
		t.Fatalf("Order() = %v", err)
	}

	t.Logf("%s order: rounds %d, positions %s", mode, order.Rounds, spread(order.At))

	floored, isFloored := pipeline.orderer.(lineageOrderer)

	run := lineageRun{
		result:     floored.result,
		order:      order,
		birth:      map[entities.DeclKey]entities.Position{},
		firstTouch: map[entities.DeclKey]entities.Position{},
		graph:      indexed.Graph,
		base:       map[entities.DeclKey]entities.Position{},
		inserted:   map[entities.DeclKey]bool{},
		limit:      limitOf(pipeline.Snapshot(), cfg.Ordering.Slack),
	}

	if isFloored != (mode == ModeLineage) {
		t.Fatalf("orderer is %T in %s mode", pipeline.orderer, mode)
	}

	for _, decl := range indexed.Graph.Decls() {
		if touch := pipeline.Snapshot().FirstTouch(decl.Key.File); touch.IsSet() {
			run.firstTouch[decl.Key] = touch
		}

		birth, born := floored.result.Births[decl.Key]
		if born {
			run.birth[decl.Key] = birth.At
			run.base[decl.Key] = birth.At

			continue
		}

		run.base[decl.Key] = run.firstTouch[decl.Key]
		run.inserted[decl.Key] = insertedIn(pipeline.Snapshot(), decl.Key.File)
	}

	return run
}
