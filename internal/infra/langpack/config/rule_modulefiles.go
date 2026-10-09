package config

import (
	"context"
	"maps"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// moduleFilesRule births a module's own files with its go.mod: the README, the
// Taskfile, the configuration samples sitting in its root. They have a module
// edge and no reader, so each was left for the trailing commit; the READMEs of
// a 1208-commit history were born at 96% of it.
//
// They join the header's atom rather than gaining edges of their own: the
// header is already waiting for the go.work use line in that atom, the files
// need the header, and one commit holds all of it.
type moduleFilesRule struct{}

func (moduleFilesRule) Name() string { return "module-files" }

func (moduleFilesRule) Edges(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Edge, error) {
	return nil, nil
}

// Anchors lower each file to its header: the file is first touched wherever
// the author wrote it, and the atom is born at the latest of its members.
func (moduleFilesRule) Anchors(
	_ context.Context,
	_ entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	var anchors []entities.Anchor

	forEachModuleFile(decls, func(file, header entities.Decl) {
		anchors = append(anchors, entities.Anchor{
			Decl:   file.Key,
			To:     header.Key,
			Reason: entities.ReasonHosts,
		})
	})

	return anchors, nil
}

func (moduleFilesRule) Atoms(
	_ context.Context,
	_ entities.Snapshot,
	decls []entities.Decl,
) (map[entities.DeclKey]string, error) {
	atoms := map[entities.DeclKey]string{}

	forEachModuleFile(decls, func(file, header entities.Decl) {
		if header.Atom != "" {
			atoms[file.Key] = header.Atom
		}
	})

	return atoms, nil
}

func forEachModuleFile(decls []entities.Decl, visit func(file, header entities.Decl)) {
	headers := moduleHeaders(decls)
	roots := slices.Sorted(maps.Keys(headers))
	placed := classify(decls)

	byKey := make(map[entities.DeclKey]entities.Decl, len(headers))

	for _, decl := range decls {
		byKey[decl.Key] = decl
	}

	for _, decl := range decls {
		if placed[decl.Key] != roleModuleOwn {
			continue
		}

		if root, found := nearestModule(roots, decl.Key.File); found {
			visit(decl, byKey[headers[root]])
		}
	}
}
