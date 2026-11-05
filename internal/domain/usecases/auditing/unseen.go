package auditing

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// Unseen is one file a build constraint left out of the main build. It is not
// a Blind query, which ERRORED: a hidden file answers the syntactic query. Typed
// answers its reference queries if it type-checks; lsp may leave its declarations
// with no edges, indistinguishable from code nothing references.
type Unseen struct {
	File       string
	Constraint string
	Tags       []string
}

// resolverCouldSee counts declarations whose file the main build holds. It is
// integrity, not shape: under lsp a hidden declaration may be missing its
// edges, so a shape number over it measures absence and reports it as tidiness.
func (a Auditor) resolverCouldSee(graph entities.Graph) entities.Measure {
	hidden := make(map[string]bool, len(a.cfg.Unseen))
	for _, unseen := range a.cfg.Unseen {
		hidden[unseen.File] = true
	}

	visible := 0

	for _, decl := range graph.Decls() {
		if !hidden[decl.Key.File] {
			visible++
		}
	}

	return entities.Measure{
		Name:  "declarations in a file no build constraint left out of the main build",
		Value: visible,
		Total: graph.Len(),
		Excluded: []string{
			"whether a readable file's queries came back empty for some other reason",
			"a constraint this run DID satisfy, which hides nothing and is not counted",
			"a file no language pack reports a constraint for, which is assumed readable",
		},
	}
}
