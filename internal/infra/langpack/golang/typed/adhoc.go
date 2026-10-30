package typed

import (
	"context"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

type adhocLoad struct {
	dir      string
	patterns []string
}

func (source *Source) Adhoc(
	ctx context.Context,
	snap entities.Snapshot,
) ([][]*packages.Package, error) {
	groups, err := source.adhoc.Do(
		ctx,
		func(loadCtx context.Context) ([][]*packages.Package, error) {
			found, checkErr := source.checkAdhoc(loadCtx, snap)
			if checkErr != nil {
				return nil, fmt.Errorf("ad hoc pass: %w", checkErr)
			}

			return found, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("typed ad hoc: %w", err)
	}

	return groups, nil
}

func (source *Source) checkAdhoc(
	ctx context.Context,
	snap entities.Snapshot,
) ([][]*packages.Package, error) {
	modules, err := modulesOf(snap)
	if err != nil {
		return nil, err
	}

	var groups [][]*packages.Package

	// The module pattern skips these directories, yet gopls answers for them by loading each as
	// a package of its own; each module loads from its own directory, the one that owns them.
	for _, load := range adhocLoads(snap, modules) {
		dir := filepath.Join(snap.Root, load.dir)

		pkgs, loadErr := typeCheck(ctx, dir, loadPass{tags: source.tags, patterns: load.patterns})
		if loadErr != nil {
			return nil, loadErr
		}

		groups = append(groups, pkgs)
	}

	return groups, nil
}

func adhocLoads(snap entities.Snapshot, modules []module) []adhocLoad {
	found := map[string]map[string]bool{}

	for _, file := range snap.Files {
		owner, inside := ownerOf(modules, file)
		if !inside || !strings.HasSuffix(file, ".go") {
			continue
		}

		rel := owner.relative(path.Dir(file))
		if !isAdhoc(rel) {
			continue
		}

		if found[owner.dir] == nil {
			found[owner.dir] = map[string]bool{}
		}

		found[owner.dir]["./"+rel] = true
	}

	loads := make([]adhocLoad, 0, len(found))

	for _, dir := range slices.Sorted(maps.Keys(found)) {
		loads = append(loads, adhocLoad{dir: dir, patterns: slices.Sorted(maps.Keys(found[dir]))})
	}

	return loads
}

func isAdhoc(rel string) bool {
	segments := strings.Split(rel, "/")
	if slices.Contains(segments, "vendor") {
		return false
	}

	return slices.ContainsFunc(segments, func(segment string) bool {
		return segment == "testdata" || strings.HasPrefix(segment, "_") ||
			strings.HasPrefix(segment, ".")
	})
}

func (found module) relative(dir string) string {
	if dir == found.dir || dir == "." {
		return ""
	}

	if found.dir == "." {
		return dir
	}

	return strings.TrimPrefix(dir, found.dir+"/")
}
