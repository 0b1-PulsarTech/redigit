package anchoring

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

const outcomeKept = "kept"

func (planner Planner) policyOf(anchor Anchor) (Policy, error) {
	policy := cmp.Or(anchor.Policy, planner.cfg.Policy, PolicyFold)

	return policy, checkPolicy(policy)
}

func (planner Planner) decide(
	ctx context.Context,
	graph entities.Graph,
	list []found,
	judge judgement,
) ([]kept, error) {
	var out []kept

	keep := keysByFile(graph)
	active := map[entities.DeclKey]entry{}
	// What no probe sees still reaches the emitter, which splices every chosen pre-image.
	var claimed []entry

	// Newest first, so the probe of an older anchor sees the pre-images a newer one kept
	// and judged built.
	for _, one := range slices.Backward(list) {
		policy, err := planner.policyOf(one.anchor)
		if err != nil {
			return nil, err
		}

		if len(one.entries) == 0 {
			out = append(
				out,
				kept{found: one, outcome: outcomeKept, reason: "it births declarations"},
			)

			continue
		}

		if clash := overlapped(claimed, one.entries); clash != "" {
			planner.fold(one, "its pre-image overlaps "+clash+", which a newer anchor keeps")

			continue
		}

		trial := maps.Clone(active)
		for _, placed := range one.entries {
			trial[placed.key] = placed
		}

		verdict, err := planner.verdictOf(ctx, one, editsOf(trial, keep), judge)
		if err != nil {
			return nil, err
		}

		switch {
		case verdict.IsUnfit:
			planner.fold(one, verdict.Detail)
		case verdict.IsUnchecked:
			claimed = append(claimed, one.entries...)
			out = append(out, kept{found: one, outcome: "unchecked", reason: verdict.Detail})
		case verdict.IsBuilt:
			// Only a pre-image that builds may stand under an older anchor's trial.
			active = trial

			built := kept{found: one, outcome: outcomeKept, trial: trial}
			built.reason = "its pre-image builds"
			out = append(out, built)
		case policy == PolicyFold:
			planner.fold(one, verdict.Detail)
		default:
			claimed = append(claimed, one.entries...)
			out = append(out, kept{found: one, isWIP: true, outcome: "wip", reason: verdict.Detail})
		}
	}

	slices.Reverse(out)

	return out, nil
}

func keysByFile(graph entities.Graph) map[string][]entities.DeclKey {
	out := map[string][]entities.DeclKey{}

	for _, decl := range graph.Decls() {
		out[decl.Key.File] = append(out[decl.Key.File], decl.Key)
	}

	return out
}

func editsOf(active map[entities.DeclKey]entry, keep map[string][]entities.DeclKey) []Edit {
	edits := make([]Edit, 0, len(active))

	for _, placed := range active {
		edits = append(edits, Edit{
			File:    placed.key.File,
			Span:    placed.span,
			Text:    placed.preimage,
			Imports: placed.imports,
			Keep:    keep[placed.key.File],
		})
	}

	slices.SortFunc(edits, func(left, right Edit) int {
		return cmp.Or(
			strings.Compare(left.File, right.File), cmp.Compare(left.Span.Start, right.Span.Start),
			cmp.Compare(left.Span.End, right.Span.End), strings.Compare(left.Text, right.Text),
		)
	})

	return slices.CompactFunc(edits, func(left, right Edit) bool {
		return left.File == right.File && left.Span == right.Span && left.Text == right.Text
	})
}

func (planner Planner) verdictOf(
	ctx context.Context,
	one found,
	edits []Edit,
	judge judgement,
) (Verdict, error) {
	rev := one.anchor.Rev
	if reason, isForced := judge.forced[rev]; isForced {
		return Verdict{Detail: reason}, nil
	}

	memo := rev + fmt.Sprint(edits)
	if verdict, isKnown := judge.memo[memo]; isKnown {
		return verdict, nil
	}

	verdict, err := planner.cfg.Probe.Builds(ctx, edits)
	if err != nil {
		return Verdict{}, fmt.Errorf("%w: %s: %w", ErrProbe, rev, err)
	}

	judge.memo[memo] = verdict

	return verdict, nil
}

func bornBefore(one kept, index map[entities.DeclKey]int, slot int) map[entities.DeclKey]entry {
	out := map[entities.DeclKey]entry{}
	maps.Copy(out, one.trial)
	maps.DeleteFunc(out, func(key entities.DeclKey, _ entry) bool { return index[key] >= slot })

	for _, placed := range one.entries {
		out[placed.key] = placed
	}

	return out
}

func overlapped(claimed, own []entry) string {
	for _, mine := range own {
		for _, other := range claimed {
			isClash := other.key.File == mine.key.File && other.key != mine.key &&
				other.span.Start < mine.span.End && mine.span.Start < other.span.End &&
				(other.span != mine.span || other.preimage != mine.preimage)
			if isClash {
				return other.key.String()
			}
		}
	}

	return ""
}
