package session

import (
	"log/slog"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/auditing"
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

// WarnUnseen names the files a build constraint left out of the main build.
// It is a DIFFERENT failure from warnBlind and needs its own line: a blind
// query errored and is counted, while a hidden file answers the syntactic
// query normally. Under the typed resolver its reference queries are answered
// too; under lsp it may contribute declarations with no edges at all, which
// is indistinguishable from code nothing references.
func WarnUnseen(unseen []auditing.Unseen) {
	if len(unseen) == 0 {
		return
	}

	slog.Warn(
		"a build constraint left some files out of the main build",
		slog.Int("files", len(unseen)),
		slog.String("first", unseen[0].File),
		slog.String("constraint", unseen[0].Constraint),
	)
}

// WarnUnclaimed names the final-tree paths no configured language pack
// claims. It is a DIFFERENT failure from WarnUnseen and needs its own line:
// an unseen file still answers the syntactic query and contributes
// declarations with no edges, while an unclaimed path is never read at all -
// no plan built from this run can reproduce it, and without this warning
// nothing says so until the final-tree invariant fails at the very end.
func WarnUnclaimed(unclaimed []auditing.Unclaimed) {
	if len(unclaimed) == 0 {
		return
	}

	slog.Warn(
		"no language pack claims some final-tree paths",
		slog.Int("files", len(unclaimed)),
		slog.String("first", unclaimed[0].File),
	)
}

func warnPartial(result indexing.Result) {
	for _, partial := range result.Partial {
		attrs := []any{
			slog.Int("count", partial.Broken),
			slog.Int("of", partial.Of),
			slog.Any("tags", partial.Tags),
		}

		if partial.FirstPackage != "" {
			attrs = append(attrs,
				slog.String("first", partial.FirstPackage),
				slog.String("cause", partial.FirstError),
				slog.Int("same_message", partial.SameMessage),
			)
		}

		slog.Warn("some packages did not type-check; their type facts are partial", attrs...)
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
