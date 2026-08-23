package emitting

import (
	"fmt"

	"github.com/wrapped-owls/redigit/internal/domain/apperr"
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// chunksFor resolves every declaration born in planned to its Chunk, in the
// topological order topoSort produces.
func chunksFor(planned entities.PlannedCommit, graph entities.Graph) ([]Chunk, error) {
	ordered := topoSort(planned.Decls, graph)
	chunks := make([]Chunk, 0, len(ordered))

	for _, key := range ordered {
		decl, found := graph.Decl(key)
		if !found {
			return nil, fmt.Errorf(
				"%w: declaration %s has no entry in the graph",
				apperr.ErrIncomplete, key,
			)
		}

		chunks = append(chunks, Chunk{Key: key, Span: decl.Span, Unit: decl.Unit})
	}

	return chunks, nil
}

// topoSort orders decls so that, wherever one of them needs another that is
// ALSO born in this same commit, the provider comes first. This is what lets
// the agent take a prefix of Chunks and re-render it: the prefix never breaks
// a dependency this commit already satisfied.
//
// It does not, and cannot, guarantee the prefix compiles - a declaration may
// need something born in no commit yet at all, which is exactly what Risk
// reports instead.
func topoSort(decls []entities.DeclKey, graph entities.Graph) []entities.DeclKey {
	dependents, inDegree := inCommitDeps(decls, graph)

	queue := make([]entities.DeclKey, 0, len(decls))

	for _, key := range decls {
		if inDegree[key] == 0 {
			queue = append(queue, key)
		}
	}

	ordered, visited := kahn(queue, dependents, inDegree, len(decls))

	// simplification: a cycle among a single commit's own declarations should
	// already have been resolved upstream, by the planner. If one still slips
	// through, append whatever is left in the plan's own order rather than
	// looping forever - nothing here decides that the cycle is fine, it just
	// refuses to drop a declaration silently.
	if len(ordered) < len(decls) {
		for _, key := range decls {
			if !visited[key] {
				ordered = append(ordered, key)
			}
		}
	}

	return ordered
}

// inCommitDeps restricts graph.Needs to edges between two members of decls:
// an edge leaving the commit says nothing about the order WITHIN it.
func inCommitDeps(
	decls []entities.DeclKey,
	graph entities.Graph,
) (dependents map[entities.DeclKey][]entities.DeclKey, inDegree map[entities.DeclKey]int) {
	inSet := make(map[entities.DeclKey]bool, len(decls))
	for _, key := range decls {
		inSet[key] = true
	}

	dependents = make(map[entities.DeclKey][]entities.DeclKey, len(decls))
	inDegree = make(map[entities.DeclKey]int, len(decls))

	for _, key := range decls {
		providers := map[entities.DeclKey]bool{}

		for _, edge := range graph.Needs(key) {
			if edge.Provider == key || !inSet[edge.Provider] {
				continue
			}

			providers[edge.Provider] = true
		}

		inDegree[key] = len(providers)

		for provider := range providers {
			dependents[provider] = append(dependents[provider], key)
		}
	}

	return dependents, inDegree
}

// kahn drains queue breadth-first, releasing a dependent as soon as every
// in-commit provider it waited on has been emitted.
func kahn(
	queue []entities.DeclKey,
	dependents map[entities.DeclKey][]entities.DeclKey,
	inDegree map[entities.DeclKey]int,
	total int,
) (ordered []entities.DeclKey, visited map[entities.DeclKey]bool) {
	ordered = make([]entities.DeclKey, 0, total)
	visited = make(map[entities.DeclKey]bool, total)

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		if visited[node] {
			continue
		}

		visited[node] = true

		ordered = append(ordered, node)

		for _, dependent := range dependents[node] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	return ordered, visited
}
