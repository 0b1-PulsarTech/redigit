package chunk

import (
	"bytes"
	"context"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
)

// queryNamePattern matches a sqlc "-- name: GetUser :one" line, the only
// place a query file names the query it holds.
var queryNamePattern = regexp.MustCompile(`(?m)^-- name:\s+(\w+)`)

// Index of the name capture group's bounds inside one FindAllSubmatchIndex
// entry: [0] and [1] hold the whole match, [2] and [3] the first group.
const (
	nameGroupStart = 2
	nameGroupEnd   = 3
)

// SQLQueries splits a sqlc query file into one Decl per "-- name:" block, the
// same granularity a generated Go file gets per function. Twenty queries in
// one file no longer have to be born together just because the first commit
// only reads three of them.
type SQLQueries struct{}

var _ indexing.Chunker = SQLQueries{}

func (SQLQueries) Name() string { return "sql-queries" }

// Handles reports whether path is a sqlc query source: a ".sql" file under a
// "queries" directory, wherever that directory sits.
func (SQLQueries) Handles(path string) bool {
	if !strings.HasSuffix(path, ".sql") {
		return false
	}

	dir := filepath.ToSlash(filepath.Dir(path))

	return slices.Contains(strings.Split(dir, "/"), "queries")
}

func (SQLQueries) Chunks(
	_ context.Context,
	path string,
	source []byte,
) ([]entities.Decl, error) {
	matches := queryNamePattern.FindAllSubmatchIndex(source, -1)
	if len(matches) == 0 {
		return []entities.Decl{wholeFileDecl(path, source)}, nil
	}

	lines := splitLines(source)
	unit := entities.UnitID(filepath.Dir(path))
	starts := queryStarts(source, lines, matches)

	decls := make([]entities.Decl, len(matches))

	for i, match := range matches {
		end := len(source)
		if i+1 < len(matches) {
			end = starts[i+1]
		}

		start := starts[i]
		if i == 0 {
			// The first query claims any file-level preamble too, the same
			// way the first require/replace line in a go.mod never does:
			// there is no earlier declaration left to own it.
			start = 0
		}

		nameStart, nameEnd := match[nameGroupStart], match[nameGroupEnd]

		decls[i] = entities.Decl{
			Key: entities.DeclKey{
				File: path,
				Kind: entities.KindOther,
				Name: string(source[nameStart:nameEnd]),
			},
			Unit:     unit,
			Span:     entities.Span{Start: start, End: end},
			Selector: locationAt(path, source, lines, nameStart),
		}
	}

	return decls, nil
}

// queryStarts returns, for each regexp match in order, the byte offset its
// Decl's span should start at: the "-- name:" line itself, or the comment
// block sitting directly above it with no blank line in between. A comment
// separated by a blank line reads as a section header, not this query's doc
// comment, the same distinction Go draws between a doc comment and prose.
func queryStarts(source []byte, lines []entities.Span, matches [][]int) []int {
	starts := make([]int, len(matches))
	lineIdx := 0

	for i, match := range matches {
		nameLineStart := match[0]

		for lineIdx < len(lines) && lines[lineIdx].Start != nameLineStart {
			lineIdx++
		}

		starts[i] = leadingCommentStart(source, lines, lineIdx)
	}

	return starts
}

func leadingCommentStart(source []byte, lines []entities.Span, nameLineIdx int) int {
	result := lines[nameLineIdx].Start

	for i := nameLineIdx - 1; i >= 0; i-- {
		trimmed := bytes.TrimRight(source[lines[i].Start:lines[i].End], "\r\n")
		if len(trimmed) == 0 || !bytes.HasPrefix(trimmed, []byte("--")) ||
			bytes.HasPrefix(trimmed, []byte("-- name:")) {
			break
		}

		result = lines[i].Start
	}

	return result
}
