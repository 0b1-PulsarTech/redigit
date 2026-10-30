package rewriting

import (
	"context"
	"errors"
	"fmt"
)

type (
	Replayed struct {
		Position int
		Message  string
		At       Stamp
		Blobs    []BlobRef
	}

	Staged struct {
		Tip     string
		Base    string
		Scratch string
	}

	ReplayRequest struct {
		Ref        string
		DateRef    string
		Verify     string
		Rows       []Row
		Subjects   map[string]string
		AllowDraft bool
	}

	ReplayResult struct {
		Commit     string
		Commits    int
		CleanupErr error
	}

	Replayer struct {
		dates   DateSource
		history ReplayHistory
	}
)

func NewReplayer(dates DateSource, history ReplayHistory) Replayer {
	return Replayer{dates: dates, history: history}
}

func (replayer Replayer) Replay(
	ctx context.Context,
	req ReplayRequest,
) (result ReplayResult, err error) {
	var (
		commits []Replayed
		staged  Staged
	)

	commits, err = replayer.plan(ctx, req)
	if err != nil {
		return ReplayResult{}, err
	}

	staged, err = replayer.history.Replay(ctx, req.Ref, commits)
	if err != nil {
		return ReplayResult{}, fmt.Errorf("replay history: %w", err)
	}

	// The scratch ref goes on every path, so a cancel cannot strand it.
	isPublished := false

	defer func() {
		discardErr := replayer.history.Discard(context.WithoutCancel(ctx), staged)
		if discardErr == nil {
			return
		}

		discardErr = fmt.Errorf("discard staged history: %w", discardErr)
		// Once the target has moved the run succeeded; only the leftover ref is reported.
		if isPublished {
			result.CleanupErr = discardErr

			return
		}

		err = errors.Join(err, discardErr)
	}()

	// The target ref moves at this one point, once everything has been checked.
	if req.Verify != "" {
		if err = replayer.history.Verify(ctx, staged.Tip, req.Verify); err != nil {
			return ReplayResult{}, fmt.Errorf("verify replay: %w", err)
		}
	}

	// A cancel is refused before the move, and a move that has started runs to its end.
	if err = ctx.Err(); err != nil {
		return ReplayResult{}, fmt.Errorf("publish replay: %w", err)
	}

	if err = replayer.history.Publish(context.WithoutCancel(ctx), req.Ref, staged); err != nil {
		return ReplayResult{}, fmt.Errorf("publish replay: %w", err)
	}

	isPublished = true

	return ReplayResult{Commit: staged.Tip, Commits: len(commits)}, nil
}

func (replayer Replayer) plan(ctx context.Context, req ReplayRequest) ([]Replayed, error) {
	if len(req.Rows) == 0 {
		return nil, fmt.Errorf("%w: the manifest holds no row", ErrManifest)
	}

	rows, err := OrderRows(req.Rows)
	if err != nil {
		return nil, err
	}

	messages, err := Messages(rows, req.Subjects, req.AllowDraft)
	if err != nil {
		return nil, err
	}

	stamps, err := replayer.dates.AuthorDates(ctx, req.DateRef)
	if err != nil {
		return nil, fmt.Errorf("read author dates: %w", err)
	}

	sampled, err := SampleDates(stamps, len(rows))
	if err != nil {
		return nil, err
	}

	commits := make([]Replayed, 0, len(rows))
	for idx, row := range rows {
		commits = append(commits, Replayed{
			Position: row.Position, Message: messages[idx], At: sampled[idx], Blobs: row.Blobs,
		})
	}

	return commits, nil
}
