package anchoring

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

func (planner Planner) Plan(
	graph entities.Graph,
	order entities.Ordering,
) (entities.Plan, error) {
	plan, err := planner.cfg.Inner.Plan(graph, order)
	if err != nil {
		return entities.Plan{}, fmt.Errorf("plan before anchoring: %w", err)
	}

	if len(planner.cfg.Anchors) == 0 {
		return plan, nil
	}

	// The planner port carries no context, so the run context is bound when the planner is built.
	ctx := planner.cfg.Bound
	if ctx == nil {
		ctx = context.Background()
	}

	list, err := planner.collect(ctx, graph, placedIn(plan))
	if err != nil {
		return entities.Plan{}, err
	}

	chosen, err := planner.settle(ctx, graph, list, plan)
	if err != nil {
		return entities.Plan{}, err
	}

	return planner.insert(plan, graph, chosen), nil
}

func (planner Planner) collect(
	ctx context.Context,
	graph entities.Graph,
	placed map[entities.DeclKey]bool,
) ([]found, error) {
	anchors := slices.Clone(planner.cfg.Anchors)
	slices.SortStableFunc(anchors, func(left, right Anchor) int {
		return cmp.Compare(left.Position, right.Position)
	})

	var out []found

	for _, anchor := range anchors {
		revision, err := planner.cfg.Revisions.Revision(ctx, anchor.Rev)
		if err != nil {
			return nil, fmt.Errorf("read revision %s: %w", anchor.Rev, err)
		}

		part, err := planner.delta(ctx, graph, anchor, revision)
		if err != nil {
			return nil, fmt.Errorf("anchor %s: %w", anchor.Rev, err)
		}

		for _, skipped := range part.skipped {
			planner.reportSkip(anchor.Rev, skipped)
		}

		entries := slices.DeleteFunc(part.entries, func(one entry) bool { return !placed[one.key] })
		born := slices.DeleteFunc(
			part.born,
			func(key entities.DeclKey) bool { return !placed[key] },
		)
		one := found{
			anchor: anchor, revision: revision, entries: entries, skipped: len(part.skipped),
			born: born,
		}

		if len(entries) == 0 && len(born) == 0 {
			planner.summarize(one, "dropped", "it changes no placed declaration")

			continue
		}

		out = append(out, one)
	}

	return out, nil
}

func (planner Planner) insert(
	plan entities.Plan,
	graph entities.Graph,
	chosen []kept,
) entities.Plan {
	if len(chosen) == 0 {
		return plan
	}

	commits, slots := planner.place(slices.Clone(plan.Commits), chosen)
	commits, stayed := planner.adoptAll(commits, graph, chosen, slots)

	live := liveEntries(chosen)

	for idx, slot := range slots {
		commits[slot].Changes = keysOf(live[idx])
	}

	commits, slots = prune(commits, slots)

	for idx := range commits {
		commits[idx].Position = entities.Position(idx)
	}

	versions := maps.Clone(plan.Versions)
	if versions == nil {
		versions = map[entities.DeclKey][]entities.Version{}
	}

	for idx, one := range chosen {
		if slots[idx] < 0 {
			planner.summarize(one.found, "dropped", "it changes no placed declaration")

			continue
		}

		for _, placed := range live[idx] {
			version := entities.Version{Text: placed.preimage, Until: entities.Position(slots[idx])}
			versions[placed.key] = append(versions[placed.key], version)
		}

		marked := 0
		if one.isWIP {
			marked = markCarriers(commits, slots[idx], keysOf(live[idx]))
		}

		planner.summarize(one.found, one.outcome, one.reason,
			slog.Int("slot", slots[idx]), slog.Int("moved", len(commits[slots[idx]].Decls)),
			slog.Int("stayed", stayed[idx]), slog.Int("marked", marked))
	}

	plan.Commits = commits
	plan.Versions = versions

	return plan
}

func (planner Planner) settle(
	ctx context.Context,
	graph entities.Graph,
	list []found,
	plan entities.Plan,
) ([]kept, error) {
	judge := judgement{forced: map[string]string{}, memo: map[string]Verdict{}}
	quiet := planner
	quiet.cfg.Logger = slog.New(slog.DiscardHandler)

	// Each round forces one more anchor, so the loop ends within len(list) rounds.
	for range len(list) + 1 {
		chosen, err := quiet.decide(ctx, graph, list, judge)
		if err != nil {
			return nil, err
		}

		rev, reason := quiet.misfit(plan, graph, chosen)
		if rev == "" {
			break
		}

		judge.forced[rev] = reason
	}

	return planner.decide(ctx, graph, list, judge)
}
