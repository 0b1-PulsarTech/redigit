package indexing

import (
	"bytes"
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// measure records on each declaration how many lines it adds and whether its
// file is generated, so the planner can cap a commit without reading a file.
//
// A declaration owns the lines up to the next one. What precedes the first
// (package clause, imports) is the file's Frame instead: summing declarations
// alone put 12 test files at 853 lines in a commit that added 1088.
func measure(file string, source []byte, decls []entities.Decl) {
	generated := IsGenerated(file, source)

	var measured []int

	for index := range decls {
		span := decls[index].Span
		if span.Start < 0 || span.End > len(source) || span.Start > span.End {
			continue
		}

		decls[index].Generated = generated

		measured = append(measured, index)
	}

	// Stable, so the member of a shared span that carries its lines is the first in declaration order.
	slices.SortStableFunc(measured, func(a, b int) int {
		return cmp.Compare(decls[a].Span.Start, decls[b].Span.Start)
	})

	for position, index := range measured {
		decls[index].Lines = 0

		// Members of one atom share a span; the planner sums lines, so only one may hold them.
		if position > 0 && decls[measured[position-1]].Span == decls[index].Span {
			continue
		}

		end := len(source)

		for _, next := range measured[position+1:] {
			if decls[next].Span != decls[index].Span {
				end = decls[next].Span.Start

				break
			}
		}

		decls[index].Lines = lineCount(
			source[decls[index].Span.Start:max(end, decls[index].Span.End)],
		)
	}

	if len(measured) == 0 {
		return
	}

	frame := bytes.Count(source[:decls[measured[0]].Span.Start], []byte{'\n'})
	for _, index := range measured {
		decls[index].Frame = frame
	}
}

// lineCount counts the lines of text, the last one included when it has no
// newline.
func lineCount(text []byte) int {
	count := bytes.Count(text, []byte{'\n'})
	if len(text) > 0 && text[len(text)-1] != '\n' {
		count++
	}

	return count
}

// checksumFiles are written by a tool and carry no header to say so.
var checksumFiles = []string{"go.sum", "atlas.sum"}

// IsGenerated applies one rule to every language: a checksum file by name, a
// Go file by the "// Code generated ... DO NOT EDIT." line the toolchain
// defines, any other file by a first line that says it is generated. A rule
// per repository would be a special case that goes stale; these three are what
// a generator announces itself with.
func IsGenerated(file string, source []byte) bool {
	if slices.Contains(checksumFiles, path.Base(file)) {
		return true
	}

	if path.Ext(file) == ".go" {
		return hasGoMarker(source)
	}

	first, _, _ := bytes.Cut(source, []byte{'\n'})

	return bytes.Contains(bytes.ToLower(first), []byte("generated"))
}

// hasGoMarker reads up to the package clause, the last place the toolchain
// accepts the marker.
func hasGoMarker(source []byte) bool {
	for line := range bytes.Lines(source) {
		text := strings.TrimRight(string(line), "\r\n")
		if strings.HasPrefix(text, "package ") {
			return false
		}

		if strings.HasPrefix(text, "// Code generated ") &&
			strings.HasSuffix(text, " DO NOT EDIT.") {
			return true
		}
	}

	return false
}
