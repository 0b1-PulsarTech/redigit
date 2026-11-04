package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

func TestShapeHookLeavesTheStagesUnchanged(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		passes string
	}{
		{"no section", ""},
		{"null section", "null"},
		{"empty list", "[]"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			hookLeavesTheStagesUnchanged(t, func(
				ctx context.Context, cfg *Config, env stageEnv, built *stages,
			) error {
				cfg.Sections = rules.File{Passes: json.RawMessage(testCase.passes)}

				return shapeHook(ctx, cfg, env, built)
			})
		})
	}
}

func shapeFixture() (entities.Graph, entities.Plan) {
	provider := entities.DeclKey{File: "pkg/b/b.go", Kind: entities.KindFunc, Name: "B"}
	host := entities.DeclKey{File: "pkg/a/a.go", Kind: entities.KindFunc, Name: "A"}
	tests := entities.DeclKey{File: "pkg/b/b_test.go", Kind: entities.KindFunc, Name: "TestB"}

	graph := entities.NewGraph(
		[]entities.Decl{
			{Key: provider, Lines: 20},
			{Key: host, Lines: 50},
			{Key: tests, Lines: 250, IsTest: true},
		},
		[]entities.Edge{{Dependent: tests, Provider: provider, Reason: entities.ReasonReference}},
		nil,
	)

	return graph, entities.Plan{Commits: []entities.PlannedCommit{
		{Subject: "feat(b): b", Decls: []entities.DeclKey{provider}},
		{Position: 1, Subject: "feat(a): a", Decls: []entities.DeclKey{host, tests}},
	}}
}

func TestShapeHookDecoratesThePlanner(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	graph, plan := shapeFixture()

	testCases := []struct {
		name         string
		inner        fakePlanner
		wantErr      error
		wantSubjects []string
	}{
		{
			"the passes reshape the planner's plan",
			fakePlanner{plan: plan},
			nil,
			[]string{"feat(b): b", "DRAFT: test(b): b_test.go", "feat(a): a"},
		},
		{"a planner failure passes through", fakePlanner{err: boom}, boom, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := Defaults()
			cfg.Sections = rules.File{Passes: json.RawMessage(`[{"type":"tidy-test-after"}]`)}
			built := &stages{planner: testCase.inner}

			if err := shapeHook(t.Context(), &cfg, stageEnv{}, built); err != nil {
				t.Fatalf("shapeHook() = %v", err)
			}

			got, err := built.planner.Plan(graph, entities.Ordering{})
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Plan() error = %v, want %v", err, testCase.wantErr)
			}

			subjects := make([]string, 0, len(got.Commits))
			for _, commit := range got.Commits {
				subjects = append(subjects, commit.Subject)
			}

			if len(subjects) != len(testCase.wantSubjects) ||
				(len(subjects) > 0 && !slices.Equal(subjects, testCase.wantSubjects)) {
				t.Fatalf("subjects = %v, want %v", subjects, testCase.wantSubjects)
			}
		})
	}
}

func TestShapeHookRejectsABadSection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		passes  string
		wantErr error
	}{
		{"unknown type", `[{"type":"nope"}]`, shaping.ErrUnknownType},
		{"not a list", `{"type":"tidy-test-after"}`, shaping.ErrMalformed},
		{"unknown field", `[{"type":"tidy-test-after","tail":1}]`, shaping.ErrMalformed},
		{"missing type", `[{}]`, shaping.ErrMissingType},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := Defaults()
			cfg.ConfigPath = "redigit.json"
			cfg.Sections = rules.File{Passes: json.RawMessage(testCase.passes)}
			kept := errors.New("kept")
			built := identityStages(kept)

			err := shapeHook(t.Context(), &cfg, stageEnv{}, built)
			if !errors.Is(err, ErrConfig) || !errors.Is(err, testCase.wantErr) {
				t.Fatalf("shapeHook() = %v, want ErrConfig and %v", err, testCase.wantErr)
			}

			_, planErr := built.planner.Plan(entities.Graph{}, entities.Ordering{})
			if !errors.Is(planErr, kept) {
				t.Fatalf("planner was replaced despite the error: %v", planErr)
			}
		})
	}
}

type slogCase struct {
	name   string
	log    func(observer slogObserver)
	want   []string
	absent []string
}

func slogCases(key entities.DeclKey) []slogCase {
	return []slogCase{
		{
			"a move is logged with its pass",
			func(observer slogObserver) {
				observer.Moved("tidy-test-after", shaping.Move{Decl: key, From: 2, To: 5})
			},
			[]string{"level=DEBUG", "pass=tidy-test-after", "decl=a.go:func:A", "from=2", "to=5"},
			nil,
		},
		{
			"a reseat is logged as such with the slot it follows",
			func(observer slogObserver) {
				observer.Moved(
					"tidy-test-after", shaping.Move{Decl: key, From: 4, To: 1, Reseat: true},
				)
			},
			[]string{"level=DEBUG", "reseated after", "decl=a.go:func:A", "slot=4", "after=1"},
			[]string{"shaping move", "from=", "to="},
		},
		{
			"a reseat to the front says front, not a slot number",
			func(observer slogObserver) {
				observer.Moved(
					"tidy-test-after",
					shaping.Move{Decl: key, From: 4, To: shaping.NoSlot, Reseat: true},
				)
			},
			[]string{"level=DEBUG", "reseated after", "slot=4", "after=front"},
			[]string{"after=-1"},
		},
		{
			"a refusal is logged as a warning",
			func(observer slogObserver) {
				observer.Refused(
					"tidy-test-after",
					[]entities.Edge{{Dependent: key, Provider: key}},
				)
			},
			[]string{"level=WARN", "pass=tidy-test-after", "added=1"},
			nil,
		},
		{
			"a refusal names both kinds of breach",
			func(observer slogObserver) {
				added := []entities.Edge{{Dependent: key, Provider: key}}
				observer.Refused("tidy-test-after", added)
			},
			[]string{"hard-edge violations or atom splits"},
			nil,
		},
		{
			"an atom split reads as a split from its atom, not as a need",
			func(observer slogObserver) {
				mate := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "B"}
				observer.Refused("tidy-test-after", []entities.Edge{
					{Dependent: mate, Provider: key, Reason: shaping.ReasonAtomSplit},
				})
			},
			[]string{"a.go:func:B split from its atom with a.go:func:A"},
			[]string{"needs"},
		},
		{
			"a refusal names only the first few added edges",
			func(observer slogObserver) {
				added := make([]entities.Edge, 0, 7)
				for index := range 7 {
					dependent := entities.DeclKey{
						File: "a.go", Kind: entities.KindFunc, Name: "D" + strconv.Itoa(index),
					}
					added = append(added, entities.Edge{Dependent: dependent, Provider: key})
				}

				observer.Refused("tidy-test-after", added)
			},
			[]string{"added=7", "a.go:func:D0 needs a.go:func:A", "a.go:func:D4 needs a.go:func:A"},
			[]string{"D5", "D6"},
		},
	}
}

func TestSlogObserver(t *testing.T) {
	t.Parallel()

	key := entities.DeclKey{File: "a.go", Kind: entities.KindFunc, Name: "A"}

	for _, testCase := range slogCases(key) {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer

			handler := slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})
			testCase.log(slogObserver{log: slog.New(handler)})

			for _, want := range testCase.want {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("log %q lacks %q", out.String(), want)
				}
			}

			for _, absent := range testCase.absent {
				if strings.Contains(out.String(), absent) {
					t.Fatalf("log %q must not name %q", out.String(), absent)
				}
			}
		})
	}
}

func TestStageHooksRunInTheFixedOrder(t *testing.T) {
	t.Parallel()

	hooks := stageHooks()
	names := make([]string, 0, len(hooks))

	for _, hook := range hooks {
		names = append(names, hook.name)
	}

	want := []string{"rules", "shaping", "lineage"}
	if !slices.Equal(names, want) {
		t.Fatalf("stageHooks() = %v, want %v", names, want)
	}
}
