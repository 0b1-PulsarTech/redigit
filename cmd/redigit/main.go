package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/wrapped-owls/redigit/internal/bootstrap"
	"github.com/wrapped-owls/redigit/internal/transport/cli"
)

// exitUsage separates a command line the tool cannot act on from a run that
// failed, so a shell script can tell the two apart.
const exitUsage = 2

// runner is what run needs to execute one command line. Production wires it
// to cli.Run; a test wires it to a fake so the exit code each outcome
// produces can be proven without a language server or a git worktree.
type runner func(ctx context.Context, args []string, stdout, stderr io.Writer) error

func main() {
	// The default logger is set once, here, before anything else runs: it is
	// what every slog call inside cli.Run and beneath it resolves to.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	})))

	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, cli.Run))
}

// run is main's body without the exit call, so a test can read the process's
// intended exit code instead of only its side effects. Its own top-of-chain
// log line goes through a logger built from stderr directly rather than the
// package-global default, so concurrent tests never race over which buffer a
// SetDefault call last pointed at.
func run(parent context.Context, args []string, stdout, stderr io.Writer, execute runner) int {
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err := execute(ctx, args, stdout, stderr)
	if err == nil {
		return 0
	}

	// main is the top of the chain, so logging here is the one place it is
	// correct rather than returning.
	logger.Error("run", slog.String("error", err.Error()))

	if errors.Is(err, cli.ErrUsage) || errors.Is(err, bootstrap.ErrConfig) {
		return exitUsage
	}

	return 1
}
