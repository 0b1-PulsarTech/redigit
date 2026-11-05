package wiring

import (
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

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
