//go:build integration

package bootstrap

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/rules"
)

// TestStart runs the real Start against a throwaway git repository holding
// one tiny Go module: the one path in this package that names a concrete
// adapter, and the one this task's unit tests cannot reach without spawning a
// language server and creating a git worktree.
func TestStart(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not on PATH")
	}

	dir := t.TempDir()
	writeFixtureModule(t, dir)
	initFixtureRepo(t, dir)

	pipeline, err := Start(t.Context(), Config{
		Repo:          dir,
		Rev:           "HEAD",
		ServerCommand: "gopls",
		ServerArgs:    []string{"-mode=stdio"},
	})
	if err != nil {
		t.Fatalf("Start() = %v", err)
	}

	result, err := pipeline.Graph(t.Context())
	if err != nil {
		t.Fatalf("Graph() = %v", err)
	}

	if result.Graph.Len() == 0 {
		t.Fatal("expected at least one declaration in the graph")
	}

	// Close's own error-folding is pinned deterministically in
	// pipeline_test.go; a real gopls process can exit on its own the instant
	// shutdown/exit lands, and that broken pipe after a clean handshake is a
	// clean close, so Close must succeed and the checkout must be gone.
	if err := pipeline.Close(context.WithoutCancel(t.Context())); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}

	if got := fixtureWorktreeCount(t, dir); got != 1 {
		t.Fatalf("worktree count after Close = %d, want 1 (the repo's own)", got)
	}
}

// writeFixtureModule seeds a repository with one Go file holding a
// declaration and a caller, so the index has at least one edge to find.
func writeFixtureModule(t testing.TB, dir string) {
	t.Helper()

	files := map[string]string{
		"go.mod": "module example.com/fixture\n\ngo 1.26\n",
		"main.go": "package fixture\n\n" +
			"func Target() int { return 1 }\n\n" +
			"func Caller() int { return Target() }\n",
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func initFixtureRepo(t testing.TB, dir string) {
	t.Helper()

	runFixtureGit(t, dir, "init", "-q", "-b", "main")
	runFixtureGit(t, dir, "config", "user.email", "tester@example.com")
	runFixtureGit(t, dir, "config", "user.name", "Tester")
	runFixtureGit(t, dir, "add", "-A")
	runFixtureGit(t, dir, "commit", "-q", "-m", "fixture module")
}

func runFixtureGit(t testing.TB, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// fixtureWorktreeCount counts the worktrees git still has registered for
// dir's repository. Close is supposed to leave only the repository's own.
func fixtureWorktreeCount(t testing.TB, dir string) int {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "git", "-C", dir, "worktree", "list", "--porcelain").
		CombinedOutput()
	if err != nil {
		t.Fatalf("git worktree list: %v\n%s", err, out)
	}

	count := 0

	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			count++
		}
	}

	return count
}

// TestSnapshotRootIsTheMaterialisedRevision pins a defect found only by running
// against a real repository: a language pack that reads a sibling file from
// disk was reading the repository's WORKING TREE while everything else read the
// revision. It failed on a file the revision holds and the checkout does not,
// and would have silently read stale bytes for a file both hold.
func TestSnapshotRootIsTheMaterialisedRevision(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not on PATH")
	}

	dir := t.TempDir()
	writeFixtureModule(t, dir)
	initFixtureRepo(t, dir)

	ctx := t.Context()

	pl, err := Start(ctx, Config{
		Repo:          dir,
		Rev:           "HEAD",
		ServerCommand: "gopls",
		ServerArgs:    []string{"-mode=stdio"},
	})
	if err != nil {
		t.Fatalf("Start() = %v", err)
	}

	defer func() { _ = pl.Close(context.WithoutCancel(ctx)) }()

	root := pl.Snapshot().Root
	if root == dir {
		t.Fatalf("Snapshot().Root is the repository %q, not the checkout", root)
	}

	if _, err := os.Stat(root); err != nil {
		t.Fatalf("Snapshot().Root %q does not exist: %v", root, err)
	}
}

func TestCloseAfterARealLanguageServerSession(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not on PATH")
	}

	dir := t.TempDir()
	writeFixtureModule(t, dir)
	initFixtureRepo(t, dir)

	pipeline, err := Start(t.Context(), Config{
		Repo:          dir,
		Rev:           "HEAD",
		ServerCommand: "gopls",
		ServerArgs:    []string{"-mode=stdio"},
		Resolver:      ResolverLSP,
	})
	if err != nil {
		t.Fatalf("Start() = %v", err)
	}

	root := pipeline.Snapshot().Root

	if _, err := pipeline.Graph(t.Context()); err != nil {
		t.Fatalf("Graph() = %v", err)
	}

	if err := pipeline.Close(context.WithoutCancel(t.Context())); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}

	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("checkout %q after Close: stat error = %v, want not exist", root, err)
	}
}

func TestStartRefusesAnUnmatchedGlobBeforeCheckout(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFixtureModule(t, dir)
	initFixtureRepo(t, dir)

	configPath := "/etc/redigit.json"

	pipeline, err := Start(t.Context(), Config{
		Repo:          dir,
		Rev:           "HEAD",
		ServerCommand: "gopls",
		ServerArgs:    []string{"-mode=stdio"},
		Rules:         []rules.Rule{globRule{[]string{"*.rs"}}},
		ConfigPath:    configPath,
	})
	if pipeline != nil {
		_ = pipeline.Close(context.WithoutCancel(t.Context()))
	}

	if !errors.Is(err, rules.ErrUnmatchedGlob) {
		t.Fatalf("Start() err = %v, want ErrUnmatchedGlob", err)
	}

	if !strings.Contains(err.Error(), configPath) {
		t.Fatalf("Start() err = %q, want the config file path", err)
	}

	if got := fixtureWorktreeCount(t, dir); got != 1 {
		t.Fatalf("worktree count = %d, want 1 (the repo's own): a checkout was made", got)
	}
}

func TestStartHooksSeeTheCheckoutRoot(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
	}{
		{name: "a hook reads the snapshot root the checkout gave"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeFixtureModule(t, dir)
			initFixtureRepo(t, dir)

			var seen string

			hooks := []namedHook{{name: "probe", run: func(
				_ context.Context, _ *Config, env stageEnv, _ *stages,
			) error {
				seen = env.opened.snapshot.Root

				return nil
			}}}

			pipeline, err := start(t.Context(), Config{Repo: dir, Rev: "HEAD"}, hooks)
			if err != nil {
				t.Fatalf("start() = %v", err)
			}

			defer func() { _ = pipeline.Close(context.WithoutCancel(t.Context())) }()

			if seen == "" || seen == dir || seen != pipeline.Snapshot().Root {
				t.Fatalf("hook saw root %q, want the checkout %q", seen, pipeline.Snapshot().Root)
			}
		})
	}
}

func TestStartClosesWhatAHookOpened(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		failLater  bool
		closeAfter bool
	}{
		{name: "a later step fails, so Start closes it once", failLater: true},
		{name: "Start succeeds, so Close closes it once", closeAfter: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeFixtureModule(t, dir)
			initFixtureRepo(t, dir)

			var order []string

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			hooks := []namedHook{{name: "probe", run: func(
				_ context.Context, _ *Config, _ stageEnv, built *stages,
			) error {
				built.closers = append(built.closers, orderedCloser{"hook", &order})

				if testCase.failLater {
					cancel()
				}

				return nil
			}}}

			pipeline, err := start(ctx, Config{Repo: dir, Rev: "HEAD"}, hooks)
			if testCase.failLater != (err != nil) {
				t.Fatalf("start() = %v, want failure %v", err, testCase.failLater)
			}

			if len(order) != 0 && !testCase.failLater {
				t.Fatalf("closed %v before Close", order)
			}

			if testCase.closeAfter {
				if err = pipeline.Close(context.WithoutCancel(t.Context())); err != nil {
					t.Fatalf("Close() = %v", err)
				}
			}

			if !slices.Equal(order, []string{"hook"}) {
				t.Fatalf("closed %v, want the hook closer once", order)
			}
		})
	}
}

func TestStartWiresTheGoDIRule(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		specs          []rules.Spec
		wantProvides   int
		wantRegistered int
		wantGate       bool
	}{
		{name: "without a rule the graph has no DI edge"},
		{
			name:           "go-di links registers to their argument types and resolves to registrations",
			specs:          []rules.Spec{{Type: "go-di", Raw: []byte(goDIRule)}},
			wantProvides:   12,
			wantRegistered: 1,
			wantGate:       true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, pipeline, err := startDimod(t, testCase.specs)
			if err != nil {
				t.Fatalf("Start() = %v", err)
			}

			defer func() { _ = pipeline.Close(context.WithoutCancel(t.Context())) }()

			byReason, gated := diFindings(t, pipeline)

			if byReason[entities.ReasonProvides] != testCase.wantProvides ||
				byReason[entities.ReasonRegistered] != testCase.wantRegistered {
				t.Fatalf("edges = %v, want provides %d registered %d",
					byReason, testCase.wantProvides, testCase.wantRegistered)
			}

			if gated != testCase.wantGate {
				t.Fatalf("DI gate measured = %v, want %v", gated, testCase.wantGate)
			}
		})
	}
}

func startDimod(t *testing.T, specs []rules.Spec) (string, *Pipeline, error) {
	t.Helper()

	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../infra/langpack/idiom/testdata/dimod")); err != nil {
		t.Fatalf("copy dimod: %v", err)
	}

	initFixtureRepo(t, dir)

	built, err := compiledRegistry().Build(specs)
	if err != nil {
		t.Fatalf("Build() = %v", err)
	}

	pipeline, err := Start(t.Context(), Config{Repo: dir, Rev: "HEAD", Rules: built})

	return dir, pipeline, err
}

func diFindings(t *testing.T, pipeline *Pipeline) (map[entities.EdgeReason]int, bool) {
	t.Helper()

	result, err := pipeline.Graph(t.Context())
	if err != nil {
		t.Fatalf("Graph() = %v", err)
	}

	order, err := pipeline.Order(result.Graph)
	if err != nil {
		t.Fatalf("Order() = %v", err)
	}

	byReason := map[entities.EdgeReason]int{}

	for _, edge := range result.Graph.Edges() {
		byReason[edge.Reason]++

		late := order.Of(edge.Provider) > order.Of(edge.Dependent)
		if edge.Reason == entities.ReasonRegistered && late {
			t.Fatalf("registration %v is after its resolve site %v", edge.Provider, edge.Dependent)
		}
	}

	audit, err := pipeline.Audit(result.Graph, order)
	if err != nil {
		t.Fatalf("Audit() = %v", err)
	}

	at := slices.IndexFunc(audit.Report.Integrity, func(measure entities.Measure) bool {
		return strings.HasPrefix(measure.Name, "DI resolve sites")
	})
	if at < 0 {
		return byReason, false
	}

	if measure := audit.Report.Integrity[at]; measure.Value != 1 || measure.Total != 1 {
		t.Fatalf("DI gate = %d of %d, want 1 of 1", measure.Value, measure.Total)
	}

	return byReason, true
}
