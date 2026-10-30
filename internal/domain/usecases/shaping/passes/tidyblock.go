package passes

import (
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/shaping"
)

type (
	declInfo struct {
		key       entities.DeclKey
		dir       string
		lines     int
		test      bool
		generated bool
		code      bool
	}

	block struct {
		dir   string
		keys  []entities.DeclKey
		tests int
		prod  int
		files []string
	}
)

func infosOf(graph entities.Graph, keys []entities.DeclKey) []declInfo {
	out := make([]declInfo, 0, len(keys))

	for _, key := range keys {
		decl, _ := graph.Decl(key)
		// Web-pack files are KindOther <file> decls, so a ts/svelte-only commit has no home
		// and the foreign rule leaves it alone.
		out = append(out, declInfo{
			key:       key,
			dir:       path.Dir(key.File),
			lines:     decl.Lines,
			test:      entities.IsTestSide(decl) || strings.HasSuffix(key.File, "_test.go"),
			generated: decl.Generated,
			code:      key.Kind != entities.KindOther,
		})
	}

	return out
}

func primaryDir(infos []declInfo) (string, bool) {
	// No hand-written production leaves no package to be foreign to; manifests and docs never vote.
	lines := map[string]int{}

	for _, info := range infos {
		if !info.test && !info.generated && info.code {
			lines[info.dir] += max(info.lines, 1)
		}
	}

	best, found := "", false

	for dir, count := range lines {
		if !found || count > lines[best] || (count == lines[best] && dir < best) {
			best, found = dir, true
		}
	}

	return best, found
}

func blocksByDir(infos []declInfo) []block {
	byDir := map[string]*block{}

	var dirs []string

	for _, info := range infos {
		held, seen := byDir[info.dir]
		if !seen {
			held = &block{dir: info.dir}
			byDir[info.dir] = held

			dirs = append(dirs, info.dir)
		}

		held.keys = append(held.keys, info.key)

		if info.test {
			held.tests += info.lines
		} else {
			held.prod += info.lines
		}

		if file := path.Base(info.key.File); !slices.Contains(held.files, file) {
			held.files = append(held.files, file)
		}
	}

	slices.Sort(dirs)

	out := make([]block, 0, len(dirs))
	for _, dir := range dirs {
		out = append(out, *byDir[dir])
	}

	return out
}

func (held block) commit() entities.PlannedCommit {
	files := slices.Clone(held.files)
	slices.Sort(files)

	const shown = 3

	names := strings.Join(files[:min(len(files), shown)], ", ")

	return entities.PlannedCommit{
		Subject: "test(" + path.Base(held.dir) + "): " + names,
		Shape:   entities.ShapeFeature,
		Reason:  "tests follow the code they test",
	}
}

func latestProvider(draft *shaping.Draft, keys []entities.DeclKey, limit int) (int, bool) {
	// The last slot at or before rank limit holding something the keys need from outside.
	inside := make(map[entities.DeclKey]bool, len(keys))
	for _, key := range keys {
		inside[key] = true
	}

	best, found := -1, false

	for _, key := range keys {
		for _, edge := range draft.Graph().Needs(key) {
			if inside[edge.Provider] {
				continue
			}

			id, placed := draft.Of(edge.Provider)
			if !placed {
				continue
			}

			if rank, live := draft.Rank(id); live && rank <= limit && (!found || rank > best) {
				best, found = rank, true
			}
		}
	}

	if !found {
		return 0, false
	}

	return draft.Slots()[best].ID, true
}
