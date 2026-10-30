package lineage

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

var errNoHistory = errors.New("no source history to scan")

type Scanner struct {
	log   RevisionLog
	cfg   Config
	limit int
}

func NewScanner(log RevisionLog, cfg Config) Scanner {
	const defaultConcurrency = 8

	limit := cfg.Concurrency
	if limit < 1 {
		limit = defaultConcurrency
	}

	return Scanner{log: log, cfg: cfg, limit: limit}
}

func (scanner Scanner) Scan(ctx context.Context, snap entities.Snapshot) (Result, error) {
	if !snap.HasHistory() {
		return Result{}, fmt.Errorf("%w: the snapshot carries no history", errNoHistory)
	}

	// One streamed pass; each distinct (blob, path) is parsed once.
	hist, err := scanner.collect(ctx, snap)
	if err != nil {
		return Result{}, err
	}

	units, err := scanner.parseAll(ctx, hist.units)
	if err != nil {
		return Result{}, err
	}

	slices.SortStableFunc(hist.sightings, func(left, right sighting) int {
		return cmp.Compare(left.at, right.at)
	})

	births := make(map[entities.DeclKey]Birth)

	for _, seen := range hist.sightings {
		if !units[seen.unit].ok {
			hist.unreadable = append(hist.unreadable, seen.rev+":"+seen.path)

			continue
		}

		for _, key := range units[seen.unit].keys {
			observe(births, key, seen.at)
		}

		if seen.move == nil || !units[seen.move.fromUnit].ok {
			continue
		}

		if err = scanner.inherit(ctx, births, seen, pairing{hist: hist, units: units}); err != nil {
			return Result{}, err
		}
	}

	slices.Sort(hist.unreadable)

	return Result{Births: births, Unreadable: hist.unreadable}, nil
}

func observe(births map[entities.DeclKey]Birth, key entities.DeclKey, at entities.Position) {
	if current, ok := births[key]; !ok || at < current.At {
		births[key] = Birth{At: at}
	}
}

type parsed struct {
	keys []entities.DeclKey
	cuts []entities.Decl
	ok   bool
}

func (scanner Scanner) parseAll(ctx context.Context, units []unit) ([]parsed, error) {
	out := make([]parsed, len(units))

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(scanner.limit)

	for idx, one := range units {
		group.Go(func() error {
			source, err := scanner.log.ReadBlob(groupCtx, one.blob)
			if err != nil {
				ctxErr := interrupted(groupCtx)
				if ctxErr != nil || errors.Is(err, ErrBlobMissing) {
					return ctxErr
				}

				return fmt.Errorf("read blob: %w", err)
			}

			found, err := scanner.cfg.Languages[one.lang].Parser.Declarations(
				groupCtx, one.path, source,
			)
			if err != nil {
				return interrupted(groupCtx)
			}

			out[idx] = parsed{keys: keysOf(found), ok: true}

			if one.needsSpans {
				out[idx].cuts = cutsOf(found)
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, fmt.Errorf("scan lineage: %w", err)
	}

	return out, nil
}

func interrupted(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("interrupted: %w", err)
	}

	return nil
}

func keysOf(found []entities.Decl) []entities.DeclKey {
	out := make([]entities.DeclKey, 0, len(found))
	for _, decl := range found {
		out = append(out, decl.Key)
	}

	return out
}

func cutsOf(found []entities.Decl) []entities.Decl {
	out := make([]entities.Decl, 0, len(found))
	for _, decl := range found {
		out = append(out, entities.Decl{Key: decl.Key, Span: decl.Span})
	}

	return out
}
