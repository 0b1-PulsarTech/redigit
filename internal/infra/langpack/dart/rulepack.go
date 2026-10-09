package dart

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// rule is one independently testable edge source, the shape the Go and config
// packs register theirs in, except that it reads the workspace the pack built
// once for every rule instead of the snapshot.
type rule interface {
	Name() string
	Edges(ctx context.Context, space *workspace, decls []entities.Decl) ([]entities.Edge, error)
	Anchors(ctx context.Context, space *workspace, decls []entities.Decl) ([]entities.Anchor, error)
}

const dartExt = ".dart"

var rules = []rule{
	referenceRule{},
	exportRule{},
	hostsRule{},
	testAnchorRule{},
}

// RulePack gives Dart the edges a language server would: no server answers
// for these files, so the references are this pack's own scope-aware scan.
type RulePack struct{}

var _ indexing.RulePack = RulePack{}

func NewRulePack() RulePack { return RulePack{} }

func (RulePack) Name() string { return "dart" }

func (RulePack) Edges(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	space := newWorkspace(snap.Files, rootReader(snap.Root))
	known := make(map[entities.DeclKey]bool, len(decls))
	seen := map[entities.Edge]bool{}

	for _, decl := range decls {
		known[decl.Key] = true
	}

	var out []entities.Edge

	for _, each := range rules {
		found, err := each.Edges(ctx, space, decls)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", each.Name(), err)
		}

		for _, edge := range found {
			isKnown := known[edge.Dependent] && known[edge.Provider]
			if isKnown && !seen[edge] && edge.Dependent != edge.Provider {
				seen[edge] = true

				out = append(out, edge)
			}
		}
	}

	return out, nil
}

func (RulePack) Anchors(
	ctx context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Anchor, error) {
	space := newWorkspace(snap.Files, rootReader(snap.Root))

	var out []entities.Anchor

	for _, each := range rules {
		found, err := each.Anchors(ctx, space, decls)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", each.Name(), err)
		}

		out = append(out, found...)
	}

	return out, nil
}

// rootReader reads the snapshot's checked-out tree, as the Go pack's rules do.
func rootReader(root string) readFunc {
	return func(_ context.Context, rel string) ([]byte, error) {
		// The path comes from the snapshot's own file list, not from input.
		source, err := os.ReadFile(filepath.Join(root, rel)) //nolint:gosec
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}

		return source, nil
	}
}

// dartFiles lists, in a stable order, the Dart files decls come from.
func dartFiles(decls []entities.Decl) []string {
	set := map[string]bool{}

	for _, decl := range decls {
		if path.Ext(decl.Key.File) == dartExt {
			set[decl.Key.File] = true
		}
	}

	return slices.Sorted(maps.Keys(set))
}

// firstDecl is the declaration standing for a file's existence: the first
// one it holds, or the whole-file one when it declares nothing.
func firstDecl(f parsedFile) entities.DeclKey {
	if own := f.decls(); len(own) > 0 {
		return own[0].key
	}

	return entities.DeclKey{File: f.path, Kind: entities.KindOther, Name: fileDeclName}
}
