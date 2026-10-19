package planning

import (
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// isDocument is a Markdown file under a docs directory. A document is cut into
// commits of its own: riding in a commit of code it merely names, a 725-line spec
// arrived with three lines of a kernel package and the commit could be named for
// neither.
func isDocument(key entities.DeclKey) bool {
	return key.Kind == entities.KindOther && path.Ext(key.File) == ".md" &&
		slices.Contains(strings.Split(path.Dir(key.File), "/"), "docs")
}

// gatherDocuments moves each document up to sit with the documents before it, as
// far as what it needs allows, so the cut makes one commit of documents instead
// of one for every stretch of code they are scattered between.
func gatherDocuments(graph entities.Graph, keys []entities.DeclKey) []entities.DeclKey {
	out := make([]entities.DeclKey, 0, len(keys))
	placed := map[entities.DeclKey]int{}
	lastDocument := -1

	for _, key := range keys {
		at := len(out)

		if decl, found := graph.Decl(key); isDocument(key) && found && decl.Atom == "" {
			at = lastDocument + 1

			for _, edge := range graph.Needs(key) {
				if index, inside := placed[edge.Provider]; inside && index >= at {
					at = index + 1
				}
			}
		}

		out = slices.Insert(out, at, key)

		for member, index := range placed {
			if index >= at {
				placed[member] = index + 1
			}
		}

		placed[key] = at

		if isDocument(key) {
			lastDocument = max(lastDocument, at)
		}
	}

	return out
}
