package anchoring

import (
	"bytes"
	"context"
	"log/slog"
	"reflect"
	"slices"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type (
	staysAnchor struct {
		rev      string
		position entities.Position
		old, new string
	}

	staysCase struct {
		name       string
		inner      []entities.PlannedCommit
		edges      []entities.Edge
		floor      map[entities.DeclKey]entities.Position
		final      string
		anchors    []staysAnchor
		wantDecls  map[string][]entities.DeclKey
		wantStayed int
	}
)

func staysPlanner(ctrl *gomock.Controller, testCase staysCase, buf *bytes.Buffer) Planner {
	delegate := NewMockDelegate(ctrl)
	delegate.EXPECT().
		Plan(gomock.Any(), gomock.Any()).
		Return(entities.Plan{Commits: testCase.inner}, nil).
		AnyTimes()

	blobs := map[string]string{}
	revisions := map[string]Revision{}
	anchors := make([]Anchor, 0, len(testCase.anchors))

	for _, one := range testCase.anchors {
		blobs[one.rev+"o"], blobs[one.rev+"n"] = one.old, one.new
		revisions[one.rev] = Revision{
			Subject: "fix(golang): " + one.rev,
			Files: []FileChange{
				{Path: "a.go", OldPath: "a.go", Blob: one.rev + "n", OldBlob: one.rev + "o"},
			},
		}

		anchors = append(anchors, Anchor{Rev: one.rev, Position: one.position})
	}

	mock := mapBlobs(ctrl, blobs)
	mock.EXPECT().
		Revision(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, rev string) (Revision, error) {
			return revisions[rev], nil
		}).
		AnyTimes()

	probe := NewMockProbe(ctrl)
	probe.EXPECT().Builds(gomock.Any(), gomock.Any()).Return(Verdict{IsBuilt: true}, nil).AnyTimes()

	return New(Config{
		Inner:     delegate,
		Anchors:   anchors,
		Floor:     testCase.floor,
		Languages: goLanguages(),
		Revisions: mock,
		Finals:    mapFinals(ctrl, map[string]string{"a.go": testCase.final}),
		Probe:     probe,
		Logger:    slog.New(slog.NewJSONHandler(buf, nil)),
	})
}

func TestPlanScopesWhatHoldsABornDeclaration(t *testing.T) {
	t.Parallel()

	commitA := entities.PlannedCommit{Subject: "feat: A", Decls: []entities.DeclKey{fkey("A")}}
	commitB := entities.PlannedCommit{Subject: "feat: B", Decls: []entities.DeclKey{fkey("B")}}
	commitH := entities.PlannedCommit{Subject: "feat: H", Decls: []entities.DeclKey{fkey("H")}}
	commitI := entities.PlannedCommit{Subject: "feat: I", Decls: []entities.DeclKey{fkey("I")}}
	commitX := entities.PlannedCommit{Subject: "feat: X", Decls: []entities.DeclKey{fkey("X")}}

	testCases := []staysCase{
		{
			name:  "a newer anchor's pre-image on a declaration placed after the slot holds nothing",
			inner: []entities.PlannedCommit{commitA, commitH, commitX},
			floor: map[entities.DeclKey]entities.Position{fkey("A"): 1, fkey("H"): 2, fkey("X"): 9},
			final: "A last\nH help\nX more\n",
			anchors: []staysAnchor{
				{rev: "o", position: 5, old: "A old\n", new: "A new\nH help\n"},
				{rev: "n", position: 9, old: "X pre H\n", new: "X more\n"},
			},
			wantDecls: map[string][]entities.DeclKey{"o": {fkey("H")}},
		},
		{
			name:  "the declaration's own pre-image does not hold it",
			inner: []entities.PlannedCommit{commitA, commitH, commitB},
			floor: map[entities.DeclKey]entities.Position{fkey("A"): 1, fkey("H"): 2, fkey("B"): 9},
			final: "A last\nH help\nB other\n",
			anchors: []staysAnchor{
				{rev: "o", position: 5, old: "A old\n", new: "A new\nH help\n"},
				{rev: "n", position: 9, old: "H spin H\n", new: "H help\n"},
			},
			wantDecls: map[string][]entities.DeclKey{"o": {fkey("H")}},
		},
		{
			name: "a moving declaration's pre-image does not hold the others",
			inner: []entities.PlannedCommit{
				commitA, commitH, commitI, commitB,
			},
			floor: map[entities.DeclKey]entities.Position{
				fkey("A"): 1, fkey("H"): 2, fkey("I"): 2, fkey("B"): 9,
			},
			final: "A last\nH help\nI aid\nB other\n",
			anchors: []staysAnchor{
				{rev: "o", position: 5, old: "A old\n", new: "A new\nH help\nI aid\n"},
				{rev: "n", position: 9, old: "I old H\n", new: "I aid\n"},
			},
			wantDecls: map[string][]entities.DeclKey{"o": {fkey("H"), fkey("I")}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assertStays(t, testCase)
		})
	}
}

func TestPlanFreesWhatANewerAnchorOrItsReceiverMoves(t *testing.T) {
	t.Parallel()

	commitA := entities.PlannedCommit{Subject: "feat: A", Decls: []entities.DeclKey{fkey("A")}}
	commitB := entities.PlannedCommit{Subject: "feat: B", Decls: []entities.DeclKey{fkey("B")}}
	commitC := entities.PlannedCommit{Subject: "feat: C", Decls: []entities.DeclKey{fkey("C")}}
	commitH := entities.PlannedCommit{Subject: "feat: H", Decls: []entities.DeclKey{fkey("H")}}
	commitT := entities.PlannedCommit{Subject: "feat: T", Decls: []entities.DeclKey{fkey("T")}}
	commitX := entities.PlannedCommit{Subject: "feat: X", Decls: []entities.DeclKey{fkey("X")}}
	commitTA := entities.PlannedCommit{
		Subject: "feat: T.Anchors", Decls: []entities.DeclKey{fkey("T.Anchors")},
	}

	testCases := []staysCase{
		{
			name: "a method moves with its receiver type past an unrelated mention of its name",
			inner: []entities.PlannedCommit{
				commitA, commitX, commitT, commitTA, commitB,
			},
			floor: map[entities.DeclKey]entities.Position{
				fkey("A"): 1, fkey("X"): 3, fkey("T"): 2, fkey("T.Anchors"): 2, fkey("B"): 9,
			},
			final: "A last\nX call\nT struct\nT.Anchors body\nB other\n",
			anchors: []staysAnchor{
				{
					rev: "o", position: 5, old: "A old\n",
					new: "A new\nT struct\nT.Anchors body\n",
				},
				{rev: "n", position: 9, old: "X calls Anchors\n", new: "X call\n"},
			},
			wantDecls: map[string][]entities.DeclKey{"o": {fkey("T"), fkey("T.Anchors")}},
		},
		{
			name:  "a consumer a newer anchor moves later no longer holds the provider",
			inner: []entities.PlannedCommit{commitA, commitC, commitH, commitX},
			edges: []entities.Edge{needs("C", "H")},
			floor: map[entities.DeclKey]entities.Position{
				fkey("A"): 1, fkey("C"): 1, fkey("H"): 2, fkey("X"): 9,
			},
			final: "A last\nC more\nH help\nX other\n",
			anchors: []staysAnchor{
				{rev: "o", position: 5, old: "A old\n", new: "A new\nH help\n"},
				{rev: "n", position: 9, old: "X other\n", new: "X other\nC more\n"},
			},
			wantDecls: map[string][]entities.DeclKey{
				"o": {fkey("H")}, "n": {fkey("C")},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assertStays(t, testCase)
		})
	}
}

func assertStays(t *testing.T, testCase staysCase) {
	t.Helper()

	var buf bytes.Buffer

	planner := staysPlanner(gomock.NewController(t), testCase, &buf)
	decls := graphOf(map[string]string{"a.go": testCase.final}).Decls()

	got, err := planner.Plan(
		entities.NewGraph(decls, testCase.edges, nil), entities.Ordering{},
	)
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	carried := map[string][]entities.DeclKey{}

	for _, commit := range got.Commits {
		for rev := range testCase.wantDecls {
			if commit.Subject == "fix(golang): "+rev {
				carried[rev] = commit.Decls
			}
		}
	}

	stayed, summed, moved := 0, 0, 0

	for _, fields := range logged(t, &buf) {
		if fields["msg"] == "anchors: born declaration stays" {
			stayed++
		}

		if fields["msg"] == "anchors: summary" {
			summed += int(fields["stayed"].(float64))
			moved += int(fields["moved"].(float64))
		}
	}

	wantMoved := 0
	for _, keys := range testCase.wantDecls {
		wantMoved += len(keys)
	}

	if summed != stayed || moved != wantMoved {
		t.Fatalf("summaries stayed %d moved %d, want %d and %d", summed, moved, stayed, wantMoved)
	}

	if !reflect.DeepEqual(carried, testCase.wantDecls) || stayed != testCase.wantStayed {
		t.Fatalf("anchor decls %v stayed %d, want %v %d",
			carried, stayed, testCase.wantDecls, testCase.wantStayed)
	}
}

func TestPlanKeepsWhatTheAnchorNeeds(t *testing.T) {
	t.Parallel()

	fix := bornFix
	commitA := entities.PlannedCommit{Subject: "feat: A", Decls: []entities.DeclKey{fkey("A")}}
	commitB := entities.PlannedCommit{Subject: "feat: B", Decls: []entities.DeclKey{fkey("B")}}
	commitC := entities.PlannedCommit{Subject: "feat: C", Decls: []entities.DeclKey{fkey("C")}}
	commitH := entities.PlannedCommit{Subject: "feat: H", Decls: []entities.DeclKey{fkey("H")}}
	commitI := entities.PlannedCommit{Subject: "feat: I", Decls: []entities.DeclKey{fkey("I")}}

	testCases := []bornCase{
		{
			name:         "a helper whose provider is born after the anchor stays",
			inner:        []entities.PlannedCommit{commitA, commitB, commitH, commitI},
			edges:        []entities.Edge{needs("H", "B")},
			wantSubjects: []string{"feat: A", fix, "feat: B", "feat: H"},
			wantDecls:    []entities.DeclKey{fkey("I")},
			wantStayed:   1,
		},
		{
			name:         "a helper that needs a staying helper stays too",
			inner:        []entities.PlannedCommit{commitA, commitB, commitH, commitI},
			edges:        []entities.Edge{needs("H", "B"), needs("I", "H")},
			wantSubjects: []string{"feat: A", fix, "feat: B", "feat: H", "feat: I"},
			wantStayed:   2,
		},
		{
			name:         "a helper only a pre-image reads stays where it was born",
			inner:        []entities.PlannedCommit{commitH, commitA, commitI, commitB},
			preimage:     "A old H\n",
			wantSubjects: []string{"feat: H", "feat: A", fix, "feat: B"},
			wantDecls:    []entities.DeclKey{fkey("I")},
			wantStayed:   1,
		},
		{
			name:         "a helper an older commit needs stays where it was born",
			inner:        []entities.PlannedCommit{commitA, commitH, commitC, commitI, commitB},
			edges:        []entities.Edge{needs("C", "H")},
			wantSubjects: []string{"feat: A", "feat: H", "feat: C", fix, "feat: B"},
			wantDecls:    []entities.DeclKey{fkey("I")},
			wantStayed:   1,
		},
		{
			name:         "atom mates move together or all stay",
			inner:        []entities.PlannedCommit{commitA, commitB, commitH, commitI},
			edges:        []entities.Edge{needs("H", "B")},
			atomed:       []string{"H", "I"},
			wantSubjects: []string{"feat: A", fix, "feat: B", "feat: H", "feat: I"},
			wantStayed:   2,
		},
		{
			name:         "atom mates move when nothing holds them",
			inner:        []entities.PlannedCommit{commitA, commitB, commitH, commitI},
			atomed:       []string{"H", "I"},
			wantSubjects: []string{"feat: A", fix, "feat: B"},
			wantDecls:    []entities.DeclKey{fkey("H"), fkey("I")},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assertBorn(t, testCase)
		})
	}
}

func TestHoldersBefore(t *testing.T) {
	t.Parallel()

	chosen := []kept{
		{found: found{entries: []entry{{key: fkey("A"), preimage: "a1"}}}},
		{found: found{entries: []entry{
			{key: fkey("A"), preimage: "a2"}, {key: fkey("B"), preimage: "b2"},
		}}},
		{found: found{entries: []entry{
			{key: fkey("B"), preimage: "b3"}, {key: fkey("C"), preimage: "c3"},
		}}},
	}

	testCases := []struct {
		name string
		at   int
		want []string
	}{
		{
			name: "the oldest anchor sees the first newer entry of each key", at: 0,
			want: []string{"a1", "b2", "c3"},
		},
		{
			name: "the middle anchor keeps its own and the older ones", at: 1,
			want: []string{"a1", "a2", "b2", "c3"},
		},
		{
			name: "the newest anchor sees every entry", at: 2,
			want: []string{"a1", "a2", "b2", "b3", "c3"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var got []string
			for _, holder := range holdersBefore(chosen, testCase.at) {
				got = append(got, holder.preimage)
			}

			if !slices.Equal(got, testCase.want) {
				t.Fatalf("holdersBefore(%d) = %v, want %v", testCase.at, got, testCase.want)
			}
		})
	}
}

func TestPlanHoldsAChainAndLoosensWhatOnlyFillsAnInterface(t *testing.T) {
	t.Parallel()

	fix := bornFix
	commitA := entities.PlannedCommit{Subject: "feat: A", Decls: []entities.DeclKey{fkey("A")}}
	commitB := entities.PlannedCommit{Subject: "feat: B", Decls: []entities.DeclKey{fkey("B")}}
	commitC := entities.PlannedCommit{Subject: "feat: C", Decls: []entities.DeclKey{fkey("C")}}
	commitH := entities.PlannedCommit{Subject: "feat: H", Decls: []entities.DeclKey{fkey("H")}}
	commitI := entities.PlannedCommit{Subject: "feat: I", Decls: []entities.DeclKey{fkey("I")}}

	testCases := []bornCase{
		{
			name:         "a helper whose provider stays only on the second pass stays too",
			inner:        []entities.PlannedCommit{commitA, commitB, commitH, commitI},
			edges:        []entities.Edge{needs("H", "I"), needs("I", "B")},
			wantSubjects: []string{"feat: A", fix, "feat: B", "feat: H", "feat: I"},
			wantStayed:   2,
		},
		{
			name:  "a provider that is an interface the helper only fills does not hold it",
			inner: []entities.PlannedCommit{commitA, commitB, commitH, commitI},
			edges: []entities.Edge{
				{Dependent: fkey("H"), Provider: fkey("B"), Reason: entities.ReasonImplements},
			},
			interfaces:   []string{"B"},
			wantSubjects: []string{"feat: A", fix, "feat: B"},
			wantDecls:    []entities.DeclKey{fkey("H"), fkey("I")},
		},
		{
			name:  "an earlier consumer that only fills the interface does not hold it",
			inner: []entities.PlannedCommit{commitA, commitH, commitC, commitI, commitB},
			edges: []entities.Edge{
				{Dependent: fkey("C"), Provider: fkey("H"), Reason: entities.ReasonImplements},
			},
			interfaces:   []string{"H"},
			wantSubjects: []string{"feat: A", "feat: C", fix, "feat: B"},
			wantDecls:    []entities.DeclKey{fkey("H"), fkey("I")},
		},
		{
			name:  "an implements edge to a type that is not an interface still holds it",
			inner: []entities.PlannedCommit{commitA, commitB, commitH, commitI},
			edges: []entities.Edge{
				{Dependent: fkey("H"), Provider: fkey("B"), Reason: entities.ReasonImplements},
			},
			wantSubjects: []string{"feat: A", fix, "feat: B", "feat: H"},
			wantDecls:    []entities.DeclKey{fkey("I")},
			wantStayed:   1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assertBorn(t, testCase)
		})
	}
}

func TestPlanHoldsByTheAnchorAConsumerBelongsTo(t *testing.T) {
	t.Parallel()

	commitA := entities.PlannedCommit{Subject: "feat: A", Decls: []entities.DeclKey{fkey("A")}}
	commitC := entities.PlannedCommit{Subject: "feat: C", Decls: []entities.DeclKey{fkey("C")}}
	commitH := entities.PlannedCommit{Subject: "feat: H", Decls: []entities.DeclKey{fkey("H")}}
	commitX := entities.PlannedCommit{Subject: "feat: X", Decls: []entities.DeclKey{fkey("X")}}

	testCases := []staysCase{
		{
			name:  "a consumer that is an entry of a newer anchor does not hold the provider",
			inner: []entities.PlannedCommit{commitA, commitC, commitH, commitX},
			edges: []entities.Edge{needs("C", "H")},
			floor: map[entities.DeclKey]entities.Position{
				fkey("A"): 1, fkey("C"): 1, fkey("H"): 2, fkey("X"): 9,
			},
			final: "A last\nC more\nH help\nX other\n",
			anchors: []staysAnchor{
				{rev: "o", position: 5, old: "A old\n", new: "A new\nH help\n"},
				{rev: "n", position: 9, old: "C pre\n", new: "C more\n"},
			},
			wantDecls: map[string][]entities.DeclKey{"o": {fkey("H")}},
		},
		{
			name:  "a consumer that is an entry of an older anchor still holds the provider",
			inner: []entities.PlannedCommit{commitA, commitC, commitH, commitX},
			edges: []entities.Edge{needs("C", "H")},
			floor: map[entities.DeclKey]entities.Position{
				fkey("A"): 1, fkey("C"): 1, fkey("H"): 2, fkey("X"): 9,
			},
			final: "A last\nC more\nH help\nX other\n",
			anchors: []staysAnchor{
				{rev: "o1", position: 3, old: "C pre\n", new: "C more\n"},
				{rev: "o2", position: 5, old: "A old\n", new: "A new\nH help\n"},
			},
			wantDecls:  map[string][]entities.DeclKey{"o2": nil},
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
