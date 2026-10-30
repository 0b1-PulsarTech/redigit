package session

import (
	"log/slog"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

func WarnShortfalls(result indexing.Result) {
	// From the result, not from the pack: a cache hit replays the same lines.
	warnPartial(result)
	warnBlind(result)
}

func WarnClosed(err error) {
	if err == nil {
		return
	}

	// The error names the kept checkout, so a leak on /tmp shows in the log.
	slog.Warn(
		"the pipeline did not close cleanly; a throwaway checkout may remain",
		slog.String("error", err.Error()),
	)
}

func warnPartial(result indexing.Result) {
	for _, partial := range result.Partial {
		slog.Warn(
			"some packages did not type-check; their type facts are partial",
			slog.Int("count", partial.Broken),
			slog.Int("of", partial.Of),
			slog.Any("tags", partial.Tags),
		)
	}
}

func warnBlind(result indexing.Result) {
	if len(result.Blind) == 0 {
		return
	}

	// To the log, not the output: a report stays machine-readable but never drops this.
	slog.Warn(
		"the resolver could not answer for some declarations",
		slog.Int("count", len(result.Blind)),
		slog.Int("of", result.Graph.Len()),
		slog.String("first", result.Blind[0].Decl.String()),
		slog.String("query", result.Blind[0].Query),
		slog.String("reason", result.Blind[0].Reason),
	)
}
