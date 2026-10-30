package slots

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

const waitDelay = 2 * time.Second

type (
	gitCall struct {
		gitDir string
		env    []string
		stdin  []byte
		args   []string
	}

	gitError struct {
		Stderr string
		name   string
		cause  error
	}
)

func (failure *gitError) Error() string {
	message := fmt.Sprintf("run git %s: %s", failure.name, failure.cause.Error())

	tail := strings.TrimSpace(failure.Stderr)
	if tail == "" {
		return message
	}

	return message + ": " + strings.ReplaceAll(tail, "\n", "; ")
}

func (failure *gitError) Unwrap() error { return failure.cause }

func GitDir(ctx context.Context, path string) (string, error) {
	// #nosec G204 -- driving the git CLI is this adapter's purpose.
	cmd := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--absolute-git-dir")
	cmd.Env, cmd.WaitDelay = withoutGitEnv(), waitDelay

	var stderr batch.TailBuffer

	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return "", &gitError{name: "rev-parse", Stderr: stderr.String(), cause: err}
	}

	return strings.TrimSpace(string(out)), nil
}

func withoutGitEnv() []string {
	env := make([]string, 0, len(os.Environ()))

	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			env = append(env, entry)
		}
	}

	return env
}

func runGit(ctx context.Context, call gitCall) ([]byte, error) {
	args := append([]string{
		"-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false",
		"-c", "core.sparseCheckout=false", "-c", "core.ignoreStat=false",
		"-c", "core.checkStat=default",
	}, call.args...)

	// #nosec G204 -- driving the git CLI is this adapter's purpose; arguments are infra config.
	cmd := exec.CommandContext(ctx, "git", args...)

	cmd.Env = append(withoutGitEnv(), "LC_ALL=C", "GIT_DIR="+call.gitDir)
	cmd.Env = append(cmd.Env, call.env...)
	cmd.WaitDelay = waitDelay

	var stdout bytes.Buffer

	var stderr batch.TailBuffer

	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	if call.stdin != nil {
		cmd.Stdin = bytes.NewReader(call.stdin)
	}

	if err := cmd.Run(); err != nil {
		return nil, &gitError{name: call.args[0], Stderr: stderr.String(), cause: err}
	}

	return stdout.Bytes(), nil
}
