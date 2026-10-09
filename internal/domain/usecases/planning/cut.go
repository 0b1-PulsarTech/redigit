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
	keys = placeTests(graph, keys)
	lastOf := atomExtent(graph, keys)

	var (
		start    int
		out      []entities.PlannedCommit
		current  []entities.DeclKey
		files    = map[string]bool{}
		pkg      string
		boundary int
		size     tally
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
			Reason:  annotate(in.reason, p.tallyOf(graph, current[:upTo]), p.cfg.MaxAddedLines),
		})

		current = slices.Clone(current[upTo:])
		start += upTo
		files = map[string]bool{}
		size = p.tallyOf(graph, current)

		for _, key := range current {
			files[key.File] = true
		}

		boundary = 0
	}

	for _, key := range keys {
		if len(current) > 0 && dirOf(key.File) != pkg {
			boundary = len(current)
		}

		weighed := p.weightOf(graph, key)

		// Roll back to the last package boundary instead of the raw cap, or a
		// package with no boundary yet before it: the cap stays an upper bound,
		// except where holding it would split an atom.
		tooWide := !files[key.File] && len(files) >= p.cfg.MaxFiles
		if tooWide || size.overflows(weighed, p.cfg.MaxAddedLines) {
			cutAt := boundary
			if cutAt == 0 {
				cutAt = len(current)
			}

			flush(safeCut(lastOf, start, cutAt))
		}

		// The package boundary only picks where the first cut falls. What stays
		// behind may still be too long to take the key, and is cut whole.
		for size.overflows(weighed, p.cfg.MaxAddedLines) {
			cutAt := safeCut(lastOf, start, len(current))
			if cutAt == 0 {
				break
			}

			flush(cutAt)
		}

		current = append(current, key)
		files[key.File] = true
		pkg = dirOf(key.File)

		size.add(weighed)
	}

	flush(len(current))

	return out
}

// atomExtent maps each key's index to the index of the last member of its atom,
// or to its own when it belongs to none. An atom is born in one commit, and
// members that tied at one position arrive interleaved with unrelated
// declarations, so a cut between two of them is not a cut the plan may make.
func atomExtent(graph entities.Graph, keys []entities.DeclKey) []int {
	last := map[string]int{}
	atomOf := make([]string, len(keys))

	for index, key := range keys {
		decl, found := graph.Decl(key)
		if !found || decl.Atom == "" {
			continue
		}

		atomOf[index] = decl.Atom
		last[decl.Atom] = index
	}

	extent := make([]int, len(keys))

	for index := range keys {
		extent[index] = index

		if atomOf[index] != "" {
			extent[index] = last[atomOf[index]]
		}
	}

	return extent
}

// safeCut is the largest cut at or before want that leaves no atom in two
// commits: current[:cut] holds keys start..start+cut-1, and none of them may
// have an atom sibling at start+cut or later. It reports 0, which flushes
// nothing, when every cut would split one: a commit over the file cap is
// better than a commit that cannot compile.
func safeCut(extent []int, start, want int) int {
	for cut := want; cut > 0; cut-- {
		if cutKeepsAtoms(extent, start, cut) {
			return cut
		}
	}

	return 0
}

func cutKeepsAtoms(extent []int, start, cut int) bool {
	for index := start; index < start+cut; index++ {
		if extent[index] >= start+cut {
			return false
		}
	}

	return true
}
