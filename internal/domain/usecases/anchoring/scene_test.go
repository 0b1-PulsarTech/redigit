package anchoring

import (
	"context"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type probeLog struct{ edits [][]Edit }

func basePlan() entities.Plan {
	return entities.Plan{Commits: []entities.PlannedCommit{
		{
			Position: 0, Subject: "feat(a): add A",
			Shape: entities.ShapeFeature, Decls: []entities.DeclKey{fkey("A")},
		},
		{
			Position: 1, Subject: "feat(b): add B",
			Shape: entities.ShapeFeature, Decls: []entities.DeclKey{fkey("B")},
		},
	}}
}

func scripted(
	ctrl *gomock.Controller,
	record *probeLog,
	verdicts []Verdict,
	failure error,
) *MockProbe {
	probe := NewMockProbe(ctrl)
	probe.EXPECT().
		Builds(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, edits []Edit) (Verdict, error) {
			record.edits = append(record.edits, edits)

			return verdicts[min(len(record.edits), len(verdicts))-1], failure
		}).
		AnyTimes()

	return probe
}

func sceneRevisions(ctrl *gomock.Controller) *MockRevisions {
	change := func(oldBlob, blob string) []FileChange {
		return []FileChange{{Path: "a.go", OldPath: "a.go", Blob: blob, OldBlob: oldBlob}}
	}
	revisions := map[string]Revision{
		"fix":   {Subject: "fix(golang): repair A", Files: change("o1", "n1")},
		"fix2":  {Subject: "perf(golang): speed up A", Files: change("o2", "n2")},
		"noop":  {Subject: "fix(golang): nothing", Files: change("o1", "o1")},
		"fixb":  {Subject: "fix(golang): repair B", Files: change("o3", "n3")},
		"twin":  {Subject: "fix(golang): repair A again", Files: change("o6", "n6")},
		"fixh":  {Subject: "fix(golang): repair A and add H", Files: change("o4", "n4")},
		"birth": {Subject: "fix(golang): add H", Files: change("o5", "n5")},
		"lost":  {Subject: "fix(golang): repair Z", Files: change("o7", "n7")},
	}
	blobs := map[string]string{
		"o1": "A one\n", "n1": "A uno\n", "o2": "A uno\n", "n2": "A dos\n",
		"o3": "B old\n", "n3": "B new\n",
		"o6": "A one\n", "n6": "A two\n",
		"o7": "Z one\n", "n7": "Z two\n",
		"o4": "A old\n", "n4": "A new\nH help\n", "o5": "A last\n", "n5": "A last\nH help\n",
	}

	mock := mapBlobs(ctrl, blobs)
	mock.EXPECT().
		Revision(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, rev string) (Revision, error) {
			found, isKnown := revisions[rev]
			if !isKnown {
				return Revision{}, ErrAnchor
			}

			return found, nil
		}).
		AnyTimes()

	return mock
}

func fixture(ctrl *gomock.Controller, probe Probe, policy Policy, anchors ...Anchor) Planner {
	inner := NewMockDelegate(ctrl)
	inner.EXPECT().Plan(gomock.Any(), gomock.Any()).Return(basePlan(), nil).AnyTimes()

	files := map[string]string{"a.go": "A last\nB other\n"}

	return New(Config{
		Inner:     inner,
		Policy:    policy,
		Anchors:   anchors,
		Floor:     map[entities.DeclKey]entities.Position{fkey("A"): 1, fkey("B"): 9},
		Languages: goLanguages(),
		Revisions: sceneRevisions(ctrl),
		Finals:    mapFinals(ctrl, files),
		Probe:     probe,
	})
}

func planOf(planner Planner, files map[string]string) (entities.Plan, error) {
	return planner.Plan(graphOf(files), entities.Ordering{})
}

type readsAfterCase struct {
	name      string
	candidate entities.Decl
	preimage  string
	final     string
	imports   []string
	at        int
	homeAt    int
	homeFile  string
	homeKind  entities.DeclKind
	reads     []string
	asks      int
	isReading bool
	isParsed  bool
	isMoving  bool
	isMissing bool
	isTested  bool
	isModule  bool
	want      bool
}

func readsAfterHome() entities.Decl {
	return entities.Decl{
		Key:  entities.DeclKey{File: "h/h.go", Kind: entities.KindFunc, Name: "H"},
		Unit: "h",
	}
}

func readsAfterOther(kind entities.DeclKind, unit entities.UnitID, name string) entities.Decl {
	key := entities.DeclKey{File: "x.go", Kind: kind, Name: name}

	return entities.Decl{Key: key, Unit: unit}
}

func readsAfterTesting(decl entities.Decl) entities.Decl {
	decl.IsTest = true

	return decl
}

func readsAfterHelper(name string) entities.Decl {
	key := entities.DeclKey{File: "h/h_test.go", Kind: entities.KindFunc, Name: name}

	return entities.Decl{Key: key, Unit: "h"}
}

func checkReadsAfter(t *testing.T, testCases []readsAfterCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			home := readsAfterHome()
			home.IsTest = testCase.isTested

			if testCase.homeKind != "" {
				home.Key.Kind = testCase.homeKind
			}

			if testCase.homeFile != "" {
				home.Key.File = testCase.homeFile
			}

			graph := entities.NewGraph([]entities.Decl{home, testCase.candidate}, nil, nil)
			view := scene{graph: graph, decls: graph.Decls(), modules: map[string]bool{}}

			if testCase.isModule {
				view.modules[testCase.candidate.Unit.Dir()] = true
			}

			if testCase.isReading {
				reader := NewMockReader(gomock.NewController(t))
				reader.EXPECT().
					Reads(home.Key.File, testCase.preimage).
					Return(testCase.reads, testCase.isParsed).
					Times(testCase.asks)

				view.reader = reader
			}

			index := map[entities.DeclKey]int{home.Key: testCase.homeAt}

			if !testCase.isMissing {
				index[testCase.candidate.Key] = testCase.at
			}

			moving := map[entities.DeclKey]bool{testCase.candidate.Key: testCase.isMoving}
			placed := entry{
				key: home.Key, preimage: testCase.preimage,
				final: testCase.final, imports: testCase.imports,
			}

			got := view.readsAfter(placed, index, moving, 1)
			if (got != "") != testCase.want {
				t.Fatalf("readsAfter() = %q, want a hit: %t", got, testCase.want)
			}
		})
	}
}
