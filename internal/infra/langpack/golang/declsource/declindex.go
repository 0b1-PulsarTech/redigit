package declsource

import "github.com/wrapped-owls/redigit/internal/domain/entities"

func GroupByFile(decls []entities.Decl) map[string][]entities.Decl {
	byFile := make(map[string][]entities.Decl, len(decls))
	for _, d := range decls {
		byFile[d.Key.File] = append(byFile[d.Key.File], d)
	}

	return byFile
}

func FindDecl(
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

func FindTypeNamed(decls []entities.Decl, name string) (entities.DeclKey, bool) {
	for _, d := range decls {
		if d.Key.Kind == entities.KindType && d.Key.Name == name {
			return d.Key, true
		}
	}

	return entities.DeclKey{}, false
}
