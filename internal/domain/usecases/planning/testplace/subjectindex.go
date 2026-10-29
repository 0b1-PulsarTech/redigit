package testplace

import (
	"path"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type subjectIndex struct {
	order  entities.Ordering
	byFile map[string][]entities.DeclKey
	byDir  map[string][]entities.DeclKey
}

func newSubjectIndex(decls []entities.Decl, order entities.Ordering) subjectIndex {
	index := subjectIndex{
		order:  order,
		byFile: map[string][]entities.DeclKey{},
		byDir:  map[string][]entities.DeclKey{},
	}

	for _, decl := range decls {
		if entities.IsTestSide(decl) || decl.Key.Kind == entities.KindOther {
			continue
		}

		index.byFile[decl.Key.File] = append(index.byFile[decl.Key.File], decl.Key)
		index.byDir[path.Dir(decl.Key.File)] = append(
			index.byDir[path.Dir(decl.Key.File)], decl.Key,
		)
	}

	return index
}

func (index subjectIndex) subjectsOf(
	decl entities.Decl,
	named []entities.DeclKey,
	fileWide map[entities.DeclKey]bool,
) []entities.DeclKey {
	if twin := index.twinOf(decl.Key.File); len(twin) > 0 {
		return index.earliest(twin)
	}

	if len(named) > 0 {
		return nil
	}

	if len(fileWide) > 0 {
		out := make([]entities.DeclKey, 0, len(fileWide))
		for key := range fileWide {
			out = append(out, key)
		}

		return index.earliest(out)
	}

	return index.earliest(index.byDir[path.Dir(decl.Key.File)])
}

func (index subjectIndex) earliest(keys []entities.DeclKey) []entities.DeclKey {
	first := entities.NoPosition

	for _, key := range keys {
		if at := index.order.Of(key); first == entities.NoPosition || at < first {
			first = at
		}
	}

	var out []entities.DeclKey

	for _, key := range keys {
		if index.order.Of(key) == first {
			out = append(out, key)
		}
	}

	return out
}

func (index subjectIndex) twinOf(testFile string) []entities.DeclKey {
	dir, base := path.Split(testFile)
	segments := strings.Split(strings.TrimSuffix(base, "_test.go"), "_")

	for cut := len(segments); cut >= 1; cut-- {
		if keys, found := index.byFile[dir+strings.Join(segments[:cut], "_")+".go"]; found {
			return keys
		}
	}

	return nil
}
