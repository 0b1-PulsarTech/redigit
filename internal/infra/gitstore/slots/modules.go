package slots

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

var _ verifying.Modules = (*Modules)(nil)

type Modules struct {
	gitDir  string
	markers map[string]string
	mutex   sync.Mutex
	parsed  map[string]goMod
}

func NewModules(gitDir string, markers map[string]string) *Modules {
	return &Modules{gitDir: gitDir, markers: markers, parsed: map[string]goMod{}}
}

func (modules *Modules) List(
	ctx context.Context,
	commit verifying.Commit,
) (verifying.Listing, error) {
	out, err := runGit(ctx, gitCall{
		gitDir: modules.gitDir, args: []string{"ls-tree", "-r", "-t", "-z", commit.Tree},
	})
	if err != nil {
		return verifying.Listing{}, fmt.Errorf("list tree: %w", err)
	}

	listing, err := parseTree(out, modules.markers)
	if err != nil {
		return verifying.Listing{}, err
	}

	files, trees := listing.files, listing.trees

	mods, err := modules.readGoMods(ctx, files)
	if err != nil {
		return verifying.Listing{}, err
	}

	works, err := modules.workspaces(ctx, listing.works)
	if err != nil {
		return verifying.Listing{}, err
	}

	found := make([]verifying.Module, 0, len(files))

	for _, file := range files {
		module := describe(file, commit, trees, mods, works)
		if listing.hasLink && module.Language == goLanguage {
			module.Subtree, module.Locals = commit.Tree, nil
		}

		found = append(found, module)
	}

	slices.SortFunc(
		found,
		func(left, right verifying.Module) int { return strings.Compare(left.Dir, right.Dir) },
	)

	return verifying.Listing{Modules: found, Strays: strayGoFiles(listing.goSrc, found)}, nil
}

func describe(
	file markerFile,
	commit verifying.Commit,
	trees map[string]string,
	mods map[string]goMod,
	works map[string]workspace,
) verifying.Module {
	module := verifying.Module{
		Dir:      file.dir,
		Path:     file.dir,
		Language: file.language,
		Subtree:  trees[file.dir],
	}
	if file.dir == "." {
		module.Subtree = commit.Tree
	}

	if file.language != goLanguage {
		return module
	}

	parsed, isParsed := mods[file.blob]
	if isParsed && parsed.path != "" {
		module.Path, module.GoVersion, module.Requires = parsed.path, parsed.version, parsed.requires
	}

	work, hasWork := nearestWorkspace(file.dir, works)
	if hasWork {
		module.Workspace = work.text
	}

	locals, isResolved := localDirs(file.dir, parsed.replaces, trees)

	if hasWork {
		shared, isSharedResolved := localDirs(work.dir, work.targets, trees)
		locals = append(locals, shared...)
		locals = append(locals, work.vendor(trees)...)
		isResolved = isResolved && isSharedResolved && !work.isOpaque
	}

	// Anything the tree cannot vouch for makes the whole commit the safest key.
	if !isResolved || parsed.isOpaque {
		module.Subtree = commit.Tree

		return module
	}

	module.Locals = locals

	return module
}

func strayGoFiles(goSrc []string, found []verifying.Module) []string {
	var strays []string

	for _, source := range goSrc {
		if !slices.ContainsFunc(found, func(module verifying.Module) bool {
			return module.Language == goLanguage && underDir(source, module.Dir)
		}) {
			strays = append(strays, source)
		}
	}

	return strays
}

func underDir(file, dir string) bool {
	return dir == "." || strings.HasPrefix(file, dir+"/")
}

func (modules *Modules) readGoMods(
	ctx context.Context,
	files []markerFile,
) (map[string]goMod, error) {
	modules.mutex.Lock()
	defer modules.mutex.Unlock()

	var missing []string

	for _, file := range files {
		if _, isKnown := modules.parsed[file.blob]; file.language == goLanguage && !isKnown &&
			!slices.Contains(missing, file.blob) {
			missing = append(missing, file.blob)
		}
	}

	if len(missing) > 0 {
		contents, err := modules.readBlobs(ctx, missing)
		if err != nil {
			return nil, err
		}

		for idx, oid := range missing {
			parsed, _ := parseGoMod(contents[idx])
			modules.parsed[oid] = parsed
		}
	}

	return maps.Clone(modules.parsed), nil
}
