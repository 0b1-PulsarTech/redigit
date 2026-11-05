package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/lineage"
)

func TestLineageHook(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		hook stageHook
	}{
		{"leaves the stages unchanged", lineageHook},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			hookLeavesTheStagesUnchanged(t, testCase.hook)
		})
	}
}

type fakeLineageScan struct {
	calls  int
	got    lineage.Config
	result lineage.Result
	err    error
}

func (fake *fakeLineageScan) scan(
	_ context.Context,
	_ *Config,
	_ stageEnv,
	config lineage.Config,
) (lineage.Result, error) {
	fake.calls++
	fake.got = config

	return fake.result, fake.err
}

func (fake *fakeLineageScan) assertScanned(t *testing.T, wantCalls int, want lineage.Config) {
	t.Helper()

	if fake.calls != wantCalls {
		t.Fatalf("scans = %d, want %d", fake.calls, wantCalls)
	}

	if fake.calls > 0 &&
		(fake.got.Follow != want.Follow || len(fake.got.Languages) != len(want.Languages)) {
		t.Fatalf("scan config = %+v, want follow %v and %d language(s)",
			fake.got, want.Follow, len(want.Languages))
	}
}

func TestLineageStage(t *testing.T) {
	t.Parallel()

	declared := entities.DeclKey{File: "x.go", Name: "A"}
	scanFailure := errors.New("scan failed")
	oneLanguage := make([]lineage.Language, 1)

	testCases := []struct {
		name         string
		mode         string
		section      string
		scanErr      error
		wantErr      error
		wantCalls    int
		wantScan     lineage.Config
		wantReplaced bool
	}{
		{name: "history mode never scans", mode: ModeHistory},
		{name: "an empty mode is history", mode: ""},
		{
			name: "a bad section fails even in history mode", mode: ModeHistory,
			section: `{"nope": 1}`, wantErr: ErrConfig,
		},
		{
			name: "lineage mode scans once and floors the orderer", mode: ModeLineage,
			wantCalls: 1, wantScan: lineage.Config{Languages: oneLanguage, Follow: true},
			wantReplaced: true,
		},
		{
			name: "follow false reaches the scanner", mode: ModeLineage,
			section: `{"follow": false}`, wantCalls: 1,
			wantScan: lineage.Config{Languages: oneLanguage}, wantReplaced: true,
		},
		{
			name: "a bad section fails before the scan", mode: ModeLineage,
			section: `{"follow": 3}`, wantErr: ErrConfig,
		},
		{
			name: "a scan error fails the stage and leaves the orderer alone", mode: ModeLineage,
			scanErr: scanFailure, wantErr: scanFailure, wantCalls: 1,
			wantScan: lineage.Config{Languages: oneLanguage, Follow: true},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			births := map[entities.DeclKey]lineage.Birth{declared: {At: 2}}
			fake := &fakeLineageScan{result: lineage.Result{Births: births}, err: testCase.scanErr}
			cfg := Defaults()
			cfg.Mode = testCase.mode
			cfg.Sections.Lineage = json.RawMessage(testCase.section)

			kept := errors.New("kept")
			built := identityStages(kept)

			deps := lineageDeps{scan: fake.scan}
			err := lineageStage(t.Context(), &cfg, goAndBareEnv(), built, deps)

			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			fake.assertScanned(t, testCase.wantCalls, testCase.wantScan)

			if len(built.closers) != 0 {
				t.Fatalf("closers = %v, want none held", built.closers)
			}

			got, orderErr := orderOneDeclaration(built, declared)

			if testCase.wantReplaced && (orderErr != nil || got != 2) {
				t.Fatalf("Order() = %d, %v, want the declaration floored at 2", got, orderErr)
			}

			if !testCase.wantReplaced && !errors.Is(orderErr, kept) {
				t.Fatalf("orderer err = %v, want the kept orderer", orderErr)
			}
		})
	}
}

func goAndBareEnv() stageEnv {
	return stageEnv{specific: []indexing.Language{
		{Name: "go", Extensions: []string{".go"}, Syntax: stubRenderer{}},
		{Name: "bare"},
	}}
}

func TestLineageStageRunsLastAndOwnsTheOrderer(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		mode        string
		wantLineage bool
	}{
		{
			name: "lineage mode leaves the lineage orderer after every hook",
			mode: ModeLineage, wantLineage: true,
		},
		{name: "history mode leaves the orderer untouched", mode: ModeHistory},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeLineageScan{result: lineage.Result{}}
			cfg := Defaults()
			cfg.Mode = testCase.mode
			built := identityStages(errors.New("kept"))

			var ran []string

			var seen orderer

			hooks := stageHooks()
			for idx := range hooks {
				name, inner := hooks[idx].name, hooks[idx].run
				if name == "lineage" {
					inner = func(ctx context.Context, conf *Config, env stageEnv, held *stages) error {
						return lineageStage(ctx, conf, env, held, lineageDeps{scan: fake.scan})
					}
				}

				hooks[idx].run = func(ctx context.Context, conf *Config, env stageEnv, held *stages) error {
					ran = append(ran, name)

					return inner(ctx, conf, env, held)
				}
			}

			hooks = append(hooks, namedHook{name: "after", run: func(
				_ context.Context, _ *Config, _ stageEnv, held *stages,
			) error {
				seen = held.orderer

				return nil
			}})

			if err := runHooks(t.Context(), &cfg, goAndBareEnv(), built, hooks); err != nil {
				t.Fatalf("runHooks() = %v", err)
			}

			if want := []string{"rules", "shaping", "lineage"}; !slices.Equal(ran, want) {
				t.Fatalf("hooks ran %v, want %v", ran, want)
			}

			if _, isLineage := seen.(lineageOrderer); isLineage != testCase.wantLineage {
				t.Fatalf("orderer after every hook is %T, lineage = %v, want %v",
					seen, isLineage, testCase.wantLineage)
			}
		})
	}
}

type anchorStageCase struct {
	name        string
	mode        string
	section     string
	openErr     error
	wantErr     error
	wantOpens   int
	wantScans   int
	wantClosers int
	wantWrapped bool
}

func runAnchorStage(t *testing.T, testCase anchorStageCase) {
	t.Helper()

	history := []entities.SourceTouch{
		{Position: 4, Rev: "3d2d883000"},
		{Position: 7, Rev: "0aab4c6000"},
		{Position: 9, Rev: "3d2d8aa000"},
	}

	var (
		opens int
		order []string
	)

	scan := &fakeLineageScan{}
	deps := lineageDeps{
		scan: scan.scan,
		open: func(context.Context, *Config, stageEnv) (anchoring.Config, closers, error) {
			opens++

			held := closers{orderedCloser{"probe", &order}}

			return anchoring.Config{}, held, testCase.openErr
		},
	}

	cfg := Defaults()
	cfg.Mode = testCase.mode
	cfg.Sections.Lineage = json.RawMessage(testCase.section)

	env := goAndBareEnv()
	env.opened.snapshot.History = history
	kept := errors.New("kept")
	built := identityStages(kept)

	err := lineageStage(t.Context(), &cfg, env, built, deps)
	if !errors.Is(err, testCase.wantErr) {
		t.Fatalf("err = %v, want %v", err, testCase.wantErr)
	}

	if opens != testCase.wantOpens || scan.calls != testCase.wantScans {
		t.Fatalf("opens = %d, scans = %d, want %d and %d",
			opens, scan.calls, testCase.wantOpens, testCase.wantScans)
	}

	if len(built.closers) != testCase.wantClosers {
		t.Fatalf("closers = %d, want %d", len(built.closers), testCase.wantClosers)
	}

	_, isWrapped := built.planner.(anchoring.Planner)
	if isWrapped != testCase.wantWrapped {
		t.Fatalf("planner is %T, want anchoring = %v", built.planner, testCase.wantWrapped)
	}

	if isWrapped {
		_, planErr := built.planner.Plan(entities.Graph{}, entities.Ordering{})
		if !errors.Is(planErr, kept) {
			t.Fatalf("Plan() = %v, want the previous planner under the anchors", planErr)
		}
	}
}

func TestLineageStageAnchors(t *testing.T) {
	t.Parallel()

	openFailure := errors.New("open failed")
	fix := `{"anchors": [{"rev": "3d2d883", "kind": "fix"}]}`

	testCases := []anchorStageCase{
		{name: "no anchors never open the probe", mode: ModeLineage, wantScans: 1},
		{name: "history mode ignores the anchors", mode: ModeHistory, section: fix},
		{
			name: "history mode refuses an unknown rev", mode: ModeHistory,
			section: `{"anchors": [{"rev": "beef000", "kind": "fix"}]}`, wantErr: ErrConfig,
		},
		{
			name: "an anchor wraps the planner and holds what it opened", mode: ModeLineage,
			section: fix, wantOpens: 1, wantScans: 1, wantClosers: 1, wantWrapped: true,
		},
		{
			name: "an unknown rev fails before the scan", mode: ModeLineage,
			section: `{"anchors": [{"rev": "beef000", "kind": "fix"}]}`, wantErr: ErrConfig,
		},
		{
			name: "an ambiguous rev fails before the scan", mode: ModeLineage,
			section: `{"anchors": [{"rev": "3d2d8", "kind": "fix"}]}`, wantErr: ErrConfig,
		},
		{
			name: "a probe that cannot open fails the stage", mode: ModeLineage,
			section: `{"anchors": [{"rev": "0aab4c6", "kind": "perf"}]}`,
			openErr: openFailure, wantErr: openFailure, wantOpens: 1, wantScans: 1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runAnchorStage(t, testCase)
		})
	}
}

func orderOneDeclaration(built *stages, declared entities.DeclKey) (entities.Position, error) {
	graph := entities.NewGraph([]entities.Decl{{Key: declared, Unit: "x"}}, nil, nil)
	snap := entities.Snapshot{
		History: []entities.SourceTouch{{Position: 5, Touched: []string{declared.File}}},
	}

	got, err := built.orderer.Order(graph, snap)

	return got.Of(declared), err
}
