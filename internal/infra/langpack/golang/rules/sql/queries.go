package sql

import (
	"regexp"
	"strings"
)

var queryNamePattern = regexp.MustCompile(`(?m)^-- name:\s+(\w+)`)

func extractQueryNames(source []byte) []string {
	matches := queryNamePattern.FindAllSubmatch(source, -1)

	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = string(m[1])
	}

	return names
}

func isQueryFile(path string) bool {
	return strings.Contains(path, "database/queries/") && strings.HasSuffix(path, ".sql")
}

func queryFiles(files []string) []string {
	var out []string

	for _, f := range files {
		if isQueryFile(f) {
			out = append(out, f)
		}
	}

	return out
}

type queryBlock struct {
	name string
	text []byte
}

func extractQueryBlocks(source []byte) []queryBlock {
	matches := queryNamePattern.FindAllSubmatchIndex(source, -1)
	blocks := make([]queryBlock, len(matches))

	for index, match := range matches {
		end := len(source)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}

		blocks[index] = queryBlock{
			name: string(source[match[2]:match[3]]),
			text: source[match[0]:end],
		}
	}

	return blocks
}
