package anchoring

import (
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func TestSlot(t *testing.T) {
	t.Parallel()

	floor := map[entities.DeclKey]entities.Position{
		fkey("A"): 1, fkey("B"): 4, fkey("C"): 6, fkey("D"): 9,
	}
	commits := []entities.PlannedCommit{
		{Position: 1, Decls: []entities.DeclKey{fkey("A")}},
		{Position: 2, Decls: []entities.DeclKey{fkey("B")}},
		{Position: 3, Decls: []entities.DeclKey{fkey("E"), fkey("C")}},
		{Position: 4, Decls: []entities.DeclKey{fkey("D")}},
	}
	bornC := []entities.DeclKey{fkey("C")}
	bornA := []entities.DeclKey{fkey("A")}

	testCases := []struct {
		name  string
		query slotQuery
		want  int
	}{
		{
			name:  "before the first commit reaching the anchor",
			query: slotQuery{at: 5}, want: 2,
		},
		{
			name:  "the max floor of a commit decides, an unfloored decl is ignored",
			query: slotQuery{at: 6}, want: 2,
		},
		{name: "an anchor older than every commit goes first", query: slotQuery{at: 1}, want: 0},
		{name: "an anchor newer than every commit goes last", query: slotQuery{at: 50}, want: 4},
		{
			name:  "clamped after the commit that births what it changed",
			query: slotQuery{at: 5, born: bornC}, want: 3,
		},
		{
			name:  "a birth before the natural slot leaves it alone",
			query: slotQuery{at: 5, born: bornA}, want: 2,
		},
		{
			name:  "not before the slot an earlier anchor took",
			query: slotQuery{at: 1, notBefore: 3}, want: 3,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			planner := New(Config{Floor: floor})

			if got := planner.slot(commits, testCase.query); got != testCase.want {
				t.Fatalf("slot() = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestMarkCarriers(t *testing.T) {
	t.Parallel()

	onlyA, onlyB, both := []entities.DeclKey{fkey("A")}, []entities.DeclKey{fkey("B")},
		[]entities.DeclKey{fkey("B"), fkey("A")}
	commits := []entities.PlannedCommit{
		{Position: 0, Subject: "feat: A", Decls: onlyA},
		{Position: 1, Subject: "feat: B", Decls: onlyB},
		{Position: 2, Subject: "fix: A", Changes: onlyA},
		{Position: 3, Subject: "feat: C", Decls: []entities.DeclKey{fkey("C")}},
	}

	testCases := []struct {
		name string
		slot int
		keys []entities.DeclKey
		want []int
	}{
		{
			name: "from the birth of the key up to the slot",
			slot: 3, keys: onlyB, want: []int{1, 2},
		},
		{
			name: "an earlier anchor commit between them carries it too",
			slot: 4, keys: onlyA, want: []int{0, 1, 2, 3},
		},
		{
			name: "the earliest birth of several keys starts the run",
			slot: 2, keys: both, want: []int{0, 1},
		},
		{name: "a key born at the slot marks nothing", slot: 1, keys: onlyB},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			planned := slices.Clone(commits)
			count := markCarriers(planned, testCase.slot, testCase.keys)

			var got []int

			for idx, commit := range planned {
				if commit.IsWIP {
					got = append(got, idx)
				}
			}

			if !slices.Equal(got, testCase.want) || count != len(testCase.want) {
				t.Fatalf("marked = %v (%d), want %v", got, count, testCase.want)
			}
		})
	}
}

func TestSlotSkipsCommitsWithoutDecls(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		floor   map[entities.DeclKey]entities.Position
		commits []entities.PlannedCommit
		query   slotQuery
		want    int
	}{
		{
			name:  "a commit that only changes is not a slot candidate",
			floor: map[entities.DeclKey]entities.Position{fkey("A"): 1, fkey("B"): 8},
			commits: []entities.PlannedCommit{
				{Position: 1, Decls: []entities.DeclKey{fkey("A")}},
				{Position: 2, Changes: []entities.DeclKey{fkey("A")}},
				{Position: 3, Decls: []entities.DeclKey{fkey("B")}},
			},
			query: slotQuery{at: 5},
			want:  2,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			planner := New(Config{Floor: testCase.floor})

			if got := planner.slot(testCase.commits, testCase.query); got != testCase.want {
				t.Fatalf("slot() = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestPrune(t *testing.T) {
	t.Parallel()

	held := []entities.DeclKey{fkey("A")}
	testCases := []struct {
		name      string
		commits   []entities.PlannedCommit
		slots     []int
		wantCount int
		wantSlots []int
	}{
		{
			name:      "nothing empty leaves everything",
			commits:   []entities.PlannedCommit{{Decls: held}, {Changes: held}},
			slots:     []int{1},
			wantCount: 2, wantSlots: []int{1},
		},
		{
			name:      "an emptied commit before a slot shifts it back",
			commits:   []entities.PlannedCommit{{Decls: held}, {}, {Changes: held}, {}},
			slots:     []int{2},
			wantCount: 2, wantSlots: []int{1},
		},
		{
			name:      "a slot commit left empty is gone",
			commits:   []entities.PlannedCommit{{Decls: held}, {}, {Changes: held}},
			slots:     []int{1, 2},
			wantCount: 2, wantSlots: []int{-1, 1},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, slots := prune(testCase.commits, testCase.slots)
			if len(got) != testCase.wantCount || !slices.Equal(slots, testCase.wantSlots) {
				t.Fatalf("prune() = %d commits slots %v, want %d %v",
					len(got), slots, testCase.wantCount, testCase.wantSlots)
			}
		})
	}
}

func TestLiveEntries(t *testing.T) {
	t.Parallel()

	same := entry{key: fkey("A"), preimage: "A one"}
	other := entry{key: fkey("A"), preimage: "A two"}
	second := entry{key: fkey("B"), preimage: "B one"}
	spaced := entry{key: fkey("A"), preimage: "A  one\n"}
	testCases := []struct {
		name   string
		chosen []kept
		want   []int
	}{
		{
			name: "distinct pre-images all live",
			chosen: []kept{
				{found: found{entries: []entry{same}}},
				{found: found{entries: []entry{other}}},
			},
			want: []int{1, 1},
		},
		{
			name: "a pre-image equal to the next one is dead",
			chosen: []kept{
				{found: found{entries: []entry{same}}},
				{found: found{entries: []entry{same}}},
			},
			want: []int{0, 1},
		},
		{
			name: "only the dead entry of an anchor goes",
			chosen: []kept{
				{found: found{entries: []entry{same, second}}},
				{found: found{entries: []entry{same}}},
			},
			want: []int{1, 1},
		},
		{
			name: "a pre-image equal to the next one but for whitespace is dead",
			chosen: []kept{
				{found: found{entries: []entry{same}}},
				{found: found{entries: []entry{spaced}}},
			},
			want: []int{0, 1},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var counts []int
			for _, live := range liveEntries(testCase.chosen) {
				counts = append(counts, len(live))
			}

			if !slices.Equal(counts, testCase.want) {
				t.Fatalf("live entries per anchor = %v, want %v", counts, testCase.want)
			}
		})
	}
}

func TestPlanKeepsWhatAnOlderAnchorChanges(t *testing.T) {
	t.Parallel()

	testCases := []staysCase{
		{
			name: "a declaration an older anchor changes stays when re-added",
			inner: []entities.PlannedCommit{
				plannedCommit("A"), plannedCommit("K"), plannedCommit("B"),
			},
			floor: map[entities.DeclKey]entities.Position{fkey("A"): 1, fkey("K"): 2, fkey("B"): 9},
			final: "A last\nK help\nB other\n",
			anchors: []staysAnchor{
				{rev: "o", position: 3, old: "K v1\n", new: "K v2\n"},
				{rev: "n", position: 9, old: "A old\n", new: "A new\nK v2\n"},
			},
			wantDecls:  map[string][]entities.DeclKey{"n": nil},
			wantStayed: 1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assertStays(t, testCase)
		})
	}
}

func TestPlanJudgesSeveralAnchorsApart(t *testing.T) {
	t.Parallel()

	both := staysCase{
		inner: []entities.PlannedCommit{
			plannedCommit("A"), plannedCommit("B"), plannedCommit("C"), plannedCommit("D"),
		},
		floor: map[entities.DeclKey]entities.Position{
			fkey("A"): 1, fkey("B"): 2, fkey("C"): 3, fkey("D"): 4,
		},
		final: "A last\nB bb\nC cc\nD dd\n",
		anchors: []staysAnchor{
			{
				rev: "x", position: 5,
				old: "A reads B end\nB bb\nC cc\nD dd\n", new: "A last\nB bb\nC cc\nD dd\n",
			},
			{
				rev: "y", position: 6,
				old: "A last\nB bb\nC reads D end\nD dd\n", new: "A last\nB bb\nC cc\nD dd\n",
			},
		},
	}
	twice := staysCase{
		inner: []entities.PlannedCommit{
			plannedCommit("K"), plannedCommit("N"), plannedCommit("B"),
		},
		floor: map[entities.DeclKey]entities.Position{fkey("K"): 1, fkey("N"): 2, fkey("B"): 9},
		final: "K last\nN note\nB other\n",
		anchors: []staysAnchor{
			{
				rev: "a1", position: 5,
				old: "K v0\nN note\nB other\n", new: "K v1\nN note\nB other\n",
			},
			{
				rev: "a2", position: 9,
				old: "K v1 reads N end\nN note\nB other\n", new: "K last\nN note\nB other\n",
			},
		},
	}

	testCases := []readsCase{
		{
			name: "two bad anchors fold apart", stays: both,
			want: map[string]readsWant{
				"x": {outcome: "folded", reason: "a.go:func:B"},
				"y": {outcome: "folded", reason: "a.go:func:D"},
			},
		},
		{
			name: "two bad anchors under wip are both marked", stays: both, policy: PolicyWIP,
			want: map[string]readsWant{
				"x": {outcome: "wip", reason: "a.go:func:B"},
				"y": {outcome: "wip", reason: "a.go:func:D"},
			},
		},
		{
			name: "a second pre-image is active from the slot of the first", stays: twice,
			want: map[string]readsWant{
				"a1": {outcome: "kept", reason: "its pre-image builds"},
				"a2": {outcome: "kept", reason: "its pre-image builds"},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assertReads(t, testCase)
		})
	}
}
