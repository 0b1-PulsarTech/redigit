package chunk

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/domain/usecases/indexing"
	"github.com/wrapped-owls/redigit/internal/infra/sqlstmt"
)

// SQLMigrations splits a migration file into one Decl per statement. A
// migration is edited in place as the schema grows, so a table's CREATE TABLE
// is born with the feature that first persists it. Whole, the file lands
// before any table in it has a reader.
type SQLMigrations struct{}

var _ indexing.Chunker = SQLMigrations{}

func (SQLMigrations) Name() string { return "sql-migrations" }

// Handles reports whether path is a ".sql" file under a "migrations"
// directory, wherever that directory sits.
func (SQLMigrations) Handles(path string) bool {
	if !strings.HasSuffix(path, ".sql") {
		return false
	}

	dir := filepath.ToSlash(filepath.Dir(path))

	return slices.Contains(strings.Split(dir, "/"), "migrations")
}

func (SQLMigrations) Chunks(
	_ context.Context,
	path string,
	source []byte,
) ([]entities.Decl, error) {
	statements := sqlstmt.Split(source)
	if len(statements) == 0 {
		return []entities.Decl{wholeFileDecl(path, source)}, nil
	}

	lines := splitLines(source)
	unit := entities.UnitID(filepath.Dir(path))
	decls := make([]entities.Decl, len(statements))

	for index, stmt := range statements {
		decls[index] = entities.Decl{
			Key:      entities.DeclKey{File: path, Kind: entities.KindOther, Name: stmt.Name},
			Unit:     unit,
			Span:     entities.Span{Start: stmt.Start, End: stmt.End},
			Selector: locationAt(path, source, lines, stmt.Start),
		}
	}

	return decls, nil
}
