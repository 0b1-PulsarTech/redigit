package golang

import (
	"path"
	"slices"
	"strings"

	"github.com/wrapped-owls/redigit/internal/domain/entities"
	"github.com/wrapped-owls/redigit/internal/infra/sqlstmt"
)

// sqlTable is one table seen from every file that describes it: the
// statements that build it, the ORM type that declares it and the struct
// sqlc maps it to.
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
}

// create is the statement that creates the table: every other statement about
// it, and every query that reads it, waits for this one.
func (table *sqlTable) create() (entities.DeclKey, bool) {
	for _, stmt := range table.stmts {
		if stmt.kind == sqlstmt.CreateTable {
			return stmt.key, true
		}
	}

	return entities.DeclKey{}, false
}

// sqlQuery is one "-- name:" block, the tables it reads and the generated
// declarations sqlc wrote for it.
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

// sqlModel is the schema as the migration files, the ORM schema, the query
// files and the sqlc output describe it together.
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

// loadSQLModel reads every migration and query file the snapshot lists and
// joins them to the declarations that realise them. A part that is absent
// leaves its field empty: a repository with no ORM schema has tables without
// an ent type, and the rules below skip what is missing.
func loadSQLModel(snap entities.Snapshot, decls []entities.Decl) (sqlModel, error) {
	model := sqlModel{
		tables:     map[string]*sqlTable{},
		extensions: map[string]entities.DeclKey{},
		decls:      groupByFile(decls),
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
		source, err := readSource(snap, file)
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
				table.stmts, sqlStmt{key: key, kind: stmt.Kind, refs: stmt.Refs},
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

// joinORM finds each table's ORM schema type: a type declared under an
// "entschema" directory whose name is the table's, singular or plural.
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

// joinModels finds the struct sqlc wrote for each table, and collects the
// generated declarations that serve no table: they are in files sqlc wrote
// that are neither a query file's nor the models file.
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

// isDBGenOutput reports whether file is Go that sqlc wrote under a "dbgen"
// directory.
func isDBGenOutput(snap entities.Snapshot, file string) bool {
	if !strings.Contains(file, "dbgen/") || !strings.HasSuffix(file, ".go") {
		return false
	}

	source, err := readSource(snap, file)

	return err == nil && isSqlcOutput(source)
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
		source, err := readSource(snap, file)
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

// generatedByName indexes the declarations in sqlc's query output by the name
// they carry, so a query finds its function, its constant and its row types
// without a scan of the whole tree per query.
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

// attachGenerated gathers what sqlc wrote for a query: the function, the
// constant holding its text and the parameter and row types, which carry the
// query's name with a suffix.
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

// ownsQueryName separates the declaration sqlc wrote for query from a
// same-named one of another kind.
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

// tableNamesType reports whether typeName is the name an ORM or a code
// generator gives the type for table: the same words, in the singular or the
// plural, without the underscores.
func tableNamesType(table, typeName string) bool {
	want := strings.ToLower(strings.ReplaceAll(table, "_", ""))
	have := strings.ToLower(typeName)

	if want == have || want == have+"s" || want == have+"es" {
		return true
	}

	stem, plural := strings.CutSuffix(have, "y")

	return plural && want == stem+"ies"
}
