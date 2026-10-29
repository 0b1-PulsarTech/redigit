package gitstore

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"time"

	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

// commandError is one git invocation that exited non-zero. git's own stderr
// is often the only useful diagnostic, so it is kept as a field rather than
// folded into Error(), which stays a message a caller can log without
// carrying whatever stderr happened to contain.
type commandError struct {
	Args   []string
	Stderr string
	cause  error
}

func (e *commandError) Error() string {
	subcommand := "git"
	if len(e.Args) > 0 {
		subcommand = e.Args[0]
	}

	return fmt.Sprintf("run git %s: %s", subcommand, e.cause.Error())
}

func (e *commandError) Unwrap() error { return e.cause }

// runGit runs git -C repoPath <args> and returns stdout. Every command this
// package issues targets an explicit repoPath rather than the process's
// working directory, so callers never depend on os.Chdir.
func runGit(ctx context.Context, repoPath string, args ...string) ([]byte, error) {
	return runGitCapped(ctx, repoPath, math.MaxInt64, args...)
}

func runGitCapped(
	ctx context.Context,
	repoPath string,
	limit int64,
	args ...string,
) ([]byte, error) {
	const waitDelay = 2 * time.Second

	fullArgs := append([]string{"-C", repoPath}, args...)

	// #nosec G204 -- driving the git CLI is this adapter's purpose; repoPath
	// and args are caller-supplied infra config, not untrusted input.
	cmd := exec.CommandContext(ctx, "git", fullArgs...)
	// Stderr is classified by text, so keep git in its C locale.
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	cmd.WaitDelay = waitDelay

	stdout := cappedBuffer{limit: limit}

	var stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if stdout.isOver {
		return nil, fmt.Errorf("run git: %w", batch.ErrTooLarge)
	}

	if err != nil {
		return nil, &commandError{Args: args, Stderr: stderr.String(), cause: err}
	}

	return stdout.buf.Bytes(), nil
}

type cappedBuffer struct {
	buf    bytes.Buffer
	limit  int64
	isOver bool
}

func (capped *cappedBuffer) Write(chunk []byte) (int, error) {
	// Refusing the write closes the pipe, which ends git.
	if int64(capped.buf.Len())+int64(len(chunk)) > capped.limit {
		capped.isOver = true

		return 0, batch.ErrTooLarge
	}

	written, err := capped.buf.Write(chunk)
	if err != nil {
		return written, fmt.Errorf("buffer stdout: %w", err)
	}

	return written, nil
}

// revParse resolves rev to the object id git rev-parse prints for it.
func revParse(ctx context.Context, repoPath, rev string) (string, error) {
	out, err := runGit(ctx, repoPath, "rev-parse", rev)
	if err != nil {
		return "", fmt.Errorf("resolve rev: %w", err)
	}

	return string(bytes.TrimSpace(out)), nil
}

// splitNUL splits a NUL-separated git output (built with git's own -z flag)
// into its entries, dropping the trailing empty entry -z leaves after the
// last terminator.
func splitNUL(out []byte) []string {
	trimmed := bytes.TrimSuffix(out, []byte{0})
	if len(trimmed) == 0 {
		return nil
	}

	parts := bytes.Split(trimmed, []byte{0})
	entries := make([]string, len(parts))

	for i, part := range parts {
		entries[i] = string(part)
	}

	return entries
}
