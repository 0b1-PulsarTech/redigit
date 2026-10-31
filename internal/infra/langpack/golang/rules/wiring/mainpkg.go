package wiring

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
)

type MainPackage struct{}

func (MainPackage) Name() string { return "main-package" }

func (MainPackage) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	mainDirs := map[string]bool{}

	for _, decl := range decls {
		if isMainFunc(decl.Key) {
			mainDirs[path.Dir(decl.Key.File)] = true
		}
	}

	if len(mainDirs) == 0 {
		return nil, nil
	}

	inMain, err := packageMainFiles(snap, decls, mainDirs)
	if err != nil {
		return nil, err
	}

	progs, err := programFiles(snap, inMain)
	if err != nil {
		return nil, err
	}

	byFile, byDir := hostMains(decls, inMain, progs)

	var edges []entities.Edge

	for _, decl := range decls {
		key := decl.Key
		if key.Kind == entities.KindStmt || isMainFunc(key) || !inMain[key.File] {
			continue
		}

		// A tag-switched main is only the host of its own file; other files wait for each one.
		// Where a main builds on every platform, the shared file waits for those alone.
		providers := byFile[key.File]
		if len(providers) == 0 {
			providers = unconstrained(byDir[path.Dir(key.File)], progs.plain)
		}

		for _, provider := range providers {
			edges = append(edges, entities.Edge{
				Dependent: key, Provider: provider, Reason: entities.ReasonHosts,
			})
		}
	}

	slices.SortFunc(edges, compareHosts)

	return edges, nil
}

func isMainFunc(key entities.DeclKey) bool {
	return key.Kind == entities.KindFunc && key.Name == "main" &&
		!strings.HasSuffix(key.File, "_test.go")
}

func packageMainFiles(
	snap entities.Snapshot,
	decls []entities.Decl,
	mainDirs map[string]bool,
) (map[string]bool, error) {
	inMain := map[string]bool{}
	seen := map[string]bool{}

	for _, decl := range decls {
		file := decl.Key.File
		if seen[file] || path.Ext(file) != ".go" || isIgnoredName(path.Base(file)) ||
			strings.HasSuffix(file, "_test.go") ||
			!mainDirs[path.Dir(file)] {
			continue
		}

		seen[file] = true

		source, err := declsource.ReadSource(snap, file)
		if err != nil {
			return nil, fmt.Errorf("main package source: %w", err)
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), file, source, parser.PackageClauseOnly)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", file, err)
		}

		inMain[file] = parsed.Name.Name == "main"
	}

	return inMain, nil
}

func compareHosts(left, right entities.Edge) int {
	if order := entities.CompareKeys(left.Dependent, right.Dependent); order != 0 {
		return order
	}

	return entities.CompareKeys(left.Provider, right.Provider)
}

func (MainPackage) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}
