package sql

import (
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/declsource"
	"github.com/wrapped-owls/redigit/internal/infra/langpack/golang/genmark"
	"github.com/wrapped-owls/redigit/internal/infra/sqlstmt"
)

type sqlTable struct {
	name  string
	stmts []sqlStmt
	// refs are the other tables its statements point a foreign key at.
	refs []string
	// entFile is the ORM schema file declaring the table, and ent its decls.
	entFile string
	entType entities.DeclKey
	ent     []entities.DeclKey
	model   entities.DeclKey
}

type sqlStmt struct {
	key  entities.DeclKey
	kind sqlstmt.Kind
	refs []string
	// words are the identifiers the statement mentions, lower-cased: a column of
	// type citext names the extension that defines it.
	words []string
}

func (table *sqlTable) create() (entities.DeclKey, bool) {
	for _, stmt := range table.stmts {
		if stmt.kind == sqlstmt.CreateTable {
			return stmt.key, true
		}
	}

	return entities.DeclKey{}, false
}

type sqlQuery struct {
	name   string
	block  entities.DeclKey
	tables []string
	// extensions are the migration statements that create the extensions
	// whose names the query mentions: sqlc cannot resolve unaccent() without.
	extensions []entities.DeclKey
	generated  []entities.DeclKey
	method     entities.DeclKey
}

type sqlModel struct {
	tables map[string]*sqlTable
	order  []string
	// extensions maps an extension name to the statement creating it.
	extensions map[string]entities.DeclKey
	queries    []*sqlQuery
	// dbInit is the sqlc output every query function and model needs and that
	// belongs to no table or query: the Queries type and its transaction hook.
	dbInit []entities.DeclKey
	// ormDir is the directory of the ORM schema, "" when there is none.
	ormDir string
	decls  map[string][]entities.Decl
}

func loadSQLModel(snap entities.Snapshot, decls []entities.Decl) (sqlModel, error) {
	model := sqlModel{
		tables:     map[string]*sqlTable{},
		extensions: map[string]entities.DeclKey{},
		decls:      declsource.GroupByFile(decls),
	}

	if err := model.readMigrations(snap); err != nil {
		return sqlModel{}, err
	}

	model.joinORM(decls)
	model.joinModels(snap)

	if err := model.readQueries(snap); err != nil {
		return sqlModel{}, err
	}

	return model, nil
}

func (model *sqlModel) joinORM(decls []entities.Decl) {
	for _, decl := range decls {
		if decl.Key.Kind != entities.KindType || !isORMFile(decl.Key.File) {
			continue
		}

		for _, name := range model.order {
			table := model.tables[name]
			if table.entFile != "" || !tableNamesType(name, decl.Key.Name) {
				continue
			}

			table.entFile, table.entType = decl.Key.File, decl.Key
			model.ormDir = path.Dir(decl.Key.File)

			for _, member := range model.decls[decl.Key.File] {
				table.ent = append(table.ent, member.Key)
			}

			break
		}
	}
}

func isORMFile(file string) bool { return strings.Contains(file, "entschema/") }

func (model *sqlModel) joinModels(snap entities.Snapshot) {
	for _, file := range sortedFiles(model.decls) {
		if !isDBGenOutput(snap, file) || strings.HasSuffix(file, ".sql.go") {
			continue
		}

		structs := 0

		for _, decl := range model.decls[file] {
			if decl.Key.Kind != entities.KindType {
				continue
			}

			if table, found := model.tableOfType(decl.Key.Name); found {
				table.model = decl.Key
				structs++
			}
		}

		if structs > 0 {
			continue
		}

		for _, decl := range model.decls[file] {
			model.dbInit = append(model.dbInit, decl.Key)
		}
	}
}

func (model *sqlModel) tableOfType(typeName string) (*sqlTable, bool) {
	for _, name := range model.order {
		if tableNamesType(name, typeName) {
			return model.tables[name], true
		}
	}

	return nil, false
}

func isDBGenOutput(snap entities.Snapshot, file string) bool {
	if !strings.Contains(file, "dbgen/") || !strings.HasSuffix(file, ".go") {
		return false
	}

	source, err := declsource.ReadSource(snap, file)

	return err == nil && genmark.IsSqlcOutput(source)
}

func sortedFiles(byFile map[string][]entities.Decl) []string {
	files := make([]string, 0, len(byFile))
	for file := range byFile {
		files = append(files, file)
	}

	slices.Sort(files)

	return files
}
