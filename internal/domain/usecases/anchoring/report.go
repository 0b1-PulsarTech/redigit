package anchoring

import (
	"log/slog"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/anchoring/selector"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/lineage"
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

func apart(graph entities.Graph, entries []entry, left []skip) ([]entry, []skip) {
	type shape struct {
		file string
		span entities.Span
	}

	held := make(map[entities.DeclKey]entry, len(entries))
	for _, placed := range entries {
		held[placed.key] = placed
	}

	isLeft := map[entities.DeclKey]bool{}
	for _, one := range left {
		isLeft[one.key] = true
	}

	sharers := map[shape][]entities.DeclKey{}

	for _, decl := range graph.Decls() {
		at := shape{decl.Key.File, decl.Span}
		sharers[at] = append(sharers[at], decl.Key)
	}

	var skipped []skip

	// A span several names own is spliced whole, so a name that holds a pre-image must agree with
	// every other that does, and a name with none to give (born at the anchor) does not count.
	kept := slices.DeleteFunc(slices.Clone(entries), func(placed entry) bool {
		reason := ""

		for _, other := range sharers[shape{placed.key.File, placed.span}] {
			mate, isHeld := held[other]

			switch {
			case isHeld && mate.preimage != placed.preimage:
				reason = reasonShared
			case !isHeld && isLeft[other] && reason == "":
				reason = reasonLeft
			}
		}

		if reason != "" {
			skipped = append(skipped, skip{key: placed.key, reason: reason})
		}

		return reason != ""
	})

	return kept, skipped
}

func repeated(lists ...[]entities.Decl) map[declName]bool {
	out := map[declName]bool{}

	for _, list := range lists {
		seen := map[declName]bool{}

		for _, decl := range list {
			name := declName{string(decl.Key.Kind), decl.Key.Name}
			out[name] = out[name] || seen[name]
			seen[name] = true
		}
	}

	return out
}

const (
	reasonShared = "shares its text with a name whose pre-image differs"
	reasonTwice  = "declared twice in one file"
	reasonLeft   = "shares its text with a name that is left alone"
)

func importsOf(parser lineage.Parser, source []byte, text string) []string {
	if reader, isReader := parser.(Importer); isReader {
		return reader.Needed(source, text)
	}

	return nil
}

func versionOf(placed entry, slot int) entities.Version {
	return entities.Version{
		Text: placed.preimage, Until: entities.Position(slot), Imports: placed.imports,
	}
}

func droppedBecause(one found) string {
	if one.lone > 0 {
		return "every placed declaration it changes is left alone"
	}

	return "it changes no placed declaration"
}

func (view scene) touches(placed entry, home, decl entities.Decl) bool {
	name := decl.Key.Name

	switch decl.Key.Kind {
	case entities.KindMethod:
		// Without types a selector cannot be told from another type's, so any method so named counts.
		_, method, _ := strings.Cut(name, ".")
		file, imports := placed.key.File, placed.imports

		if !view.memo.Removed(file, placed.preimage, placed.final, method, imports) {
			return false
		}

		_, isImported := selector.Names(placed.imports, decl.Unit, view.modules)

		return decl.Unit == home.Unit || isImported
	case entities.KindFunc, entities.KindType, entities.KindValue:
		if decl.Unit == home.Unit {
			return wordIn(placed.preimage, name) && view.reads(placed, name)
		}

		names, _ := selector.Names(placed.imports, decl.Unit, view.modules)

		return wordIn(placed.preimage, name) && slices.ContainsFunc(names, func(local string) bool {
			return view.reads(placed, local+"."+name)
		})
	}

	return false
}
