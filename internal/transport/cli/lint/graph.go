package lint

import (
	"context"
	"errors"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/linting"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/stmtread"
	"github.com/wrapped-owls/redigit/internal/transport/cli/manifest"
	"github.com/wrapped-owls/redigit/internal/transport/cli/session"
	"github.com/wrapped-owls/redigit/internal/transport/cli/write"
)

func checkStructure(
	ctx context.Context,
	opts options,
	decoded []manifest.Row,
	limits linting.Limits,
	env session.Env,
) (linting.Report, error) {
	cfg, err := pipelineConfig(opts, opts.file)
	if err != nil {
		return linting.Report{}, err
	}

	commit, err := env.Commit(ctx, cfg.Repo, cfg.Rev)
	if err != nil {
		return linting.Report{}, fmt.Errorf("resolve the revision: %w", err)
	}

	start := env.Start
	if opts.pipe.fromCache {
		start = session.WithCache(ctx, keyFlags(opts), cfg, env)
	}

	pipe, err := start(ctx, cfg)
	if err != nil {
		return linting.Report{}, fmt.Errorf("start the pipeline: %w", err)
	}

	defer func() { session.WarnClosed(pipe.Close(ctx)) }()

	result, err := pipe.Graph(ctx)
	if err != nil {
		return linting.Report{}, fmt.Errorf("build the graph: %w", err)
	}

	session.WarnShortfalls(result)

	reader := gitstore.NewReader(cfg.Repo, commit)
	defer func() { session.WarnClosed(reader.Close()) }()

	report, err := measureGraph(
		ctx, result.Graph, decoded, stmtread.New(result.Graph, reader.Read), limits,
	)
	if err != nil {
		return linting.Report{}, err
	}

	report.Graph.Commit = commit

	return report, nil
}

func measureGraph(
	ctx context.Context,
	graph entities.Graph,
	decoded []manifest.Row,
	syntax linting.Syntax,
	limits linting.Limits,
) (linting.Report, error) {
	born, err := bornRows(graph, decoded)
	if err != nil {
		return linting.Report{}, err
	}

	subjects := make(map[int]string, len(decoded))
	for _, row := range decoded {
		subjects[row.Position] = row.Subject
	}

	report, err := linting.CheckStructure(ctx, linting.Structure{
		Graph: graph, Born: born, Subjects: subjects, Syntax: syntax,
	}, limits)
	if errors.Is(err, linting.ErrMismatch) {
		return linting.Report{}, fmt.Errorf("%w: %w", write.ErrUsage, err)
	}

	return report, err //nolint:wrapcheck // CheckStructure names the file it failed on
}
