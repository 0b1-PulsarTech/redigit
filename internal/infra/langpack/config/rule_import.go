package config

import (
	"context"
	"go/parser"
	"go/token"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// importRule makes a Go declaration wait for the go.mod "require" providing a
// package its file imports. No reference names the module that ships a package,
// so the first importer landed hundreds of commits before the require it needs
// and every tree between failed with "no required module provides package".
// The edge is per file: a render prunes imports and a require is a leaf.
type importRule struct{}

func (importRule) Name() string { return "import" }

func (importRule) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	headers := moduleHeaders(decls)
	if len(headers) == 0 {
		return nil, nil
	}

	roots := slices.Sorted(maps.Keys(headers))
	required := requiredModules(decls)
	byFile := map[string][]entities.DeclKey{}

	for _, decl := range decls {
		if strings.HasSuffix(decl.Key.File, ".go") {
			byFile[decl.Key.File] = append(byFile[decl.Key.File], decl.Key)
		}
	}

	edges := indirectEdges(snap, decls)

	for _, file := range slices.Sorted(maps.Keys(byFile)) {
		root, found := nearestModule(roots, file)
		if !found {
			continue
		}

		for _, provider := range providedRequires(snap, file, required[root]) {
			for _, dependent := range byFile[file] {
				edges = append(edges, entities.Edge{
					Dependent: dependent,
					Provider:  provider,
					Reason:    entities.ReasonImport,
				})
			}
		}
	}

	return edges, nil
}

func (importRule) Anchors(
	_ context.Context,
	_ entities.Snapshot,
	_ []entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

// requiredModules maps each module directory to its require pieces, keyed by
// the module path they require.
func requiredModules(decls []entities.Decl) map[string]map[string]entities.DeclKey {
	out := map[string]map[string]entities.DeclKey{}

	for _, decl := range decls {
		module, isRequire := strings.CutPrefix(decl.Key.Name, requirePrefix)
		if !isRequire || path.Base(decl.Key.File) != goModFile {
			continue
		}

		dir := path.Dir(decl.Key.File)
		if out[dir] == nil {
			out[dir] = map[string]entities.DeclKey{}
		}

		out[dir][module] = decl.Key
	}

	return out
}

// providedRequires lists the require pieces of root's go.mod that provide a
// package file imports. A file that cannot be read or parsed yields none: one
// missing edge orders one declaration by history, while failing loses every
// edge.
func providedRequires(
	snap entities.Snapshot,
	file string,
	required map[string]entities.DeclKey,
) []entities.DeclKey {
	source, err := readSource(snap, file)
	if err != nil {
		return nil
	}

	parsed, err := parser.ParseFile(token.NewFileSet(), file, source, parser.ImportsOnly)
	if err != nil {
		return nil
	}

	seen := map[string]bool{}

	var out []entities.DeclKey

	for _, spec := range parsed.Imports {
		imported, unquoteErr := strconv.Unquote(spec.Path.Value)
		if unquoteErr != nil {
			continue
		}

		module, found := owningModule(imported, required)
		if !found || seen[module] {
			continue
		}

		seen[module] = true

		out = append(out, required[module])
	}

	return out
}

// owningModule is the longest required module path that is imported, or
// prefixes it at a path boundary. Longest, not first: "example.com/a" and
// "example.com/a/b" can both be required, and the import belongs to the
// deeper one.
func owningModule(imported string, required map[string]entities.DeclKey) (string, bool) {
	best := ""

	for module := range required {
		if imported != module && !strings.HasPrefix(imported, module+"/") {
			continue
		}

		if len(module) > len(best) {
			best = module
		}
	}

	return best, best != ""
}

// indirectEdges makes every direct require wait for the "// indirect" requires
// of its own go.mod. A module graph is pruned: the go command wants every
// module providing a package the build transitively imports listed in the main
// go.mod, and answers "updates to go.mod needed" until it is. Which direct
// require needs which indirect one is the dependency's own go.mod, which a
// tree under construction does not hold, so each direct require pulls them all.
func indirectEdges(snap entities.Snapshot, decls []entities.Decl) []entities.Edge {
	direct := map[string][]entities.DeclKey{}
	indirect := map[string][]entities.DeclKey{}
	sources := map[string][]byte{}

	for _, decl := range decls {
		if !strings.HasPrefix(decl.Key.Name, requirePrefix) ||
			path.Base(decl.Key.File) != goModFile {
			continue
		}

		dir := path.Dir(decl.Key.File)

		if isIndirect(snap, decl, sources) {
			indirect[dir] = append(indirect[dir], decl.Key)

			continue
		}

		direct[dir] = append(direct[dir], decl.Key)
	}

	var edges []entities.Edge

	for _, dir := range slices.Sorted(maps.Keys(direct)) {
		for _, dependent := range direct[dir] {
			for _, provider := range indirect[dir] {
				edges = append(edges, entities.Edge{
					Dependent: dependent,
					Provider:  provider,
					Reason:    entities.ReasonImport,
				})
			}
		}
	}

	return edges
}

// isIndirect reports whether a require piece carries the "// indirect" comment.
// An unreadable go.mod makes it direct: that adds no edge, so the cost of the
// guess is one line ordered by history.
func isIndirect(snap entities.Snapshot, decl entities.Decl, sources map[string][]byte) bool {
	source, cached := sources[decl.Key.File]
	if !cached {
		source, _ = readSource(snap, decl.Key.File)
		sources[decl.Key.File] = source
	}

	_, comment, found := strings.Cut(sliceSpan(source, decl.Span), "//")

	return found && strings.HasPrefix(strings.TrimSpace(comment), "indirect")
}
