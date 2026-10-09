package web

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"
)

// scopedParts splits "@scope/name/sub" into the package name's two parts and
// the subpath.
const scopedParts = 3

type readFunc func(ctx context.Context, path string) ([]byte, error)

// workspace resolves a specifier written in one file of the tree to another
// file of it, the way Vite and svelte-check would: tsconfig "paths" first,
// then relative paths, "#" imports, and the workspace packages by name.
type workspace struct {
	files     map[string]bool
	read      readFunc
	once      sync.Once
	loadErr   error
	packages  map[string]manifest
	byName    map[string]string
	tsconfigs map[string]tsconfig
}

func newWorkspace(files []string, read readFunc) *workspace {
	set := make(map[string]bool, len(files))
	for _, file := range files {
		set[file] = true
	}

	return &workspace{files: set, read: read}
}

// load reads every package.json and tsconfig.json once, under the first
// caller's context; a file that is not valid JSON, as a tsconfig with comments
// may be, resolves nothing.
func (w *workspace) load(ctx context.Context) error {
	w.once.Do(func() { w.loadErr = w.scan(ctx) })

	return w.loadErr
}

func (w *workspace) scan(ctx context.Context) error {
	w.packages, w.byName, w.tsconfigs = map[string]manifest{}, map[string]string{}, map[string]tsconfig{}

	for file := range w.files {
		base := path.Base(file)
		if base != packageJSON && base != tsconfigJSON {
			continue
		}

		source, err := w.read(ctx, file)
		if err != nil {
			return fmt.Errorf("read %s: %w", file, err)
		}

		w.index(file, source)
	}

	return nil
}

func (w *workspace) index(file string, source []byte) {
	dir := path.Dir(file)

	if path.Base(file) == tsconfigJSON {
		var parsed tsconfig
		if json.Unmarshal(source, &parsed) == nil {
			w.tsconfigs[dir] = parsed
		}

		return
	}

	var parsed manifest
	if json.Unmarshal(source, &parsed) != nil {
		return
	}

	w.packages[dir] = parsed

	if parsed.Name != "" {
		w.byName[parsed.Name] = dir
	}
}

// resolve maps spec, written in from, to a file of the tree, or to "" for a
// module outside it: a dependency, a Node built-in, a SvelteKit alias.
func (w *workspace) resolve(ctx context.Context, from, spec string) (string, error) {
	if err := w.load(ctx); err != nil {
		return "", err
	}

	dir := path.Dir(from)

	if root, config, found := nearest(w.tsconfigs, dir); found {
		if mapped, ok := match(config.CompilerOptions.Paths, spec, fillPaths); ok {
			if file := w.probe(path.Join(root, mapped)); file != "" {
				return file, nil
			}
		}
	}

	switch {
	case strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../"):
		return w.probe(path.Join(dir, spec)), nil
	case strings.HasPrefix(spec, "#"):
		root, pkg, found := nearest(w.packages, dir)
		if mapped, ok := match(pkg.Imports, spec, fillRaw); found && ok {
			return w.probe(path.Join(root, mapped)), nil
		}

		return "", nil
	default:
		return w.bare(spec), nil
	}
}

func (w *workspace) bare(spec string) string {
	name, sub := spec, ""
	if parts := strings.SplitN(
		spec,
		"/",
		scopedParts,
	); strings.HasPrefix(spec, "@") &&
		len(parts) == scopedParts {
		name, sub = parts[0]+"/"+parts[1], parts[2]
	} else if before, after, cut := strings.Cut(
		spec,
		"/",
	); cut &&
		!strings.HasPrefix(spec, "@") {
		name, sub = before, after
	}

	root, found := w.byName[name]
	if !found {
		return ""
	}

	key := "."
	if sub != "" {
		key = "./" + sub
	}

	if exports := w.packages[root].exportsMap(); exports != nil {
		mapped, ok := match(exports, key, fillRaw)
		if !ok {
			return ""
		}

		return w.probe(path.Join(root, mapped))
	}

	return w.probe(path.Join(root, sub))
}

// probe finds the file a resolved path names, trying what TypeScript and Vite
// add: a ".ts" written as ".js", a missing extension, a directory's index.
func (w *workspace) probe(target string) string {
	candidates := []string{
		target, target + tsExt, target + jsExt,
		strings.TrimSuffix(target, jsExt) + tsExt,
		target + "/index.ts", target + "/index.js",
	}

	for _, candidate := range candidates {
		if w.files[candidate] {
			return candidate
		}
	}

	return ""
}
