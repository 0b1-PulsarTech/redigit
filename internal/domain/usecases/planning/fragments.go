package planning

import (
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// mergeFragments folds a commit that is only an interface assertion, or that
// adds fewer than MinCommitLines lines, into the commit beside it
// when the caps allow. Only neighbours merge, so no declaration moves past
// another commit's: the assertion was ready only after the code it names, and
// that code sits in the commit just before it.
func (p Planner) mergeFragments(
	graph entities.Graph,
	commits []entities.PlannedCommit,
) []entities.PlannedCommit {
	if p.cfg.MinCommitLines < 1 {
		return commits
	}

	out := make([]entities.PlannedCommit, 0, len(commits))

	for index, commit := range commits {
		if !p.isFragment(graph, commit) {
			out = append(out, commit)

			continue
		}

		if last := len(out) - 1; last >= 0 && p.canMerge(graph, out[last], commit) {
			out[last] = joinCommits(out[last], commit)

			continue
		}

		if index+1 < len(commits) && p.canMerge(graph, commit, commits[index+1]) {
			host := commits[index+1]
			host.Decls = append(slices.Clone(commit.Decls), host.Decls...)
			host.Subject = draftSubject(host.Shape, FeatureID(host.Feature), host.Decls)
			host.IsWIP = host.IsWIP || commit.IsWIP
			commits[index+1] = host

			continue
		}

		out = append(out, commit)
	}

	return out
}

func joinCommits(first, second entities.PlannedCommit) entities.PlannedCommit {
	first.Decls = append(slices.Clone(first.Decls), second.Decls...)
	first.Subject = draftSubject(first.Shape, FeatureID(first.Feature), first.Decls)
	first.IsWIP = first.IsWIP || second.IsWIP

	return first
}

func (p Planner) isFragment(graph entities.Graph, commit entities.PlannedCommit) bool {
	if len(commit.Decls) == 0 {
		return false
	}

	if allAssertions(commit.Decls) {
		return true
	}

	size := p.tallyOf(graph, commit.Decls)

	return size.hand+size.generated < p.cfg.MinCommitLines
}

func (p Planner) canMerge(graph entities.Graph, first, second entities.PlannedCommit) bool {
	if first.Shape != second.Shape || isDocument(first.Decls[0]) != isDocument(second.Decls[0]) {
		return false
	}

	joined := append(slices.Clone(first.Decls), second.Decls...)
	size := p.tallyOf(graph, joined)

	return max(size.hand, size.generated) <= p.cfg.MaxAddedLines &&
		len(size.files) <= p.cfg.MaxFiles
}

func allAssertions(keys []entities.DeclKey) bool {
	for _, key := range keys {
		if key.Kind != entities.KindValue || !strings.HasPrefix(key.Name, "_ ") {
			return false
		}
	}

	return true
}
