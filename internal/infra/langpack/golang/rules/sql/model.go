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

func (model *sqlModel) readMigrations(snap entities.Snapshot) error {
	for _, file := range migrationFiles(snap.Files) {
		source, err := declsource.ReadSource(snap, file)
		if err != nil {
			return err
		}

		for _, stmt := range sqlstmt.Split(source) {
			key := entities.DeclKey{File: file, Kind: entities.KindOther, Name: stmt.Name}

			if name, isExtension := strings.CutPrefix(stmt.Name, "extension:"); isExtension {
				model.extensions[name] = key
			}

			if stmt.Kind == sqlstmt.CreateTable {
				model.tables[stmt.Table] = &sqlTable{name: stmt.Table}
				model.order = append(model.order, stmt.Table)
			}

			table, known := model.tables[stmt.Table]
			if !known {
				continue
			}

			table.stmts = append(
				table.stmts, sqlStmt{
					key: key, kind: stmt.Kind, refs: stmt.Refs,
					words: sqlstmt.Idents(source[stmt.Start:stmt.End]),
				},
			)

			for _, ref := range stmt.Refs {
				if ref != stmt.Table && !slices.Contains(table.refs, ref) {
					table.refs = append(table.refs, ref)
				}
			}
		}
	}

	return nil
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

func (model *sqlModel) readQueries(snap entities.Snapshot) error {
	generated := model.generatedByName(snap)

	for _, file := range queryFiles(snap.Files) {
		source, err := declsource.ReadSource(snap, file)
		if err != nil {
			return err
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
