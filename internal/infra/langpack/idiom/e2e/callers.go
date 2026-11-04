package e2e

import (
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/facts"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/idiom"
)

type (
	chain struct {
		prefix string
		sites  []entities.DeclKey
	}

	topology struct {
		callers map[string][]facts.Call
		mount   []string
	}
)

func funcName(site entities.DeclKey) string {
	name := site.Name
	if site.Kind == entities.KindStmt {
		name = name[:strings.LastIndex(name, "#")]
	}

	return site.File + "\x00" + name
}

func newTopology(decls []entities.Decl, known idiom.Facts, mount []string) topology {
	top := topology{callers: map[string][]facts.Call{}, mount: mount}

	for _, decl := range decls {
		switch decl.Key.Kind {
		case entities.KindFunc, entities.KindMethod, entities.KindStmt:
		default:
			continue
		}

		if entities.IsTestSide(decl) {
			continue
		}

		for _, call := range known.Calls(decl.Key) {
			if !matchesAny(call, mount) {
				continue
			}

			targets := call.FuncArgs
			if !call.CalleeKey.IsZero() {
				targets = append([]entities.DeclKey{call.CalleeKey}, targets...)
			}

			for _, target := range targets {
				top.callers[funcName(target)] = append(top.callers[funcName(target)], call)
			}
		}
	}

	return top
}

func (top topology) chains(site entities.DeclKey) []chain {
	found := top.climb(funcName(site), nil)
	if len(found) == 0 {
		return []chain{{}}
	}

	return found
}

func (top topology) climb(fn string, path []string) []chain {
	var out []chain

	for _, call := range top.callers[fn] {
		parent := funcName(call.Site)
		if parent == fn || slices.Contains(path, parent) {
			continue
		}

		prefix := top.prefixOf(call)

		outer := top.climb(parent, append(slices.Clone(path), fn))
		if len(outer) == 0 {
			out = append(out, chain{prefix: prefix, sites: []entities.DeclKey{call.Site}})

			continue
		}

		for _, up := range outer {
			out = append(out, chain{
				prefix: strings.TrimSuffix(up.prefix, "/") + prefix,
				sites:  append([]entities.DeclKey{call.Site}, up.sites...),
			})
		}
	}

	return out
}

func (top topology) prefixOf(call facts.Call) string {
	prefix := pathLiteral(call.Literals)

	if within := call.Within; within != nil && matchesAny(*within, top.mount) {
		prefix = strings.TrimSuffix(pathLiteral(within.Literals), "/") + prefix
	}

	return prefix
}

func pathLiteral(literals []string) string {
	at := slices.IndexFunc(literals, func(literal string) bool {
		return strings.HasPrefix(literal, "/")
	})
	if at < 0 {
		return ""
	}

	return literals[at]
}
