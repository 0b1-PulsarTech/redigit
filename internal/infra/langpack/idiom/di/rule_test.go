package di

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"path"
	"slices"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

func newRule(t *testing.T, raw string, logs *bytes.Buffer) *Rule {
	t.Helper()

	handler := slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(handler)

	built, err := Constructor("go-di", logger)(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("Constructor() = %v", err)
	}

	rule, isRule := built.(*Rule)
	if !isRule {
		t.Fatalf("Constructor() built %T, want *Rule", built)
	}

	return rule
}

func TestConstructor(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		raw     string
		wantErr error
	}{
		{
			name: "a config that registers builds the rule",
			raw:  `{"type":"go-di","registerFuncs":["R"]}`,
		},
		{name: "no register functions", raw: `{"type":"go-di"}`, wantErr: ErrNoRegisterFuncs},
		{
			name:    "an unknown key",
			raw:     `{"type":"go-di","registerFuncs":["R"],"x":1}`,
			wantErr: ErrConfig,
		},
		{
			name:    "a malformed pattern",
			raw:     `{"type":"go-di","registerFuncs":["Reg[ister"]}`,
			wantErr: ErrBadPattern,
		},
		{
			name:    "a pattern listed twice",
			raw:     `{"type":"go-di","registerFuncs":["R","R"]}`,
			wantErr: ErrDuplicatePattern,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			built, err := Constructor("go-di", slog.Default())(json.RawMessage(testCase.raw))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Constructor() err = %v, want %v", err, testCase.wantErr)
			}

			if _, isRule := built.(*Rule); isRule != (testCase.wantErr == nil) {
				t.Fatalf("Constructor() built %T with err %v", built, err)
			}
		})
	}
}

func TestRuleIsAGlobberOfItsPaths(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "the configured paths",
			raw:  `{"type":"go-di","registerFuncs":["R"],"paths":["cmd/","a/*.go"]}`,
			want: []string{"cmd/", "a/*.go"},
		},
		{name: "no path lists no glob", raw: `{"type":"go-di","registerFuncs":["R"]}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := newRule(t, testCase.raw, &bytes.Buffer{})

			var _ rules.Globber = rule

			if got := rule.Globs(); !slices.Equal(got, testCase.want) {
				t.Fatalf("Globs() = %v, want %v", got, testCase.want)
			}

			if rule.Type() != "go-di" {
				t.Fatalf("Type() = %q, want go-di", rule.Type())
			}
		})
	}
}

func TestRuleEdges(t *testing.T) {
	t.Parallel()

	first, second := stmtKey("Boot", "1"), stmtKey("Boot", "2")
	known := world{calls: map[entities.DeclKey][]facts.Call{
		first:  {register(first, "RegisterOne", "p.Repo")},
		second: {get(second, "Get", "p.Repo")},
	}}
	decls := []entities.Decl{declOf(first), declOf(second)}
	raw := `{"type":"go-di","registerFuncs":["Register*"],"resolveFuncs":["Get"]}`

	testCases := []struct {
		name      string
		patterns  []string
		want      []entities.Edge
		wantError error
	}{
		{
			name: "a resolve linked to its registration with nothing to warn about",
			want: []entities.Edge{registered(second, first)},
		},
		{
			name:      "a pattern that cannot match",
			patterns:  []string{"Reg[ister"},
			wantError: path.ErrBadPattern,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer

			rule := newRule(t, raw, &logs)
			if testCase.patterns != nil {
				rule.cfg.RegisterFuncs = testCase.patterns
			}

			got, err := rule.Edges(
				context.Background(), entities.Snapshot{}, decls,
				known.facts(gomock.NewController(t)),
			)
			if !errors.Is(err, testCase.wantError) || !slices.Equal(got, testCase.want) {
				t.Fatalf("Edges() = %v, %v, want %v, %v",
					got, err, testCase.want, testCase.wantError)
			}

			if logs.Len() != 0 {
				t.Fatalf("a rule with register sites logged %q", logs.String())
			}
		})
	}
}

func TestRuleWarnsOnce(t *testing.T) {
	t.Parallel()

	site := fnKey("Boot")
	first, second, third := stmtKey("Boot", "1"), stmtKey("Boot", "2"), stmtKey("Boot", "3")
	iface, pg, mem := typeKey("Store"), typeKey("Pg"), typeKey("Mem")
	registers := `{"type":"go-di","registerFuncs":["Register"],"resolveFuncs":["Get"]}`

	testCases := []struct {
		name  string
		raw   string
		decls []entities.Decl
		known world
		want  string
	}{
		{
			name:  "nothing registers",
			raw:   `{"type":"go-di","registerFuncs":["Nothing*"],"resolveFuncs":["Get"]}`,
			decls: []entities.Decl{declOf(site)},
			known: world{calls: map[entities.DeclKey][]facts.Call{
				site: {get(site, "Get", "p.Repo")},
			}},
			want: "0 register sites",
		},
		{
			name:  "a resolve is ambiguous",
			raw:   registers,
			decls: []entities.Decl{declOf(first), declOf(second), declOf(third)},
			known: world{
				calls: map[entities.DeclKey][]facts.Call{
					first:  {keyed(register(first, "Register", "p.Pg"), pg)},
					second: {keyed(register(second, "Register", "p.Mem"), mem)},
					third:  {keyed(get(third, "Get", "p.Store"), iface)},
				},
				impls: map[entities.DeclKey][]entities.DeclKey{iface: {mem, pg}},
			},
			want: "ambiguousResolves=1",
		},
		{
			name:  "a call has no static bound type",
			raw:   registers,
			decls: []entities.Decl{declOf(site)},
			known: world{calls: map[entities.DeclKey][]facts.Call{
				site: {register(site, "Register", "p.A"), get(site, "Get", "")},
			}},
			want: "no static bound type",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer

			rule := newRule(t, testCase.raw, &logs)
			mock := testCase.known.facts(gomock.NewController(t))

			for range 2 {
				_, err := rule.Edges(
					context.Background(), entities.Snapshot{}, testCase.decls, mock,
				)
				if err != nil {
					t.Fatalf("Edges() = %v", err)
				}
			}

			count := strings.Count(logs.String(), testCase.want)
			if count != 1 {
				t.Fatalf("warned %d times of %q: %q", count, testCase.want, logs.String())
			}
		})
	}
}

func TestRuleGateBindsTheConfig(t *testing.T) {
	t.Parallel()

	first, second := stmtKey("Boot", "1"), stmtKey("Boot", "2")
	known := world{calls: map[entities.DeclKey][]facts.Call{
		first:  {register(first, "Register", "p.Repo")},
		second: {get(second, "Get", "p.Repo")},
	}}

	testCases := []struct {
		name      string
		factsErr  error
		wantTotal int
		wantText  string
	}{
		{name: "the configured functions are measured", wantTotal: 1},
		{
			name:     "a facts failure is reported as not measured",
			factsErr: errNoFunc,
			wantText: "not measured",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			raw := `{"type":"go-di","registerFuncs":["Register"],"resolveFuncs":["Get"]}`
			rule := newRule(t, raw, &bytes.Buffer{})
			mock := known.facts(gomock.NewController(t))

			built := rule.Gate(func([]entities.Decl) (idiom.Facts, error) {
				if testCase.factsErr != nil {
					return nil, testCase.factsErr
				}

				return mock, nil
			})
			if built.Name() != "go-di" {
				t.Fatalf("Name() = %q, want go-di", built.Name())
			}

			graph := entities.NewGraph([]entities.Decl{declOf(first), declOf(second)}, nil, nil)
			placed := map[entities.DeclKey]entities.Position{first: 1, second: 2}
			order := entities.Ordering{At: placed}

			measured := built.Measure(graph, order)
			if measured.Total != testCase.wantTotal ||
				!strings.Contains(strings.Join(measured.Excluded, "|"), testCase.wantText) {
				t.Fatalf("Measure() = %+v, want %d sites and %q",
					measured, testCase.wantTotal, testCase.wantText)
			}
		})
	}
}

func TestRuleReportsTheFindingOfEachPass(t *testing.T) {
	t.Parallel()

	one, two, three := fnKey("One"), fnKey("Two"), fnKey("Three")
	known := world{calls: map[entities.DeclKey][]facts.Call{
		one:   {register(one, "Register", "p.A"), get(one, "Get", "")},
		two:   {get(two, "Get", "")},
		three: {register(three, "Register", "p.A")},
	}}
	raw := `{"type":"go-di","registerFuncs":["Register"],"resolveFuncs":["Get"]}`

	testCases := []struct {
		name   string
		passes [][]entities.Decl
		want   []string
	}{
		{
			name:   "a count that grows is reported again",
			passes: [][]entities.Decl{{declOf(one)}, {declOf(one), declOf(two)}},
			want:   []string{"untypedCalls=1", "untypedCalls=2"},
		},
		{
			name:   "a finding a later pass no longer has is said to be gone",
			passes: [][]entities.Decl{{declOf(one)}, {declOf(three)}},
			want:   []string{"untypedCalls=1", "go-di: no finding on the latest pass"},
		},
		{
			name:   "the same finding is reported once",
			passes: [][]entities.Decl{{declOf(one)}, {declOf(one)}},
			want:   []string{"untypedCalls=1"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer

			rule := newRule(t, raw, &logs)
			mock := known.facts(gomock.NewController(t))

			for _, decls := range testCase.passes {
				_, err := rule.Edges(context.Background(), entities.Snapshot{}, decls, mock)
				if err != nil {
					t.Fatalf("Edges() = %v", err)
				}
			}

			lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
			if len(lines) != len(testCase.want) {
				t.Fatalf("logged %q, want %d lines", logs.String(), len(testCase.want))
			}

			for at, want := range testCase.want {
				if !strings.Contains(lines[at], want) {
					t.Fatalf("line %d = %q, want it to hold %q", at, lines[at], want)
				}
			}
		})
	}
}
