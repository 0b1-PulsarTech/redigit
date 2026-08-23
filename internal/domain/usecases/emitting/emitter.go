package emitting

import (
	"context"
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// Emitter turns a reviewed plan into a materialised emission: per commit, the
// declarations born there, the resulting file blobs, and the forward
// references it can see without compiling anything. It writes no history -
// that judgement belongs to the agent that consumes what this produces (see
// docs/PLANO-EMISSAO.md section 1.1).
type Emitter struct {
	source   Source
	renderer Renderer
	sink     Sink
}

func New(source Source, renderer Renderer, sink Sink) Emitter {
	return Emitter{source: source, renderer: renderer, sink: sink}
}

// Emit materialises every commit in the plan, in order. graph supplies what
// the plan itself does not carry: a declaration's span and unit, and the
// edges that turn into risks.
func (e Emitter) Emit(
	ctx context.Context,
	plan entities.Plan,
	graph entities.Graph,
) ([]Commit, error) {
	if err := check(plan); err != nil {
		return nil, err
	}

	positions := declPositions(plan)
	if err := checkCoverage(graph, positions); err != nil {
		return nil, err
	}

	born := map[string][]entities.DeclKey{}
	cache := map[string]sourceFile{}
	commits := make([]Commit, 0, plan.Len())

	for _, planned := range plan.Commits {
		touched := map[string]bool{}

		for _, key := range planned.Decls {
			born[key.File] = append(born[key.File], key)
			touched[key.File] = true
		}

		blobs, err := e.renderTouched(ctx, touched, born, cache)
		if err != nil {
			return nil, err
		}

		chunks, err := chunksFor(planned, graph)
		if err != nil {
			return nil, err
		}

		commit := Commit{
			Position: planned.Position,
			Subject:  planned.Subject,
			Shape:    planned.Shape,
			Feature:  planned.Feature,
			Reason:   planned.Reason,
			IsWIP:    planned.IsWIP,
			Chunks:   chunks,
			Blobs:    blobs,
			Risks:    risksFor(planned, graph, positions),
		}

		if commit.IsEmpty() {
			return nil, fmt.Errorf(
				"%w: commit %d (%q) changes nothing",
				apperr.ErrIncomplete, planned.Position, planned.Subject,
			)
		}

		commits = append(commits, commit)
	}

	return commits, nil
}

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

	return nil
}

// checkCoverage refuses to proceed on a declaration the graph knows about
// that landed in no commit. check already refuses any plan carrying a
// non-empty Unplaced, so by the time this runs every declaration the planner
// gave up on has already stopped the run; a graph declaration still missing
// from positions is not a deferral the planner recorded, it is a hole the
// plan never accounted for at all - the same hazard Plan.Unplaced exists to
// name, just missed by whoever built or edited this plan.
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
