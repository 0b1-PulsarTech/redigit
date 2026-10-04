package config

import (
	"context"
	"path"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// Anchors pull the workspace down to the modules it lists. A go.work is born
// at the position its file was first touched, long after most modules exist,
// and a module that exists while the go.work does not is built without it:
// the final go.mod files lean on the workspace for the siblings they replace,
// so every tree before it stops at a missing require. Each use line is
// anchored to the module it names and the go.work header to the earliest of
// them, which makes the workspace grow module by module from the first one.
func (workspaceRule) Anchors(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	headers := moduleHeaders(decls)
	works := workHeaders(decls)
	sources := map[string][]byte{}

	var anchors []entities.Anchor

	for _, decl := range decls {
		work, found := works[decl.Key.File]
		if !found || !isUse(decl.Key) {
			continue
		}

		target, names := localTarget(decl, snap, sources)
		provider, held := headers[target]

		if !names || !held {
			continue
		}

		anchors = append(anchors,
			entities.Anchor{Decl: decl.Key, To: provider, Reason: entities.ReasonWorkspace},
			entities.Anchor{Decl: work, To: provider, Reason: entities.ReasonWorkspace},
		)
	}

	return anchors, nil
}

// workHeaders maps each go.work file to its header piece, the one carrying the
// "go" directive: the only piece not named after a "use" line.
func workHeaders(decls []entities.Decl) map[string]entities.DeclKey {
	out := map[string]entities.DeclKey{}

	for _, decl := range decls {
		if path.Base(decl.Key.File) == goWorkFile && !isUse(decl.Key) {
			out[decl.Key.File] = decl.Key
		}
	}

	return out
}

func isUse(key entities.DeclKey) bool {
	return path.Base(key.File) == goWorkFile && strings.HasPrefix(key.Name, usePrefix)
}
