package anchoring

import (
	"context"

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
	}
	blobs := map[string]string{
		"o1": "A one\n", "n1": "A uno\n", "o2": "A uno\n", "n2": "A dos\n",
		"o3": "B old\n", "n3": "B new\n",
		"o6": "A one\n", "n6": "A two\n",
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
