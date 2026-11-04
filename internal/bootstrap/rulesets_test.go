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
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom/e2e"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom/router"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/pinrule"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

const (
	goDIRule     = `{"type":"go-di","registerFuncs":["Register*"],"resolveFuncs":["Get"]}`
	goRouterRule = `{"type":"go-router","handle":["*.GET"],"mount":["Mount*"],"groupMethods":true}`
	goE2ERule    = `{"type":"go-e2e","router":{"handle":["*.GET"]},"dirs":["test/"]}`
	pinRule      = `{"type":"pin","decl":"a.go:func:A","after":"b.go:func:B"}`
)

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

	diSpec := rules.Spec{Type: "go-di", Raw: []byte(goDIRule)}
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
		{
			name:          "go-router adds one language and one gate",
			specs:         []rules.Spec{{Type: "go-router", Raw: []byte(goRouterRule)}},
			wantLanguages: []string{"kept", "go-idioms"},
			wantGates:     1,
		},
		{
			name:          "go-e2e adds one language and no gate",
			specs:         []rules.Spec{{Type: "go-e2e", Raw: []byte(goE2ERule)}},
			wantLanguages: []string{"kept", "go-idioms"},
		},
		{
			name: "go-e2e beside go-router shares one language",
			specs: []rules.Spec{
				{Type: "go-router", Raw: []byte(goRouterRule)},
				{Type: "go-e2e", Raw: []byte(goE2ERule)},
			},
			wantLanguages: []string{"kept", "go-idioms"},
			wantGates:     1,
		},
		{
			name:          "go-di and go-router share one language",
			specs:         []rules.Spec{diSpec, {Type: "go-router", Raw: []byte(goRouterRule)}},
			wantLanguages: []string{"kept", "go-idioms"},
			wantGates:     2,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			checkHook(t, testCase.specs, testCase.extra, testCase.wantLanguages, testCase.wantGates)
		})
	}
}

func TestRuleHookWiresPins(t *testing.T) {
	t.Parallel()

	diSpec := rules.Spec{Type: "go-di", Raw: []byte(goDIRule)}
	pinSpec := rules.Spec{Type: "pin", Raw: []byte(pinRule)}
	testCases := []struct {
		name          string
		specs         []rules.Spec
		wantLanguages []string
		wantGates     int
	}{
		{
			name:          "a pin adds the pins language and no idiom language or gate",
			specs:         []rules.Spec{pinSpec},
			wantLanguages: []string{"kept", "pins"},
		},
		{
			name:          "a pin beside go-di adds both languages",
			specs:         []rules.Spec{diSpec, pinSpec},
			wantLanguages: []string{"kept", "go-idioms", "pins"},
			wantGates:     1,
		},
		{
			name:          "two pins share one language",
			specs:         []rules.Spec{pinSpec, pinSpec},
			wantLanguages: []string{"kept", "pins"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			checkHook(t, testCase.specs, nil, testCase.wantLanguages, testCase.wantGates)
		})
	}
}

func checkHook(
	t *testing.T,
	specs []rules.Spec,
	extra []rules.Rule,
	wantLanguages []string,
	wantGates int,
) {
	t.Helper()

	built, err := compiledRegistry().Build(specs)
	if err != nil {
		t.Fatalf("Build() = %v", err)
	}

	cfg := Defaults()
	cfg.Rules = slices.Concat(built, extra)
	stage := identityStages(errors.New("kept"))
	env := stageEnv{goSource: typed.NewSource(nil)}

	if err = ruleHook(t.Context(), &cfg, env, stage); err != nil {
		t.Fatalf("ruleHook() = %v", err)
	}

	names := make([]string, 0, len(stage.languages))
	for _, language := range stage.languages {
		names = append(names, language.Name)
	}

	if !slices.Equal(names, wantLanguages) {
		t.Fatalf("languages = %v, want %v", names, wantLanguages)
	}

	if got := len(cfg.Auditing.Gates); got != wantGates {
		t.Fatalf("gates = %d, want %d", got, wantGates)
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
			wantText: "known: go-di, go-e2e, go-router, pin",
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
		{
			name:    "go-router builds a rule",
			specs:   []rules.Spec{{Type: "go-router", Raw: []byte(goRouterRule)}},
			wantLen: 1,
		},
		{
			name: "go-router names the rule it refuses",
			specs: []rules.Spec{
				{Type: "go-router", Raw: []byte(`{"type":"go-router","mount":["M*"]}`)},
			},
			wantErr:  router.ErrNoHandle,
			wantText: "rules[0] (go-router)",
		},
		{
			name:    "pin builds a rule",
			specs:   []rules.Spec{{Type: "pin", Raw: []byte(pinRule)}},
			wantLen: 1,
		},
		{
			name:     "pin names the rule it refuses",
			specs:    []rules.Spec{{Type: "pin", Raw: []byte(`{"type":"pin","decl":"a"}`)}},
			wantErr:  pinrule.ErrMissingField,
			wantText: "rules[0] (pin)",
		},
	}

	if got := len(ruleConstructors()); got != 4 {
		t.Fatalf("len(ruleConstructors()) = %d, want 4", got)
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

func TestRuleConstructorsGoE2E(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		raw      string
		wantErr  error
		wantText string
	}{
		{name: "go-e2e builds a rule", raw: goE2ERule},
		{
			name: "go-e2e takes a requests list",
			raw:  `{"type":"go-e2e","router":{"handle":["*.GET"]},"dirs":["test/"],"requests":["Get"]}`,
		},
		{
			name:     "go-e2e refuses a dir that is a pattern",
			raw:      `{"type":"go-e2e","router":{"handle":["*.GET"]},"dirs":["t*/"]}`,
			wantErr:  e2e.ErrBadDir,
			wantText: "rules[0] (go-e2e)",
		},
		{
			name:     "go-e2e names the rule it refuses",
			raw:      `{"type":"go-e2e","router":{"handle":["*.GET"]}}`,
			wantErr:  e2e.ErrNoDirs,
			wantText: "rules[0] (go-e2e)",
		},
		{
			name:     "go-e2e refuses a router that handles nothing",
			raw:      `{"type":"go-e2e","router":{},"dirs":["test/"]}`,
			wantErr:  router.ErrNoHandle,
			wantText: "rules[0] (go-e2e)",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			built, err := compiledRegistry().Build([]rules.Spec{
				{Type: "go-e2e", Raw: []byte(testCase.raw)},
			})
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Build() err = %v, want %v", err, testCase.wantErr)
			}

			if err != nil && !strings.Contains(err.Error(), testCase.wantText) {
				t.Fatalf("Build() err = %q, want it to say %q", err, testCase.wantText)
			}

			if err == nil && len(built) != 1 {
				t.Fatalf("Build() = %v, want 1 rule", built)
			}
		})
	}
}
