package planning

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// cut splits one bucket into commits no wider than MaxFiles, keeping the
// declarations in the order the fixed point put them so a commit never carries
// a reader ahead of what it reads.
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
	keys, shape, feature, reason := in.keys, in.shape, in.feature, in.reason

	keys = sortForBirth(graph, order, keys, nil)

	var (
		out     []entities.PlannedCommit
		current []entities.DeclKey
		files   = map[string]bool{}
	)

	flush := func() {
		if len(current) == 0 {
			return
		}

		out = append(out, entities.PlannedCommit{
			Subject: draftSubject(shape, feature, current),
			Shape:   shape,
			Feature: string(feature),
			Decls:   slices.Clone(current),
			Reason:  reason,
		})
		current = current[:0]
		files = map[string]bool{}
	}

	for _, key := range keys {
		if !files[key.File] && len(files) >= p.cfg.MaxFiles {
			flush()
		}

		files[key.File] = true

		current = append(current, key)
	}

	flush()

	return out
}
