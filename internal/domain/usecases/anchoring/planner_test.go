package anchoring

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func finalFiles() map[string]string {
	return map[string]string{"a.go": "A last\nB other\n"}
}

type planCase struct {
	name         string
	inner        []entities.PlannedCommit
	innerErr     error
	anchors      []Anchor
	edges        []entities.Edge
	wantErr      error
	wantSubjects []string
	wantChanges  []entities.DeclKey
	wantVersions map[entities.DeclKey][]entities.Version
	wantEdits    [][]Edit
}

func TestPlan(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	keep := []entities.DeclKey{fkey("A"), fkey("B"), fkey("H")}
	editA := Edit{
		File: "a.go", Span: entities.Span{Start: 0, End: 6}, Text: "A one",
		Keep: keep,
	}

	testCases := []planCase{
		{
			name:         "no anchors is the delegate plan",
			wantSubjects: []string{"feat(a): add A", "feat(b): add B"},
		},
		{
			name:     "a delegate failure",
			innerErr: boom,
			anchors:  []Anchor{{Rev: "fix", Position: 5}},
			wantErr:  boom,
		},
		{
			name:    "an anchor is a real commit",
			anchors: []Anchor{{Rev: "fix", Position: 5}},
			wantSubjects: []string{
				"feat(a): add A", "fix(golang): repair A", "feat(b): add B",
			},
			wantChanges: []entities.DeclKey{fkey("A")},
			wantVersions: map[entities.DeclKey][]entities.Version{
				fkey("A"): {{Text: "A one", Until: 1}},
			},
			wantEdits: [][]Edit{{editA}},
		},
		{
			name:    "two anchors chain on one declaration",
			anchors: []Anchor{{Rev: "fix2", Position: 7}, {Rev: "fix", Position: 5}},
			wantSubjects: []string{
				"feat(a): add A", "fix(golang): repair A", "perf(golang): speed up A",
				"feat(b): add B",
			},
			wantChanges: []entities.DeclKey{fkey("A")},
			wantVersions: map[entities.DeclKey][]entities.Version{
				fkey("A"): {{Text: "A one", Until: 1}, {Text: "A uno", Until: 2}},
			},
			wantEdits: [][]Edit{
				{{
					File: "a.go", Span: entities.Span{Start: 0, End: 6}, Text: "A uno",
					Keep: keep,
				}},
				{editA},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assertPlan(t, testCase)
		})
	}
}

func TestPlanSlotsAfterRemovedCommits(t *testing.T) {
	t.Parallel()

	withHelper := []entities.PlannedCommit{
		{Subject: "feat: A", Decls: []entities.DeclKey{fkey("A")}},
		{Subject: "feat: H", Decls: []entities.DeclKey{fkey("H")}},
		{Subject: "feat: B", Decls: []entities.DeclKey{fkey("B")}},
	}
	keep := []entities.DeclKey{fkey("A"), fkey("B"), fkey("H")}
	again := Edit{File: "a.go", Span: entities.Span{End: 6}, Text: "A one", Keep: keep}
	oldA := Edit{File: "a.go", Span: entities.Span{Start: 0, End: 6}, Text: "A old", Keep: keep}
	oldB := Edit{File: "a.go", Span: entities.Span{Start: 7, End: 14}, Text: "B old", Keep: keep}

	testCases := []planCase{
		{
			name:    "a later anchor keeps its slot after an emptied commit goes",
			inner:   withHelper,
			anchors: []Anchor{{Rev: "fixh", Position: 5}, {Rev: "fixb", Position: 10}},
			wantSubjects: []string{
				"feat: A", "fix(golang): repair A and add H", "feat: B", "fix(golang): repair B",
			},
			wantChanges: []entities.DeclKey{fkey("A")},
			wantVersions: map[entities.DeclKey][]entities.Version{
				fkey("A"): {{Text: "A old", Until: 1}},
				fkey("B"): {{Text: "B old", Until: 3}},
			},
			wantEdits: [][]Edit{{oldB}, {oldA, oldB}, {oldA}},
		},
		{
			name:    "an anchor whose rewrite cancels against the next one is dropped",
			anchors: []Anchor{{Rev: "fix", Position: 5}, {Rev: "twin", Position: 7}},
			wantSubjects: []string{
				"feat(a): add A",
				"fix(golang): repair A again",
				"feat(b): add B",
			},
			wantChanges: []entities.DeclKey{fkey("A")},
			wantVersions: map[entities.DeclKey][]entities.Version{
				fkey("A"): {{Text: "A one", Until: 1}},
			},
			wantEdits: [][]Edit{{again}, {again}},
		},
		{
			name:         "a fix that only births a declaration stays when it takes one in",
			inner:        withHelper,
			anchors:      []Anchor{{Rev: "birth", Position: 5}},
			wantSubjects: []string{"feat: A", "fix(golang): add H", "feat: B"},
			wantVersions: map[entities.DeclKey][]entities.Version{},
		},
		{
			name:  "a fix that only births a declaration is dropped when none can move",
			inner: []entities.PlannedCommit{withHelper[0], withHelper[2], withHelper[1]},
			edges: []entities.Edge{
				{Dependent: fkey("H"), Provider: fkey("B"), Reason: entities.ReasonReference},
			},
			anchors:      []Anchor{{Rev: "birth", Position: 5}},
			wantSubjects: []string{"feat: A", "feat: B", "feat: H"},
			wantVersions: map[entities.DeclKey][]entities.Version{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assertPlan(t, testCase)
		})
	}
}

func assertPlan(t *testing.T, testCase planCase) {
	t.Helper()

	var record probeLog

	ctrl := gomock.NewController(t)
	inner := NewMockDelegate(ctrl)
	plan := basePlan()

	if testCase.inner != nil {
		plan = entities.Plan{Commits: testCase.inner}
	}

	inner.EXPECT().Plan(gomock.Any(), gomock.Any()).Return(plan, testCase.innerErr)

	planner := New(Config{
		Inner:   inner,
		Anchors: testCase.anchors,
		Floor: map[entities.DeclKey]entities.Position{
			fkey("A"): 1, fkey("B"): 9, fkey("H"): 2,
		},
		Languages: goLanguages(),
		Revisions: sceneRevisions(ctrl),
		Finals:    mapFinals(ctrl, map[string]string{"a.go": "A last\nB other\nH help\n"}),
		Probe:     scripted(ctrl, &record, []Verdict{{IsBuilt: true}}, nil),
	})
	graph := graphOf(map[string]string{"a.go": "A last\nB other\nH help\n"})

	got, err := planner.Plan(
		entities.NewGraph(graph.Decls(), testCase.edges, nil),
		entities.Ordering{},
	)
	if !errors.Is(err, testCase.wantErr) {
		t.Fatalf("Plan() error = %v, want %v", err, testCase.wantErr)
	}

	if err != nil {
		return
	}

	var (
		subjects []string
		changes  []entities.DeclKey
	)

	for idx, commit := range got.Commits {
		subjects = append(subjects, commit.Subject)

		if commit.Position != entities.Position(idx) {
			t.Fatalf("commit %d carries position %d", idx, commit.Position)
		}

		if len(commit.Changes) > 0 && changes == nil {
			changes = commit.Changes
		}
	}

	if !reflect.DeepEqual(subjects, testCase.wantSubjects) ||
		!reflect.DeepEqual(changes, testCase.wantChanges) ||
		!reflect.DeepEqual(got.Versions, testCase.wantVersions) ||
		!reflect.DeepEqual(record.edits, testCase.wantEdits) {
		t.Fatalf("subjects %q changes %v versions %+v edits %+v, want %q %v %+v %+v",
			subjects, changes, got.Versions, record.edits, testCase.wantSubjects,
			testCase.wantChanges, testCase.wantVersions, testCase.wantEdits)
	}
}

func TestPlanBindsTheRunContextToTheProbe(t *testing.T) {
	t.Parallel()

	type tag struct{}

	cancelled, cancel := context.WithCancel(context.WithValue(context.Background(), tag{}, "run"))
	cancel()

	testCases := []struct {
		name      string
		bound     context.Context
		wantTag   any
		wantErr   error
		wantCalls int
	}{
		{name: "no bound context falls back to a background one", wantCalls: 1},
		{
			name:    "the run context reaches the probe",
			bound:   context.WithValue(context.Background(), tag{}, "run"),
			wantTag: "run", wantCalls: 1,
		},
		{
			name: "a cancelled run ends the plan", bound: cancelled,
			wantTag: "run", wantErr: context.Canceled, wantCalls: 1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			inner := NewMockDelegate(ctrl)
			inner.EXPECT().Plan(gomock.Any(), gomock.Any()).Return(basePlan(), nil)

			calls := 0
			probe := NewMockProbe(ctrl)
			probe.EXPECT().
				Builds(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, _ []Edit) (Verdict, error) {
					calls++
					if got := ctx.Value(tag{}); got != testCase.wantTag {
						t.Errorf("probe context tag = %v, want %v", got, testCase.wantTag)
					}

					return Verdict{IsBuilt: true}, ctx.Err()
				}).
				AnyTimes()

			planner := New(Config{
				Inner: inner, Anchors: []Anchor{{Rev: "fix", Position: 5}}, Bound: testCase.bound,
				Floor:     map[entities.DeclKey]entities.Position{fkey("A"): 1, fkey("B"): 9},
				Languages: goLanguages(), Revisions: sceneRevisions(ctrl),
				Finals: mapFinals(ctrl, finalFiles()), Probe: probe,
			})

			_, err := planOf(planner, finalFiles())
			if !errors.Is(err, testCase.wantErr) || calls != testCase.wantCalls {
				t.Fatalf("Plan() error = %v after %d probes, want %v after %d",
					err, calls, testCase.wantErr, testCase.wantCalls)
			}
		})
	}
}

type readsWant struct {
	outcome string
	reason  string
	stayed  int
}

func summariesOf(t *testing.T, buf *bytes.Buffer) map[string]readsWant {
	t.Helper()

	got := map[string]readsWant{}

	for _, fields := range logged(t, buf) {
		if fields["msg"] != "anchors: summary" {
			continue
		}

		stayed, _ := fields["stayed"].(float64)

		got[fields["rev"].(string)] = readsWant{
			outcome: fields["outcome"].(string),
			reason:  fields["reason"].(string),
			stayed:  int(stayed),
		}
	}

	return got
}

type readsCase struct {
	name        string
	stays       staysCase
	policy      Policy
	isUnchecked bool
	want        map[string]readsWant
}

func plannedCommit(name string) entities.PlannedCommit {
	return entities.PlannedCommit{Subject: "feat: " + name, Decls: []entities.DeclKey{fkey(name)}}
}

func assertReads(t *testing.T, testCase readsCase) {
	t.Helper()

	var buf bytes.Buffer

	ctrl := gomock.NewController(t)
	planner := staysPlanner(ctrl, testCase.stays, &buf)
	planner.cfg.Policy = testCase.policy

	if testCase.isUnchecked {
		probe := NewMockProbe(ctrl)
		probe.EXPECT().
			Builds(gomock.Any(), gomock.Any()).
			Return(Verdict{IsUnchecked: true, Detail: "go build: no module"}, nil).
			AnyTimes()

		planner.cfg.Probe = probe
	}

	graph := graphOf(map[string]string{"a.go": testCase.stays.final})
	if _, err := planner.Plan(graph, entities.Ordering{}); err != nil {
		t.Fatalf("Plan() = %v", err)
	}

	got := summariesOf(t, &buf)

	for rev, want := range testCase.want {
		if got[rev].outcome != want.outcome || got[rev].stayed != want.stayed ||
			!strings.Contains(got[rev].reason, want.reason) {
			t.Errorf("%s = %+v, want %+v", rev, got[rev], want)
		}
	}
}

func TestPlanJudgesWhatAPreImageReadsPlacedAfterIt(t *testing.T) {
	t.Parallel()

	lone := staysCase{
		inner:   []entities.PlannedCommit{plannedCommit("A"), plannedCommit("B")},
		floor:   map[entities.DeclKey]entities.Position{fkey("A"): 1, fkey("B"): 9},
		final:   "A last\nB other\n",
		anchors: []staysAnchor{{rev: "fix", position: 5, old: "A reads B\n", new: "A last\n"}},
	}
	earlier := lone
	earlier.inner = []entities.PlannedCommit{plannedCommit("B"), plannedCommit("A")}
	earlier.final = "B other\nA last\n"
	moved := staysCase{
		inner: []entities.PlannedCommit{plannedCommit("A"), plannedCommit("N"), plannedCommit("H")},
		floor: map[entities.DeclKey]entities.Position{fkey("A"): 1, fkey("N"): 9, fkey("H"): 9},
		final: "A last\nN note\nH help\n",
		anchors: []staysAnchor{
			{rev: "o", position: 5, old: "A old\n", new: "A new\nH help\n"},
			{rev: "n", position: 9, old: "H reads N\n", new: "H help\n"},
		},
	}

	testCases := []readsCase{
		{
			name: "a lone anchor reading a later declaration folds", stays: lone,
			want: map[string]readsWant{"fix": {outcome: "folded", reason: "a.go:func:B"}},
		},
		{
			name: "the same anchor under wip is marked", stays: lone, policy: PolicyWIP,
			want: map[string]readsWant{"fix": {outcome: "wip", reason: "a.go:func:B"}},
		},
		{
			name: "reading a declaration placed before it is kept", stays: earlier,
			want: map[string]readsWant{"fix": {outcome: "kept", reason: "its pre-image builds"}},
		},
		{
			name:  "an unchecked verdict stays unchecked when its pre-image reads a later one",
			stays: lone, isUnchecked: true,
			want: map[string]readsWant{"fix": {outcome: "unchecked", reason: "go build"}},
		},
		{
			name: "a declaration whose own pre-image reads a later one stays", stays: moved,
			want: map[string]readsWant{
				"o": {outcome: "kept", stayed: 1}, "n": {outcome: "kept"},
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
