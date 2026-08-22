package golang

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
)

// migrationRule places a migration between the ORM schema it was generated
// from and the queries that read the table it creates: after the schema,
// before the queries. Fixing only the query side pushed 9 of 12 migrations
// ahead of their own source, one by 735 commits - the schema side is not
// optional.
type migrationRule struct{}

func (migrationRule) Name() string { return "migration" }

// createTablePattern matches a CREATE TABLE statement's name, optionally
// backticked, optionally guarded by IF NOT EXISTS.
var createTablePattern = regexp.MustCompile(
	`(?i)CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+` + "`?" + `(\w+)` + "`?",
)

func (migrationRule) Edges(
	_ context.Context,
	snap entities.Snapshot,
	decls []entities.Decl,
) ([]entities.Edge, error) {
	var edges []entities.Edge

	for _, mf := range migrationFiles(snap.Files) {
		source, err := readSource(snap, mf)
		if err != nil {
			return nil, err
		}

		for _, table := range createTablePattern.FindAllSubmatch(source, -1) {
			found, err := tableEdges(snap, decls, mf, string(table[1]))
			if err != nil {
				return nil, err
			}

			edges = append(edges, found...)
		}
	}

	return edges, nil
}

// tableEdges links one migration-created table both ways: to the schema
// declaration it was generated from, and forward to every query that
// mentions the table, which can only run once the migration has landed.
func tableEdges(
	snap entities.Snapshot,
	decls []entities.Decl,
	migrationFile, table string,
) ([]entities.Edge, error) {
	migrationKey := entities.DeclKey{File: migrationFile, Kind: entities.KindOther, Name: table}

	var edges []entities.Edge

	if schemaKey, found := findSchemaDecl(decls, table); found {
		edges = append(edges, entities.Edge{
			Dependent: migrationKey,
			Provider:  schemaKey,
			Reason:    entities.ReasonSchema,
		})
	}

	for _, qf := range queryFiles(snap.Files) {
		source, err := readSource(snap, qf)
		if err != nil {
			return nil, err
		}

		// simplification: a query file "reads the migration" when its text
		// mentions the table anywhere, not per matched query - sqlc query
		// files are conventionally one file per entity, so this is close to
		// exact without re-parsing each query's own SQL body.
		if !strings.Contains(strings.ToLower(string(source)), strings.ToLower(table)) {
			continue
		}

		for _, name := range extractQueryNames(source) {
			edges = append(edges, entities.Edge{
				Dependent: entities.DeclKey{File: qf, Kind: entities.KindOther, Name: name},
				Provider:  migrationKey,
				Reason:    entities.ReasonQuery,
			})
		}
	}

	return edges, nil
}

// findSchemaDecl matches a created table to the ORM schema type that
// generated it: a KindType declaration under an entschema directory, named
// for the table (exactly, or singularized by trimming a trailing "s").
func findSchemaDecl(decls []entities.Decl, table string) (entities.DeclKey, bool) {
	candidates := []string{table, strings.TrimSuffix(table, "s")}

	for _, d := range decls {
		if d.Key.Kind != entities.KindType ||
			!strings.Contains(filepath.ToSlash(d.Key.File), "entschema/") {
			continue
		}

		for _, name := range candidates {
			if strings.EqualFold(d.Key.Name, name) {
				return d.Key, true
			}
		}
	}

	return entities.DeclKey{}, false
}

func migrationFiles(files []string) []string {
	var out []string

	for _, f := range files {
		if strings.Contains(f, "migrations/") && strings.HasSuffix(f, ".sql") {
			out = append(out, f)
		}
	}

	return out
}

func (migrationRule) Anchors(
	context.Context,
	entities.Snapshot,
	[]entities.Decl,
) ([]entities.Anchor, error) {
	return nil, nil
}
