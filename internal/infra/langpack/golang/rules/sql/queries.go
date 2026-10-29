package sql

import (
	"regexp"
	"strings"
)

// queryNamePattern matches a sqlc "-- name: GetUser :one" line, the only
// place a query file names the query it holds. Neither the query nor the
// generated Go function that comes from it otherwise mentions the other.
var queryNamePattern = regexp.MustCompile(`(?m)^-- name:\s+(\w+)`)

// extractQueryNames returns every query name a sqlc query file declares, in
// the order they appear.
func extractQueryNames(source []byte) []string {
	matches := queryNamePattern.FindAllSubmatch(source, -1)

	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = string(m[1])
	}

	return names
}

// isQueryFile reports whether path is a sqlc query source, database/queries/*.sql.
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

// queryBlock is one sqlc query: its name and its text, which runs from its
// "-- name:" line to the next one.
type queryBlock struct {
	name string
	text []byte
}

// extractQueryBlocks cuts a query file at its "-- name:" lines.
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
