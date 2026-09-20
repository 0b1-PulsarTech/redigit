package planning

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// bucket is one group of declarations headed for commits of their own.
type bucket struct {
	keys    []entities.DeclKey
	shape   entities.CommitShape
	feature FeatureID
	reason  string
}

func (p Planner) cut(
	graph entities.Graph,
	order entities.Ordering,
	in bucket,
) []entities.PlannedCommit {
	keys := sortForBirth(graph, order, in.keys, nil)

	var (
		out      []entities.PlannedCommit
		current  []entities.DeclKey
		files    = map[string]bool{}
		pkg      string
		boundary int
	)

	flush := func(upTo int) {
		if upTo == 0 {
			return
		}

		out = append(out, entities.PlannedCommit{
			Subject: draftSubject(in.shape, in.feature, current[:upTo]),
			Shape:   in.shape,
			Feature: string(in.feature),
			Decls:   slices.Clone(current[:upTo]),
			Reason:  in.reason,
		})

		current = slices.Clone(current[upTo:])
		files = map[string]bool{}

		for _, key := range current {
			files[key.File] = true
		}

		boundary = 0
	}

	for _, key := range keys {
		if len(current) > 0 && dirOf(key.File) != pkg {
			boundary = len(current)
		}

		// Roll back to the last package boundary instead of the raw cap, or a
		// package with no boundary yet before it: the cap stays an upper bound.
		if !files[key.File] && len(files) >= p.cfg.MaxFiles {
			cutAt := boundary
			if cutAt == 0 {
				cutAt = len(current)
			}

			flush(cutAt)
		}

		current = append(current, key)
		files[key.File] = true
		pkg = dirOf(key.File)
	}

	flush(len(current))

	return out
}
