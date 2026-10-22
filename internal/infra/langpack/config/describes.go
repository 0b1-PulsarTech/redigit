package config

import (
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// proseExtensions are the formats a person reads rather than a tool.
var proseExtensions = []string{".md", ".mdx", ".rst", ".adoc", ".txt", ".mmd"}

// isProse reports a document: a prose format, or anything under a docs
// directory.
func isProse(file string) bool {
	if slices.Contains(proseExtensions, path.Ext(file)) {
		return true
	}

	segments := strings.Split(path.Dir(file), "/")

	return slices.Contains(segments, "docs") || slices.Contains(segments, "doc")
}

// codeIndex answers which code a document is about, by the paths and module
// paths its text names and by the directory it sits in.
type codeIndex struct {
	snap    entities.Snapshot
	byDir   map[string][]entities.DeclKey
	direct  map[string]bool
	modules map[string]string
	texts   map[string]string
}

// newCodeIndex lists the declarations of every directory above them. Tests are
// left out: they wait for what they exercise and are never the first of a
// package.
func newCodeIndex(
	snap entities.Snapshot,
	headers map[string]entities.DeclKey,
	decls []entities.Decl,
) codeIndex {
	index := codeIndex{
		snap:    snap,
		byDir:   map[string][]entities.DeclKey{},
		direct:  map[string]bool{},
		modules: map[string]string{},
		texts:   map[string]string{},
	}

	for root, header := range headers {
		index.modules[header.Name] = root
		index.direct[root] = true
	}

	for _, decl := range decls {
		if decl.Key.Kind == entities.KindOther || decl.IsTest {
			continue
		}

		index.direct[path.Dir(decl.Key.File)] = true

		for dir := path.Dir(decl.Key.File); dir != "."; dir = path.Dir(dir) {
			index.byDir[dir] = append(index.byDir[dir], decl.Key)
		}
	}

	return index
}

// besideCode is the code a document sits beside: what lives in its own
// directory, which is what a package README is about.
func (c codeIndex) besideCode(file string) []entities.DeclKey {
	found := map[entities.DeclKey]bool{}

	for _, key := range c.byDir[path.Dir(file)] {
		found[key] = true
	}

	return sortedDecls(found)
}

// namedCode is the code a document says it is about: what lives in a directory
// its text names with a path, and in a module whose path its text names. A bare
// word names nothing: "database" or "tools" are words before they are
// directories, so a directory counts only as a path.
func (c codeIndex) namedCode(file string) []entities.DeclKey {
	text := c.read(file)
	found := map[entities.DeclKey]bool{}

	for dir, keys := range c.byDir {
		if !c.direct[dir] || !strings.Contains(dir, "/") || !names(text, dir, c.byDir) {
			continue
		}

		for _, key := range keys {
			found[key] = true
		}
	}

	for modulePath, root := range c.modules {
		if names(text, modulePath, nil) {
			for _, key := range c.byDir[root] {
				found[key] = true
			}
		}
	}

	return sortedDecls(found)
}

func (c codeIndex) read(file string) string {
	if text, cached := c.texts[file]; cached {
		return text
	}

	source, err := readSource(c.snap, file)
	if err != nil {
		source = nil
	}

	c.texts[file] = string(source)

	return c.texts[file]
}

func sortedDecls(set map[entities.DeclKey]bool) []entities.DeclKey {
	out := make([]entities.DeclKey, 0, len(set))
	for key := range set {
		out = append(out, key)
	}

	slices.SortFunc(out, func(a, b entities.DeclKey) int {
		return strings.Compare(a.String(), b.String())
	})

	return out
}
