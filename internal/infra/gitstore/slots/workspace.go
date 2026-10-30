package slots

import (
	"context"
	"fmt"
	"path"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/wrapped-owls/redigit/internal/domain/usecases/verifying"
)

func (modules *Modules) readBlobs(ctx context.Context, oids []string) ([][]byte, error) {
	out, err := runGit(ctx, gitCall{
		gitDir: modules.gitDir, args: []string{"cat-file", "--batch"},
		stdin: []byte(strings.Join(oids, "\n") + "\n"),
	})
	if err != nil {
		return nil, fmt.Errorf("read blobs: %w", err)
	}

	return parseBlobs(out, oids)
}

type workspace struct {
	text     string
	dir      string
	targets  []string
	isOpaque bool
}

func parseWork(dir, text string) workspace {
	found := workspace{text: dir + "\x00" + text, dir: dir}

	file, err := modfile.ParseWork("go.work", []byte(text), nil)
	if err != nil {
		found.isOpaque = true

		return found
	}

	for _, use := range file.Use {
		found.targets = append(found.targets, use.Path)
	}

	for _, replace := range file.Replace {
		if modfile.IsDirectoryPath(replace.New.Path) {
			found.targets = append(found.targets, replace.New.Path)
		}
	}

	return found
}

func (found workspace) vendor(trees map[string]string) []verifying.Local {
	dir := path.Join(found.dir, "vendor")

	// go work vendor builds from this tree, which no module subtree contains.
	subtree, isTree := trees[dir]
	if !isTree {
		return nil
	}

	return []verifying.Local{{Dir: dir, Subtree: subtree}}
}

func (modules *Modules) workspaces(
	ctx context.Context,
	works []markerFile,
) (map[string]workspace, error) {
	if len(works) == 0 {
		return map[string]workspace{}, nil
	}

	oids := make([]string, 0, len(works))
	for _, file := range works {
		oids = append(oids, file.blob)
	}

	contents, err := modules.readBlobs(ctx, oids)
	if err != nil {
		return nil, err
	}

	texts, sums := map[string]string{}, map[string]string{}

	for idx, file := range works {
		if file.name == goWorkName {
			texts[file.dir] = string(contents[idx])
		} else {
			sums[file.dir] = string(contents[idx])
		}
	}

	found := make(map[string]workspace, len(texts))

	for dir, text := range texts {
		work := parseWork(dir, text)
		work.text += "\x00" + sums[dir]
		found[dir] = work
	}

	return found, nil
}

func nearestWorkspace(dir string, found map[string]workspace) (workspace, bool) {
	for current := dir; ; current = path.Dir(current) {
		if work, hasWork := found[current]; hasWork {
			return work, true
		}

		if current == "." {
			return workspace{}, false
		}
	}
}
