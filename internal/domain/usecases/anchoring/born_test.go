package anchoring

import (
	"bytes"
	"cmp"
	"log/slog"
	"reflect"
	"slices"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func bornPlanner(
	ctrl *gomock.Controller,
	inner entities.Plan,
	preimage string,
	buf *bytes.Buffer,
) Planner {
	delegate := NewMockDelegate(ctrl)
	delegate.EXPECT().Plan(gomock.Any(), gomock.Any()).Return(inner, nil).AnyTimes()

	files := map[string]string{"a.go": bornFinal}
	revisions := mapBlobs(ctrl, map[string]string{"o": preimage, "n": "A new\nH help\nI aid\n"})
	revisions.EXPECT().
		Revision(gomock.Any(), "fix").
		Return(Revision{
			Subject: "fix(golang): repair A",
			Files:   []FileChange{{Path: "a.go", OldPath: "a.go", Blob: "n", OldBlob: "o"}},
		}, nil).
		AnyTimes()

	probe := NewMockProbe(ctrl)
	probe.EXPECT().Builds(gomock.Any(), gomock.Any()).Return(Verdict{IsBuilt: true}, nil).AnyTimes()

	return New(Config{
		Inner:   delegate,
		Anchors: []Anchor{{Rev: "fix", Position: 5}},
		Floor: map[entities.DeclKey]entities.Position{
			fkey("A"): 1, fkey("B"): 9, fkey("H"): 2, fkey("I"): 2, fkey("C"): 1,
		},
		Languages: goLanguages(),
		Revisions: revisions,
		Finals:    mapFinals(ctrl, files),
		Probe:     probe,
		Logger:    slog.New(slog.NewJSONHandler(buf, nil)),
	})
}

func needs(dependent, provider string) entities.Edge {
	return entities.Edge{
		Dependent: fkey(dependent), Provider: fkey(provider), Reason: entities.ReasonReference,
	}
}

const bornFinal = "A last\nB other\nH help\nI aid\nC more\n"

const bornFix = "fix(golang): repair A"

type bornCase struct {
	name         string
	inner        []entities.PlannedCommit
	edges        []entities.Edge
	preimage     string
	atomed       []string
	interfaces   []string
	wantSubjects []string
	wantDecls    []entities.DeclKey
	wantStayed   int
}

func TestPlanMovesWhatTheAnchorBorn(t *testing.T) {
	t.Parallel()

	fix := bornFix
	commitA := entities.PlannedCommit{Subject: "feat: A", Decls: []entities.DeclKey{fkey("A")}}
	commitB := entities.PlannedCommit{Subject: "feat: B", Decls: []entities.DeclKey{fkey("B")}}
	commitH := entities.PlannedCommit{Subject: "feat: H", Decls: []entities.DeclKey{fkey("H")}}
	commitI := entities.PlannedCommit{Subject: "feat: I", Decls: []entities.DeclKey{fkey("I")}}
	commitHIC := entities.PlannedCommit{
		Subject: "DRAFT: feat(a): H, I, C",
		Decls:   []entities.DeclKey{fkey("H"), fkey("I"), fkey("C")},
	}
	commitHI := entities.PlannedCommit{
		Subject: "DRAFT: feat(a): H, I", Decls: []entities.DeclKey{fkey("H"), fkey("I")},
	}

	testCases := []bornCase{
		{
			name:         "a helper planned after the anchor moves into the anchor commit",
			inner:        []entities.PlannedCommit{commitA, commitB, commitH, commitI},
			wantSubjects: []string{"feat: A", fix, "feat: B"},
			wantDecls:    []entities.DeclKey{fkey("H"), fkey("I")},
		},
		{
			name: "a pre-image reading a helper born after it folds the anchor",
			inner: []entities.PlannedCommit{
				commitA,
				{Position: 1, Subject: "feat: H", Decls: []entities.DeclKey{fkey("H")}},
				{Position: 2, Subject: "feat: I", Decls: []entities.DeclKey{fkey("I")}},
				{Position: 3, Subject: "feat: B", Decls: []entities.DeclKey{fkey("B")}},
			},
			preimage:     "A old H\n",
			wantSubjects: []string{"feat: A", "feat: H", "feat: I", "feat: B"},
		},
		{
			name:         "a helper already born before the anchor moves into it",
			inner:        []entities.PlannedCommit{commitA, commitHI, commitB},
			wantSubjects: []string{"feat: A", fix, "feat: B"},
			wantDecls:    []entities.DeclKey{fkey("H"), fkey("I")},
		},
		{
			name:         "a draft subject drops the names that moved",
			inner:        []entities.PlannedCommit{commitA, commitHIC, commitB},
			wantSubjects: []string{"feat: A", "DRAFT: feat(a): C", fix, "feat: B"},
			wantDecls:    []entities.DeclKey{fkey("H"), fkey("I")},
		},
		{
			name:         "a helper needing a declaration born earlier moves",
			inner:        []entities.PlannedCommit{commitA, commitB, commitHI},
			edges:        []entities.Edge{needs("H", "A"), needs("I", "H")},
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

func assertBorn(t *testing.T, testCase bornCase) {
	t.Helper()

	var buf bytes.Buffer

	preimage := cmp.Or(testCase.preimage, "A old\n")
	planner := bornPlanner(
		gomock.NewController(t), entities.Plan{Commits: testCase.inner}, preimage, &buf,
	)
	decls := graphOf(map[string]string{"a.go": bornFinal}).Decls()

	for idx := range decls {
		if slices.Contains(testCase.atomed, decls[idx].Key.Name) {
			decls[idx].Atom = "pair"
		}

		decls[idx].IsInterface = slices.Contains(testCase.interfaces, decls[idx].Key.Name)
	}

	got, err := planner.Plan(entities.NewGraph(decls, testCase.edges, nil), entities.Ordering{})
	if err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	subjects := make([]string, 0, len(got.Commits))

	var carried []entities.DeclKey

	for idx, commit := range got.Commits {
		subjects = append(subjects, commit.Subject)

		if commit.Subject == bornFix {
			carried = commit.Decls
		}

		if commit.Position != entities.Position(idx) {
			t.Fatalf("commit %d carries position %d", idx, commit.Position)
		}
	}

	stayed := 0

	for _, fields := range logged(t, &buf) {
		if fields["msg"] == "anchors: born declaration stays" && fields["level"] == "WARN" {
			stayed++
		}
	}

	if !slices.Equal(subjects, testCase.wantSubjects) ||
		!reflect.DeepEqual(carried, testCase.wantDecls) || stayed != testCase.wantStayed {
		t.Fatalf("subjects %q anchor decls %v stayed %d, want %q %v %d",
			subjects, carried, stayed, testCase.wantSubjects, testCase.wantDecls,
			testCase.wantStayed)
	}
}

func TestHoldersBeforeKeepsThePreImageActiveJustBeforeTheSlot(t *testing.T) {
	t.Parallel()

	chosen := []kept{
		{found: found{entries: []entry{{key: fkey("K"), preimage: "k1"}}}},
		{found: found{entries: []entry{{key: fkey("N"), preimage: "n2"}}}},
		{found: found{entries: []entry{{key: fkey("K"), preimage: "k3"}}}},
		{found: found{entries: []entry{{key: fkey("K"), preimage: "k4"}}}},
	}

	testCases := []struct {
		name string
		at   int
		want []string
	}{
		{
			name: "a key with entries before and after the anchor keeps the first newer one",
			at:   1, want: []string{"k1", "n2", "k3"},
		},
		{
			name: "a key the anchor itself holds skips every newer entry",
			at:   2, want: []string{"k1", "n2", "k3"},
		},
		{
			name: "a key seen only in newer entries keeps the first of them",
			at:   0, want: []string{"k1", "n2"},
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

func TestMentionsSkipsAMethodOnlyWhenItsReceiverInTheSameFileMoves(t *testing.T) {
	t.Parallel()

	method := entities.DeclKey{File: "a.go", Kind: entities.KindMethod, Name: "Owner.Run"}
	holder := entry{key: fkey("X"), preimage: "X calls Run"}

	testCases := []struct {
		name   string
		moving map[entities.DeclKey]bool
		want   bool
	}{
		{name: "no receiver moves", want: true},
		{
			name: "the receiver type in the same file moves",
			moving: map[entities.DeclKey]bool{
				{File: "a.go", Kind: entities.KindType, Name: "Owner"}: true,
			},
		},
		{
			name: "a type of that name in another file moves",
			moving: map[entities.DeclKey]bool{
				{File: "b.go", Kind: entities.KindType, Name: "Owner"}: true,
			},
			want: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			view := scene{holders: []entry{holder}}
			index := map[entities.DeclKey]int{holder.key: 0}

			if got := view.mentions(method, testCase.moving, index, 1); got != testCase.want {
				t.Fatalf("mentions() = %t, want %t", got, testCase.want)
			}
		})
	}
}
