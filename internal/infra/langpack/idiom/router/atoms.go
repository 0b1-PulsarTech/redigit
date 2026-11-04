package router

import (
	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type groupKey struct {
	fn   entities.DeclKey
	path string
}

func (view View) atoms(cfg Config) map[entities.DeclKey]string {
	parent := view.groupJoins(cfg)

	for _, mount := range view.Mounts {
		join(parent, mount.Fn, mount.Caller)
	}

	return labels(parent)
}

func (view View) groupJoins(cfg Config) map[entities.DeclKey]entities.DeclKey {
	parent := map[entities.DeclKey]entities.DeclKey{}
	if !cfg.GroupMethods {
		return parent
	}

	first := map[groupKey]entities.DeclKey{}

	for _, route := range view.Routes {
		if route.Path == "" || route.Site.Kind != entities.KindStmt {
			continue
		}

		key := groupKey{fn: heads(route.Site)[1], path: route.Path}
		if head, found := first[key]; found {
			join(parent, head, route.Site)

			continue
		}

		first[key] = route.Site
	}

	return parent
}

func join(parent map[entities.DeclKey]entities.DeclKey, left, right entities.DeclKey) {
	rootLeft, rootRight := root(parent, left), root(parent, right)
	if rootLeft == rootRight {
		return
	}

	// The smaller key is the root, so a label does not depend on the order of the joins.
	if entities.CompareKeys(rootRight, rootLeft) < 0 {
		rootLeft, rootRight = rootRight, rootLeft
	}

	parent[rootRight] = rootLeft
	if _, known := parent[rootLeft]; !known {
		parent[rootLeft] = rootLeft
	}
}

func root(parent map[entities.DeclKey]entities.DeclKey, key entities.DeclKey) entities.DeclKey {
	for {
		up, known := parent[key]
		if !known || up == key {
			return key
		}

		key = up
	}
}

func labels(parent map[entities.DeclKey]entities.DeclKey) map[entities.DeclKey]string {
	out := make(map[entities.DeclKey]string, len(parent))

	for key := range parent {
		out[key] = "go-router:" + root(parent, key).String()
	}

	return out
}

func (view View) withoutSharedCallers(cfg Config, decls []entities.Decl) View {
	// Mounts sharing an atom or a statement group rise together; private providers stay later.
	parent := view.groupJoins(cfg)
	firstOf := map[string]entities.DeclKey{}

	for _, decl := range decls {
		if decl.Atom == "" {
			continue
		}

		if first, known := firstOf[decl.Atom]; known {
			join(parent, first, decl.Key)

			continue
		}

		firstOf[decl.Atom] = decl.Key
	}

	hosted := map[entities.DeclKey]int{}
	for _, mount := range view.Mounts {
		hosted[root(parent, mount.Caller)]++
	}

	alone := make([]Mount, 0, len(view.Mounts))

	for _, mount := range view.Mounts {
		if hosted[root(parent, mount.Caller)] == 1 {
			alone = append(alone, mount)
		}
	}

	view.Mounts = alone

	return view
}
