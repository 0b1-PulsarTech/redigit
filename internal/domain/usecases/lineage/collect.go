package lineage

import (
	"context"
	"fmt"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type (
	unitKey struct {
		blob, path string
	}

	unit struct {
		blob, path string
		lang       int
		needsSpans bool
	}

	move struct {
		fromUnit  int
		fileScore int
	}

	sighting struct {
		at   entities.Position
		rev  string
		path string
		unit int
		move *move
	}

	history struct {
		index      map[unitKey]int
		units      []unit
		sightings  []sighting
		unreadable []string
	}
)

func (hist *history) unitFor(blob, path string, lang int) int {
	key := unitKey{blob: blob, path: path}
	if idx, known := hist.index[key]; known {
		return idx
	}

	idx := len(hist.units)
	hist.index[key] = idx
	hist.units = append(hist.units, unit{blob: blob, path: path, lang: lang})

	return idx
}

func (scanner Scanner) collect(ctx context.Context, snap entities.Snapshot) (*history, error) {
	positions := make(map[string]entities.Position, len(snap.History))
	for _, touch := range snap.History {
		positions[touch.Rev] = touch.Position
	}

	hist := &history{index: map[unitKey]int{}}

	for revision, err := range scanner.log.Changes(ctx) {
		if err != nil {
			return nil, fmt.Errorf("scan lineage: read history: %w", err)
		}

		if position, known := positions[revision.Rev]; known {
			scanner.record(hist, revision, position)
		}
	}

	return hist, nil
}

func (scanner Scanner) record(hist *history, revision Revision, at entities.Position) {
	for _, change := range revision.Changes {
		lang, owned := scanner.languageFor(change.Path)
		if !owned {
			continue
		}

		if change.Status == StatusDeleted {
			hist.unreadable = append(hist.unreadable, revision.Rev+":"+change.Path)

			continue
		}

		seen := sighting{
			at: at, rev: revision.Rev, path: change.Path,
			unit: hist.unitFor(change.Blob, change.Path, lang),
		}

		if scanner.cfg.Follow && change.Status == StatusRenamed {
			seen.move = scanner.moveOf(hist, change, lang)

			if seen.move != nil {
				hist.units[seen.unit].needsSpans = true
				hist.units[seen.move.fromUnit].needsSpans = true
			}
		}

		hist.sightings = append(hist.sightings, seen)
	}
}

func (scanner Scanner) moveOf(hist *history, change Change, lang int) *move {
	from, owned := scanner.languageFor(change.OldPath)
	if !owned || from != lang || change.OldBlob == "" {
		return nil
	}

	return &move{
		fromUnit:  hist.unitFor(change.OldBlob, change.OldPath, lang),
		fileScore: change.Similarity,
	}
}

func (scanner Scanner) languageFor(path string) (int, bool) {
	for idx, lang := range scanner.cfg.Languages {
		for _, ext := range lang.Extensions {
			if strings.HasSuffix(path, ext) {
				return idx, true
			}
		}
	}

	return 0, false
}
