package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

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

	cfg, far, err := parse(args[1:])
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

func parse(args []string) (bootstrap.Config, int, error) {
	cfg := bootstrap.Defaults()

	var (
		tags         string
		layerFile    string
		featureFile  string
		chunkMapFile string
		far          int
	)

	set := flag.NewFlagSet("redigit", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&cfg.Repo, "repo", "", "repository to read")
	set.StringVar(&cfg.Rev, "rev", cfg.Rev, "revision to read")
	set.StringVar(&tags, "tags", "", "comma-separated build tags")
	set.IntVar(&far, "far", 0, "positions before a declaration counts as adrift")
	set.StringVar(
		&cfg.BlobDir,
		"blob-dir",
		"",
		"write blobs to this directory instead of the object database",
	)
	set.StringVar(&cfg.ServerCommand, "server", cfg.ServerCommand, "language server command")
	set.StringVar(&layerFile, "layers", "", "layer file: which path prefixes come first")
	set.StringVar(
		&featureFile,
		"features",
		"",
		"feature file: the vertical slices layers act inside",
	)
	set.StringVar(
		&chunkMapFile,
		"chunk-map",
		"",
		"chunk map file: JSON spans for a format redigit cannot parse on its own",
	)

	if err := set.Parse(args); err != nil {
		return bootstrap.Config{}, 0, fmt.Errorf("%w: %s", ErrUsage, err)
	}

	if cfg.Repo == "" {
		return bootstrap.Config{}, 0, fmt.Errorf("%w: --repo is required", ErrUsage)
	}

	cfg.BuildTags = splitTags(tags)

	if err := loadSlicing(&cfg, layerFile, featureFile); err != nil {
		return bootstrap.Config{}, 0, err
	}

	if err := loadChunkMap(&cfg, chunkMapFile); err != nil {
		return bootstrap.Config{}, 0, err
	}

	return cfg, far, nil
}

// loadSlicing reads the two files that decide how a layer may reorder: the
// layers themselves, and the features they act inside.
func loadSlicing(cfg *bootstrap.Config, layerFile, featureFile string) error {
	if layerFile != "" {
		layers, err := bootstrap.LoadLayers(layerFile)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrUsage, err)
		}

		cfg.Ordering.Layers = layers
	}

	if featureFile != "" {
		features, err := bootstrap.LoadFeatures(featureFile)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrUsage, err)
		}

		cfg.Ordering.Features = features
	}

	return nil
}

func splitTags(tags string) []string {
	if strings.TrimSpace(tags) == "" {
		return nil
	}

	parts := strings.Split(tags, ",")

	out := make([]string, 0, len(parts))

	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}

	return out
}
