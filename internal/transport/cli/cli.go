package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
)

// ErrUsage reports a command line the tool cannot act on. It is separate from
// a run failure so a caller can print usage instead of a stack of wrapping.
var ErrUsage = errors.New("usage")

// The command names dispatch recognises, named once so the switch below and
// its tests never drift from each other on a typo.
const (
	cmdAudit  = "audit"
	cmdChunks = "chunks"
	cmdEmit   = "emit"
	cmdGraph  = "graph"
	cmdPlan   = "plan"
	cmdReport = "report"
)

// Run executes one command line. It returns ErrUsage for anything it cannot
// parse, so main can tell a mistake from a failure.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return dispatch(ctx, args, stdout, stderr, realStart)
}

// dispatch is Run's body, parameterised over how a pipeline gets built. Run
// wires that to bootstrap.Start; a test wires it to a fake pipeline so
// dispatch can be proven without a language server or a git worktree.
func dispatch(ctx context.Context, args []string, stdout, stderr io.Writer, start starter) error {
	if len(args) < 1 {
		_, _ = fmt.Fprint(stderr, usage)

		return fmt.Errorf("%w: no command given", ErrUsage)
	}

	command := args[0]

	cfg, far, err := parse(newFlagSet(), args[1:])
	if err != nil {
		_, _ = fmt.Fprint(stderr, usage)

		return err
	}

	cfg.Auditing.FarApart = far

	switch command {
	case cmdAudit:
		return runAudit(ctx, cfg, stdout, start)
	case cmdGraph:
		return runGraph(ctx, cfg, stdout, start)
	case cmdPlan:
		return runPlan(ctx, cfg, stdout, start)
	case cmdReport:
		return runReport(ctx, cfg, stdout, start)
	case cmdEmit:
		return runEmit(ctx, cfg, start, stdout)
	case cmdChunks:
		return runChunks(ctx, cfg, bootstrap.Cut, stdout)
	}

	_, _ = fmt.Fprint(stderr, usage)

	return fmt.Errorf("%w: unknown command %q", ErrUsage, command)
}
