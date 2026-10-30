package slots

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

var errExit = errors.New("exit status 128")

func TestWithoutGitEnv(t *testing.T) {
	t.Setenv("GIT_DIR", "/elsewhere/.git")
	t.Setenv("GIT_WORK_TREE", "/elsewhere")
	t.Setenv("REDIGIT_KEEP", "yes")

	env := withoutGitEnv()

	for _, entry := range env {
		if strings.HasPrefix(entry, "GIT_") {
			t.Fatalf("env still holds %q", entry)
		}
	}

	if !slices.Contains(env, "REDIGIT_KEEP=yes") {
		t.Fatal("scrubbing git variables dropped an unrelated one")
	}
}

func TestGitErrorCarriesTheStderrTail(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		stderr string
		want   string
	}{
		{
			"a stderr tail", "fatal: bad tree\n",
			"run git read-tree: exit status 128: fatal: bad tree",
		},
		{
			"a multi-line tail", "error: one\nfatal: two\n",
			"run git read-tree: exit status 128: error: one; fatal: two",
		},
		{"no stderr", "", "run git read-tree: exit status 128"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			failure := &gitError{name: "read-tree", Stderr: testCase.stderr, cause: errExit}

			if got := failure.Error(); got != testCase.want {
				t.Fatalf("Error() = %q, want %q", got, testCase.want)
			}

			if !errors.Is(failure, errExit) || strings.Contains(failure.Error(), "\n") {
				t.Fatalf("err = %q, want it to unwrap and stay on one line", failure)
			}
		})
	}
}
