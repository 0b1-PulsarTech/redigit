package write

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/rewriting"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore"
	"github.com/wrapped-owls/redigit/internal/infra/gitstore/batch"
)

type (
	dateSource struct{ repo string }

	result struct {
		Ref     string `json:"ref"`
		Commit  string `json:"commit"`
		Commits int    `json:"commits"`
	}
)

var _ rewriting.DateSource = dateSource{}

func (source dateSource) AuthorDates(ctx context.Context, ref string) ([]rewriting.Stamp, error) {
	stamps, err := batch.AuthorDates(ctx, source.repo, ref)
	if err != nil {
		return nil, fmt.Errorf("author dates: %w", err)
	}

	return stamps, nil
}

func execute(ctx context.Context, opts options, stdout io.Writer) error {
	rows, err := readRows(opts.manifest)
	if err != nil {
		return err
	}

	subjects := map[string]string{}

	if opts.subjects != "" {
		if subjects, err = readSubjects(opts.subjects); err != nil {
			return err
		}
	}

	// Replay reads no target modes, so there is no target to name.
	history := gitstore.NewHistory(
		opts.repo, "", gitstore.Identity{Name: opts.authorName, Email: opts.authorEmail},
	)
	if opts.blobDir != "" {
		history = history.WithBlobDir(opts.blobDir)
	}

	replayed, err := rewriting.NewReplayer(dateSource{repo: opts.repo}, history).Replay(
		ctx, rewriting.ReplayRequest{
			Ref: opts.ref, DateRef: opts.dateRef, Verify: opts.verify,
			Rows: rows, Subjects: subjects, AllowDraft: opts.allowDraft,
		},
	)
	if err != nil {
		return fmt.Errorf("replay manifest: %w", err)
	}

	report(stdout, slog.Default(), opts.ref, replayed)

	return nil
}

func report(
	stdout io.Writer,
	logger *slog.Logger,
	ref string,
	replayed rewriting.ReplayResult,
) {
	// The target has moved, so a leftover scratch ref is a warning and the run still succeeds.
	if replayed.CleanupErr != nil {
		logger.Warn(
			"the target moved but its scratch ref was not removed",
			slog.String("error", replayed.CleanupErr.Error()),
		)
	}

	err := json.NewEncoder(stdout).Encode(result{
		Ref: ref, Commit: replayed.Commit, Commits: replayed.Commits,
	})
	if err != nil {
		// The target has moved, so a closed pipe must not turn the exit code into a failure.
		logger.Warn(
			"the target moved but its result line was not written",
			slog.String("ref", ref),
			slog.String("commit", replayed.Commit),
			slog.String("error", err.Error()),
		)
	}
}
