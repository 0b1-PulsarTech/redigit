package anchoring

import (
	"log/slog"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func (planner Planner) summarize(one found, outcome, reason string, extra ...any) {
	line, _, _ := strings.Cut(reason, "\n")
	attrs := append([]any{
		slog.String("rev", one.anchor.Rev),
		slog.String("outcome", outcome),
		slog.String("reason", line),
		slog.Int("decls", len(one.entries)),
		slog.Int("skipped", one.skipped),
	}, extra...)

	planner.cfg.Logger.Warn("anchors: summary", attrs...)
}

func (planner Planner) reportSkip(rev string, one skip) {
	subject := slog.String("decl", one.key.String())
	if one.key.Name == "" {
		subject = slog.String("file", one.key.File)
	}

	planner.cfg.Logger.Warn("anchors: not spliced",
		slog.String("rev", rev), subject, slog.String("reason", one.reason),
	)
}

func (planner Planner) reportMissing(rev string, key entities.DeclKey) {
	planner.cfg.Logger.Warn("anchors: born declaration not in the final tree",
		slog.String("rev", rev),
		slog.String("decl", key.String()),
	)
}

func (planner Planner) fold(one found, reason string) {
	for _, placed := range one.entries {
		planner.cfg.Logger.Info("anchors: folded declaration",
			slog.String("rev", one.anchor.Rev),
			slog.String("decl", placed.key.String()),
		)
	}

	planner.summarize(one, "folded", reason)
}
