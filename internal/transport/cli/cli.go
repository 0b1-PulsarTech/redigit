package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/infra/profiling"
	"github.com/wrapped-owls/redigit/internal/transport/cli/session"
)

// ErrUsage reports a command line the tool cannot act on. It is separate from
// a run failure so a caller can print usage instead of a stack of wrapping.
var ErrUsage = errors.New("usage")

// The command names dispatch recognises, named once so the switch below and
// its tests never drift from each other on a typo.
const (
	cmdAudit  = "audit"
	cmdGraph  = "graph"
	cmdPlan   = "plan"
	cmdReport = "report"
	cmdEmit   = "emit"
	cmdChunks = "chunks"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return dispatch(ctx, args, stdout, stderr, realEnv())
}

type invocation struct {
	command string
	cfg     bootstrap.Config
	start   starter
	stdout  io.Writer
	stderr  io.Writer
}

func dispatch(ctx context.Context, args []string, stdout, stderr io.Writer, env runEnv) error {
	if len(args) < 1 {
		_, _ = fmt.Fprint(stderr, usage+runUsage)

		return fmt.Errorf("%w: no command given", ErrUsage)
	}

	set := newFlagSet()
	opts := registerRunFlags(set)

	cfg, far, err := parse(set, args[1:])
	if err != nil {
		// A config error already names the file and the key; the flag list buries it.
		if !errors.Is(err, bootstrap.ErrConfig) {
			_, _ = fmt.Fprint(stderr, usage+runUsage)
		}

		return err
	}

	cfg.Auditing.FarApart = far

	// Before any profile file is made or any cache key is built for nothing.
	command := args[0]
	if !isCommand(command) {
		return unknownCommand(stderr, command)
	}

	profile, err := startProfiling(opts, stderr)
	if err != nil {
		return err
	}

	timings := profiling.NewTimings()
	stopTotal := timings.Measure("total")

	// A hit may launch the pipeline later, from inside the cache, so it must start timed.
	if opts.isTiming {
		env.Start = session.TimedStart(env.Start, timings)
	}

	env.Measure = timings.Measure

	start := env.Start

	if opts.isFromCache && usesCache(command) {
		stopKey := timings.Measure("key")
		start = session.WithCache(ctx, set, cfg, env)

		stopKey()
	}

	if opts.isFromCache && command == cmdChunks {
		slog.Warn("--from-cache has no effect on chunks")
	}

	runErr := route(ctx, invocation{
		command: command, cfg: cfg, start: start, stdout: stdout, stderr: stderr,
	})

	stopTotal()

	return finishRun(profile, timings, opts, stderr, runErr)
}

func usesCache(command string) bool {
	switch command {
	case cmdAudit, cmdGraph, cmdPlan, cmdReport, cmdEmit:
		return true
	}

	return false
}

func isCommand(command string) bool { return usesCache(command) || command == cmdChunks }

func unknownCommand(stderr io.Writer, command string) error {
	_, _ = fmt.Fprint(stderr, usage+runUsage)

	return fmt.Errorf("%w: unknown command %q", ErrUsage, command)
}

func route(ctx context.Context, call invocation) error {
	switch call.command {
	case cmdAudit:
		return runAudit(ctx, call.cfg, call.stdout, call.start)
	case cmdGraph:
		return runGraph(ctx, call.cfg, call.stdout, call.start)
	case cmdPlan:
		return runPlan(ctx, call.cfg, call.stdout, call.start)
	case cmdReport:
		return runReport(ctx, call.cfg, call.stdout, call.start)
	case cmdEmit:
		return runEmit(ctx, call.cfg, call.start, call.stdout)
	case cmdChunks:
		return runChunks(ctx, call.cfg, bootstrap.Cut, call.stdout)
	}

	return unknownCommand(call.stderr, call.command)
}
