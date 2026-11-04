package emitting

import (
	"fmt"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// check refuses a plan this stage cannot trust: one that is empty, one the
// planner already gave up on part of, one whose positions are not the dense
// ordinal sequence section 3 of the design doc promises, or one that places
// the same declaration in two commits (which would duplicate its content into
// every blob rendered from that point on).
func check(plan entities.Plan) error {
	if plan.Len() == 0 {
		return fmt.Errorf("%w: the plan holds no commit", apperr.ErrIncomplete)
	}

	if len(plan.Unplaced) > 0 {
		return fmt.Errorf(
			"%w: %d declarations are in no commit, starting at %s",
			apperr.ErrIncomplete, len(plan.Unplaced), plan.Unplaced[0],
		)
	}

	placed := map[entities.DeclKey]bool{}

	for i, commit := range plan.Commits {
		if commit.Position != entities.Position(i) {
			return fmt.Errorf(
				"%w: commit at index %d carries position %d, want the dense ordinal sequence",
				apperr.ErrIncomplete, i, commit.Position,
			)
		}

		for _, key := range commit.Changes {
			if !placed[key] {
				return fmt.Errorf(
					"%w: commit %d changes %s before any commit births it",
					apperr.ErrIncomplete, commit.Position, key,
				)
			}
		}

		for _, key := range commit.Decls {
			if placed[key] {
				return fmt.Errorf(
					"%w: declaration %s is placed in more than one commit",
					apperr.ErrIncomplete, key,
				)
			}

			placed[key] = true
		}
	}

	return checkVersions(plan)
}

func checkVersions(plan entities.Plan) error {
	for key, versions := range plan.Versions {
		for _, version := range versions {
			at := int(version.Until)
			if at < 0 || at >= plan.Len() || !slices.Contains(plan.Commits[at].Changes, key) {
				return fmt.Errorf(
					"%w: a pre-image of %s ends at %d, which is no commit changing it",
					apperr.ErrIncomplete, key, version.Until,
				)
			}
		}
	}

	return nil
}

// checkCoverage refuses to proceed on a declaration the graph knows about that
// landed in no commit. check already refuses a plan with a non-empty Unplaced,
// so one still missing here is not a deferral the planner recorded: it is a
// hole the plan never accounted for.
func checkCoverage(graph entities.Graph, positions map[entities.DeclKey]entities.Position) error {
	for _, decl := range graph.Decls() {
		if _, ok := positions[decl.Key]; !ok {
			return fmt.Errorf(
				"%w: declaration %s is known to the graph but placed in no commit",
				apperr.ErrIncomplete, decl.Key,
			)
		}
	}

	return nil
}

// declPositions maps every placed declaration to the commit it is born in,
// which is what turns a forward reference (section 3 of the design doc) into
// a Risk without needing to compile anything.
func declPositions(plan entities.Plan) map[entities.DeclKey]entities.Position {
	positions := make(map[entities.DeclKey]entities.Position)

	for _, commit := range plan.Commits {
		for _, key := range commit.Decls {
			positions[key] = commit.Position
		}
	}

	return positions
}
