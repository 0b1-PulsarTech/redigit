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
	derivers []Deriver
	// derived is set by Emit for the run in flight; see deriving.
	derived *derivation
}

func New(source Source, renderer Renderer, sink Sink, derivers ...Deriver) Emitter {
	return Emitter{source: source, renderer: renderer, sink: sink, derivers: derivers}
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

	e = e.deriving(plan)
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

		blobs, err = e.deriveTouched(ctx, touched, blobs, cache)
		if err != nil {
			return nil, err
		}

		chunks, err := chunksFor(planned, graph)
		if err != nil {
			return nil, err
		}

		risks := risksFor(planned, graph, positions)

		// A commit that leaves a piece incomplete says so. Only the final tree
		// has to be green; a vertical slice routinely lands one that is not,
		// and an unmarked one is indistinguishable from a defect.
		subject, isWIP := planned.Subject, planned.IsWIP
		if len(risks) > 0 {
			subject, isWIP = entities.MarkWIP(subject), true
		}

		commit := Commit{
			Position: planned.Position,
			Subject:  subject,
			Shape:    planned.Shape,
			Feature:  planned.Feature,
			Reason:   planned.Reason,
			IsWIP:    isWIP,
			Chunks:   chunks,
			Blobs:    blobs,
			Risks:    risks,
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
