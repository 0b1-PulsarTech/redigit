package indexing

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// Births is where each declaration first appeared in an existing history, and
// what the scan could not read.
type Births struct {
	At map[entities.DeclKey]entities.Position
	// Unreadable names the (revision, path) pairs the scan skipped. It is
	// reported rather than swallowed: a scan that silently skipped a tenth of
	// the history would still return a plausible-looking map.
	Unreadable []string
}

// Scanner finds, per declaration, the first commit whose version of its file
// already held it.
//
// This is the answer the file's first touch only approximates. A file is born
// with three declarations and gains eight over months, so anchoring on the
// file dates all of them to the earliest, and pairing a test with its twin
// file's birth rather than with the declaration it exercises is what breaks a
// move: the twin was born nearly empty and grew hundreds of commits later.
type Scanner struct {
	log   RevisionLog
	langs []Language
	limit int
}

func NewScanner(log RevisionLog, cfg Config) Scanner {
	limit := cfg.Concurrency
	if limit < 1 {
		limit = defaultConcurrency
	}

	return Scanner{log: log, langs: cfg.Languages, limit: limit}
}

type (
	blobAt struct{ blob, path string }

	parsedUnit struct {
		keys     []entities.DeclKey
		isParsed bool
	}

	sighting struct {
		at   entities.Position
		unit int
		rev  string
		path string
	}
)

type history struct {
	units      map[blobAt]int
	order      []blobAt
	langs      []Language
	sightings  []sighting
	unreadable []string
}

func (scanner Scanner) Scan(ctx context.Context, snap entities.Snapshot) (Births, error) {
	if !snap.HasHistory() {
		return Births{}, fmt.Errorf("%w: the snapshot carries no history", errNoHistory)
	}

	// One streamed pass; each distinct (blob, path) is parsed once, not once per sighting.
	seen, err := scanner.collect(ctx, snap)
	if err != nil {
		return Births{}, err
	}

	parsed, err := scanner.parseAll(ctx, seen.order, seen.langs)
	if err != nil {
		return Births{}, err
	}

	at := map[entities.DeclKey]entities.Position{}

	for _, sight := range seen.sightings {
		unit := parsed[sight.unit]
		if !unit.isParsed {
			seen.unreadable = append(seen.unreadable, sight.rev+":"+sight.path)

			continue
		}

		for _, key := range unit.keys {
			if current, ok := at[key]; !ok || sight.at < current {
				at[key] = sight.at
			}
		}
	}

	slices.Sort(seen.unreadable)

	return Births{At: at, Unreadable: seen.unreadable}, nil
}

func (scanner Scanner) collect(ctx context.Context, snap entities.Snapshot) (*history, error) {
	positions := make(map[string]entities.Position, len(snap.History))
	for _, touch := range snap.History {
		positions[touch.Rev] = touch.Position
	}

	seen := &history{units: map[blobAt]int{}}

	for change, err := range scanner.log.Changes(ctx) {
		if err != nil {
			return nil, fmt.Errorf("scan births: read history: %w", err)
		}

		if position, known := positions[change.Rev]; known {
			scanner.record(seen, change, position)
		}
	}

	return seen, nil
}

func (scanner Scanner) record(seen *history, change RevisionChange, position entities.Position) {
	for _, file := range change.Files {
		lang, owned := scanner.languageFor(file.Path)
		if !owned {
			continue
		}

		if file.IsDeleted {
			seen.unreadable = append(seen.unreadable, change.Rev+":"+file.Path)

			continue
		}

		key := blobAt{blob: file.Blob, path: file.Path}

		unit, known := seen.units[key]
		if !known {
			unit = len(seen.order)
			seen.units[key] = unit
			seen.order = append(seen.order, key)
			seen.langs = append(seen.langs, lang)
		}

		seen.sightings = append(seen.sightings, sighting{
			at: position, unit: unit, rev: change.Rev, path: file.Path,
		})
	}
}

var errNoHistory = errors.New("no source history to scan")
