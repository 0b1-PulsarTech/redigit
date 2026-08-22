package golang

import "github.com/wrapped-owls/redigit/internal/domain/entities"

// groupByFile indexes declarations by the file they came from, the shape
// every rule that reads a file's sibling declarations needs.
func groupByFile(decls []entities.Decl) map[string][]entities.Decl {
	byFile := make(map[string][]entities.Decl, len(decls))
	for _, d := range decls {
		byFile[d.Key.File] = append(byFile[d.Key.File], d)
	}

	return byFile
}

// findDecl looks up a declaration by its exact file, kind, and name.
func findDecl(
	decls []entities.Decl,
	file string,
	kind entities.DeclKind,
	name string,
) (entities.DeclKey, bool) {
	for _, d := range decls {
		if d.Key.File == file && d.Key.Kind == kind && d.Key.Name == name {
			return d.Key, true
		}
	}

	return entities.DeclKey{}, false
}

// findTypeNamed looks up a type declaration by name alone, for callers that
// already scoped decls to the one file or directory that matters.
func findTypeNamed(decls []entities.Decl, name string) (entities.DeclKey, bool) {
	for _, d := range decls {
		if d.Key.Kind == entities.KindType && d.Key.Name == name {
			return d.Key, true
		}
	}

	return entities.DeclKey{}, false
}
