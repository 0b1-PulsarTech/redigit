package batch

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr tailBuffer
	reap   sync.Once
}

func (proc *process) kill() { _ = proc.cmd.Process.Kill() }

func (proc *process) retire() {
	// Reaping makes stderr safe to read afterwards.
	proc.reap.Do(func() {
		proc.kill()
		_ = proc.stdin.Close()
		_ = proc.cmd.Wait()
	})
}

func start(ctx context.Context, repo string) (*process, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("start cat-file: %w", err)
	}

	// #nosec G204 -- driving the git CLI is this adapter's purpose.
	cmd := exec.CommandContext(context.WithoutCancel(ctx), "git", "-C", repo, "cat-file", "--batch")
	// A grandchild holding stderr open must not hang Wait.
	cmd.WaitDelay = waitDelay

	proc := &process{cmd: cmd}
	cmd.Stderr = &proc.stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("%w: open stdin: %w", ErrProcess, err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("%w: open stdout: %w", ErrProcess, err)
	}

	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("%w: start: %w", ErrProcess, err)
	}

	proc.stdin = stdin
	proc.stdout = bufio.NewReaderSize(stdout, pipeBuffer)

	return proc, nil
}

func (proc *process) exchange(spec string) ([]byte, error) {
	if _, err := io.WriteString(proc.stdin, spec+"\n"); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	header, err := proc.stdout.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	line := strings.TrimSuffix(header, "\n")

	// The spec is echoed first and may hold spaces, so the verdict is a suffix.
	switch {
	case strings.HasSuffix(line, " missing"):
		return nil, ErrMissing
	case strings.HasSuffix(line, " ambiguous"):
		return nil, ErrAmbiguous
	}

	fields := strings.Fields(header)

	const (
		headerFields = 3
		maxDrain     = 4 << 30
	)

	if len(fields) != headerFields {
		return nil, fmt.Errorf("%w: %q", errProtocol, header)
	}

	size, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || size < 0 {
		return nil, fmt.Errorf("%w: bad size in %q", errProtocol, header)
	}

	// Past maxDrain the blob cannot be drained cheaply, so the process is dropped instead.
	if size > maxDrain {
		return nil, fmt.Errorf("%w: %w", ErrTooLarge, errUndrained)
	}

	// An oversize blob is drained so the pipe stays in step and the process is kept.
	if size > MaxObject {
		if _, err = io.CopyN(io.Discard, proc.stdout, size+1); err != nil {
			return nil, fmt.Errorf("discard content: %w", err)
		}

		return nil, ErrTooLarge
	}

	content := make([]byte, size+1)
	if _, err = io.ReadFull(proc.stdout, content); err != nil {
		return nil, fmt.Errorf("read content: %w", err)
	}

	// The content is consumed first so a refused object leaves the pipe in step.
	if fields[1] != "blob" {
		return nil, fmt.Errorf("%w: %s", ErrNotBlob, fields[1])
	}

	return content[:size], nil
}
