package golang

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

// isDBGenFile reports whether path is a sqlc-generated Go source, dbgen/*.sql.go.
func isDBGenFile(path string) bool {
	return strings.Contains(path, "dbgen/") && strings.HasSuffix(path, ".sql.go")
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
