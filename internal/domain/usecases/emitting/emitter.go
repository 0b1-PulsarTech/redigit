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
