package bootstrap

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/typed"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom/di"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

const goDIRule = `{"type":"go-di","registerFuncs":["Register*"],"resolveFuncs":["Get"]}`

func TestRuleHook(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		hook stageHook
	}{
		{"leaves the stages unchanged", ruleHook},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			hookLeavesTheStagesUnchanged(t, testCase.hook)
		})
	}
}

func TestRuleHookWiresGoIdioms(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		specs         []rules.Spec
		extra         []rules.Rule
		wantLanguages []string
		wantGates     int
	}{
		{name: "no rules adds no language and no gate", wantLanguages: []string{"kept"}},
		{
			name:          "a rule that is no idiom adds nothing",
			extra:         []rules.Rule{globRule{[]string{"*.go"}}},
			wantLanguages: []string{"kept"},
		},
		{
			name:          "go-di adds one language and one gate",
			specs:         []rules.Spec{{Type: "go-di", Raw: []byte(goDIRule)}},
			wantLanguages: []string{"kept", "go-idioms"},
			wantGates:     1,
		},
		{
			name: "two go-di rules share one language",
			specs: []rules.Spec{
				{Type: "go-di", Raw: []byte(goDIRule)}, {Type: "go-di", Raw: []byte(goDIRule)},
			},
			wantLanguages: []string{"kept", "go-idioms"},
			wantGates:     2,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			built, err := compiledRegistry().Build(testCase.specs)
			if err != nil {
				t.Fatalf("Build() = %v", err)
			}

			cfg := Defaults()
			cfg.Rules = slices.Concat(built, testCase.extra)
			stage := identityStages(errors.New("kept"))
			env := stageEnv{goSource: typed.NewSource(nil)}

			if err = ruleHook(t.Context(), &cfg, env, stage); err != nil {
				t.Fatalf("ruleHook() = %v", err)
			}

			names := make([]string, 0, len(stage.languages))
			for _, language := range stage.languages {
				names = append(names, language.Name)
			}

			if !slices.Equal(names, testCase.wantLanguages) {
				t.Fatalf("languages = %v, want %v", names, testCase.wantLanguages)
			}

			if got := len(cfg.Auditing.Gates); got != testCase.wantGates {
				t.Fatalf("gates = %d, want %d", got, testCase.wantGates)
			}
		})
	}
}

type countingProvider struct{ calls *int }

//nolint:ireturn // idiom.Provider is declared to hand back its own Facts port.
func (provider countingProvider) Facts(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) (idiom.Facts, error) {
	*provider.calls++

	return struct{ idiom.Facts }{}, nil
}

func runShape(t *testing.T, pack idiom.Pack, shape string) {
	t.Helper()

	var err error

	switch shape {
	case "edges":
		_, err = pack.Edges(context.Background(), entities.Snapshot{}, nil)
	case "anchors":
		_, err = pack.Anchors(context.Background(), entities.Snapshot{}, nil)
	case "atoms":
		_, err = pack.Atoms(context.Background(), entities.Snapshot{}, nil)
	}

	if err != nil {
		t.Fatalf("%s = %v", shape, err)
	}
}

func TestPackBuildsFactsOncePerShape(t *testing.T) {
	t.Parallel()

	one := rules.Spec{Type: "go-di", Raw: []byte(goDIRule)}

	testCases := []struct {
		name  string
		shape string
		want  int
	}{
		{"edges of two rules share one facts build", "edges", 1},
		{"anchors that no rule offers build no facts", "anchors", 0},
		{"atoms that no rule offers build no facts", "atoms", 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			built, err := compiledRegistry().Build([]rules.Spec{one, one})
			if err != nil {
				t.Fatalf("Build() = %v", err)
			}

			calls := 0
			pack := idiom.NewPack(countingProvider{calls: &calls}, []string{".go"}, built...)

			runShape(t, pack, testCase.shape)

			if calls != testCase.want {
				t.Fatalf("Facts called %d times, want %d", calls, testCase.want)
			}
		})
	}
}

func TestRuleConstructors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		specs    []rules.Spec
		wantErr  error
		wantText string
		wantLen  int
	}{
		{name: "no rules build to nothing"},
		{
			name:     "a rule type nobody registered is refused and the known ones listed",
			specs:    []rules.Spec{{Type: "no-such-rule"}},
			wantErr:  rules.ErrUnknownType,
			wantText: "known: go-di",
		},
		{
			name:    "go-di builds a rule",
			specs:   []rules.Spec{{Type: "go-di", Raw: []byte(goDIRule)}},
			wantLen: 1,
		},
		{
			name:     "go-di names the rule it refuses",
			specs:    []rules.Spec{{Type: "go-di", Raw: []byte(`{"type":"go-di","bogus":1}`)}},
			wantErr:  di.ErrConfig,
			wantText: "rules[0] (go-di)",
		},
	}

	if got := len(ruleConstructors()); got != 1 {
		t.Fatalf("len(ruleConstructors()) = %d, want 1", got)
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			built, err := compiledRegistry().Build(testCase.specs)

			if testCase.wantErr != nil && !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Build() err = %v, want %v", err, testCase.wantErr)
			}

			if (err != nil) != (testCase.wantText != "") {
				t.Fatalf("Build() err = %v, want text %q", err, testCase.wantText)
			}

			if err != nil && !strings.Contains(err.Error(), testCase.wantText) {
				t.Fatalf("Build() err = %q, want it to say %q", err, testCase.wantText)
			}

			if err == nil && len(built) != testCase.wantLen {
				t.Fatalf("Build() = %v, want %d rules", built, testCase.wantLen)
			}
		})
	}
}
