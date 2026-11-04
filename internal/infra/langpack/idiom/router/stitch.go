package router

import (
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
)

type stitched struct {
	idiom.Facts

	parts map[entities.DeclKey][]entities.DeclKey
}

//nolint:ireturn // the idiom port's own Facts is what the rule consumes.
func newStitched(known idiom.Facts, decls []entities.Decl) idiom.Facts {
	var parts map[entities.DeclKey][]entities.DeclKey

	for _, decl := range decls {
		if decl.Key.Kind != entities.KindStmt {
			continue
		}

		if parts == nil {
			parts = map[entities.DeclKey][]entities.DeclKey{}
		}

		// A function cut into statements records its calls under Name#i, never at its head.
		for _, head := range heads(decl.Key)[1:] {
			parts[head] = append(parts[head], decl.Key)
		}
	}

	if parts == nil {
		return known
	}

	return stitched{Facts: known, parts: parts}
}

func (view stitched) Calls(site entities.DeclKey) []facts.Call {
	out := view.Facts.Calls(site)

	for _, part := range view.parts[site] {
		out = append(slices.Clip(out), view.Facts.Calls(part)...)
	}

	return out
}

func heads(site entities.DeclKey) []entities.DeclKey {
	if site.Kind != entities.KindStmt {
		return []entities.DeclKey{site}
	}

	name := site.Name[:strings.LastIndex(site.Name, "#")]

	return []entities.DeclKey{
		site,
		{File: site.File, Kind: entities.KindFunc, Name: name},
		{File: site.File, Kind: entities.KindMethod, Name: name},
	}
}

func enclosing(site entities.DeclKey) entities.DeclKey {
	if site.Kind == entities.KindStmt {
		return heads(site)[1]
	}

	return entities.DeclKey{File: site.File, Kind: entities.KindFunc, Name: site.Name}
}
