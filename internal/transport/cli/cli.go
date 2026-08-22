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

const usage = `redigit rebuilds a repository's history so every commit creates exactly what it needs.

  redigit audit  --repo PATH [--rev REF] [--tags TAGS] [--far N]
      Index, order, and report. Says which declarations are born far from
      whatever reads them, and what each number does not look at.

  redigit report --repo PATH [--rev REF] [--tags TAGS]
      One JSON object per line, per declaration: where it was born in this
      history, every reader outside its own unit with the reason, and the gap
      to the nearest one. Birth is read per declaration, not per file.

  redigit graph  --repo PATH [--rev REF] [--tags TAGS]
      Emit the declaration graph: every declaration and every edge, with the
      reason each edge exists.

  redigit plan   --repo PATH [--rev REF] [--tags TAGS]
      Emit a commit plan, cut along features where a feature owns the work and
      along layers where none does. Subjects come out marked DRAFT: nothing
      writes a history from an unreviewed plan.

Flags:
  --repo PATH   repository to read (required)
  --rev  REF    revision holding the tree to read (default HEAD)
  --tags TAGS   comma-separated build tags. Passing none is a real choice:
                a file behind a build constraint is invisible to reference
                queries, and nothing reports that it was skipped.
  --far  N      positions a declaration may sit ahead of its first outside
                reader before audit reports it (default 25)
`

// Run executes one command line. It returns ErrUsage for anything it cannot
// parse, so main can tell a mistake from a failure.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
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
	case "audit":
		return runAudit(ctx, cfg, stdout)
	case "graph":
		return runGraph(ctx, cfg, stdout)
	case "plan":
		return runPlan(ctx, cfg, stdout)
	case "report":
		return runReport(ctx, cfg, stdout)
	}

	_, _ = fmt.Fprint(stderr, usage)

	return fmt.Errorf("%w: unknown command %q", ErrUsage, command)
}

func parse(args []string) (bootstrap.Config, int, error) {
	cfg := bootstrap.Defaults()

	var (
		tags string
		far  int
	)

	set := flag.NewFlagSet("redigit", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&cfg.Repo, "repo", "", "repository to read")
	set.StringVar(&cfg.Rev, "rev", cfg.Rev, "revision to read")
	set.StringVar(&tags, "tags", "", "comma-separated build tags")
	set.IntVar(&far, "far", 0, "positions before a declaration counts as adrift")
	set.StringVar(&cfg.ServerCommand, "server", cfg.ServerCommand, "language server command")

	if err := set.Parse(args); err != nil {
		return bootstrap.Config{}, 0, fmt.Errorf("%w: %s", ErrUsage, err)
	}

	if cfg.Repo == "" {
		return bootstrap.Config{}, 0, fmt.Errorf("%w: --repo is required", ErrUsage)
	}

	cfg.BuildTags = splitTags(tags)

	return cfg, far, nil
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
