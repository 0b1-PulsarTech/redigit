package config

import (
	"cmp"
	"path"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// packageIndex answers what makes an in-repo package exist. A package an import
// names is not found by the go command until one of its non-test files is in
// the tree ("no required module provides package"), and when the importer's
// reference to it is one the language server cannot answer, nothing else orders
// the two.
type packageIndex struct {
	snap       entities.Snapshot
	moduleDirs map[string]string
	byDir      map[string]entities.Decl
	byName     map[string][]entities.DeclKey
}

// newPackageIndex maps each module path to its directory and each package
// directory to its earliest non-test declaration. Earliest by the first touch
// of its file: one declaration is enough to make the package exist, and the
// earliest is the one the history places first.
func newPackageIndex(
	snap entities.Snapshot,
	decls []entities.Decl,
	headers map[string]entities.DeclKey,
) packageIndex {
	index := packageIndex{
		snap:       snap,
		moduleDirs: make(map[string]string, len(headers)),
		byDir:      map[string]entities.Decl{},
		byName:     map[string][]entities.DeclKey{},
	}

	for dir, header := range headers {
		index.moduleDirs[header.Name] = dir
	}

	touched := map[string]entities.Position{}

	for _, decl := range decls {
		file := decl.Key.File
		if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
			continue
		}

		if _, seen := touched[file]; !seen {
			touched[file] = snap.FirstTouch(file)
		}

		dir := path.Dir(file)
		index.byName[dir+"\x00"+decl.Key.Name] = append(
			index.byName[dir+"\x00"+decl.Key.Name], decl.Key,
		)
		best, found := index.byDir[dir]

		if !found || earlier(touched[file], decl.Key, touched[best.Key.File], best.Key) {
			index.byDir[dir] = decl
		}
	}

	return index
}

// earlier orders by first touch, an unset one last, then by key so the choice
// is the same on every run.
func earlier(
	a entities.Position,
	aKey entities.DeclKey,
	b entities.Position,
	bKey entities.DeclKey,
) bool {
	if a.IsSet() != b.IsSet() {
		return a.IsSet()
	}

	if a != b {
		return a < b
	}

	return cmp.Less(aKey.String(), bKey.String())
}

// directoryOf resolves an import path to a repository directory through the
// longest module path that prefixes it.
func (index packageIndex) directoryOf(imported string) (string, bool) {
	best := ""

	for module := range index.moduleDirs {
		if (imported == module || strings.HasPrefix(imported, module+"/")) &&
			len(module) > len(best) {
			best = module
		}
	}

	if best == "" {
		return "", false
	}

	return path.Join(index.moduleDirs[best], strings.TrimPrefix(imported, best)), true
}
