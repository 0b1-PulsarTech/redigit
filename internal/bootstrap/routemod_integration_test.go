//go:build integration

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

const (
	routerFull = `{"type":"go-router","handle":["*.GET","*.POST"],` +
		`"mount":["Mount*","*.Mount","*.Route"],"groupMethods":true}`
)

func pipelineResult(
	t *testing.T,
	dir, raw string,
) (entities.Graph, entities.Ordering, entities.Report) {
	t.Helper()

	var specs []rules.Spec
	if raw != "" {
		specs = []rules.Spec{{Type: "go-router", Raw: []byte(raw)}}
	}

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

	order, err := pipeline.Order(result.Graph)
	if err != nil {
		t.Fatalf("Order() = %v", err)
	}

	audit, err := pipeline.Audit(result.Graph, order)
	if err != nil {
		t.Fatalf("Audit() = %v", err)
	}

	return result.Graph, order, audit.Report
}

func routemodResult(
	t *testing.T,
	raw string,
) (entities.Graph, entities.Ordering, entities.Report) {
	t.Helper()

	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../infra/langpack/idiom/testdata/routemod")); err != nil {
		t.Fatalf("copy routemod: %v", err)
	}

	initFixtureRepo(t, dir)

	return pipelineResult(t, dir, raw)
}

func atkResult(t *testing.T, raw string) (entities.Graph, entities.Ordering, entities.Report) {
	t.Helper()

	dir := t.TempDir()
	fixtures := "../infra/langpack/idiom/testdata"
	stages := [][][2]string{
		{{"go.mod", "atk/go.mod"}, {"router/router.go", "atk/router/router.go"}},
		{
			{"users/users.go", "atk/users/users.go"},
			{"app/routes.go", "atk_routes_early.go.txt"},
			{"zother/other.go", "atk/zother/other.go"},
		},
		{
			{"orders/orders.go", "atk/orders/orders.go"},
			{"billing/billing.go", "atk/billing/billing.go"},
			{"app/routes.go", "atk_routes_two_callers.go.txt"},
		},
	}

	runFixtureGit(t, dir, "init", "-q", "-b", "main")
	runFixtureGit(t, dir, "config", "user.email", "tester@example.com")
	runFixtureGit(t, dir, "config", "user.name", "Tester")

	for _, stage := range stages {
		for _, file := range stage {
			body, err := os.ReadFile(filepath.Join(fixtures, file[1]))
			if err != nil {
				t.Fatalf("read %s: %v", file[1], err)
			}

			target := filepath.Join(dir, file[0])
			if err = os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			if err = os.WriteFile(target, body, 0o600); err != nil {
				t.Fatalf("write %s: %v", file[0], err)
			}
		}

		runFixtureGit(t, dir, "add", "-A")
		runFixtureGit(t, dir, "commit", "-q", "-m", "feat: "+stage[0][0])
	}

	return pipelineResult(t, dir, raw)
}

func atomMembers(graph entities.Graph) map[string][]entities.DeclKey {
	members := map[string][]entities.DeclKey{}

	for _, decl := range graph.Decls() {
		if decl.Atom != "" && strings.HasPrefix(decl.Atom, "go-router:") {
			members[decl.Atom] = append(members[decl.Atom], decl.Key)
		}
	}

	return members
}

func TestGoRouterBirthsAMountWithItsCaller(t *testing.T) {
	t.Parallel()

	graph, order, _ := routemodResult(t, routerFull)
	members := atomMembers(graph)

	testCases := []struct {
		name  string
		mount string
	}{
		{name: "a plain function call", mount: "MountUsers"},
		{name: "a func handed to a router method", mount: "MountBilling"},
		{name: "a call after the other mounts", mount: "MountAudit"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			mount := entities.DeclKey{
				File: "app/mounts.go",
				Kind: entities.KindFunc,
				Name: testCase.mount,
			}
			decl, _ := graph.Decl(mount)

			if decl.Atom == "" {
				t.Fatalf("%s is in no atom", mount)
			}

			var callers []entities.DeclKey

			for _, member := range members[decl.Atom] {
				if member.Kind == entities.KindStmt && strings.HasPrefix(member.Name, "Routes#") {
					callers = append(callers, member)
				}
			}

			if len(callers) == 0 {
				t.Fatalf(
					"%s shares its atom with no statement of Routes: %v",
					mount,
					members[decl.Atom],
				)
			}

			for _, caller := range callers {
				if order.Of(caller) != order.Of(mount) {
					t.Fatalf(
						"%s at %d, caller %s at %d",
						mount,
						order.Of(mount),
						caller,
						order.Of(caller),
					)
				}
			}
		})
	}
}

type benchTimes struct{ graph, audit time.Duration }

func writeSyntheticModule(b *testing.B) string {
	b.Helper()

	dir := b.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../infra/langpack/idiom/testdata/routemod")); err != nil {
		b.Fatalf("copy routemod: %v", err)
	}

	var handlers, routes strings.Builder

	handlers.WriteString("package app\n\nimport \"example.com/routemod/router\"\n\n")
	routes.WriteString("package app\n\nimport \"example.com/routemod/router\"\n\n")
	routes.WriteString("func Generated(mux *router.Router) {\n")

	for idx := range 500 {
		fmt.Fprintf(&handlers, "func Gen%d(ctx *router.Ctx) { ctx.Path = \"g%d\" }\n", idx, idx)
		fmt.Fprintf(&routes, "\tmux.GET(\"/gen/%d\", Gen%d)\n", idx, idx)
	}

	routes.WriteString("}\n")

	for name, body := range map[string]string{
		"gen_handlers.go": handlers.String(), "gen_routes.go": routes.String(),
	} {
		if err := os.WriteFile(filepath.Join(dir, "app", name), []byte(body), 0o600); err != nil {
			b.Fatalf("write %s: %v", name, err)
		}
	}

	initFixtureRepo(b, dir)

	return dir
}

func timePipeline(b *testing.B, repo string, specs []rules.Spec) benchTimes {
	b.Helper()

	built, err := compiledRegistry().Build(specs)
	if err != nil {
		b.Fatalf("Build() = %v", err)
	}

	pipeline, err := Start(b.Context(), Config{Repo: repo, Rev: "HEAD", Rules: built})
	if err != nil {
		b.Fatalf("Start() = %v", err)
	}

	defer func() { _ = pipeline.Close(context.WithoutCancel(b.Context())) }()

	begin := time.Now()

	result, err := pipeline.Graph(b.Context())
	if err != nil {
		b.Fatalf("Graph() = %v", err)
	}

	graphDone := time.Now()

	order, err := pipeline.Order(result.Graph)
	if err != nil {
		b.Fatalf("Order() = %v", err)
	}

	auditStart := time.Now()

	if _, err = pipeline.Audit(result.Graph, order); err != nil {
		b.Fatalf("Audit() = %v", err)
	}

	return benchTimes{graph: graphDone.Sub(begin), audit: time.Since(auditStart)}
}

func BenchmarkGoRouter(b *testing.B) {
	synthetic := writeSyntheticModule(b)
	routerSpec := rules.Spec{Type: "go-router", Raw: []byte(routerFull)}

	testCases := []struct {
		name  string
		repo  string
		specs []rules.Spec
	}{
		{name: "synthetic500/without", repo: synthetic},
		{name: "synthetic500/with", repo: synthetic, specs: []rules.Spec{routerSpec}},
	}

	for _, testCase := range testCases {
		b.Run(testCase.name, func(b *testing.B) {
			var graph, audit time.Duration

			for b.Loop() {
				got := timePipeline(b, testCase.repo, testCase.specs)
				graph += got.graph
				audit += got.audit
			}

			b.ReportMetric(float64(graph.Microseconds())/1000/float64(b.N), "graph-ms/op")
			b.ReportMetric(float64(audit.Microseconds())/1000/float64(b.N), "audit-ms/op")
		})
	}
}
