package checkrun

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

const (
	defaultTimeout = 15 * time.Minute
	killGrace      = 2 * time.Second
)

type stepRun struct {
	dir  string
	env  []string
	step Step
}

func label(argv []string) string {
	if len(argv) > 1 && !strings.HasPrefix(argv[1], "-") {
		return argv[0] + " " + argv[1]
	}

	return argv[0]
}

func isTolerated(code int, output string, tolerated []string) bool {
	if code != 1 {
		return false
	}

	var hasTolerated bool

	for line := range strings.Lines(output) {
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}

		if !slices.Contains(tolerated, text) {
			return false
		}

		hasTolerated = true
	}

	return hasTolerated
}

func runStep(ctx context.Context, job stepRun) (verifying.Outcome, error) {
	if len(job.step.Argv) == 0 {
		return verifying.Outcome{}, fmt.Errorf("%w: a step without a command", ErrNoSteps)
	}

	timeout := job.step.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// #nosec G204 -- running the configured build tool is this adapter's purpose.
	cmd := exec.CommandContext(stepCtx, job.step.Argv[0], job.step.Argv[1:]...)
	cmd.Dir, cmd.Env, cmd.WaitDelay = job.dir, job.env, killGrace

	var tail batch.TailBuffer

	cmd.Stdout, cmd.Stderr = &tail, &tail

	isolate(cmd)

	if _, statErr := os.Stat(job.dir); statErr != nil {
		return verifying.Outcome{}, fmt.Errorf("%w: %w", ErrModuleDir, statErr)
	}

	err := cmd.Run()
	// A child that outlived the step must not keep building into the slot's next commit.
	reap(cmd)

	name := label(job.step.Argv)

	var exit *exec.ExitError

	switch {
	case err == nil || errors.Is(err, exec.ErrWaitDelay):
		return verifying.Outcome{Step: name}, nil
	case ctx.Err() != nil:
		return verifying.Outcome{}, fmt.Errorf("run %s: %w", name, ctx.Err())
	case errors.Is(stepCtx.Err(), context.DeadlineExceeded):
		return verifying.Outcome{
			IsFailed:  true,
			IsTimeout: true,
			Step:      name,
			Tail:      tail.String() + "\ntimed out after " + timeout.String(),
		}, nil
	case errors.As(err, &exit):
		if isTolerated(exit.ExitCode(), tail.String(), job.step.Tolerate) {
			return verifying.Outcome{Step: name}, nil
		}

		return verifying.Outcome{IsFailed: true, Step: name, Tail: tail.String()}, nil
	case errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist):
		return verifying.Outcome{}, fmt.Errorf("%w: %s", ErrToolMissing, job.step.Argv[0])
	}

	return verifying.Outcome{}, fmt.Errorf("start %s: %w", name, err)
}

func probe(ctx context.Context, job stepRun, argv []string) (string, error) {
	const probeTimeout = time.Minute

	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	// #nosec G204 -- the version probe is configured infra, like the steps.
	cmd := exec.CommandContext(probeCtx, argv[0], argv[1:]...)
	cmd.Dir, cmd.Env, cmd.WaitDelay = job.dir, job.env, killGrace

	var stderr batch.TailBuffer

	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("probe version: %w", ctx.Err())
		}

		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%w: %s", ErrToolMissing, argv[0])
		}

		detail := lastLines(stderr.String())

		return "", fmt.Errorf("%w: %s: %w: %s", ErrVersion, argv[0], err, detail)
	}

	return strings.TrimSpace(string(out)), nil
}

func lastLines(text string) string {
	const keep = 20

	lines := strings.Split(text, "\n")

	return strings.Join(lines[max(0, len(lines)-keep):], "\n")
}
