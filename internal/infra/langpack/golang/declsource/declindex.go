package declsource

import "github.com/wrapped-owls/redigit/internal/domain/entities"

func GroupByFile(decls []entities.Decl) map[string][]entities.Decl {
	byFile := make(map[string][]entities.Decl, len(decls))
	for _, decl := range decls {
		byFile[decl.Key.File] = append(byFile[decl.Key.File], decl)
	}

	return byFile
}

func FindDecl(
	decls []entities.Decl,
	file string,
	kind entities.DeclKind,
	name string,
) (entities.DeclKey, bool) {
	for _, decl := range decls {
		if decl.Key.File == file && decl.Key.Kind == kind && decl.Key.Name == name {
			return decl.Key, true
		}
	}

	return entities.DeclKey{}, false
}

func FindTypeNamed(decls []entities.Decl, name string) (entities.DeclKey, bool) {
	for _, decl := range decls {
		if decl.Key.Kind == entities.KindType && decl.Key.Name == name {
			return decl.Key, true
		}
	}

	return entities.DeclKey{}, false
}
