package checkrun

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

const day = 24 * time.Hour

func agedDir(t *testing.T, path string, mtime time.Time) {
	t.Helper()

	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}

	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

func goCacheOf(t *testing.T, root string, forced []string) (value string, env []string) {
	t.Helper()

	run, keyed := stepEnv(goUnit(), forced, "")

	var err error
	if env, _, err = withGoCache(root, run, keyed); err != nil {
		t.Fatalf("withGoCache: %v", err)
	}

	for _, entry := range env {
		if after, found := strings.CutPrefix(entry, "GOCACHE="); found {
			value = after
		}
	}

	return value, env
}

func TestWithGoCacheKeysTheDirectoryOnTheHashedEnvironment(t *testing.T) {
	testCases := []struct {
		name     string
		key      string
		wantSame bool
	}{
		{"CPATH", "CPATH", false},
		{"an unknown variable", "REDIGIT_TEST_UNKNOWN", false},
		{"CGO_CFLAGS", "CGO_CFLAGS", false},
		{"TMPDIR", "TMPDIR", true},
		{"HOME", "HOME", true},
		{"the inherited GOCACHE", "GOCACHE", true},
		{"PWD", "PWD", true},
		{"LANG", "LANG", false},
		{"LC_ALL", "LC_ALL", false},
		{"CLAUDE_CODE_SESSION_ID", "CLAUDE_CODE_SESSION_ID", true},
		{"STARSHIP_SESSION_KEY", "STARSHIP_SESSION_KEY", true},
		{"GNOME_TERMINAL_SCREEN", "GNOME_TERMINAL_SCREEN", true},
		{"VTE_VERSION", "VTE_VERSION", true},
		{"SSH_AGENT_PID", "SSH_AGENT_PID", true},
		{"ZELLIJ_PANE_ID", "ZELLIJ_PANE_ID", true},
		{"VSCODE_IPC_HOOK_CLI", "VSCODE_IPC_HOOK_CLI", true},
		{"VSCODE_GIT_IPC_HANDLE", "VSCODE_GIT_IPC_HANDLE", true},
		{"ATUIN_SESSION", "ATUIN_SESSION", true},
		{"NVIM", "NVIM", true},
		{"TERM_SESSION_ID", "TERM_SESSION_ID", true},
		{"DIRENV_DIR", "DIRENV_DIR", true},
		{"SHELL_SESSION_ID", "SHELL_SESSION_ID", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()

			t.Setenv(testCase.key, "before")

			before, _ := goCacheOf(t, root, nil)

			t.Setenv(testCase.key, "after")

			after, _ := goCacheOf(t, root, nil)
			if (before == after) != testCase.wantSame {
				t.Fatalf("%s: same = %v, want %v", testCase.key, before == after, testCase.wantSame)
			}
		})
	}
}

func TestWithGoCacheKeysAForcedVariableAndOwnsTheDirectory(t *testing.T) {
	root := t.TempDir()

	t.Setenv("GOCACHE", "/inherited/gocache")
	t.Setenv("HOME", "before")

	plain, _ := goCacheOf(t, root, nil)

	t.Setenv("HOME", "after")

	forced, env := goCacheOf(t, root, []string{"HOME"})
	if forced == plain {
		t.Fatalf("a forced HOME shares %s with the unforced env", plain)
	}

	if !strings.HasPrefix(forced, filepath.Join(root, "gocache")+string(filepath.Separator)) {
		t.Fatalf("GOCACHE = %s, want it under %s/gocache", forced, root)
	}

	if key := filepath.Base(forced); len(key) < 16 {
		t.Fatalf("key %q is shorter than 16 hex digits", key)
	}

	for _, entry := range env {
		if strings.Contains(entry, "/inherited/gocache") {
			t.Fatalf("step env still carries the inherited cache: %s", entry)
		}
	}
}

func TestGoUnitsRefuseAnEffectiveGoCacheProg(t *testing.T) {
	useFakeGo(t)

	testCases := []struct {
		name    string
		prog    string
		unit    verifying.Unit
		wantErr error
	}{
		{"a go unit with a cache program", "cacheprog --remote", goUnit(), ErrCacheProg},
		{"a go unit without one", "", goUnit(), nil},
		{"a custom language is not probed", "cacheprog", rustUnit(), nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("FAKEGO_GOCACHEPROG", testCase.prog)

			slotDir := filepath.Join(t.TempDir(), "slot")
			if err := os.MkdirAll(filepath.Join(slotDir, "mod"), 0o750); err != nil {
				t.Fatalf("module dir: %v", err)
			}

			steps := []Step{fake("pass")}
			runner := mustRunner(t, Config{
				Languages: map[string]Language{"go": {Steps: steps}, "rust": {Steps: steps}},
				CacheDir:  t.TempDir(),
			})

			if _, err := runner.Check(t.Context(), slotDir, testCase.unit); !errors.Is(
				err, testCase.wantErr,
			) {
				t.Fatalf("check err = %v, want %v", err, testCase.wantErr)
			}

			if _, err := runner.Identity(t.Context(), testCase.unit); !errors.Is(
				err, testCase.wantErr,
			) {
				t.Fatalf("identity err = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestCheckGivesEveryLanguageItsOwnGoCache(t *testing.T) {
	useFakeGo(t)
	t.Setenv("GOCACHE", "/inherited/gocache")

	testCases := []struct {
		name string
		unit verifying.Unit
	}{
		{"go", goUnit()},
		{"a custom language", rustUnit()},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			dump, root := filepath.Join(t.TempDir(), "env"), t.TempDir()
			slotDir := filepath.Join(t.TempDir(), "slot")

			if err := os.MkdirAll(filepath.Join(slotDir, "mod"), 0o750); err != nil {
				t.Fatalf("module dir: %v", err)
			}

			steps := []Step{fake("env", dump)}
			runner := mustRunner(t, Config{
				Languages: map[string]Language{"go": {Steps: steps}, "rust": {Steps: steps}},
				CacheDir:  root,
			})

			if _, err := runner.Check(t.Context(), slotDir, testCase.unit); err != nil {
				t.Fatalf("check: %v", err)
			}

			content, err := os.ReadFile(dump)
			if err != nil {
				t.Fatalf("read dump: %v", err)
			}

			want := "\nGOCACHE=" + filepath.Join(root, "gocache") + string(filepath.Separator)
			if got := "\n" + string(content); !strings.Contains(got, want) ||
				strings.Contains(got, "/inherited/gocache") {
				t.Fatalf("dump = %q, want GOCACHE under %s, not the inherited", content, want)
			}
		})
	}
}

func TestWithGoCacheTouchesTheDirectoryItUses(t *testing.T) {
	testCases := []struct {
		name  string
		mtime time.Time
	}{
		{"a long unused dir", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"a dir from last year", time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			run, keyed := stepEnv(rustUnit(), nil, "")

			_, dir, err := withGoCache(root, run, keyed)
			if err != nil {
				t.Fatalf("withGoCache: %v", err)
			}

			agedDir(t, dir, testCase.mtime)

			if _, _, err = withGoCache(root, run, keyed); err != nil {
				t.Fatalf("withGoCache: %v", err)
			}

			stat, err := os.Stat(dir)
			if err != nil {
				t.Fatalf("stat: %v", err)
			}

			if !stat.ModTime().After(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)) {
				t.Fatalf("mtime = %v, want it refreshed by the use", stat.ModTime())
			}
		})
	}
}

func TestPrune(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

	testCases := []struct {
		name     string
		ages     map[string]time.Duration
		wantKept []string
		wantGone []string
	}{
		{
			"only the dirs older than 14 days go",
			map[string]time.Duration{"stale": 15 * day, "edge": 14 * day, "fresh": 13 * day},
			[]string{"edge", "fresh"},
			[]string{"stale"},
		},
		{"nothing is stale", map[string]time.Duration{"fresh": day}, []string{"fresh"}, nil},
		{"no cache dir yet", nil, nil, nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			runner := mustRunner(t, Config{CacheDir: t.TempDir()})
			root := filepath.Join(runner.cfg.CacheDir, goCacheDir)

			for name, age := range testCase.ages {
				agedDir(t, filepath.Join(root, name), now.Add(-age))
			}

			if err := runner.Prune(now); err != nil {
				t.Fatalf("Prune: %v", err)
			}

			for _, name := range testCase.wantKept {
				if !exists(filepath.Join(root, name)) {
					t.Errorf("%s was pruned, want it kept", name)
				}
			}

			for _, name := range testCase.wantGone {
				if exists(filepath.Join(root, name)) {
					t.Errorf("%s is still there, want it pruned", name)
				}
			}
		})
	}
}

func TestPruneSparesTheDirectoriesThisRunUsed(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	slotDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(slotDir, "mod"), 0o750); err != nil {
		t.Fatalf("module dir: %v", err)
	}

	runner := mustRunner(t, Config{
		Languages: map[string]Language{"rust": {Steps: []Step{fake("pass")}}},
		CacheDir:  t.TempDir(),
	})

	if _, err := runner.Check(t.Context(), slotDir, rustUnit()); err != nil {
		t.Fatalf("check: %v", err)
	}

	root := filepath.Join(runner.cfg.CacheDir, goCacheDir)

	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("gocache dirs = %v err = %v, want the one this run used", entries, err)
	}

	used := filepath.Join(root, entries[0].Name())
	stale := filepath.Join(root, "stale")

	agedDir(t, used, now.Add(-30*day))
	agedDir(t, stale, now.Add(-30*day))

	if err = runner.Prune(now); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	if !exists(used) || exists(stale) {
		t.Fatalf("used kept = %v, stale kept = %v, want true, false", exists(used), exists(stale))
	}
}

func TestPruneReportsAnUnreadableRoot(t *testing.T) {
	runner := mustRunner(t, Config{CacheDir: t.TempDir()})

	if err := os.WriteFile(filepath.Join(runner.cfg.CacheDir, goCacheDir), nil, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := runner.Prune(time.Now()); err == nil {
		t.Fatal("Prune over a file in place of the cache root returned nil")
	}
}
