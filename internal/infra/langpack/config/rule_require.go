package config

import (
	"path"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// requireEdges makes a "require X" wait for its own file's "replace X" and,
// when that replace points at a local directory, for that directory's module.
// The lines are one fact: a locally replaced require carries the pseudo-version
// v0.0.0-00010101000000-000000000000, and without the replace or its target the
// go command reaches the network for it and fails.
func requireEdges(
	decls []entities.Decl,
	headers map[string]entities.DeclKey,
	replaced map[string]string,
) []entities.Edge {
	replaces := map[entities.DeclKey]bool{}

	for _, decl := range decls {
		if strings.HasPrefix(decl.Key.Name, replacePrefix) &&
			path.Base(decl.Key.File) == goModFile {
			replaces[decl.Key] = true
		}
	}

	var edges []entities.Edge

	for _, decl := range decls {
		module, isRequire := strings.CutPrefix(decl.Key.Name, requirePrefix)
		if !isRequire || path.Base(decl.Key.File) != goModFile {
			continue
		}

		replace := entities.DeclKey{
			File: decl.Key.File,
			Kind: entities.KindOther,
			Name: replacePrefix + module,
		}

		if replaces[replace] {
			edges = append(edges, entities.Edge{
				Dependent: decl.Key,
				Provider:  replace,
				Reason:    entities.ReasonWorkspace,
			})
		}

		if provider, found := headers[replaced[decl.Key.File+"\x00"+module]]; found {
			edges = append(edges, entities.Edge{
				Dependent: decl.Key,
				Provider:  provider,
				Reason:    entities.ReasonWorkspace,
			})
		}
	}

	return edges
}
