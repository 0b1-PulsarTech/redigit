package sql

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
	"github.com/wrapped-owls/redigit/internal/infra/sqlstmt"
)

var queryNamePattern = regexp.MustCompile(`(?m)^-- name:\s+(\w+)`)

func extractQueryNames(source []byte) []string {
	matches := queryNamePattern.FindAllSubmatch(source, -1)

	names := make([]string, len(matches))
	for index, match := range matches {
		names[index] = string(match[1])
	}

	return names
}

func isQueryFile(path string) bool {
	return strings.Contains(path, "database/queries/") && strings.HasSuffix(path, ".sql")
}

func queryFiles(files []string) []string {
	var out []string

	for _, file := range files {
		if isQueryFile(file) {
			out = append(out, file)
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

func (model *sqlModel) readQueries(snap entities.Snapshot) error {
	generated := model.generatedByName(snap)

	for _, file := range queryFiles(snap.Files) {
		source, err := declsource.ReadSource(snap, file)
		if err != nil {
			return fmt.Errorf("sql query source: %w", err)
		}

		for _, block := range extractQueryBlocks(source) {
			query := &sqlQuery{
				name:  block.name,
				block: entities.DeclKey{File: file, Kind: entities.KindOther, Name: block.name},
			}

			for _, word := range sqlstmt.Idents(block.text) {
				_, isTable := model.tables[word]
				if isTable && !slices.Contains(query.tables, word) {
					query.tables = append(query.tables, word)
				}

				if extension, isExtension := model.extensions[word]; isExtension {
					query.extensions = append(query.extensions, extension)
				}
			}

			model.attachGenerated(query, generated)
			model.queries = append(model.queries, query)
		}
	}

	return nil
}

func (model *sqlModel) generatedByName(snap entities.Snapshot) map[string][]entities.Decl {
	out := map[string][]entities.Decl{}

	for _, file := range sortedFiles(model.decls) {
		if !strings.HasSuffix(file, ".sql.go") || !isDBGenOutput(snap, file) {
			continue
		}

		for _, decl := range model.decls[file] {
			name := decl.Key.Name
			if decl.Key.Kind == entities.KindMethod {
				_, name, _ = strings.Cut(name, ".")
			}

			out[name] = append(out[name], decl)
		}
	}

	return out
}

func (model *sqlModel) attachGenerated(query *sqlQuery, generated map[string][]entities.Decl) {
	for _, name := range []string{
		query.name, lowerFirst(query.name), query.name + "Params", query.name + "Row",
	} {
		for _, decl := range generated[name] {
			if !ownsQueryName(decl.Key.Kind, name, query.name) {
				continue
			}

			query.generated = append(query.generated, decl.Key)

			if decl.Key.Kind == entities.KindMethod {
				query.method = decl.Key
			}
		}
	}
}

func ownsQueryName(kind entities.DeclKind, matched, query string) bool {
	switch kind {
	case entities.KindMethod:
		return matched == query
	case entities.KindValue:
		return matched == query || matched == lowerFirst(query)
	case entities.KindType:
		return strings.HasPrefix(matched, query) && matched != query
	default:
		return false
	}
}

func lowerFirst(name string) string {
	if name == "" {
		return name
	}

	return strings.ToLower(name[:1]) + name[1:]
}

func tableNamesType(table, typeName string) bool {
	want := strings.ToLower(strings.ReplaceAll(table, "_", ""))
	have := strings.ToLower(typeName)

	if want == have || want == have+"s" || want == have+"es" {
		return true
	}

	stem, plural := strings.CutSuffix(have, "y")

	return plural && want == stem+"ies"
}
