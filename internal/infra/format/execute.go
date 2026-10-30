package format

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	killGrace = time.Second

	minOutputCap = 8 << 20
)

type capped struct {
	buf   bytes.Buffer
	limit int
	over  bool
}

func (sink *capped) Write(chunk []byte) (int, error) {
	// what passes the limit is dropped but still drained, so the tool is never blocked on a pipe
	room := max(sink.limit-sink.buf.Len(), 0)
	if len(chunk) > room {
		sink.over = true
		sink.buf.Write(chunk[:room])
	} else {
		sink.buf.Write(chunk)
	}

	return len(chunk), nil
}

func outputCap(input int) int {
	const outputFactor = 8

	return max(minOutputCap, outputFactor*input)
}

func (engine *Engine) pipe(ctx context.Context, step tool, content []byte) ([]byte, string, error) {
	runCtx, cancel := context.WithTimeout(ctx, step.timeout)
	defer cancel()

	limit := outputCap(len(content))
	stdout, stderr := &capped{limit: limit}, &capped{limit: limit}

	cmd := command(runCtx, step.argv)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewReader(content), stdout, stderr
	cmd.Dir = engine.configRoot
	cmd.WaitDelay = killGrace

	if err := runGroup(cmd); err != nil {
		return nil, stderr.buf.String(), runError(runCtx, err)
	}

	if stdout.over {
		return nil, stderr.buf.String(), fmt.Errorf("%w: stdout", ErrOutputTooLarge)
	}

	return stdout.buf.Bytes(), stderr.buf.String(), nil
}

func (engine *Engine) inPlace(ctx context.Context, step tool, jobs []job) run {
	dir, err := os.MkdirTemp("", "redigit-format-")
	if err != nil {
		return unstarted(fmt.Errorf("make format dir: %w", err))
	}

	defer func() { _ = os.RemoveAll(dir) }()

	// the root keeps every read and write inside the temp dir, whatever the tool leaves behind
	root, err := os.OpenRoot(dir)
	if err != nil {
		return unstarted(fmt.Errorf("open format dir: %w", err))
	}

	defer func() { _ = root.Close() }()

	if err = engine.copyConfig(root); err != nil {
		return unstarted(err)
	}

	args := make([]string, 0, len(step.argv)+len(jobs))
	args = append(args, step.argv...)

	for _, item := range jobs {
		// a blob whose own path or content cannot be written fails alone, never the step
		if err = writeUnder(root, item.clean, item.content); err != nil {
			return run{err: err}
		}

		args = append(args, "./"+filepath.ToSlash(item.clean))
	}

	runCtx, cancel := context.WithTimeout(ctx, step.timeout)
	defer cancel()

	stderr := &capped{limit: minOutputCap}

	cmd := command(runCtx, args)
	cmd.Dir, cmd.Stderr, cmd.WaitDelay = dir, stderr, killGrace

	if err = runGroup(cmd); err != nil {
		return run{stderr: stderr.buf.String(), err: runError(runCtx, err)}
	}

	contents := make([][]byte, len(jobs))

	for position, item := range jobs {
		contents[position], err = readCapped(root, item.clean, outputCap(len(item.content)))
		if err != nil {
			return run{stderr: stderr.buf.String(), err: err}
		}
	}

	return run{stderr: stderr.buf.String(), contents: contents}
}

func runError(runCtx context.Context, err error) error {
	switch cause := runCtx.Err(); {
	case errors.Is(cause, context.DeadlineExceeded):
		return fmt.Errorf("timed out: %w", cause)
	case cause != nil:
		return fmt.Errorf("stopped: %w", cause)
	case argTooLong(err):
		// it depends on how many blobs share the run, so the chunk is split instead of the step ended
		return fmt.Errorf("argument list too long: %w", err)
	case startFailed(err):
		return fmt.Errorf("%w: %w", errStart, err)
	default:
		return err
	}
}

func startFailed(err error) bool {
	_, isExec := errors.AsType[*exec.Error](err)
	_, isPath := errors.AsType[*fs.PathError](err)

	return isExec || isPath
}

func command(ctx context.Context, argv []string) *exec.Cmd {
	//nolint:gosec // G204: argv is the owner config, never repo content
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	isolate(cmd)

	return cmd
}
