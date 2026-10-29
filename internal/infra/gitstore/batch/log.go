package batch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os/exec"
	"time"
)

const waitDelay = 2 * time.Second

func Log(ctx context.Context, repo, ref string) iter.Seq2[Commit, error] {
	return func(yield func(Commit, error) bool) {
		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		// -M overrides a user's diff.renames setting, so entries do not depend on config.
		// #nosec G204 -- driving the git CLI is this adapter's purpose.
		cmd := exec.CommandContext(runCtx, "git", "-C", repo, "log", "--reverse",
			"--raw", "-z", "--no-abbrev", "-M", "--format=%x01%H%x20%at", "--end-of-options", ref)
		// A grandchild holding stderr open must not hang Wait.
		cmd.WaitDelay = waitDelay

		var stderr tailBuffer
		cmd.Stderr = &stderr

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			yield(Commit{}, fmt.Errorf("%w: open stdout: %w", ErrProcess, err))

			return
		}

		if err = cmd.Start(); err != nil {
			yield(Commit{}, fmt.Errorf("%w: start log: %w", ErrProcess, err))

			return
		}

		var (
			held   Commit
			isHeld bool
		)

		abort := func() {
			cancel()

			_ = cmd.Wait()
		}

		for commit, parseErr := range ParseLog(stdout) {
			if parseErr != nil {
				yield(Commit{}, parseFailure(ctx, parseFailureInput{
					cmd: cmd, cancel: cancel, stdout: stdout, stderr: &stderr, cause: parseErr,
				}))

				return
			}

			if isHeld && !yield(held, nil) {
				abort()

				return
			}

			held, isHeld = commit, true
		}

		// On a clean EOF git is left to exit; killing it would hide its status.
		if waitErr := cmd.Wait(); waitErr != nil {
			yield(Commit{}, runFailure(ctx, waitErr, &stderr))

			return
		}

		// Holding the last commit keeps a git that died mid-stream from leaking a truncated one.
		if isHeld {
			yield(held, nil)
		}
	}
}

func runFailure(ctx context.Context, waitErr error, stderr *tailBuffer) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("run log: %w", ctxErr)
	}

	return fmt.Errorf("%w: log: %w: %s", ErrProcess, waitErr, stderr.String())
}

type parseFailureInput struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	stdout io.Reader
	stderr *tailBuffer
	cause  error
}

func parseFailure(ctx context.Context, input parseFailureInput) error {
	// A git still writing past a pipe of output is killed; one that died keeps its status.
	drained, _ := io.Copy(io.Discard, io.LimitReader(input.stdout, pipeBuffer))
	isRunning := drained == pipeBuffer

	if isRunning {
		input.cancel()
	}

	waitErr := input.cmd.Wait()

	// A cancelled run is a cancellation, however it surfaced in the parser.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return errors.Join(input.cause, fmt.Errorf("run log: %w", ctxErr))
	}

	if isRunning || waitErr == nil {
		return input.cause
	}

	return errors.Join(input.cause, runFailure(ctx, waitErr, input.stderr))
}
