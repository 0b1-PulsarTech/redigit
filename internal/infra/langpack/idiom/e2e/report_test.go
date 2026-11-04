package e2e

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"maps"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
)

type reportCase struct {
	name      string
	decls     []entities.Decl
	calls     []facts.Call
	wantLine  string
	wantLevel string
	wantWarns int
}

func requireReports(t *testing.T, suite entities.Decl, known world, testCases []reportCase) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var logged bytes.Buffer

			built, err := Constructor("go-e2e", slog.New(slog.NewTextHandler(&logged, nil)))(
				json.RawMessage(defaultCfg),
			)
			if err != nil {
				t.Fatalf("Constructor() = %v", err)
			}

			rule, _ := built.(*Rule)
			calls := maps.Clone(known)
			calls[suite.Key] = testCase.calls

			for range 2 {
				_, err = rule.Edges(t.Context(), entities.Snapshot{}, testCase.decls, calls)
				if err != nil {
					t.Fatalf("Edges() = %v", err)
				}
			}

			out := logged.String()
			if strings.Count(out, testCase.wantLine) != 1 {
				t.Fatalf("log = %q, want %q exactly once", out, testCase.wantLine)
			}

			if !strings.Contains(out, testCase.wantLevel) ||
				strings.Count(out, "level=WARN") != testCase.wantWarns {
				t.Fatalf("log = %q, want %d warnings and %s",
					out, testCase.wantWarns, testCase.wantLevel)
			}
		})
	}
}

func TestRuleReportsOncePerRunWithALevel(t *testing.T) {
	t.Parallel()

	set := newRouteSet()
	suite := entities.Decl{Key: suiteKey("test", "TestHealth")}
	withRoutes := append([]entities.Decl{suite}, set.decls...)

	requireReports(t, suite, set.known, []reportCase{
		{
			name:  "a blind suite warns",
			decls: []entities.Decl{suite},
			wantLine: "go-e2e: 1 test declarations in dirs, 1 without call facts (nested module?), " +
				"0 routes, 0 edges, 0 urls with no known route path",
			wantLevel: "level=WARN",
			wantWarns: 1,
		},
		{
			name:  "edges found are information",
			decls: withRoutes,
			calls: hitting(suite.Key, "/health", "/gone"),
			wantLine: "go-e2e: 1 test declarations in dirs, 0 without call facts (nested module?), " +
				"9 routes, 2 edges, 1 urls with no known route path",
			wantLevel: "level=INFO",
			wantWarns: 1,
		},
		{
			name:  "no listening declaration is information",
			decls: set.decls,
			wantLine: "go-e2e: 0 test declarations in dirs, 0 without call facts (nested module?), " +
				"9 routes, 0 edges, 0 urls with no known route path",
			wantLevel: "level=INFO",
		},
		{
			name:  "listening with no edge warns",
			decls: withRoutes,
			calls: hitting(suite.Key, "/gone"),
			wantLine: "go-e2e: 1 test declarations in dirs, 0 without call facts (nested module?), " +
				"9 routes, 0 edges, 1 urls with no known route path",
			wantLevel: "level=WARN",
			wantWarns: 2,
		},
		{
			name:  "the urls warning is said once and does not claim a url is unserved",
			decls: withRoutes,
			calls: hitting(suite.Key, "/gone"),
			wantLine: "go-e2e: no known route path for these urls, so no edge was added; " +
				"a group variable, a prefix that is not a constant or a URL built elsewhere is not seen",
			wantLevel: "level=WARN",
			wantWarns: 2,
		},
	})
}
