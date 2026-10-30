package passes

import (
	"encoding/json"
	"math/rand/v2"
	"reflect"
	"strconv"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping"
)

type shape struct{ atoms, forward, long, apart bool }

type atomRun struct {
	name string
	left int
}

func (run *atomRun) next(rng *rand.Rand, idx int) (member string, isFollower bool) {
	if run.left > 0 {
		run.left--

		return run.name, true
	}

	if rng.IntN(8) != 0 {
		return "", false
	}

	run.name, run.left = "atom"+strconv.Itoa(idx), 1+rng.IntN(2)

	return run.name, false
}

func randomDraft(rng *rand.Rand) (*shaping.Draft, shape) {
	dirs := []string{"pkg/a", "pkg/b", "pkg/c", "pkg/d"}
	reasons := []entities.EdgeReason{
		entities.ReasonReference, entities.ReasonReference, entities.ReasonEncloses,
		entities.ReasonTestOf,
	}

	const apartOdds = 3

	count, rowOdds := 8+rng.IntN(24), 3
	if rng.IntN(5) == 0 {
		count, rowOdds = 70+rng.IntN(30), 2
	}

	decls := make([]entities.Decl, 0, count)

	var (
		edges []entities.Edge
		plan  entities.Plan
		seen  shape
		run   atomRun
	)

	for idx := range count {
		dir := dirs[rng.IntN(len(dirs))]
		isTest := rng.IntN(3) == 0
		key := entities.DeclKey{
			File: dir + "/f.go", Kind: entities.KindFunc, Name: "D" + strconv.Itoa(idx),
		}
		lines := 5 + rng.IntN(100)

		switch {
		case isTest:
			key.File, lines = dir+"/f_test.go", 20+rng.IntN(300)
		case rng.IntN(10) == 0:
			key.File, key.Kind = dir+"/view.ts", entities.KindOther
		}

		member, follower := run.next(rng, idx)
		seen.atoms = seen.atoms || follower

		decls = append(decls, entities.Decl{
			Key: key, Lines: lines, IsTest: isTest, Generated: isTest && rng.IntN(4) == 0,
			Atom: member,
		})

		for range rng.IntN(3) {
			if idx > 0 {
				edges = append(edges, entities.Edge{
					Dependent: key,
					Provider:  decls[rng.IntN(idx)].Key,
					Reason:    reasons[rng.IntN(len(reasons))],
				})
			}
		}

		apart := follower && rng.IntN(apartOdds) == 0
		seen.apart = seen.apart || apart

		if idx == 0 || apart || (!follower && rng.IntN(rowOdds) == 0) {
			plan.Commits = append(plan.Commits, entities.PlannedCommit{
				Position: entities.Position(len(plan.Commits)),
				Subject:  "row " + strconv.Itoa(len(plan.Commits)),
			})
		}

		last := &plan.Commits[len(plan.Commits)-1]
		last.Decls = append(last.Decls, key)
	}

	forward := forwardSoftEdges(rng, decls)
	edges = append(edges, forward...)

	const longPlan = 30

	seen.forward, seen.long = len(forward) > 0, len(plan.Commits) >= longPlan

	return shaping.NewDraft(entities.NewGraph(decls, edges, nil), plan), seen
}

func forwardSoftEdges(rng *rand.Rand, decls []entities.Decl) []entities.Edge {
	soft := []entities.EdgeReason{entities.ReasonTestOf, entities.ReasonImplements}

	out := make([]entities.Edge, 0, 3)

	for range rng.IntN(4) {
		from := rng.IntN(len(decls) - 1)
		to := from + 1 + rng.IntN(len(decls)-from-1)
		out = append(out, entities.Edge{
			Dependent: decls[from].Key, Provider: decls[to].Key, Reason: soft[rng.IntN(len(soft))],
		})
	}

	return out
}

func atomSpread(draft *shaping.Draft) map[string]int {
	slots := map[string]map[int]bool{}

	for index, commit := range draft.Plan().Commits {
		for _, key := range commit.Decls {
			decl, _ := draft.Graph().Decl(key)
			if decl.Atom == "" {
				continue
			}

			if slots[decl.Atom] == nil {
				slots[decl.Atom] = map[int]bool{}
			}

			slots[decl.Atom][index] = true
		}
	}

	spread := make(map[string]int, len(slots))
	for name, held := range slots {
		spread[name] = len(held)
	}

	return spread
}

func movedApartAtom(draft *shaping.Draft, moves shaping.Moves, before map[string]int) bool {
	for _, move := range moves {
		decl, _ := draft.Graph().Decl(move.Decl)
		if decl.Atom != "" && before[decl.Atom] > 1 {
			return true
		}
	}

	return false
}

func checkSeed(t *testing.T, tidy Tidy, pass shaping.Pass, seed uint64, tally map[string]int) {
	t.Helper()

	const seedStream = 7

	draft, seen := randomDraft(rand.New(rand.NewPCG(seed, seedStream)))
	before := atomSpread(draft)

	direct, err := tidy.Apply(t.Context(), draft.Clone())
	if err != nil {
		t.Fatalf("seed %d: direct Apply() = %v", seed, err)
	}

	first, err := pass.Apply(t.Context(), draft)
	if err != nil || len(first) != len(direct) {
		t.Fatalf("seed %d: Apply() = %d moves, %v, want %d", seed, len(first), err, len(direct))
	}

	settled := draft.Plan()

	again, err := pass.Apply(t.Context(), draft)
	if err != nil || len(again) != 0 || !reflect.DeepEqual(settled, draft.Plan()) {
		t.Fatalf("seed %d: second Apply() = %d moves, %v", seed, len(again), err)
	}

	if violations := draft.Violations(); len(violations) != 0 {
		t.Fatalf("seed %d: %d hard violations: %v", seed, len(violations), violations)
	}

	for name, count := range atomSpread(draft) {
		if count > before[name] {
			t.Fatalf("seed %d: atom %s over %d slots, was %d", seed, name, count, before[name])
		}
	}

	shapes := map[string]bool{
		"any": true, "atoms": seen.atoms, "forward": seen.forward, "long": seen.long,
		"apart": seen.apart, "apart-touched": movedApartAtom(draft, first, before),
	}
	for name, hit := range shapes {
		if hit {
			tally[name]++

			if len(first) > 0 {
				tally[name+"+moved"]++
			}
		}
	}
}

func TestTidyHoldsOnGeneratedPlans(t *testing.T) {
	t.Parallel()

	const seedCount = 300

	testCases := []struct {
		name      string
		config    string
		pipelined bool
	}{
		{"defaults", `{"type":"tidy-test-after"}`, false},
		{"no tail window", `{"type":"tidy-test-after","tail_window":0}`, false},
		{"small test threshold", `{"type":"tidy-test-after","min_test_lines":30}`, false},
		{"through the pipeline", `{"type":"tidy-test-after"}`, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tidy, err := NewTidy(json.RawMessage(testCase.config))
			if err != nil {
				t.Fatalf("NewTidy() = %v", err)
			}

			var pass shaping.Pass = tidy
			if testCase.pipelined {
				pass = shaping.NewPipeline(nopObserver{}, tidy)
			}

			tally := map[string]int{}

			for seed := range uint64(seedCount) {
				checkSeed(t, tidy, pass, seed, tally)
			}

			t.Logf("%v", tally)

			least := map[string]int{
				"atoms+moved": 100, "forward+moved": 70, "long+moved": 40,
				"apart-touched+moved": 10,
			}
			for name, floor := range least {
				if tally[name] < floor {
					t.Fatalf("%s seeds = %d, want at least %d", name, tally[name], floor)
				}
			}
		})
	}
}
