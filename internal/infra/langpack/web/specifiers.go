package web

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
)

// The scan is textual on purpose: it reads a .svelte file's markup along with
// its script, so a "from '...'" written in prose would count as an import. A
// file holding one is rare, and an extra edge only holds the reader back.
var (
	fromClause   = regexp.MustCompile(`\bfrom\s*['"]([^'"\n]+)['"]`)
	bareImport   = regexp.MustCompile(`\bimport\s*['"]([^'"\n]+)['"]`)
	dynamicCall  = regexp.MustCompile(`\bimport\(\s*['"]([^'"\n]+)['"]\s*\)`)
	visitedRoute = regexp.MustCompile("['\"`](/[a-z][a-z0-9-]*(?:/[a-z0-9-]+)*)/?['\"`?#]")
)

// specGroup is the capture group each import pattern puts the specifier in.
const specGroup = 1

// specifiers lists, in order of appearance, every module a file names.
func specifiers(source string) []string {
	type found struct {
		at   int
		spec string
	}

	var all []found

	for _, pattern := range []*regexp.Regexp{fromClause, bareImport, dynamicCall} {
		for _, match := range pattern.FindAllStringSubmatchIndex(source, -1) {
			spec := match[specGroup*2 : specGroup*2+2]
			all = append(all, found{at: match[0], spec: source[spec[0]:spec[1]]})
		}
	}

	slices.SortFunc(all, func(a, b found) int { return cmp.Compare(a.at, b.at) })

	out := make([]string, 0, len(all))

	for _, each := range all {
		out = append(out, each.spec)
	}

	return out
}

// visits lists the app paths a test reaches for in a string literal, such as
// "/conversas" or `/funil?periodo=`.
func visits(source string) []string {
	matches := visitedRoute.FindAllStringSubmatch(source, -1)
	out := make([]string, 0, len(matches))

	for _, match := range matches {
		out = append(out, strings.TrimSuffix(match[1], "/"))
	}

	return out
}
