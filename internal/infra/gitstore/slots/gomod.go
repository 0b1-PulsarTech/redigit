package slots

import (
	"path"

	"golang.org/x/mod/modfile"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

type goMod struct {
	path     string
	version  string
	requires []string
	replaces []string
	isOpaque bool
}

func parseGoMod(content []byte) (goMod, bool) {
	file, err := modfile.ParseLax("go.mod", content, nil)
	if err != nil || file.Module == nil {
		return goMod{}, false
	}

	parsed := goMod{path: file.Module.Mod.Path}
	if file.Go != nil {
		parsed.version = file.Go.Version
	}

	for _, require := range file.Require {
		parsed.requires = append(parsed.requires, require.Mod.Path)
	}

	// Lax parsing drops every replace, so only a strict parse can name the local targets.
	strict, err := modfile.Parse("go.mod", content, nil)
	if err != nil {
		parsed.isOpaque = true

		return parsed, true
	}

	for _, replace := range strict.Replace {
		if modfile.IsDirectoryPath(replace.New.Path) {
			parsed.replaces = append(parsed.replaces, replace.New.Path)
		}
	}

	return parsed, true
}

func localDirs(base string, targets []string, trees map[string]string) ([]verifying.Local, bool) {
	locals := make([]verifying.Local, 0, len(targets))

	for _, target := range targets {
		resolved := path.Join(base, target)

		// A symlink, a path out of the repository or a missing directory is absent from the trees.
		subtree, isTree := trees[resolved]
		if path.IsAbs(target) || !isTree {
			return nil, false
		}

		locals = append(locals, verifying.Local{Dir: resolved, Subtree: subtree})
	}

	return locals, true
}
