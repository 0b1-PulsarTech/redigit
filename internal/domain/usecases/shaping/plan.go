package shaping

import (
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func (draft *Draft) Plan() entities.Plan {
	commits := make([]entities.PlannedCommit, 0, len(draft.slots))

	for _, slot := range draft.slots {
		if len(slot.Commit.Decls) == 0 {
			continue
		}

		commit := slot.Commit
		commit.Decls = slices.Clone(commit.Decls)
		commit.Position = entities.Position(len(commits))

		// A pass knows what it moved, not what a person would call it.
		if slot.Added && !entities.IsDraft(commit.Subject) {
			commit.Subject = entities.DraftMarker + commit.Subject
		}

		commits = append(commits, commit)
	}

	return entities.Plan{Commits: commits, Unplaced: slices.Clone(draft.unplaced)}
}
