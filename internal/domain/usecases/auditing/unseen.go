package auditing

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// Unseen is one file a build constraint hid from the reference queries. It is
// not a Blind query: that one ERRORED, while a hidden file answers the
// syntactic query and contributes declarations carrying no edges, which is
// indistinguishable from code nothing references.
type Unseen struct {
	File       string
	Constraint string
	Tags       []string
}

// resolverCouldSee counts declarations whose file the queries could read. It
// is integrity, not shape: a hidden declaration is missing its edges, so a
// shape number over it measures absence and reports it as tidiness.
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
		Name:  "declarations in a file the reference queries could read",
		Value: visible,
		Total: graph.Len(),
		Excluded: []string{
			"whether a readable file's queries came back empty for some other reason",
			"a constraint this run DID satisfy, which hides nothing and is not counted",
			"a file no language pack reports a constraint for, which is assumed readable",
		},
	}
}
