//go:build integration

package bootstrap

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

func e2eGraph(t *testing.T, specs []rules.Spec) entities.Graph {
	t.Helper()

	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../infra/langpack/idiom/testdata/routemod")); err != nil {
		t.Fatalf("copy routemod: %v", err)
	}

	if err := os.CopyFS(dir, os.DirFS("../infra/langpack/idiom/e2e/testdata")); err != nil {
		t.Fatalf("copy e2e testdata: %v", err)
	}

	initFixtureRepo(t, dir)

	built, err := compiledRegistry().Build(specs)
	if err != nil {
		t.Fatalf("Build() = %v", err)
	}

	pipeline, err := Start(t.Context(), Config{Repo: dir, Rev: "HEAD", Rules: built})
	if err != nil {
		t.Fatalf("Start() = %v", err)
	}

	defer func() { _ = pipeline.Close(context.WithoutCancel(t.Context())) }()

	result, err := pipeline.Graph(t.Context())
	if err != nil {
		t.Fatalf("Graph() = %v", err)
	}

	return result.Graph
}

func headOf(key entities.DeclKey) string {
	name, _, _ := strings.Cut(key.Name, "#")

	return key.File + ":" + name
}

func TestGoE2ERuleMakesATestWaitForItsMount(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		specs []rules.Spec
		want  []string
	}{
		{name: "no rule adds no edge"},
		{
			name: "the rule adds the waits of the calling tests",
			specs: []rules.Spec{{Type: "go-e2e", Raw: []byte(
				`{"type":"go-e2e","router":{"handle":["*.GET","*.POST"],` +
					`"mount":["Mount*","*.Mount","*.Route"]},"dirs":["test/"]}`,
			)}},
			want: []string{
				"test/api_test.go:TestCreateOrder -> app/routes.go:Routes",
				"test/api_test.go:TestPing -> app/routes.go:Routes",
				"test/api_test.go:TestUser -> app/mounts.go:MountUsers",
				"test/api_test.go:TestUser -> app/routes.go:Routes",
				"test/api_test.go:TestUsers -> app/mounts.go:MountUsers",
				"test/api_test.go:TestUsers -> app/routes.go:Routes",
			},
		},
	}

	without := e2eGraph(t, nil)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				with  = e2eGraph(t, testCase.specs)
				added []string
				raw   int
			)

			for _, edge := range with.Edges() {
				if edge.Reason == entities.ReasonE2E {
					raw++

					added = append(added, headOf(edge.Dependent)+" -> "+headOf(edge.Provider))
				}
			}

			slices.Sort(added)

			if got := slices.Compact(added); !slices.Equal(got, testCase.want) {
				t.Fatalf("e2e edges = %v, want %v", got, testCase.want)
			}

			if got, base := len(with.Edges()), len(without.Edges()); got-base != raw {
				t.Fatalf("edges %d against %d unconfigured, want only the %d added", got, base, raw)
			}

			for _, edge := range without.Edges() {
				if edge.Reason == entities.ReasonE2E || edge.Reason == entities.ReasonTestOf {
					t.Fatalf("unconfigured graph has %v", edge)
				}
			}
		})
	}
}
