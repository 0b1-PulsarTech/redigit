package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/wrapped-owls/redigit/internal/transport/cli"
)

// exitUsage separates a command line the tool cannot act on from a run that
// failed, so a shell script can tell the two apart.
const exitUsage = 2

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	})))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		// main is the top of the chain, so logging here is the one place it
		// is correct rather than returning.
		slog.Error("run", slog.String("error", err.Error()))

		if errors.Is(err, cli.ErrUsage) {
			os.Exit(exitUsage)
		}

		os.Exit(1)
	}
}
