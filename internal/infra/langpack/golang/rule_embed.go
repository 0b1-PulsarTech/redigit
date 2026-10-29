package golang

import (
	"bytes"
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

// embedRule makes a //go:embed declaration wait for a file its pattern matches.
// The pattern is a string no reference query reads, and go refuses one that
// matches nothing ("pattern *.sql: no matching files found"), so an embedding
// file born before its data broke every tree in between.
//
// One matching file is enough to build, so the first by name is the provider:
// the rest of the data may arrive later without breaking anything.
type embedRule struct{}

func (embedRule) Name() string { return "embed" }

const (
	embedDirective = "//go:embed"
	allPrefix      = "all:"
)

func (embedRule) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	byFile := GroupByFile(decls)

	var edges []entities.Edge

	for _, file := range slices.Sorted(maps.Keys(byFile)) {
		if !strings.HasSuffix(file, ".go") {
			continue
		}

		source, err := ReadSource(snap, file)
		if err != nil || !bytes.Contains(source, []byte(embedDirective)) {
			continue
		}

		for _, use := range embedUses(file, source) {
			for _, pattern := range use.patterns {
				provider, found := firstMatch(snap.Files, byFile, path.Dir(file), pattern)
				if !found {
					continue
				}

				for _, dependent := range embedders(byFile[file], use.offset) {
					edges = append(edges, entities.Edge{
						Dependent: dependent.Key,
						Provider:  provider,
						Reason:    entities.ReasonEmbed,
					})
				}
			}
		}
	}

	return edges, nil
}

func (embedRule) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}

// embedUse is one //go:embed line: where it sits and the patterns it names.
type embedUse struct {
	offset   int
	patterns []string
}

// embedUses reads the directives out of file's comments. A file that does not
// parse names none: one missing edge orders one declaration by history alone.
func embedUses(file string, source []byte) []embedUse {
	fset := token.NewFileSet()

	parsed, err := parser.ParseFile(fset, file, source, parser.ParseComments)
	if err != nil {
		return nil
	}

	var uses []embedUse

	for _, group := range parsed.Comments {
		for _, comment := range group.List {
			args, isEmbed := strings.CutPrefix(comment.Text, embedDirective+" ")
			if !isEmbed {
				continue
			}

			uses = append(uses, embedUse{
				offset:   fset.Position(comment.Pos()).Offset,
				patterns: splitPatterns(args),
			})
		}
	}

	return uses
}

// splitPatterns separates a directive's arguments the way go does: a pattern
// is a bare word, or a quoted string when it holds a space.
func splitPatterns(args string) []string {
	var out []string

	for rest := strings.TrimSpace(args); rest != ""; rest = strings.TrimSpace(rest) {
		if quoted, err := strconv.QuotedPrefix(rest); err == nil {
			if pattern, err := strconv.Unquote(quoted); err == nil {
				out = append(out, pattern)
			}

			rest = rest[len(quoted):]

			continue
		}

		word, tail, _ := strings.Cut(rest, " ")
		out = append(out, word)
		rest = tail
	}

	return out
}

// embedders are the declarations the directive belongs to: the one whose span
// holds it. A directive no declaration holds belongs to the whole file, which
// is the safe side to err on.
func embedders(fileDecls []entities.Decl, offset int) []entities.Decl {
	var holding []entities.Decl

	for _, decl := range fileDecls {
		if decl.Span.Start <= offset && offset < decl.Span.End {
			holding = append(holding, decl)
		}
	}

	if len(holding) == 0 {
		return fileDecls
	}

	return holding
}

// firstMatch is the first file, by name, under dir that pattern matches and
// that holds a declaration. A pattern naming a directory matches everything
// beneath it, minus names starting with "." or "_" unless the pattern says
// "all:" - the same rule go applies.
func firstMatch(
	files []string,
	byFile map[string][]entities.Decl,
	dir, pattern string,
) (entities.DeclKey, bool) {
	includeHidden := strings.HasPrefix(pattern, allPrefix)
	pattern = strings.TrimPrefix(pattern, allPrefix)

	for _, file := range slices.Sorted(slices.Values(files)) {
		rel, inside := relativeTo(dir, file)
		if !inside || len(byFile[file]) == 0 || !embedMatches(pattern, rel, includeHidden) {
			continue
		}

		return byFile[file][0].Key, true
	}

	return entities.DeclKey{}, false
}

func relativeTo(dir, file string) (string, bool) {
	if dir == "." {
		return file, true
	}

	return strings.CutPrefix(file, dir+"/")
}

// embedMatches reports whether rel is, or sits under a directory that is,
// matched by pattern.
func embedMatches(pattern, rel string, includeHidden bool) bool {
	parts := strings.Split(rel, "/")

	for depth := range parts {
		matched, err := path.Match(pattern, strings.Join(parts[:depth+1], "/"))
		if err != nil || !matched {
			continue
		}

		// A file named outright is embedded whatever its name; only what a
		// directory pulls in is filtered.
		return depth == len(parts)-1 || includeHidden || !hiddenAny(parts[depth+1:])
	}

	return false
}

func hiddenAny(parts []string) bool {
	return slices.ContainsFunc(parts, func(part string) bool {
		return strings.HasPrefix(part, ".") || strings.HasPrefix(part, "_")
	})
}
